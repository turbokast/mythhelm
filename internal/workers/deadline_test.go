package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/journal"
)

func TestLaunchDeadlineRequired(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	body, err := json.Marshal(a.launch(t, "happy"))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"missing", "0001-01-01T00:00:00Z", "not-a-date"} {
		t.Run(value, func(t *testing.T) {
			if value == "missing" {
				delete(fields, "execution_deadline")
			} else {
				fields["execution_deadline"] = value
			}
			raw, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := readLaunch(strings.NewReader(string(raw))); err == nil {
				t.Fatal("handoff without a valid finite deadline was accepted")
			}
		})
	}
	fields["execution_deadline"] = time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readLaunch(strings.NewReader(string(raw))); err != nil {
		t.Fatalf("valid finite handoff rejected: %v", err)
	}
}

func TestExpiredDeadlineDoesNotLaunch(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	l := a.launch(t, "happy")
	l.ExecutionDeadline = time.Now().Add(-time.Second)
	proc := a.spawn(t, l)
	if !waitExit(proc, 10*time.Second) {
		t.Fatal("expired worker did not return")
	}
	for _, ev := range a.events(t) {
		if ev.Type == "attempt.launched" || ev.Type == "attempt.native_session" {
			t.Fatalf("expired handoff started native execution: %s", ev.Type)
		}
	}
	id, err := ReadIdentity(a.dir)
	if err == nil && (id.NativePID != nil || id.NativeLaunchIntent != nil) {
		t.Fatalf("expired handoff attempted launch: %+v", id)
	}
	if !strings.Contains(a.workerLog(), "deadline") {
		t.Fatalf("expired handoff lacked diagnostic: %s", a.workerLog())
	}
}

// The legacy decoder fixture freezes the private handoff fields predating
// ExecutionDeadline. Unknown fields are rejected exactly as the old worker did.
type legacyLaunch struct {
	LaunchToken       string            `json:"launch_token"`
	TaskID            string            `json:"task_id"`
	AdapterID         string            `json:"adapter_id"`
	Path              string            `json:"path"`
	NativeSHA256      string            `json:"native_sha256"`
	Args              []string          `json:"args"`
	Dir               string            `json:"dir"`
	Env               []string          `json:"env"`
	PromptPath        string            `json:"prompt_path"`
	StopLadder        json.RawMessage   `json:"stop_ladder"`
	LadderVersion     string            `json:"ladder_version"`
	Nonce             string            `json:"nonce"`
	Generation        int64             `json:"generation"`
	Containment       json.RawMessage   `json:"containment,omitempty"`
	ProxyAllow        []string          `json:"proxy_allow,omitempty"`
	UserConfigPaths   map[string]string `json:"user_config_paths,omitempty"`
	UserConfigDigests map[string]string `json:"user_config_digests,omitempty"`
}

func TestMixedWorkerHandoffVersions(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	current, err := json.Marshal(a.launch(t, "happy"))
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(current))
	dec.DisallowUnknownFields()
	var legacy legacyLaunch
	if err := dec.Decode(&legacy); err == nil || !strings.Contains(err.Error(), "execution_deadline") {
		t.Fatalf("old strict worker accepted current handoff: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(current, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "execution_deadline")
	old, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	dec = json.NewDecoder(bytes.NewReader(old))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&legacy); err != nil {
		t.Fatalf("invalid legacy fixture: %v", err)
	}
	if _, err := readLaunch(bytes.NewReader(old)); err == nil {
		t.Fatal("new worker accepted old handoff")
	}
	cmd := exec.CommandContext(t.Context(), os.Args[0], Command, "--state", a.state, "--run", a.runID, "--attempt", a.attemptID) //nolint:gosec // G702: current test binary, fixed worker flags and test-owned identifiers; no shell.
	cmd.Stdin = bytes.NewReader(old)
	output, err := cmd.CombinedOutput()
	if err == nil || cmd.ProcessState.ExitCode() != 2 {
		t.Fatalf("old handoff exit = %v: %s", err, output)
	}
	if _, err := os.Stat(a.dir); !os.IsNotExist(err) {
		t.Fatalf("rejected handoff created attempt evidence/native: %v", err)
	}
}

func TestWorkerDeadlineWithoutSupervisor(t *testing.T) {
	t.Parallel()
	for _, gen := range []int64{0, 7} {
		t.Run(strconv.FormatInt(gen, 10), func(t *testing.T) {
			t.Parallel()
			a := newAttempt(t)
			l := a.launch(t, "slow")
			l.Generation = gen
			l.ExecutionDeadline = time.Now().Add(2 * time.Second)
			proc := a.spawn(t, l)
			t.Cleanup(func() {
				_ = RequestStop(a.dir, "cleanup")
				writeBeatFile(t, a.dir, 1, time.Now().Add(time.Minute), gen)
				// Cleanup also covers the deliberately disabled-timer counterfactual:
				// let the worker stop/reap its native before spawn's kill fallback.
				a.waitDone(t)
			})
			a.waitSessionStarted(t)
			end := time.Now().Add(10 * time.Second)
			var evs []journal.Event
			for time.Now().Before(end) {
				evs = a.events(t)
				terminal := false
				for _, ev := range evs {
					if ev.Type == "attempt.state_changed" && stateOf(t, ev).State == "stopped" {
						terminal = true
					}
				}
				if terminal {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			st, reason := final(t, evs)
			if st != "stopped" || reason != "envelope_deadline_exceeded" {
				t.Fatalf("no supervisor: final = %s/%s; events=%s", st, reason, types(evs))
			}
			requests := 0
			for _, ev := range evs {
				if ev.Type == "attempt.stop_requested" {
					requests++
					var payload struct {
						By string `json:"requested_by"`
					}
					if err := json.Unmarshal(ev.Payload, &payload); err != nil {
						t.Fatal(err)
					}
					if payload.By != "execution-deadline" {
						t.Fatalf("requester=%q", payload.By)
					}
				}
			}
			if requests != 1 || !stoppedOf(t, evs).Confirmed {
				t.Fatalf("requests=%d, stopped=%+v", requests, stoppedOf(t, evs))
			}
			if _, err := os.Stat(filepath.Join(a.dir, supervisorBeatFile)); !os.IsNotExist(err) {
				t.Fatalf("unexpected supervisor beat: %v", err)
			}
			if !waitExit(proc, 3*time.Second) {
				t.Fatal("deadline terminal worker stayed in reconnect wait")
			}
		})
	}
}

// deadlineSession models an owned native: the ladder releases Done only
// after its stop report is supplied. No external process or timer is mocked.
type deadlineSession struct {
	obs     chan adapter.Observation
	done    chan adapter.NativeExit
	entered chan struct{}
	release chan struct{}
	count   atomic.Int32
	once    sync.Once
	report  adapter.InterruptReport
	noExit  bool
}

func newDeadlineSession() *deadlineSession {
	s := &deadlineSession{obs: make(chan adapter.Observation), done: make(chan adapter.NativeExit, 1), entered: make(chan struct{}), release: make(chan struct{}), report: adapter.InterruptReport{Confirmed: true}}
	close(s.obs)
	return s
}
func (s *deadlineSession) Observations() <-chan adapter.Observation { return s.obs }
func (s *deadlineSession) Done() <-chan adapter.NativeExit          { return s.done }
func (s *deadlineSession) Interrupt(context.Context) adapter.InterruptReport {
	s.count.Add(1)
	s.once.Do(func() { close(s.entered) })
	<-s.release
	if !s.noExit {
		s.done <- adapter.NativeExit{Code: 1}
	}
	return s.report
}
func deadlineFixture(t *testing.T) (*worker, *spool, *deadlineSession) {
	t.Helper()
	dir := t.TempDir()
	sp, err := createSpool(filepath.Join(dir, spoolFile), "run_deadline", "task_deadline", "att_deadline")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sp.close() })
	w := &worker{dir: dir, runID: "run_deadline", attemptID: "att_deadline", launch: Launch{ExecutionDeadline: time.Now().Add(-time.Second)}, log: slog.New(slog.NewTextHandler(io.Discard, nil)), stderr: &ring{max: stderrRingBytes}}
	return w, sp, newDeadlineSession()
}

func TestDeadlineStopFailureStaysUnresolved(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"malformed-request", "unreadable-request", "request-write-failure"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			w, sp, sess := deadlineFixture(t)
			switch mode {
			case "malformed-request":
				if err := os.WriteFile(filepath.Join(w.dir, stopFile), []byte("broken"), 0600); err != nil {
					t.Fatal(err)
				}
			case "unreadable-request":
				if err := os.Mkdir(filepath.Join(w.dir, stopFile), 0700); err != nil {
					t.Fatal(err)
				}
			case "request-write-failure":
				w.dir = filepath.Join(w.dir, "missing")
			}
			close(sess.release)
			out, err := w.supervise(t.Context(), sp, sess)
			if err == nil {
				t.Fatalf("%s was treated as a valid stop: %+v", mode, out)
			}
			if err := w.abort(t.Context(), sp, sess, out, err); err == nil {
				t.Fatal("abort discarded request error")
			}
			if sess.count.Load() != 1 {
				t.Fatalf("ladders=%d, want one bounded cleanup", sess.count.Load())
			}
			st, reason := final(t, attempt{dir: filepath.Dir(sp.f.Name())}.events(t))
			if st != "interrupted" || reason == DeadlineReason {
				t.Fatalf("I/O failure became %s/%s", st, reason)
			}
		})
	}
	for _, mode := range []string{"unconfirmed", "escaped"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			out := outcome{stopBy: DeadlineRequester, stopReason: DeadlineReason, report: adapter.InterruptReport{Confirmed: mode == "escaped"}}
			st, reason := classify(out, mode == "escaped")
			if st != "interrupted" || reason == DeadlineReason {
				t.Fatalf("unresolved ownership became %s/%s", st, reason)
			}
		})
	}
}

func TestDeadlineRequestPrecedence(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"user-file-first", "deadline-file-first", "already-running-user-ladder", "simultaneous-timer-poll"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w, sp, sess := deadlineFixture(t)
			first := "user-first"
			if name == "deadline-file-first" {
				first = DeadlineRequester
			}
			if err := RequestStop(w.dir, first); err != nil {
				t.Fatal(err)
			}
			later := DeadlineRequester
			if first == DeadlineRequester {
				later = "later-user"
			}
			if err := RequestStop(w.dir, later); err != nil {
				t.Fatal(err)
			}
			out := outcome{}
			// Select executes one ready case at a time. Exercise both ready timer/poll
			// orders, and a timer arriving while the accepted user ladder is blocked.
			timerFirst := name != "already-running-user-ladder" && name != "simultaneous-timer-poll"
			if err := w.acceptStop(t.Context(), sp, sess, &out, timerFirst); err != nil {
				t.Fatal(err)
			}
			select {
			case <-sess.entered:
			case <-time.After(time.Second):
				t.Fatal("ladder never entered")
			}
			if err := w.acceptStop(t.Context(), sp, sess, &out, !timerFirst); err != nil {
				t.Fatal(err)
			}
			if err := w.acceptStop(t.Context(), sp, sess, &out, true); err != nil {
				t.Fatal(err)
			}
			close(sess.release)
			out.report = <-out.pending
			out.pending = nil
			out.exit = <-sess.done
			out.exited = true
			if err := w.conclude(t.Context(), sp, sess, out); err != nil {
				t.Fatal(err)
			}
			evs := attempt{dir: w.dir}.events(t)
			st, reason := final(t, evs)
			wantReason := ""
			if first == DeadlineRequester {
				wantReason = DeadlineReason
			}
			if st != "stopped" || reason != wantReason || out.stopBy != first || sess.count.Load() != 1 {
				t.Fatalf("winner %s: state=%s/%s outcome=%+v ladders=%d", first, st, reason, out, sess.count.Load())
			}
			requests := 0
			for _, ev := range evs {
				if ev.Type == evStopRequested {
					requests++
					var got map[string]string
					if err := json.Unmarshal(ev.Payload, &got); err != nil {
						t.Fatal(err)
					}
					if got["requested_by"] != first {
						t.Fatalf("durable requester=%q, want %q", got["requested_by"], first)
					}
				}
			}
			if requests != 1 {
				t.Fatalf("stop requests=%d, want exactly one", requests)
			}
		})
	}
}

func TestDeadlineStopWithoutNativeExitIsBounded(t *testing.T) {
	t.Parallel()
	w, sp, sess := deadlineFixture(t)
	sess.noExit = true
	close(sess.release)
	result := make(chan outcome, 1)
	errs := make(chan error, 1)
	go func() { out, err := w.supervise(t.Context(), sp, sess); result <- out; errs <- err }()
	var out outcome
	select {
	case out = <-result:
	case <-time.After(waitDelay + 2*time.Second):
		// Release the wrong implementation too, so a failing assertion joins
		// its goroutine instead of leaving a fixture behind.
		sess.done <- adapter.NativeExit{Code: 1}
		<-result
		<-errs
		t.Fatal("deadline stop still waited for native exit after its finite cleanup budget")
	}
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if out.exited || out.report.Confirmed || sess.count.Load() != 1 {
		t.Fatalf("missing native exit became confirmed: %+v, ladders=%d", out, sess.count.Load())
	}
	if err := w.conclude(t.Context(), sp, sess, out); err != nil {
		t.Fatal(err)
	}
	st, reason := final(t, attempt{dir: w.dir}.events(t))
	if st != "interrupted" || reason != "stop_unconfirmed" {
		t.Fatalf("bounded unresolved native became %s/%s", st, reason)
	}
}

// Durable launch-intent persistence is deliberately held until the immutable
// pin expires. A check before adapter preparation cannot satisfy this case.
func TestExpiredDeadlineDuringPreparationDoesNotLaunch(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	l := a.launch(t, "happy")
	l.ExecutionDeadline = time.Now().Add(2 * time.Second)
	reached := false
	w := &worker{writeFile: func(dir, name string, body []byte) error {
		var id Identity
		if err := json.Unmarshal(body, &id); err != nil {
			return err
		}
		if id.NativeLaunchIntent != nil && id.NativePID == nil {
			reached = true
			<-time.After(max(time.Until(l.ExecutionDeadline), 0))
		}
		return WriteFileAtomic(dir, name, body)
	}}
	if err := a.runInProcess(t, l, w); err != nil {
		t.Fatal(err)
	}
	if !reached {
		t.Fatal("fixture never reached durable native launch intent")
	}
	if w.id.NativePID != nil || w.proc != nil {
		t.Fatalf("preparation expiry started native execution: %+v", w.id)
	}
	evs := a.events(t)
	for _, ev := range evs {
		if ev.Type == evLaunched || ev.Type == evNativeSession {
			t.Fatalf("expired preparation emitted %s", ev.Type)
		}
	}
	st, reason := final(t, evs)
	if st != "failed_native" || reason != "launch_failed" {
		t.Fatalf("expired preparation = %s/%s", st, reason)
	}
}
