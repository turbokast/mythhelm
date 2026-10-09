package workers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/contain"
)

// connectNativeEnv switches the test binary into a native that CONNECTs to the
// targets in connectTargetsEnv ("name=host:port" lines) through HTTPS_PROXY, or
// dials them directly when no proxy is pinned, and prints one result per line.
const (
	connectNativeEnv  = "MYTHHELM_TEST_CONNECT_NATIVE"
	connectTargetsEnv = "MYTHHELM_TEST_CONNECT_TARGETS"
)

func connectNative() int {
	proxy := strings.TrimPrefix(os.Getenv("HTTPS_PROXY"), "http://")
	for line := range strings.SplitSeq(os.Getenv(connectTargetsEnv), "\n") {
		name, target, _ := strings.Cut(line, "=")
		if proxy == "" {
			conn, err := net.DialTimeout("tcp", target, 5*time.Second)
			if err != nil {
				fmt.Printf("%s=dial-failed\n", name)
				continue
			}
			_ = conn.Close()
			fmt.Printf("%s=direct-connected\n", name)
			continue
		}
		conn, err := net.DialTimeout("tcp", proxy, 5*time.Second) //nolint:gosec // G704: the pinned test proxy
		if err != nil {
			fmt.Printf("%s=proxy-unreachable\n", name)
			continue
		}
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		_, _ = fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
		status, _ := bufio.NewReader(conn).ReadString('\n')
		_ = conn.Close()
		fields := strings.Fields(status)
		if len(fields) < 2 {
			fmt.Printf("%s=no-status\n", name)
			continue
		}
		fmt.Printf("%s=%s\n", name, fields[1])
	}
	return 0
}

func TestLaunchWithoutContainmentUnchanged(t *testing.T) {
	spec := adapter.ProcSpec{Path: "/bin/echo", Args: []string{"a b", "c"}, Dir: "/", Env: []string{"A=1", "B=2"}, Stdin: strings.NewReader("prompt")}

	cmd, h, err := Launch{}.command(spec)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path != spec.Path || !reflect.DeepEqual(cmd.Args, []string{"/bin/echo", "a b", "c"}) {
		t.Errorf("argv = %q (path %q), want the native directly", cmd.Args, cmd.Path)
	}
	if cmd.Dir != "/" || !reflect.DeepEqual(cmd.Env, spec.Env) {
		t.Errorf("dir = %q, env = %q, want the admitted values", cmd.Dir, cmd.Env)
	}
	if cmd.Stdin != spec.Stdin || len(cmd.ExtraFiles) != 0 {
		t.Errorf("stdin = %v, extra files = %d, want the prompt on stdin and no extra fd", cmd.Stdin, len(cmd.ExtraFiles))
	}
	if !reflect.DeepEqual(cmd.SysProcAttr, nativeAttr()) {
		t.Errorf("SysProcAttr = %+v, want the native's own group only", cmd.SysProcAttr)
	}
	if h.afterStart != nil || h.cleanup != nil {
		t.Error("an uncontained native has spawn hooks")
	}

	// The same spec with containment routes through __contain, or is refused
	// where the boundary does not exist.
	l := Launch{Containment: &contain.Policy{Profile: "restricted", Workdir: t.TempDir()}}
	cmd, h, err = l.command(spec)
	if runtime.GOOS != "linux" {
		if !errors.Is(err, contain.ErrUnsupported) {
			t.Fatalf("contained command on %s: err = %v, want ErrUnsupported", runtime.GOOS, err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer h.stop()
	if len(cmd.Args) != 2 || cmd.Args[1] != contain.Command || len(cmd.ExtraFiles) != 1 {
		t.Errorf("argv = %q, extra files = %d, want <mythhelm> %s with the prompt pipe on fd 3", cmd.Args, len(cmd.ExtraFiles), contain.Command)
	}
	var got contain.ContainSpec
	if err := json.NewDecoder(cmd.Stdin).Decode(&got); err != nil {
		t.Fatalf("stdin is not a ContainSpec: %v", err)
	}
	if got.Path != spec.Path || !reflect.DeepEqual(got.Args, []string{"/bin/echo", "a b", "c"}) {
		t.Errorf("spec argv = %q (path %q), want the native's full argv", got.Args, got.Path)
	}
}

// containTestLauncher is the launcher of a worker whose launch is l.
func containTestLauncher(t *testing.T, l Launch) *launcher {
	t.Helper()
	w := &worker{dir: t.TempDir(), runID: "run_1", attemptID: "att_1", launch: l, stderr: &ring{max: stderrRingBytes}}
	w.log = slog.New(slog.NewTextHandler(io.Discard, nil))
	return &launcher{w: w}
}

func requireBoundary(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the boundary is Linux-only in v1")
	}
	if a := contain.ProbeLinux(); !a.Supported {
		t.Skipf("boundary unavailable here: %s", a.Reason)
	}
}

// outsideTmpDir makes a directory outside /tmp, which the boundary replaces
// with its own tmpfs.
func outsideTmpDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", "contain") //nolint:usetesting // must sit outside /tmp, which t.TempDir uses
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// runNative launches spec through l and returns its stdout and exit.
func runNative(t *testing.T, l Launch, spec adapter.ProcSpec) (string, adapter.NativeExit) {
	t.Helper()
	proc, err := containTestLauncher(t, l).Launch(t.Context(), spec)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	out, err := io.ReadAll(proc.Stdout())
	if err != nil {
		t.Fatal(err)
	}
	return string(out), proc.Wait()
}

func TestContainedLaunchEndToEnd(t *testing.T) {
	requireBoundary(t)
	workdir, outside, home := t.TempDir(), outsideTmpDir(t), outsideTmpDir(t)
	script := `touch "$WORKDIR/inside"; touch "$OUTSIDE/outside"; true`
	spec := adapter.ProcSpec{
		Path: "/bin/sh",
		Args: []string{"-c", script},
		Dir:  workdir,
		Env:  []string{"PATH=/usr/bin:/bin", "HOME=" + home, "WORKDIR=" + workdir, "OUTSIDE=" + outside},
	}
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
	reset := func() {
		_ = os.Remove(filepath.Join(workdir, "inside"))
		_ = os.Remove(filepath.Join(outside, "outside"))
	}

	reset()
	runNative(t, Launch{}, spec)
	if !exists(filepath.Join(workdir, "inside")) || !exists(filepath.Join(outside, "outside")) {
		t.Fatal("the uncontained fixture did not write at both places; it proves nothing")
	}

	reset()
	l := Launch{Containment: &contain.Policy{Profile: "restricted", Workdir: workdir}}
	runNative(t, l, spec)
	if !exists(filepath.Join(workdir, "inside")) {
		t.Error("the contained native could not write inside the workdir")
	}
	if exists(filepath.Join(outside, "outside")) {
		t.Error("the contained native wrote outside the workdir")
	}
}

func TestContainedStdinForwardedByteForByte(t *testing.T) {
	requireBoundary(t)
	cat, err := exec.LookPath("cat")
	if err != nil {
		t.Skip("no cat")
	}
	prompt := []byte("$(rm -rf /); `id` 'q' \"d\" \\ | & < > ;\r\n\x00\x00end\n\n")
	for i := range 300 << 10 { // beyond a pipe buffer, with NUL and newline bytes throughout
		prompt = append(prompt, byte(i*7+i/251))
	}
	promptFile := filepath.Join(t.TempDir(), "prompt")
	if err := os.WriteFile(promptFile, prompt, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(promptFile) //nolint:gosec // G304: a test temp path
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	workdir := t.TempDir()
	spec := adapter.ProcSpec{Path: cat, Dir: workdir, Env: []string{"PATH=/usr/bin:/bin", "HOME=" + outsideTmpDir(t)}, Stdin: f}
	l := Launch{Containment: &contain.Policy{Profile: "restricted", Workdir: workdir}}
	out, exit := runNative(t, l, spec)
	if exit.Code != 0 {
		t.Fatalf("cat exit = %+v", exit)
	}
	if !bytes.Equal([]byte(out), prompt) {
		t.Fatalf("native read %d bytes, want the prompt's %d byte for byte", len(out), len(prompt))
	}
}

func TestContainedProxyDenyThroughPinnedEnv(t *testing.T) {
	requireBoundary(t)
	allowed := httptest.NewServer(http.NotFoundHandler())
	defer allowed.Close()
	evil := httptest.NewServer(http.NotFoundHandler())
	defer evil.Close()
	allowedAddr := strings.TrimPrefix(allowed.URL, "http://")
	evilAddr := strings.TrimPrefix(evil.URL, "http://")

	// The native is a copy of the test binary outside /tmp, which the
	// boundary replaces.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(exe) //nolint:gosec // G304: this test binary
	if err != nil {
		t.Fatal(err)
	}
	native := filepath.Join(outsideTmpDir(t), "native")
	if err := os.WriteFile(native, body, 0o700); err != nil { //nolint:gosec // G306: an executable fixture
		t.Fatal(err)
	}

	workdir := t.TempDir()
	spec := adapter.ProcSpec{
		Path: native,
		Dir:  workdir,
		Env: []string{
			"PATH=/usr/bin:/bin", "HOME=" + outsideTmpDir(t), connectNativeEnv + "=1",
			connectTargetsEnv + "=evil=" + evilAddr + "\nallowed=" + allowedAddr,
		},
	}
	l := Launch{Containment: &contain.Policy{Profile: "restricted", Workdir: workdir}, ProxyAllow: []string{allowedAddr}}

	cmd, h, err := l.command(spec)
	if err != nil {
		t.Fatal(err)
	}
	var sent contain.ContainSpec
	if err := json.NewDecoder(cmd.Stdin).Decode(&sent); err != nil {
		t.Fatal(err)
	}
	h.stop()
	if sent.Policy.ProxyAddr == "" || !slices.Contains(sent.Env, "HTTPS_PROXY=http://"+sent.Policy.ProxyAddr) {
		t.Fatalf("launched spec: ProxyAddr = %q, env = %q, want the address pinned in HTTPS_PROXY", sent.Policy.ProxyAddr, sent.Env)
	}

	out, exit := runNative(t, l, spec)
	if exit.Code != 0 {
		t.Fatalf("native exit = %+v (output %q)", exit, out)
	}
	if !strings.Contains(out, "evil=403") {
		t.Errorf("evil CONNECT result missing a 403 denial:\n%s", out)
	}
	if !strings.Contains(out, "allowed=200") {
		t.Errorf("allowlisted CONNECT result missing a 200:\n%s", out)
	}
}
