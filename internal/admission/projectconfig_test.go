package admission_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/admission"
)

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
