// Adversarial escape fixtures for the v1 Linux boundary (design §2.10,
// AC-8.1). Linux-only by filename suffix: every fixture needs user
// namespaces and /bin/sh, and the proxy leg needs python3 for its CONNECT
// client.
//
// Each fixture FAILS inside a restricted run and SUCCEEDS on trusted-host,
// with the counterfactual in the same test: a fixture that fails for the
// wrong reason (a broken script, a check that never ran) fails the
// trusted-host leg, and deleting containment from the run under test fails
// the restricted leg. The filesystem, mount, subprocess and credential
// fixtures run as admitted checks in real runs; the filtering proxy exists
// only on the native spawn path (the worker starts it per design §2.3,
// checks carry none), so the evil-host fixture drives the same packaged
// __contain entry the worker uses, with the same policy shape
// launchForAttempt builds and the same ProxyEnv pins the worker amends.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/contain"
)

// requireBoundary skips unless the Linux boundary mechanism is available
// here. CI Linux runners provide user namespaces; tighter containers skip.
func requireBoundary(t *testing.T) {
	t.Helper()
	if a := contain.ProbeLinux(); !a.Supported {
		t.Skipf("boundary unavailable here: %s", a.Reason)
	}
}

// isOutsideTmp reports whether path resolves outside /tmp, which the
// boundary replaces with its own tmpfs.
func isOutsideTmp(path string) bool {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	tmp, err := filepath.EvalSymlinks("/tmp")
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(tmp, resolved)
	return err == nil && (rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// outsideBase makes a scratch directory outside /tmp for the contained
// fixtures: a HOME beneath /tmp would not survive EnterLinux, and fixture
// paths beneath it would fail with ENOENT instead of the enforced denial.
// It tries the test temp root, the package directory and /var/tmp in turn
// and skips when none qualifies.
func outsideBase(t *testing.T) string {
	t.Helper()
	for _, parent := range []string{os.TempDir(), ".", "/var/tmp"} {
		if dir, ok := tryOutsideBase(t, parent); ok {
			return dir
		}
	}
	t.Skip("no writable directory outside /tmp for the contained fixtures")
	return ""
}

// tryOutsideBase makes one candidate base under parent.
func tryOutsideBase(t *testing.T, parent string) (string, bool) {
	t.Helper()
	dir, err := os.MkdirTemp(parent, "e2e-contain-") //nolint:usetesting // must sit outside /tmp, which t.TempDir uses
	if err != nil {
		return "", false
	}
	abs, err := filepath.Abs(dir)
	if err != nil || !isOutsideTmp(abs) {
		_ = os.RemoveAll(dir)
		return "", false
	}
	t.Cleanup(func() { _ = os.RemoveAll(abs) })
	return abs, true
}

// containBin returns a mythhelm binary a contained child can exec. The
// boundary replaces /tmp with a fresh tmpfs, so a binary staged beneath it
// (mythhelmBin when TMPDIR is /tmp) would vanish before the native execs;
// the copy lives under base and is removed with it.
func containBin(t *testing.T, base string) string {
	t.Helper()
	if isOutsideTmp(mythhelmBin) {
		return mythhelmBin
	}
	dst := filepath.Join(base, "mythhelm-e2e-contain")
	src, err := os.Open(mythhelmBin)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = src.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755) //nolint:gosec // G304: dst is a fixed name under the test-owned scratch base
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(out, src)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

// newEnvWithHome is newEnv with HOME (and its XDG siblings) moved to an
// outside-/tmp home the boundary accepts.
func newEnvWithHome(t *testing.T, home string) env {
	t.Helper()
	e := newEnv(t)
	e.home = home
	for i, kv := range e.vars {
		name, _, _ := strings.Cut(kv, "=")
		switch name {
		case "HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_STATE_HOME":
			e.vars[i] = name + "=" + home
		}
	}
	return e
}

// containCheck is one admitted check of a fixture repository.
type containCheck struct {
	name string
	argv []string
}

// mkContainRepo seeds a committed repository with the given admitted checks
// and returns the repo path and the config digest the run must trust.
func mkContainRepo(t *testing.T, e env, root string, checks []containCheck) (string, string) {
	t.Helper()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o750); err != nil {
		t.Fatal(err)
	}
	git := func(invocations ...[]string) {
		t.Helper()
		for _, args := range invocations {
			cmd := exec.Command("git", args...)
			cmd.Dir = repo
			cmd.Env = e.withParent(t)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
			}
		}
	}
	git([]string{"init", "--quiet", "--initial-branch=main"},
		[]string{"config", "user.name", "E2E"},
		[]string{"config", "user.email", "e2e@example.com"})
	var config strings.Builder
	config.WriteString("schema_version = 1\n")
	for _, c := range checks {
		config.WriteString("[[checks]]\nname = " + strconv.Quote(c.name) + "\nargv = [")
		for i, a := range c.argv {
			if i > 0 {
				config.WriteString(", ")
			}
			config.WriteString(strconv.Quote(a))
		}
		config.WriteString("]\ntimeout = \"30s\"\n")
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("e2e\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "task.md"), []byte("# E2E task\n\nWrite demo.txt.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "mythhelm.toml"), []byte(config.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	git([]string{"add", "README.md", "task.md", "mythhelm.toml"},
		[]string{"commit", "--quiet", "-m", "seed"})
	_, digest, err := admission.ParseProjectConfig([]byte(config.String()))
	if err != nil {
		t.Fatal(err)
	}
	return repo, digest
}

// evidenceOf concatenates every check evidence log journaled under state.
// Each leg runs alone in a fresh state dir, so exactly one run's logs
// match; when none do the check never ran, which is unavailability rather
// than the enforced failure the restricted leg must show.
func evidenceOf(t *testing.T, state string) string {
	t.Helper()
	logs, err := filepath.Glob(filepath.Join(state, "runs", "*", "evidence", "*", "*.log"))
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) == 0 {
		t.Fatal("no check evidence was journaled: the check never ran (unavailable, not failed)")
	}
	var b strings.Builder
	for _, l := range logs {
		content, err := os.ReadFile(l) //nolint:gosec // G304: l comes from a Glob under the test-owned state dir
		if err != nil {
			t.Fatal(err)
		}
		b.Write(content)
	}
	return b.String()
}

// checkLegs runs one escape fixture under restricted (the attack must fail,
// exit 5) and trusted-host (it must succeed, exit 0). The script must print
// PROBE_OK once its shell runs and ATTACK_SUCCEEDED only when the attack
// lands; marker is the attack's side-effect file ("" when the evidence
// carries the outcome instead), and secret is ambient bytes the evidence
// must hide contained and show on the host ("" to skip).
func checkLegs(t *testing.T, bin, home string, check containCheck, marker, secret string) {
	t.Helper()
	if marker != "" {
		t.Cleanup(func() { _ = os.Remove(marker) })
	}
	repo, digest := mkContainRepo(t, newEnvWithHome(t, home), t.TempDir(), []containCheck{check})
	for _, leg := range []struct {
		profile string
		want    int
	}{{"restricted", 5}, {"trusted-host", 0}} {
		e := newEnvWithHome(t, home)
		_ = os.Remove(marker)
		code, _, stderr := runBin(t, e.withParent(t), repo, bin, "run", "--task-file", "task.md", "--adapter", "fake",
			"--billing", "local-scripted", "--execution-profile", leg.profile,
			"--non-interactive", "--trust-project-config", "sha256:"+digest, "--scenario", "readonly")
		if code != leg.want {
			t.Fatalf("%s: exit %d, want %d\nstderr:\n%s", leg.profile, code, leg.want, stderr)
		}
		evidence := evidenceOf(t, e.state)
		if !strings.Contains(evidence, "PROBE_OK") {
			t.Fatalf("%s: check evidence lacks PROBE_OK, so the shell never ran:\n%s", leg.profile, evidence)
		}
		switch leg.profile {
		case "restricted":
			if strings.Contains(evidence, "ATTACK_SUCCEEDED") {
				t.Fatalf("restricted: the attack succeeded inside the boundary:\n%s", evidence)
			}
			if marker != "" {
				if _, err := os.Stat(marker); err == nil {
					t.Fatalf("restricted: attack marker %s exists", marker)
				} else if !os.IsNotExist(err) {
					t.Fatalf("restricted: stat attack marker %s: %v", marker, err)
				}
			}
			if secret != "" && strings.Contains(evidence, secret) {
				t.Fatal("restricted: ambient credential bytes leaked into the check evidence")
			}
		case "trusted-host":
			if !strings.Contains(evidence, "ATTACK_SUCCEEDED") {
				t.Fatalf("trusted-host: the attack failed without containment, so the fixture proves nothing:\n%s", evidence)
			}
			if marker != "" {
				if _, err := os.Stat(marker); err != nil {
					t.Fatalf("trusted-host: attack marker %s missing: %v", marker, err)
				}
			}
			if secret != "" && !strings.Contains(evidence, secret) {
				t.Fatal("trusted-host: ambient credential bytes missing from the check evidence")
			}
		}
	}
}

// dquote double-quotes a fixture path for sh. Fixture paths come from
// MkdirTemp, whose names never contain quotes or whitespace.
func dquote(path string) string {
	return `"` + path + `"`
}

// probeScript prefixes an attack with a shell liveness probe: the attack
// verdict means nothing unless the shell ran, so a probe failure exits 43
// (harness broken) rather than the attack's 1.
func probeScript(name, attack string) string {
	return "echo probe > \"$HOME/" + name + ".probe\" || exit 43\n" +
		"echo PROBE_OK\n" + attack + " || exit 1\necho ATTACK_SUCCEEDED\n"
}

// TestRestrictedEscapeFixturesFail runs the five adversarial fixtures: each
// fails inside a restricted run and succeeds on trusted-host.
func TestRestrictedEscapeFixturesFail(t *testing.T) {
	requireBoundary(t)
	base := outsideBase(t)
	bin := containBin(t, base)
	home := filepath.Join(base, "home")
	outside := filepath.Join(base, "outside")
	work := filepath.Join(base, "work")
	for _, dir := range []string{home, outside, work} {
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	secret := "ambient-fixture-bytes-" + filepath.Base(base)
	if err := os.WriteFile(filepath.Join(home, ".e2e-ambient-credential"), []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("outside-write", func(t *testing.T) {
		marker := filepath.Join(outside, "pwned")
		checkLegs(t, bin, home, containCheck{"outside-write",
			[]string{"/bin/sh", "-c", probeScript("outside-write", "echo pwned > "+dquote(marker))}},
			marker, "")
	})

	t.Run("child-mount-write", func(t *testing.T) {
		marker := filepath.Join("/dev/shm", "mythhelm-e2e-"+filepath.Base(base))
		probe := marker + "-probe"
		if err := os.WriteFile(probe, []byte("probe\n"), 0o600); err != nil {
			t.Skipf("no writable child mount at /dev/shm to probe: %v", err)
		}
		_ = os.Remove(probe)
		checkLegs(t, bin, home, containCheck{"child-mount-write",
			[]string{"/bin/sh", "-c", probeScript("child-mount-write", "echo pwned > "+dquote(marker))}},
			marker, "")
	})

	t.Run("subprocess-escape", func(t *testing.T) {
		marker := filepath.Join(outside, "sub-pwned")
		nested := "sh -c 'echo pwned > " + dquote(marker) + "'"
		checkLegs(t, bin, home, containCheck{"subprocess-escape",
			[]string{"/bin/sh", "-c", probeScript("subprocess-escape", nested)}},
			marker, "")
	})

	t.Run("credential-read", func(t *testing.T) {
		checkLegs(t, bin, home, containCheck{"credential-read",
			[]string{"/bin/sh", "-c", probeScript("credential-read", `cat "$HOME/.e2e-ambient-credential"`)}},
			"", secret)
	})

	t.Run("evil-fetch", func(t *testing.T) {
		evilFetchLegs(t, bin, home, outside, work)
	})
}

// fetchPython fetches two targets through $HTTPS_PROXY when pinned (CONNECT,
// like a proxied native) or directly otherwise, and prints one result line
// per target. It uses only double-quoted strings so the sh heredoc below
// stays quotable.
const fetchPython = `import os, socket, sys
evil, allowed = sys.argv[1], sys.argv[2]
proxy = os.environ.get("HTTPS_PROXY", "")
if proxy.startswith("http://"):
    proxy = proxy[len("http://"):]
def fetch(target):
    if proxy:
        host, _, port = proxy.rpartition(":")
        s = socket.create_connection((host, int(port)), timeout=10)
        s.sendall(("CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\n\r\n").encode())
        resp = b""
        while b"\r\n\r\n" not in resp:
            chunk = s.recv(4096)
            if not chunk:
                break
            resp += chunk
        parts = resp.split(b"\r\n", 1)[0].decode().split(" ")
        if len(parts) < 2 or parts[1] != "200":
            s.close()
            return "denied:" + (parts[1] if len(parts) >= 2 else "no-status")
    else:
        host, _, port = target.rpartition(":")
        s = socket.create_connection((host, int(port)), timeout=10)
    s.sendall(b"GET / HTTP/1.0\r\n\r\n")
    body = b""
    while True:
        chunk = s.recv(4096)
        if not chunk:
            break
        body += chunk
    s.close()
    return "ok:" + body.decode()
print("allowed=" + fetch(allowed))
print("evil=" + fetch(evil))
`

// serveFixtureBytes serves body over HTTP on loopback and counts dials.
func serveFixtureBytes(t *testing.T, body string) (string, *atomic.Int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	var hits atomic.Int32
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			hits.Add(1)
			go func() {
				defer func() { _ = conn.Close() }()
				_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
				_, _ = conn.Read(make([]byte, 4096))
				_, _ = fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
					len(body), body)
			}()
		}
	}()
	return ln.Addr().String(), &hits
}

// fixturePython returns an absolute runnable python3 for the CONNECT
// fixture. It prefers /usr/bin/python3: PATH may resolve to a
// version-manager shim that runs on the host but cannot run inside the
// boundary (its version state lives under a HOME the boundary replaces),
// and mere presence on PATH does not prove the interpreter runs. Without
// a runnable interpreter the fixture cannot bite, so the caller skips.
func fixturePython(t *testing.T) string {
	t.Helper()
	if runnablePython("/usr/bin/python3") {
		return "/usr/bin/python3"
	}
	if path, err := exec.LookPath("python3"); err == nil && runnablePython(path) {
		return path
	}
	t.Skip("no runnable python3 for the CONNECT fixture")
	return ""
}

// runnablePython reports whether path executes (a bare LookPath hit may be
// a broken shim).
func runnablePython(path string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, path, "--version").Run() == nil
}

// evilFetchLegs runs the network fixture: an evil-host fetch through the
// pinned filtering proxy must be denied inside the boundary (exit 1, and
// the evil server is never dialled), while the same fetch without proxy
// pins (the trusted-host shape) succeeds. The contained leg also guards
// against a vacuous pass: an outside write that lands exits 42 (the
// boundary is missing, not enforcing), a broken interpreter exits 44, and
// an allowlisted fetch that fails exits 43 (the proxy is broken, not
// denying).
func evilFetchLegs(t *testing.T, bin, home, outside, work string) {
	t.Helper()
	python := fixturePython(t)
	evilAddr, evilHits := serveFixtureBytes(t, "evil-bytes")
	allowedAddr, allowedHits := serveFixtureBytes(t, "allowed-bytes")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	proxyAddr, stop, err := contain.ServeProxy(ctx, []string{allowedAddr})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	probe := filepath.Join(outside, "evil-contained-probe")
	t.Cleanup(func() { _ = os.Remove(probe) })
	containedScript := "touch " + dquote(probe) + " 2>/dev/null && exit 42\n" +
		"out=$(" + dquote(python) + " - " + dquote(evilAddr) + " " + dquote(allowedAddr) + " <<'PYEOF'\n" + fetchPython + "PYEOF\n" +
		") || exit 44\necho \"$out\"\n" +
		"case \"$out\" in *\"allowed=ok:\"*) ;; *) exit 43 ;; esac\n" +
		"case \"$out\" in *\"evil=denied:403\"*) exit 1 ;; *) exit 0 ;; esac\n"
	spec := contain.ContainSpec{
		Path: "/bin/sh",
		Args: []string{"sh", "-c", containedScript},
		Dir:  work,
		Env: []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"),
			"HTTPS_PROXY=http://" + proxyAddr, "HTTP_PROXY=http://" + proxyAddr,
			"https_proxy=http://" + proxyAddr, "http_proxy=http://" + proxyAddr},
		Policy: contain.Policy{Profile: "restricted", Workdir: work, ProxyAddr: proxyAddr},
	}
	body, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = null.Close() }()
	containedCtx, stopContained := context.WithTimeout(context.Background(), 2*time.Minute)
	defer stopContained()
	contained := exec.CommandContext(containedCtx, bin, contain.Command)
	// The worker spawns __contain in new user and mount namespaces;
	// without the clone flags EnterLinux would operate on the host mounts.
	contained.SysProcAttr = contain.NamespaceAttr()
	contained.Stdin = bytes.NewReader(body)
	contained.ExtraFiles = []*os.File{null}
	var cout, cerr bytes.Buffer
	contained.Stdout, contained.Stderr = &cout, &cerr
	runErr := contained.Run()
	_ = os.Remove(probe)
	if exit := exitCode(runErr); exit != 1 {
		if exit == 44 {
			t.Fatalf("contained evil fetch cannot run its interpreter:\nstdout:\n%s\nstderr:\n%s", cout.String(), cerr.String())
		}
		t.Fatalf("contained evil fetch exited %d, want 1 (denied)\nstdout:\n%s\nstderr:\n%s", exit, cout.String(), cerr.String())
	}
	if !strings.Contains(cout.String(), "evil=denied:403") {
		t.Fatalf("contained evil fetch lacks the 403 verdict, so the proxy never ruled:\n%s\nstderr:\n%s",
			cout.String(), cerr.String())
	}
	if evilHits.Load() != 0 {
		t.Fatalf("the evil server took %d dials through the denying proxy", evilHits.Load())
	}
	if allowedHits.Load() == 0 {
		t.Fatal("the allowlisted server took no dials: the proxy never forwarded")
	}

	hostScript := "out=$(" + dquote(python) + " - " + dquote(evilAddr) + " " + dquote(allowedAddr) + " <<'PYEOF'\n" + fetchPython +
		"PYEOF\n)\necho \"$out\"\n" +
		"case \"$out\" in *\"allowed=ok:\"*\"evil=ok:\"*) exit 0 ;; *) exit 1 ;; esac\n"
	hostEnv := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(name) {
		case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY":
			return true
		}
		return false
	})
	hostCtx, stopHost := context.WithTimeout(context.Background(), 2*time.Minute)
	defer stopHost()
	host := exec.CommandContext(hostCtx, "/bin/sh", "-c", hostScript)
	host.Env = hostEnv
	var hout, herr bytes.Buffer
	host.Stdout, host.Stderr = &hout, &herr
	if err := host.Run(); err != nil {
		t.Fatalf("unpinned evil fetch failed, so the fixture proves nothing: %v\nstdout:\n%s\nstderr:\n%s",
			err, hout.String(), herr.String())
	}
	if !strings.Contains(hout.String(), "evil-bytes") || !strings.Contains(hout.String(), "allowed-bytes") {
		t.Fatalf("unpinned evil fetch lacks the fixture bytes:\n%s", hout.String())
	}
	if evilHits.Load() == 0 || allowedHits.Load() == 0 {
		t.Fatal("the unpinned fetch dialled nothing: the servers never bit")
	}
}

// exitCode unwraps an exec result to its exit code, or -1 when the process
// never ran.
func exitCode(err error) int {
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return exit.ExitCode()
	}
	if err != nil {
		return -1
	}
	return 0
}

// TestInspectWriteFails pins the read-only half of the shared mechanism: the
// same workdir write the happy scenario lands under restricted (exit 0,
// demo.txt present) fails under inspect (exit 4, demo.txt absent), because
// the workdir is mounted read-only there.
func TestInspectWriteFails(t *testing.T) {
	requireBoundary(t)
	base := outsideBase(t)
	bin := containBin(t, base)
	home := filepath.Join(base, "home")
	if err := os.Mkdir(home, 0o750); err != nil {
		t.Fatal(err)
	}
	pass := containCheck{"e2e", []string{"/bin/sh", "-c", "exit 0"}}
	for _, leg := range []struct {
		profile  string
		wantExit int
		wantDemo bool
	}{
		{"inspect", 4, false},
		{"restricted", 0, true},
	} {
		e := newEnvWithHome(t, home)
		repo, digest := mkContainRepo(t, e, t.TempDir(), []containCheck{pass})
		code, _, stderr := runBin(t, e.withParent(t), repo, bin, "run", "--task-file", "task.md", "--adapter", "fake",
			"--billing", "local-scripted", "--execution-profile", leg.profile,
			"--non-interactive", "--trust-project-config", "sha256:"+digest, "--scenario", "happy")
		if code != leg.wantExit {
			t.Fatalf("%s: exit %d, want %d\nstderr:\n%s", leg.profile, code, leg.wantExit, stderr)
		}
		demos, err := filepath.Glob(filepath.Join(e.state, "runs", "*", "workspace", "demo.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if got := len(demos) != 0; got != leg.wantDemo {
			t.Fatalf("%s: demo.txt present = %v, want %v", leg.profile, got, leg.wantDemo)
		}
	}
}
