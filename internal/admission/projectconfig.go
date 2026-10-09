package admission

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/turbokast/mythhelm/adapters/claudecode"
	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/security"
	"github.com/turbokast/mythhelm/internal/workspace"
)

// ProjectConfigFile is the admitted configuration file name.
const (
	ProjectConfigFile = "mythhelm.toml"
	maxProjectConfig  = 1 << 20
)

var (
	// ErrNoProjectConfig means the admitted revision has no checks file.
	ErrNoProjectConfig = errors.New("mythhelm.toml is missing")
	// ErrProjectConfig marks invalid or unsafe configuration (CLI exit 2).
	ErrProjectConfig = errors.New("invalid mythhelm.toml")
	checkName        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
)

// ProjectConfig is the strict design §8 subset. Digest identifies the exact
// bytes that were admitted; no candidate-edited copy is ever executed.
type ProjectConfig struct {
	SchemaVersion int `toml:"schema_version"`
	Environment   struct {
		Passthrough []string `toml:"passthrough"`
	} `toml:"environment"`
	Adapters struct {
		ClaudeCode struct {
			AllowedTools []string `toml:"allowed_tools"`
		} `toml:"claudecode"`
	} `toml:"adapters"`
	Checks    []CheckConfig   `toml:"checks"`
	Envelopes EnvelopesConfig `toml:"envelopes"`
}

// EnvelopesConfig is the optional [envelopes] table: the run-configuration
// layer of the finite run envelope. An absent field is unset, not zero.
type EnvelopesConfig struct {
	Execution        string `toml:"execution"`
	Repairs          *int   `toml:"repairs"`
	Replans          *int   `toml:"replans"`
	TransportRetries *int   `toml:"transport_retries"`
}

// ToCeilings returns the file layer with unset fields negative, or nil when
// the table is absent. An invalid duration or a negative count is an error,
// never a default (I02).
func (e EnvelopesConfig) ToCeilings() (*billing.Ceilings, error) {
	if e.Execution == "" && e.Repairs == nil && e.Replans == nil && e.TransportRetries == nil {
		return nil, nil
	}
	c := billing.Ceilings{Execution: -1, Repairs: -1, Replans: -1, TransportRetries: -1}
	if e.Execution != "" {
		d, err := time.ParseDuration(e.Execution)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("envelopes.execution %q must be a positive duration", e.Execution)
		}
		c.Execution = d
	}
	for _, f := range []struct {
		name string
		in   *int
		out  *int
	}{{"repairs", e.Repairs, &c.Repairs}, {"replans", e.Replans, &c.Replans}, {"transport_retries", e.TransportRetries, &c.TransportRetries}} {
		if f.in == nil {
			continue
		}
		if *f.in < 0 {
			return nil, fmt.Errorf("envelopes.%s must not be negative, got %d", f.name, *f.in)
		}
		*f.out = *f.in
	}
	return &c, nil
}

// CheckConfig is an argv check and its output/timeout policy.
type CheckConfig struct {
	Name         string   `toml:"name"`
	Argv         []string `toml:"argv"`
	FailOnOutput bool     `toml:"fail_on_output"`
	Timeout      string   `toml:"timeout"`
}

// Duration returns the validated timeout.
func (c CheckConfig) Duration() time.Duration {
	d, _ := time.ParseDuration(c.Timeout)
	return d
}

// LoadProjectConfig reads the snapshot's committed blob when the directory is
// a Git clone. Git checkout may change line endings on Windows, but trust is
// bound to the committed bytes. Non-Git directories are accepted for direct
// config validation. In both cases symlink escapes and unknown keys fail.
func LoadProjectConfig(snapshotDir string) (ProjectConfig, string, error) {
	var zero ProjectConfig
	outside, err := security.ResolvesOutside(snapshotDir, ProjectConfigFile)
	if err != nil || outside {
		return zero, "", fmt.Errorf("%w: config path is not safely inside the snapshot", ErrProjectConfig)
	}
	_, gitErr := os.Lstat(filepath.Join(snapshotDir, ".git"))
	if gitErr != nil && !errors.Is(gitErr, os.ErrNotExist) {
		return zero, "", fmt.Errorf("%w: inspect snapshot Git directory: %w", ErrProjectConfig, gitErr)
	}
	if gitErr == nil {
		entry, err := workspace.Git(context.Background(), snapshotDir, false, "ls-tree", "HEAD", "--", ProjectConfigFile)
		if err != nil {
			return zero, "", fmt.Errorf("%w: locate snapshot config: %w", ErrProjectConfig, err)
		}
		if len(entry) == 0 {
			return zero, "", ErrNoProjectConfig
		}
		if !strings.HasPrefix(string(entry), "100644 blob ") && !strings.HasPrefix(string(entry), "100755 blob ") {
			return zero, "", fmt.Errorf("%w: config must be a regular committed file", ErrProjectConfig)
		}
		raw, err := workspace.Git(context.Background(), snapshotDir, false, "show", "HEAD:"+ProjectConfigFile)
		if err != nil {
			return zero, "", fmt.Errorf("%w: read snapshot config: %w", ErrProjectConfig, err)
		}
		return ParseProjectConfig(raw)
	}
	f, err := os.Open(filepath.Join(snapshotDir, ProjectConfigFile)) //nolint:gosec // The path was checked against the snapshot root.
	if errors.Is(err, os.ErrNotExist) {
		return zero, "", ErrNoProjectConfig
	}
	if err != nil {
		return zero, "", fmt.Errorf("%w: open: %w", ErrProjectConfig, err)
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, maxProjectConfig+1))
	if err != nil {
		return zero, "", fmt.Errorf("%w: read: %w", ErrProjectConfig, err)
	}
	return ParseProjectConfig(raw)
}

// ParseProjectConfig validates exactly the admitted Git blob. A separate
// snapshot read must return the same digest before the worker is launched.
func ParseProjectConfig(raw []byte) (ProjectConfig, string, error) {
	var cfg ProjectConfig
	if len(raw) > maxProjectConfig {
		return cfg, "", fmt.Errorf("%w: larger than %d bytes", ErrProjectConfig, maxProjectConfig)
	}
	meta, err := toml.Decode(string(raw), &cfg)
	if err != nil {
		return cfg, "", fmt.Errorf("%w: %w", ErrProjectConfig, err)
	}
	if keys := meta.Undecoded(); len(keys) > 0 {
		return cfg, "", fmt.Errorf("%w: unknown key %s", ErrProjectConfig, keys[0].String())
	}
	if cfg.SchemaVersion != 1 {
		return cfg, "", fmt.Errorf("%w: schema_version must be 1", ErrProjectConfig)
	}
	if _, err := security.BuildEnv(nil, cfg.Environment.Passthrough, nil); err != nil {
		return cfg, "", fmt.Errorf("%w: %w", ErrProjectConfig, err)
	}
	for _, rule := range cfg.Adapters.ClaudeCode.AllowedTools {
		// Dash-prefixed rules are refused at the config gate too: past
		// --allowedTools the native would parse them as its own options.
		if rule == "" || len(rule) > claudecode.MaxAllowedToolRule || strings.ContainsRune(rule, 0) || strings.HasPrefix(rule, "-") {
			return cfg, "", fmt.Errorf("%w: invalid adapters.claudecode allowed_tools rule %q", ErrProjectConfig, rule)
		}
	}
	if _, err := cfg.Envelopes.ToCeilings(); err != nil {
		return cfg, "", fmt.Errorf("%w: %w", ErrProjectConfig, err)
	}
	seen := make(map[string]bool, len(cfg.Checks))
	for _, check := range cfg.Checks {
		if !checkName.MatchString(check.Name) || seen[check.Name] {
			return cfg, "", fmt.Errorf("%w: invalid or duplicate check name %q", ErrProjectConfig, check.Name)
		}
		seen[check.Name] = true
		if len(check.Argv) == 0 || strings.TrimSpace(check.Argv[0]) == "" {
			return cfg, "", fmt.Errorf("%w: check %s needs argv", ErrProjectConfig, check.Name)
		}
		for _, arg := range check.Argv {
			if strings.ContainsRune(arg, 0) {
				return cfg, "", fmt.Errorf("%w: check %s argv contains NUL", ErrProjectConfig, check.Name)
			}
		}
		d, err := time.ParseDuration(check.Timeout)
		if err != nil || d <= 0 {
			return cfg, "", fmt.Errorf("%w: check %s needs a positive timeout", ErrProjectConfig, check.Name)
		}
	}
	sum := sha256.Sum256(raw)
	return cfg, hex.EncodeToString(sum[:]), nil
}
