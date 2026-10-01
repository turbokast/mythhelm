package claudecode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/adapter"
)

func TestMain(m *testing.M) {
	if os.Getenv("GO_WANT_FAKECLAUDE") == "1" {
		switch strings.Join(os.Args[1:], " ") {
		case "--version":
			if link := os.Getenv("FAKE_SWAP_LINK"); link != "" {
				if err := os.Remove(link); err != nil { // #nosec G703 -- Fake helper changes only the symlink supplied by its isolated test fixture.
					os.Exit(9)
				}
				if err := os.Symlink(os.Getenv("FAKE_NEW_BINARY"), link); err != nil {
					os.Exit(9)
				}
			}
			fmt.Printf("%s (Claude Code)\n", os.Getenv("FAKE_VERSION"))
		case "auth status":
			fmt.Print(os.Getenv("FAKE_AUTH"))
		default:
			os.Exit(8)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fakeProbeInput(t *testing.T, version string) adapter.ProbeInput {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return adapter.ProbeInput{ExecutableOverride: exe, Workdir: t.TempDir(), Env: []string{"GO_WANT_FAKECLAUDE=1", "GORACE=atexit_sleep_ms=0", "FAKE_VERSION=" + version}}
}

func TestUntestedMinorVersionExit7(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"2.2.0", "3.0.0", "2.1.283"} {
		t.Run(version, func(t *testing.T) {
			in := fakeProbeInput(t, version)
			_, err := probePlatform(context.Background(), in, "linux")
			if !errors.Is(err, ErrCapability) {
				t.Fatalf("untested version must have capability refusal (exit 7), got %v", err)
			}
			in.AllowUntestedNativeVersion = true
			p, err := probePlatform(context.Background(), in, "linux")
			if err != nil || p.Version != version || !strings.Contains(p.Compatibility, "experimental") {
				t.Fatalf("explicit opt-in = %+v, %v", p, err)
			}
		})
	}
	for _, version := range []string{"2.1.284", "2.1.285"} {
		p, err := probePlatform(context.Background(), fakeProbeInput(t, version), "linux")
		if err != nil || p.Version != version || !strings.Contains(p.Compatibility, "fixture-tested on 2.1.284") {
			t.Fatalf("supported minor = %+v, %v", p, err)
		}
	}
}

func TestWindowsClaudecodeExit7(t *testing.T) {
	t.Parallel()
	_, err := probePlatform(context.Background(), adapter.ProbeInput{ExecutableOverride: "must-not-execute"}, "windows")
	if !errors.Is(err, ErrCapability) {
		t.Fatalf("Windows must refuse before launching, got %v", err)
	}
	cap := New().Capabilities(adapter.Probe{OS: "windows", Version: "2.1.284"})
	if cap.Platform.ProcessTreeOwnership != adapter.Unsupported || cap.Billing.IncludedOnlySupported != adapter.Unsupported {
		t.Fatalf("unsupported facts = %+v", cap)
	}
}

func TestResolvedVersionedBinaryLaunched(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native Claude Code is unsupported on Windows; symlink fixture requires Unix")
	}
	t.Parallel()
	in := fakeProbeInput(t, "2.1.284")
	versioned := filepath.Join(t.TempDir(), "2.1.284")
	raw, err := os.ReadFile(in.ExecutableOverride)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(versioned, raw, 0o700); err != nil { // #nosec G306 G703 -- Executable fixture is created only under t.TempDir and must be executable.
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "claude")
	if err = os.Symlink(versioned, link); err != nil {
		t.Fatal(err)
	}
	newBinary := filepath.Join(t.TempDir(), "missing-new-version")
	in.ExecutableOverride = link
	in.Env = append(in.Env, "FAKE_SWAP_LINK="+link, "FAKE_NEW_BINARY="+newBinary)
	p, err := probePlatform(context.Background(), in, "linux")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.EvalSymlinks(versioned)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if p.Executable != expected || p.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("resolved identity = %+v", p)
	}
	in.Env = append(in.Env, "FAKE_AUTH="+fmt.Sprintf(`{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty","subscriptionType":"max","orgId":"test-identity","configDirectory":%q}`, in.Workdir))
	evidence, err := AuthStatus(context.Background(), p, in.Env, in.Workdir)
	if err != nil || evidence.AuthMethod != "claude.ai" || evidence.IdentityRef == "" {
		t.Fatalf("auth must use pinned old binary after symlink swap: %+v, %v", evidence, err)
	}
}

func writeConfig(t *testing.T, root, relative, text string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsEnvCredentialBlocksNoStripOption(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "") // InventorySettings resolves the documented user config root.
	for _, name := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY", "ANTHROPIC_BASE_URL", "CLAUDE_CODE_OAUTH_TOKEN"} {
		t.Run(name, func(t *testing.T) {
			home, workspace := t.TempDir(), t.TempDir()
			writeConfig(t, home, filepath.Join(".claude", "settings.json"), fmt.Sprintf(`{"env":{%q:"planted-value"}}`, name))
			_, err := InventorySettings(home, workspace)
			var block *adapter.BlockedError
			if !errors.As(err, &block) || block.Code != "native_settings_credential_override" || !strings.Contains(err.Error(), name) || strings.Contains(err.Error(), "planted-value") {
				t.Fatalf("settings credential must block names only, got %v", err)
			}
		})
	}
}

func TestApiKeyHelperBlocks(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	home, workspace := t.TempDir(), t.TempDir()
	writeConfig(t, workspace, filepath.Join(".claude", "settings.local.json"), `{"apiKeyHelper":"never-run-this-helper"}`)
	_, err := InventorySettings(home, workspace)
	var block *adapter.BlockedError
	if !errors.As(err, &block) || block.Field != "project_local:apiKeyHelper" || strings.Contains(err.Error(), "never-run-this-helper") {
		t.Fatalf("apiKeyHelper must be refused without evaluation, got %v", err)
	}
}

func TestClaudeJSONAccountDataNeverDecoded(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	typ := reflect.TypeFor[claudeJSON]()
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Name != "MCPServers" && f.Name != "Projects" {
			t.Fatalf("account field in narrow decode struct: %s", f.Name)
		}
	}
	home, workspace := t.TempDir(), t.TempDir()
	writeConfig(t, home, ".claude.json", `{"oauthAccount":{"accessToken":"planted-native-session","emailAddress":"planted-account"},"mcpServers":{"user-fixture":{"command":"never-execute"}}}`)
	manifest, err := InventorySettings(home, workspace)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.RequiresTrust || len(manifest.MCPServers) != 1 {
		t.Fatalf("fixture MCP definition must be inventoried: %+v", manifest)
	}
	for _, value := range []string{"planted-native-session", "planted-account", "never-execute"} {
		if strings.Contains(string(encoded), value) {
			t.Fatalf("account/native definition value persisted in manifest: %s", value)
		}
	}
}

func TestNativeJSONRejectsAmbiguousAndMalformedSources(t *testing.T) {
	t.Parallel()
	for i, raw := range []string{
		`{"env":{"ANTHROPIC_API_KEY":"planted"},"env":{}}`,
		`{"hooks":{},"hooks":{"SessionStart":[]}}`,
		`{"forceLoginMethod":"console","forceLoginMethod":"claudeai"}`,
		`{"apiKeyHelper":"fixture-helper","APIKeyHelper":null}`,
		`{"env":{"ANTHROPIC_BASE_URL":"fixture-route"},"Env":null}`,
		`{"forceLoginMethod":"console","ForceLoginMethod":"claudeai"}`,
		`{"policyHelper":"fixture-helper","PolicyHelper":null}`,
		"{\"apiKeyHelper\":\"fixture-helper\",\"apiKeyHelper\":null}",
		`{"mcpServers":{"a":{}},"mcpſervers":{"b":{}}}`,
		`{"env":[]} `,
		`[]`,
		`{"broken":`,
		string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}),
		`{"nested":` + strings.Repeat("[", 65) + `0` + strings.Repeat("]", 65) + `}`,
	} {
		t.Run(fmt.Sprintf("case_%02d_len_%d", i, len(raw)), func(t *testing.T) {
			home, workspace := t.TempDir(), t.TempDir()
			writeConfig(t, home, filepath.Join(".claude", "settings.json"), raw)
			_, err := inventorySettings(home, workspace, filepath.Join(home, ".claude"), filepath.Join(home, ".claude.json"), "")
			var block *adapter.BlockedError
			if !errors.As(err, &block) || block.Code != "native_config_unreadable" {
				t.Fatalf("malformed/ambiguous config allowed: %v", err)
			}
		})
	}
}

func TestFoldKeyMatchesEqualFold(t *testing.T) {
	t.Parallel()
	folded := [][2]string{
		{"apiKeyHelper", "APIKeyHelper"},
		{"env", "ENV"},
		{"apiKeyHelper", "apiKeyHelper"}, // Kelvin sign folds with K
		{"mcpServers", "mcpſervers"},     // long s folds with s/S
		{"subscriptionType", "ſubscriptionType"},
	}
	for _, pair := range folded {
		if !strings.EqualFold(pair[0], pair[1]) {
			t.Fatalf("fixture pair is not fold-equal: %q %q", pair[0], pair[1])
		}
		if foldKey(pair[0]) != foldKey(pair[1]) {
			t.Errorf("foldKey splits fold-equal keys: %q %q", pair[0], pair[1])
		}
	}
	distinct := [][2]string{
		{"env", "env "},
		{"apiKeyHelper", "apiKeyHelpers"},
		{"mcpServers", "mcServers"},
		{"env", "énv"},
	}
	for _, pair := range distinct {
		if strings.EqualFold(pair[0], pair[1]) {
			t.Fatalf("fixture pair unexpectedly fold-equal: %q %q", pair[0], pair[1])
		}
		if foldKey(pair[0]) == foldKey(pair[1]) {
			t.Errorf("foldKey merges distinct keys: %q %q", pair[0], pair[1])
		}
	}
}

func TestNativeTrustDigestIncludesEveryWorkspaceAlias(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinked native workspace fixture requires Unix; Claude is unsupported on Windows")
	}
	t.Parallel()
	home := t.TempDir()
	workspace := t.TempDir()
	canonical, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "workspace-alias")
	if err = os.Symlink(canonical, alias); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(home, ".claude.json")
	write := func(lexicalCommand, canonicalCommand string) {
		t.Helper()
		writeConfig(t, home, ".claude.json", fmt.Sprintf(`{"projects":{%q:{"mcpServers":{"lexical":{"command":%q}}},%q:{"mcpServers":{"canonical":{"command":%q}}}}}`, alias, lexicalCommand, canonical, canonicalCommand))
	}
	inventory := func() Manifest {
		t.Helper()
		m, e := inventorySettings(home, alias, filepath.Join(home, ".claude"), source, "")
		if e != nil {
			t.Fatal(e)
		}
		if len(m.MCPServers) != 2 {
			t.Fatalf("both workspace aliases must be inventoried: %+v", m)
		}
		return m
	}
	write("lexical-original", "canonical-original")
	initial := inventory()
	write("lexical-changed", "canonical-original")
	lexicalChanged := inventory()
	if initial.Digest == lexicalChanged.Digest {
		t.Fatal("lexical MCP definition changed without invalidating trust digest")
	}
	write("lexical-original", "canonical-changed")
	canonicalChanged := inventory()
	if initial.Digest == canonicalChanged.Digest {
		t.Fatal("canonical MCP definition changed without invalidating trust digest")
	}
}

func TestManagedFragmentsAndMCPAreInventoried(t *testing.T) {
	t.Parallel()
	home, workspace, managed := t.TempDir(), t.TempDir(), t.TempDir()
	writeConfig(t, managed, filepath.Join("managed-settings.d", "10-hooks.json"), `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"managed-fixture-hook"}]}]}}`)
	writeConfig(t, managed, "managed-mcp.json", `{"mcpServers":{"managed-fixture":{"command":"never-execute"}}}`)
	m, err := inventorySettings(home, workspace, filepath.Join(home, ".claude"), filepath.Join(home, ".claude.json"), managed)
	if err != nil || m.Hooks != 1 || len(m.MCPServers) != 1 || len(m.Digests) != 2 || !m.RequiresTrust {
		t.Fatalf("managed executable sources omitted: %+v, %v", m, err)
	}
	writeConfig(t, managed, "managed-settings.json", `{"env":{"ANTHROPIC_API_KEY":"planted-managed-value"}}`)
	_, err = inventorySettings(home, workspace, filepath.Join(home, ".claude"), filepath.Join(home, ".claude.json"), managed)
	var block *adapter.BlockedError
	if !errors.As(err, &block) || block.Code != "native_settings_credential_override" {
		t.Fatalf("managed credential bypassed inventory: %v", err)
	}
}

func TestSettingsNeverFollowCredentialSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture requires Unix")
	}
	t.Parallel()
	home, workspace := t.TempDir(), t.TempDir()
	config := filepath.Join(home, ".claude")
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	// A credential-shaped env source would give a different reason if read.
	// The source symlink must be refused before any target content is decoded.
	target := filepath.Join(config, ".credentials.json")
	writeConfig(t, home, filepath.Join(".claude", ".credentials.json"), `{"env":{"ANTHROPIC_API_KEY":"planted-native-credential"}}`)
	if err := os.Symlink(target, filepath.Join(config, "settings.json")); err != nil {
		t.Fatal(err)
	}
	_, err := inventorySettings(home, workspace, config, filepath.Join(home, ".claude.json"), "")
	var block *adapter.BlockedError
	if !errors.As(err, &block) || block.Code != "native_config_unreadable" {
		t.Fatalf("native credential symlink allowed: %v", err)
	}
}

func TestForceLoginMethodRejectsEverySetNonSubscriptionValue(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, raw string
		allowed   bool
	}{{"absent", `{}`, true}, {"claudeai", `{"forceLoginMethod":"claudeai"}`, true}, {"console", `{"forceLoginMethod":"console"}`, false}, {"empty", `{"forceLoginMethod":""}`, false}, {"null", `{"forceLoginMethod":null}`, false}, {"wrong type", `{"forceLoginMethod":false}`, false}} {
		t.Run(tc.name, func(t *testing.T) {
			home, workspace := t.TempDir(), t.TempDir()
			writeConfig(t, home, filepath.Join(".claude", "settings.json"), tc.raw)
			_, err := inventorySettings(home, workspace, filepath.Join(home, ".claude"), filepath.Join(home, ".claude.json"), "")
			if tc.allowed {
				if err != nil {
					t.Fatalf("native subscription login setting refused: %v", err)
				}
				return
			}
			var block *adapter.BlockedError
			if !errors.As(err, &block) || block.Code != "native_settings_credential_override" {
				t.Fatalf("set non-subscription login method allowed: %v", err)
			}
		})
	}
}

func TestUserMCPDigestIgnoresUnconfiguredWorkspacePaths(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	writeConfig(t, home, ".claude.json", `{"mcpServers":{"fixture":{"command":"same-command"}}}`)
	manifests := make([]Manifest, 2)
	for i := range manifests {
		m, err := inventorySettings(home, t.TempDir(), filepath.Join(home, ".claude"), filepath.Join(home, ".claude.json"), "")
		if err != nil {
			t.Fatal(err)
		}
		manifests[i] = m
	}
	if manifests[0].Digest != manifests[1].Digest {
		t.Fatal("unchanged user MCP needs different trust for each generated workspace path")
	}
}
