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
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/billing"
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

func envelopesConfig(table string) []byte {
	return []byte("schema_version = 1\n" + table + "\n[[checks]]\nname = \"test\"\nargv = [\"true\"]\ntimeout = \"1s\"\n")
}

func TestEnvelopesStrictDecode(t *testing.T) {
	t.Parallel()
	cfg, _, err := admission.ParseProjectConfig(envelopesConfig("[envelopes]\nexecution = \"45m\"\nrepairs = 1\nreplans = 0\ntransport_retries = 7"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := cfg.Envelopes.ToCeilings()
	want := &billing.Ceilings{Execution: 45 * time.Minute, Repairs: 1, Replans: 0, TransportRetries: 7}
	if err != nil || got == nil || *got != *want {
		t.Fatalf("ToCeilings = %+v, %v; want %+v (a zero count is a set value)", got, err, want)
	}

	// A partial table leaves the other fields unset (negative), never zero.
	cfg, _, err = admission.ParseProjectConfig(envelopesConfig("[envelopes]\nrepairs = 4"))
	if err != nil {
		t.Fatal(err)
	}
	got, err = cfg.Envelopes.ToCeilings()
	if err != nil || got == nil || *got != (billing.Ceilings{Execution: -1, Repairs: 4, Replans: -1, TransportRetries: -1}) {
		t.Fatalf("partial ToCeilings = %+v, %v", got, err)
	}

	// No table at all is no file layer.
	cfg, _, err = admission.ParseProjectConfig(envelopesConfig(""))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := cfg.Envelopes.ToCeilings(); err != nil || got != nil {
		t.Fatalf("absent table ToCeilings = %+v, %v; want nil layer", got, err)
	}

	for name, table := range map[string]string{
		"unknown key":          "[envelopes]\nrepair = 3",
		"unknown nested table": "[envelopes.extra]\nx = 1",
		"negative count":       "[envelopes]\nrepairs = -1",
		"negative replans":     "[envelopes]\nreplans = -2",
		"negative transport":   "[envelopes]\ntransport_retries = -1",
		"invalid duration":     "[envelopes]\nexecution = \"soon\"",
		"zero duration":        "[envelopes]\nexecution = \"0s\"",
		"negative duration":    "[envelopes]\nexecution = \"-5m\"",
		"integer duration":     "[envelopes]\nexecution = 30",
		"fractional count":     "[envelopes]\nrepairs = 1.5",
		"string count":         "[envelopes]\nrepairs = \"3\"",
	} {
		if _, _, err := admission.ParseProjectConfig(envelopesConfig(table)); !errors.Is(err, admission.ErrProjectConfig) {
			t.Errorf("%s: err = %v, want ErrProjectConfig", name, err)
		}
	}
}
