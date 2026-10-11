package codex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
)

// TestMain re-executes the test binary as a synthetic native Codex when
// GO_WANT_FAKECODEX is set: it logs its argv, optionally sleeps, prints
// FAKE_OUT and exits FAKE_EXIT. It never contacts a model.
func TestMain(m *testing.M) {
	if os.Getenv("GO_WANT_FAKECODEX") == "1" {
		os.Exit(fakeCodex())
	}
	os.Exit(m.Run())
}

func fakeCodex() int {
	if path := os.Getenv("FAKE_LOG"); path != "" {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 G302 -- Fake helper writes only the log its isolated test fixture names.
		if err != nil {
			return 9
		}
		_, _ = fmt.Fprintln(f, strings.Join(os.Args[1:], " "))
		_ = f.Close()
	}
	if os.Getenv("FAKE_SLEEP") == "1" {
		time.Sleep(10 * time.Second)
	}
	fmt.Print(os.Getenv("FAKE_OUT"))
	if os.Getenv("FAKE_EXIT") == "1" {
		return 1
	}
	return 0
}

func fakeProbeInput(t *testing.T, out string, extra ...string) (adapter.ProbeInput, string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "invocations.log")
	env := append([]string{"GO_WANT_FAKECODEX=1", "GORACE=atexit_sleep_ms=0", "FAKE_LOG=" + log, "FAKE_OUT=" + out}, extra...)
	return adapter.ProbeInput{ExecutableOverride: exe, Workdir: t.TempDir(), Env: env}, log
}

func versionOut(version string) string { return "codex-cli " + version + "\n" }

func TestProbeReturnsIdentityWithoutModelTask(t *testing.T) { // I01 (v2 §2), AT-02 (v2 §18.2)
	t.Parallel()
	in, log := fakeProbeInput(t, versionOut(fixtureVersion))
	p, err := New().Probe(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(in.ExecutableOverride)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	wantExe, err := filepath.EvalSymlinks(in.ExecutableOverride)
	if err != nil {
		t.Fatal(err)
	}
	want := adapter.Probe{
		Executable:    wantExe,
		Version:       fixtureVersion,
		SHA256:        hex.EncodeToString(sum[:]),
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		Compatibility: "fixture-tested on " + fixtureVersion,
	}
	if p != want {
		t.Fatalf("probe = %+v, want %+v", p, want)
	}
	if !filepath.IsAbs(p.Executable) {
		t.Fatalf("executable %q is not absolute", p.Executable)
	}
	invoked, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if string(invoked) != "--version\n" {
		t.Fatalf("native invocations = %q, want exactly one --version and no model task", invoked)
	}
}

func TestProbeResolvesSymlinkedExecutable(t *testing.T) { // I01 (v2 §2)
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture requires Unix")
	}
	t.Parallel()
	in, _ := fakeProbeInput(t, versionOut(fixtureVersion))
	link := filepath.Join(t.TempDir(), "codex")
	if err := os.Symlink(in.ExecutableOverride, link); err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(in.ExecutableOverride)
	if err != nil {
		t.Fatal(err)
	}
	in.ExecutableOverride = link
	p, err := New().Probe(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if p.Executable != real {
		t.Fatalf("executable = %q, want the symlink target %q", p.Executable, real)
	}
}

func TestProbeResolvesCodexOnPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH lookup of an extensionless fixture requires Unix")
	}
	in, _ := fakeProbeInput(t, versionOut(fixtureVersion))
	bin := t.TempDir()
	if err := os.Symlink(in.ExecutableOverride, filepath.Join(bin, "codex")); err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(in.ExecutableOverride)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin) // exec.LookPath reads the process PATH; this test is not parallel.
	in.ExecutableOverride = ""
	p, err := New().Probe(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if p.Executable != real {
		t.Fatalf("executable = %q, want %q resolved from PATH", p.Executable, real)
	}
}

func TestProbeMissingExecutableFailsClosed(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // an empty directory: no codex; this test is not parallel.
	in, _ := fakeProbeInput(t, "")
	in.ExecutableOverride = ""
	_, err := New().Probe(context.Background(), in)
	if !errors.Is(err, ErrCapability) || !strings.Contains(err.Error(), "native_executable_missing") {
		t.Fatalf("probe = %v, want ErrCapability native_executable_missing", err)
	}
}

func TestProbeFailsClosedOnUnusableVersionOutput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		out     string
		extra   []string
		wantErr string
	}{
		{name: "an unrecognised banner", out: "something else\n", wantErr: "native_version_unrecognised"},
		{name: "a two-part version", out: "codex-cli 0.0\n", wantErr: "native_version_unrecognised"},
		{name: "a version line with trailing text", out: "codex-cli 0.0.0 extra\n", wantErr: "native_version_unrecognised"},
		{name: "empty output", out: "", wantErr: "native_version_unrecognised"},
		{name: "a failing native", out: versionOut(fixtureVersion), extra: []string{"FAKE_EXIT=1"}, wantErr: "native_version_probe_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in, _ := fakeProbeInput(t, tt.out, tt.extra...)
			_, err := New().Probe(context.Background(), in)
			if !errors.Is(err, ErrCapability) || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("probe = %v, want ErrCapability containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestProbeUntestedVersionBlocksUnlessOverridden(t *testing.T) { // I09 (v2 §2)
	t.Parallel()
	in, _ := fakeProbeInput(t, versionOut("0.0.1"))
	_, err := New().Probe(context.Background(), in)
	if !errors.Is(err, ErrCapability) || !strings.Contains(err.Error(), "native_version_untested") {
		t.Fatalf("untested probe = %v, want ErrCapability native_version_untested", err)
	}
	in.AllowUntestedNativeVersion = true
	p, err := New().Probe(context.Background(), in)
	if err != nil || p.Version != "0.0.1" || !strings.HasPrefix(p.Compatibility, "experimental:") {
		t.Fatalf("overridden probe = %+v, %v; want version 0.0.1 labelled experimental", p, err)
	}
}

func TestProbeBoundedOutput(t *testing.T) { // I02 (v2 §2): an unbounded or hung probe fails closed
	t.Parallel()
	padded := func(n int) string {
		line := versionOut(fixtureVersion)
		return line + strings.Repeat(" ", n-len(line))
	}
	t.Run("output exactly at the limit is read", func(t *testing.T) {
		t.Parallel()
		in, _ := fakeProbeInput(t, padded(maxProbeOutput))
		p, err := New().Probe(context.Background(), in)
		if err != nil || p.Version != fixtureVersion {
			t.Fatalf("probe = %+v, %v; want the version read at the %d byte limit", p, err, maxProbeOutput)
		}
	})
	t.Run("output over the limit fails closed", func(t *testing.T) {
		t.Parallel()
		in, _ := fakeProbeInput(t, padded(maxProbeOutput+1))
		_, err := New().Probe(context.Background(), in)
		if !errors.Is(err, ErrCapability) || !strings.Contains(err.Error(), "native_version_probe_failed") {
			t.Fatalf("probe = %v, want ErrCapability native_version_probe_failed", err)
		}
	})
	t.Run("a probe that outlives the timeout fails closed", func(t *testing.T) {
		t.Parallel()
		in, _ := fakeProbeInput(t, versionOut(fixtureVersion), "FAKE_SLEEP=1")
		a := &codexAdapter{probeTimeout: 300 * time.Millisecond}
		start := time.Now()
		_, err := a.Probe(context.Background(), in)
		if !errors.Is(err, ErrCapability) || !strings.Contains(err.Error(), "native_version_probe_failed") {
			t.Fatalf("probe = %v, want ErrCapability native_version_probe_failed", err)
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("probe took %s, want it bounded by the %s timeout", elapsed, a.probeTimeout)
		}
	})
}

func TestProbeLimitsAreFiveSecondsAndFourKiB(t *testing.T) {
	t.Parallel()
	a, ok := New().(*codexAdapter)
	if !ok || a.probeTimeout != 5*time.Second {
		t.Fatalf("New() = %#v, want a codexAdapter with a 5s probe timeout", New())
	}
	if maxProbeOutput != 4096 {
		t.Fatalf("maxProbeOutput = %d, want 4 KiB", maxProbeOutput)
	}
}

func TestDescriptorNamesTheCodexSurface(t *testing.T) {
	t.Parallel()
	d := New().Descriptor()
	want := adapter.Descriptor{ID: "builtin/codex", Version: "1", Harness: "codex", Surface: "native-app-server (structured)"}
	if d != want {
		t.Fatalf("descriptor = %+v, want %+v", d, want)
	}
}
