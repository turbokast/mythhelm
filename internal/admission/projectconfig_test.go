package admission_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/admission"
)

func TestLoadProjectConfigUsesCommittedBytes(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...) // #nosec G204 -- fixed test argv only
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "--quiet", "--initial-branch=main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	raw := []byte("schema_version = 1\n[[checks]]\nname = \"test\"\nargv = [\"true\"]\ntimeout = \"1s\"\n")
	path := filepath.Join(dir, "mythhelm.toml")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "mythhelm.toml")
	git("commit", "--quiet", "-m", "config")
	// Git checkout on Windows may produce CRLF. A candidate may also edit the
	// working file; neither changes the admitted committed bytes.
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(raw), "\n", "\r\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	_, digest, err := admission.LoadProjectConfig(dir)
	sum := sha256.Sum256(raw)
	if err != nil || digest != hex.EncodeToString(sum[:]) {
		t.Fatalf("committed digest = %s, want %x; err = %v", digest, sum, err)
	}
}

func TestLoadProjectConfigStrictAndDigest(t *testing.T) {
	dir := t.TempDir()
	raw := []byte("schema_version = 1\n[[checks]]\nname = \"test\"\nargv = [\"go\", \"test\", \"./...\"]\ntimeout = \"1m\"\n")
	if err := os.WriteFile(filepath.Join(dir, "mythhelm.toml"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, digest, err := admission.LoadProjectConfig(dir)
	if err != nil || len(cfg.Checks) != 1 || len(digest) != 64 {
		t.Fatalf("config = %+v, digest = %s, err = %v", cfg, digest, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mythhelm.toml"), append(raw, []byte("unknown = true\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err = admission.LoadProjectConfig(dir)
	if !errors.Is(err, admission.ErrProjectConfig) || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("unknown key err = %v", err)
	}
}

func TestLoadProjectConfigValidatesAllowedTools(t *testing.T) {
	t.Parallel()
	good := "schema_version = 1\n[adapters.claudecode]\nallowed_tools = [\"Bash(go test *)\"]\n[[checks]]\nname = \"test\"\nargv = [\"true\"]\ntimeout = \"1s\"\n"
	cfg, _, err := admission.ParseProjectConfig([]byte(good))
	if err != nil || len(cfg.Adapters.ClaudeCode.AllowedTools) != 1 {
		t.Fatalf("good rules = %+v, err = %v", cfg, err)
	}
	bad := []string{
		"schema_version = 1\n[adapters.claudecode]\nallowed_tools = [\"\"]\n",
		"schema_version = 1\n[adapters.claudecode]\nallowed_tools = [\"ok\", \"bad\x00rule\"]\n",
		"schema_version = 1\n[adapters.claudecode]\nallowed_tools = [\"" + strings.Repeat("x", 1025) + "\"]\n",
		"schema_version = 1\n[adapters.claudecode]\nallowed_tools = [\"--dangerously-skip-permissions\"]\n",
		"schema_version = 1\n[adapters.claudecode]\nallowed_tools = [\"-p\"]\n",
	}
	for _, raw := range bad {
		if _, _, err := admission.ParseProjectConfig([]byte(raw)); !errors.Is(err, admission.ErrProjectConfig) {
			t.Fatalf("bad rules err = %v, want ErrProjectConfig", err)
		}
	}
}

func TestLoadProjectConfigRejectsSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.toml")
	if err := os.WriteFile(outside, []byte("schema_version = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "mythhelm.toml")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, _, err := admission.LoadProjectConfig(dir)
	if !errors.Is(err, admission.ErrProjectConfig) {
		t.Fatalf("outside symlink err = %v", err)
	}
}
