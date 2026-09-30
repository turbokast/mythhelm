package claudecode

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/turbokast/mythhelm/internal/adapter"
)

const maxSettingsBytes = 4 << 20
const maxSettingsSources = 256

// Manifest retains configuration digests and execution metadata, never native
// commands, endpoint URLs, env values, account data or credential values.
type Manifest struct {
	Digests       map[string]string `json:"digests"`
	Digest        string            `json:"digest"`
	Hooks         int               `json:"hooks"`
	MCPServers    []string          `json:"mcp_servers"`
	RequiresTrust bool              `json:"requires_trust"`
}

// claudeJSON deliberately has no session or account fields (AC-4.6).
type claudeJSON struct {
	MCPServers map[string]json.RawMessage `json:"mcpServers"`
	Projects   map[string]struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	} `json:"projects"`
}

type nativeSettings struct {
	APIKeyHelper     json.RawMessage            `json:"apiKeyHelper"`
	ForceLoginMethod json.RawMessage            `json:"forceLoginMethod"`
	Env              map[string]json.RawMessage `json:"env"`
	Hooks            map[string][]struct {
		Hooks []json.RawMessage `json:"hooks"`
	} `json:"hooks"`
	MCPServers        map[string]json.RawMessage `json:"mcpServers"`
	ManagedMCPServers map[string]json.RawMessage `json:"managedMcpServers"`
	EnabledPlugins    map[string]bool            `json:"enabledPlugins"`
	PolicyHelper      json.RawMessage            `json:"policyHelper"`
}

// InventorySettings inventories the design §6.2 file sources for this host.
// It never reads .credentials.json or any native secret storage.
func InventorySettings(home, workspace string) (Manifest, error) {
	return InventorySettingsForEnv(home, workspace, os.Environ())
}

// InventorySettingsForEnv binds the config root to the admitted child env.
// Callers must inventory the exact workspace the native will use.
func InventorySettingsForEnv(home, workspace string, env []string) (Manifest, error) {
	configDir := filepath.Join(home, ".claude")
	custom := false
	for _, kv := range env {
		if value, ok := strings.CutPrefix(kv, "CLAUDE_CONFIG_DIR="); ok && value != "" {
			configDir = value
			custom = true
		}
	}
	claudeJSONPath := filepath.Join(home, ".claude.json")
	if custom {
		claudeJSONPath = filepath.Join(configDir, ".claude.json")
	}
	managed := ""
	switch runtime.GOOS {
	case "linux":
		managed = "/etc/claude-code"
	case "darwin":
		managed = "/Library/Application Support/ClaudeCode"
	case "windows":
		managed = `C:\Program Files\ClaudeCode`
	}
	return inventorySettings(home, workspace, configDir, claudeJSONPath, managed)
}

func inventorySettings(home, workspace, configDir, claudeJSONPath, managed string) (Manifest, error) {
	manifest := Manifest{Digests: map[string]string{}, MCPServers: []string{}}
	if !filepath.IsAbs(home) || !filepath.IsAbs(workspace) || !filepath.IsAbs(configDir) {
		return Manifest{}, settingsError("config_root")
	}
	type source struct{ name, path, kind string }
	sources := []source{{"user", filepath.Join(configDir, "settings.json"), "settings"}, {"project", filepath.Join(workspace, ".claude", "settings.json"), "settings"}, {"project_local", filepath.Join(workspace, ".claude", "settings.local.json"), "settings"}, {"project_mcp", filepath.Join(workspace, ".mcp.json"), "mcp"}, {"user_mcp", claudeJSONPath, "claude_json"}}
	if managed != "" {
		sources = append(sources, source{"managed", filepath.Join(managed, "managed-settings.json"), "settings"}, source{"managed_mcp", filepath.Join(managed, "managed-mcp.json"), "mcp"})
		entries, err := managedFragments(filepath.Join(managed, "managed-settings.d"))
		if err != nil {
			return Manifest{}, settingsError("managed_fragments")
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			sources = append(sources, source{fmt.Sprintf("managed_fragment_%d", len(sources)), filepath.Join(managed, "managed-settings.d", entry.Name()), "settings"})
		}
	}
	for _, src := range sources {
		raw, present, err := readSettings(src.path)
		if err != nil {
			return Manifest{}, settingsError(src.name)
		}
		if !present {
			continue
		}
		if !validObject(raw) {
			return Manifest{}, settingsError(src.name)
		}
		switch src.kind {
		case "claude_json":
			var cfg claudeJSON
			if json.Unmarshal(raw, &cfg) != nil {
				return Manifest{}, settingsError(src.name)
			}
			if err = manifest.addMCP(src.name, cfg.MCPServers); err != nil {
				return Manifest{}, err
			}
			// Native per-project keys may use the canonical workspace path.
			paths := []string{workspace}
			if real, e := filepath.EvalSymlinks(workspace); e == nil && real != workspace {
				paths = append(paths, real)
			}
			for _, path := range paths {
				if err = manifest.addMCP(src.name+":project", cfg.Projects[path].MCPServers); err != nil {
					return Manifest{}, err
				}
			}
			// Hash only the selected MCP data. Account/session changes do not affect
			// native executable configuration or invalidate unrelated trust grants.
			selected := struct {
				User     map[string]json.RawMessage
				Projects map[string]map[string]json.RawMessage
			}{cfg.MCPServers, make(map[string]map[string]json.RawMessage)}
			for i, path := range paths {
				scope := "lexical"
				if i > 0 {
					scope = "canonical"
				}
				selected.Projects[scope] = cfg.Projects[path].MCPServers
			}
			raw, err = json.Marshal(selected)
			if err != nil {
				return Manifest{}, settingsError(src.name)
			}
		default:
			manifest.RequiresTrust = true // Every settings/MCP source is a trust item (AC-2.5).
			var cfg nativeSettings
			if json.Unmarshal(raw, &cfg) != nil {
				return Manifest{}, settingsError(src.name)
			}
			if nonNull(cfg.APIKeyHelper) {
				return Manifest{}, settingsBlock("native_settings_credential_override", src.name+":apiKeyHelper")
			}
			if len(cfg.ForceLoginMethod) > 0 {
				var method string
				if json.Unmarshal(cfg.ForceLoginMethod, &method) != nil || method != "claudeai" {
					return Manifest{}, settingsBlock("native_settings_credential_override", src.name+":forceLoginMethod")
				}
			}
			keys := make([]string, 0, len(cfg.Env))
			for name := range cfg.Env {
				keys = append(keys, name)
			}
			slices.Sort(keys)
			for _, name := range keys {
				if credentialName(name) {
					return Manifest{}, settingsBlock("native_settings_credential_override", src.name+":"+name)
				}
			}
			// policyHelper can fetch executable managed policy unavailable to this
			// file-only inventory. Do not execute it or pretend it was inventoried.
			if nonNull(cfg.PolicyHelper) {
				return Manifest{}, settingsBlock("native_config_source_unavailable", src.name+":policyHelper")
			}
			for _, matchers := range cfg.Hooks {
				for _, matcher := range matchers {
					manifest.Hooks += len(matcher.Hooks)
				}
			}
			if manifest.Hooks > maxSettingsSources {
				return Manifest{}, settingsError(src.name)
			}
			if err = manifest.addMCP(src.name, cfg.MCPServers); err != nil {
				return Manifest{}, err
			}
			if err = manifest.addMCP(src.name, cfg.ManagedMCPServers); err != nil {
				return Manifest{}, err
			}
			for _, enabled := range cfg.EnabledPlugins {
				if enabled {
					manifest.RequiresTrust = true
				}
			}
		}
		sum := sha256.Sum256(raw)
		manifest.Digests[src.name] = hex.EncodeToString(sum[:])
	}
	manifest.RequiresTrust = manifest.RequiresTrust || manifest.Hooks > 0 || len(manifest.MCPServers) > 0
	slices.Sort(manifest.MCPServers)
	encoded, err := json.Marshal(manifest.Digests)
	if err != nil {
		return Manifest{}, settingsError("manifest")
	}
	sum := sha256.Sum256(encoded)
	manifest.Digest = hex.EncodeToString(sum[:])
	return manifest, nil
}

func (m *Manifest) addMCP(source string, servers map[string]json.RawMessage) error {
	if len(servers)+len(m.MCPServers) > maxSettingsSources {
		return settingsError(source)
	}
	for name, definition := range servers {
		if name == "" || len(name) > 128 || strings.ContainsFunc(name, func(r rune) bool { return !unicode.IsPrint(r) }) || !validObject(definition) {
			return settingsError(source)
		}
		m.MCPServers = append(m.MCPServers, source+":"+name)
	}
	return nil
}
func nonNull(raw json.RawMessage) bool {
	return len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}
func settingsBlock(code, field string) error {
	return &adapter.BlockedError{Code: code, Field: field, Action: "fix it in native settings; MYTHHELM does not edit or strip native configuration"}
}
func settingsError(source string) error { return settingsBlock("native_config_unreadable", source) }
func readSettings(path string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxSettingsBytes {
		return nil, true, settingsError("file")
	}
	f, err := os.Open(path) //nolint:gosec // Known native config source; never a credentials path.
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = f.Close() }()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		return nil, true, settingsError("file")
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxSettingsBytes+1))
	if err != nil || len(raw) > maxSettingsBytes {
		return nil, true, settingsError("file")
	}
	return raw, true, nil
}

// validObject checks syntax, depth and duplicate object keys without decoding
// string values. In particular, account/session values in .claude.json never
// enter a decoded generic map or an account-bearing Go struct.
func validObject(raw []byte) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || len(raw) > maxSettingsBytes || raw[0] != '{' || !utf8.Valid(raw) || !json.Valid(raw) {
		return false
	}
	type frame struct {
		object, key bool
		keys        map[string]bool
	}
	stack := make([]frame, 0, 8)
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '{', '[':
			if len(stack) >= 64 {
				return false
			}
			object := raw[i] == '{'
			stack = append(stack, frame{object: object, key: object, keys: map[string]bool{}})
		case '}', ']':
			stack = stack[:len(stack)-1]
		case ',':
			if len(stack) > 0 && stack[len(stack)-1].object {
				stack[len(stack)-1].key = true
			}
		case '"':
			start := i
			i++
			for i < len(raw) {
				if raw[i] == '\\' {
					i += 2
					continue
				}
				if raw[i] == '"' {
					break
				}
				i++
			}
			if len(stack) > 0 && stack[len(stack)-1].object && stack[len(stack)-1].key {
				var key string
				if json.Unmarshal(raw[start:i+1], &key) != nil {
					return false
				}
				f := &stack[len(stack)-1]
				if f.keys[key] {
					return false
				}
				f.keys[key] = true
				f.key = false
			}
		}
	}
	return true
}

func managedFragments(path string) ([]os.DirEntry, error) {
	f, err := os.Open(path) //nolint:gosec // Fixed managed config directory, never credentials.
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	entries, err := f.ReadDir(maxSettingsSources + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > maxSettingsSources {
		return nil, errors.New("too many managed sources")
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, nil
}
