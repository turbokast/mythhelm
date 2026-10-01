package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// hashTree hashes every file under root: relative path, mode and content.
// A read-only command leaves the digest unchanged.
func hashTree(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		_, _ = h.Write([]byte(rel + "\x00" + info.Mode().String() + "\x00"))
		if d.IsDir() {
			return nil
		}
		// #nosec G304 G122 -- hashing the test's own fixture tree, which holds no symlinks
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, _ = h.Write(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func seedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{"hooks":{"PreToolUse":[{"hooks":[{}]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"mcpServers":{"demo":{"command":"demo"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestDoctorIsReadOnly(t *testing.T) {
	home := seedHome(t)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	state := t.TempDir()
	if err := os.WriteFile(filepath.Join(state, "keep.db"), []byte("state-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYTHHELM_HOME", state)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	before := map[string]string{"home": hashTree(t, home), "state": hashTree(t, state), "repo": hashTree(t, cwd)}
	code, _, stderr := runMain("doctor")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	for name, root := range map[string]string{"home": home, "state": state, "repo": cwd} {
		if got := hashTree(t, root); got != before[name] {
			t.Errorf("%s tree changed under a read-only doctor", name)
		}
	}
	missing := filepath.Join(t.TempDir(), "no-such-state")
	t.Setenv("MYTHHELM_HOME", missing)
	if code, _, stderr := runMain("doctor"); code != 0 {
		t.Fatalf("absent-state exit %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Errorf("doctor created the absent state dir: %v", err)
	}
}

func TestDoctorNeverPrintsCredentialValues(t *testing.T) {
	// Names and values are assembled at run time so the secret scans
	// never see a credential literal in this file.
	secrets := map[string]string{
		"ANTHROPIC_" + "API_KEY":       "planted-" + "anthropic-secret",
		"CLAUDE_CODE_" + "OAUTH_TOKEN": "planted-" + "oauth-secret",
		"AWS_" + "SECRET_ACCESS_KEY":   "planted-" + "aws-secret",
	}
	for name, value := range secrets {
		t.Setenv(name, value)
	}
	for _, format := range []string{"plain", "jsonl"} {
		args := []string{"doctor"}
		if format == "jsonl" {
			args = append(args, "--format", "jsonl")
		}
		code, stdout, stderr := runMain(args...)
		if code != 0 {
			t.Fatalf("%s exit %d, stderr %q", format, code, stderr)
		}
		for name, value := range secrets {
			if strings.Contains(stdout, value) || strings.Contains(stderr, value) {
				t.Errorf("%s leaked the value of %s", format, name)
			}
			if !strings.Contains(stdout, name) {
				t.Errorf("%s names no %s route", format, name)
			}
		}
	}
}

func TestDoctorDoesNotRunAuthStatus(t *testing.T) {
	// A logging fake on PATH records every invocation; doctor may ask
	// for --version but must never ask for auth status.
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "invocations")
	t.Setenv("FAKECLAUDE_LOG", log)
	if runtime.GOOS == "windows" {
		script := "@echo off\r\necho %*>> \"%FAKECLAUDE_LOG%\"\r\nif \"%1\"==\"--version\" echo 9.9.9 (Claude Code)\r\n"
		if err := os.WriteFile(filepath.Join(bin, "claude.cmd"), []byte(script), 0o600); err != nil {
			t.Fatal(err)
		}
	} else {
		script := "#!/bin/sh\necho \"$@\" >> \"$FAKECLAUDE_LOG\"\nif [ \"$1\" = \"--version\" ]; then echo \"9.9.9 (Claude Code)\"; fi\nexit 0\n"
		if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o700); err != nil { // #nosec G306 -- executable test fixture under t.TempDir
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	code, stdout, stderr := runMain("doctor")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "9.9.9") {
		t.Fatalf("doctor did not report the fake version:\n%s", stdout)
	}
	if !strings.Contains(stdout, "run admission to query") {
		t.Fatalf("doctor does not defer auth to admission:\n%s", stdout)
	}
	raw, err := os.ReadFile(log) // #nosec G304 -- the test's own invocation log
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "--version") {
		t.Errorf("fake saw no version query: %q", raw)
	}
	if strings.Contains(string(raw), "auth") {
		t.Errorf("doctor ran an auth query: %q", raw)
	}
}

func TestDoctorUnreadableStateDirIsNotAbsent(t *testing.T) {
	// A regular file as the parent makes Lstat fail with ENOTDIR, not
	// ENOENT: doctor must report unreadable, never absent.
	blocker := filepath.Join(t.TempDir(), "file-not-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYTHHELM_HOME", filepath.Join(blocker, "state"))
	code, stdout, stderr := runMain("doctor")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "unreadable") || strings.Contains(stdout, "absent (not created)") {
		t.Fatalf("unreadable state reported as:\n%s", stdout)
	}
}

func TestRunBoundedCapsOutput(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	out, err := runBoundedEnv(exe, []string{"__bigout"}, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != maxProbeOutput {
		t.Fatalf("captured %d bytes, want exactly the %d-byte cap", len(out), maxProbeOutput)
	}
}

func TestDoctorClaudeMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	code, stdout, stderr := runMain("doctor")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "claude: not found") {
		t.Fatalf("doctor did not report the missing native:\n%s", stdout)
	}
}

func TestDoctorJsonl(t *testing.T) {
	code, stdout, stderr := runMain("doctor", "--format", "jsonl")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	var rep struct {
		Type     string         `json:"type"`
		Mythhelm map[string]any `json:"mythhelm"`
		Git      map[string]any `json:"git"`
		Claude   map[string]any `json:"claude"`
		CredEnv  []string       `json:"credential_env_names"`
		Settings map[string]any `json:"native_settings"`
		Sandbox  map[string]any `json:"sandbox"`
		StateDir map[string]any `json:"state_dir"`
		Terminal map[string]any `json:"terminal"`
	}
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatalf("doctor jsonl: %v\n%s", err, stdout)
	}
	if rep.Type != "doctor" || rep.Mythhelm == nil || rep.Git == nil || rep.Claude == nil ||
		rep.CredEnv == nil || rep.Settings == nil || rep.Sandbox == nil || rep.StateDir == nil || rep.Terminal == nil {
		t.Fatalf("doctor jsonl misses a section: %+v", rep)
	}
	if rep.Claude["auth"] != "run admission to query" {
		t.Fatalf("doctor jsonl auth = %v", rep.Claude["auth"])
	}
}
