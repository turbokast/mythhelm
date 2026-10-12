package supervisor_test

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workers"
)

func TestStreamEventSetPinned(t *testing.T) {
	t.Parallel()
	want := []string{
		"admission.decided",
		"apply.completed",
		"apply.intent_recorded",
		"attempt.admission_pinned",
		"attempt.launch_intent_recorded",
		"attempt.launched",
		"attempt.native_error",
		"attempt.native_result",
		"attempt.native_session",
		"attempt.permission_denied",
		"attempt.progress",
		"attempt.protocol_counters",
		"attempt.recovery_decided",
		"attempt.state_changed",
		"attempt.stop_requested",
		"attempt.stopped",
		"candidate.frozen",
		"check.completed",
		"migration.imported",
		"migration.quarantined",
		"receipt.written",
		"recovery.ownership_resolved",
		"run.created",
		"run.extension_granted",
		"run.result",
		"run.state_changed",
		"verification.completed",
		"verification.started",
		"workspace.snapshot_created",
	}
	got := supervisor.StreamEventTypes()
	if !slices.Equal(got, want) {
		t.Fatalf("StreamEventTypes() = %v, want %v", got, want)
	}
	for _, typ := range want {
		if !supervisor.IsStreamEvent(typ) {
			t.Errorf("IsStreamEvent(%q) = false", typ)
		}
	}
	if supervisor.IsStreamEvent("attempt.surprise") {
		t.Error("IsStreamEvent accepted attempt.surprise")
	}
	got[0] = "tampered"
	if again := supervisor.StreamEventTypes(); again[0] != want[0] {
		t.Errorf("StreamEventTypes returned its backing slice: first entry now %q", again[0])
	}
}

// emittedTypes collects the event types a Go source declares or emits
// (design §2.10): the string value of every constant or variable named
// Event*, ev*, *Event or *EventType, and the string literal at the event-type
// position of newEvent (4th argument), a method append (2nd) and applyEvent
// (5th). A type built by an expression is not collected.
func emittedTypes(t *testing.T, filename string, src any) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filename, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	literal := func(e ast.Expr) {
		lit, ok := e.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return
		}
		if s, err := strconv.Unquote(lit.Value); err == nil {
			found = append(found, s)
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.ValueSpec:
			for i, name := range n.Names {
				if i < len(n.Values) && eventTypeName(name.Name) {
					literal(n.Values[i])
				}
			}
		case *ast.CallExpr:
			position := -1
			switch fun := n.Fun.(type) {
			case *ast.Ident:
				switch fun.Name {
				case "newEvent":
					position = 3
				case "applyEvent":
					position = 4
				}
			case *ast.SelectorExpr:
				switch fun.Sel.Name {
				case "newEvent":
					position = 3
				case "append":
					position = 1
				case "applyEvent":
					position = 4
				}
			}
			if position >= 0 && position < len(n.Args) {
				literal(n.Args[position])
			}
		}
		return true
	})
	return found
}

func eventTypeName(name string) bool {
	upperAfter := func(prefix string) bool {
		rest, ok := strings.CutPrefix(name, prefix)
		return ok && rest != "" && unicode.IsUpper(rune(rest[0]))
	}
	return upperAfter("Event") || upperAfter("ev") || strings.HasSuffix(name, "Event") || strings.HasSuffix(name, "EventType")
}

func undocumented(types []string) []string {
	var out []string
	for _, typ := range types {
		if !supervisor.IsStreamEvent(typ) {
			out = append(out, typ)
		}
	}
	return out
}

// streamedTypes decodes every stdout line strictly and returns the event types
// in order, failing when a line is not a documented envelope.
func streamedTypes(t *testing.T, stdout, stderr string) []string {
	t.Helper()
	evs, err := decodeEnvelopes(stdout)
	if err != nil {
		t.Fatalf("%v\nstderr: %s", err, stderr)
	}
	types := make([]string, len(evs))
	for i, ev := range evs {
		types[i] = ev.Type
	}
	return types
}

func TestNativeErrorStreamsDocumented(t *testing.T) {
	f := newFixture(t)
	code, stdout, stderr := f.run(t, f.fakeRun("--format", "jsonl", "--scenario", "allowance-exhausted")...)
	types := streamedTypes(t, stdout, stderr)
	if !slices.Contains(types, "attempt.native_error") {
		t.Fatalf("exit %d: stream %v has no attempt.native_error line; stderr: %s", code, types, stderr)
	}
}

// TestClaudeRateLimitStreamsDocumented is issue #74's case: a claudecode
// rate-limit error is a journaled attempt.native_error, and it streams.
func TestClaudeRateLimitStreamsDocumented(t *testing.T) {
	requireUnixClaude(t)
	installFakeClaude(t, fakeClaudeConfig{Version: "2.1.284", Auth: fakeAuth(t), Exit: 1, Stream: []string{
		claudeInit("none"),
		`{"type":"assistant","error":{"class":"rate_limit"}}`,
		claudeResult("error_retry", true, ""),
	}})
	f := newFixture(t)
	code, stdout, stderr := f.run(t, f.claudeRun("--format", "jsonl")...)
	types := streamedTypes(t, stdout, stderr)
	if !slices.Contains(types, "attempt.native_error") {
		t.Fatalf("exit %d: stream %v has no attempt.native_error line; stderr: %s", code, types, stderr)
	}
}

// TestRecoveryOwnershipResolvedDocumented: recover replays a run's whole
// history, and the recovery evidence it adds (I12, v2 §2) is documented.
func TestRecoveryOwnershipResolvedDocumented(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux orphan scanner acceptance: the escaped descendant is found by /proc")
	}
	f := newFixture(t)
	d, _, wait := seedLaunch(t, f, "escapee")
	wait()
	if code, _, stderr := f.run(t, "recover", d.RunID); code != 6 {
		t.Fatalf("escaped writer recover %d: %s", code, stderr)
	}
	pids, err := workers.OrphanPIDs(d.AttemptID)
	if err != nil || len(pids) != 1 {
		t.Fatalf("orphan scan %v %v", pids, err)
	}
	// The test owns this scripted descendant and verifies its recorded start
	// time before killing it. Production recovery only observes it.
	var identities []workers.ProcessIdentity
	for _, ev := range f.events(t, d.RunID) {
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

	code, stdout, stderr := f.run(t, "recover", "--format", "jsonl", d.RunID)
	if code != 4 {
		t.Fatalf("resolved partial recovery %d: %s", code, stderr)
	}
	types := streamedTypes(t, stdout, stderr)
	if !slices.Contains(types, "recovery.ownership_resolved") {
		t.Fatalf("recover stream %v has no recovery.ownership_resolved line", types)
	}
}

func TestEveryEmittedEventTypeIsDocumented(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for _, dir := range []string{"../supervisor", "../workers", "../control", "../migrate"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			for _, typ := range emittedTypes(t, file, nil) {
				seen[typ] = true
			}
		}
	}
	types := slices.Sorted(maps.Keys(seen))
	t.Logf("scan found %d distinct event types", len(types))
	if len(types) < 25 {
		t.Fatalf("scan found %d distinct event types %v; it is not reaching the emitters", len(types), types)
	}
	if bad := undocumented(types); len(bad) > 0 {
		t.Fatalf("emitted event types missing from supervisor.StreamEventTypes: %v", bad)
	}

	t.Run("an undocumented emitter is flagged", func(t *testing.T) {
		t.Parallel()
		src := `package fixture
const surpriseEvent = "attempt.surprise"
func emit(p *pipeline, ctx context.Context) {
	_ = p.append(ctx, "attempt.other_surprise", nil)
	ev, _ := newEvent(runID, "", "", "run.third_surprise", nil, now)
	_ = applyEvent(ctx, j, p, runID, "apply.fourth_surprise", rec)
}
`
		got := undocumented(emittedTypes(t, "fixture.go", src))
		want := []string{"attempt.surprise", "attempt.other_surprise", "run.third_surprise", "apply.fourth_surprise"}
		if !slices.Equal(got, want) {
			t.Fatalf("flagged %v, want %v", got, want)
		}
	})
	t.Run("a payload string is not collected", func(t *testing.T) {
		t.Parallel()
		src := `package fixture
func emit(p *pipeline, ctx context.Context) {
	_ = p.append(ctx, "receipt.written", map[string]any{"schema_version": 1, "path": "receipt.json"})
	out := append(list, "receipt.json")
}
`
		got := emittedTypes(t, "fixture.go", src)
		if !slices.Equal(got, []string{"receipt.written"}) {
			t.Fatalf("collected %v, want only receipt.written", got)
		}
	})
}
