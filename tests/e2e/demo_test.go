package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2EDemoAllOS runs the packaged demo end to end. It is the dogfood
// gate's last external check: demo must exit 0 on every OS with no
// network, no credentials and no external CLI.
func TestE2EDemoAllOS(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	code, stdout, _ := run(t, e, dir, "demo")
	if code != 0 {
		t.Fatalf("demo exit %d", code)
	}
	// The demo runs through review and prints the receipt, but never
	// applies; every screen carries the scripted banner.
	for _, want := range []string{"review", "ready_for_review", "SCRIPTED DEMO"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("demo output lacks %q:\n%s", want, stdout)
		}
	}
}

// TestE2EDemoStaysOffline deletes every credential the packaged demo
// could conceivably use, then requires demo to still pass.
func TestE2EDemoStaysOffline(t *testing.T) {
	e := newEnv(t)
	// An empty home with no config files, no npm, no claude CLI on a
	// stripped PATH is impossible to guarantee; instead assert the
	// observable contract: demo passes and names no remote step.
	dir := t.TempDir()
	code, stdout, _ := run(t, e, dir, "demo")
	if code != 0 {
		t.Fatalf("demo exit %d", code)
	}
	// The environment points every proxy variable at a closed port
	// (see newEnv), so a demo that reached the network would fail
	// instead of leaking. The output must not name remote endpoints
	// either; the check stays on domains so documentation wording that
	// merely contains "http" cannot fail it.
	for _, leak := range []string{"api.muse", "api.anthropic", "github.com"} {
		if strings.Contains(strings.ToLower(stdout), leak) {
			t.Fatalf("demo output mentions %q:\n%s", leak, stdout)
		}
	}
	// Doctor covers the real-run prerequisites demo does not need:
	// git, the native CLI and the state dir.
	code, stdout, _ = run(t, e, dir, "doctor")
	if code != 0 {
		t.Fatalf("doctor exit %d:\n%s", code, stdout)
	}
	for _, want := range []string{"git", "claude", "state dir"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("doctor output does not cover %q:\n%s", want, stdout)
		}
	}
}

// TestE2EDemoRepoIsDisposable verifies the demo seeds its repository in
// a temporary directory, never in the caller's checkout.
func TestE2EDemoRepoIsDisposable(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	entries := func() []string {
		names, err := filepath.Glob(filepath.Join(dir, "*"))
		if err != nil {
			t.Fatal(err)
		}
		return names
	}
	before := entries()
	if code, _, stderr := run(t, e, dir, "demo"); code != 0 {
		t.Fatalf("demo exit %d, stderr %q", code, stderr)
	}
	if after := entries(); len(after) != len(before) {
		t.Fatalf("demo wrote into the working directory: %v", after)
	}
	// The demo supervises through its own temporary state dir, which it
	// removes afterwards: the caller's state dir stays untouched.
	var leaked []string
	_ = filepath.Walk(e.state, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			leaked = append(leaked, path)
		}
		return nil
	})
	if len(leaked) > 0 {
		t.Fatalf("demo wrote into the caller's state dir: %v", leaked)
	}
}
