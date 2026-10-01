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

func recordApplyCompleted(t *testing.T, f fixture, runID, branch, commit string) {
	t.Helper()
	j := f.journal(t)
	payload, err := json.Marshal(map[string]string{"branch": branch, "target_repo": f.onlyRun(t).SourceRepo, "candidate_commit": commit})
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Append(t.Context(), journal.Event{SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"),
		RunID: runID, ProducerID: ids.New("sup"), ProducerSequence: 1, Generation: 1,
		ObservedAt: time.Now().UTC(), Type: "apply.completed", Payload: payload}, nil); err != nil {
		t.Fatal(err)
	}
	p := supervisor.NewProducer(ids.New("sup"), 1)
	if err := supervisor.TransitionRun(t.Context(), j, runID, supervisor.RunCompleted, "", p); err != nil {
		t.Fatal(err)
	}
}

func breakWorkspace(t *testing.T, f fixture, attemptID string) {
	t.Helper()
	if _, err := applyDB(t, f.state).ExecContext(t.Context(), `UPDATE attempts SET workspace_path = ? WHERE attempt_id = ?`,
		filepath.Join(t.TempDir(), "missing"), attemptID); err != nil {
		t.Fatal(err)
	}
}

func eventTypeCounts(t *testing.T, f fixture, runID string) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, ev := range f.events(t, runID) {
		counts[ev.Type]++
	}
	return counts
}

func latestReceiptDigest(t *testing.T, f fixture, runID string) (sha string, schema float64) {
	t.Helper()
	for _, ev := range f.events(t, runID) {
		if ev.Type != "receipt.written" {
			continue
		}
		var m struct {
			SchemaVersion float64 `json:"schema_version"`
			SHA256        string  `json:"sha256"`
		}
		if err := json.Unmarshal(ev.Payload, &m); err != nil {
			t.Fatal(err)
		}
		sha, schema = m.SHA256, m.SchemaVersion
	}
	if sha == "" {
		t.Fatal("no journaled receipt")
	}
	return sha, schema
}

func receiptFileSHA(t *testing.T, path string) (raw []byte, sha string) {
	t.Helper()
	var err error
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:])
}

// dropV2ReceiptJournal simulates a crash between the v2 receipt file write
// and its digest journal append: the file stays, the journal keeps the v1
// digest. It returns the deleted v2 digest. Test-only: production code can
// never delete journal events (journal_no_delete trigger).
func dropV2ReceiptJournal(t *testing.T, f fixture, runID string) string {
	t.Helper()
	db := applyDB(t, f.state)
	if _, err := db.ExecContext(t.Context(), `DROP TRIGGER journal_no_delete`); err != nil {
		t.Fatal(err)
	}
	var payload string
	if err := db.QueryRowContext(t.Context(), `SELECT payload FROM journal WHERE run_id = ? AND type = 'receipt.written' ORDER BY seq DESC LIMIT 1`, runID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var m struct {
		SHA256 string `json:"sha256"`
	}
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `DELETE FROM journal WHERE run_id = ? AND type = 'receipt.written' AND payload = ?`, runID, payload); err != nil {
		t.Fatal(err)
	}
	return m.SHA256
}

func applyCandidate(t *testing.T, f fixture, runID string) (attemptID, commit string) {
	t.Helper()
	j := f.journal(t)
	attempt, err := j.LatestAttempt(t.Context(), runID)
	if err != nil {
		t.Fatal(err)
	}
	c, err := j.Candidate(t.Context(), attempt.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	return attempt.AttemptID, c.Commit
}

func assertNoWorkerActivity(t *testing.T, before, after map[string]int) {
	t.Helper()
	for typ, count := range after {
		if !strings.HasPrefix(typ, "attempt.") {
			continue
		}
		if count != before[typ] {
			t.Fatalf("worker event %s grew %d -> %d during apply recovery", typ, before[typ], count)
		}
	}
}

func TestRecoverApplyCreatesAbsentBranch(t *testing.T) {
	f, runID := readyApplyFixture(t)
	_, commit := applyCandidate(t, f, runID)
	branch := "review/recovered-absent"
	recordApplyIntent(t, f, runID, branch, commit)
	before := eventTypeCounts(t, f, runID)
	if code, _, stderr := f.run(t, "recover", runID); code != 0 {
		t.Fatalf("recover exit %d: %s", code, stderr)
	}
	if run := f.onlyRun(t); run.State != string(supervisor.RunCompleted) {
		t.Fatalf("run projected %s, want completed", run.State)
	}
	got, err := workspace.BranchCommit(t.Context(), f.repo, branch)
	if err != nil || got != commit {
		t.Fatalf("branch %s at %s, want %s", branch, got, commit)
	}
	raw, sha := receiptFileSHA(t, filepath.Join(f.state, "runs", runID, "receipt.json"))
	var r map[string]any
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	if r["schema_version"] != float64(2) || r["state"] != string(supervisor.RunCompleted) {
		t.Fatalf("receipt not v2 completed: %v %v", r["schema_version"], r["state"])
	}
	effects := r["external_effects"].([]any)
	if len(effects) != 1 || effects[0].(map[string]any)["type"] != "branch_created" {
		t.Fatalf("external effects = %v, want one branch_created", r["external_effects"])
	}
	if digest, _ := latestReceiptDigest(t, f, runID); digest != sha {
		t.Fatal("repaired v2 digest not journaled")
	}
	assertNoWorkerActivity(t, before, eventTypeCounts(t, f, runID))
}

func TestRecoverApplyCompletesAfterBranchCreation(t *testing.T) {
	f, runID := readyApplyFixture(t)
	attemptID, commit := applyCandidate(t, f, runID)
	branch := "review/recovered-branch"
	recordApplyIntent(t, f, runID, branch, commit)
	j := f.journal(t)
	attempt, err := j.LatestAttempt(t.Context(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.ApplyBranch(t.Context(), f.repo, attempt.WorkspacePath,
		"refs/mythhelm/candidates/"+attempt.AttemptID, branch, commit); err != nil {
		t.Fatal(err)
	}
	// Reconciliation must complete from the existing branch and journal
	// without issuing another fetch.
	breakWorkspace(t, f, attemptID)
	fingerprint := f.fingerprint(t)
	before := eventTypeCounts(t, f, runID)
	if code, _, stderr := f.run(t, "recover", runID); code != 0 {
		t.Fatalf("recover exit %d: %s", code, stderr)
	}
	if run := f.onlyRun(t); run.State != string(supervisor.RunCompleted) {
		t.Fatalf("run projected %s, want completed", run.State)
	}
	if got, err := workspace.BranchCommit(t.Context(), f.repo, branch); err != nil || got != commit {
		t.Fatalf("branch %s at %s, want %s", branch, got, commit)
	}
	if f.fingerprint(t) != fingerprint {
		t.Fatal("recovery touched the source beyond the recorded branch")
	}
	_, sha := receiptFileSHA(t, filepath.Join(f.state, "runs", runID, "receipt.json"))
	if digest, schema := latestReceiptDigest(t, f, runID); digest != sha || schema != 2 {
		t.Fatal("repaired v2 digest not journaled")
	}
	assertNoWorkerActivity(t, before, eventTypeCounts(t, f, runID))
}

func TestRecoverApplyCompletesAfterCompletedTransition(t *testing.T) {
	f, runID := readyApplyFixture(t)
	attemptID, commit := applyCandidate(t, f, runID)
	branch := "review/recovered-transition"
	recordApplyIntent(t, f, runID, branch, commit)
	j := f.journal(t)
	attempt, err := j.LatestAttempt(t.Context(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.ApplyBranch(t.Context(), f.repo, attempt.WorkspacePath,
		"refs/mythhelm/candidates/"+attempt.AttemptID, branch, commit); err != nil {
		t.Fatal(err)
	}
	recordApplyCompleted(t, f, runID, branch, commit)
	breakWorkspace(t, f, attemptID)
	raw, _ := receiptFileSHA(t, filepath.Join(f.state, "runs", runID, "receipt.json"))
	var before map[string]any
	if err := json.Unmarshal(raw, &before); err != nil {
		t.Fatal(err)
	}
	if before["schema_version"] != float64(1) {
		t.Fatalf("fixture receipt schema = %v, want v1", before["schema_version"])
	}
	fingerprint := f.fingerprint(t)
	counts := eventTypeCounts(t, f, runID)
	if code, _, stderr := f.run(t, "recover", runID); code != 0 {
		t.Fatalf("recover exit %d: %s", code, stderr)
	}
	if run := f.onlyRun(t); run.State != string(supervisor.RunCompleted) {
		t.Fatalf("run projected %s, want completed", run.State)
	}
	if f.fingerprint(t) != fingerprint {
		t.Fatal("recovery touched the source beyond the recorded branch")
	}
	_, sha := receiptFileSHA(t, filepath.Join(f.state, "runs", runID, "receipt.json"))
	if digest, schema := latestReceiptDigest(t, f, runID); digest != sha || schema != 2 {
		t.Fatal("repaired v2 digest not journaled")
	}
	assertNoWorkerActivity(t, counts, eventTypeCounts(t, f, runID))
}

func TestRecoverApplyAdoptsUnjournaledV2(t *testing.T) {
	f, runID := readyApplyFixture(t)
	branch := "review/recovered-v2"
	if code, _, stderr := f.run(t, "apply", runID, "--to-branch", branch, "--format", "jsonl"); code != 0 {
		t.Fatalf("apply exit %d: %s", code, stderr)
	}
	v2Path := filepath.Join(f.state, "runs", runID, "receipt.json")
	v1Path := filepath.Join(f.state, "runs", runID, "receipt.v1.json")
	_, v2sha := receiptFileSHA(t, v2Path)
	_, v1sha := receiptFileSHA(t, v1Path)
	if dropped := dropV2ReceiptJournal(t, f, runID); dropped != v2sha {
		t.Fatalf("dropped digest %s, want the v2 file digest %s", dropped, v2sha)
	}
	if digest, schema := latestReceiptDigest(t, f, runID); digest != v1sha || schema != 1 {
		t.Fatal("journal no longer describes the preserved v1")
	}
	fingerprint := f.fingerprint(t)
	if code, _, stderr := f.run(t, "recover", runID); code != 0 {
		t.Fatalf("recover exit %d: %s", code, stderr)
	}
	if run := f.onlyRun(t); run.State != string(supervisor.RunCompleted) {
		t.Fatalf("run projected %s, want completed", run.State)
	}
	// Adoption journals the digest of the exact bytes; it never rewrites them.
	if _, sha := receiptFileSHA(t, v2Path); sha != v2sha {
		t.Fatal("recovery rewrote the unjournaled v2 file")
	}
	if _, sha := receiptFileSHA(t, v1Path); sha != v1sha {
		t.Fatal("recovery rewrote the preserved v1 file")
	}
	if digest, schema := latestReceiptDigest(t, f, runID); digest != v2sha || schema != 2 {
		t.Fatal("adopted v2 digest not journaled")
	}
	if f.fingerprint(t) != fingerprint {
		t.Fatal("recovery touched the source checkout")
	}
}

func TestRecoverApplyRejectsForgedOrMissingPreservedV1(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		forge      func(t *testing.T, path string)
	}{
		{"forged", "preserved v1 receipt does not match its journaled digest", func(t *testing.T, path string) {
			t.Helper()
			if err := os.WriteFile(path, []byte(`{"forged":true}`), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing", "preserved v1 receipt unavailable", func(t *testing.T, path string) {
			t.Helper()
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, runID := readyApplyFixture(t)
			branch := "review/recovered-" + tc.name
			if code, _, stderr := f.run(t, "apply", runID, "--to-branch", branch, "--format", "jsonl"); code != 0 {
				t.Fatalf("apply exit %d: %s", code, stderr)
			}
			v2Path := filepath.Join(f.state, "runs", runID, "receipt.json")
			_, v2sha := receiptFileSHA(t, v2Path)
			dropV2ReceiptJournal(t, f, runID)
			tc.forge(t, filepath.Join(f.state, "runs", runID, "receipt.v1.json"))
			events := len(f.events(t, runID))
			fingerprint := f.fingerprint(t)
			code, _, stderr := f.run(t, "recover", runID)
			if code != 6 || !strings.Contains(stderr, tc.want) {
				t.Fatalf("recover exit %d: %s, want 6 naming %q", code, stderr, tc.want)
			}
			if run := f.onlyRun(t); run.State != string(supervisor.RunCompleted) {
				t.Fatalf("run projected %s, want completed", run.State)
			}
			if n := len(f.events(t, runID)); n != events {
				t.Fatalf("journal grew %d -> %d on rejected adoption", events, n)
			}
			if _, sha := receiptFileSHA(t, v2Path); sha != v2sha {
				t.Fatal("rejected adoption rewrote the receipt file")
			}
			if f.fingerprint(t) != fingerprint {
				t.Fatal("rejected adoption touched the source checkout")
			}
		})
	}
}

func TestRecoverApplyRejectsMissingIntent(t *testing.T) {
	for _, tc := range []struct {
		name, state string
		force       func(t *testing.T, f fixture, runID string)
	}{
		{"applying", string(supervisor.RunApplying), func(t *testing.T, f fixture, runID string) {
			t.Helper()
			p := supervisor.NewProducer(ids.New("sup"), 1)
			if err := supervisor.TransitionRun(t.Context(), f.journal(t), runID, supervisor.RunApplying, "", p); err != nil {
				t.Fatal(err)
			}
		}},
		{"completed", string(supervisor.RunCompleted), func(t *testing.T, f fixture, runID string) {
			t.Helper()
			if _, err := applyDB(t, f.state).ExecContext(t.Context(), `UPDATE runs SET state = 'completed' WHERE run_id = ?`, runID); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, runID := readyApplyFixture(t)
			tc.force(t, f, runID)
			events := len(f.events(t, runID))
			fingerprint := f.fingerprint(t)
			code, _, stderr := f.run(t, "recover", runID)
			if code != 6 || !strings.Contains(stderr, "without a journaled apply intent") {
				t.Fatalf("recover exit %d: %s, want 6 naming the missing intent", code, stderr)
			}
			if run := f.onlyRun(t); run.State != tc.state {
				t.Fatalf("run projected %s, want %s", run.State, tc.state)
			}
			if n := len(f.events(t, runID)); n != events {
				t.Fatalf("journal grew %d -> %d without an intent", events, n)
			}
			if got, err := workspace.BranchCommit(t.Context(), f.repo, "review/never-created"); err != nil || got != "" {
				t.Fatalf("branch created without intent: %s, %v", got, err)
			}
			if f.fingerprint(t) != fingerprint {
				t.Fatal("recovery without intent touched the source checkout")
			}
		})
	}
}

func TestRecoverApplyRejectsMismatchedIntent(t *testing.T) {
	f, runID := readyApplyFixture(t)
	recordApplyIntent(t, f, runID, "review/recovered-mismatch", strings.Repeat("0", 40))
	events := len(f.events(t, runID))
	fingerprint := f.fingerprint(t)
	code, _, stderr := f.run(t, "recover", runID)
	if code != 6 || !strings.Contains(stderr, "intent no longer matches the run") {
		t.Fatalf("recover exit %d: %s, want 6 naming the mismatched intent", code, stderr)
	}
	if run := f.onlyRun(t); run.State != string(supervisor.RunApplying) {
		t.Fatalf("run projected %s, want applying", run.State)
	}
	if n := len(f.events(t, runID)); n != events {
		t.Fatalf("journal grew %d -> %d on mismatched intent", events, n)
	}
	if f.fingerprint(t) != fingerprint {
		t.Fatal("recovery on mismatched intent touched the source checkout")
	}
}

func TestRecoverApplyBlocksConflictingBranch(t *testing.T) {
	for _, tc := range []struct {
		name  string
		plant func(t *testing.T, f fixture, branch string)
	}{
		{"ordinary", func(t *testing.T, f fixture, branch string) {
			t.Helper()
			f.git(t, "branch", branch, "HEAD~1")
		}},
		{"symbolic", func(t *testing.T, f fixture, branch string) {
			t.Helper()
			f.git(t, "symbolic-ref", "refs/heads/"+branch, "refs/heads/untouched")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, runID := readyApplyFixture(t)
			attemptID, commit := applyCandidate(t, f, runID)
			branch := "review/recovered-conflict"
			recordApplyIntent(t, f, runID, branch, commit)
			tc.plant(t, f, branch)
			breakWorkspace(t, f, attemptID)
			fingerprint := f.fingerprint(t)
			code, _, _ := f.run(t, "recover", runID)
			if code != 3 {
				t.Fatalf("recover exit %d, want 3 for the conflicting destination", code)
			}
			if run := f.onlyRun(t); run.State != "blocked" || run.Reason != "branch_exists" {
				t.Fatalf("run projected %s/%s, want blocked/branch_exists", run.State, run.Reason)
			}
			raw, sha := receiptFileSHA(t, filepath.Join(f.state, "runs", runID, "receipt.json"))
			var r map[string]any
			if err := json.Unmarshal(raw, &r); err != nil {
				t.Fatal(err)
			}
			if r["schema_version"] != float64(1) || r["state"] != "blocked" {
				t.Fatalf("blocked receipt = %v/%v, want v1 blocked", r["schema_version"], r["state"])
			}
			if digest, _ := latestReceiptDigest(t, f, runID); digest != sha {
				t.Fatal("blocked v1 digest not journaled")
			}
			if f.fingerprint(t) != fingerprint {
				t.Fatal("conflicting destination changed during recovery")
			}
		})
	}
}

func TestRecoverApplyIsIdempotent(t *testing.T) {
	f, runID := readyApplyFixture(t)
	branch := "review/recovered-idempotent"
	if code, _, stderr := f.run(t, "apply", runID, "--to-branch", branch, "--format", "jsonl"); code != 0 {
		t.Fatalf("apply exit %d: %s", code, stderr)
	}
	_, sha := receiptFileSHA(t, filepath.Join(f.state, "runs", runID, "receipt.json"))
	events := len(f.events(t, runID))
	for i := 0; i < 2; i++ {
		if code, _, stderr := f.run(t, "recover", runID); code != 0 {
			t.Fatalf("recover %d exit %d: %s", i, code, stderr)
		}
	}
	if n := len(f.events(t, runID)); n != events {
		t.Fatalf("journal grew %d -> %d on reconciled recovery", events, n)
	}
	if _, after := receiptFileSHA(t, filepath.Join(f.state, "runs", runID, "receipt.json")); after != sha {
		t.Fatal("reconciled recovery rewrote the receipt file")
	}
}

func TestRecoverApplyLeavesCompletedBranchAlone(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		disturb    func(t *testing.T, f fixture, branch string)
	}{
		{"absent", "apply branch is absent after completion", func(t *testing.T, f fixture, branch string) {
			t.Helper()
			f.git(t, "branch", "-D", branch)
		}},
		{"moved", "apply branch no longer matches the candidate", func(t *testing.T, f fixture, branch string) {
			t.Helper()
			f.git(t, "update-ref", "refs/heads/"+branch, "HEAD~1")
		}},
		{"symbolic", "apply destination is now a symbolic ref", func(t *testing.T, f fixture, branch string) {
			t.Helper()
			f.git(t, "update-ref", "-d", "refs/heads/"+branch)
			f.git(t, "symbolic-ref", "refs/heads/"+branch, "refs/heads/untouched")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, runID := readyApplyFixture(t)
			branch := "review/recovered-undisturbed"
			if code, _, stderr := f.run(t, "apply", runID, "--to-branch", branch, "--format", "jsonl"); code != 0 {
				t.Fatalf("apply exit %d: %s", code, stderr)
			}
			tc.disturb(t, f, branch)
			events := len(f.events(t, runID))
			fingerprint := f.fingerprint(t)
			code, _, stderr := f.run(t, "recover", runID)
			if code != 6 || !strings.Contains(stderr, tc.want) {
				t.Fatalf("recover exit %d: %s, want 6 naming %q", code, stderr, tc.want)
			}
			if run := f.onlyRun(t); run.State != string(supervisor.RunCompleted) {
				t.Fatalf("run projected %s, want completed", run.State)
			}
			if n := len(f.events(t, runID)); n != events {
				t.Fatalf("journal grew %d -> %d on disturbed branch", events, n)
			}
			if f.fingerprint(t) != fingerprint {
				t.Fatal("recovery touched the disturbed destination")
			}
		})
	}
}

func TestRecoverApplyUnresolvedWhenV1ReceiptLost(t *testing.T) {
	f, runID := readyApplyFixture(t)
	_, commit := applyCandidate(t, f, runID)
	branch := "review/recovered-lost-v1"
	recordApplyIntent(t, f, runID, branch, commit)
	if err := os.Remove(filepath.Join(f.state, "runs", runID, "receipt.json")); err != nil {
		t.Fatal(err)
	}
	events := len(f.events(t, runID))
	fingerprint := f.fingerprint(t)
	code, _, stderr := f.run(t, "recover", runID)
	if code != 6 || !strings.Contains(stderr, "reading receipt") {
		t.Fatalf("recover exit %d: %s, want 6 naming the lost receipt", code, stderr)
	}
	if run := f.onlyRun(t); run.State != string(supervisor.RunApplying) {
		t.Fatalf("run projected %s, want applying", run.State)
	}
	if n := len(f.events(t, runID)); n != events {
		t.Fatalf("journal grew %d -> %d on lost receipt", events, n)
	}
	if got, err := workspace.BranchCommit(t.Context(), f.repo, branch); err != nil || got != "" {
		t.Fatalf("branch created without a verified receipt: %s, %v", got, err)
	}
	if f.fingerprint(t) != fingerprint {
		t.Fatal("recovery without a receipt touched the source checkout")
	}
}
