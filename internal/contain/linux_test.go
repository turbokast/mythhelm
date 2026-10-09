package contain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

const helperEnv = "MYTHHELM_CONTAIN_TEST_HELPER"

func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "enter":
		os.Exit(helperRun(true))
	case "plain":
		os.Exit(helperRun(false))
	}
	if os.Getenv(ProbeEnv) != "" {
		os.Exit(RunProbeChild())
	}
	os.Exit(m.Run())
}

func helperRun(contained bool) int {
	var spec ContainSpec
	if err := json.NewDecoder(os.Stdin).Decode(&spec); err != nil {
		fmt.Fprint(os.Stderr, "decode: ", err, "\n")
		return 2
	}
	if contained {
		err := EnterLinux(spec)
		fmt.Fprint(os.Stderr, "enter: ", err, "\n")
		return 1
	}
	err := syscall.Exec(spec.Path, spec.Args, spec.Env)
	fmt.Fprint(os.Stderr, "exec: ", err, "\n")
	return 1
}

func nsSpec(t *testing.T, path string, args []string, env []string, p Policy) ContainSpec {
	t.Helper()
	return ContainSpec{Path: path, Args: args, Dir: p.Workdir, Env: env, Policy: p}
}

func runHelper(t *testing.T, mode string, spec ContainSpec) (string, error) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("namespaces need Linux")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	in, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), helperEnv+"="+mode)
	cmd.Stdin = bytes.NewReader(in)
	cmd.SysProcAttr = nsSysProcAttr()
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err = cmd.Run()
	return out.String(), err
}

func requireUserNamespaces(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("namespaces need Linux")
	}
	if a := ProbeLinux(); !a.Supported {
		t.Skipf("boundary unavailable here: %s", a.Reason)
	}
}

// homeOutsideTmp makes a $HOME that satisfies the boundary's contract: not
// beneath /tmp, which gets its own tmpfs.
func homeOutsideTmp(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", "home") //nolint:usetesting // must sit outside /tmp, which t.TempDir uses
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

func baseEnv(workdir, outside, home string) []string {
	return []string{
		"PATH=/usr/bin:/bin",
		"WORKDIR=" + workdir,
		"OUTSIDE=" + outside,
		"HOME=" + home,
	}
}

func TestProbeLinuxNamesReason(t *testing.T) {
	a := ProbeLinux()
	if a.Supported {
		if a.Version == "" {
			t.Fatalf("supported probe has empty version: %+v", a)
		}
		if a.Reason != "" {
			t.Fatalf("supported probe carries a reason: %+v", a)
		}
		return
	}
	if a.Reason == "" {
		t.Fatalf("unsupported probe names no missing capability: %+v", a)
	}
	if runtime.GOOS != "linux" && !strings.Contains(a.Reason, runtime.GOOS) {
		t.Fatalf("reason %q does not name the unsupported OS %s", a.Reason, runtime.GOOS)
	}
}

func TestProbeLinuxRejectsUnavailableSetup(t *testing.T) {
	steps := []string{"propagation", "readonly", "tmpfs"}
	for _, step := range steps {
		t.Run(step, func(t *testing.T) {
			v, err := probeChild(denyOps{step: step})
			if err == nil {
				t.Fatalf("probe child succeeded (version %q) with %s denied", v, step)
			}
			if !strings.Contains(err.Error(), step) {
				t.Fatalf("error %q does not name the denied step %s", err, step)
			}
			a := probeResult([]byte(err.Error()), errors.New("exit status 1"))
			if a.Supported || a.Reason == "" || !strings.Contains(a.Reason, step) {
				t.Fatalf("probe result for a failed child = %+v, want unsupported naming %s", a, step)
			}
		})
	}

	a := probeResult([]byte("fork/exec: operation not permitted"), errors.New("start failed"))
	if a.Supported || !strings.Contains(a.Reason, "user namespaces") {
		t.Fatalf("probe result for a child that cannot start = %+v, want a user-namespace reason", a)
	}
}

type denyOps struct{ step string }

func (d denyOps) deny(step string) error {
	if d.step == step {
		return syscall.EPERM
	}
	return nil
}

func (d denyOps) Private() error             { return d.deny("propagation") }
func (d denyOps) ReadOnlyTree() error        { return d.deny("readonly") }
func (d denyOps) Tmpfs(string, uint32) error { return d.deny("tmpfs") }
func (d denyOps) Release() (string, error)   { return "6.0-test", nil }

func TestEnterLinuxConfinesFilesystem(t *testing.T) {
	requireUserNamespaces(t)
	base := t.TempDir()
	workdir := filepath.Join(base, "work")
	outside := filepath.Join(base, "outside")
	home := homeOutsideTmp(t)
	for _, d := range []string{workdir, outside} {
		if err := os.Mkdir(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	shm := "/dev/shm"
	if st, err := os.Stat(shm); err != nil || !st.IsDir() {
		t.Skip("no writable child mount at /dev/shm to probe")
	}
	shmFile := filepath.Join(shm, "mythhelm-contain-"+filepath.Base(base))
	t.Cleanup(func() { _ = os.Remove(shmFile) })

	script := `touch "$OUTSIDE/pwned"; touch "$WORKDIR/ok"; touch "$SHMFILE"`
	env := append(baseEnv(workdir, outside, home), "SHMFILE="+shmFile)
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
	reset := func() {
		for _, p := range []string{filepath.Join(outside, "pwned"), filepath.Join(workdir, "ok"), shmFile} {
			_ = os.Remove(p)
		}
	}

	spec := nsSpec(t, "/bin/sh", []string{"sh", "-c", script}, env, Policy{Profile: "restricted", Workdir: workdir})

	t.Run("fixture bites uncontained", func(t *testing.T) {
		reset()
		if out, err := runHelper(t, "plain", spec); err != nil {
			t.Fatalf("uncontained run: %v\n%s", err, out)
		}
		if !exists(filepath.Join(outside, "pwned")) || !exists(shmFile) {
			t.Fatal("uncontained run did not write outside the workdir; the fixture proves nothing")
		}
	})

	t.Run("read-write workdir", func(t *testing.T) {
		reset()
		_, _ = runHelper(t, "enter", spec)
		if exists(filepath.Join(outside, "pwned")) {
			t.Error("write outside the workdir succeeded")
		}
		if exists(shmFile) {
			t.Error("write through the inherited writable child mount succeeded")
		}
		if !exists(filepath.Join(workdir, "ok")) {
			t.Error("write inside the workdir failed")
		}
	})

	t.Run("read-only workdir", func(t *testing.T) {
		reset()
		ro := spec
		ro.Policy.ReadOnly = true
		_, _ = runHelper(t, "enter", ro)
		if exists(filepath.Join(workdir, "ok")) {
			t.Error("write inside a read-only workdir succeeded")
		}
		if exists(filepath.Join(outside, "pwned")) {
			t.Error("write outside the workdir succeeded")
		}
	})
}

func TestEnterLinuxDropsCapabilities(t *testing.T) {
	requireUserNamespaces(t)
	base := t.TempDir()
	workdir := filepath.Join(base, "work")
	home := homeOutsideTmp(t)
	for _, d := range []string{workdir} {
		if err := os.Mkdir(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	script := `while read k v; do case $k in CapEff:|CapPrm:) echo "$k $v";; esac; done < /proc/self/status
mount -o remount,rw "$WORKDIR" 2>/dev/null
if touch "$WORKDIR/x" 2>/dev/null; then echo WRITABLE; else echo READONLY; fi`
	env := baseEnv(workdir, base, home)
	spec := nsSpec(t, "/bin/sh", []string{"sh", "-c", script}, env, Policy{Profile: "inspect", Workdir: workdir, ReadOnly: true})

	zero := "0000000000000000"
	out, err := runHelper(t, "enter", spec)
	if err != nil {
		t.Fatalf("contained run: %v\n%s", err, out)
	}
	for _, line := range []string{"CapEff: " + zero, "CapPrm: " + zero} {
		if !strings.Contains(out, line) {
			t.Errorf("contained output lacks %q:\n%s", line, out)
		}
	}
	if !strings.Contains(out, "READONLY") {
		t.Errorf("remounting the workdir read-write made it writable:\n%s", out)
	}

	out, err = runHelper(t, "plain", spec)
	if err != nil {
		t.Fatalf("uncontained run: %v\n%s", err, out)
	}
	if strings.Contains(out, "CapEff: "+zero) || strings.Contains(out, "CapPrm: "+zero) {
		t.Fatalf("uncontained run also reports empty capabilities; the fixture proves nothing:\n%s", out)
	}
}

func TestEnterLinuxBindsAuthFile(t *testing.T) {
	requireUserNamespaces(t)
	base := t.TempDir()
	workdir := filepath.Join(base, "work")
	home := homeOutsideTmp(t)
	for _, d := range []string{workdir} {
		if err := os.Mkdir(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	secretPath := filepath.Join(t.TempDir(), "secret")
	const secret = "s3cr3t-bytes"
	if err := os.WriteFile(secretPath, []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	env := append(baseEnv(workdir, base, home), "SECRET_SRC="+secretPath)
	script := `cat "$HOME/token"; if [ -e "$SECRET_SRC" ]; then echo AMBIENT-REACHABLE; fi`
	p := Policy{
		Profile:   "restricted",
		Workdir:   workdir,
		AuthBinds: []AuthBind{{Source: secretPath, Target: filepath.Join(home, "token")}},
	}
	spec := nsSpec(t, "/bin/sh", []string{"sh", "-c", script}, env, p)

	out, err := runHelper(t, "enter", spec)
	if err != nil {
		t.Fatalf("contained run with bind: %v\n%s", err, out)
	}
	if out != secret {
		t.Fatalf("output = %q, want exactly the secret %q (and the ambient path unreachable)", out, secret)
	}

	spec.Policy.AuthBinds = nil
	out, err = runHelper(t, "enter", spec)
	if err == nil && strings.Contains(out, secret) {
		t.Fatalf("secret readable without the bind: %q", out)
	}
}

func TestEnterLinuxMasksAuthSource(t *testing.T) {
	requireUserNamespaces(t)
	base := t.TempDir()
	workdir := filepath.Join(base, "work")
	home := homeOutsideTmp(t)
	for _, d := range []string{workdir} {
		if err := os.Mkdir(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	// The source must sit outside /tmp, $HOME and the workdir, the only
	// places the boundary already hides or replaces.
	srcDir, err := os.MkdirTemp(".", "authsrc") //nolint:usetesting // must sit outside /tmp, which t.TempDir uses
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(srcDir) })
	srcDir, err = filepath.Abs(srcDir)
	if err != nil {
		t.Fatal(err)
	}
	const secret = "s3cr3t-bytes"
	source := filepath.Join(srcDir, "secret")
	if err := os.WriteFile(source, []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	env := append(baseEnv(workdir, base, home), "SECRET_SRC="+source)
	script := `cat "$HOME/token"; echo "|"; cat "$SECRET_SRC"`
	p := Policy{
		Profile:   "restricted",
		Workdir:   workdir,
		AuthBinds: []AuthBind{{Source: source, Target: filepath.Join(home, "token")}},
	}

	out, err := runHelper(t, "enter", nsSpec(t, "/bin/sh", []string{"sh", "-c", script}, env, p))
	if err != nil {
		t.Fatalf("contained run: %v\n%s", err, out)
	}
	if out != secret+"|\n" {
		t.Fatalf("output = %q, want the secret at the target and nothing at the original path", out)
	}
}

func TestEnterLinuxRejectsUnstableAuthSource(t *testing.T) {
	requireUserNamespaces(t)
	base := t.TempDir()
	workdir := filepath.Join(base, "work")
	home := homeOutsideTmp(t)
	for _, d := range []string{workdir} {
		if err := os.Mkdir(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	regular := filepath.Join(base, "regular")
	if err := os.WriteFile(regular, []byte("s3cr3t-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(regular, link); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, "dir")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}

	for name, source := range map[string]string{"terminal symlink": link, "directory": dir, "regular file": regular} {
		t.Run(name, func(t *testing.T) {
			p := Policy{
				Profile:   "restricted",
				Workdir:   workdir,
				AuthBinds: []AuthBind{{Source: source, Target: filepath.Join(home, "token")}},
			}
			spec := nsSpec(t, "/bin/sh", []string{"sh", "-c", `cat "$HOME/token"`}, baseEnv(workdir, base, home), p)
			out, err := runHelper(t, "enter", spec)
			if name == "regular file" {
				if err != nil || out != "s3cr3t-bytes" {
					t.Fatalf("regular file bind: err=%v out=%q", err, out)
				}
				return
			}
			if err == nil || strings.Contains(out, "s3cr3t-bytes") {
				t.Fatalf("%s source was bound: err=%v out=%q", name, err, out)
			}
		})
	}
}

func TestEnterLinuxRejectsInvalidLayout(t *testing.T) {
	requireUserNamespaces(t)
	base := t.TempDir()
	workdir := filepath.Join(base, "work")
	if err := os.Mkdir(workdir, 0o750); err != nil {
		t.Fatal(err)
	}
	inWork := filepath.Join(workdir, "secret")
	if err := os.WriteFile(inWork, []byte("s3cr3t-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	home := homeOutsideTmp(t)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		workdir string
		home    string
		binds   []AuthBind
	}{
		{"HOME beneath /tmp", workdir, filepath.Join(base, "home"), nil},
		{"HOME is /tmp", workdir, "/tmp", nil},
		{"HOME is an ancestor of /tmp", workdir, "/", nil},
		{"auth source inside the workdir", workdir, home, []AuthBind{{Source: inWork, Target: filepath.Join(home, "token")}}},
		{"workdir is HOME", home, home, nil},
		{"workdir is an ancestor of HOME", cwd, home, nil},
		{"workdir is /", "/", home, nil},
		{"workdir is /tmp", "/tmp", home, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := Policy{Profile: "restricted", Workdir: tc.workdir, AuthBinds: tc.binds}
			spec := nsSpec(t, "/bin/sh", []string{"sh", "-c", "echo ran"}, baseEnv(tc.workdir, base, tc.home), p)
			out, err := runHelper(t, "enter", spec)
			if err == nil || strings.Contains(out, "ran") {
				t.Fatalf("invalid layout was accepted: err=%v out=%q", err, out)
			}
		})
	}
}

func TestEnterLinuxResolvesSymlinksBeforeChecks(t *testing.T) {
	requireUserNamespaces(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	parent, err := os.MkdirTemp(cwd, "layout") //nolint:usetesting // must sit outside /tmp, which t.TempDir uses
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(parent) })
	realHome := filepath.Join(parent, "user")
	realWork := filepath.Join(parent, "work")
	inner := filepath.Join(realWork, "home")
	for _, d := range []string{realHome, realWork, inner} {
		if err := os.Mkdir(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	alias := filepath.Join(parent, "alias")
	if err := os.Symlink(parent, alias); err != nil {
		t.Fatal(err)
	}
	homeLink := filepath.Join(parent, "homelink")
	if err := os.Symlink(inner, homeLink); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()

	refused := []struct{ name, workdir, home string }{
		{"symlinked HOME resolves into the workdir", realWork, homeLink},
		{"workdir behind a symlinked ancestor resolves to HOME", filepath.Join(alias, "user"), realHome},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			p := Policy{Profile: "restricted", Workdir: tc.workdir}
			spec := nsSpec(t, "/bin/sh", []string{"sh", "-c", "echo ran"}, baseEnv(tc.workdir, base, tc.home), p)
			out, err := runHelper(t, "enter", spec)
			if err == nil || strings.Contains(out, "ran") {
				t.Fatalf("overlapping layout was accepted: err=%v out=%q", err, out)
			}
		})
	}

	t.Run("workdir behind a symlinked ancestor still works", func(t *testing.T) {
		workdir := filepath.Join(alias, "work")
		p := Policy{Profile: "restricted", Workdir: workdir}
		spec := nsSpec(t, "/bin/sh", []string{"sh", "-c", `touch "$WORKDIR/ok"`}, baseEnv(workdir, base, realHome), p)
		if out, err := runHelper(t, "enter", spec); err != nil {
			t.Fatalf("contained run: %v\n%s", err, out)
		}
		if _, err := os.Stat(filepath.Join(realWork, "ok")); err != nil {
			t.Fatalf("write through the resolved workdir failed: %v", err)
		}
	})
}
