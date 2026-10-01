package claudecode

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/security"
)

const AdapterID = "builtin/claudecode"
const maxProbeOutput = 1 << 20

// ErrUnavailable keeps execution unavailable until Task 17 adds the owned
// launch/session implementation. Probing never launches an inference task.
var ErrUnavailable = errors.New("claude code launch is unavailable until launch integration")

type binaryKey struct {
	path  string
	size  int64
	mtime time.Time
}
type claudeAdapter struct {
	mu     sync.Mutex // guards the per-adapter executable digest cache
	hashes map[binaryKey]string
}

func New() adapter.Adapter { return &claudeAdapter{hashes: make(map[binaryKey]string)} }
func (*claudeAdapter) Descriptor() adapter.Descriptor {
	return adapter.Descriptor{ID: AdapterID, Version: "0.1.0", Harness: "claude-code", Surface: "native-cli-structured (print, stream-json)"}
}
func (a *claudeAdapter) Probe(ctx context.Context, in adapter.ProbeInput) (adapter.Probe, error) {
	return a.probe(ctx, in, runtime.GOOS)
}
func probePlatform(ctx context.Context, in adapter.ProbeInput, goos string) (adapter.Probe, error) {
	return (&claudeAdapter{hashes: make(map[binaryKey]string)}).probe(ctx, in, goos)
}

func (a *claudeAdapter) probe(ctx context.Context, in adapter.ProbeInput, goos string) (adapter.Probe, error) {
	if goos == "windows" {
		return adapter.Probe{}, fmt.Errorf("%w: process_tree_ownership unsupported on Windows", ErrCapability)
	}
	if goos != "linux" && goos != "darwin" {
		return adapter.Probe{}, fmt.Errorf("%w: platform unqualified", ErrCapability)
	}
	exe := in.ExecutableOverride
	if exe != "" && !testing.Testing() {
		return adapter.Probe{}, fmt.Errorf("%w: test executable override unavailable", ErrCapability)
	}
	if exe == "" {
		var err error
		exe, err = exec.LookPath("claude")
		if err != nil {
			return adapter.Probe{}, fmt.Errorf("%w: native_executable_missing: install Claude Code through its native setup", ErrCapability)
		}
	}
	exe, err := filepath.Abs(exe)
	if err != nil {
		return adapter.Probe{}, fmt.Errorf("%w: resolve native path", ErrCapability)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return adapter.Probe{}, fmt.Errorf("%w: resolve native path", ErrCapability)
	}
	info, err := os.Stat(exe)
	if err != nil || !info.Mode().IsRegular() {
		return adapter.Probe{}, fmt.Errorf("%w: native_executable_unreadable", ErrCapability)
	}
	key := binaryKey{exe, info.Size(), info.ModTime()}
	a.mu.Lock()
	digest := a.hashes[key]
	a.mu.Unlock()
	if digest == "" {
		digest, err = fileSHA256(exe)
		if err != nil {
			return adapter.Probe{}, fmt.Errorf("%w: hash native executable", ErrCapability)
		}
		a.mu.Lock()
		a.hashes[key] = digest
		a.mu.Unlock()
	}
	env := in.Env
	if env == nil {
		env, err = security.BuildEnv(os.Environ(), nil, nil)
		if err != nil {
			return adapter.Probe{}, err
		}
	}
	stdout, err := runProbe(ctx, 10*time.Second, exe, env, in.Workdir, "--version")
	if err != nil {
		return adapter.Probe{}, fmt.Errorf("%w: native_version_probe_failed", ErrCapability)
	}
	match := regexp.MustCompile(`^(\d+\.\d+\.\d+) \(Claude Code\)$`).FindSubmatch(bytes.TrimSpace(stdout))
	if len(match) != 2 {
		return adapter.Probe{}, fmt.Errorf("%w: native_version_unrecognised", ErrCapability)
	}
	version := string(match[1])
	label, err := compatibility(version, in.AllowUntestedNativeVersion)
	if err != nil {
		return adapter.Probe{}, err
	}
	return adapter.Probe{Executable: exe, Version: version, SHA256: digest, OS: goos, Arch: runtime.GOARCH, Compatibility: label}, nil
}

func (*claudeAdapter) Capabilities(p adapter.Probe) adapter.CapabilityRecord {
	platform := adapter.Platform{OS: p.OS, WorkerDetachment: adapter.Unknown, ProcessTreeOwnership: adapter.Unknown}
	if p.OS == "windows" {
		platform.WorkerDetachment = adapter.Unsupported
		platform.ProcessTreeOwnership = adapter.Unsupported
	}
	return adapter.CapabilityRecord{
		SchemaVersion: 1, AdapterID: AdapterID, AdapterVersion: "0.1.0", RuntimeVersion: p.Version, Mode: "headless", ExecutionSurface: "native-cli-structured (print, stream-json)", HarnessID: "claude-code",
		FidelityQualification: "native executable; launch fidelity awaits fixture validation",
		Billing:               adapter.BillingCapabilities{Entitlement: "user-declared, NOT verified by MYTHHELM", IncludedOnlySupported: adapter.Unsupported, PaidOveragePrevention: adapter.Unknown},
		HostIntegration:       adapter.HostIntegration{HerdrEmbedded: adapter.Unsupported, HerdrStateBridge: adapter.Unsupported, NativeInteractiveAttach: adapter.Unsupported},
		Capabilities:          adapter.Capabilities{StructuredEvents: adapter.Unknown, Resume: adapter.Unsupported, LiveSteer: adapter.Unsupported, ApprovalBridge: adapter.Unsupported, UsageTokens: adapter.Unknown, QuotaRemaining: adapter.Unknown, HardMonetaryLimit: adapter.Unsupported, NativeSubagents: adapter.Unknown},
		Sandbox:               adapter.Sandbox{Status: adapter.Unsupported, Scope: "trusted-host: not contained"}, Platform: platform, Qualification: p.Compatibility + "; G05 not-passed",
	}
}
func (*claudeAdapter) Prepare(context.Context, adapter.PrepareInput) (adapter.LaunchProposal, error) {
	return adapter.LaunchProposal{}, ErrUnavailable
}
func (*claudeAdapter) Start(context.Context, adapter.LaunchProposal, adapter.Launcher) (adapter.Session, error) {
	return nil, ErrUnavailable
}

// AuthEvidence contains only supported non-secret status fields and a hashed
// account reference. It cannot carry email, orgName or the raw orgId.
type AuthEvidence struct {
	LoggedIn         bool   `json:"logged_in"`
	AuthMethod       string `json:"auth_method"`
	APIProvider      string `json:"api_provider"`
	SubscriptionType string `json:"subscription_type"`
	ConfigDirectory  string `json:"-"`
	IdentityRef      string `json:"identity_ref"`
}

// AuthStatus invokes the already-resolved executable with the exact admitted
// child environment. Native output and stderr are never included in errors.
func AuthStatus(ctx context.Context, p adapter.Probe, env []string, workdir string) (AuthEvidence, error) {
	if !filepath.IsAbs(p.Executable) || !filepath.IsAbs(workdir) {
		return AuthEvidence{}, authSetupError()
	}
	digest, err := fileSHA256(p.Executable)
	if err != nil || digest != p.SHA256 {
		return AuthEvidence{}, fmt.Errorf("%w: native_executable_changed", ErrCapability)
	}
	raw, err := runProbe(ctx, 20*time.Second, p.Executable, env, workdir, "auth", "status")
	if err != nil {
		return AuthEvidence{}, authSetupError()
	}
	var status struct {
		LoggedIn         bool   `json:"loggedIn"`
		AuthMethod       string `json:"authMethod"`
		APIProvider      string `json:"apiProvider"`
		SubscriptionType string `json:"subscriptionType"`
		ConfigDirectory  string `json:"configDirectory"`
		OrgID            string `json:"orgId"`
	}
	if !validObject(raw) || json.Unmarshal(raw, &status) != nil || !status.LoggedIn || status.AuthMethod != "claude.ai" || status.APIProvider != "firstParty" || !slices.Contains([]string{"pro", "max", "team", "enterprise"}, status.SubscriptionType) || !safeConfigDirectory(status.ConfigDirectory) || status.OrgID == "" || len(status.OrgID) > 256 {
		return AuthEvidence{}, authSetupError()
	}
	// A length-prefixed pair prevents ambiguous account/config concatenations.
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s%d:%s", len(status.OrgID), status.OrgID, len(status.ConfigDirectory), status.ConfigDirectory)))
	return AuthEvidence{LoggedIn: status.LoggedIn, AuthMethod: status.AuthMethod, APIProvider: status.APIProvider, SubscriptionType: status.SubscriptionType, ConfigDirectory: status.ConfigDirectory, IdentityRef: hex.EncodeToString(sum[:])}, nil
}

func safeConfigDirectory(dir string) bool {
	return filepath.IsAbs(dir) && filepath.Clean(dir) == dir && len(dir) <= 4096 && !strings.ContainsFunc(dir, func(r rune) bool { return !unicode.IsPrint(r) }) && len(security.SecretPatternNames([]byte(dir))) == 0
}

func authSetupError() error {
	return &adapter.BlockedError{Code: "needs_native_setup", Field: "auth status", Action: "run 'claude' and /login with your subscription"}
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // Native path is resolved before inspection.
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type boundedOutput struct {
	bytes.Buffer
	exceeded bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > maxProbeOutput-b.Len() {
		b.exceeded = true
		return 0, errors.New("probe output exceeded limit")
	}
	return b.Buffer.Write(p)
}
func runProbe(ctx context.Context, timeout time.Duration, exe string, env []string, dir string, args ...string) ([]byte, error) {
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, exe, args...) //nolint:gosec // Fixed read-only argv; the native executable has been resolved.
	cmd.Env = append([]string{}, env...)
	cmd.Dir = dir
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	var output boundedOutput
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	if output.exceeded {
		return nil, errors.New("probe output exceeded limit")
	}
	return bytes.TrimSpace(output.Bytes()), nil
}

// CredentialOverrideNames lists only route variable names; callers never
// include their values in admission records or diagnostics.
func CredentialOverrideNames() []string {
	return []string{"CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL", "CLAUDE_CODE_OAUTH_TOKEN"}
}

func credentialName(name string) bool {
	for _, n := range CredentialOverrideNames() {
		if strings.EqualFold(name, n) {
			return true
		}
	}
	return false
}
