package claudecode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
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
			fakeClaudeLaunch()
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakeClaudeLaunch is the launch-mode fake native: it asserts the argv, the
// stdin prompt and the child environment, then replays a fixture. Any
// assertion failure exits 9; the replayed stream exits FAKE_EXIT (default 0).
func fakeClaudeLaunch() {
	fail := func(format string, args ...any) {
		fmt.Fprintf(os.Stderr, "fakeclaude: "+format+"\n", args...)
		os.Exit(9)
	}
	want := []string{"-p", "--output-format", "stream-json", "--verbose", "--input-format", "text",
		"--permission-mode", "acceptEdits", "--permission-prompts", "none"}
	argv := os.Args[1:]
	if len(argv) < len(want) {
		fail("argv too short: %q", argv)
	}
	for i, w := range want {
		if argv[i] != w {
			fail("argv[%d] = %q, want %q (argv %q)", i, argv[i], w, argv)
		}
	}
	rest := argv[len(want):]
	rules := strings.Split(os.Getenv("FAKE_ALLOWED_TOOLS"), "\x1f")
	if os.Getenv("FAKE_ALLOWED_TOOLS") == "" {
		rules = nil
	}
	if len(rules) == 0 {
		if len(rest) != 0 {
			fail("unexpected argv tail %q without allowed tools", rest)
		}
	} else {
		if len(rest) != len(rules)+1 || rest[0] != "--allowedTools" {
			fail("argv tail %q, want --allowedTools followed by %q", rest, rules)
		}
		for i, r := range rules {
			if rest[i+1] != r {
				fail("argv tail %q, want --allowedTools followed by %q", rest, rules)
			}
		}
	}
	if marker := os.Getenv("FAKE_PROMPT_MARKER"); marker != "" {
		for _, arg := range argv {
			if strings.Contains(arg, marker) {
				fail("prompt marker %q appears in argv", marker)
			}
		}
	}
	prompt, err := io.ReadAll(io.LimitReader(os.Stdin, maxProbeOutput+1))
	if err != nil || len(prompt) > maxProbeOutput {
		fail("reading stdin: %v", err)
	}
	if path := os.Getenv("FAKE_PROMPT"); path != "" {
		wantPrompt, err := os.ReadFile(path) // #nosec G304 G703 -- Fake helper reads only the path its isolated test fixture names.
		if err != nil || string(prompt) != string(wantPrompt) {
			fail("stdin prompt %q does not match %s", prompt, path)
		}
	}
	for _, name := range []string{"ANTHROPIC_API_KEY", "AWS_SECRET_ACCESS_KEY"} {
		for _, kv := range os.Environ() {
			if strings.HasPrefix(kv, name+"=") {
				fail("forbidden child variable %s is present", name)
			}
		}
	}
	for _, kv := range []string{"DISABLE_AUTOUPDATER=1", "CLAUDE_CODE_STARTUP_FAILURE_RESULTS=1"} {
		if !slices.Contains(os.Environ(), kv) {
			fail("child variable %s is missing", kv)
		}
	}
	if attempt, ok := os.LookupEnv("MYTHHELM_ATTEMPT_ID"); !ok || attempt == "" || attempt != os.Getenv("FAKE_ATTEMPT") {
		fail("MYTHHELM_ATTEMPT_ID = %q, want %q", attempt, os.Getenv("FAKE_ATTEMPT"))
	}
	if passthrough := os.Getenv("FAKE_PASSTHROUGH"); passthrough != "" {
		if !slices.Contains(os.Environ(), passthrough) {
			fail("passthrough %s is missing from the child", passthrough)
		}
	}
	if stream := os.Getenv("FAKE_STREAM"); stream != "" {
		raw, err := os.ReadFile(stream) // #nosec G304 G703 -- Fake helper reads only the path its isolated test fixture names.
		if err != nil {
			fail("reading stream %s: %v", stream, err)
		}
		for line := range strings.Lines(string(raw)) {
			if line == "\n" || strings.HasPrefix(line, "#") {
				continue
			}
			fmt.Print(line)
		}
	}
	if code := os.Getenv("FAKE_EXIT"); code != "" && code != "0" {
		n, _ := strconv.Atoi(code)
		os.Exit(n)
	}
	os.Exit(0)
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
	p, err := probePlatform(context.Background(), fakeProbeInput(t, "2.1.285"), "linux")
	if err != nil || !strings.Contains(p.Compatibility, "recorded on 2.1.285") {
		t.Fatalf("2.1.285 must name its Task 20 recording, got %+v, %v", p, err)
	}
}

func TestWindowsClaudecodeExit7(t *testing.T) {
	t.Parallel()
	_, err := probePlatform(context.Background(), adapter.ProbeInput{ExecutableOverride: "must-not-execute"}, "windows")
	if !errors.Is(err, ErrCapability) {
		t.Fatalf("Windows must refuse before launching, got %v", err)
	}
	caps := New().Capabilities(adapter.Probe{OS: "windows", Version: "2.1.284"})
	if caps.Platform.ProcessTreeOwnership != adapter.Unsupported || caps.Billing.IncludedOnlySupported != adapter.Unsupported {
		t.Fatalf("unsupported facts = %+v", caps)
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

func TestInventoryAdmittedProjectUsesBlobs(t *testing.T) {
	t.Parallel()
	home, workspace := t.TempDir(), t.TempDir()
	// A dirty file on disk must not shadow the admitted blob.
	writeConfig(t, workspace, filepath.Join(".claude", "settings.json"), `{"apiKeyHelper":"disk-helper"}`)
	blobs := map[string][]byte{"project": []byte(`{"hooks":{"PreToolUse":[{"hooks":[{}]}]}}`)}
	m, err := InventoryAdmittedProject(home, workspace, blobs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !m.RequiresTrust || m.Hooks != 1 {
		t.Fatalf("blob manifest = %+v, want the admitted hooks inventoried", m)
	}
	bad := map[string][]byte{"project": []byte(`{"apiKeyHelper":"blob-helper"}`)}
	_, err = InventoryAdmittedProject(home, workspace, bad, nil)
	var block *adapter.BlockedError
	if !errors.As(err, &block) || block.Code != "native_settings_credential_override" || strings.Contains(err.Error(), "blob-helper") {
		t.Fatalf("blob credential must block names only, got %v", err)
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
	for f := range typ.Fields() {
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
			_, err := inventorySettings(home, workspace, filepath.Join(home, ".claude"), filepath.Join(home, ".claude.json"), "", nil)
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
		m, e := inventorySettings(home, alias, filepath.Join(home, ".claude"), source, "", nil)
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
	m, err := inventorySettings(home, workspace, filepath.Join(home, ".claude"), filepath.Join(home, ".claude.json"), managed, nil)
	if err != nil || m.Hooks != 1 || len(m.MCPServers) != 1 || len(m.Digests) != 2 || !m.RequiresTrust {
		t.Fatalf("managed executable sources omitted: %+v, %v", m, err)
	}
	writeConfig(t, managed, "managed-settings.json", `{"env":{"ANTHROPIC_API_KEY":"planted-managed-value"}}`)
	_, err = inventorySettings(home, workspace, filepath.Join(home, ".claude"), filepath.Join(home, ".claude.json"), managed, nil)
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
	_, err := inventorySettings(home, workspace, config, filepath.Join(home, ".claude.json"), "", nil)
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
			_, err := inventorySettings(home, workspace, filepath.Join(home, ".claude"), filepath.Join(home, ".claude.json"), "", nil)
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
		m, err := inventorySettings(home, t.TempDir(), filepath.Join(home, ".claude"), filepath.Join(home, ".claude.json"), "", nil)
		if err != nil {
			t.Fatal(err)
		}
		manifests[i] = m
	}
	if manifests[0].Digest != manifests[1].Digest {
		t.Fatal("unchanged user MCP needs different trust for each generated workspace path")
	}
}
