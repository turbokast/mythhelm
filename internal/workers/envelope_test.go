package workers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
)

func TestCheckEnvelope(t *testing.T) {
	t.Parallel()
	env := EpisodeEnvelope{AttemptID: "att_1", LaunchID: "tok_abc", Generation: 3}
	if err := CheckEnvelope(env, "att_1", "tok_abc", 3); err != nil {
		t.Fatalf("matching envelope: %v", err)
	}
	cases := []struct {
		name       string
		env        EpisodeEnvelope
		attemptID  string
		launchID   string
		generation int64
		wantField  string
	}{
		{"wrong attempt", env, "att_2", "tok_abc", 3, "attempt"},
		{"wrong launch", env, "att_1", "tok_other", 3, "launch"},
		{"wrong generation", env, "att_1", "tok_abc", 4, "generation"},
		{"empty launch in envelope", EpisodeEnvelope{AttemptID: "att_1", Generation: 3}, "att_1", "tok_abc", 3, "launch"},
		{"empty attempt presented", env, "", "tok_abc", 3, "attempt"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := CheckEnvelope(c.env, c.attemptID, c.launchID, c.generation)
			if !errors.Is(err, errRevisionConflict) {
				t.Fatalf("err = %v, want revision_conflict", err)
			}
			if got := err.Error(); !strings.Contains(got, c.wantField) {
				t.Fatalf("err %q does not name %q", got, c.wantField)
			}
		})
	}
}

// TestCheckEnvelopeGenerationZeroUnfenced pins the generation-0 envelope
// meaning: an unfenced worker accepts any supervisor generation, since no
// generation was pinned at its admission.
func TestCheckEnvelopeGenerationZeroUnfenced(t *testing.T) {
	t.Parallel()
	env := EpisodeEnvelope{AttemptID: "att_1", LaunchID: "tok_abc"}
	for _, gen := range []int64{0, 1, 7} {
		if err := CheckEnvelope(env, "att_1", "tok_abc", gen); err != nil {
			t.Fatalf("unfenced envelope vs generation %d: %v", gen, err)
		}
	}
	// Unfenced mutes only the generation, never identity.
	if err := CheckEnvelope(env, "att_1", "tok_other", 1); !errors.Is(err, errRevisionConflict) {
		t.Fatalf("unfenced envelope vs wrong launch: %v, want revision_conflict", err)
	}
}

func TestBeatFreshBoundary(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	if !beatFresh(now.Add(-MaxSupervisorBeatAge), now) {
		t.Fatal("a beat exactly MaxSupervisorBeatAge old reads stale, want fresh")
	}
	if beatFresh(now.Add(-MaxSupervisorBeatAge-time.Nanosecond), now) {
		t.Fatal("a beat older than MaxSupervisorBeatAge reads fresh, want stale")
	}
	if !beatFresh(now.Add(time.Minute), now) {
		t.Fatal("a future beat reads stale, want fresh")
	}
}

func TestMaxSupervisorBeatAgeIsThreeTicks(t *testing.T) {
	t.Parallel()
	// OQ-SR3: 3x the 100ms supervisor-beat interval. control mirrors this
	// value as control.MaxSupervisorBeatAge (workers must not import
	// control); both pin 300ms.
	if MaxSupervisorBeatAge != 300*time.Millisecond {
		t.Fatalf("MaxSupervisorBeatAge = %s, want 300ms", MaxSupervisorBeatAge)
	}
}

// writeBeatFile writes a supervisor.beat fixture with the control writer's
// exact keys (control.WriteSupervisorBeat pins the same JSON from its
// side); drift between the two breaks the beat tests here.
func writeBeatFile(t *testing.T, dir string, beat uint64, at time.Time, generation int64) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"beat": beat, "at": at.UTC().Format(time.RFC3339Nano), "generation": generation})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, supervisorBeatFile), append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadSupervisorBeat(t *testing.T) {
	t.Parallel()
	t.Run("fresh beat reads", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		at := time.Now().UTC().Truncate(time.Microsecond)
		writeBeatFile(t, dir, 41, at, 3)
		gotAt, gen, ok := ReadSupervisorBeat(dir)
		if !ok {
			t.Fatal("fresh beat reads not-ok")
		}
		if gen != 3 {
			t.Fatalf("generation = %d, want 3", gen)
		}
		if !gotAt.Equal(at) {
			t.Fatalf("at = %s, want %s", gotAt, at)
		}
	})
	t.Run("missing corrupt and torn beats read stale", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if _, _, ok := ReadSupervisorBeat(dir); ok {
			t.Fatal("missing beat reads ok, want stale")
		}
		path := filepath.Join(dir, supervisorBeatFile)
		for name, body := range map[string]string{
			"corrupt":   "{not json",
			"empty":     "",
			"bad clock": "{\"beat\":1,\"at\":\"not-a-time\",\"generation\":1}\n",
			"no clock":  "{\"beat\":1,\"generation\":1}\n",
		} {
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, ok := ReadSupervisorBeat(dir); ok {
				t.Errorf("%s beat reads ok, want stale", name)
			}
		}
	})
	t.Run("stale beat still parses", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeBeatFile(t, dir, 7, time.Now().Add(-time.Hour).UTC(), 2)
		at, gen, ok := ReadSupervisorBeat(dir)
		if !ok {
			t.Fatal("stale beat reads not-ok: Read reports the beat, staleness is the caller's")
		}
		if gen != 2 || time.Since(at) < time.Minute {
			t.Fatalf("beat = (%s, %d), want the hour-old generation-2 beat", at, gen)
		}
	})
}

func TestSupervisorBeatStale(t *testing.T) {
	t.Parallel()
	fresh := t.TempDir()
	writeBeatFile(t, fresh, 1, time.Now().UTC(), 1)
	if SupervisorBeatStale(fresh, time.Now()) {
		t.Fatal("fresh beat reads stale")
	}
	stale := t.TempDir()
	writeBeatFile(t, stale, 1, time.Now().Add(-time.Hour).UTC(), 1)
	if !SupervisorBeatStale(stale, time.Now()) {
		t.Fatal("hour-old beat reads fresh")
	}
	missing := t.TempDir()
	if !SupervisorBeatStale(missing, time.Now()) {
		t.Fatal("missing beat reads fresh")
	}
}

func TestAwaitReconnectReturnsOnFreshBeat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	env := EpisodeEnvelope{AttemptID: "att_1", LaunchID: "tok_abc", Generation: 1}
	done := make(chan error, 1)
	go func() { done <- AwaitReconnect(context.Background(), dir, env) }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		select {
		case err := <-done:
			t.Fatalf("AwaitReconnect returned %v with no beat written", err)
		case <-time.After(awaitPollInterval * 3):
			writeBeatFile(t, dir, 9, time.Now().UTC(), 1)
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("AwaitReconnect = %v, want nil on a fresh beat", err)
				}
				return
			case <-time.After(10 * time.Second):
				t.Fatal("AwaitReconnect did not return after a fresh beat")
			}
		case <-time.After(time.Until(deadline)):
			t.Fatal("timed out driving AwaitReconnect")
		}
	}
}

// TestAwaitReconnectAcceptsAnyGeneration pins the return rule: a fresh beat
// of any generation ends the wait. A changed generation means the
// supervisor restarted; the handshake re-homes the worker, which never
// treats the mismatch as permission to improvise.
func TestAwaitReconnectAcceptsAnyGeneration(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	env := EpisodeEnvelope{AttemptID: "att_1", LaunchID: "tok_abc", Generation: 1}
	done := make(chan error, 1)
	go func() { done <- AwaitReconnect(context.Background(), dir, env) }()
	time.Sleep(awaitPollInterval * 3)
	writeBeatFile(t, dir, 3, time.Now().UTC(), 2)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("AwaitReconnect = %v, want nil on a fresh new-generation beat", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("AwaitReconnect did not return on a fresh new-generation beat")
	}
}

func TestAwaitReconnectRefusesMalformedEnvelope(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	err := AwaitReconnect(context.Background(), dir, EpisodeEnvelope{AttemptID: "att_1"})
	if !errors.Is(err, errRevisionConflict) {
		t.Fatalf("AwaitReconnect with an empty launch id = %v, want revision_conflict without waiting", err)
	}
}

func TestAwaitReconnectCancelLeavesSpool(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	spoolPath := filepath.Join(dir, "spool.jsonl")
	before := "{\"stuck\":\"in the spool\"}\n[not even json\n"
	if err := os.WriteFile(spoolPath, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(spoolPath)
	if err != nil {
		t.Fatal(err)
	}
	env := EpisodeEnvelope{AttemptID: "att_1", LaunchID: "tok_abc", Generation: 1}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- AwaitReconnect(ctx, dir, env) }()
	time.Sleep(awaitPollInterval * 3)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("AwaitReconnect = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cancelling ctx did not end AwaitReconnect")
	}
	after, err := os.ReadFile(spoolPath) //nolint:gosec // G304: a test temp path
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != before {
		t.Fatalf("spool changed by a cancelled wait: %q vs %q", after, before)
	}
	if st2, err := os.Stat(spoolPath); err != nil || st2.Size() != st.Size() {
		t.Fatalf("spool size changed by a cancelled wait")
	}
}

// TestSpoolFailureStopsBlindWorker pins the unsupervised spool failure:
// with no supervisor beat, a worker that cannot spool concludes the
// attempt as interrupted with the failure recorded, and exits instead of
// entering the reconnect wait (which would hang: no beat ever comes).
func TestSpoolFailureStopsBlindWorker(t *testing.T) {
	t.Parallel()
	dir := t.TempDir() // no supervisor.beat: the supervisor is lost
	sp, err := createSpool(filepath.Join(dir, "spool.jsonl"), "run_blind", "task_1", "att_blind")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sp.close() })
	// The first sync fails (the supervise-phase write), then the spool
	// heals so the abort records the conclusion.
	syncs := 0
	sp.sync = func(*os.File) error {
		syncs++
		if syncs == 1 {
			return errors.New("disk hiccup")
		}
		return nil
	}
	w := &worker{dir: dir, runID: "run_blind", attemptID: "att_blind",
		launch: Launch{LaunchToken: "tok_blind", Generation: 1},
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)), stderr: &ring{max: stderrRingBytes}}
	sess := &scriptedSession{obs: []adapter.Observation{adapter.SessionStarted{SessionID: "s", APIKeySource: "none"}}, exit: adapter.NativeExit{Code: 0}}
	out, err := w.supervise(context.Background(), sp, sess)
	if err == nil {
		t.Fatal("supervise with a failing spool returned nil")
	}
	// The abort path stops the native and concludes the attempt as far as
	// the healed spool accepts: interrupted with the failure recorded.
	if err := w.abort(context.Background(), sp, sess, out, err); err == nil {
		t.Fatal("abort with a failing spool returned nil")
	}
	if sess.interrupts != 1 {
		t.Fatalf("abort ran %d stop ladders, want 1: the blind native must be stopped", sess.interrupts)
	}
	evs := attempt{dir: dir}.events(t)
	state, reason := final(t, evs)
	if state != "interrupted" || reason != "worker_persistence_failed" {
		t.Fatalf("concluded state = %s/%s, want interrupted/worker_persistence_failed", state, reason)
	}
	// The blind worker exits instead of entering the reconnect wait: with
	// no beat ever written, AwaitReconnect would hang, so returning here
	// proves the failure path never waits.
	if _, _, ok := ReadSupervisorBeat(dir); ok {
		t.Fatal("fixture beat exists, want the supervisor lost throughout")
	}
}

// scriptedSession plays canned observations, then the exit.
type scriptedSession struct {
	obs        []adapter.Observation
	exit       adapter.NativeExit
	interrupts int
}

func (s *scriptedSession) Observations() <-chan adapter.Observation {
	ch := make(chan adapter.Observation, len(s.obs))
	for _, ob := range s.obs {
		ch <- ob
	}
	close(ch)
	return ch
}

func (s *scriptedSession) Done() <-chan adapter.NativeExit {
	ch := make(chan adapter.NativeExit, 1)
	ch <- s.exit
	return ch
}

func (s *scriptedSession) Interrupt(context.Context) adapter.InterruptReport {
	s.interrupts++
	return adapter.InterruptReport{Confirmed: true}
}

func TestWorkerEnvelopeBuildsFromHeldValues(t *testing.T) {
	t.Parallel()
	w := &worker{attemptID: "att_9", launch: Launch{LaunchToken: "tok_9", Generation: 5}}
	env := w.envelope()
	if env != (EpisodeEnvelope{AttemptID: "att_9", LaunchID: "tok_9", Generation: 5}) {
		t.Fatalf("envelope = %+v, want the argv attempt, launch token and pinned generation", env)
	}
}

// waitExit waits for proc to exit, reporting whether it did before the timeout.
func waitExit(proc *os.Process, timeout time.Duration) (exited bool) {
	done := make(chan error, 1)
	go func() { _, err := proc.Wait(); done <- err }()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func TestSupervisorBeatLossAndReturn(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	l := a.launch(t, "slow")
	l.Generation = 1
	writeBeatFile(t, a.dir, 1, time.Now().UTC(), 1) // fresh: the episode runs supervised
	proc := a.spawn(t, l)
	a.waitSessionStarted(t) // mid-episode, supervised
	if err := os.Remove(filepath.Join(a.dir, supervisorBeatFile)); err != nil {
		t.Fatal(err)
	}
	// The loss does not abort the episode: the stop still climbs and the
	// terminal state spools.
	if err := RequestStop(a.dir, "test-loss-stop"); err != nil {
		t.Fatal(err)
	}
	evs := a.waitDone(t)
	if st, _ := final(t, evs); st != "stopped" {
		t.Fatalf("final state = %s, want stopped", st)
	}
	// ... but the worker does not exit: with the beat stale it entered
	// the reconnect wait after finishing and spooling.
	if waitExit(proc, time.Second) {
		t.Fatal("the worker exited under a stale beat, want it awaiting reconnection")
	}
	// The supervisor returns: a fresh beat ends the wait.
	writeBeatFile(t, a.dir, 2, time.Now().UTC(), 1)
	if !waitExit(proc, 30*time.Second) {
		t.Fatal("the worker did not exit after a fresh beat")
	}
}

func TestUnsupervisedWorkerStartsNothing(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	l := a.launch(t, "happy")
	l.Generation = 1
	// No beat file at all: the supervisor is lost for the whole episode.
	proc := a.spawn(t, l)
	a.waitDone(t) // the admitted episode finishes and spools
	// Probe 1 (Unix: the descendant scan exists on Linux and macOS only):
	// no new task started — nothing alive carries the attempt marker.
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		marked, err := markedPIDs(attemptMarker(a.attemptID))
		if err != nil {
			t.Fatal(err)
		}
		if len(marked) != 0 {
			t.Fatalf("pids %v still carry the attempt marker, want nothing started after the episode", marked)
		}
		// The probe is load-bearing: a misbehaving worker that starts a
		// new task trips it.
		sleeper := exec.Command("sleep", "60")
		sleeper.Env = append(os.Environ(), attemptMarker(a.attemptID))
		sleeper.Dir = t.TempDir()
		if err := sleeper.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = sleeper.Process.Kill() }()
		marked, err = markedPIDs(attemptMarker(a.attemptID))
		if err != nil {
			t.Fatal(err)
		}
		if len(marked) == 0 {
			t.Fatal("a marker-carrying sleeper was not found: the probe is blind")
		}
		if err := sleeper.Process.Kill(); err != nil {
			t.Fatal(err)
		}
	}
	// Probe 2: no new effect approved — the spool is frozen past the
	// terminal state.
	spoolPath := filepath.Join(a.dir, "spool.jsonl")
	size := func() int64 {
		st, err := os.Stat(spoolPath)
		if err != nil {
			t.Fatal(err)
		}
		return st.Size()
	}
	frozen := size()
	time.Sleep(time.Second)
	if got := size(); got != frozen {
		t.Fatalf("spool grew from %d to %d bytes while unsupervised, want it frozen", frozen, got)
	}
	// The probe is load-bearing: a new spooled effect trips it.
	f, err := os.OpenFile(spoolPath, os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // G304: a test temp path
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{}\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if got := size(); got == frozen {
		t.Fatal("an appended spool line left the size unchanged: the probe is blind")
	}
	// An envelope check against the wrong launch fails closed, and the
	// worker keeps waiting.
	env := EpisodeEnvelope{AttemptID: a.attemptID, LaunchID: l.LaunchToken, Generation: 1}
	if err := CheckEnvelope(env, a.attemptID, "tok_wrong", 1); !errors.Is(err, errRevisionConflict) {
		t.Fatalf("CheckEnvelope with a wrong launch_id = %v, want revision_conflict", err)
	}
	if waitExit(proc, time.Second) {
		t.Fatal("the worker exited with no beat written, want it still awaiting reconnection")
	}
	// Release it: a fresh beat ends the wait.
	writeBeatFile(t, a.dir, 1, time.Now().UTC(), 1)
	if !waitExit(proc, 30*time.Second) {
		t.Fatal("the worker did not exit after a fresh beat")
	}
}

func TestUILossHasNoExecutionEffect(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	l := a.launch(t, "happy")
	l.Generation = 1
	// The beat stays fresh (future-dated, so no writer is needed); no UI
	// consumer reads anything mid-run: no spool poll, no heartbeat or log
	// tail. Cutting every UI-facing read leaves execution unaffected.
	writeBeatFile(t, a.dir, 1, time.Now().Add(5*time.Minute).UTC(), 1)
	proc := a.spawn(t, l)
	if !waitExit(proc, 30*time.Second) {
		t.Fatal("the episode did not complete with the UI channel cut")
	}
	// Only now, after the exit, is the spool read: the episode ran to its
	// terminal state with no reconnect wait.
	evs := a.events(t)
	if st, _ := final(t, evs); st != "succeeded_native" {
		t.Fatalf("final state = %s, want succeeded_native", st)
	}
	// Contrast: the same worker with a stale beat does wait.
	b := newAttempt(t)
	lb := b.launch(t, "happy")
	lb.Generation = 1
	bproc := b.spawn(t, lb)
	bevs := b.waitDone(t)
	if st, _ := final(t, bevs); st != "succeeded_native" {
		t.Fatalf("final state = %s, want succeeded_native", st)
	}
	if waitExit(bproc, time.Second) {
		t.Fatal("the worker exited under a stale beat: only a stale beat enters the wait, and this one should have")
	}
	writeBeatFile(t, b.dir, 1, time.Now().UTC(), 1)
	if !waitExit(bproc, 30*time.Second) {
		t.Fatal("the worker did not exit after a fresh beat")
	}
}

// TestEnvelopeGenerationZeroUnfenced pins the generation-0 envelope
// meaning: with no supervisor generation pinned, the worker never waits,
// even with no beat file at all — exactly the pre-envelope behaviour.
func TestEnvelopeGenerationZeroUnfenced(t *testing.T) {
	t.Parallel()
	a := newAttempt(t)
	l := a.launch(t, "happy")
	if l.Generation != 0 {
		t.Fatalf("fixture launch generation = %d, want the unfenced 0", l.Generation)
	}
	proc := a.spawn(t, l)
	if !waitExit(proc, 30*time.Second) {
		t.Fatal("the unfenced worker did not exit with no beat file")
	}
	if st, _ := final(t, a.events(t)); st != "succeeded_native" {
		t.Fatalf("final state = %s, want succeeded_native", st)
	}
}
