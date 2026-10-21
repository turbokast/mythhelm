//go:build unix

package e2e

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/workers"
)

// This file proves the live TUI lifecycle through the packaged binary on a
// real pseudo-terminal (design §8; AC-6.2, AC-6.5, AC-6.7, AC-6.13,
// AC-6.14). It is Unix only: the driver uses POSIX pseudo-terminals and the
// tests deliver SIGINT, SIGHUP and SIGTERM. Windows runs the portable tests
// and claims no qualified detach (I14 (v2 §2)).
//
// Every synchronisation is a named durable barrier: a journal or spool row,
// a gate helper's arrival file, a process identity, or text the view draws
// in answer to a key. No step waits on elapsed time alone.

// lifecycleFixture is one isolated packaged run: canonical home and state, a
// private runtime directory (no host supervisor is consulted or disturbed),
// a committed repository and the gate helpers that hold a phase on demand.
type lifecycleFixture struct {
	e      env
	repo   string
	digest string
	gate   string // barrier directory shared by the gate helpers
	bin    string // holds the gate helpers, installed as git and the check
}

// gatedCheck "" runs an ungated passing check; "pass" or "fail" holds the
// check at the gate and then ends it with that outcome.
func newLifecycleFixture(t *testing.T, gatedCheck string) *lifecycleFixture {
	t.Helper()
	e := canonicalEnv(t)
	e.vars = append(e.vars, "XDG_RUNTIME_DIR="+canonicalDir(t, t.TempDir()), "TERM=xterm-256color")
	f := &lifecycleFixture{e: e, gate: canonicalDir(t, t.TempDir()), bin: canonicalDir(t, t.TempDir())}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"git", checkGateName} {
		if err := os.Symlink(self, filepath.Join(f.bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	check := []string{mythhelmBin, "__demo-check", "pass"}
	if gatedCheck != "" {
		check = []string{filepath.Join(f.bin, checkGateName), f.gate, gatedCheck}
	}
	f.repo, f.digest = seedCheckRepo(t, e, canonicalDir(t, t.TempDir()), check)
	return f
}

// seedCheckRepo is mkRepo with an explicit check argv.
func seedCheckRepo(t *testing.T, e env, root string, check []string) (repo, digest string) {
	t.Helper()
	repo = filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o750); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = e.withParent(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	git("init", "--quiet", "--initial-branch=main")
	git("config", "user.name", "E2E")
	git("config", "user.email", "e2e@example.com")
	quoted := make([]string, len(check))
	for i, arg := range check {
		quoted[i] = strconv.Quote(arg)
	}
	config := "schema_version = 1\n[[checks]]\nname = \"e2e\"\nargv = [" + strings.Join(quoted, ", ") + "]\ntimeout = \"30s\"\n"
	for name, body := range map[string]string{"README.md": "e2e\n", "task.md": "# E2E task\n\nWrite demo.txt.\n", "mythhelm.toml": config} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git("add", "README.md", "task.md", "mythhelm.toml")
	git("commit", "--quiet", "-m", "seed")
	_, digest, err := admission.ParseProjectConfig([]byte(config))
	if err != nil {
		t.Fatal(err)
	}
	return repo, digest
}

// environ is the packaged process's environment. A non-empty gitGate puts
// the git gate first on PATH, armed to hold that Git subcommand.
func (f *lifecycleFixture) environ(t *testing.T, gitGate string) []string {
	t.Helper()
	vars := f.e.withParent(t)
	if gitGate == "" {
		return vars
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	return append(vars, "PATH="+f.bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"E2E_REAL_GIT="+realGit, "E2E_GATE_GIT="+gitGate, "E2E_GATE_DIR="+f.gate)
}

// start runs `mythhelm run` with the fake scenario on the terminal.
func (f *lifecycleFixture) start(t *testing.T, gitGate, scenario string) *ptyClient {
	t.Helper()
	return startPTY(t, f.environ(t, gitGate), f.repo, append(baseRunArgs(f.digest), "--scenario", scenario)...)
}

// waitGate waits for the gate helper's arrival file.
func (f *lifecycleFixture) waitGate(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		if _, err := os.Stat(filepath.Join(f.gate, gateEntered)); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the gated phase was never entered")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (f *lifecycleFixture) release(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.gate, gateRelease), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

// detachPaths are the two confirmed ways a person detaches a running view.
// Each printable key waits for the view's own redraw before the next one.
var detachPaths = []struct {
	name   string
	detach func(c *ptyClient)
}{
	{"d confirmation", func(c *ptyClient) {
		openDetach(c)
		c.send(confirmKeys)
	}},
	{"q detach row", func(c *ptyClient) {
		// The d dialog proves the gate is open; Enter on its initial
		// Cancel closes it. q opens on back; two rows up is detach.
		openDetach(c)
		at := c.mark()
		c.send("\rq")
		at = c.waitText("Exit options", at, "")
		at = c.waitText("▶ back", at, "")
		c.send("k")
		at = c.waitText("▶ request stop", at, "")
		c.send("k")
		c.waitText("▶ detach (leave running)", at, "")
		c.send("\r")
	}},
}

// TestTUIDetachRecoverBinary detaches a running live TUI by its q row and by
// d with its confirmation, then executes the exact printed recovery command
// (AC-6.2, AC-6.7). The client exits 6 without a stop: the journal and the
// worker's spool hold no stop request and the recorded worker and native
// keep running. The printed command, run from a caller whose MYTHHELM_HOME
// is unrelated, takes over the same attempt and finishes it with no second
// native launch. I06/I12/I18/I21 (v2 §2).
func TestTUIDetachRecoverBinary(t *testing.T) {
	t.Parallel()
	for _, tc := range detachPaths {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newLifecycleFixture(t, "")
			c := f.start(t, "", "paced")
			state := f.e.state
			runID := onlyRun(t, state)
			waitAttemptState(t, state, runID, "running")
			dir, worker := liveAttempt(t, state, runID)

			tc.detach(c)
			res := c.exit(2 * time.Minute)
			if res.Code != 6 {
				t.Fatalf("client exit %d, want 6:\n%s", res.Code, res.Output)
			}
			want := fmt.Sprintf("detached from run %s; run is not complete; worker keeps running; %s", runID, wantRecover(state, runID))
			if !hasExactLine(res.Post, want) {
				t.Fatalf("output after the view lacks the exact detach line %q:\n%s", want, res.Post)
			}
			if strings.Contains(res.Post, "mythhelm review") {
				t.Fatalf("detach output offers review guidance:\n%s", res.Post)
			}
			command := printedCommand(t, res.Post)
			run, a, counts := runSnapshot(t, state, runID)
			if run.State != "executing" || a.State != "running" || counts["attempt.stop_requested"] != 0 || counts["attempt.launched"] != 1 {
				t.Fatalf("after detach run %s, attempt %s, events %v; want executing/running, no stop, one launch", run.State, a.State, counts)
			}
			if spoolHasStop(spoolEvents(t, dir)) {
				t.Fatal("the worker spooled a stop request after a stop-less detach")
			}
			if !workerAlive(worker) || !nativeAlive(worker) {
				t.Fatalf("recorded worker %d / native %d did not survive the detach", worker.PID, *worker.NativePID)
			}

			home := unrelatedHome(t)
			code, out := runPrinted(t, f.e.withParent(t), home, command)
			if code != 0 {
				t.Fatalf("printed recovery exit %d, want 0:\n%s", code, out)
			}
			if !strings.Contains(out, fmt.Sprintf("reattached to worker pid %d", worker.PID)) {
				t.Fatalf("recovery did not reattach to the recorded worker %d:\n%s", worker.PID, out)
			}
			run, after, counts := runSnapshot(t, state, runID)
			if run.State != "ready_for_review" || after.AttemptID != a.AttemptID || counts["attempt.launched"] != 1 ||
				counts["attempt.launch_intent_recorded"] > 1 || counts["attempt.stop_requested"] != 0 {
				t.Fatalf("after recovery run %s, attempt %s (was %s), events %v; want ready_for_review on the same single launch",
					run.State, after.AttemptID, a.AttemptID, counts)
			}
			waitProcessesGone(t, worker)
			assertUntouched(t, home)
		})
	}
}

// TestTUIStopDetachRecoverBinary confirms a stop, detaches while the worker
// has not confirmed it, and recovers the stopping run with the printed
// command (AC-6.5, AC-6.7). The client says the stop is not yet confirmed;
// recovery ends cancelled with a confirmed attempt.stopped and the single
// original stop request. I06 (v2 §2): stop requested is never reported as
// stopped before the worker confirms it.
func TestTUIStopDetachRecoverBinary(t *testing.T) {
	t.Parallel()
	f := newLifecycleFixture(t, "")
	c := f.start(t, "", "ignore-term")
	state := f.e.state
	runID := onlyRun(t, state)
	waitAttemptState(t, state, runID, "running")
	dir, worker := liveAttempt(t, state, runID)

	openStop(c)
	c.send(confirmKeys)
	waitRunState(t, state, runID, "stopping")
	waitSpool(t, dir, "the worker's stop request", spoolHasStop)
	openDetach(c)
	c.send(confirmKeys)
	res := c.exit(2 * time.Minute)
	if res.Code != 6 {
		t.Fatalf("client exit %d, want 6:\n%s", res.Code, res.Output)
	}
	if want := "stop not yet confirmed; " + wantRecover(state, runID); !hasExactLine(res.Post, want) {
		t.Fatalf("output after the view lacks the exact pending-stop line %q:\n%s", want, res.Post)
	}
	command := printedCommand(t, res.Post)
	run, _, counts := runSnapshot(t, state, runID)
	if run.State != "stopping" || counts["attempt.stopped"] != 0 {
		t.Fatalf("after detach run %s, events %v; want stopping with no stopped event", run.State, counts)
	}
	if !workerAlive(worker) {
		t.Fatal("the worker exited before its stop ladder finished")
	}

	home := unrelatedHome(t)
	code, out := runPrinted(t, f.e.withParent(t), home, command)
	if code != 130 {
		t.Fatalf("printed recovery exit %d, want 130:\n%s", code, out)
	}
	run, a, counts := runSnapshot(t, state, runID)
	if run.State != "cancelled" || a.State != "stopped" || counts["attempt.stopped"] != 1 ||
		counts["attempt.stop_requested"] != 1 || counts["attempt.launched"] != 1 {
		t.Fatalf("after recovery run %s, attempt %s, events %v; want cancelled/stopped with one stop and one launch", run.State, a.State, counts)
	}
	if got := stopRequesters(t, spoolEvents(t, dir)); !slices.Equal(got, []string{"user-interrupt"}) {
		t.Fatalf("spooled stop requesters %v, want the single user interrupt", got)
	}
	assertConfirmedStop(t, state, runID)
	waitProcessesGone(t, worker)
	assertUntouched(t, home)
}

// assertConfirmedStop requires the journaled attempt.stopped to be the
// worker's confirmation, not a request.
func assertConfirmedStop(t *testing.T, state, runID string) {
	t.Helper()
	j := ledgerOpenState(t, env{state: state})
	events, err := j.Events(t.Context(), runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		if ev.Type != "attempt.stopped" {
			continue
		}
		var p struct {
			Confirmed bool `json:"confirmed"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err != nil || !p.Confirmed {
			t.Fatalf("attempt.stopped %s is not a confirmed stop (%v)", ev.Payload, err)
		}
		return
	}
	t.Fatal("no attempt.stopped journaled")
}

// TestTUISIGINTOnceBinary delivers one OS SIGINT to the live client
// (AC-6.5, AC-6.14). The CLI is the only signal owner, so exactly one stop
// request reaches the worker, the view stays open while the run is
// cancelled, and quitting afterwards exits 130. A second owner would turn
// the same signal into a second interrupt, which detaches and exits 6.
func TestTUISIGINTOnceBinary(t *testing.T) {
	t.Parallel()
	f := newLifecycleFixture(t, "")
	c := f.start(t, "", "slow")
	state := f.e.state
	runID := onlyRun(t, state)
	waitAttemptState(t, state, runID, "running")
	dir, worker := liveAttempt(t, state, runID)
	// The open detach gate proves the view holds the running snapshot, so
	// the interrupt meets the executing row; Enter on Cancel closes it.
	openDetach(c)
	c.send("\r")

	c.signal(syscall.SIGINT)
	waitRunState(t, state, runID, "cancelled")
	if !c.alive() {
		t.Fatal("one SIGINT ended the client; the view must stay open after a stop request")
	}
	at := c.mark()
	c.send("q")
	c.waitText("Exit options", at, "")
	c.waitText("quit", at, "")
	c.send("k\r")
	res := c.exit(2 * time.Minute)
	if res.Code != 130 {
		t.Fatalf("client exit %d, want 130 after a confirmed stop:\n%s", res.Code, res.Output)
	}
	run, a, counts := runSnapshot(t, state, runID)
	if run.State != "cancelled" || a.State != "stopped" || counts["attempt.stop_requested"] != 1 {
		t.Fatalf("run %s, attempt %s, events %v; want cancelled/stopped with exactly one stop request", run.State, a.State, counts)
	}
	if got := stopRequesters(t, spoolEvents(t, dir)); !slices.Equal(got, []string{"user-interrupt"}) {
		t.Fatalf("spooled stop requesters %v, want exactly one user interrupt", got)
	}
	waitProcessesGone(t, worker)
}

// TestTUITerminalLossBinary loses the terminal (SIGHUP, and SIGTERM, which
// the CLI owns in place of Bubble Tea) while each phase is held at a durable
// barrier (AC-6.13). Early and running clients detach without a stop once a
// worker is accepted; stopping detaches with its single stop pending;
// verifying and applying finish headless and keep the canonical outcome.
// Every client exits 6, no run is left in created or admission, and no
// extra stop is requested. I06/I12/I18/I21 (v2 §2).
func TestTUITerminalLossBinary(t *testing.T) {
	t.Parallel()
	phases := []struct {
		name string
		run  func(t *testing.T, sig syscall.Signal)
	}{
		{"early", lossEarly},
		{"running", lossRunning},
		{"stopping", lossStopping},
		{"verifying", lossVerifying},
		{"verifying failed", lossVerifyingFailed},
		{"applying", lossApplying},
	}
	for _, sig := range []struct {
		name string
		sig  syscall.Signal
	}{{"SIGHUP", syscall.SIGHUP}, {"SIGTERM", syscall.SIGTERM}} {
		for _, phase := range phases {
			t.Run(phase.name+"/"+sig.name, func(t *testing.T) {
				t.Parallel()
				phase.run(t, sig.sig)
			})
		}
	}
}

// lostClient waits for the client's exit after terminal loss and checks the
// approved client code and the run's canonical state. The journal may lag
// the worker's spool by one ingest when the client detaches, so at most
// maxStops stop requests may be journaled; callers check the spool exactly.
func lostClient(t *testing.T, c *ptyClient, state, runID, wantState string, maxStops int) {
	t.Helper()
	lostClientCode(t, c, state, runID, wantState, maxStops, 6)
}

func lostClientCode(t *testing.T, c *ptyClient, state, runID, wantState string, maxStops, wantCode int) {
	t.Helper()
	res := c.exit(2 * time.Minute)
	if res.Code != wantCode {
		t.Fatalf("client exit %d after terminal loss, want %d:\n%s", res.Code, wantCode, res.Output)
	}
	run, _, counts := runSnapshot(t, state, runID)
	if run.State != wantState || counts["attempt.stop_requested"] > maxStops {
		t.Fatalf("after terminal loss run %s, events %v; want %s with at most %d stop requests", run.State, counts, wantState, maxStops)
	}
}

// headless requires the view to have closed while the client itself keeps
// running the held phase.
func headless(t *testing.T, c *ptyClient) {
	t.Helper()
	c.waitViewClosed()
	if !c.alive() {
		t.Fatal("the client exited instead of continuing headless")
	}
}

func lossEarly(t *testing.T, sig syscall.Signal) {
	f := newLifecycleFixture(t, "")
	c := f.start(t, "clone", "slow")
	state := f.e.state
	runID := onlyRun(t, state)
	f.waitGate(t)
	waitRunState(t, state, runID, "admission")
	c.signal(sig)
	headless(t, c)
	waitRunState(t, state, runID, "admission")
	f.release(t)
	waitAttemptState(t, state, runID, "running")
	dir, worker := liveAttempt(t, state, runID)
	lostClient(t, c, state, runID, "executing", 0)
	if spoolHasStop(spoolEvents(t, dir)) || !workerAlive(worker) || !nativeAlive(worker) {
		t.Fatal("early terminal loss stopped the accepted worker instead of detaching")
	}
}

func lossRunning(t *testing.T, sig syscall.Signal) {
	f := newLifecycleFixture(t, "")
	c := f.start(t, "", "slow")
	state := f.e.state
	runID := onlyRun(t, state)
	waitAttemptState(t, state, runID, "running")
	dir, worker := liveAttempt(t, state, runID)
	c.signal(sig)
	lostClient(t, c, state, runID, "executing", 0)
	if spoolHasStop(spoolEvents(t, dir)) || !workerAlive(worker) || !nativeAlive(worker) {
		t.Fatal("terminal loss stopped the running worker instead of detaching")
	}
}

func lossStopping(t *testing.T, sig syscall.Signal) {
	f := newLifecycleFixture(t, "")
	c := f.start(t, "", "ignore-term")
	state := f.e.state
	runID := onlyRun(t, state)
	waitAttemptState(t, state, runID, "running")
	dir, worker := liveAttempt(t, state, runID)
	openStop(c)
	c.send(confirmKeys)
	waitRunState(t, state, runID, "stopping")
	waitSpool(t, dir, "the worker's stop request", spoolHasStop)
	c.signal(sig)
	lostClient(t, c, state, runID, "stopping", 1)
	if got := stopRequesters(t, spoolEvents(t, dir)); !slices.Equal(got, []string{"user-interrupt"}) || !workerAlive(worker) {
		t.Fatalf("stopping terminal loss: spooled requesters %v, worker alive %v; want one stop with the ladder still running", got, workerAlive(worker))
	}
}

func lossVerifying(t *testing.T, sig syscall.Signal) {
	f := newLifecycleFixture(t, "pass")
	c := f.start(t, "", "happy")
	state := f.e.state
	runID := onlyRun(t, state)
	f.waitGate(t)
	waitRunState(t, state, runID, "verifying")
	c.signal(sig)
	headless(t, c)
	f.release(t)
	lostClient(t, c, state, runID, "ready_for_review", 0)
	assertHeadlessOutcome(t, state, runID, "passed")
}

// lossVerifyingFailed loses the terminal while a failing check is held: the
// headless verification still fails, the client keeps the run's own nonzero
// failure exit instead of turning it into a detach or success, and the
// canonical failed receipt stands with no stop and no second launch.
func lossVerifyingFailed(t *testing.T, sig syscall.Signal) {
	f := newLifecycleFixture(t, "fail")
	c := f.start(t, "", "happy")
	state := f.e.state
	runID := onlyRun(t, state)
	f.waitGate(t)
	waitRunState(t, state, runID, "verifying")
	c.signal(sig)
	headless(t, c)
	f.release(t)
	lostClientCode(t, c, state, runID, "failed", 0, 5)
	assertHeadlessOutcome(t, state, runID, "failed")
}

// assertHeadlessOutcome requires the canonical outcome of a phase finished
// headless: one launch whose native succeeded and ended once (the native's
// own end, as on every completed run), no stop request, one verification
// with the wanted result and one receipt.
func assertHeadlessOutcome(t *testing.T, state, runID, verification string) {
	t.Helper()
	_, a, counts := runSnapshot(t, state, runID)
	if a.State != "succeeded_native" || counts["attempt.launched"] != 1 || counts["attempt.stop_requested"] != 0 ||
		counts["attempt.stopped"] != 1 || counts["verification.completed"] != 1 || counts["receipt.written"] != 1 {
		t.Fatalf("headless outcome attempt %s, events %v; want one launch succeeded_native ended once, no stop request, one verification and one receipt", a.State, counts)
	}
	j := ledgerOpenState(t, env{state: state})
	events, err := j.Events(t.Context(), runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		if ev.Type != "verification.completed" {
			continue
		}
		var p struct {
			Result string `json:"result"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err != nil || p.Result != verification {
			t.Fatalf("verification.completed %s, want result %q (%v)", ev.Payload, verification, err)
		}
	}
}

func lossApplying(t *testing.T, sig syscall.Signal) {
	f := newLifecycleFixture(t, "")
	c := f.start(t, "fetch", "happy")
	state := f.e.state
	runID := onlyRun(t, state)
	waitRunState(t, state, runID, "ready_for_review")
	// Replace the prefilled source branch, move to the buttons, focus the
	// confirm button and activate it.
	c.waitText("Apply candidate", c.mark(), "a")
	c.send(strings.Repeat("\x7f", 32) + "e2e-applied" + "\t\t\t" + "\x1b[C" + "\r")
	f.waitGate(t)
	waitRunState(t, state, runID, "applying")
	c.signal(sig)
	headless(t, c)
	f.release(t)
	lostClient(t, c, state, runID, "completed", 0)
	_, a, _ := runSnapshot(t, state, runID)
	j := ledgerOpenState(t, env{state: state})
	candidate, err := j.Candidate(t.Context(), a.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "rev-parse", "--verify", "refs/heads/e2e-applied")
	cmd.Dir = f.repo
	cmd.Env = f.e.withParent(t)
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != candidate.Commit {
		t.Fatalf("applied branch is %q (%v), want candidate %s", out, err, candidate.Commit)
	}
}

// TestDemoDetachRecoverBinary detaches the live TUI demo by d and by q
// (AC-6.7): the client exits 6 with both demo directories retained and no
// review screen, the recorded worker and native keep running untouched, and
// the exact printed command, run from a caller with an unrelated
// MYTHHELM_HOME, recovers the same single launch. Only after recovery proves
// a terminal outcome and no live writer does the fixture, as the operator,
// remove exactly the two retained directories. The temporary root is
// canonical before it is exported and spells a space and an apostrophe, so
// the printed paths and command must quote them. I06/I12/I18/I21 (v2 §2).
func TestDemoDetachRecoverBinary(t *testing.T) {
	t.Parallel()
	for _, tc := range detachPaths {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := canonicalEnv(t)
			tmp := filepath.Join(canonicalDir(t, t.TempDir()), "demo tmp's root")
			unrelated := filepath.Join(tmp, "unrelated")
			if err := os.MkdirAll(unrelated, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(unrelated, "keep"), []byte("unrelated\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			environ := append(e.withParent(t), "XDG_RUNTIME_DIR="+canonicalDir(t, t.TempDir()), "TERM=xterm-256color",
				"TMPDIR="+tmp, "TMP="+tmp, "TEMP="+tmp)
			c := startPTY(t, environ, canonicalDir(t, t.TempDir()), "demo")
			state := waitDemoDir(t, tmp, "mythhelm-demo-state-")
			repo := waitDemoDir(t, tmp, "mythhelm-demo-repo-")
			runID := onlyRun(t, state)
			waitAttemptState(t, state, runID, "running")
			dir, worker := liveAttempt(t, state, runID)
			_, a, _ := runSnapshot(t, state, runID)

			tc.detach(c)
			res := c.exit(2 * time.Minute)
			if res.Code != 6 {
				t.Fatalf("demo client exit %d, want 6:\n%s", res.Code, res.Output)
			}
			for _, want := range []string{"demo repository retained: " + repo, "demo state retained: " + state, wantRecover(state, runID)} {
				if !hasExactLine(res.Post, want) {
					t.Fatalf("demo output after the view lacks the exact line %q:\n%s", want, res.Post)
				}
			}
			if strings.Contains(res.Output, "SCRIPTED DEMO: review ===") || strings.Contains(res.Output, "was removed afterwards") {
				t.Fatalf("detached demo reviewed or claimed removal before recovery:\n%s", res.Output)
			}
			command := printedCommand(t, res.Post)
			assertDemoRetained(t, tmp, repo, state, a.WorkspacePath, dir)
			row, live, counts := runSnapshot(t, state, runID)
			if row.State != "executing" || live.State != "running" || live.AttemptID != a.AttemptID ||
				counts["attempt.stop_requested"] != 0 || counts["attempt.launched"] != 1 {
				t.Fatalf("after detach run %s, attempt %s %s, events %v; want executing/running %s, no stop, one launch",
					row.State, live.AttemptID, live.State, counts, a.AttemptID)
			}
			if spoolHasStop(spoolEvents(t, dir)) {
				t.Fatal("the demo worker spooled a stop request after a stop-less detach")
			}
			if !workerAlive(worker) || !nativeAlive(worker) {
				t.Fatalf("recorded demo worker %d / native %d did not survive the detach", worker.PID, *worker.NativePID)
			}

			home := unrelatedHome(t)
			code, out := runPrinted(t, environ, home, command)
			if code != 0 {
				t.Fatalf("printed demo recovery exit %d, want 0:\n%s", code, out)
			}
			if !strings.Contains(out, fmt.Sprintf("reattached to worker pid %d", worker.PID)) {
				t.Fatalf("demo recovery did not reattach to the recorded worker %d:\n%s", worker.PID, out)
			}
			row, after, counts := runSnapshot(t, state, runID)
			if row.State != "ready_for_review" || after.AttemptID != a.AttemptID || counts["attempt.launched"] != 1 ||
				counts["attempt.launch_intent_recorded"] > 1 || counts["attempt.stop_requested"] != 0 {
				t.Fatalf("after demo recovery run %s, attempt %s (was %s), events %v; want ready_for_review on the same single launch",
					row.State, after.AttemptID, a.AttemptID, counts)
			}
			waitProcessesGone(t, worker)
			assertUntouched(t, home)
			assertDemoRetained(t, tmp, repo, state, a.WorkspacePath, dir)

			// The operator's explicit removal, only now and only of the two
			// directories the demo named.
			for _, retained := range []string{repo, state} {
				if err := os.RemoveAll(retained); err != nil {
					t.Fatal(err)
				}
			}
			entries, err := os.ReadDir(tmp)
			if err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(filepath.Join(unrelated, "keep"))
			if len(entries) != 1 || entries[0].Name() != "unrelated" || err != nil || string(body) != "unrelated\n" {
				t.Fatalf("temporary root after removal holds %v, unrelated %q (%v); want only the untouched unrelated directory", entries, body, err)
			}
		})
	}
}

// waitDemoDir waits for the single demo directory with prefix directly under
// tmp and returns it.
func waitDemoDir(t *testing.T, tmp, prefix string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		matches, err := filepath.Glob(filepath.Join(tmp, prefix+"*"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) > 1 {
			t.Fatalf("temporary root holds %d %s directories, want one", len(matches), prefix)
		}
		if len(matches) == 1 {
			return matches[0]
		}
		if time.Now().After(deadline) {
			t.Fatalf("demo never created its %s directory under %s", prefix, tmp)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func assertDemoRetained(t *testing.T, tmp, repo, state, workspace, attemptDir string) {
	t.Helper()
	if err := demoRetained(tmp, repo, state, workspace, attemptDir); err != nil {
		t.Fatal(err)
	}
}

// demoRetained requires both named demo directories to sit directly under
// the temporary root with their demo prefixes, and the managed workspace and
// spool evidence to remain.
func demoRetained(tmp, repo, state, workspace, attemptDir string) error {
	for _, d := range []struct{ dir, prefix string }{{repo, "mythhelm-demo-repo-"}, {state, "mythhelm-demo-state-"}} {
		if filepath.Dir(d.dir) != tmp || !strings.HasPrefix(filepath.Base(d.dir), d.prefix) {
			return fmt.Errorf("retained %s is not a %s directory directly under %s", d.dir, d.prefix, tmp)
		}
	}
	if workspace == "" {
		return errors.New("the attempt records no managed workspace")
	}
	for _, path := range []string{repo, filepath.Join(repo, ".git"), state, workspace, filepath.Join(attemptDir, "spool.jsonl")} {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("retained demo evidence %s: %w", path, err)
		}
	}
	return nil
}

// TestDemoRetainedRejectsLostEvidence keeps the broken inputs the demo
// retention check exists to catch before any recovery runs: a state
// directory without its demo prefix, one outside the temporary root, and
// evidence an unconditional cleanup would have removed.
func TestDemoRetainedRejectsLostEvidence(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "mythhelm-demo-repo-1")
	state := filepath.Join(tmp, "mythhelm-demo-state-1")
	workspace := filepath.Join(state, "workspace")
	attempt := filepath.Join(state, "attempt")
	for _, dir := range []string{filepath.Join(repo, ".git"), workspace, attempt} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(attempt, "spool.jsonl"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := demoRetained(tmp, repo, state, workspace, attempt); err != nil {
		t.Fatalf("intact retention rejected: %v", err)
	}
	unprefixed := filepath.Join(tmp, "state")
	if err := os.Mkdir(unprefixed, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                          string
		repo, state, workspace, spool string
	}{
		{"state without its demo prefix", repo, unprefixed, workspace, attempt},
		{"state outside the temporary root", repo, filepath.Join(state, "mythhelm-demo-state-2"), workspace, attempt},
		{"no recorded workspace", repo, state, "", attempt},
		{"removed workspace", repo, state, filepath.Join(state, "gone"), attempt},
		{"removed spool", repo, state, workspace, filepath.Join(state, "gone")},
		{"removed repository", filepath.Join(tmp, "mythhelm-demo-repo-gone"), state, workspace, attempt},
	} {
		if err := demoRetained(tmp, tc.repo, tc.state, tc.workspace, tc.spool); err == nil {
			t.Errorf("%s: retention accepted", tc.name)
		}
	}
}

// TestKillIdentifiedSparesReusedPID binds a live process by PID and start
// time: a changed start time, as a reused PID would show, must neither count
// as alive nor be killed. The real identity is then killed and observed gone.
func TestKillIdentifiedSparesReusedPID(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("sleep", "600")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-done
	})
	start, err := workers.ProcessStartTime(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	reused := start.Add(-time.Second)
	pid := cmd.Process.Pid
	procs := attemptProcs{Identity: workers.Identity{PID: pid, StartTime: reused, NativePID: &pid}, NativeStart: reused}
	if workerAlive(procs) || nativeAlive(procs) {
		t.Fatal("a changed start time counted as the recorded process")
	}
	killIdentified(pid, reused)
	select {
	case <-done:
		t.Fatal("a process holding the PID under another identity was killed")
	case <-time.After(200 * time.Millisecond):
	}
	if !sameProcess(pid, start) {
		t.Fatal("the real identity does not match its own start time")
	}
	killIdentified(pid, start)
	select {
	case <-done:
	case <-time.After(ownedJoinGrace):
		t.Fatal("the identified process survived its kill")
	}
}

// TestJoinOwnedEscalatesIgnoredStop runs an owned child that ignores SIGTERM:
// the bounded join must escalate to a kill and join the child instead of
// waiting forever, and a child that obeys is joined without the kill.
func TestJoinOwnedEscalatesIgnoredStop(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		script     string
		wantKilled bool
	}{
		{"ignored stop is killed", `trap '' TERM; echo ready; while :; do sleep 1; done`, true},
		{"obeyed stop is joined", `echo ready; exec sleep 600`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command("/bin/sh", "-c", tc.script)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			// The trap is installed once the child prints ready.
			if _, err := bufio.NewReader(stdout).ReadString('\n'); err != nil {
				t.Fatal(err)
			}
			done := make(chan struct{})
			go func() {
				_ = cmd.Wait()
				close(done)
			}()
			t.Cleanup(func() {
				_ = cmd.Process.Kill()
				select {
				case <-done:
				case <-time.After(ownedJoinGrace):
					t.Errorf("child %d survived its kill", cmd.Process.Pid)
				}
			})
			began := time.Now()
			killed, err := joinOwned(done, func() { _ = cmd.Process.Signal(syscall.SIGTERM) }, func() { _ = cmd.Process.Kill() }, time.Second)
			if err != nil || killed != tc.wantKilled {
				t.Fatalf("joinOwned killed %v, err %v; want killed %v and joined", killed, err, tc.wantKilled)
			}
			select {
			case <-done:
			default:
				t.Fatal("joinOwned returned before the child was joined")
			}
			if elapsed := time.Since(began); elapsed > 3*time.Second {
				t.Fatalf("join took %v, beyond its two bounded steps", elapsed)
			}
		})
	}
}
