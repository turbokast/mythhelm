package codex

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
	"slices"
	"strings"

	"github.com/turbokast/mythhelm/internal/adapter"
)

const maxConfigBytes = 4 << 20

// ErrNonRegularSource marks an enumerated source that is not a regular file:
// the native would read something this inventory cannot establish.
var ErrNonRegularSource = errors.New("native config source is not a regular committed file")

// Manifest retains configuration digests and execution metadata, never
// native commands, env values or credential values.
type Manifest struct {
	Digests       map[string]string `json:"digests"`
	Digest        string            `json:"digest"`
	Hooks         int               `json:"hooks"`
	MCPServers    []string          `json:"mcp_servers"`
	RequiresTrust bool              `json:"requires_trust"`
}

// UnresolvedSources lists the startup sources no inventory covers. The
// vendor's actual load order is unverified (OQ6), so each one blocks strict
// admission until new inventory code or a scoping rationale removes it.
var UnresolvedSources = []string{"managed-policy", "os-managed-preferences"}

// GapSources returns the unresolved sources in input order: every one applies
// on every platform because its absence cannot be proved.
func GapSources(unresolved []string) []string {
	return append([]string{}, unresolved...)
}

// ProjectSources maps the project source names to their repo-relative paths.
// The paths are placeholders confirmed by OQ6.
var ProjectSources = map[string]string{
	"project_instructions": "AGENTS.md",
	"project_config":       ".codex/config.json",
}

const userConfigName = "user_config"

// credentialKeys are top-level config keys that supply or pin credentials;
// MYTHHELM neither reads nor edits them, so their presence blocks.
var credentialKeys = []string{"api_key", "openai_api_key", "preferred_auth_method"}

type codexConfig struct {
	Hooks      map[string][]json.RawMessage `json:"hooks"`
	MCPServers map[string]json.RawMessage   `json:"mcp_servers"`
}

type configSource struct{ name, path string }

func configHome(home string, env []string) string {
	dir := filepath.Join(home, ".codex")
	for _, kv := range env {
		if value, ok := strings.CutPrefix(kv, "CODEX_HOME="); ok && value != "" {
			dir = value
		}
	}
	return dir
}

// sources is the enumerated inventory list in digest order. Both the
// inventory and the admitted-path mapping build on it.
func sources(home, workdir string, env []string) []configSource {
	list := []configSource{{userConfigName, filepath.Join(configHome(home, env), "config.json")}}
	for _, name := range []string{"project_instructions", "project_config"} {
		list = append(list, configSource{name, filepath.Join(workdir, filepath.FromSlash(ProjectSources[name]))})
	}
	return list
}

// InventorySettingsForEnv inventories the enumerated sources on disk. It
// never opens a file outside the list, credential stores included.
func InventorySettingsForEnv(home, workdir string, env []string) (Manifest, error) {
	return inventorySettings(home, workdir, env, nil, readSource)
}

// InventoryAdmittedProject inventories a run whose workspace clone does not
// exist yet: the admitted revision's committed blobs, keyed by ProjectSources
// name, stand in for the project sources, and an absent key means the file is
// not in the revision. Only the user source is read from disk.
func InventoryAdmittedProject(home, workdir string, blobs map[string][]byte, env []string) (Manifest, error) {
	if blobs == nil {
		blobs = map[string][]byte{}
	}
	return inventorySettings(home, workdir, env, blobs, readSource)
}

// AdmittedConfigPathsForEnv maps each inventory source name to its absolute
// path so the worker can re-hash the mutable files behind an admitted
// manifest. Non-absolute roots yield an empty map.
func AdmittedConfigPathsForEnv(home, workdir string, env []string) map[string]string {
	if !filepath.IsAbs(home) || !filepath.IsAbs(workdir) || !filepath.IsAbs(configHome(home, env)) {
		return map[string]string{}
	}
	paths := map[string]string{}
	for _, src := range sources(home, workdir, env) {
		paths[src.name] = src.path
	}
	return paths
}

type sourceReader func(path string) ([]byte, bool, error)

func inventorySettings(home, workdir string, env []string, blobs map[string][]byte, read sourceReader) (Manifest, error) {
	if !filepath.IsAbs(home) || !filepath.IsAbs(workdir) || !filepath.IsAbs(configHome(home, env)) {
		return Manifest{}, fmt.Errorf("%w: native_config_unreadable: config roots must be absolute", ErrCapability)
	}
	manifest := Manifest{Digests: map[string]string{}, MCPServers: []string{}}
	for _, src := range sources(home, workdir, env) {
		var raw []byte
		var present bool
		if _, project := ProjectSources[src.name]; project && blobs != nil {
			raw, present = blobs[src.name]
			if len(raw) > maxConfigBytes {
				return Manifest{}, configBlock("native_config_unreadable", src.name)
			}
		} else {
			var err error
			if raw, present, err = read(src.path); err != nil {
				return Manifest{}, err
			}
		}
		if !present {
			continue
		}
		if src.name != "project_instructions" {
			if err := manifest.addConfig(src.name, raw); err != nil {
				return Manifest{}, err
			}
		}
		sum := sha256.Sum256(raw)
		manifest.Digests[src.name] = hex.EncodeToString(sum[:])
	}
	manifest.RequiresTrust = manifest.Hooks > 0 || len(manifest.MCPServers) > 0
	slices.Sort(manifest.MCPServers)
	encoded, err := json.Marshal(manifest.Digests)
	if err != nil {
		return Manifest{}, configBlock("native_config_unreadable", "manifest")
	}
	sum := sha256.Sum256(encoded)
	manifest.Digest = hex.EncodeToString(sum[:])
	return manifest, nil
}

func (m *Manifest) addConfig(source string, raw []byte) error {
	var top map[string]json.RawMessage
	if err := strictDecode(raw, &top); err != nil {
		return configBlock("native_config_unreadable", source)
	}
	for key := range top {
		if slices.Contains(credentialKeys, strings.ToLower(key)) {
			return configBlock("native_settings_credential_override", source+":"+key)
		}
	}
	var cfg codexConfig
	if err := strictDecode(raw, &cfg); err != nil {
		return configBlock("native_config_unreadable", source)
	}
	for _, entries := range cfg.Hooks {
		m.Hooks += len(entries)
	}
	for name := range cfg.MCPServers {
		m.MCPServers = append(m.MCPServers, source+":"+name)
	}
	return nil
}

// strictDecode rejects trailing data and duplicate top-level keys; unknown
// fields are tolerated because the native config carries many unrelated keys.
func strictDecode(raw []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(out); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("trailing data")
	}
	if _, ok := out.(*map[string]json.RawMessage); ok {
		return rejectDuplicateKeys(raw)
	}
	return nil
}

func rejectDuplicateKeys(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if _, err := dec.Token(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key, _ := tok.(string)
		if seen[strings.ToLower(key)] {
			return errors.New("duplicate key")
		}
		seen[strings.ToLower(key)] = true
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return err
		}
	}
	return nil
}

func configBlock(code, field string) error {
	return &adapter.BlockedError{Code: code, Field: field, Action: "fix it in native configuration; MYTHHELM does not edit or strip native configuration"}
}

// readSource reads one enumerated file: absent is not an error, anything that
// is not a regular file is ErrNonRegularSource and any other failure is
// ErrCapability.
func readSource(path string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, fmt.Errorf("%w: native_config_unreadable: %s", ErrCapability, filepath.Base(path))
	}
	if !info.Mode().IsRegular() {
		return nil, true, fmt.Errorf("%w: %s", ErrNonRegularSource, filepath.Base(path))
	}
	f, err := os.Open(path) //nolint:gosec // Enumerated native config source; never a credentials path.
	if err != nil {
		return nil, true, fmt.Errorf("%w: native_config_unreadable: %s", ErrCapability, filepath.Base(path))
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	if err != nil || len(raw) > maxConfigBytes {
		return nil, true, fmt.Errorf("%w: native_config_unreadable: %s", ErrCapability, filepath.Base(path))
	}
	return raw, true, nil
}
