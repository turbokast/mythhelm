package supervisor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtensionGrantsOnRecover(t *testing.T) {
	t.Parallel()
	w := blockedRun(t, "envelope_replans_exhausted")
	if err := os.MkdirAll(filepath.Join(w.j.StateDir(), "runs", w.runID), 0o700); err != nil {
		t.Fatal(err)
	}
	out, err := RecoverWithHooks(t.Context(), w.j, w.runID, Hooks{Extension: &ExtensionGrant{Kind: "replans", RaisedTo: 4, DecidedBy: "operator"}})
	if err != nil {
		t.Fatalf("RecoverWithHooks: %v", err)
	}
	if out.State != RunBlocked {
		t.Fatalf("state = %s, want blocked: resuming the run is the launch gate's job", out.State)
	}
	if got := len(w.extensions(t)); got != 1 {
		t.Fatalf("run.extension_granted events = %d, want 1", got)
	}
	if env, err := w.j.RunEnvelope(t.Context(), w.runID); err != nil || env.Replans != 4 {
		t.Fatalf("envelope = %+v, %v; want replans 4", env, err)
	}
	if ok, err := CheckExtension(t.Context(), w.j, w.runID, "replans"); err != nil || !ok {
		t.Fatalf("CheckExtension = %v, %v; want true", ok, err)
	}
}

func TestRecoverWithoutExtensionGrantsNothing(t *testing.T) {
	t.Parallel()
	w := blockedRun(t, "envelope_replans_exhausted")
	if err := os.MkdirAll(filepath.Join(w.j.StateDir(), "runs", w.runID), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Recover(t.Context(), w.j, w.runID); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if got := len(w.extensions(t)); got != 0 {
		t.Fatalf("plain recover journaled %d extension events, want 0", got)
	}
}

func TestRecoverRefusesExtensionForWrongKind(t *testing.T) {
	t.Parallel()
	w := blockedRun(t, "envelope_replans_exhausted")
	if err := os.MkdirAll(filepath.Join(w.j.StateDir(), "runs", w.runID), 0o700); err != nil {
		t.Fatal(err)
	}
	// A kind the run is not blocked for errors before anything is journaled.
	_, err := RecoverWithHooks(t.Context(), w.j, w.runID, Hooks{Extension: &ExtensionGrant{Kind: "repairs", RaisedTo: 9, DecidedBy: "operator"}})
	if err == nil {
		t.Fatal("extension for the wrong kind succeeded")
	}
	if got := len(w.extensions(t)); got != 0 {
		t.Fatalf("refused extension journaled %d events", got)
	}
}
