package claudecode_test

import (
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
