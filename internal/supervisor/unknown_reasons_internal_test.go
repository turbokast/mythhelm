package supervisor

import (
	"slices"
	"testing"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
)

// The external tests drive the unexported reason walk through these names.
var AttachUnknownReasons = attachUnknownReasons

// UnknownReasons is the registered reason table.
func UnknownReasons() map[string]string { return unknownReasons() }

func TestAttachUnknownReasonsReturnsUnregisteredLeaves(t *testing.T) {
	t.Parallel()
	r := Receipt{
		"a":       map[string]any{"b": "unknown", "c": []any{map[string]any{"d": "unknown"}, "unknown"}, "n": 0},
		"e":       []string{"unknown", "known"},
		"billing": map[string]any{"tokens": map[string]any{"by_model": map[string]any{"native-chosen-key": "unknown"}}},
	}
	reasons := map[string]string{"a.b": "b is missing", "a.c[].d": "d is missing", "a.n": "orphan: n is not unknown"}
	missing := attachUnknownReasons(r, reasons)
	if want := []string{"a.c[]", "e[]"}; !slices.Equal(missing, want) {
		t.Errorf("missing = %v, want the unregistered unknown leaves %v", missing, want)
	}
	want := map[string]string{"a.b": "b is missing", "a.c[].d": "d is missing"}
	got, ok := r["unknown_reasons"].(map[string]string)
	if !ok || len(got) != len(want) || got["a.b"] != want["a.b"] || got["a.c[].d"] != want["a.c[].d"] {
		t.Errorf("unknown_reasons = %#v, want exactly the reasons of the unknown leaves %v", r["unknown_reasons"], want)
	}
}

func TestBuildReceiptRendersFallbackReasonForMissingPath(t *testing.T) {
	t.Parallel()
	j, err := journal.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	runID := ids.New("run")
	prod := NewProducer(ids.New("sup"), 1)
	run := journal.RunRow{RunID: runID, AdapterID: "builtin/fake", SourceRepo: "repo", TaskSHA256: "t",
		BillingPosture: "local-scripted", ExecutionProfile: "trusted-host"}
	if err := CreateRun(t.Context(), j, run, prod); err != nil {
		t.Fatal(err)
	}
	registered := unknownReasons()
	delete(registered, "candidate.commit")
	r, err := buildReceipt(t.Context(), j, runID, registered)
	if err != nil {
		t.Fatalf("a path with no registered reason failed the receipt: %v", err)
	}
	reasons, ok := r["unknown_reasons"].(map[string]string)
	if !ok {
		t.Fatalf("unknown_reasons = %#v, want a path-to-reason map", r["unknown_reasons"])
	}
	if got := reasons["candidate.commit"]; got != "no reason recorded" {
		t.Errorf("reason of the unregistered path = %q, want the fallback %q", got, "no reason recorded")
	}
	if got := reasons["candidate.tree_id"]; got == "" || got == "no reason recorded" {
		t.Errorf("reason of a registered path = %q, want its registered reason", got)
	}
}
