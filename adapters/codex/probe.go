package codex

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/security"
)

// AdapterID identifies the Codex native adapter in descriptors and records.
const AdapterID = "builtin/codex"

const (
	// maxProbeOutput bounds the bytes read from the native's --version.
	maxProbeOutput = 4096
	probeTimeout   = 5 * time.Second
)

var versionLine = regexp.MustCompile(`^codex-cli (\d+\.\d+\.\d+)$`)

type codexAdapter struct {
	probeTimeout time.Duration
}

// New returns a Codex adapter.
func New() adapter.Adapter { return &codexAdapter{probeTimeout: probeTimeout} }

func descriptor() adapter.Descriptor {
	return adapter.Descriptor{ID: AdapterID, Version: "1", Harness: "codex", Surface: "native-app-server (structured)"}
}

func (*codexAdapter) Descriptor() adapter.Descriptor { return descriptor() }

// Probe resolves the executable and runs only its bounded --version: it
// never starts a model task (AC-2.1).
func (a *codexAdapter) Probe(ctx context.Context, in adapter.ProbeInput) (adapter.Probe, error) {
	exe := in.ExecutableOverride
	if exe != "" && !testing.Testing() {
		return adapter.Probe{}, fmt.Errorf("%w: test executable override unavailable", ErrCapability)
	}
	if exe == "" {
		var err error
		exe, err = exec.LookPath("codex")
		if err != nil {
			return adapter.Probe{}, fmt.Errorf("%w: native_executable_missing: install Codex through its native setup", ErrCapability)
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
	digest, err := fileSHA256(exe)
	if err != nil {
		return adapter.Probe{}, fmt.Errorf("%w: hash native executable", ErrCapability)
	}
	env := in.Env
	if env == nil {
		env, err = security.BuildEnv(os.Environ(), nil, nil)
		if err != nil {
			return adapter.Probe{}, err
		}
	}
	stdout, err := runProbe(ctx, a.probeTimeout, exe, env, in.Workdir, "--version")
	if err != nil {
		return adapter.Probe{}, fmt.Errorf("%w: native_version_probe_failed", ErrCapability)
	}
	match := versionLine.FindSubmatch(stdout)
	if len(match) != 2 {
		return adapter.Probe{}, fmt.Errorf("%w: native_version_unrecognised", ErrCapability)
	}
	version := string(match[1])
	label, err := compatibility(version, in.AllowUntestedNativeVersion)
	if err != nil {
		return adapter.Probe{}, err
	}
	return adapter.Probe{Executable: exe, Version: version, SHA256: digest, OS: runtime.GOOS, Arch: runtime.GOARCH, Compatibility: label}, nil
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

// boundedOutput holds the buffer by name, not by embedding: an embedded
// bytes.Buffer would promote ReadFrom, which io.Copy prefers over Write and
// which does not enforce the limit.
type boundedOutput struct {
	buf      bytes.Buffer
	exceeded bool
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > maxProbeOutput-b.buf.Len() {
		b.exceeded = true
		return 0, errors.New("probe output exceeded limit")
	}
	return b.buf.Write(p)
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
	return bytes.TrimSpace(output.buf.Bytes()), nil
}
