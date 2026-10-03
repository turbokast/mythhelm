package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/tui"
	"github.com/turbokast/mythhelm/internal/tui/caps"
	"github.com/turbokast/mythhelm/internal/tui/theme"
)

// forcedDeps returns a tuiDeps with a forced isTerminal branch and the given
// runTUI, so tests select the TTY legs without a PTY.
func forcedDeps(terminal bool, runTUI func(context.Context, tui.Config) error) tuiDeps {
	return tuiDeps{
		isTerminal: func(io.Writer) bool { return terminal },
		runTUI:     runTUI,
	}
}

func bareGetenv(string) string { return "" }

func dumbGetenv(key string) string {
	if key == "TERM" {
		return "dumb"
	}
	return ""
}

// resolveTUIFlags parses args on a fresh TUI flag set and resolves them.
func resolveTUIFlags(t *testing.T, getenv func(string) string, args ...string) tuiOptions {
	t.Helper()
	fs := newFlagSet("tui-test")
	tf := addTUIFlags(fs)
	if _, err := ParseInterspersed(fs, args); err != nil {
		t.Fatalf("ParseInterspersed(%q): %v", args, err)
	}
	opts, err := tf.resolve(getenv)
	if err != nil {
		t.Fatalf("resolve(%q): %v", args, err)
	}
	return opts
}

// testStdio returns a piped Stdio with its buffers.
func testStdio() (Stdio, *bytes.Buffer, *bytes.Buffer) {
	var out, errOut bytes.Buffer
	return Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut}, &out, &errOut
}

// driveRun builds a fresh run driven through states, returning its state dir
// and run ID.
func driveRun(t *testing.T, states ...supervisor.RunState) (string, string) {
	t.Helper()
	return driveRunRepo(t, "/tmp/repo", states...)
}

// driveRunRepo is driveRun with an explicit source repository: branches are
// validated against it, so tests reaching past CheckBranch pass a real
// directory.
func driveRunRepo(t *testing.T, repo string, states ...supervisor.RunState) (string, string) {
	t.Helper()
	dir := stateHome(t)
	j := openJournal(t, dir)
	defer func() { _ = j.Close() }()
	producer := supervisor.NewProducer(ids.New("sup"), 1)
	runID := createRun(t, j, producer, repo)
	for _, state := range states {
		if err := supervisor.TransitionRun(t.Context(), j, runID, state, "", producer); err != nil {
			t.Fatalf("TransitionRun %s: %v", state, err)
		}
	}
	return dir, runID
}

// insertAttempt records one running attempt for runID, returning its ID.
func insertAttempt(t *testing.T, dir, runID string) string {
	t.Helper()
	j := openJournal(t, dir)
	defer func() { _ = j.Close() }()
	attemptID := ids.New("att")
	err := j.Append(t.Context(), journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"), RunID: runID, AttemptID: attemptID,
		ProducerID: ids.New("sup"), ProducerSequence: 1, Generation: 1,
		ObservedAt: time.Now().UTC(), Type: "attempt.launch_intent_recorded", Payload: []byte(`{}`),
	}, func(tx *sql.Tx) error {
		return journal.InsertAttempt(t.Context(), tx, journal.AttemptRow{
			AttemptID: attemptID, RunID: runID, TaskID: ids.New("task"), AttemptNumber: 1,
			State: "running", LaunchTokenSHA256: "token", WorkspacePath: dir,
		})
	})
	if err != nil {
		t.Fatalf("Append attempt: %v", err)
	}
	return attemptID
}

// writeTestReceipt writes a receipt in the given state and journals its
// receipt.written event.
func writeTestReceipt(t *testing.T, dir, runID, state string) {
	t.Helper()
	j := openJournal(t, dir)
	defer func() { _ = j.Close() }()
	runDir := filepath.Join(dir, "runs", runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	r := supervisor.Receipt{
		"schema_version": 1, "run_id": runID, "state": state, "exit_code": 0,
		"requested_outcome": map[string]any{"title": "Demo task"},
		"candidate":         map[string]any{"commit": "unknown", "flags": []any{}},
		"verification":      map[string]any{"checks": []any{}},
	}
	sha, err := supervisor.WriteReceipt(runDir, r)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"schema_version": 1, "sha256": sha, "path": "receipt.json"})
	if err != nil {
		t.Fatal(err)
	}
	err = j.Append(t.Context(), journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"), RunID: runID,
		ProducerID: ids.New("sup"), ProducerSequence: 1, Generation: 1,
		ObservedAt: time.Now().UTC(), Type: "receipt.written", Payload: payload,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

// executingFixture builds a run in executing with no receipt.
func executingFixture(t *testing.T) (string, string) {
	t.Helper()
	return driveRun(t, supervisor.RunAdmission, supervisor.RunExecuting)
}

// completedFixture builds a run driven to completed with a matching receipt.
func completedFixture(t *testing.T) (string, string) {
	t.Helper()
	dir, runID := driveRun(t,
		supervisor.RunAdmission, supervisor.RunExecuting, supervisor.RunVerifying,
		supervisor.RunReadyForReview, supervisor.RunApplying, supervisor.RunCompleted)
	writeTestReceipt(t, dir, runID, string(supervisor.RunCompleted))
	return dir, runID
}

// accessibleTrailer returns the next-after cursor from out's last line,
// failing unless it is the trailer.
func accessibleTrailer(t *testing.T, out string) int64 {
	t.Helper()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	var after int64
	if _, err := fmt.Sscanf(lines[len(lines)-1], "next-after: %d", &after); err != nil {
		t.Fatalf("last line %q is not a next-after trailer: %v", lines[len(lines)-1], err)
	}
	return after
}

// accessibleEventSeqs returns the run sequences of out's event lines, in
// output order.
func accessibleEventSeqs(t *testing.T, out string) []int64 {
	t.Helper()
	var seqs []int64
	for _, line := range strings.Split(out, "\n") {
		rest, ok := strings.CutPrefix(line, "event ")
		if !ok {
			continue
		}
		num, _, _ := strings.Cut(rest, ":")
		n, err := strconv.ParseInt(strings.TrimSpace(num), 10, 64)
		if err != nil {
			t.Fatalf("event line %q has no sequence: %v", line, err)
		}
		seqs = append(seqs, n)
	}
	return seqs
}

func TestLaunchRuleMatrix(t *testing.T) {
	t.Run("format jsonl stays JSONL", func(t *testing.T) {
		stateHome(t)
		code, out, _ := runMain("run", "--format", "jsonl")
		if code == 0 {
			t.Fatal("run --format jsonl without a task unexpectedly succeeded")
		}
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if len(lines) != 1 {
			t.Fatalf("JSONL branch printed %d lines, want 1: %q", len(lines), out)
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(lines[0]), &obj); err != nil {
			t.Fatalf("JSONL branch printed no JSON object: %q", out)
		}
		if obj["type"] != "run.result" {
			t.Fatalf("JSONL branch type = %v, want run.result", obj["type"])
		}
		// TUI flags present must not disturb machine output.
		code, out2, _ := runMain("run", "--format", "jsonl",
			"--colour", "always", "--motion", "full", "--icons", "unicode",
			"--accessible", "--after", "5")
		if code == 0 {
			t.Fatal("run --format jsonl with TUI flags unexpectedly succeeded")
		}
		var obj2 map[string]any
		if err := json.Unmarshal([]byte(strings.TrimRight(out2, "\n")), &obj2); err != nil {
			t.Fatalf("JSONL branch with TUI flags printed no JSON object: %q", out2)
		}
		if obj2["type"] != "run.result" {
			t.Fatalf("JSONL branch with TUI flags type = %v, want run.result", obj2["type"])
		}
	})

	t.Run("plain and non-TTY select linear", func(t *testing.T) {
		stateHome(t)
		plainCode, plainOut, plainErr := runMain("run", "--plain")
		defCode, defOut, defErr := runMain("run")
		if plainCode == 0 || defCode == 0 {
			t.Fatal("run without a task unexpectedly succeeded")
		}
		if !strings.HasPrefix(plainOut, "result: ") || !strings.HasPrefix(defOut, "result: ") {
			t.Fatalf("linear legs printed no result line: %q vs %q", plainOut, defOut)
		}
		if plainOut != defOut || plainErr != defErr || plainCode != defCode {
			t.Fatalf("--plain output differs from default piped output:\n%q/%q/%d\nvs\n%q/%q/%d",
				plainOut, plainErr, plainCode, defOut, defErr, defCode)
		}
	})

	t.Run("accessible selects the accessible stream", func(t *testing.T) {
		_, runID := completedFixture(t)
		code, out, stderr := runMain("review", runID, "--accessible")
		if code != 0 {
			t.Fatalf("review --accessible exit %d: %s", code, stderr)
		}
		if !strings.Contains(out, "goal: Demo task") || !strings.Contains(out, "run state: completed") {
			t.Fatalf("accessible stream lacks its summary labels: %q", out)
		}
		seqs := accessibleEventSeqs(t, out)
		if len(seqs) == 0 || seqs[0] != 1 {
			t.Fatalf("accessible stream seqs = %v, want a stream from 1", seqs)
		}
		if got := accessibleTrailer(t, out); got != seqs[len(seqs)-1] {
			t.Fatalf("trailer next-after = %d, last streamed = %d", got, seqs[len(seqs)-1])
		}
	})

	t.Run("review linear legs unchanged", func(t *testing.T) {
		_, runID := completedFixture(t)
		code, out, stderr := runMain("review", runID, "--no-diff")
		if code != 0 {
			t.Fatalf("review exit %d: %s", code, stderr)
		}
		if !strings.Contains(out, "Run: "+runID) || !strings.Contains(out, "State: completed") {
			t.Fatalf("linear review lacks its labels: %q", out)
		}
		code, out, stderr = runMain("review", runID, "--format", "jsonl")
		if code != 0 {
			t.Fatalf("review --format jsonl exit %d: %s", code, stderr)
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(strings.TrimRight(out, "\n")), &obj); err != nil {
			t.Fatalf("review --format jsonl printed no JSON object: %q", out)
		}
	})

	t.Run("selection table", func(t *testing.T) {
		var buf bytes.Buffer
		tty := forcedDeps(true, nil)
		pipe := forcedDeps(false, nil)
		tests := []struct {
			name       string
			format     string
			plain      bool
			accessible bool
			deps       tuiDeps
			getenv     func(string) string
			want       launchMode
		}{
			{"jsonl wins on a TTY", "jsonl", false, false, tty, bareGetenv, launchJSONL},
			{"jsonl beats accessible", "jsonl", false, true, tty, bareGetenv, launchJSONL},
			{"jsonl wins piped", "jsonl", false, false, pipe, bareGetenv, launchJSONL},
			{"explicit plain wins on a TTY", "plain", true, false, tty, bareGetenv, launchLinear},
			{"explicit plain beats accessible", "plain", true, true, tty, bareGetenv, launchLinear},
			{"accessible streams piped", "plain", false, true, pipe, bareGetenv, launchAccessible},
			{"accessible wins on a TTY", "plain", false, true, tty, bareGetenv, launchAccessible},
			{"accessible beats dumb", "plain", false, true, tty, dumbGetenv, launchAccessible},
			{"piped stays linear", "plain", false, false, pipe, bareGetenv, launchLinear},
			{"dumb stays linear", "plain", false, false, tty, dumbGetenv, launchLinear},
			{"TTY launches the TUI", "plain", false, false, tty, bareGetenv, launchTUI},
			{"format plain still launches the TUI", "plain", false, false, tty, bareGetenv, launchTUI},
			{"demo forced TTY launches the TUI", "plain", false, false, tty, bareGetenv, launchTUI},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := selectLaunch(tt.format, tt.plain, tt.accessible, &buf, tt.deps, tt.getenv); got != tt.want {
					t.Fatalf("selectLaunch(%q, plain=%v, accessible=%v) = %d, want %d",
						tt.format, tt.plain, tt.accessible, got, tt.want)
				}
			})
		}
	})

	t.Run("demo offers no format flag", func(t *testing.T) {
		stateHome(t)
		if code, _, _ := runMain("demo", "--format", "jsonl"); code != 2 {
			t.Fatalf("demo --format exit %d, want 2 (no --format flag)", code)
		}
	})

	t.Run("plain stays run-only", func(t *testing.T) {
		stateHome(t)
		if code, _, _ := runMain("review", "run_missing", "--plain"); code != 2 {
			t.Fatalf("review --plain exit %d, want 2 (no --plain flag)", code)
		}
		if code, _, _ := runMain("demo", "--plain"); code != 2 {
			t.Fatalf("demo --plain exit %d, want 2 (no --plain flag)", code)
		}
	})

	t.Run("live branch wires LiveFeed", func(t *testing.T) {
		dir := t.TempDir()
		const runID = "run_livewire1"
		opts := resolveTUIFlags(t, bareGetenv)
		stdio, out, _ := testStdio()
		probe := journal.Event{SchemaVersion: journal.EnvelopeVersion, EventID: "evt_probe", RunID: runID, Type: "probe"}
		supervise := func(_ context.Context, h supervisor.Hooks) (supervisor.Outcome, error) {
			if h.Event == nil || h.Notice == nil {
				t.Error("live supervise hooks lack Event or Notice")
			}
			if h.Interrupt == nil {
				t.Error("live supervise hooks lack Interrupt")
			} else if cap(h.Interrupt) != 2 {
				t.Errorf("cap(Interrupt) = %d, want 2 as in executeRun", cap(h.Interrupt))
			}
			// The probe proves cfg.Events is the channel these hooks feed.
			h.Event(probe)
			return supervisor.Outcome{RunID: runID, State: supervisor.RunCompleted}, nil
		}
		var gotCfg tui.Config
		deps := forcedDeps(true, func(_ context.Context, cfg tui.Config) error {
			gotCfg = cfg
			if cfg.RunID != runID || cfg.StateDir != dir {
				t.Errorf("cfg identity = %q/%q, want %q/%q", cfg.RunID, cfg.StateDir, runID, dir)
			}
			if cap(cfg.Events) != 256 {
				t.Errorf("cap(cfg.Events) = %d, want 256 from tui.LiveFeed", cap(cfg.Events))
			}
			if cfg.Notices == nil {
				t.Error("cfg.Notices is nil, want the LiveFeed backlog")
			} else {
				cfg.Notices.Add("notice-probe")
				if drained := cfg.Notices.Drain(); len(drained) != 1 || drained[0] != "notice-probe" {
					t.Errorf("Notices Add/Drain round-trip = %q, want [notice-probe]", drained)
				}
			}
			if cfg.Actions.Stop == nil || cfg.Actions.Recover == nil || cfg.Actions.Apply == nil {
				t.Error("cfg.Actions has a nil entry, want the wired supervisor calls")
			}
			select {
			case ev := <-cfg.Events:
				if ev.Type != "probe" || ev.EventID != "evt_probe" {
					t.Errorf("probe event = %+v, want the supervised probe", ev)
				}
			case <-time.After(10 * time.Second):
				t.Error("supervised probe never arrived on cfg.Events")
			}
			return nil
		})
		outcome, err := runLiveTUI(stdio, deps, dir, runID, opts, supervise)
		if err != nil {
			t.Fatalf("runLiveTUI: %v", err)
		}
		if outcome.State != supervisor.RunCompleted {
			t.Fatalf("outcome state = %s, want completed", outcome.State)
		}
		if out.Len() != 0 {
			t.Fatalf("completed live run printed %q, want silence", out.String())
		}
		wantTokens, err := theme.BuiltIn("dark")
		if err != nil {
			t.Fatal(err)
		}
		if gotCfg.Tokens != wantTokens {
			t.Fatalf("cfg.Tokens = %+v, want the dark built-in", gotCfg.Tokens)
		}
		if gotCfg.Caps != opts.caps {
			t.Fatalf("cfg.Caps = %+v, want %+v", gotCfg.Caps, opts.caps)
		}
	})

	t.Run("inspect branch carries no live channel", func(t *testing.T) {
		dir := t.TempDir()
		const runID = "run_inspect1"
		opts := resolveTUIFlags(t, bareGetenv)
		calls := 0
		deps := forcedDeps(true, func(_ context.Context, cfg tui.Config) error {
			calls++
			if cfg.RunID != runID || cfg.StateDir != dir {
				t.Errorf("cfg identity = %q/%q, want %q/%q", cfg.RunID, cfg.StateDir, runID, dir)
			}
			if cfg.Events != nil {
				t.Error("inspect cfg.Events is non-nil, want no live channel")
			}
			if cfg.Actions.Stop == nil || cfg.Actions.Recover == nil || cfg.Actions.Apply == nil {
				t.Error("inspect cfg.Actions has a nil entry")
			}
			return nil
		})
		if err := runInspectTUI(deps, dir, runID, opts); err != nil {
			t.Fatalf("runInspectTUI: %v", err)
		}
		if calls != 1 {
			t.Fatalf("runTUI calls = %d, want 1", calls)
		}
		sentinel := errors.New("tui_test: tea failed")
		deps = forcedDeps(true, func(context.Context, tui.Config) error { return sentinel })
		if err := runInspectTUI(deps, dir, runID, opts); !errors.Is(err, sentinel) {
			t.Fatalf("runInspectTUI error = %v, want the runTUI failure", err)
		}
	})
}

func TestInvalidModesExit2(t *testing.T) {
	tests := []struct {
		args []string
		flag string
	}{
		{[]string{"run", "--colour", "rainbow"}, "--colour"},
		{[]string{"run", "--motion", "turbo"}, "--motion"},
		{[]string{"run", "--icons", "emoji"}, "--icons"},
		{[]string{"run", "--after", "xyz"}, "--after"},
		{[]string{"review", "run_missing", "--colour", "rainbow"}, "--colour"},
		{[]string{"review", "run_missing", "--motion", "turbo"}, "--motion"},
		{[]string{"review", "run_missing", "--icons", "emoji"}, "--icons"},
		{[]string{"review", "run_missing", "--after", "xyz"}, "--after"},
		{[]string{"demo", "--colour", "rainbow"}, "--colour"},
		{[]string{"demo", "--motion", "turbo"}, "--motion"},
		{[]string{"demo", "--icons", "emoji"}, "--icons"},
		{[]string{"demo", "--after", "xyz"}, "--after"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			stateHome(t)
			code, _, stderr := runMain(tt.args...)
			if code != 2 {
				t.Fatalf("%q exit %d, want 2", tt.args, code)
			}
			if !strings.Contains(stderr, tt.flag) {
				t.Fatalf("%q stderr %q names no %s", tt.args, stderr, tt.flag)
			}
		})
	}

	t.Run("capability errors wrap ErrInvalidMode", func(t *testing.T) {
		for _, args := range [][]string{
			{"--colour", "rainbow"},
			{"--motion", "turbo"},
			{"--icons", "emoji"},
		} {
			fs := newFlagSet("tui-test")
			tf := addTUIFlags(fs)
			if _, err := ParseInterspersed(fs, args); err != nil {
				t.Fatal(err)
			}
			_, err := tf.resolve(bareGetenv)
			if !errors.Is(err, caps.ErrInvalidMode) {
				t.Fatalf("resolve(%q) error = %v, want caps.ErrInvalidMode", args, err)
			}
			var usage usageError
			if !errors.As(err, &usage) {
				t.Fatalf("resolve(%q) error = %T, want a usageError", args, err)
			}
		}
	})

	t.Run("after parses integers", func(t *testing.T) {
		if got := resolveTUIFlags(t, bareGetenv, "--after", "42").after; got != 42 {
			t.Fatalf("after = %d, want 42", got)
		}
		if got := resolveTUIFlags(t, bareGetenv).after; got != 0 {
			t.Fatalf("default after = %d, want 0", got)
		}
	})
}

func TestAfterFlagResumesAccessible(t *testing.T) {
	_, runID := completedFixture(t)
	code, out, stderr := runMain("review", runID, "--accessible", "--after", "3")
	if code != 0 {
		t.Fatalf("review --accessible --after exit %d: %s", code, stderr)
	}
	if strings.Contains(out, "goal: ") {
		t.Fatalf("resume stream repeats the summary labels: %q", out)
	}
	seqs := accessibleEventSeqs(t, out)
	if len(seqs) == 0 {
		t.Fatalf("resume stream has no events: %q", out)
	}
	for i, seq := range seqs {
		if want := int64(4 + i); seq != want {
			t.Fatalf("resume seqs = %v, want the contiguous stream from 4", seqs)
		}
	}
	if got := accessibleTrailer(t, out); got != seqs[len(seqs)-1] {
		t.Fatalf("trailer next-after = %d, last streamed = %d", got, seqs[len(seqs)-1])
	}

	// Off the accessible branch --after is ignored: the linear output is
	// byte-identical with and without it.
	_, plainWith, _ := runMain("review", runID, "--no-diff", "--after", "3")
	code, plainWithout, stderr := runMain("review", runID, "--no-diff")
	if code != 0 {
		t.Fatalf("review --no-diff exit %d: %s", code, stderr)
	}
	if plainWith != plainWithout {
		t.Fatalf("--after changed linear output:\n%q\nvs\n%q", plainWith, plainWithout)
	}
}

func TestReviewAccessibleQuiescentReturnsPromptly(t *testing.T) {
	// ready_for_review is inactive (outside supervisor activeStates) with a
	// static journal: the stream must end on its own with its trailer
	// instead of following live until Ctrl-C.
	dir, runID := driveRun(t,
		supervisor.RunAdmission, supervisor.RunExecuting, supervisor.RunVerifying,
		supervisor.RunReadyForReview)
	writeTestReceipt(t, dir, runID, string(supervisor.RunReadyForReview))
	type result struct {
		code        int
		out, stderr string
	}
	done := make(chan result, 1)
	go func() {
		code, out, stderr := runMain("review", runID, "--accessible")
		done <- result{code, out, stderr}
	}()
	select {
	case res := <-done:
		if res.code != 0 {
			t.Fatalf("review --accessible exit %d: %s", res.code, res.stderr)
		}
		seqs := accessibleEventSeqs(t, res.out)
		if len(seqs) == 0 {
			t.Fatalf("quiescent stream has no events: %q", res.out)
		}
		if got := accessibleTrailer(t, res.out); got != seqs[len(seqs)-1] {
			t.Fatalf("trailer next-after = %d, last streamed = %d", got, seqs[len(seqs)-1])
		}
	case <-time.After(10 * time.Second):
		t.Fatal("review --accessible on a ready_for_review run never returned")
	}
}

func TestColorAlias(t *testing.T) {
	colour := resolveTUIFlags(t, bareGetenv, "--colour", "never")
	alias := resolveTUIFlags(t, bareGetenv, "--color", "never") //nolint:misspell // --color is the required alias spelling.
	if colour.prefs.Colour != "never" || alias.prefs.Colour != "never" {
		t.Fatalf("prefs = %q/%q, want never/never", colour.prefs.Colour, alias.prefs.Colour)
	}
	if colour.caps != alias.caps || colour.caps.Colour != caps.ColourNever {
		t.Fatalf("caps = %+v/%+v, want matching ColourNever", colour.caps, alias.caps)
	}
	def := resolveTUIFlags(t, bareGetenv)
	if def.prefs.Colour != "auto" {
		t.Fatalf("default colour = %q, want auto", def.prefs.Colour)
	}
}

func TestUntouchedVerbsStable(t *testing.T) {
	// Goldens captured from the pre-change tree (origin/main a58a6d5): the
	// new presentation flags exist only on run, demo and review, so every
	// other verb's output with the flags absent must match byte for byte.
	t.Run("runs list", func(t *testing.T) {
		stateHome(t)
		code, out, stderr := runMain("runs", "list")
		if code != 0 || out != "no runs\n" || stderr != "" {
			t.Fatalf("runs list = %d/%q/%q, want 0/\"no runs\\n\"/\"\"", code, out, stderr)
		}
		code, out, stderr = runMain("runs", "list", "--format", "jsonl")
		if code != 0 || out != "" || stderr != "" {
			t.Fatalf("runs list --format jsonl = %d/%q/%q, want empty output", code, out, stderr)
		}
	})

	t.Run("error paths", func(t *testing.T) {
		dir := stateHome(t)
		j := openJournal(t, dir)
		if err := j.Close(); err != nil {
			t.Fatal(err)
		}
		tests := []struct {
			name   string
			args   []string
			code   int
			stdout string
			stderr string
		}{
			{"stop unknown run", []string{"stop", "run_missing"}, 1, "",
				"mythhelm stop: journal: run run_missing: not found\n"},
			{"recover unknown run", []string{"recover", "run_missing"}, 1,
				"result: not admitted (internal); exit 1 internal\n",
				"mythhelm recover: journal: run run_missing: not found\n"},
			{"apply unknown run", []string{"apply", "run_missing", "--to-branch", "x"}, 1, "",
				"mythhelm apply: journal: run run_missing: not found\n"},
			{"stop usage", []string{"stop"}, 2, "",
				"mythhelm stop: expected one valid run ID\nrun 'mythhelm stop -h' for usage\n"},
			{"runs usage", []string{"runs"}, 2, "",
				"mythhelm runs: missing subcommand; usage: mythhelm runs list [--format plain|jsonl] [--limit N]\nrun 'mythhelm runs -h' for usage\n"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				code, out, stderr := runMain(tt.args...)
				if code != tt.code || out != tt.stdout || stderr != tt.stderr {
					t.Fatalf("%q = %d/%q/%q, want %d/%q/%q",
						tt.args, code, out, stderr, tt.code, tt.stdout, tt.stderr)
				}
			})
		}
	})

	t.Run("version template", func(t *testing.T) {
		stateHome(t)
		normalise := func(s string) string {
			s = regexp.MustCompile(`(?m)^commit: .*$`).ReplaceAllString(s, "commit: TEST")
			s = regexp.MustCompile(`(?m)^go: .*$`).ReplaceAllString(s, "go: TEST")
			return regexp.MustCompile(`(?m)^platform: .*$`).ReplaceAllString(s, "platform: TEST")
		}
		code, out, stderr := runMain("version")
		if code != 0 || stderr != "" {
			t.Fatalf("version = %d/%q/%q, want exit 0 and no stderr", code, out, stderr)
		}
		// The commit, toolchain and platform lines vary by checkout and
		// machine; the template around them is the stable contract.
		if want, got := "mythhelm devel\ncommit: TEST\ngo: TEST\nplatform: TEST\n", normalise(out); got != want {
			t.Fatalf("version template = %q, want %q", got, want)
		}
	})

	t.Run("doctor shape", func(t *testing.T) {
		stateHome(t)
		code, out, stderr := runMain("doctor")
		if code != 0 || stderr != "" {
			t.Fatalf("doctor = %d/%q/%q, want exit 0 and no stderr", code, out, stderr)
		}
		// Doctor reports live environment facts, so only its line labels
		// and their order are stable across machines.
		prefixes := []string{"mythhelm: ", "git: ", "claude: ", "credential env names: ",
			"native settings: ", "sandbox present: ", "state dir: ", "terminal: TERM="}
		at := 0
		for _, prefix := range prefixes {
			i := strings.Index(out[at:], prefix)
			if i < 0 {
				t.Fatalf("doctor output lacks %q after offset %d: %q", prefix, at, out)
			}
			at += i + len(prefix)
		}
	})
}

func TestQuitLiveDetaches(t *testing.T) {
	dir := t.TempDir()
	const runID = "run_detach1"
	opts := resolveTUIFlags(t, bareGetenv)
	stdio, out, _ := testStdio()
	started := make(chan struct{})
	finish := make(chan struct{})
	interrupts := make(chan int, 1)
	supervise := func(_ context.Context, h supervisor.Hooks) (supervisor.Outcome, error) {
		close(started)
		defer close(finish)
		seen := 0
		for seen < 2 {
			select {
			case <-h.Interrupt:
				seen++
			case <-time.After(10 * time.Second):
				t.Error("timed out waiting for the detach interrupts")
				interrupts <- seen
				return supervisor.Outcome{}, errors.New("tui_test: interrupt timeout")
			}
		}
		interrupts <- seen
		return supervisor.Outcome{RunID: runID, State: supervisor.RunInterrupted, Detached: true}, nil
	}
	deps := forcedDeps(true, func(context.Context, tui.Config) error {
		select {
		case <-started:
		case <-time.After(10 * time.Second):
			t.Fatal("pipeline never started")
		}
		select {
		case <-finish:
			t.Error("pipeline finished before the TUI quit")
		default:
		}
		return nil
	})
	outcome, err := runLiveTUI(stdio, deps, dir, runID, opts, supervise)
	if !outcome.Detached {
		t.Fatalf("outcome = %+v, want a detach", outcome)
	}
	seen := <-interrupts
	if seen != 2 {
		t.Fatalf("pipeline saw %d interrupts, want 2 (detach, not stop)", seen)
	}
	want := "detached from run run_detach1; reattach with: mythhelm review run_detach1\n"
	if out.String() != want {
		t.Fatalf("detach output = %q, want %q", out.String(), want)
	}
	if exitCode(err) != ExitOwnership {
		t.Fatalf("detach exit = %d, want %d", exitCode(err), ExitOwnership)
	}
	if !strings.Contains(err.Error(), "detached") {
		t.Fatalf("detach error = %q, want the detach wording", err.Error())
	}
}

func TestReviewUnknownRunExits1(t *testing.T) {
	t.Run("through Main without a database", func(t *testing.T) {
		stateHome(t)
		code, out, stderr := runMain("review", "run_missing")
		if code != 1 || out != "" || stderr != "mythhelm review: run run_missing: not found\n" {
			t.Fatalf("review run_missing = %d/%q/%q, want exit 1 with the not-found message",
				code, out, stderr)
		}
	})

	t.Run("through Main with a database", func(t *testing.T) {
		dir := stateHome(t)
		j := openJournal(t, dir)
		if err := j.Close(); err != nil {
			t.Fatal(err)
		}
		code, out, stderr := runMain("review", "run_missing")
		if code != 1 || out != "" || stderr != "mythhelm review: journal: run run_missing: not found\n" {
			t.Fatalf("review run_missing = %d/%q/%q, want exit 1 with the not-found message",
				code, out, stderr)
		}
	})

	t.Run("forced TTY starts no TUI", func(t *testing.T) {
		dir := stateHome(t)
		j := openJournal(t, dir)
		if err := j.Close(); err != nil {
			t.Fatal(err)
		}
		opts := resolveTUIFlags(t, bareGetenv)
		stdio, _, _ := testStdio()
		called := false
		deps := forcedDeps(true, func(context.Context, tui.Config) error {
			called = true
			return nil
		})
		err := runReviewDispatch(stdio, deps, bareGetenv, dir, "run_missing", "plain", true, opts)
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("dispatch error = %v, want the not-found failure", err)
		}
		if called {
			t.Fatal("dispatch started the TUI for an unknown run")
		}
	})
}

func TestReviewReceiptlessLaunchesTUI(t *testing.T) {
	dir, runID := executingFixture(t)
	opts := resolveTUIFlags(t, bareGetenv)
	stdio, _, _ := testStdio()
	called := false
	var gotCfg tui.Config
	deps := forcedDeps(true, func(_ context.Context, cfg tui.Config) error {
		called = true
		gotCfg = cfg
		return nil
	})
	if err := runReviewDispatch(stdio, deps, bareGetenv, dir, runID, "plain", true, opts); err != nil {
		t.Fatalf("receiptless dispatch: %v", err)
	}
	if !called {
		t.Fatal("receiptless dispatch never reached the TUI entry")
	}
	if gotCfg.RunID != runID {
		t.Fatalf("cfg.RunID = %q, want %q", gotCfg.RunID, runID)
	}
	if gotCfg.Events != nil {
		t.Fatal("review cfg.Events is non-nil, want inspect mode")
	}

	t.Run("linear leg keeps the receipt gate", func(t *testing.T) {
		called := false
		deps := forcedDeps(false, func(context.Context, tui.Config) error {
			called = true
			return nil
		})
		err := runReviewDispatch(stdio, deps, bareGetenv, dir, runID, "plain", true, opts)
		if err == nil {
			t.Fatal("piped receiptless review succeeded, want the receipt-gate failure")
		}
		if called {
			t.Fatal("piped receiptless review started the TUI")
		}
	})
}

func TestActionsWiringHoldsOwnerLockForApply(t *testing.T) {
	// The source repo must exist for CheckBranch's git invocation; the run
	// itself stays non-ready so the free-lock leg reaches ErrApplyBlocked.
	dir, runID := driveRunRepo(t, t.TempDir(), supervisor.RunAdmission, supervisor.RunExecuting)
	runDir := filepath.Join(dir, "runs", runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	actions := wireActions(dir)
	release, err := supervisor.AcquireOwner(runDir)
	if err != nil {
		t.Fatalf("AcquireOwner: %v", err)
	}
	_, err = actions.Apply(t.Context(), runID, "branch-x", false, false)
	if !errors.Is(err, supervisor.ErrOwnerHeld) {
		release()
		t.Fatalf("Apply under a held lock = %v, want ErrOwnerHeld", err)
	}
	release()
	_, err = actions.Apply(t.Context(), runID, "branch-x", false, false)
	if !errors.Is(err, supervisor.ErrApplyBlocked) {
		t.Fatalf("Apply on a non-ready run = %v, want ErrApplyBlocked", err)
	}
}

func TestActionsWiringStopReadOnly(t *testing.T) {
	dir, runID := executingFixture(t)
	insertAttempt(t, dir, runID)
	db, err := os.ReadFile(filepath.Join(dir, "mythhelm.db")) // #nosec G304 -- the test's own fixture database
	if err != nil {
		t.Fatal(err)
	}
	before := sha256.Sum256(db)
	// The ownership failure proves Stop ran its read path (identity check)
	// rather than failing to open the database.
	_, err = wireActions(dir).Stop(t.Context(), runID)
	if !errors.Is(err, supervisor.ErrOwnership) {
		t.Fatalf("Stop = %v, want the no-live-worker ownership failure", err)
	}
	after, err := os.ReadFile(filepath.Join(dir, "mythhelm.db")) // #nosec G304 -- the test's own fixture database
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(after) != before {
		t.Fatal("wired Stop modified mythhelm.db, want a read-only call")
	}
}

// TestWatchWaitsForStreamStart pins the review watch's startup gate: on an
// already-terminal run the watch must not stop the stream before its first
// write (cancelling mid-startup would yield a bare trailer with the summary
// and history silently lost), and must stop it promptly once started.
func TestWatchWaitsForStreamStart(t *testing.T) {
	t.Parallel()
	f := newAccessibleFixture(t)
	runID := ids.New("run")
	f.addRun(runID, "completed", strings.Repeat("0", 64))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{})
	go watchRunTerminal(ctx, cancel, started, f.dir, runID)
	select {
	case <-ctx.Done():
		t.Fatal("watch stopped a terminal run before the stream started")
	case <-time.After(350 * time.Millisecond):
	}
	close(started)
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("watch did not stop a started terminal run")
	}
}

// TestReviewResumePastEndReturnsPromptly pins the empty-page case: a resume
// past the last event of a terminal run emits no lines before following,
// so the watch must still start and end the stream promptly with the echo
// trailer — never hang until Ctrl-C.
func TestReviewResumePastEndReturnsPromptly(t *testing.T) {
	t.Parallel()
	f := newAccessibleFixture(t)
	runID := ids.New("run")
	f.addRun(runID, "completed", strings.Repeat("0", 64))
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- runAccessibleReview(Stdio{Out: &out}, f.dir, runID, 999)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runAccessibleReview: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("resume past end hung: watch never started on the empty page")
	}
	if got := out.String(); got != "next-after: 999\n" {
		t.Fatalf("output = %q, want only the echo trailer", got)
	}
}

// TestWaitForRunJournaledSeesJournaledRun pins the live stream's startup
// wait: a journaled run row ends the wait without any cancellation.
func TestWaitForRunJournaledSeesJournaledRun(t *testing.T) {
	t.Parallel()
	f := newAccessibleFixture(t)
	runID := ids.New("run")
	f.addRun(runID, "executing", strings.Repeat("0", 64))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- waitForRunJournaled(ctx, f.dir, runID)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("waitForRunJournaled = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("waitForRunJournaled did not see the journaled run")
	}
}

// TestWaitForRunJournaledEndsOnCancel pins the wait's bound: a run that
// never journals (no database at all) must not hang the live stream —
// ending the context ends the wait, so it cannot outlive the run.
func TestWaitForRunJournaledEndsOnCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			done <- waitForRunJournaled(ctx, t.TempDir(), "run_missing")
		}()
		// The waiter is blocked (not merely unscheduled) before cancel.
		synctest.Wait()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("waitForRunJournaled = %v, want context.Canceled", err)
		}
	})
}

// TestWaitForRunJournaledSurfacesJournalErrors pins the wait's error path:
// a corrupt database is not "not yet" — it returns promptly instead of
// spinning until supervision ends.
func TestWaitForRunJournaledSurfacesJournalErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, journal.DBName), []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A direct call: any return is prompt; a hang fails the suite timeout.
	err := waitForRunJournaled(context.Background(), dir, "run_broken")
	if err == nil {
		t.Fatal("waitForRunJournaled on a corrupt database = nil, want an error")
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, journal.ErrNoDatabase) || errors.Is(err, journal.ErrNotFound) {
		t.Fatalf("waitForRunJournaled = %v, want the journal error verbatim", err)
	}
}
