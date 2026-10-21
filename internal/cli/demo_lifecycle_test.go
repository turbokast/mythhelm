package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/tui"
)

func TestDemoRecoveryCommand(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state's $value;`literal` space")
	tests := []struct {
		name, goos, dir, id, want string
		bad                       bool
	}{
		{name: "posix", goos: "linux", dir: dir, id: "run_safe_123", want: "env 'MYTHHELM_HOME=" + strings.ReplaceAll(dir, "'", "'\"'\"'") + "' mythhelm recover 'run_safe_123'"},
		{name: "mac", goos: "darwin", dir: dir, id: "run_safe_123", want: "env 'MYTHHELM_HOME=" + strings.ReplaceAll(dir, "'", "'\"'\"'") + "' mythhelm recover 'run_safe_123'"},
		{name: "relative", goos: "linux", dir: "relative", id: "run_safe", bad: true},
		{name: "newline state", goos: "linux", dir: dir + "\n", id: "run_safe", bad: true},
		{name: "escape state", goos: "windows", dir: dir + "\x1b", id: "run_safe", bad: true},
		{name: "unsupported shell platform", goos: "plan9", dir: dir, id: "run_safe", bad: true},
	}
	for _, id := range []string{"not_run_safe", "run_", "run_a;touch", "run_$(evil)", "run_`evil`", "run_a'b", "run_a b", "run_a\n", "run_a.b", "run_" + strings.Repeat("a", 129)} {
		tests = append(tests, struct {
			name, goos, dir, id, want string
			bad                       bool
		}{name: "invalid " + id, goos: "linux", dir: dir, id: id, bad: true})
	}
	// Literal witnesses independently name every parser-recognised quote.
	for _, witness := range []struct{ input, escaped string }{
		{"'", "''"}, {"‘", "‘‘"}, {"’", "’’"}, {"‚", "‚‚"}, {"‛", "‛‛"},
		{"'‘’‚‛", "''‘‘’’‚‚‛‛"}, {"’ ; Set-Content injected owned; #", "’’ ; Set-Content injected owned; #"},
	} {
		root := filepath.VolumeName(dir) + string(os.PathSeparator)
		state := root + "state" + witness.input + "tail"
		tests = append(tests, struct {
			name, goos, dir, id, want string
			bad                       bool
		}{name: "PowerShell " + witness.input, goos: "windows", dir: state, id: "run_safe_123", want: "$env:MYTHHELM_HOME = '" + root + "state" + witness.escaped + "tail'; mythhelm recover 'run_safe_123'"})
	}
	longID := "run_" + strings.Repeat("A", 128)
	tests = append(tests, struct {
		name, goos, dir, id, want string
		bad                       bool
	}{name: "maximum bounded run ID", goos: "linux", dir: dir, id: longID, want: "env 'MYTHHELM_HOME=" + strings.ReplaceAll(dir, "'", "'\"'\"'") + "' mythhelm recover '" + longID + "'"})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := recoveryCommand(tc.dir, tc.id, tc.goos)
			if tc.bad {
				if err == nil || got != "" {
					t.Fatalf("unsafe input produced %q, %v", got, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("command = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

// The test binary is the native mythhelm stub: no script or second parser can
// hide argument/environment corruption by the generated command.
func recoveryCommandProbe(t *testing.T, shell string) {
	t.Helper()
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	name := "mythhelm"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	data, err := os.ReadFile(exe) //nolint:gosec // The trusted running test executable is copied as the native fixture.
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o700); err != nil { //nolint:gosec // The owned native stub must be executable.
		t.Fatal(err)
	}
	sentinel := filepath.Join(dir, "injected")
	variants := []string{"'", "‘", "’", "‚", "‛", "'‘’‚‛"}
	if shell == "pwsh" {
		variants = append(variants, "’ ; [System.IO.File]::WriteAllText('"+sentinel+"','owned'); #")
	} else {
		variants = append(variants, "' ; touch '"+sentinel+"' ; #")
	}
	for i, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			state := filepath.Join(dir, "state space "+variant+" suffix")
			output := filepath.Join(dir, "capture.json")
			command, err := recoveryCommand(state, "run_probe_42", runtime.GOOS)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			args := []string{"-c", command}
			if shell == "pwsh" {
				args = []string{"-NoProfile", "-NonInteractive", "-Command", command}
			}
			cmd := exec.CommandContext(ctx, shell, args...)
			cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "MYTHHELM_HOME=unrelated-caller-state", "MYTHHELM_TEST_RECOVERY_CAPTURE="+output)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("case %d command failed: %v: %s", i, err, out)
			}
			captureRoot, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			captured, err := captureRoot.ReadFile("capture.json")
			closeErr := captureRoot.Close()
			if closeErr != nil {
				t.Fatal(closeErr)
			}
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				State string
				Args  []string
			}
			if err := json.Unmarshal(captured, &got); err != nil {
				t.Fatal(err)
			}
			if got.State != state || len(got.Args) != 2 || got.Args[0] != "recover" || got.Args[1] != "run_probe_42" {
				t.Fatalf("capture = %+v; want state %q and exact recover argv", got, state)
			}
			if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
				t.Fatalf("injected effect exists: %v", err)
			}
		})
	}
}
func TestPOSIXRecoveryCommandExecution(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX execution is covered on Unix; native Windows requires PowerShell")
	}
	recoveryCommandProbe(t, "sh")
}
func TestPowerShellRecoveryCommandExecution(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("native PowerShell execution requires Windows CI")
	}
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Fatalf("required Windows PowerShell executable unavailable: %v", err)
	}
	recoveryCommandProbe(t, "pwsh")
}

// Process environment chooses the temporary parent and launch mode, so this
// test and its subtests deliberately do not run in parallel.
func TestDemoDetachRetainsWorkspace(t *testing.T) {
	for _, tc := range []struct {
		name   string
		out    supervisor.Outcome
		err    error
		retain bool
	}{
		{name: "running detach", out: supervisor.Outcome{State: supervisor.RunExecuting, AttemptState: supervisor.AttemptRunning, DetachedRunning: true}, retain: true},
		{name: "pending stop detach", out: supervisor.Outcome{State: supervisor.RunStopping, AttemptState: supervisor.AttemptStopRequested, Detached: true}, retain: true},
		{name: "active", out: supervisor.Outcome{State: supervisor.RunExecuting, AttemptState: supervisor.AttemptRunning}, retain: true},
		{name: "interrupted", out: supervisor.Outcome{State: supervisor.RunInterrupted, AttemptState: supervisor.AttemptInterrupted}, retain: true},
		{name: "terminal run active attempt", out: supervisor.Outcome{State: supervisor.RunCompleted, AttemptState: supervisor.AttemptRunning}, retain: true},
		{name: "terminal run missing attempt", out: supervisor.Outcome{State: supervisor.RunCompleted}, retain: true},
		{name: "unknown internal1", out: supervisor.Outcome{State: "unknown"}, err: errors.New("coordinator failed"), retain: true},
		{name: "unknown invalid2", out: supervisor.Outcome{State: "unknown"}, err: usageErrorf("action invalid"), retain: true},
		{name: "unknown blocked3", out: supervisor.Outcome{State: "unknown"}, err: &outcomeError{code: ExitBlocked, err: errors.New("action blocked")}, retain: true},
		{name: "terminal startup failure", out: supervisor.Outcome{State: supervisor.RunReadyForReview, AttemptState: supervisor.AttemptSucceededNative}, err: errors.New("startup failed"), retain: true},
		{name: "completed", out: supervisor.Outcome{State: supervisor.RunReadyForReview, AttemptState: supervisor.AttemptSucceededNative}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("TMPDIR", root)
			t.Setenv("TMP", root)
			t.Setenv("TEMP", root)
			t.Setenv("TERM", "xterm")
			stdio, stdout, stderr := testStdio()
			viewDone := make(chan struct{})
			joined := make(chan struct{})
			var repo, state, run string
			reviews := 0
			deps := demoDeps{tui: forcedDeps(true, func(context.Context, tui.Config) error { close(viewDone); return nil }), supervise: func(_ context.Context, d admission.Decision, _ supervisor.Hooks) (supervisor.Outcome, error) {
				<-viewDone
				if d.Scenario != "paced" {
					t.Errorf("live demo scenario %q, want paced", d.Scenario)
				}
				if got := string(d.Task.Content); got != "# Demo task\n\nWrite greeting.txt.\n" {
					t.Errorf("admitted live task %q does not describe the paced scenario's greeting.txt effect", got)
				}
				repo, state, run = d.Snapshot.SourceRepo, d.StateDir, d.RunID
				for _, dir := range []string{repo, state} {
					if err := os.WriteFile(filepath.Join(dir, "writer-sentinel"), []byte("preserve"), 0o600); err != nil {
						t.Error(err)
					}
				}
				out := tc.out
				out.RunID = d.RunID
				out.AttemptID = d.AttemptID
				close(joined)
				return out, tc.err
			}, review: func(_ io.Writer, gotState, gotRun string) error {
				reviews++
				select {
				case <-joined:
				default:
					t.Error("review before owner joined")
				}
				if gotState != state || gotRun != run {
					t.Errorf("review destination %q %q", gotState, gotRun)
				}
				for _, dir := range []string{repo, state} {
					if _, err := os.Stat(filepath.Join(dir, "writer-sentinel")); err != nil {
						t.Errorf("cleanup before review: %v", err)
					}
				}
				return nil
			}}
			err := runDemoWithDeps(nil, stdio, deps)
			if tc.out.Detached || tc.out.DetachedRunning {
				if exitCode(err) != ExitOwnership {
					t.Errorf("detach exit %d, want6", exitCode(err))
				}
			}
			if tc.err != nil && !errors.Is(err, tc.err) {
				t.Errorf("original cause lost: %v", err)
			}
			for _, dir := range []string{repo, state} {
				_, statErr := os.Stat(filepath.Join(dir, "writer-sentinel"))
				if tc.retain && statErr != nil {
					t.Errorf("retained directory %q removed: %v", dir, statErr)
				}
				if !tc.retain && !os.IsNotExist(statErr) {
					t.Errorf("completed directory %q not removed: %v", dir, statErr)
				}
			}
			if tc.retain {
				combined := stdout.String() + stderr.String()
				if err != nil {
					combined += err.Error()
				}
				if strings.Count(combined, "mythhelm recover") != 1 {
					t.Errorf("expected one recovery command across all output: %s", combined)
				}
				for _, want := range []string{"demo repository retained: " + repo, "demo state retained: " + state, "terminal outcome and no live writer", "Recovery preserves evidence", "Only then remove these two demo directories explicitly"} {
					if !strings.Contains(stdout.String(), want) {
						t.Errorf("retention notice missing %q: %s", want, stdout.String())
					}
				}
				if strings.Contains(combined, "removed afterwards") || strings.Contains(combined, ": done ===") {
					t.Errorf("premature cleanup claim: %s", combined)
				}
				if tc.err != nil && exitCode(err) != exitCode(tc.err) {
					t.Errorf("mapped error changed: %d != %d", exitCode(err), exitCode(tc.err))
				}
				if reviews != 0 || strings.Contains(stdout.String(), ": review ===") {
					t.Errorf("premature review (%d): %s", reviews, stdout.String())
				}
			} else if reviews != 1 {
				t.Errorf("completed review count %d", reviews)
			}
		})
	}
}

func TestRecoveryHintShellLabel(t *testing.T) {
	t.Parallel()
	state := t.TempDir()
	for _, tc := range []struct{ goos, label string }{{"linux", "POSIX sh/bash/zsh"}, {"darwin", "POSIX sh/bash/zsh"}, {"windows", "PowerShell"}} {
		t.Run(tc.goos, func(t *testing.T) {
			got := recoveryHint(state, "run_label", tc.goos)
			if !strings.HasPrefix(got, tc.label+": ") || !strings.Contains(got, "mythhelm recover 'run_label'") || !strings.Contains(got, "MYTHHELM_HOME") {
				t.Errorf("unusable labelled hint %q", got)
			}
			if strings.Contains(got, "fish") || strings.Contains(got, "generic shell") {
				t.Errorf("unsupported shell claim: %q", got)
			}
		})
	}
	manual := recoveryHint(state, "run_label", "plan9")
	if strings.Contains(manual, "mythhelm recover") || !strings.Contains(manual, "state "+state) || !strings.Contains(manual, "run run_label") || !strings.Contains(manual, "open a supported") {
		t.Errorf("unsupported platform guidance %q", manual)
	}
	invalid := recoveryHint(state+"\n", "run_bad;echo", "linux")
	if strings.Contains(invalid, "mythhelm recover") || strings.ContainsAny(invalid, "\n\x1b") {
		t.Errorf("unsafe invalid hint %q", invalid)
	}
}

func TestDemoRetentionNotice(t *testing.T) {
	// Owns process environment to force live launch and contain owned directories.
	root := t.TempDir()
	t.Setenv("TMPDIR", root)
	t.Setenv("TMP", root)
	t.Setenv("TEMP", root)
	t.Setenv("TERM", "xterm")
	stdio, stdout, stderr := testStdio()
	fault := errors.New("view startup failed")
	viewEnded := make(chan struct{})
	var repo, state, run string
	deps := demoDeps{tui: forcedDeps(true, func(context.Context, tui.Config) error { close(viewEnded); return fault }), supervise: func(_ context.Context, d admission.Decision, _ supervisor.Hooks) (supervisor.Outcome, error) {
		<-viewEnded
		repo, state, run = d.Snapshot.SourceRepo, d.StateDir, d.RunID
		return supervisor.Outcome{RunID: d.RunID, AttemptID: d.AttemptID, State: supervisor.RunReadyForReview, AttemptState: supervisor.AttemptSucceededNative}, nil
	}, review: func(io.Writer, string, string) error { t.Error("startup failure caused review"); return nil }}
	err := runDemoWithDeps(nil, stdio, deps)
	if !errors.Is(err, fault) {
		t.Fatalf("startup error lost: %v", err)
	}
	combined := stdout.String() + stderr.String() + err.Error()
	for _, want := range []string{"demo repository retained: " + repo, "demo state retained: " + state, "terminal outcome and no live writer", "fish or cmd", "Recovery preserves evidence"} {
		if !strings.Contains(combined, want) {
			t.Errorf("notice missing %q: %s", want, combined)
		}
	}
	if strings.Count(combined, "mythhelm recover") != 1 || strings.Count(combined, "mythhelm review") != 1 {
		t.Errorf("wrong recovery/review command count: %s", combined)
	}
	if !strings.Contains(stderr.String(), strings.Replace(literalRecoveryHintForTest(runtime.GOOS, state, run), "mythhelm recover", "mythhelm review", 1)) {
		t.Errorf("startup review command not state-bound: %s", stderr.String())
	}
	if strings.Contains(combined, "removed afterwards") || strings.Contains(combined, ": done ===") || strings.Contains(combined, ": review ===") {
		t.Errorf("premature completion: %s", combined)
	}
	for _, dir := range []string{repo, state} {
		if !isDir(dir) {
			t.Errorf("retained directory missing %q", dir)
		}
	}
}

func TestDemoReviewFailureRetainsEvidence(t *testing.T) {
	// Environment isolates the real temporary resources and forces TUI launch.
	root := t.TempDir()
	t.Setenv("TMPDIR", root)
	t.Setenv("TMP", root)
	t.Setenv("TEMP", root)
	t.Setenv("TERM", "xterm")
	stdio, stdout, stderr := testStdio()
	fault := errors.New("receipt unavailable")
	var repo, state string
	deps := demoDeps{tui: forcedDeps(true, func(context.Context, tui.Config) error { return nil }), supervise: func(_ context.Context, d admission.Decision, _ supervisor.Hooks) (supervisor.Outcome, error) {
		repo, state = d.Snapshot.SourceRepo, d.StateDir
		return supervisor.Outcome{RunID: d.RunID, AttemptID: d.AttemptID, State: supervisor.RunReadyForReview, AttemptState: supervisor.AttemptSucceededNative}, nil
	}, review: func(io.Writer, string, string) error { return fault }}
	err := runDemoWithDeps(nil, stdio, deps)
	if !errors.Is(err, fault) {
		t.Fatalf("review cause lost: %v", err)
	}
	if !isDir(repo) || !isDir(state) {
		t.Fatal("review failure deleted retained evidence")
	}
	combined := stdout.String() + stderr.String() + err.Error()
	if strings.Count(combined, "mythhelm recover") != 1 || strings.Contains(combined, ": done ===") || strings.Contains(combined, "was removed afterwards") {
		t.Errorf("invalid retention output: %s", combined)
	}
}
