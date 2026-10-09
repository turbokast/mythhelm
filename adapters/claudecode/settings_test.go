package claudecode_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/turbokast/mythhelm/adapters/claudecode"
)

func inventoryBlobs(t *testing.T, blobs map[string][]byte) claudecode.Manifest {
	t.Helper()
	home, workspace := t.TempDir(), t.TempDir()
	env := []string{"CLAUDE_CONFIG_DIR=" + filepath.Join(home, "isolated-config")}
	m, err := claudecode.InventoryAdmittedProject(home, workspace, blobs, env)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestManifestCollectsPluginNames(t *testing.T) {
	t.Parallel()
	m := inventoryBlobs(t, map[string][]byte{
		"project":       []byte(`{"enabledPlugins":{"zeta@synthetic":true,"alpha@synthetic":true,"off@synthetic":false}}`),
		"project_local": []byte(`{"enabledPlugins":{"mid@synthetic":true,"alpha@synthetic":true}}`),
	})
	if want := []string{"alpha@synthetic", "mid@synthetic", "zeta@synthetic"}; !slices.Equal(m.EnabledPlugins, want) {
		t.Fatalf("EnabledPlugins = %v, want %v (sorted, true entries only, deduplicated)", m.EnabledPlugins, want)
	}
	if !m.RequiresTrust {
		t.Fatal("enabled plugins must keep RequiresTrust")
	}
	off := inventoryBlobs(t, map[string][]byte{"project": []byte(`{"enabledPlugins":{"off@synthetic":false}}`)})
	if len(off.EnabledPlugins) != 0 {
		t.Fatalf("disabled plugin collected: %v", off.EnabledPlugins)
	}
}

func TestManifestRequiresTrustTriggersUnchanged(t *testing.T) {
	t.Parallel()
	if m := inventoryBlobs(t, map[string][]byte{}); m.RequiresTrust || m.EnabledPlugins == nil || len(m.EnabledPlugins) != 0 {
		t.Fatalf("bare manifest = %+v, want no trust and an empty non-nil plugin list", m)
	}
	for name, blob := range map[string]string{
		"hooks": `{"hooks":{"PreToolUse":[{"hooks":[{"type":"command"}]}]}}`,
		"mcp":   `{"mcpServers":{"synthetic":{}}}`,
	} {
		m := inventoryBlobs(t, map[string][]byte{"project": []byte(blob)})
		if !m.RequiresTrust {
			t.Errorf("%s: RequiresTrust = false", name)
		}
	}
}

func TestAdmittedPathsCoverManifest(t *testing.T) {
	home, workspace := t.TempDir(), t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	writeFixture := func(path string, raw []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeFixture(filepath.Join(home, ".claude", "settings.json"), []byte(`{"enabledPlugins":{"alpha@synthetic":true}}`))
	writeFixture(filepath.Join(workspace, ".claude", "settings.json"), []byte(`{"hooks":{"PreToolUse":[{"hooks":[{"type":"command"}]}]}}`))
	writeFixture(filepath.Join(workspace, ".claude", "settings.local.json"), []byte(`{"env":{"EDITOR":"vi"}}`))
	writeFixture(filepath.Join(workspace, ".mcp.json"), []byte(`{"mcpServers":{"proj":{}}}`))
	claudeRaw, err := json.Marshal(map[string]any{
		"mcpServers": map[string]any{"up": map[string]any{}},
		"projects":   map[string]any{workspace: map[string]any{"mcpServers": map[string]any{"pj": map[string]any{}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(filepath.Join(home, ".claude.json"), claudeRaw)

	manifest, err := claudecode.InventorySettings(home, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Digests) < 5 {
		t.Fatalf("fixture yielded %d digests, want at least 5 (user, project, project_local, project_mcp, user_mcp)", len(manifest.Digests))
	}
	paths := claudecode.AdmittedConfigPaths(home, workspace)
	for name, digest := range manifest.Digests {
		path, ok := paths[name]
		if !ok {
			t.Errorf("digest source %q has no admitted path", name)
			continue
		}
		if !filepath.IsAbs(path) {
			t.Errorf("admitted path for %q is %q, want absolute", name, path)
		}
		raw, err := os.ReadFile(path) // #nosec G304 -- Test reads the fixture file the mapping returned.
		if err != nil {
			t.Errorf("admitted path for %q unreadable: %v", name, err)
			continue
		}
		if name == "user_mcp" {
			// The user_mcp digest covers the selected MCP subset only
			// (account/session data excluded by design), so the whole
			// file never hashes to it; pin the path instead.
			if want := filepath.Join(home, ".claude.json"); path != want {
				t.Errorf("admitted path for user_mcp = %q, want %q", path, want)
			}
			continue
		}
		sum := sha256.Sum256(raw)
		if got := hex.EncodeToString(sum[:]); got != digest {
			t.Errorf("admitted path for %q hashes to %s, want digest %s", name, got, digest)
		}
	}
}
