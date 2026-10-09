package security_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/adapters/claudecode"
	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/security"
)

// TestCredentialEnvDenied pins the environment channel of AC-4.2 (I03):
// variables on the credential denylist never reach the child environment
// without the explicit opt-in, and adding one to the child fails. The
// opt-in covers exactly CLAUDE_CODE_OAUTH_TOKEN (AC-4.7); every other
// route stays denied even beside it.
func TestCredentialEnvDenied(t *testing.T) {
	t.Parallel()
	denied := []string{"ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN"}

	t.Run("denylisted parent entries never reach the child", func(t *testing.T) {
		t.Parallel()
		parent := []string{"PATH=/usr/bin", "HOME=/home/u"}
		for _, name := range denied {
			parent = append(parent, name+"=planted-secret")
		}
		child, err := security.BuildEnv(parent, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(child, " ")
		for _, name := range denied {
			if strings.Contains(joined, name) {
				t.Errorf("child env carries %s: %v", name, child)
			}
		}
		if strings.Contains(joined, "planted-secret") {
			t.Errorf("child env leaks the credential value: %v", child)
		}
		if !slices.Contains(child, "PATH=/usr/bin") || !slices.Contains(child, "HOME=/home/u") {
			t.Errorf("allowlisted entries dropped from the child: %v", child)
		}
	})

	t.Run("adding a denylisted variable to the child fails", func(t *testing.T) {
		t.Parallel()
		for _, name := range denied {
			if _, err := security.BuildEnv([]string{"PATH=/usr/bin"}, []string{name}, nil); err == nil {
				t.Errorf("passthrough %s: err = nil, want ErrDeniedPassthrough", name)
			} else if !strings.Contains(err.Error(), security.ErrDeniedPassthrough.Error()) {
				t.Errorf("passthrough %s: err = %v, want ErrDeniedPassthrough", name, err)
			}
			if _, err := security.BuildEnv([]string{"PATH=/usr/bin"}, nil, map[string]string{name: "planted-secret"}); err == nil {
				t.Errorf("set %s: err = nil, want ErrDeniedPassthrough", name)
			} else if !strings.Contains(err.Error(), security.ErrDeniedPassthrough.Error()) {
				t.Errorf("set %s: err = %v, want ErrDeniedPassthrough", name, err)
			}
		}
	})

	t.Run("admission blocks or strips without the opt-in", func(t *testing.T) {
		t.Parallel()
		for _, name := range denied {
			if _, _, err := admission.ResolveCredentialEnv([]string{"PATH=/fixture", name + "=planted-secret"}, false, false, nil); err == nil {
				t.Errorf("%s without --strip-credential-env: err = nil, want credential_route_override", name)
			}
			child, overrides, err := admission.ResolveCredentialEnv([]string{"HOME=/fixture", name + "=planted-secret"}, true, false, nil)
			if err != nil {
				t.Errorf("%s stripped: %v", name, err)
				continue
			}
			if strings.Contains(strings.Join(child, " "), name) {
				t.Errorf("%s stripped: child still carries it: %v", name, child)
			}
			if len(overrides) != 1 || overrides[0].Name != name || overrides[0].Kind != "env_remove" {
				t.Errorf("%s stripped: overrides = %+v, want one env_remove", name, overrides)
			}
		}
	})

	t.Run("explicit opt-in preserves only the oauth token", func(t *testing.T) {
		t.Parallel()
		child, _, err := admission.ResolveCredentialEnv(
			[]string{"HOME=/fixture", "CLAUDE_CODE_OAUTH_TOKEN=planted-secret"}, true, true, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(child, "CLAUDE_CODE_OAUTH_TOKEN=planted-secret") {
			t.Fatalf("opted-in child lacks the token: %v", child)
		}
		// The opt-in is per-variable: the API key route stays denied beside it.
		if _, _, err := admission.ResolveCredentialEnv(
			[]string{"HOME=/fixture", "ANTHROPIC_API_KEY=planted-secret", "CLAUDE_CODE_OAUTH_TOKEN=planted-secret"},
			false, true, nil); err == nil {
			t.Error("API key beside the opt-in without strip: err = nil, want credential_route_override")
		}
		child, _, err = admission.ResolveCredentialEnv(
			[]string{"HOME=/fixture", "ANTHROPIC_API_KEY=planted-secret", "CLAUDE_CODE_OAUTH_TOKEN=planted-secret"},
			true, true, nil)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.Join(child, " "), "ANTHROPIC_API_KEY") {
			t.Errorf("opted-in stripped child carries the API key: %v", child)
		}
	})

	t.Run("prepare preserves an admitted token but never invents one", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("claudecode Prepare refuses on Windows (no process-group ownership)")
		}
		exe := filepath.Join(t.TempDir(), "claude")
		if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		prepare := func(t *testing.T, base []string) []string {
			t.Helper()
			lp, err := claudecode.New().Prepare(t.Context(), adapter.PrepareInput{
				Workdir: t.TempDir(), AttemptID: "att_testcred", Env: base,
				Prompt: strings.NewReader("do it"),
				Probe:  adapter.Probe{Executable: exe, SHA256: fileSHA256(t, exe)},
			})
			if err != nil {
				t.Fatal(err)
			}
			return lp.Spec.Env
		}
		if env := prepare(t, []string{"HOME=/fixture"}); slices.ContainsFunc(env, func(kv string) bool {
			return strings.HasPrefix(kv, "CLAUDE_CODE_OAUTH_TOKEN=")
		}) {
			t.Errorf("Prepare invented an oauth entry: %v", env)
		}
		env := prepare(t, []string{"HOME=/fixture", "CLAUDE_CODE_OAUTH_TOKEN=planted-secret"})
		if !slices.Contains(env, "CLAUDE_CODE_OAUTH_TOKEN=planted-secret") {
			t.Errorf("Prepare dropped the admitted oauth entry: %v", env)
		}
		env = prepare(t, []string{"HOME=/fixture", "ANTHROPIC_API_KEY=planted-secret"})
		if slices.ContainsFunc(env, func(kv string) bool { return strings.HasPrefix(kv, "ANTHROPIC_API_KEY=") }) {
			t.Errorf("Prepare let the API key reach the child: %v", env)
		}
	})
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	// #nosec G304 -- test fixture the test just wrote
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
