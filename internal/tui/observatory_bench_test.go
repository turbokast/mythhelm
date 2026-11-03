package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/tui/theme"
)

// benchModel is the NFR-2 renderer fixture: 10,000 diff lines and 200
// retained agent events at 140x40 with the Diff pane focused. It is
// synthetic renderer evidence, never a native-CLI performance comparison.
func benchModel(b *testing.B) *Model {
	b.Helper()
	tokens, err := theme.BuiltIn("dark")
	if err != nil {
		b.Fatal(err)
	}
	m := newModel(Config{RunID: "run_t6", StateDir: b.TempDir(), Caps: fullCaps, Tokens: tokens})
	lines := make([]DiffLine, 10000)
	for i := range lines {
		lines[i] = DiffLine{Kind: DiffContext, Text: fmt.Sprintf("context line %05d", i)}
	}
	m.loadDiff = stubDiff(Diff{Lines: lines})
	snap := richSnapshot()
	for i := range 200 {
		payload := json.RawMessage(fmt.Sprintf(`{"assistant_turns":%d,"tool_uses":{"read":%d}}`, i, i))
		snap.AgentEvents = append(snap.AgentEvents, journal.Event{Type: "attempt.progress", RunID: "run_t6", AttemptID: "att_1", RunSequence: int64(i + 13), Payload: payload})
	}
	m.setSnapshot(b.Context(), snap)
	m.width, m.height = 140, 40
	m.openDetails()
	return m
}

// benchView renders once and enforces bounded key work by row count: the
// acceptance control, independent of the machine-dependent ns/op.
func benchView(b *testing.B, m *Model) {
	b.Helper()
	b.ReportAllocs()
	for b.Loop() {
		if rows := strings.Count(m.View(), "\n") + 1; rows != 40 {
			b.Fatalf("view rendered %d rows, want 40", rows)
		}
	}
}

func BenchmarkObservatoryView(b *testing.B) {
	benchView(b, benchModel(b))
}

func BenchmarkObservatoryOverlay(b *testing.B) {
	m := benchModel(b)
	m.helpOpen = true
	benchView(b, m)
}
