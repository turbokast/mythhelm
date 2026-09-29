package workers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/adapters/fake"
	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
)

// spawnHelperEnv switches the test binary into the helper that spawns a
// worker and then waits to be killed, standing in for the `mythhelm run` CLI.
const spawnHelperEnv = "MYTHHELM_TEST_SPAWN_HELPER"

// TestMain lets the test binary play every process of an attempt: the
// worker (__worker), the fake agent (__fake-agent) and the spawning CLI.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case Command:
			os.Exit(Main(os.Args[2:]))
		case fake.AgentCommand:
			os.Exit(fake.AgentMain(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
		}
	}
	if args := os.Getenv(spawnHelperEnv); args != "" {
		os.Exit(spawnHelper(strings.Split(args, "\n")))
	}
	// Every re-executed -race child otherwise sleeps a second before exiting.
	if err := os.Setenv("GORACE", "atexit_sleep_ms=0"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// spawnHelper spawns a worker from the launch on stdin, prints its PID and
// then sleeps until it is killed.
func spawnHelper(args []string) int {
	var l Launch
	if err := json.NewDecoder(os.Stdin).Decode(&l); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	proc, err := Spawn(os.Args[0], args[0], args[1], args[2], l)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(proc.Pid)
	time.Sleep(time.Hour)
	return 0
}

// attempt is one worker run in a temporary state directory.
type attempt struct {
	state, runID, attemptID, dir, workdir string
}

func newAttempt(t *testing.T) attempt {
	t.Helper()
	a := attempt{state: t.TempDir(), runID: ids.New("run"), attemptID: ids.New("att"), workdir: t.TempDir()}
	a.dir = AttemptDir(a.state, a.runID, a.attemptID)
	return a
}

// launch builds the admitted launch of a fake scenario, as the supervisor
// would from the adapter's proposal.
func (a attempt) launch(t *testing.T, scenario string) Launch {
	t.Helper()
	lp, err := fake.New().Prepare(context.Background(), adapter.PrepareInput{
		Workdir:   a.workdir,
		AttemptID: a.attemptID,
		Env:       os.Environ(),
		Scenario:  scenario,
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(prompt, []byte("# Task\nedit a file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return Launch{
		LaunchToken: "tok_" + a.attemptID,
		TaskID:      "task_1",
		AdapterID:   fake.New().Descriptor().ID,
		Path:        lp.Spec.Path,
		Args:        lp.Spec.Args,
		Dir:         lp.Spec.Dir,
		Env:         lp.Spec.Env,
		PromptPath:  prompt,
		StopLadder:  lp.StopLadder,
	}
}

// spawn starts the worker as its own detached process and reaps it when the
// test ends.
func (a attempt) spawn(t *testing.T, l Launch) *os.Process {
	t.Helper()
	proc, err := Spawn(os.Args[0], a.state, a.runID, a.attemptID, l)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	t.Cleanup(func() {
		_ = proc.Kill()
		_, _ = proc.Wait()
	})
	return proc
}

// events reads the spool; a torn last line is left for the next read.
func (a attempt) events(t *testing.T) []journal.Event {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(a.dir, "spool.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []journal.Event
	for line := range strings.Lines(string(b)) {
		if !strings.HasSuffix(line, "\n") {
			break
		}
		var ev journal.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("spool line %q: %v", line, err)
		}
		out = append(out, ev)
	}
	return out
}

// waitFor polls the spool until an event satisfies match.
func (a attempt) waitFor(t *testing.T, what string, match func(journal.Event) bool) []journal.Event {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		evs := a.events(t)
		if slices.ContainsFunc(evs, match) {
			return evs
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s; spool has %s (worker log %q)", what, types(evs), a.workerLog())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitDone waits for the attempt's terminal state.
func (a attempt) waitDone(t *testing.T) []journal.Event {
	t.Helper()
	return a.waitFor(t, "a terminal attempt state", func(ev journal.Event) bool {
		if ev.Type != "attempt.state_changed" {
			return false
		}
		s := stateOf(t, ev)
		return !slices.Contains([]string{"launching", "running", "stop_requested"}, s.State)
	})
}

func (a attempt) waitSessionStarted(t *testing.T) {
	t.Helper()
	a.waitFor(t, "attempt.native_session", func(ev journal.Event) bool { return ev.Type == "attempt.native_session" })
}

func (a attempt) workerLog() string {
	b, _ := os.ReadFile(filepath.Join(a.dir, "worker.log"))
	return string(b)
}

func types(evs []journal.Event) []string {
	out := make([]string, len(evs))
	for i, ev := range evs {
		out[i] = ev.Type
	}
	return out
}

func find(t *testing.T, evs []journal.Event, typ string) journal.Event {
	t.Helper()
	for _, ev := range evs {
		if ev.Type == typ {
			return ev
		}
	}
	t.Fatalf("no %s event in %s", typ, types(evs))
	return journal.Event{}
}

type statePayload struct {
	State  string  `json:"state"`
	Reason *string `json:"reason"`
}

func stateOf(t *testing.T, ev journal.Event) statePayload {
	t.Helper()
	var s statePayload
	if err := json.Unmarshal(ev.Payload, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// final returns the attempt's terminal state and reason ("" for null).
func final(t *testing.T, evs []journal.Event) (state, reason string) {
	t.Helper()
	last := evs[len(evs)-1]
	if last.Type != "attempt.state_changed" {
		t.Fatalf("last event = %s, want attempt.state_changed (events %s)", last.Type, types(evs))
	}
	s := stateOf(t, last)
	if s.Reason != nil {
		reason = *s.Reason
	}
	return s.State, reason
}

type stoppedPayload struct {
	Confirmed      bool                 `json:"confirmed"`
	UnresolvedPIDs []int                `json:"unresolved_pids"`
	SignalsSent    []adapter.StopSignal `json:"signals_sent"`
	DescendantScan string               `json:"descendant_scan"`
}

func stoppedOf(t *testing.T, evs []journal.Event) stoppedPayload {
	t.Helper()
	var p stoppedPayload
	raw := find(t, evs, "attempt.stopped").Payload
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if p.UnresolvedPIDs == nil {
		t.Errorf("attempt.stopped %s: unresolved_pids must be an explicit list", raw)
	}
	return p
}

func nativeResultOf(t *testing.T, evs []journal.Event) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(find(t, evs, "attempt.native_result").Payload, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// requestStopOnce asks the worker to stop once its native session is up.
func (a attempt) requestStopOnce(t *testing.T, requestID string) {
	t.Helper()
	a.waitSessionStarted(t)
	if err := RequestStop(a.dir, requestID); err != nil {
		t.Fatal(err)
	}
}

func TestSpoolSequencesContiguous(t *testing.T) {
	a := newAttempt(t)
	proc := a.spawn(t, a.launch(t, "happy"))
	evs := a.waitDone(t)
	if st, err := proc.Wait(); err != nil || !st.Success() {
		t.Fatalf("worker exit = %v, %v (log %q)", st, err, a.workerLog())
	}

	want := []string{
		"attempt.state_changed", "attempt.launched", "attempt.state_changed", "attempt.native_session",
		"attempt.progress", "attempt.protocol_counters", "attempt.native_result", "attempt.stopped",
		"attempt.state_changed",
	}
	if got := types(evs); !slices.Equal(got, want) {
		t.Fatalf("event types = %q, want %q", got, want)
	}
	seen := map[string]bool{}
	for i, ev := range evs {
		if ev.ProducerSequence != int64(i+1) {
			t.Errorf("event %d (%s) producer_sequence = %d, want %d", i, ev.Type, ev.ProducerSequence, i+1)
		}
		if ev.ProducerID != "wrk_"+a.attemptID || ev.RunID != a.runID || ev.AttemptID != a.attemptID || ev.TaskID != "task_1" {
			t.Errorf("event %d envelope = %+v, want producer wrk_%s of run %s, attempt %s, task task_1", i, ev, a.attemptID, a.runID, a.attemptID)
		}
		if seen[ev.EventID] || !strings.HasPrefix(ev.EventID, "evt_") {
			t.Errorf("event %d id %q is duplicated or not an evt_ id", i, ev.EventID)
		}
		seen[ev.EventID] = true
	}
	if st, reason := final(t, evs); st != "succeeded_native" || reason != "" {
		t.Errorf("final state = %s (%s), want succeeded_native", st, reason)
	}

	// The journal, which rejects gaps and regressions, ingests the spool as is.
	j, err := journal.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	for _, ev := range evs {
		if err := j.Append(context.Background(), ev, nil); err != nil {
			t.Fatalf("Append(%s #%d): %v", ev.Type, ev.ProducerSequence, err)
		}
	}
}

func TestCriticalEventsFsynced(t *testing.T) {
	dir := t.TempDir()
	sp, err := createSpool(filepath.Join(dir, "spool.jsonl"), "run_1", "task_1", "att_1")
	if err != nil {
		t.Fatal(err)
	}
	var syncedAt []int64 // spool size at each sync
	sp.sync = func(f *os.File) error {
		fi, err := f.Stat()
		if err != nil {
			return err
		}
		syncedAt = append(syncedAt, fi.Size())
		return nil
	}
	emitted := []string{
		"attempt.state_changed", "attempt.launched", "attempt.native_session", "attempt.progress",
		"attempt.progress", "attempt.permission_denied", "attempt.protocol_counters", "attempt.native_result",
		"attempt.stop_requested", "attempt.stopped", "attempt.state_changed",
	}
	for _, typ := range emitted {
		if err := sp.emit(typ, map[string]any{}); err != nil {
			t.Fatal(err)
		}
	}
	if err := sp.close(); err != nil {
		t.Fatal(err)
	}

	b, err := fs.ReadFile(os.DirFS(dir), "spool.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var end int64
	var wantSynced []int64
	for line := range strings.Lines(string(b)) {
		end += int64(len(line))
		var ev journal.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatal(err)
		}
		if ev.Type != "attempt.progress" && ev.Type != "attempt.protocol_counters" {
			wantSynced = append(wantSynced, end)
		}
	}
	// Each critical event is synced right after it is written, and nothing
	// else is.
	if !slices.Equal(syncedAt, wantSynced) {
		t.Errorf("synced at offsets %v, want the end of each critical event %v", syncedAt, wantSynced)
	}
}

func TestProcessStartTimeStable(t *testing.T) {
	self, err := ProcessStartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	again, err := ProcessStartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if !self.Equal(again) {
		t.Errorf("two reads of our start time differ: %v, %v", self, again)
	}
	if self.After(time.Now()) || time.Since(self) > 24*time.Hour {
		t.Errorf("start time %v is not a plausible recent past", self)
	}
	// The parent (go test, or a shell) started before this binary was built.
	parent, err := ProcessStartTime(os.Getppid())
	if err != nil {
		t.Fatal(err)
	}
	if parent.Equal(self) {
		t.Errorf("parent %d and self %d report the same start time %v", os.Getppid(), os.Getpid(), self)
	}
	if _, err := ProcessStartTime(1 << 30); !errors.Is(err, ErrNoProcess) {
		t.Errorf("ProcessStartTime(nonexistent) error = %v, want ErrNoProcess", err)
	}
}

func TestFakeStopExit130ClassifiedStopped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no SIGINT for the fake; its ladder is Process.Kill (TestWindowsFakeStopConfirmed)")
	}
	a := newAttempt(t)
	a.spawn(t, a.launch(t, "slow"))
	a.requestStopOnce(t, "test-stop")
	evs := a.waitDone(t)

	nr := nativeResultOf(t, evs)
	if string(nr["exit_code"]) != "130" || string(nr["signal"]) != "null" || string(nr["stop_requested"]) != "true" {
		t.Errorf("native_result = exit %s signal %s stop_requested %s, want exit 130 on the first SIGINT", nr["exit_code"], nr["signal"], nr["stop_requested"])
	}
	if p := stoppedOf(t, evs); !p.Confirmed || !slices.Equal(p.SignalsSent, []adapter.StopSignal{adapter.StopInterrupt}) {
		t.Errorf("attempt.stopped = %+v, want confirmed after SIGINT alone", p)
	}
	if st, reason := final(t, evs); st != "stopped" {
		t.Errorf("final state = %s (%s), want stopped, never failed_native", st, reason)
	}
}

func TestStoppedExitEmitsNativeResultWithNulls(t *testing.T) {
	a := newAttempt(t)
	a.spawn(t, a.launch(t, "slow"))
	a.requestStopOnce(t, "test-stop")
	evs := a.waitDone(t)

	nr := nativeResultOf(t, evs)
	if string(nr["result_observed"]) != "false" {
		t.Errorf("result_observed = %s, want false", nr["result_observed"])
	}
	for _, field := range []string{"subtype", "is_error", "num_turns", "duration_ms", "stop_reason", "usage_native_reported", "retail_equivalent_estimate_usd", "denials"} {
		if v, ok := nr[field]; !ok || string(v) != "null" {
			t.Errorf("native_result.%s = %s (present %v), want an explicit null", field, v, ok)
		}
	}
	if st, _ := final(t, evs); st != "stopped" {
		t.Errorf("final state = %s, want stopped", st)
	}
	var req struct {
		RequestedBy string `json:"requested_by"`
	}
	if err := json.Unmarshal(find(t, evs, "attempt.stop_requested").Payload, &req); err != nil || req.RequestedBy != "test-stop" {
		t.Errorf("stop_requested.requested_by = %q, %v; want test-stop", req.RequestedBy, err)
	}
}

func TestStopLadderEscalatesToKill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Windows ladder is Process.Kill alone (TestWindowsFakeStopConfirmed)")
	}
	a := newAttempt(t)
	l := a.launch(t, "ignore-term")
	// The fake ignores SIGINT and SIGTERM, so every rung is reached whatever
	// the grace; short graces keep the test fast.
	l.StopLadder = []adapter.StopStep{
		{Signal: adapter.StopInterrupt, Grace: 200 * time.Millisecond},
		{Signal: adapter.StopTerminate, Grace: 200 * time.Millisecond},
		{Signal: adapter.StopKill, Grace: 10 * time.Second},
	}
	a.spawn(t, l)
	a.requestStopOnce(t, "test-stop")
	// A second request neither replaces the first nor fails.
	if err := RequestStop(a.dir, "second"); err != nil {
		t.Fatal(err)
	}
	evs := a.waitDone(t)

	want := []adapter.StopSignal{adapter.StopInterrupt, adapter.StopTerminate, adapter.StopKill}
	if p := stoppedOf(t, evs); !p.Confirmed || !slices.Equal(p.SignalsSent, want) || len(p.UnresolvedPIDs) != 0 {
		t.Errorf("attempt.stopped = %+v, want confirmed after %v with nothing unresolved", p, want)
	}
	nr := nativeResultOf(t, evs)
	if string(nr["signal"]) != `"killed"` || string(nr["exit_code"]) != "null" {
		t.Errorf("native_result = signal %s exit %s, want killed with a null exit code", nr["signal"], nr["exit_code"])
	}
	if st, _ := final(t, evs); st != "stopped" {
		t.Errorf("final state = %s, want stopped", st)
	}
	if n := strings.Count(string(find(t, evs, "attempt.stop_requested").Payload), "test-stop"); n != 1 {
		t.Errorf("stop_requested payload %s does not name the first request", find(t, evs, "attempt.stop_requested").Payload)
	}
}

func TestWindowsFakeStopConfirmed(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows only: the Unix ladder is covered by TestStopLadderEscalatesToKill")
	}
	a := newAttempt(t)
	proc := a.spawn(t, a.launch(t, "slow"))
	a.requestStopOnce(t, "test-stop")
	evs := a.waitDone(t)
	if _, err := proc.Wait(); err != nil {
		t.Fatal(err)
	}
	if p := stoppedOf(t, evs); !p.Confirmed || !slices.Equal(p.SignalsSent, []adapter.StopSignal{adapter.StopKill}) || p.DescendantScan != "unavailable" {
		t.Errorf("attempt.stopped = %+v, want confirmed by Kill then Wait, and the descendant scan reported unavailable", p)
	}
	if st, _ := final(t, evs); st != "stopped" {
		t.Errorf("final state = %s, want stopped", st)
	}
}

func TestUnresolvedDescendantReported(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("escapees need Unix sessions, and the descendant scan exists on Linux and macOS only")
	}
	a := newAttempt(t)
	a.spawn(t, a.launch(t, "escapee"))
	evs := a.waitDone(t)
	marked, err := markedPIDs(attemptMarker(a.attemptID))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, pid := range marked {
			if p, err := os.FindProcess(pid); err == nil {
				_ = p.Kill()
			}
		}
	})

	p := stoppedOf(t, evs)
	if !p.Confirmed || len(p.UnresolvedPIDs) != 1 || !slices.Equal(p.UnresolvedPIDs, marked) {
		t.Errorf("attempt.stopped = %+v, want the process group confirmed and the escapee %v unresolved", p, marked)
	}
	if want := map[string]string{"linux": "proc-environ", "darwin": "sysctl-procargs2"}[runtime.GOOS]; p.DescendantScan != want {
		t.Errorf("descendant_scan = %q, want %q", p.DescendantScan, want)
	}
	if st, reason := final(t, evs); st != "interrupted" || reason != "unresolved_descendants" {
		t.Errorf("final state = %s (%s), want interrupted (unresolved_descendants) despite the native success", st, reason)
	}
	if nr := nativeResultOf(t, evs); string(nr["subtype"]) != `"success"` || string(nr["result_observed"]) != "true" {
		t.Errorf("native_result = %v, want the observed success", nr)
	}
}

func TestWorkerRefusesSecondLaunch(t *testing.T) {
	a := newAttempt(t)
	if err := os.MkdirAll(a.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.dir, "spool.jsonl"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	proc := a.spawn(t, a.launch(t, "happy"))
	st, err := proc.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if st.Success() {
		t.Errorf("worker exited 0 for an attempt that already has a spool")
	}
	if b, _ := os.ReadFile(filepath.Join(a.dir, "spool.jsonl")); len(b) != 0 {
		t.Errorf("spool was written: %q", b)
	}
	if _, err := os.Stat(filepath.Join(a.workdir, "demo.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the native ran (demo.txt: %v); a worker launches at most once per attempt", err)
	}
}

func TestWorkerRejectsInvalidLaunch(t *testing.T) {
	a := newAttempt(t)
	good := a.launch(t, "happy")
	for name, mutate := range map[string]func(*Launch){
		"no token":        func(l *Launch) { l.LaunchToken = "" },
		"unknown adapter": func(l *Launch) { l.AdapterID = "builtin/nope" },
		"relative path":   func(l *Launch) { l.Path = "mythhelm" },
		"no ladder":       func(l *Launch) { l.StopLadder = nil },
	} {
		t.Run(name, func(t *testing.T) {
			l := good
			mutate(&l)
			body, err := json.Marshal(l)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := readLaunch(strings.NewReader(string(body))); err == nil {
				t.Errorf("readLaunch accepted a launch with %s", name)
			}
		})
	}
	if _, err := readLaunch(io.LimitReader(neverEnding{}, maxLaunchBytes+10)); err == nil {
		t.Error("readLaunch accepted an oversized launch")
	}
}

type neverEnding struct{}

func (neverEnding) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	return len(p), nil
}
