package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/admission"
)

func TestProjectionTaskArtifactBoundaries(t *testing.T) {
	t.Parallel()
	if maxProjectionTaskBytes != admission.MaxTaskBytes {
		t.Fatal("projection artefact bound differs from admission")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "runs", "run_artefact")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "task.md")
	task := []byte("synthetic task artefact\n")
	if err := os.WriteFile(path, task, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(task)
	want := "sha256:" + hex.EncodeToString(sum[:])
	if got := projectionTaskDigest(t.Context(), root, "run_artefact"); got != want {
		t.Fatalf("digest %q != %q", got, want)
	}
	for _, id := range []string{"../run_artefact", "run_artefact/../run_artefact", "", "run-missing", "run_missing"} {
		if got := projectionTaskDigest(t.Context(), root, id); got != "" {
			t.Fatalf("invalid/missing artefact %q produced digest", id)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got := projectionTaskDigest(ctx, root, "run_artefact"); got != "" {
		t.Fatal("cancelled artefact read produced digest")
	}
	limitTask := []byte(strings.Repeat("x", admission.MaxTaskBytes))
	if err := os.WriteFile(path, limitTask, 0600); err != nil {
		t.Fatal(err)
	}
	limitSum := sha256.Sum256(limitTask)
	if got := projectionTaskDigest(t.Context(), root, "run_artefact"); got != "sha256:"+hex.EncodeToString(limitSum[:]) {
		t.Fatal("exact-limit artefact was rejected or misread")
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", admission.MaxTaskBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if got := projectionTaskDigest(t.Context(), root, "run_artefact"); got != "" {
		t.Fatal("oversized artefact accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if got := projectionTaskDigest(t.Context(), root, "run_artefact"); got != "" {
		t.Fatal("directory accepted as task artefact")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "synthetic-other-task")
	if err := os.WriteFile(target, task, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Logf("symlink setup unavailable on this platform: %v", err)
		return
	}
	if got := projectionTaskDigest(t.Context(), root, "run_artefact"); got != "" {
		t.Fatal("symlink accepted as task artefact")
	}
}

func TestProjectionIdentifierSanitization(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"/private/prompt", "task\x1b]0;secret", "task\u202esecret", strings.Repeat("x", 129), ""} {
		if safeProjectionID(value) != nil {
			t.Fatalf("unsafe ID %q selected", value)
		}
	}
	if got := safeProjectionID("task_ABC123"); got == nil || *got != "task_ABC123" {
		t.Fatal("canonical opaque identity lost")
	}
}
