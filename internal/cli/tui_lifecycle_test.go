package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/tui"
	"github.com/turbokast/mythhelm/internal/workspace"
)

func TestTUIStartupFailureJoinsWithoutStop(t *testing.T) {
	t.Parallel()
	stdio, _, stderr := testStdio()
	fault := errors.New("program startup failed")
	finished := make(chan struct{})
	deps := forcedDeps(true, func(_ context.Context, cfg tui.Config) error {
		if cfg.Mode != tui.ModeLive || cfg.RequestStop == nil || cfg.Detach == nil || cfg.OnExit == nil || cfg.OnHeadlessAction == nil || cfg.Signals == nil {
			t.Error("live typed controls are not wired")
		}
		close(finished)
		return fault
	})
	out, err := runLiveTUI(stdio, deps, t.TempDir(), "run_startup", resolveTUIFlags(t, bareGetenv), func(_ context.Context, h supervisor.Hooks) (supervisor.Outcome, error) {
		<-finished
		select {
		case <-h.Interrupt:
			t.Error("startup error requested stop")
		default:
		}
		select {
		case <-h.Detach:
			t.Error("startup error requested detach")
		default:
		}
		return supervisor.Outcome{RunID: "run_startup", State: supervisor.RunCompleted}, nil
	})
	if !errors.Is(err, fault) || out.State != supervisor.RunCompleted {
		t.Fatalf("joined result = %+v, %v", out, err)
	}
	want := "run run_startup continues in this process; inspect with: mythhelm review run_startup\n"
	if stderr.String() != want {
		t.Errorf("notice = %q, want %q", stderr.String(), want)
	}
}

func TestLiveTUIIntentOutcomes(t *testing.T) {
	t.Parallel()
	for _, pending := range []bool{false, true} {
		t.Run(strconv.FormatBool(pending), func(t *testing.T) {
			stdio, stdout, _ := testStdio()
			dir := t.TempDir()
			deps := forcedDeps(true, func(_ context.Context, cfg tui.Config) error {
				cfg.Detach()
				cfg.Detach()
				cfg.OnExit(tui.ExitIntent{Kind: tui.ExitDetach, Phase: "executing"})
				return nil
			})
			out, err := runLiveTUI(stdio, deps, dir, "run_detach", resolveTUIFlags(t, bareGetenv), func(_ context.Context, h supervisor.Hooks) (supervisor.Outcome, error) {
				<-h.Detach
				select {
				case <-h.Interrupt:
					t.Error("detach manufactured stop")
				default:
				}
				select {
				case <-h.Detach:
					t.Error("duplicate detach")
				default:
				}
				return supervisor.Outcome{RunID: "run_detach", State: supervisor.RunExecuting, Detached: pending, DetachedRunning: !pending}, nil
			})
			if exitCode(err) != ExitOwnership || out.Detached != pending {
				t.Fatalf("outcome %+v, %v", out, err)
			}
			text := stdout.String()
			want := "detached from run run_detach; run is not complete; worker keeps running; reattach with: mythhelm recover run_detach"
			if pending {
				want = "stop not yet confirmed; run 'mythhelm recover run_detach'"
			}
			want += "; " + tuiRecoveryHint(dir, "run_detach") + "; not qualified on this platform\n"
			if text != want {
				t.Errorf("notice = %q, want %q", text, want)
			}
			if strings.Count(text, "\n") != 1 || strings.Contains(text, "review") || !strings.Contains(text, "mythhelm recover") || !strings.Contains(text, "not qualified on this platform") {
				t.Fatalf("detach notice %q", text)
			}
			if strings.Contains(text, "stop not yet confirmed") != pending {
				t.Fatalf("wrong stop meaning %q", text)
			}
		})
	}
	t.Run("plain return joins without controls", func(t *testing.T) {
		stdio, stdout, _ := testStdio()
		viewReturned := make(chan struct{})
		deps := forcedDeps(true, func(context.Context, tui.Config) error { close(viewReturned); return nil })
		out, err := runLiveTUI(stdio, deps, t.TempDir(), "run_plain", resolveTUIFlags(t, bareGetenv), func(_ context.Context, h supervisor.Hooks) (supervisor.Outcome, error) {
			<-viewReturned
			select {
			case <-h.Interrupt:
				t.Error("plain return stopped run")
			default:
			}
			select {
			case <-h.Detach:
				t.Error("plain return detached run")
			default:
			}
			return supervisor.Outcome{State: supervisor.RunCompleted}, nil
		})
		if err != nil || out.State != supervisor.RunCompleted || stdout.Len() != 0 {
			t.Fatalf("plain outcome %+v, %v, %q", out, err, stdout.String())
		}
	})
}

// lossDeps injects the same terminal-loss observation as the OS pump without
// sending a process-wide signal to unrelated parallel tests.
func lossDeps(run func(context.Context, tui.Config) error) (tuiDeps, *tuiLifecycle) {
	owner := newTUILifecycle()
	deps := forcedDeps(true, run)
	deps.lifecycle = func() *tuiLifecycle { return owner }
	return deps, owner
}
func loseTerminal(owner *tuiLifecycle) {
	owner.update(func(s *lifecycleState) { s.lost = true })
	owner.signals <- tui.SignalTerminalLost
}

func TestTerminalLossStateTable(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"unknown", "created", "admission", "executing", "stopping", "verifying", "applying", "completed", "blocked", "failed"} {
		t.Run(phase, func(t *testing.T) {
			for _, failure := range []bool{false, true} {
				t.Run(strconv.FormatBool(failure), func(t *testing.T) {
					stdio, stdout, _ := testStdio()
					var owner *tuiLifecycle
					deps, c := lossDeps(func(_ context.Context, cfg tui.Config) error {
						loseTerminal(owner)
						<-cfg.Signals
						kind := tui.ExitFinishHeadless
						switch phase {
						case "unknown", "created", "admission", "executing", "stopping":
							kind = tui.ExitDetach
							cfg.Detach()
						}
						cfg.OnExit(tui.ExitIntent{Kind: kind, Phase: phase})
						return nil
					})
					owner = c
					state := supervisor.RunCompleted
					if failure {
						state = supervisor.RunFailed
					}
					out, err := runLiveTUI(stdio, deps, t.TempDir(), "run_loss", resolveTUIFlags(t, bareGetenv), func(_ context.Context, h supervisor.Hooks) (supervisor.Outcome, error) {
						switch phase {
						case "unknown", "created", "admission", "executing", "stopping":
							<-h.Detach // accepted worker or earlier terminal outcome owns disposition
							select {
							case <-h.Interrupt:
								t.Error("loss requested stop")
							default:
							}
							if !failure {
								return supervisor.Outcome{RunID: "run_loss", State: supervisor.RunExecuting, DetachedRunning: true}, nil
							}
						}
						return supervisor.Outcome{RunID: "run_loss", State: state, Reason: "native_failed"}, nil
					})
					want := ExitOwnership
					if failure {
						want = ExitNative
					}
					if exitCode(err) != want {
						t.Fatalf("%s: %+v, %v code %d want %d", phase, out, err, exitCode(err), want)
					}
					if !out.DetachedRunning && out.State != state {
						t.Fatalf("canonical state rewritten: %+v", out)
					}
					if out.DetachedRunning && !strings.Contains(stdout.String(), "not qualified on this platform") {
						t.Fatalf("missing qualification %q", stdout.String())
					}
				})
			}
		})
	}
}

func TestInspectUnresolvedExit(t *testing.T) {
	t.Parallel()
	// These are distinct completion inputs to the CLI projection seam. Actual
	// action cancellation/join is proven by tui.TestExplicitActionQuitCancelsAndJoins
	// and the real live ApplyRun cancellation test, rather than simulated here.
	for _, tc := range []struct {
		name    string
		action  *tui.ActionCompletion
		intent  tui.ExitKind
		message string
	}{
		{name: "ordinary quit", intent: tui.ExitView},
		{name: "pending stop completion", action: &tui.ActionCompletion{Kind: "stop", StopState: "stop_requested"}, intent: tui.ExitFinishHeadless, message: "stop requested; stop not yet confirmed"},
		{name: "cancelled recover completion", action: &tui.ActionCompletion{Kind: "recover", Err: context.Canceled}, intent: tui.ExitUnresolvedAction, message: "action cancelled"},
		{name: "cancelled apply completion", action: &tui.ActionCompletion{Kind: "apply", Err: context.Canceled}, intent: tui.ExitUnresolvedAction, message: "action cancelled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := forcedDeps(true, func(_ context.Context, cfg tui.Config) error {
				if cfg.Mode != tui.ModeInspect {
					t.Error("inspect mode missing")
				}
				if tc.action != nil {
					cfg.OnHeadlessAction(*tc.action)
				}
				cfg.OnExit(tui.ExitIntent{Kind: tc.intent})
				return nil
			})
			err := runInspectTUI(deps, t.TempDir(), "run_inspect", resolveTUIFlags(t, bareGetenv))
			want := ExitOwnership
			if tc.action == nil {
				want = ExitOK
			}
			if exitCode(err) != want {
				t.Fatalf("exit %d want %d: %v", exitCode(err), want, err)
			}
			if tc.action != nil && (!strings.Contains(err.Error(), "MYTHHELM_HOME=") || !strings.Contains(err.Error(), tc.message)) {
				t.Fatalf("wrong completion guidance: %v", err)
			}
		})
	}
}
func TestInspectStopTerminalLoss(t *testing.T) {
	// Fixtures bind process-wide MYTHHELM_HOME.
	for _, state := range []string{"stopped", "stop_requested", "", "unknown", "error"} {
		t.Run(state, func(t *testing.T) {
			dir, id := executingFixture(t)
			var owner *tuiLifecycle
			deps, c := lossDeps(func(_ context.Context, cfg tui.Config) error {
				loseTerminal(owner)
				<-cfg.Signals
				cfg.OnExit(tui.ExitIntent{Kind: tui.ExitFinishHeadless})
				result := tui.ActionCompletion{Kind: "stop", StopState: state}
				if state == "error" {
					result.Err = errors.New("stop failed")
				}
				cfg.OnHeadlessAction(result)
				return nil
			})
			owner = c
			err := runInspectTUI(deps, dir, id, resolveTUIFlags(t, bareGetenv))
			want := ExitOwnership

			if exitCode(err) != want {
				t.Fatalf("exit %d want %d: %v", exitCode(err), want, err)
			}
			if state == "stopped" && !strings.Contains(err.Error(), "stop confirmed") {
				t.Fatalf("confirmed stop %v", err)
			}
			if state != "stopped" && strings.Contains(err.Error(), "stop confirmed") {
				t.Fatalf("false confirmation %v", err)
			}
			if state == "stop_requested" && (!strings.Contains(err.Error(), "not yet confirmed") || !strings.Contains(err.Error(), "MYTHHELM_HOME=")) {
				t.Fatalf("pending hint %v", err)
			}
			j := openJournal(t, dir)
			defer func() { _ = j.Close() }()
			run, e := j.Run(t.Context(), id)
			if e != nil || run.State != "executing" {
				t.Fatalf("canonical run changed %+v %v", run, e)
			}
		})
	}
}

func TestSignalSingleOwner(t *testing.T) {
	// The real signal row owns process-wide signal delivery. The injected
	// mailbox row runs on every platform without qualifying OS delivery.
	t.Run("portable OS mailbox", func(t *testing.T) { checkSignalSingleOwner(t, false) })
	t.Run("real Unix interrupt", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("os.Process.Signal interrupt is unavailable on Windows; portable mailbox row still runs")
		}
		checkSignalSingleOwner(t, true)
	})
}
func checkSignalSingleOwner(t *testing.T, realSignal bool) {
	t.Helper()
	stdio, _, _ := testStdio()
	sent := make(chan struct{})
	owner := newTUILifecycle()
	deps := forcedDeps(true, func(_ context.Context, cfg tui.Config) error {
		defer close(sent)
		if realSignal {
			process, e := os.FindProcess(os.Getpid())
			if e != nil {
				return e
			}
			if e = process.Signal(os.Interrupt); e != nil {
				return e
			}
		} else {
			owner.osSignals <- os.Interrupt
		}
		select {
		case got := <-cfg.Signals:
			if got != tui.SignalInterrupt {
				t.Errorf("signal %v", got)
			}
		case <-time.After(5 * time.Second):
			return errors.New("signal not delivered")
		}
		cfg.RequestStop()
		cfg.RequestStop()
		return nil
	})
	deps.lifecycle = func() *tuiLifecycle { return owner }
	_, err := runLiveTUI(stdio, deps, t.TempDir(), "run_signal", resolveTUIFlags(t, bareGetenv), func(_ context.Context, h supervisor.Hooks) (supervisor.Outcome, error) {
		<-sent
		select {
		case <-h.Interrupt:
		case <-time.After(5 * time.Second):
			return supervisor.Outcome{}, errors.New("stop not delivered")
		}
		select {
		case <-h.Interrupt:
			t.Error("duplicate stop")
		default:
		}
		select {
		case <-h.Detach:
			t.Error("one signal detached")
		default:
		}
		return supervisor.Outcome{State: supervisor.RunCancelled}, nil
	})
	if exitCode(err) != ExitCancelled {
		t.Fatalf("signal exit %v", err)
	}
	select {
	case <-owner.done:
	default:
		t.Fatal("signal pump was not joined")
	}
	if len(owner.interrupt) != 0 || len(owner.detach) != 0 {
		t.Fatalf("late or duplicate controls after join: stop=%d detach=%d", len(owner.interrupt), len(owner.detach))
	}
}

func TestCoordinatorJoins(t *testing.T) {
	t.Parallel()
	for range 50 {
		owner := newTUILifecycle()
		var calls sync.WaitGroup
		for range 8 {
			calls.Go(func() {
				for range 20 {
					owner.update(func(s *lifecycleState) {
						s.stop = true
						s.detach = true
						s.lost = true
						if !s.hasIntent {
							s.hasIntent = true
							s.intent = tui.ExitIntent{Kind: tui.ExitDetach}
						}
					})
				}
			})
		}
		calls.Wait()
		owner.close()
		if !owner.snapshot().hasIntent || len(owner.interrupt) != 1 || len(owner.detach) != 1 {
			t.Fatalf("coordinator did not finish once: %+v, controls %d/%d", owner.snapshot(), len(owner.interrupt), len(owner.detach))
		}
		select {
		case <-owner.done:
		default:
			t.Fatal("pump not joined")
		}
	}
}

// applyGateGit builds an argv-only Git proxy. It pauses the actual fetch
// inside ApplyRun after durable apply intent, never replacing ApplyRun or its
// journal/ref/receipt effects. Its own deadline bounds failed-test cleanup.
func applyGateGit(t *testing.T) (string, string, string) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	gate := filepath.Join(dir, "gate")
	if err = os.Mkdir(gate, 0700); err != nil {
		t.Fatal(err)
	}
	source := fmt.Sprintf(`package main
import("context";"encoding/json";"io";"os";"os/exec";"path/filepath";"strings";"time")
type boundedStderr struct {value strings.Builder}
func(b *boundedStderr)Write(p []byte)(int,error){n:=len(p);left:=4096-b.value.Len();if left>0 {_,_=b.value.Write(p[:min(left,len(p))])};return n,nil}
func main(){
 begin:=time.Now()
 ctx,cancel:=context.WithTimeout(context.Background(),15*time.Second);defer cancel()
 gate:=%q
 record:=func(stage string,runErr error,stderr string){
  f,e:=os.OpenFile(filepath.Join(gate,"proxy.jsonl"),os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600);if e!=nil{return};defer f.Close()
  errorText,contextText:="",""
  if runErr!=nil {errorText=runErr.Error()};if ctx.Err()!=nil {contextText=ctx.Err().Error()}
  var status any
  if runErr==nil {status=0} else if ee,ok:=runErr.(*exec.ExitError);ok{status=ee.ExitCode()}
  _=json.NewEncoder(f).Encode(map[string]any{"stage":stage,"argv":os.Args[1:],"started":begin.UTC(),"at":time.Now().UTC(),"elapsed":time.Since(begin).String(),"context_error":contextText,"error":errorText,"status":status,"stderr":stderr})
 }
 record("start",nil,"")
 block:=false
 for _,arg:=range os.Args[1:] {if arg=="fetch" {block=true}}
 if block {
  record("barrier_enter",nil,"")
  if err:=os.WriteFile(filepath.Join(gate,"entered"),[]byte("fetch"),0600);err!=nil {record("barrier_error",err,"");os.Exit(89)}
  ticker:=time.NewTicker(time.Millisecond);defer ticker.Stop()
  for {if _,err:=os.Stat(filepath.Join(gate,"release"));err==nil {break};select{case <-ctx.Done():record("barrier_error",ctx.Err(),"");os.Exit(88);case <-ticker.C:}}
  record("barrier_exit",nil,"")
 }
 stderr:=&boundedStderr{}
 cmd:=exec.CommandContext(ctx,%q,os.Args[1:]...);cmd.Stdin=os.Stdin;cmd.Stdout=os.Stdout;cmd.Stderr=io.MultiWriter(os.Stderr,stderr)
 err:=cmd.Run();record("end",err,stderr.value.String())
 if err!=nil {if e,ok:=err.(*exec.ExitError);ok {os.Exit(e.ExitCode())};os.Exit(87)}
}
`, gate, realGit)
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		root, e := os.OpenRoot(gate)
		if e != nil {
			t.Logf("Git proxy diagnostic root unavailable: %v", e)
			return
		}
		defer func() { _ = root.Close() }()
		f, e := root.Open("proxy.jsonl")
		if e != nil {
			t.Logf("Git proxy diagnostics unavailable: %v", e)
			return
		}
		defer func() { _ = f.Close() }()
		data, e := io.ReadAll(io.LimitReader(f, 1<<20))
		t.Logf("Git proxy diagnostics (bounded 1 MiB):\n%s\nread error: %v", data, e)
	})
	src := filepath.Join(dir, "proxy.go")
	if err = os.WriteFile(src, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "git")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", binary, src)
	if output, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("build Git proxy: %v %s", e, output)
	}
	return dir, gate, realGit
}
func awaitLifecycleFile(t *testing.T, path string) {
	t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case <-timer.C:
			t.Fatalf("operation did not reach %s", filepath.Base(path))
		case <-ticker.C:
		}
	}
}
func readyLifecycleRun(t *testing.T) (string, string, string, supervisor.Outcome) {
	t.Helper()
	dir, repo := t.TempDir(), t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	task, digest, err := seedDemoRepo(repo, exe, "pass")
	if err != nil {
		t.Fatal(err)
	}
	d, err := admission.Decide(t.Context(), admission.Request{StateDir: dir, Repo: repo, TaskFile: task, Adapter: "fake", Billing: "local-scripted", ExecutionProfile: "trusted-host", TrustProjectConfig: "sha256:" + digest, Env: os.Environ()})
	if err != nil {
		t.Fatal(err)
	}
	out, err := supervisor.Run(t.Context(), d, supervisor.Hooks{})
	if err != nil || out.State != supervisor.RunReadyForReview {
		t.Fatalf("real fixture: %+v %v", out, err)
	}
	return dir, repo, d.RunID, out
}

func TestTerminalLossApplyOutcome(t *testing.T) {
	// PATH changes select the controlled Git executable; no parallel tests.
	for _, mode := range []tui.ClientMode{tui.ModeLive, tui.ModeInspect} {
		for _, conflict := range []bool{false, true} {
			t.Run(fmt.Sprintf("mode%d/conflict%v", mode, conflict), func(t *testing.T) {
				dir, repo, id, original := readyLifecycleRun(t)
				proxy, gate, realGit := applyGateGit(t)
				t.Setenv("PATH", proxy+string(os.PathListSeparator)+os.Getenv("PATH"))
				release := func() { _ = os.WriteFile(filepath.Join(gate, "release"), []byte("release"), 0600) }
				var once sync.Once
				releaseOnce := func() { once.Do(release) }
				actionJoined := make(chan struct{})
				var owner *tuiLifecycle
				var calls atomic.Int32
				var receipt supervisor.Receipt
				var applyErr error
				deps, c := lossDeps(func(_ context.Context, cfg tui.Config) error {
					if cfg.Mode != mode {
						t.Errorf("mode %d want %d", cfg.Mode, mode)
					}
					actionDone := make(chan struct{})
					actionCtx, cancel := context.WithCancel(context.Background())
					defer cancel()
					go func() {
						defer close(actionDone)
						calls.Add(1)
						receipt, applyErr = cfg.Actions.Apply(actionCtx, id, "review/lifecycle", false, false)
					}()
					defer func() { releaseOnce(); <-actionDone; close(actionJoined) }()
					awaitLifecycleFile(t, filepath.Join(gate, "entered"))
					lockPath := filepath.Join(dir, "runs", id)
					assertHeld := func() {
						unlock, e := supervisor.AcquireOwner(lockPath)
						if e == nil {
							unlock()
							t.Error("apply owner lock released while action is in flight")
						} else if !errors.Is(e, supervisor.ErrOwnerHeld) {
							t.Errorf("owner lock %v", e)
						}
					}
					assertHeld()
					loseTerminal(owner)
					if got := <-cfg.Signals; got != tui.SignalTerminalLost {
						t.Errorf("loss %v", got)
					}
					cfg.OnExit(tui.ExitIntent{Kind: tui.ExitFinishHeadless, Phase: "applying"})
					assertHeld()
					select {
					case <-actionCtx.Done():
						t.Error("terminal loss cancelled same action")
					default:
					}
					select {
					case <-actionDone:
						t.Error("blocked apply returned before release")
					default:
					}
					j := openJournal(t, dir)
					row, e := j.Run(t.Context(), id)
					_ = j.Close()
					if e != nil || row.State != "applying" {
						t.Errorf("ApplyRun has not journaled intent: %+v %v", row, e)
					}
					if conflict {
						// A concurrent external writer wins the destination while apply is
						// paused. The real compare-and-swap must refuse overwriting it.
						cmd := exec.CommandContext(t.Context(), realGit, "-C", repo, "branch", "review/lifecycle", "HEAD")
						if output, e := cmd.CombinedOutput(); e != nil {
							t.Errorf("create conflicting ref %v %s", e, output)
						}
					}
					releaseOnce()
					<-actionDone
					cfg.OnHeadlessAction(tui.ActionCompletion{Kind: "apply", Receipt: receipt, Err: applyErr})
					return nil
				})
				owner = c
				// Cleanup is registered before entering any assertion-bearing action.
				t.Cleanup(releaseOnce)
				stdio, _, _ := testStdio()
				var err error
				var liveResult supervisor.Outcome
				if mode == tui.ModeLive {
					liveResult, err = runLiveTUI(stdio, deps, dir, id, resolveTUIFlags(t, bareGetenv), func(context.Context, supervisor.Hooks) (supervisor.Outcome, error) { return original, nil })
				} else {
					err = runInspectTUI(deps, dir, id, resolveTUIFlags(t, bareGetenv))
				}
				select {
				case <-actionJoined:
				default:
					t.Fatal("CLI returned without joining apply call")
				}
				if calls.Load() != 1 {
					t.Fatalf("apply calls %d", calls.Load())
				}
				unlock, e := supervisor.AcquireOwner(filepath.Join(dir, "runs", id))
				if e != nil {
					t.Fatalf("owner not released after join: %v", e)
				}
				unlock()
				j := openJournal(t, dir)
				defer func() { _ = j.Close() }()
				row, e := j.Run(t.Context(), id)
				if e != nil {
					t.Fatal(e)
				}
				final, _, e := supervisor.ReadReceipt(t.Context(), j, id)
				if e != nil {
					t.Fatal(e)
				}
				events, e := j.Events(t.Context(), id, 0)
				if e != nil {
					t.Fatal(e)
				}
				intents, completed := 0, 0
				for _, event := range events {
					if event.Type == "apply.intent_recorded" {
						intents++
					}
					if event.Type == "apply.completed" {
						completed++
					}
				}
				if intents != 1 {
					t.Fatalf("apply intents %d", intents)
				}
				branch, e := workspace.BranchCommit(t.Context(), repo, "review/lifecycle")
				if e != nil {
					t.Fatal(e)
				}
				if mode == tui.ModeLive && string(liveResult.State) != row.State {
					t.Fatalf("stale returned run outcome %+v, actual %+v", liveResult, row)
				}
				if conflict {
					if exitCode(err) != ExitBlocked || row.State != "blocked" || final["state"] != "blocked" || completed != 0 || !errors.Is(applyErr, workspace.ErrBranchExists) {
						t.Fatalf("failed apply was hidden: code %d run %+v receipt %v completed %d err %v", exitCode(err), row, final, completed, err)
					}
					base, e := workspace.Git(t.Context(), repo, true, "rev-parse", "HEAD")
					if e != nil || branch != strings.TrimSpace(string(base)) {
						t.Fatalf("conflicting ref overwritten %s %s %v", branch, base, e)
					}
				} else {
					candidate, e := j.LatestAttempt(t.Context(), id)
					if e != nil {
						t.Fatal(e)
					}
					frozen, e := j.Candidate(t.Context(), candidate.AttemptID)
					if e != nil {
						t.Fatal(e)
					}
					if exitCode(err) != ExitOwnership || row.State != "completed" || final["state"] != "completed" || final["schema_version"] != float64(2) || branch != frozen.Commit || completed != 1 || applyErr != nil {
						t.Fatalf("successful apply: code %d row %+v receipt %v branch %s completed %d error %v", exitCode(err), row, final, branch, completed, err)
					}
					if mode == tui.ModeInspect && (!strings.Contains(err.Error(), "apply completed") || strings.Contains(err.Error(), "action unresolved")) {
						t.Fatalf("successful action misreported: %v", err)
					}
					if receipt["state"] != "completed" {
						t.Fatalf("actual result lost %v", receipt)
					}
				}
			})
		}
	}
}

func TestAccessibleLiveIncludesFinalReceipt(t *testing.T) {
	// Real fake-worker runs with natural stream timing: no baseline consumer
	// delay and no synthetic event substitution.
	for range 3 {
		dir, repo := t.TempDir(), t.TempDir()
		exe, e := os.Executable()
		if e != nil {
			t.Fatal(e)
		}
		task, digest, e := seedDemoRepo(repo, exe, "pass")
		if e != nil {
			t.Fatal(e)
		}
		d, e := admission.Decide(t.Context(), admission.Request{StateDir: dir, Repo: repo, TaskFile: task, Adapter: "fake", Billing: "local-scripted", ExecutionProfile: "trusted-host", TrustProjectConfig: "sha256:" + digest, Env: os.Environ()})
		if e != nil {
			t.Fatal(e)
		}
		stdio, stdout, _ := testStdio()
		out, e := runAccessibleLive(stdio, dir, d.RunID, 0, func(ctx context.Context, h supervisor.Hooks) (supervisor.Outcome, error) {
			return supervisor.Run(ctx, d, h)
		})
		if e != nil || out.State != supervisor.RunReadyForReview {
			t.Fatalf("stream outcome %+v %v", out, e)
		}
		j := openJournal(t, dir)
		events, e := j.Events(t.Context(), d.RunID, 0)
		_ = j.Close()
		if e != nil {
			t.Fatal(e)
		}
		last := events[len(events)-1]
		if last.Type != "receipt.written" || accessibleTrailer(t, stdout.String()) != last.RunSequence {
			t.Fatalf("final receipt missing: last %+v output %q", last, stdout.String())
		}
	}
}

func TestLiveUnresolvedExit(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"recover", "apply"} {
		t.Run(kind, func(t *testing.T) {
			stdio, _, _ := testStdio()
			deps := forcedDeps(true, func(_ context.Context, cfg tui.Config) error {
				cfg.OnExit(tui.ExitIntent{Kind: tui.ExitUnresolvedAction, Phase: "ready_for_review"})
				return nil
			})
			_, err := runLiveTUI(stdio, deps, t.TempDir(), "run_cancelled", resolveTUIFlags(t, bareGetenv), func(_ context.Context, h supervisor.Hooks) (supervisor.Outcome, error) {
				select {
				case <-h.Interrupt:
					t.Error("action cancellation requested worker stop")
				default:
				}
				return supervisor.Outcome{State: supervisor.RunReadyForReview}, nil
			})
			if exitCode(err) != ExitOwnership || !strings.Contains(err.Error(), "MYTHHELM_HOME=") || !strings.Contains(err.Error(), "mythhelm recover") {
				t.Fatalf("cancelled %s exit: %v", kind, err)
			}
		})
	}
}

func TestHeadlessRecoverOutcomes(t *testing.T) {
	// Real fixture helpers and recovery use process-wide admission environment.
	for _, mode := range []tui.ClientMode{tui.ModeLive, tui.ModeInspect} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			dir, _, id, original := readyLifecycleRun(t)
			var owner *tuiLifecycle
			var actual supervisor.RecoveryOutcome
			deps, c := lossDeps(func(_ context.Context, cfg tui.Config) error {
				loseTerminal(owner)
				<-cfg.Signals
				cfg.OnExit(tui.ExitIntent{Kind: tui.ExitFinishHeadless, Phase: "recovering"})
				out, err := cfg.Actions.Recover(t.Context(), id, supervisor.Hooks{})
				actual = out
				cfg.OnHeadlessAction(tui.ActionCompletion{Kind: "recover", Recovery: out, Err: err})
				return nil
			})
			owner = c
			var err error
			if mode == tui.ModeLive {
				stdio, _, _ := testStdio()
				out, e := runLiveTUI(stdio, deps, dir, id, resolveTUIFlags(t, bareGetenv), func(context.Context, supervisor.Hooks) (supervisor.Outcome, error) { return original, nil })
				err = e
				if out.State != actual.State || out.Reason != actual.Reason {
					t.Fatalf("recovery outcome substituted %+v actual %+v", out, actual)
				}
			} else {
				err = runInspectTUI(deps, dir, id, resolveTUIFlags(t, bareGetenv))
			}
			if actual.State != supervisor.RunReadyForReview || exitCode(err) != ExitOwnership {
				t.Fatalf("actual recovery %+v, client %v", actual, err)
			}
			if mode == tui.ModeInspect && (strings.Contains(err.Error(), "action unresolved") || !strings.Contains(err.Error(), "recovery result:")) {
				t.Fatalf("successful recovery misreported %v", err)
			}
		})
	}
}

func TestAccessibleTerminalWatcherPrecedesReceipt(t *testing.T) {
	// This retained control demonstrates the causal race in the old live
	// wiring. The inspect watcher legitimately ends at ready_for_review while
	// the owner can still be holding its final receipt barrier.
	dir, id := driveRun(t, supervisor.RunAdmission, supervisor.RunExecuting, supervisor.RunVerifying, supervisor.RunReadyForReview)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{})
	close(started)
	joined := make(chan struct{})
	go func() { defer close(joined); watchRunTerminal(ctx, cancel, started, dir, id) }()
	t.Cleanup(func() { cancel(); <-joined })
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("terminal watcher never observed ready run")
	}
	<-joined
	j := openJournal(t, dir)
	defer func() { _ = j.Close() }()
	events, err := j.Events(t.Context(), id, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type == "receipt.written" {
			t.Fatal("receipt barrier was not held")
		}
	}
	// The same owner can now append its final receipt after watcher shutdown.
	writeTestReceipt(t, dir, id, "ready_for_review")
	events, err = j.Events(t.Context(), id, 0)
	if err != nil {
		t.Fatal(err)
	}
	if events[len(events)-1].Type != "receipt.written" {
		t.Fatal("owner did not append receipt after terminal observation")
	}
}

func TestHeadlessApplyInvalidBranchExit(t *testing.T) {
	// Standard apply and fixture routing bind process-wide MYTHHELM_HOME.
	for _, mode := range []tui.ClientMode{tui.ModeLive, tui.ModeInspect} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			dir, id := driveRunRepo(t, t.TempDir(), supervisor.RunAdmission, supervisor.RunExecuting, supervisor.RunVerifying, supervisor.RunReadyForReview)
			if e := os.MkdirAll(filepath.Join(dir, "runs", id), 0700); e != nil {
				t.Fatal(e)
			}
			const branch = "invalid..branch"
			code, _, stderr := runMain("apply", id, "--to-branch", branch)
			if code != int(ExitInvalid) || !strings.Contains(stderr, "invalid destination branch") {
				t.Fatalf("standard apply result %d: %q", code, stderr)
			}
			var owner *tuiLifecycle
			deps, c := lossDeps(func(_ context.Context, cfg tui.Config) error {
				loseTerminal(owner)
				<-cfg.Signals
				cfg.OnExit(tui.ExitIntent{Kind: tui.ExitFinishHeadless, Phase: "ready_for_review"})
				receipt, err := cfg.Actions.Apply(t.Context(), id, branch, false, false)
				if !errors.Is(err, workspace.ErrInvalidBranch) {
					t.Errorf("actual guarded apply error %v", err)
				}
				cfg.OnHeadlessAction(tui.ActionCompletion{Kind: "apply", Receipt: receipt, Err: err})
				return nil
			})
			owner = c
			var err error
			if mode == tui.ModeLive {
				stdio, _, _ := testStdio()
				_, err = runLiveTUI(stdio, deps, dir, id, resolveTUIFlags(t, bareGetenv), func(context.Context, supervisor.Hooks) (supervisor.Outcome, error) {
					return supervisor.Outcome{RunID: id, State: supervisor.RunReadyForReview}, nil
				})
			} else {
				err = runInspectTUI(deps, dir, id, resolveTUIFlags(t, bareGetenv))
			}
			if int(exitCode(err)) != code || !errors.Is(err, workspace.ErrInvalidBranch) {
				t.Fatalf("headless apply differs from standard exit %d or loses cause: code %d error %v", code, exitCode(err), err)
			}
			j := openJournal(t, dir)
			defer func() { _ = j.Close() }()
			row, e := j.Run(t.Context(), id)
			if e != nil || row.State != "ready_for_review" {
				t.Fatalf("refusal changed canonical state %+v %v", row, e)
			}
		})
	}
}

// Ordinary view exit and explicit cancellation do not emit OnHeadlessAction.
// Both must project the actual guarded action's journal after it has joined.
func TestLiveViewApplyProjection(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancelled%v", cancelled), func(t *testing.T) {
			dir, repo, id, original := readyLifecycleRun(t)
			original.AttemptID, original.AttemptState, original.AttemptReason = "old_attempt", supervisor.AttemptState("failed"), "old failure"
			proxy, gate, _ := applyGateGit(t)
			t.Setenv("PATH", proxy+string(os.PathListSeparator)+os.Getenv("PATH"))
			release := func() { _ = os.WriteFile(filepath.Join(gate, "release"), []byte("release"), 0600) }
			t.Cleanup(release)
			joined := make(chan struct{})
			var actionErr error
			deps := forcedDeps(true, func(_ context.Context, cfg tui.Config) error {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := make(chan struct{})
				go func() { defer close(done); _, actionErr = cfg.Actions.Apply(ctx, id, "review/view-exit", false, false) }()
				defer func() { cancel(); release(); <-done; close(joined) }()
				awaitLifecycleFile(t, filepath.Join(gate, "entered"))
				unlock, lockErr := supervisor.AcquireOwner(filepath.Join(dir, "runs", id))
				if lockErr == nil {
					unlock()
					t.Error("apply owner was not held at barrier")
				} else if !errors.Is(lockErr, supervisor.ErrOwnerHeld) {
					t.Error(lockErr)
				}
				kind := tui.ExitView
				if cancelled {
					cancel()
					kind = tui.ExitUnresolvedAction
				} else {
					release()
				}
				<-done
				if cancelled && (ctx.Err() != context.Canceled || actionErr == nil) {
					t.Errorf("cancelled action: %v, %v", ctx.Err(), actionErr)
				}
				if !cancelled && actionErr != nil {
					t.Errorf("successful action: %v", actionErr)
				}
				cfg.OnExit(tui.ExitIntent{Kind: kind, Phase: "applying"})
				return nil
			})
			stdio, stdout, _ := testStdio()
			out, err := runLiveTUI(stdio, deps, dir, id, resolveTUIFlags(t, bareGetenv), func(context.Context, supervisor.Hooks) (supervisor.Outcome, error) { return original, nil })
			select {
			case <-joined:
			default:
				t.Fatal("apply not joined")
			}
			unlock, lockErr := supervisor.AcquireOwner(filepath.Join(dir, "runs", id))
			if lockErr != nil {
				t.Fatal(lockErr)
			}
			unlock()
			j := openJournal(t, dir)
			defer func() { _ = j.Close() }()
			row, readErr := j.Run(t.Context(), id)
			if readErr != nil {
				t.Fatal(readErr)
			}
			attempt, readErr := j.LatestAttempt(t.Context(), id)
			if readErr != nil {
				t.Fatal(readErr)
			}
			receipt, _, readErr := supervisor.ReadReceipt(t.Context(), j, id)
			if readErr != nil {
				t.Fatal(readErr)
			}
			branch, readErr := workspace.BranchCommit(t.Context(), repo, "review/view-exit")
			if readErr != nil {
				t.Fatal(readErr)
			}
			wantState, wantExit := "completed", ExitOK
			if cancelled {
				wantState, wantExit = "applying", ExitOwnership
			}
			if row.State != wantState || exitCode(err) != wantExit {
				t.Fatalf("canonical %+v exit %d: %v", row, exitCode(err), err)
			}
			if cancelled && (branch != "" || receipt["state"] != "ready_for_review") {
				t.Fatalf("cancel changed ref/receipt: %q %v", branch, receipt)
			}
			if !cancelled && (branch == "" || receipt["state"] != "completed") {
				t.Fatalf("apply effects missing: %q %v", branch, receipt)
			}
			if string(out.State) != row.State || out.Reason != row.Reason || out.AttemptID != attempt.AttemptID || string(out.AttemptState) != attempt.State || out.AttemptReason != attempt.Reason {
				t.Fatalf("stale returned projection %+v; actual run %+v attempt %+v", out, row, attempt)
			}
			_ = finish(newRenderer("plain", stdio), out, err)
			if !strings.Contains(stdout.String(), "result: "+wantState) || strings.Contains(stdout.String(), "result: ready_for_review") {
				t.Fatalf("stale rendered result %q", stdout.String())
			}
			if cancelled && !strings.Contains(err.Error(), "MYTHHELM_HOME=") {
				t.Fatalf("missing recovery hint: %v", err)
			}
		})
	}
}

func TestLiveProjectionUnavailable(t *testing.T) {
	dir, _, id, original := readyLifecycleRun(t)
	fault := errors.New("projection read denied")
	for _, kind := range []tui.ExitKind{tui.ExitView, tui.ExitUnresolvedAction} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			before, paths := dbSHA256(t, dir), dirFiles(t, dir)
			calls := 0
			deps := forcedDeps(true, func(_ context.Context, cfg tui.Config) error { cfg.OnExit(tui.ExitIntent{Kind: kind}); return nil })
			deps.canonicalOutcome = func(context.Context, string, string) (supervisor.Outcome, error) {
				calls++
				return supervisor.Outcome{}, fault
			}
			stdio, stdout, _ := testStdio()
			original.Reason, original.AttemptReason = "stale run reason", "stale attempt reason"
			out, err := runLiveTUI(stdio, deps, dir, id, resolveTUIFlags(t, bareGetenv), func(context.Context, supervisor.Hooks) (supervisor.Outcome, error) { return original, nil })
			_ = finish(newRenderer("plain", stdio), out, err)
			if calls != 1 || out.RunID != id || out.State != "unknown" || out.Reason != "" || out.AttemptID != "" || out.AttemptState != "" || out.AttemptReason != "" || exitCode(err) != ExitOwnership || !errors.Is(err, fault) {
				t.Fatalf("unavailable projection %+v calls %d err %v", out, calls, err)
			}
			if !strings.Contains(err.Error(), "canonical state unavailable") || !strings.Contains(err.Error(), "MYTHHELM_HOME=") {
				t.Fatalf("unavailable guidance %v", err)
			}
			if !strings.Contains(stdout.String(), "result: unknown") || strings.Contains(stdout.String(), "not admitted") || strings.Contains(stdout.String(), "ready_for_review") || strings.Contains(stdout.String(), "stale") {
				t.Fatalf("false result %q", stdout.String())
			}
			if before != dbSHA256(t, dir) || fmt.Sprint(paths) != fmt.Sprint(dirFiles(t, dir)) {
				t.Fatal("projection failure changed canonical database or paths")
			}
		})
	}
}

func TestLiveViewPreservesOwnerError(t *testing.T) {
	dir, _, id, original := readyLifecycleRun(t)
	fault := errors.New("final receipt persistence failed")
	readFault := errors.New("projection denied")
	for _, unavailable := range []bool{false, true} {
		t.Run(strconv.FormatBool(unavailable), func(t *testing.T) {
			deps := forcedDeps(true, func(_ context.Context, cfg tui.Config) error {
				cfg.OnExit(tui.ExitIntent{Kind: tui.ExitView})
				return nil
			})
			if unavailable {
				deps.canonicalOutcome = func(context.Context, string, string) (supervisor.Outcome, error) {
					return supervisor.Outcome{}, readFault
				}
			}
			stdio, _, _ := testStdio()
			out, err := runLiveTUI(stdio, deps, dir, id, resolveTUIFlags(t, bareGetenv), func(context.Context, supervisor.Hooks) (supervisor.Outcome, error) { return original, fault })
			if exitCode(err) == ExitOK || !errors.Is(err, fault) {
				t.Fatalf("owner error lost: %+v %v", out, err)
			}
			if unavailable {
				if out.State != "unknown" || exitCode(err) != ExitOwnership || !errors.Is(err, readFault) {
					t.Fatalf("read failure lost: %+v %v", out, err)
				}
			} else if out.State != supervisor.RunReadyForReview {
				t.Fatalf("canonical projection lost: %+v", out)
			}
		})
	}
}

func TestHeadlessActionFailureProjectionUnavailable(t *testing.T) {
	dir, _, id, original := readyLifecycleRun(t)
	readFault := errors.New("canonical projection unavailable")
	for _, tc := range []struct {
		name        string
		kind        string
		actionFault error
		code        ExitCode
	}{
		{"usage", "apply", workspace.ErrInvalidBranch, ExitInvalid},
		{"blocked", "apply", supervisor.ErrApplyBlocked, ExitBlocked},
		{"internal", "apply", errors.New("apply persistence failed"), ExitInternal},
		{"recover open failure", "recover", errors.New("recover journal open failed"), ExitInternal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var owner *tuiLifecycle
			deps, c := lossDeps(func(_ context.Context, cfg tui.Config) error {
				loseTerminal(owner)
				<-cfg.Signals
				cfg.OnExit(tui.ExitIntent{Kind: tui.ExitFinishHeadless, Phase: "applying"})
				cfg.OnHeadlessAction(tui.ActionCompletion{Kind: tc.kind, Err: tc.actionFault})
				return nil
			})
			owner = c
			deps.canonicalOutcome = func(context.Context, string, string) (supervisor.Outcome, error) {
				return supervisor.Outcome{}, readFault
			}
			stdio, stdout, _ := testStdio()
			out, err := runLiveTUI(stdio, deps, dir, id, resolveTUIFlags(t, bareGetenv), func(context.Context, supervisor.Hooks) (supervisor.Outcome, error) { return original, nil })
			_ = finish(newRenderer("plain", stdio), out, err)
			if exitCode(err) != tc.code || !errors.Is(err, tc.actionFault) || !errors.Is(err, readFault) {
				t.Fatalf("failure precedence: code %d want %d: %v", exitCode(err), tc.code, err)
			}
			if out.RunID != id || out.State != "unknown" || out.Reason != "" || out.AttemptID != "" || out.AttemptState != "" || out.AttemptReason != "" {
				t.Fatalf("stale projection %+v", out)
			}
			if !strings.Contains(err.Error(), "canonical state unavailable") || !strings.Contains(err.Error(), "MYTHHELM_HOME=") || !strings.Contains(stdout.String(), "result: unknown") || strings.Contains(stdout.String(), "ready_for_review") || strings.Contains(stdout.String(), "not admitted") {
				t.Fatalf("unavailable guidance/output %v / %q", err, stdout.String())
			}
		})
	}
}

func TestHeadlessRecoverMissingOutcome(t *testing.T) {
	dir, _, id, original := readyLifecycleRun(t)
	fault := errors.New("recover journal open failed")
	for _, failed := range []bool{false, true} {
		t.Run(strconv.FormatBool(failed), func(t *testing.T) {
			var owner *tuiLifecycle
			deps, c := lossDeps(func(_ context.Context, cfg tui.Config) error {
				loseTerminal(owner)
				<-cfg.Signals
				cfg.OnExit(tui.ExitIntent{Kind: tui.ExitFinishHeadless, Phase: "recovering"})
				result := tui.ActionCompletion{Kind: "recover"}
				if failed {
					result.Err = fault
				}
				cfg.OnHeadlessAction(result)
				return nil
			})
			owner = c
			stdio, stdout, _ := testStdio()
			out, err := runLiveTUI(stdio, deps, dir, id, resolveTUIFlags(t, bareGetenv), func(context.Context, supervisor.Hooks) (supervisor.Outcome, error) { return original, nil })
			_ = finish(newRenderer("plain", stdio), out, err)
			want := ExitOwnership
			if failed {
				want = ExitInternal
			}
			if exitCode(err) != want || (failed && !errors.Is(err, fault)) || out.RunID != id || out.State != original.State || out.AttemptID != original.AttemptID {
				t.Fatalf("recover projection %+v code %d want %d: %v", out, exitCode(err), want, err)
			}
			if strings.Contains(stdout.String(), "not admitted") || !strings.Contains(stdout.String(), "result: ready_for_review") {
				t.Fatalf("false recovery result %q", stdout.String())
			}
		})
	}
}

func TestInspectRecoverMissingOutcome(t *testing.T) {
	deps := forcedDeps(true, func(_ context.Context, cfg tui.Config) error {
		cfg.OnExit(tui.ExitIntent{Kind: tui.ExitFinishHeadless, Phase: "recovering"})
		cfg.OnHeadlessAction(tui.ActionCompletion{Kind: "recover"})
		return nil
	})
	err := runInspectTUI(deps, t.TempDir(), "run_missing_recovery", resolveTUIFlags(t, bareGetenv))
	if exitCode(err) != ExitOwnership || !strings.Contains(err.Error(), "MYTHHELM_HOME=") || !strings.Contains(err.Error(), "recovery outcome unresolved") || strings.Contains(err.Error(), "recovery result:") {
		t.Fatalf("missing result should remain unresolved: %v", err)
	}
}

// Registered signal injection proves classifier/mailbox policy, not physical
// console-close delivery or any platform's detachment qualification.
func registeredTerminalSignal(t *testing.T) os.Signal {
	t.Helper()
	if tuiTerminalSignal(os.Interrupt) {
		t.Fatal("interrupt classified as terminal loss")
	}
	for _, sig := range tuiOSSignals() {
		if tuiTerminalSignal(sig) {
			return sig
		}
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		t.Fatal("platform has no registered terminal-loss signal")
	}
	t.Skip("platform has no defined terminal-loss signal")
	return nil
}

func TestRegisteredTerminalSignal(t *testing.T) {
	sig := registeredTerminalSignal(t)
	owner := newTUILifecycle()
	owner.osSignals <- sig
	select {
	case got := <-owner.signals:
		if got != tui.SignalTerminalLost {
			t.Errorf("typed signal %v", got)
		}
	case <-time.After(5 * time.Second):
		t.Error("terminal signal not delivered")
	}
	owner.close()
	if !owner.snapshot().lost {
		t.Error("terminal loss not latched")
	}
	select {
	case <-owner.detach:
		t.Error("loss detached while view still active")
	default:
	}
}

func TestTerminalLossAfterViewExit(t *testing.T) {
	sig := registeredTerminalSignal(t)
	for _, tc := range []struct {
		name                 string
		intent               *tui.ExitIntent
		action, stop, detach bool
	}{
		{name: "no intent before acceptance", detach: true},
		{name: "explicit detach", intent: &tui.ExitIntent{Kind: tui.ExitDetach}, detach: true},
		{name: "pending stop before loss", stop: true, detach: true},
		{name: "finish headless", intent: &tui.ExitIntent{Kind: tui.ExitFinishHeadless}},
		{name: "headless preserves earlier stop", intent: &tui.ExitIntent{Kind: tui.ExitFinishHeadless}, stop: true},
		{name: "unresolved action", intent: &tui.ExitIntent{Kind: tui.ExitUnresolvedAction}},
		{name: "ordinary view exit", intent: &tui.ExitIntent{Kind: tui.ExitView}},
		{name: "confirmed action", action: true},
		{name: "confirmed action with detach intent", action: true, intent: &tui.ExitIntent{Kind: tui.ExitDetach}},
	} {
		for _, lossFirst := range []bool{false, true} {
			t.Run(tc.name+"/loss-first-"+strconv.FormatBool(lossFirst), func(t *testing.T) {
				owner := newTUILifecycle()
				cfg := owner.config(t.TempDir(), "run_late_loss", resolveTUIFlags(t, bareGetenv), tui.ModeLive)
				if tc.intent != nil {
					cfg.OnExit(*tc.intent)
				}
				if tc.action {
					cfg.OnHeadlessAction(tui.ActionCompletion{Kind: "stop", StopState: "stopped"})
				}
				if tc.stop {
					cfg.RequestStop()
				}
				if !lossFirst {
					owner.endView()
				}
				owner.osSignals <- sig
				select {
				case got := <-cfg.Signals:
					if got != tui.SignalTerminalLost {
						t.Errorf("typed signal %v", got)
					}
				case <-time.After(5 * time.Second):
					t.Error("signal not delivered")
				}
				if lossFirst {
					owner.endView()
				}
				owner.close() // joins and drains the mailbox before checking absence.
				if !owner.snapshot().lost {
					t.Error("loss not retained")
				}
				gotDetach := false
				select {
				case <-owner.detach:
					gotDetach = true
				default:
				}
				if gotDetach != tc.detach {
					t.Errorf("detach %v want %v", gotDetach, tc.detach)
				}
				select {
				case <-owner.detach:
					t.Error("duplicate detach")
				default:
				}
				gotStop := false
				select {
				case <-owner.interrupt:
					gotStop = true
				default:
				}
				if gotStop != tc.stop {
					t.Errorf("stop %v want %v", gotStop, tc.stop)
				}
				select {
				case <-owner.interrupt:
					t.Error("duplicate stop")
				default:
				}
			})
		}

	}
}
