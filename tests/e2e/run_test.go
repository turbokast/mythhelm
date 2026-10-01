package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/workers"
	"github.com/turbokast/mythhelm/internal/workspace"
)

// env isolates one packaged invocation: a private home, state dir and git
// environment. PATH is inherited so git resolves on every runner.
type env struct {
	home  string
	state string
	vars  []string
}

func newEnv(t *testing.T) env {
	t.Helper()
	e := env{home: t.TempDir(), state: t.TempDir()}
	e.vars = []string{
		"HOME=" + e.home, "USERPROFILE=" + e.home,
		"XDG_CONFIG_HOME=" + e.home, "XDG_STATE_HOME=" + e.home,
		"MYTHHELM_HOME=" + e.state, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=E2E", "GIT_AUTHOR_EMAIL=e2e@example.com",
		"GIT_COMMITTER_NAME=E2E", "GIT_COMMITTER_EMAIL=e2e@example.com",
		// No E2E check needs the network: every proxy points at the
		// closed discard port, so a network attempt fails loudly.
		"HTTP_PROXY=http://127.0.0.1:9/", "HTTPS_PROXY=http://127.0.0.1:9/",
		"ALL_PROXY=http://127.0.0.1:9/", "http_proxy=http://127.0.0.1:9/",
		"https_proxy=http://127.0.0.1:9/", "all_proxy=http://127.0.0.1:9/",
	}
	return e
}

func (e env) withParent(t *testing.T) []string {
	t.Helper()
	out := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return slices.Contains([]string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_STATE_HOME",
			"MYTHHELM_HOME", "GIT_CONFIG_NOSYSTEM", "GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL",
			"GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL",
			"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY",
			"http_proxy", "https_proxy", "all_proxy"}, name)
	})
	return append(out, e.vars...)
}

// mkRepo seeds a committed repository with the standard E2E task and a
// mythhelm.toml whose check re-executes the packaged binary. It returns
// the repo path and the config digest the run must trust.
func mkRepo(t *testing.T, e env, root, checkMode string) (repo, digest string) {
	t.Helper()
	repo = filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o750); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = e.withParent(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	git("init", "--quiet", "--initial-branch=main")
	git("config", "user.name", "E2E")
	git("config", "user.email", "e2e@example.com")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("e2e\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "task.md"), []byte("# E2E task\n\nWrite demo.txt.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := "schema_version = 1\n[[checks]]\nname = \"e2e\"\nargv = [" +
		strconv.Quote(mythhelmBin) + ", \"__demo-check\", " + strconv.Quote(checkMode) + "]\ntimeout = \"30s\"\n"
	if err := os.WriteFile(filepath.Join(repo, "mythhelm.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "README.md", "task.md", "mythhelm.toml")
	git("commit", "--quiet", "-m", "seed")
	_, digest, err := admission.ParseProjectConfig([]byte(config))
	if err != nil {
		t.Fatal(err)
	}
	return repo, digest
}

// run executes the packaged binary to completion.
func run(t *testing.T, e env, dir string, args ...string) (exit int, stdout, stderr string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, mythhelmBin, args...)
	cmd.Dir = dir
	cmd.Env = e.withParent(t)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	_ = cmd.Run()
	if cmd.ProcessState == nil {
		t.Fatalf("mythhelm %s did not run", strings.Join(args, " "))
	}
	return cmd.ProcessState.ExitCode(), out.String(), errOut.String()
}

// liveRun is a packaged process with streaming stdout.
type liveRun struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	lines  chan string
	stderr *bytes.Buffer
}

// startLive starts the packaged binary; waitLine and wait consume it.
func startLive(t *testing.T, e env, dir string, args ...string) *liveRun {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	cmd := exec.CommandContext(ctx, mythhelmBin, args...)
	cmd.Dir = dir
	cmd.Env = e.withParent(t)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	r := &liveRun{cmd: cmd, cancel: cancel, lines: make(chan string, 64), stderr: &bytes.Buffer{}}
	cmd.Stderr = r.stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		defer close(r.lines)
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 1024), 1<<20)
		for sc.Scan() {
			r.lines <- sc.Text()
		}
	}()
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})
	return r
}

// waitLine blocks until a stdout line matches, failing if stdout closes or
// two minutes pass first.
func (r *liveRun) waitLine(t *testing.T, what string, match func(string) bool) string {
	t.Helper()
	deadline := time.After(2 * time.Minute)
	for {
		select {
		case line, ok := <-r.lines:
			if !ok {
				t.Fatalf("stdout closed before %s", what)
			}
			if match(line) {
				return line
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// wait drains stdout and waits for the exit.
func (r *liveRun) wait(t *testing.T) (exit int, stdout []string, stderr string) {
	t.Helper()
	for line := range r.lines {
		stdout = append(stdout, line)
	}
	_ = r.cmd.Wait()
	r.cancel()
	if r.cmd.ProcessState == nil {
		t.Fatal("process did not exit")
	}
	return r.cmd.ProcessState.ExitCode(), stdout, r.stderr.String()
}

// runResultOf parses the run.result envelope from jsonl stdout, returning
// the envelope's run ID and its payload.
func runResultOf(t *testing.T, stdout string) (string, map[string]any) {
	t.Helper()
	for line := range strings.Lines(strings.TrimSpace(stdout)) {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) != nil {
			continue
		}
		if m["type"] == "run.result" {
			runID, _ := m["run_id"].(string)
			payload, _ := m["payload"].(map[string]any)
			return runID, payload
		}
	}
	t.Fatalf("no run.result in stdout:\n%s", stdout)
	return "", nil
}

// plainStateOf returns the state word from a plain `result:` line,
// stripping any parenthesised reason and cell padding.
func plainStateOf(t *testing.T, stdout []string) string {
	t.Helper()
	for _, line := range stdout {
		rest, ok := strings.CutPrefix(line, "result:")
		if !ok {
			continue
		}
		state, _, _ := strings.Cut(strings.TrimSpace(rest), ";")
		state, _, _ = strings.Cut(strings.TrimSpace(state), " (")
		return state
	}
	t.Fatalf("no result line in stdout:\n%s", strings.Join(stdout, "\n"))
	return ""
}

func fingerprint(t *testing.T, repo string) string {
	t.Helper()
	fp, err := workspace.SourceFingerprint(repo)
	if err != nil {
		t.Fatal(err)
	}
	return fp
}

func TestRunLeavesSourceCheckoutUntouched(t *testing.T) {
	e := newEnv(t)
	repo, digest := mkRepo(t, e, t.TempDir(), "pass")
	// The apply target already exists, so apply fails without writing.
	cmd := exec.Command("git", "branch", "taken")
	cmd.Dir = repo
	cmd.Env = e.withParent(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v\n%s", err, out)
	}
	before := fingerprint(t, repo)
	args := []string{"run", "--task-file", "task.md", "--adapter", "fake",
		"--billing", "local-scripted", "--execution-profile", "trusted-host",
		"--non-interactive", "--trust-project-config", "sha256:" + digest,
		"--format", "jsonl"}
	code, stdout, stderr := run(t, e, repo, args...)
	if code != 0 {
		t.Fatalf("run exit %d, stderr %q", code, stderr)
	}
	runID, res := runResultOf(t, stdout)
	if res["state"] != "ready_for_review" {
		t.Fatalf("run.result state = %v", res["state"])
	}
	if runID == "" {
		t.Fatal("run.result has no run_id")
	}
	if got := fingerprint(t, repo); got != before {
		t.Fatal("source checkout changed during run")
	}
	if code, _, stderr := run(t, e, repo, "review", runID); code != 0 {
		t.Fatalf("review exit %d, stderr %q", code, stderr)
	}
	if got := fingerprint(t, repo); got != before {
		t.Fatal("source checkout changed during review")
	}
	code, _, _ = run(t, e, repo, "apply", runID, "--to-branch", "taken")
	if code == 0 {
		t.Fatal("apply to an existing branch unexpectedly succeeded")
	}
	if got := fingerprint(t, repo); got != before {
		t.Fatal("source checkout changed during a failed apply")
	}
}

func TestE2EJSONLContract(t *testing.T) {
	e := newEnv(t)
	repo, digest := mkRepo(t, e, t.TempDir(), "fail")
	args := []string{"run", "--task-file", "task.md", "--adapter", "fake",
		"--billing", "local-scripted", "--execution-profile", "trusted-host",
		"--non-interactive", "--trust-project-config", "sha256:" + digest,
		"--format", "jsonl"}
	code, stdout, stderr := run(t, e, repo, args...)
	if code != 5 {
		t.Fatalf("run exit %d, stderr %q; want 5", code, stderr)
	}
	// Every stdout line is a strict envelope; the failure surfaces on
	// stderr, never as a non-envelope stdout line.
	var last journal.Event
	n := 0
	for line := range strings.Lines(strings.TrimSpace(stdout)) {
		dec := json.NewDecoder(strings.NewReader(line))
		dec.DisallowUnknownFields()
		var ev journal.Event
		if err := dec.Decode(&ev); err != nil {
			t.Fatalf("line %d is not an envelope: %v", n+1, err)
		}
		if ev.SchemaVersion != journal.EnvelopeVersion || ev.EventID == "" ||
			ev.RunID == "" || ev.ProducerID == "" || ev.Type == "" {
			t.Fatalf("line %d breaks the envelope contract: %q", n+1, line)
		}
		last, n = ev, n+1
	}
	if n == 0 || last.Type != "run.result" {
		t.Fatalf("stdout has %d lines ending in %q, want envelopes ending in run.result", n, last.Type)
	}
	var payload struct {
		ExitCode int `json:"exit_code"`
	}
	if err := json.Unmarshal(last.Payload, &payload); err != nil || payload.ExitCode != code {
		t.Fatalf("run.result exit_code = %+v (err %v), process exited %d", payload, err, code)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Fatal("stderr is empty; the failure must surface there")
	}
}

func TestE2EPathsWithSpacesAndUnicode(t *testing.T) {
	e := newEnv(t)
	root := filepath.Join(t.TempDir(), "e2e spaced ✓ dir")
	repo, digest := mkRepo(t, e, root, "pass")
	// The task file itself carries spaces and unicode, committed so the
	// checkout stays clean.
	task := filepath.Join(repo, "my task ✓.md")
	if err := os.WriteFile(task, []byte("# E2E task\n\nWrite demo.txt.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "my task ✓.md"}, {"commit", "--quiet", "-m", "unicode task"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = e.withParent(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	args := []string{"run", "--task-file", task, "--adapter", "fake",
		"--billing", "local-scripted", "--execution-profile", "trusted-host",
		"--non-interactive", "--trust-project-config", "sha256:" + digest}
	code, _, stderr := run(t, e, repo, args...)
	if code != 0 {
		t.Fatalf("run exit %d, stderr %q", code, stderr)
	}
}

func TestE2EKillCLIWorkerSurvivesThenRecover(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGKILL/SIGINT process semantics are Unix-only; Windows recovery is covered by unit tests")
	}
	e := newEnv(t)
	repo, digest := mkRepo(t, e, t.TempDir(), "pass")
	args := []string{"run", "--task-file", "task.md", "--adapter", "fake",
		"--billing", "local-scripted", "--execution-profile", "trusted-host",
		"--non-interactive", "--trust-project-config", "sha256:" + digest,
		"--format", "jsonl", "--scenario", "slow"}
	r := startLive(t, e, repo, args...)
	session := r.waitLine(t, "the native session", func(line string) bool {
		return strings.Contains(line, `"type":"attempt.native_session"`)
	})
	var ev journal.Event
	if err := json.Unmarshal([]byte(session), &ev); err != nil {
		t.Fatal(err)
	}
	runID := ev.RunID
	// The CLI dies; the detached worker must not.
	if err := r.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_, _, _ = r.wait(t)
	j, err := journal.OpenReadOnly(context.Background(), e.state)
	if err != nil {
		t.Fatal(err)
	}
	a, err := j.LatestAttempt(context.Background(), runID)
	_ = j.Close()
	if err != nil {
		t.Fatal(err)
	}
	id, err := workers.ReadIdentity(workers.AttemptDir(e.state, runID, a.AttemptID))
	if err != nil {
		t.Fatal(err)
	}
	proc, err := os.FindProcess(id.PID)
	if err != nil {
		t.Fatal(err)
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("worker pid %d did not survive the CLI: %v", id.PID, err)
	}
	// If the test fails before cancellation, reap the detached worker
	// and its slow native (10-minute sleep) by pid. No process-group
	// kill: the native owns a separate group, so a group kill would
	// miss it and could hit an unrelated group.
	var native *os.Process
	if id.NativePID != nil {
		native, _ = os.FindProcess(*id.NativePID)
	}
	t.Cleanup(func() {
		_ = proc.Kill()
		if native != nil {
			_ = native.Kill()
		}
	})
	// Recovery reattaches to the live worker; interrupting it stops the
	// slow native and cancels the run without relaunching anything.
	rec := startLive(t, e, repo, "recover", runID)
	reattached := rec.waitLine(t, "the reattach notice", func(line string) bool {
		return strings.Contains(line, fmt.Sprintf("reattached to worker pid %d", id.PID))
	})
	_ = reattached
	if err := rec.cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := rec.wait(t)
	if code != 130 {
		t.Fatalf("recover exit %d, want 130", code)
	}
	if state := plainStateOf(t, stdout); state != "cancelled" {
		t.Fatalf("recovered state = %q, want cancelled", state)
	}
}
