package supervisor_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/adapters/fake"
	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workers"
	"github.com/turbokast/mythhelm/internal/workspace"
)

func TestOwnerLockHeldByLiveProcess(t *testing.T) {
	f := newFixture(t)
	if code, _, stderr := f.run(t, f.fakeRun()...); code != 5 {
		t.Fatalf("run exit %d: %s", code, stderr)
	}
	runID := f.onlyRun(t).RunID
	release, err := supervisor.AcquireOwner(filepath.Join(f.state, "runs", runID))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	code, _, stderr := f.run(t, "recover", runID)
	if code != 6 {
		t.Fatalf("recover with live owner exit %d: %s", code, stderr)
	}
}

func TestStopReportsRequestedUntilConfirmed(t *testing.T) {
	f := newFixture(t)
	r := startLive(t, f, f.fakeRun("--format", "jsonl", "--scenario", "ignore-sigint")...)
	r.waitStdout(t, "native session", isType("attempt.native_session"))
	runID := f.onlyRun(t).RunID
	attemptID := findAttempt(t, f, runID)
	dir := workers.AttemptDir(f.state, runID, attemptID)
	waited := false
	t.Cleanup(func() {
		if waited {
			return
		}
		if err := workers.RequestStop(dir, "test-cleanup"); err != nil {
			t.Error(err)
			return
		}
		waitSpool(t, filepath.Join(dir, supervisor.SpoolFile), `"type":"attempt.stopped"`)
		_, _, _ = r.wait(t)
	})
	code, out, stderr := f.run(t, "stop", runID, "--format", "jsonl", "--non-interactive")
	if code != 0 {
		t.Fatalf("stop exit %d: %s", code, stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result["type"] != "stop.result" || result["state"] != "stop_requested" || result["exit_code"] != float64(0) {
		t.Fatalf("stop prematurely reported confirmation: %v", result)
	}
	waited = true
	if code, _, stderr := r.wait(t); code != 130 {
		t.Fatalf("stopped run exit %d: %s", code, stderr)
	}
	code, out, stderr = f.run(t, "stop", runID, "--format", "jsonl")
	if code != 0 {
		t.Fatalf("confirmed stop exit %d: %s", code, stderr)
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result["state"] != "stopped" {
		t.Fatalf("confirmed stop result: %v", result)
	}
}

// seedLaunch stops the supervisor's durable history immediately after intent,
// then starts exactly one real worker without ingesting its acknowledgement.
// This makes the pre-ack crash boundary deterministic without timing a kill.
func seedLaunch(t *testing.T, f fixture, scenario string, trustConfig ...string) (admission.Decision, *os.Process, func()) {
	t.Helper()
	noChecks, digest := true, ""
	if len(trustConfig) > 0 {
		noChecks, digest = false, trustConfig[0]
	}
	d, err := admission.Decide(t.Context(), admission.Request{TrustProjectConfig: digest, StateDir: f.state, Repo: f.repo, TaskFile: f.task, Adapter: "fake", Billing: "local-scripted", ExecutionProfile: "trusted-host", NoChecks: noChecks, Scenario: scenario, Env: os.Environ()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(d.RunDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d.RunDir, "task.md"), d.Task.Content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := workspace.Snapshot(t.Context(), d.Snapshot.SourceRepo, d.Snapshot.BaseRev, d.Workdir); err != nil {
		t.Fatal(err)
	}
	j := f.journal(t)
	prod := supervisor.NewProducer(ids.New("sup"), 1)
	if err := supervisor.CreateRun(t.Context(), j, journal.RunRow{RunID: d.RunID, AdapterID: d.Adapter.ID, SourceRepo: d.Snapshot.SourceRepo, SourceBranch: d.Snapshot.Branch, BaseRev: d.Snapshot.BaseRev, TaskSHA256: d.Task.SHA256, BillingPosture: d.Proposal.Billing.Mode, ExecutionProfile: d.Profile.Name}, prod); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.TransitionRun(t.Context(), j, d.RunID, supervisor.RunAdmission, "", prod); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(d.Record())
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Append(t.Context(), journal.Event{SchemaVersion: 1, EventID: ids.New("evt"), RunID: d.RunID, Type: "admission.decided", ProducerID: ids.New("fixture"), ProducerSequence: 1, Generation: 1, ObservedAt: time.Now().UTC(), Payload: payload}, nil); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.TransitionRun(t.Context(), j, d.RunID, supervisor.RunExecuting, "", prod); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("1", 64)
	sum := sha256.Sum256([]byte(token))
	if err := supervisor.RecordLaunchIntent(t.Context(), j, journal.AttemptRow{AttemptID: d.AttemptID, RunID: d.RunID, TaskID: d.TaskID, AttemptNumber: 1, LaunchTokenSHA256: hex.EncodeToString(sum[:]), WorkspacePath: d.Workdir}, prod); err != nil {
		t.Fatal(err)
	}
	proc, err := workers.Spawn(os.Args[0], f.state, d.RunID, d.AttemptID, workers.Launch{LaunchToken: token, TaskID: d.TaskID, AdapterID: d.Adapter.ID, Path: d.Proposal.Spec.Path, Args: d.Proposal.Spec.Args, Dir: d.Proposal.Spec.Dir, Env: d.Proposal.Spec.Env, PromptPath: filepath.Join(d.RunDir, "task.md"), StopLadder: d.Proposal.StopLadder})
	if err != nil {
		t.Fatal(err)
	}
	waited := false
	wait := func() {
		if !waited {
			waited = true
			if _, err := proc.Wait(); err != nil {
				t.Error(err)
			}
		}
	}
	t.Cleanup(func() {
		if !waited {
			if err := workers.RequestStop(workers.AttemptDir(f.state, d.RunID, d.AttemptID), "test-cleanup"); err != nil {
				t.Error(err)
			}
			wait()
		}
	})
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := workers.ReadIdentity(workers.AttemptDir(f.state, d.RunID, d.AttemptID)); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker identity unavailable")
		}
		time.Sleep(20 * time.Millisecond)
	}
	return d, proc, wait
}

func TestCrashAfterLaunchIntentBeforeAck(t *testing.T) {
	f := newFixture(t)
	d, proc, wait := seedLaunch(t, f, "slow")
	if a, err := f.journal(t).LatestAttempt(t.Context(), d.RunID); err != nil || a.State != "launch_intent_recorded" {
		t.Fatalf("crash boundary %v %v", a, err)
	}
	r := startLive(t, f, "recover", d.RunID, "--format", "jsonl", "--non-interactive")
	r.waitStdout(t, "reattached worker acknowledgement", isType("attempt.launched"))
	id, err := workers.ReadIdentity(workers.AttemptDir(f.state, d.RunID, d.AttemptID))
	if err != nil {
		t.Fatal(err)
	}
	if id.PID != proc.Pid {
		t.Fatalf("worker replaced: %d -> %d", proc.Pid, id.PID)
	}
	if err := workers.RequestStop(workers.AttemptDir(f.state, d.RunID, d.AttemptID), "test-stop"); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := r.wait(t)
	wait()
	if code != 130 || !strings.Contains(stderr, "reattached to worker") {
		t.Fatalf("recover exit %d: %s", code, stderr)
	}
	count := 0
	for _, ev := range f.events(t, d.RunID) {
		if ev.Type == "attempt.launched" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("worker acknowledgements %d", count)
	}
}

func TestRecoverContinuesAfterNativeExit(t *testing.T) {
	f := newFixture(t)
	d, _, wait := seedLaunch(t, f, "happy")
	wait()
	code, _, stderr := f.run(t, "recover", d.RunID, "--format", "jsonl")
	if code != 5 {
		t.Fatalf("recover exit %d: %s", code, stderr)
	}
	if got := f.onlyRun(t); got.State != "ready_for_review" || got.Reason != "unverified" {
		t.Fatalf("run %v", got)
	}
	if code, _, stderr := f.run(t, "recover", d.RunID); code != 5 {
		t.Fatalf("repeat recovery %d: %s", code, stderr)
	}
	count := 0
	for _, ev := range f.events(t, d.RunID) {
		if ev.Type == "candidate.frozen" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("candidate frozen %d times", count)
	}
}

func truncateBeforeExit(t *testing.T, f fixture, d admission.Decision) {
	t.Helper()
	path := filepath.Join(workers.AttemptDir(f.state, d.RunID, d.AttemptID), supervisor.SpoolFile)
	raw, err := os.ReadFile(path) // #nosec G304 -- test-owned temporary spool
	if err != nil {
		t.Fatal(err)
	}
	var prefix []byte
	for _, line := range bytes.Split(raw, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var ev journal.Event
		if err := json.Unmarshal(line, &ev); err != nil {
			t.Fatal(err)
		}
		if ev.Type == "attempt.native_result" {
			break
		}
		prefix = append(prefix, line...)
		prefix = append(prefix, '\n')
	}
	err = os.WriteFile(path, prefix, 0o600) // #nosec G703 -- test-owned temporary spool
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecoverQuarantinesLostWorker(t *testing.T) {
	f := newFixture(t)
	d, _, wait := seedLaunch(t, f, "happy")
	wait()
	truncateBeforeExit(t, f, d)
	if code, _, stderr := f.run(t, "recover", d.RunID); code != 6 {
		t.Fatalf("lost recover %d: %s", code, stderr)
	}
	if got := f.onlyRun(t); got.State != "interrupted" {
		t.Fatalf("lost run %v", got)
	}
	a, err := f.journal(t).LatestAttempt(t.Context(), d.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if a.State != "quarantined" {
		t.Fatalf("attempt %s", a.State)
	}
	if _, err := f.journal(t).Candidate(t.Context(), d.AttemptID); !errors.Is(err, journal.ErrNotFound) {
		t.Fatalf("lost writer got candidate: %v", err)
	}
	if code, _, stderr := f.run(t, f.fakeRun()...); code != 3 || !strings.Contains(stderr, "active_run_exists") {
		t.Fatalf("new writer admitted %d %s", code, stderr)
	}
}

func TestWorkerPIDReuseTreatedAsLost(t *testing.T) {
	f := newFixture(t)
	d, _, wait := seedLaunch(t, f, "happy")
	wait()
	truncateBeforeExit(t, f, d)
	dir := workers.AttemptDir(f.state, d.RunID, d.AttemptID)
	id, err := workers.ReadIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	// The current test process is live. Any erroneous recovery signal would
	// kill this test; its deliberately forged start time must be rejected.
	id.PID = os.Getpid()
	id.StartTime = time.Unix(1, 0).UTC()
	raw, err := json.Marshal(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worker.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := f.run(t, "recover", d.RunID); code != 6 {
		t.Fatalf("reused PID %d: %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "stop.request")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("forged PID received stop request: %v", err)
	}
	a, err := f.journal(t).LatestAttempt(t.Context(), d.RunID)
	if err != nil || a.State != "quarantined" {
		t.Fatalf("reused attempt %v %v", a, err)
	}
}

func TestRecoverNeverRelaunchesNative(t *testing.T) {
	f := newFixture(t)
	d, _, wait := seedLaunch(t, f, "count-launches")
	wait()
	for i := 0; i < 2; i++ {
		if code, _, stderr := f.run(t, "recover", d.RunID); code != 5 {
			t.Fatalf("recover %d: %s", code, stderr)
		}
	}
	raw, err := os.ReadFile(filepath.Join(d.Workdir, "launch-count.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "launch\n" {
		t.Fatalf("native launch count %q", raw)
	}
}

func TestHeadlessPermissionDenialNoHang(t *testing.T) {
	f := newFixture(t)
	if code, _, stderr := f.run(t, f.fakeRun("--scenario", "denied", "--non-interactive")...); code != 5 {
		t.Fatalf("denied run %d: %s", code, stderr)
	}
	raw, err := os.ReadFile(filepath.Join(f.state, "runs", f.onlyRun(t).RunID, "receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt map[string]any
	if err := json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	denials := receipt["native_result"].(map[string]any)["permission_denials"].([]any)
	if !slices.Contains(denials, any("Bash")) {
		t.Fatalf("missing denial: %v", denials)
	}
}

func TestLockedDatabaseStopsAdmission(t *testing.T) {
	f := newFixture(t)
	j := f.journal(t)
	_ = j
	path := filepath.ToSlash(filepath.Join(f.state, "mythhelm.db"))
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	db, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: path}).String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(t.Context(), "BEGIN EXCLUSIVE"); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), "ROLLBACK") }()
	code, _, stderr := f.run(t, f.fakeRun()...)
	if code != 3 {
		t.Fatalf("locked admission %d: %s", code, stderr)
	}
	entries, err := filepath.Glob(filepath.Join(f.state, "runs", "*", "attempts", "*", "worker.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("worker spawned with locked DB: %v", entries)
	}
}

// streamFaultLauncher runs the scripted fake on a pipe in this process so
// the decoder's heap use can be measured, in addition to each real CLI run.
type streamFaultLauncher struct{ scenario string }
type streamFaultProc struct {
	out  *io.PipeReader
	done chan adapter.NativeExit
}

func (l streamFaultLauncher) Launch(_ context.Context, spec adapter.ProcSpec) (adapter.OwnedProc, error) {
	r, w := io.Pipe()
	p := &streamFaultProc{out: r, done: make(chan adapter.NativeExit, 1)}
	go func() {
		code := fake.AgentMain([]string{"--scenario", l.scenario, "--workdir", spec.Dir}, strings.NewReader(""), w, io.Discard)
		_ = w.Close()
		p.done <- adapter.NativeExit{Code: code}
	}()
	return p, nil
}
func (p *streamFaultProc) Stdout() io.Reader { return p.out }
func (p *streamFaultProc) Signal(adapter.StopSignal) error {
	return errors.New("unexpected signal in bounded stream test")
}
func (p *streamFaultProc) Wait() adapter.NativeExit { return <-p.done }
func (p *streamFaultProc) GroupGone() bool          { return true }

func TestMalformedDeepOversizedBadUTF8Bounded(t *testing.T) {
	for _, tt := range []struct{ scenario, counter string }{{"malformed", "malformed"}, {"deep", "depth_exceeded"}, {"oversized", "oversized"}, {"bad-utf8", "invalid_utf8"}} {
		t.Run(tt.scenario, func(t *testing.T) {
			f := newFixture(t)
			code, _, stderr := f.run(t, f.fakeRun("--scenario", tt.scenario)...)
			if code != 4 && code != 5 {
				t.Fatalf("fault run %d: %s", code, stderr)
			}
			found := false
			for _, ev := range f.events(t, f.onlyRun(t).RunID) {
				if ev.Type == "attempt.protocol_counters" {
					if n, ok := payloadOf(t, ev)[tt.counter].(float64); ok && n > 0 {
						found = true
					}
				}
			}
			if !found {
				t.Fatalf("missing %s count", tt.counter)
			}
			runtime.GC()
			var before runtime.MemStats
			runtime.ReadMemStats(&before)
			var peak atomic.Uint64
			peak.Store(before.Alloc)
			sampleDone := make(chan struct{})
			sampleStopped := make(chan struct{})
			go func() {
				defer close(sampleStopped)
				tick := time.NewTicker(time.Millisecond)
				defer tick.Stop()
				for {
					select {
					case <-sampleDone:
						return
					case <-tick.C:
						var m runtime.MemStats
						runtime.ReadMemStats(&m)
						if m.Alloc > peak.Load() {
							peak.Store(m.Alloc)
						}
					}
				}
			}()
			sess, err := fake.New().Start(t.Context(), adapter.LaunchProposal{Spec: adapter.ProcSpec{Dir: t.TempDir()}}, streamFaultLauncher{tt.scenario})
			if err != nil {
				close(sampleDone)
				<-sampleStopped
				t.Fatal(err)
			}
			for range sess.Observations() {
			}
			<-sess.Done()
			close(sampleDone)
			<-sampleStopped
			var after runtime.MemStats
			runtime.ReadMemStats(&after)
			if after.Alloc > peak.Load() {
				peak.Store(after.Alloc)
			}
			const heapLimit = 192 << 20
			if peak.Load() > before.Alloc+heapLimit || after.TotalAlloc-before.TotalAlloc > heapLimit {
				t.Fatalf("unbounded decoder: peak delta %d allocated %d", peak.Load()-before.Alloc, after.TotalAlloc-before.TotalAlloc)
			}
			t.Logf("decoder peak heap delta %d; total allocated %d", peak.Load()-before.Alloc, after.TotalAlloc-before.TotalAlloc)
		})
	}
}

func TestRecoverFreezesOnlyAfterDescendantsGone(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux orphan scanner acceptance")
	}
	f := newFixture(t)
	d, _, wait := seedLaunch(t, f, "escapee")
	wait()
	code, _, stderr := f.run(t, "recover", d.RunID)
	if code != 6 {
		t.Fatalf("escaped writer recover %d: %s", code, stderr)
	}
	if _, err := f.journal(t).Candidate(t.Context(), d.AttemptID); !errors.Is(err, journal.ErrNotFound) {
		t.Fatalf("candidate before descendant gone: %v", err)
	}
	pids, err := workers.OrphanPIDs(d.AttemptID)
	if err != nil || len(pids) != 1 {
		t.Fatalf("orphan scan %v %v", pids, err)
	}
	// The test owns this scripted descendant and verifies its recorded start
	// time before killing it. Production recovery only observes it.
	events := f.events(t, d.RunID)
	var identities []workers.ProcessIdentity
	for _, ev := range events {
		if ev.Type == "attempt.stopped" {
			var stop struct {
				Identities []workers.ProcessIdentity `json:"unresolved_identities"`
			}
			if err := json.Unmarshal(ev.Payload, &stop); err != nil {
				t.Fatal(err)
			}
			identities = stop.Identities
		}
	}
	if len(identities) != 1 || identities[0].PID != pids[0] {
		t.Fatalf("descendant identity %v", identities)
	}
	start, err := workers.ProcessStartTime(pids[0])
	if err != nil || !start.Equal(identities[0].StartTime) {
		t.Fatalf("descendant identity changed %v", err)
	}
	proc, err := os.FindProcess(pids[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := proc.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = proc.Release()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := workers.ProcessStartTime(pids[0]); errors.Is(err, workers.ErrNoProcess) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("descendant still alive")
		}
		time.Sleep(20 * time.Millisecond)
	}
	code, _, stderr = f.run(t, "recover", d.RunID)
	if code != 4 {
		t.Fatalf("resolved partial recovery %d: %s", code, stderr)
	}
	c, err := f.journal(t).Candidate(t.Context(), d.AttemptID)
	if err != nil || !c.Partial {
		t.Fatalf("partial candidate %v %v", c, err)
	}
	if got := f.onlyRun(t); got.State != "failed" || got.Reason != "recovered_partial" {
		t.Fatalf("recovered partial labelled %v", got)
	}
}

func TestRecoverRepairsTerminalReceipt(t *testing.T) {
	f := newFixture(t)
	if code, _, stderr := f.run(t, f.fakeRun()...); code != 5 {
		t.Fatalf("run %d %s", code, stderr)
	}
	runID := f.onlyRun(t).RunID
	path := filepath.Join(f.state, "runs", runID, "receipt.json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := f.run(t, "recover", runID); code != 5 {
		t.Fatalf("repair %d: %s", code, stderr)
	}
	if code, _, stderr := f.run(t, "review", runID, "--no-diff"); code != 0 {
		t.Fatalf("repaired review %d: %s", code, stderr)
	}
}

func TestStopDoesNotConfirmFailedDescendantScan(t *testing.T) {
	f := newFixture(t)
	d, _, wait := seedLaunch(t, f, "happy")
	wait()
	payload := json.RawMessage(`{"confirmed":true,"unresolved_pids":[],"descendant_scan":"failed"}`)
	if err := f.journal(t).Append(t.Context(), journal.Event{SchemaVersion: 1, EventID: ids.New("evt"), RunID: d.RunID, AttemptID: d.AttemptID, Type: "attempt.stopped", ProducerID: ids.New("fixture"), ProducerSequence: 1, Generation: 1, ObservedAt: time.Now().UTC(), Payload: payload}, nil); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := f.run(t, "stop", d.RunID, "--format", "jsonl")
	if code != 6 {
		t.Fatalf("failed scan reported stopped: exit %d stdout %s stderr %s", code, out, stderr)
	}
}

func TestRecoverUsesAdmittedChecksAndIdentity(t *testing.T) {
	f := newFixture(t)
	digest := f.config(t, "pass")
	d, _, wait := seedLaunch(t, f, "happy", "sha256:"+digest)
	wait()
	// Neither candidate-edited checks nor mutable source Git identity may
	// replace the decision the operator approved before native execution.
	if err := os.WriteFile(filepath.Join(d.Workdir, "mythhelm.toml"), []byte("invalid candidate config"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.git(t, "config", "user.name", "Changed after admission")
	code, _, stderr := f.run(t, "recover", d.RunID)
	if code != 0 {
		t.Fatalf("admitted checks not used: %d %s", code, stderr)
	}
	j := f.journal(t)
	v, err := j.LatestVerification(t.Context(), d.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if v.ConfigSHA256 != digest || v.Result != "passed" {
		t.Fatalf("verification %v", v)
	}
	c, err := j.Candidate(t.Context(), d.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := workspace.Git(t.Context(), d.Workdir, false, "show", "-s", "--format=%an", c.Commit)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != "Test" {
		t.Fatalf("mutable identity used %q", raw)
	}
}

func TestRecoverRefusesOversizedTask(t *testing.T) {
	f := newFixture(t)
	d, _, wait := seedLaunch(t, f, "happy")
	wait()
	if err := os.WriteFile(filepath.Join(d.RunDir, "task.md"), bytes.Repeat([]byte("x"), admission.MaxTaskBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := f.run(t, "recover", d.RunID); code != 6 {
		t.Fatalf("oversized task %d: %s", code, stderr)
	}
	if _, err := f.journal(t).Candidate(t.Context(), d.AttemptID); !errors.Is(err, journal.ErrNotFound) {
		t.Fatalf("corrupt task yielded candidate: %v", err)
	}
}

func TestRecoverListsOrphansInNativeLaunchWindow(t *testing.T) {
	f := newFixture(t)
	d, _, wait := seedLaunch(t, f, "happy")
	wait()
	truncateBeforeExit(t, f, d)
	dir := workers.AttemptDir(f.state, d.RunID, d.AttemptID)
	id, err := workers.ReadIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	id.NativePID, id.NativePGID = nil, nil
	if id.NativeLaunchIntent == nil {
		t.Fatal("fixture did not record native launch intent")
	}
	raw, err := json.Marshal(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worker.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	child := exec.Command(os.Args[0], fake.AgentCommand, "--scenario", "slow", "--workdir", d.Workdir) //nolint:gosec // G702: test executable and fixed fake argv
	child.Env = append(os.Environ(), workers.AttemptMarker(d.AttemptID))
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	code, _, stderr := f.run(t, "recover", d.RunID, "--format", "jsonl")
	if code != 6 || !strings.Contains(stderr, workers.AttemptMarker(d.AttemptID)) {
		t.Fatalf("native launch uncertainty hidden: %d %s", code, stderr)
	}
	if runtime.GOOS == "linux" && !strings.Contains(stderr, strconv.Itoa(child.Process.Pid)) {
		t.Fatalf("orphan PID missing: %s", stderr)
	}
	if _, err := workers.ProcessStartTime(child.Process.Pid); err != nil {
		t.Fatalf("recovery signalled unknown native: %v", err)
	}
}
