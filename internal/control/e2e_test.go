//go:build linux || darwin

package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

var (
	e2eOnce sync.Once
	e2eDir  string
	e2eBin  string
	e2eErr  error
)

// buildBinary builds the packaged mythhelm binary once for the package.
func buildBinary(t *testing.T) string {
	t.Helper()
	e2eOnce.Do(func() {
		_, file, _, _ := runtime.Caller(0)
		root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
		e2eDir, e2eErr = os.MkdirTemp("", "mythhelm-control-e2e-") //nolint:usetesting // the binary outlives the test that builds it; TestMain removes it
		if e2eErr != nil {
			return
		}
		e2eBin = filepath.Join(e2eDir, "mythhelm")
		cmd := exec.Command("go", "build", "-o", e2eBin, "./cmd/mythhelm")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			e2eErr = errors.New("building mythhelm: " + err.Error() + "\n" + string(out))
		}
	})
	if e2eErr != nil {
		t.Fatal(e2eErr)
	}
	return e2eBin
}

func cleanupE2EBinary() {
	if e2eDir != "" {
		_ = os.RemoveAll(e2eDir)
	}
}

// e2eHost is an isolated state root and runtime directory for one test's
// supervisor. The runtime directory is short: it holds a Unix socket.
type e2eHost struct {
	t    *testing.T
	bin  string
	home string
	rt   string
}

func newE2EHost(t *testing.T) *e2eHost {
	t.Helper()
	bin := buildBinary(t)
	rt, err := os.MkdirTemp("", "mh") //nolint:usetesting // t.TempDir is too long for a Unix socket path on macOS
	if err != nil {
		t.Fatal(err)
	}
	h := &e2eHost{t: t, bin: bin, home: t.TempDir(), rt: rt}
	t.Cleanup(func() {
		h.killSupervisor()
		_ = os.RemoveAll(rt)
	})
	return h
}

func (h *e2eHost) run(args ...string) (stdout, stderr string, code int) {
	h.t.Helper()
	cmd := exec.Command(h.bin, args...)
	cmd.Env = []string{"MYTHHELM_HOME=" + h.home, "XDG_RUNTIME_DIR=" + h.rt, "HOME=" + h.home, "TMPDIR=" + h.rt}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		code = exit.ExitCode()
	} else if err != nil {
		h.t.Fatalf("running mythhelm %v: %v", args, err)
	}
	return out.String(), errb.String(), code
}

type status struct {
	PID        int    `json:"pid"`
	Generation uint64 `json:"generation"`
	Root       string `json:"root"`
	Endpoint   string `json:"endpoint"`
}

func (h *e2eHost) status() status {
	h.t.Helper()
	out, stderr, code := h.run("supervisor", "status", "--format", "jsonl")
	if code != 0 {
		h.t.Fatalf("supervisor status exited %d: %s", code, stderr)
	}
	var st status
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		h.t.Fatalf("status output %q: %v", out, err)
	}
	return st
}

// killSupervisor stops whatever supervisor this host started.
func (h *e2eHost) killSupervisor() {
	matches, _ := filepath.Glob(filepath.Join(h.rt, "mythhelm-*", "supervisor.lock"))
	for _, lock := range matches {
		raw, err := os.ReadFile(filepath.Clean(lock))
		if err != nil {
			continue
		}
		var meta lockMetadata
		if json.Unmarshal(raw, &meta) == nil && meta.PID > 0 {
			_ = syscall.Kill(meta.PID, syscall.SIGKILL)
		}
	}
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func TestLazySpawnThenStatus(t *testing.T) {
	t.Parallel()
	h := newE2EHost(t)
	out, stderr, code := h.run("supervisor", "status")
	if code != 0 {
		t.Fatalf("supervisor status exited %d with no supervisor running: %s", code, stderr)
	}
	root, err := filepath.EvalSymlinks(h.home)
	if err != nil {
		t.Fatal(err)
	}
	st := h.status() // a client of the supervisor the first call started
	if st.PID <= 0 || !alive(st.PID) || st.Generation != 1 || st.Root != root || filepath.Base(st.Endpoint) != "control.sock" {
		t.Fatalf("status = %+v, want a live pid, generation 1, root %s and the control socket", st, root)
	}
	for _, want := range []string{"pid:        " + strconv.Itoa(st.PID), "generation: 1", "root:       " + root, "endpoint:   " + st.Endpoint} {
		if !strings.Contains(out, want) {
			t.Errorf("plain status %q lacks %q", out, want)
		}
	}
}

func TestConcurrentSpawnBecomesClient(t *testing.T) {
	t.Parallel()
	h := newE2EHost(t)
	const clients = 5
	pids := make([]int, clients)
	var wg sync.WaitGroup
	for i := range clients {
		wg.Go(func() {
			out, stderr, code := h.run("supervisor", "status", "--format", "jsonl")
			if code != 0 || strings.Contains(stderr, "already running") {
				t.Errorf("client %d: exit %d, stderr %q", i, code, stderr)
				return
			}
			var st status
			if err := json.Unmarshal([]byte(out), &st); err != nil {
				t.Errorf("client %d: %v", i, err)
				return
			}
			pids[i] = st.PID
		})
	}
	wg.Wait()
	for i, pid := range pids {
		if pid == 0 || pid != pids[0] {
			t.Fatalf("client %d saw supervisor pid %d, want everyone on %d: %v", i, pid, pids[0], pids)
		}
	}
}

func TestStatusJsonlCarriesInstanceState(t *testing.T) {
	t.Parallel()
	h := newE2EHost(t)
	out, stderr, code := h.run("supervisor", "status", "--format", "jsonl")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Fatalf("jsonl status has %d lines, want one object: %q", len(lines), out)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &obj); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"pid", "generation", "root", "endpoint"} {
		if v, ok := obj[key]; !ok || v == "" || v == nil {
			t.Errorf("jsonl status %s lacks %q", lines[0], key)
		}
	}
}

func TestSupervisorSurvivesClientExit(t *testing.T) { // I06 (v2 §2)
	t.Parallel()
	h := newE2EHost(t)
	first := h.status() // the client that started the supervisor has now exited
	if !alive(first.PID) {
		t.Fatalf("supervisor %d died with its first client", first.PID)
	}
	// Detached: the supervisor leads its own session, so nothing sent to the
	// client's session or process group reaches it.
	sid, err := unix.Getsid(first.PID)
	if err != nil {
		t.Fatal(err)
	}
	ours, err := unix.Getsid(0)
	if err != nil {
		t.Fatal(err)
	}
	if sid != first.PID || sid == ours {
		t.Fatalf("supervisor %d is in session %d (ours %d), want its own session", first.PID, sid, ours)
	}
	second := h.status()
	if second.PID != first.PID || second.Generation != first.Generation {
		t.Fatalf("a later client saw %+v, want the same supervisor as %+v and no restart", second, first)
	}
}

func TestSupervisorStatusUsage(t *testing.T) {
	t.Parallel()
	h := newE2EHost(t)
	if _, _, code := h.run("supervisor"); code != exitInvalid {
		t.Errorf("supervisor with no subcommand exited %d, want a usage error", code)
	}
	if _, _, code := h.run("supervisor", "status", "--format", "xml"); code != exitInvalid {
		t.Errorf("status with a bad format exited %d, want a usage error", code)
	}
}

// exitInvalid is the exit code of an invalid command line; control must not
// import the cli package that defines it.
const exitInvalid = 2
