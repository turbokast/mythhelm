package supervisor_test

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/adapters/fake"
	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/cli"
	"github.com/turbokast/mythhelm/internal/contain"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workers"
	"github.com/turbokast/mythhelm/internal/workspace"
)

// cliEnv switches the test binary into `mythhelm` itself, so a test runs the
// real command line as a child process that it can signal.
const cliEnv = "MYTHHELM_TEST_CLI"

// TestMain lets the test binary play every process of a run: the CLI, the
// worker (__worker) and the fake agent (__fake-agent).
func TestMain(m *testing.M) {
	// A copy of the test binary installed as `claude` on PATH plays the
	// native for claudecode runs. Its behaviour comes from the
	// fakeclaude.json sidecar in its own directory: PATH and the child
	// environment cannot carry fixture selectors through the allowlist.
	if base := filepath.Base(os.Args[0]); base == "claude" || base == "claude.exe" {
		os.Exit(fakeClaudeMain())
	}
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "__check":
			switch os.Args[2] {
			case "pass":
				os.Exit(0)
			case "fail":
				os.Exit(1)
			case "output":
				fmt.Print("not formatted\n")
				os.Exit(0)
			case "marker":
				if err := os.WriteFile(os.Args[3], []byte("ran"), 0o600); err != nil { //nolint:gosec // G703: the test's own marker path
					os.Exit(3)
				}
				os.Exit(0)
			}
			os.Exit(2)
		case workers.Command:
			os.Exit(workers.Main(os.Args[2:]))
		case fake.AgentCommand:
			os.Exit(fake.AgentMain(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
		}
	}
	if os.Getenv(cliEnv) == "1" {
		os.Exit(cli.Main(os.Args[1:], cli.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}))
	}
	// Every re-executed -race child otherwise sleeps a second before exiting.
	if err := os.Setenv("GORACE", "atexit_sleep_ms=0"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// fixture is a user repository, a task file and an empty state directory.
type fixture struct {
	state, repo, task, home string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	f := fixture{state: t.TempDir(), repo: t.TempDir(), home: t.TempDir()}
	f.task = filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(f.task, []byte("# Demo task\n\nWrite demo.txt.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.git(t, "init", "--quiet", "--initial-branch=main")
	f.git(t, "config", "user.name", "Test")
	f.git(t, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(f.repo, "README.md"), []byte("demo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.git(t, "add", "README.md")
	f.git(t, "commit", "--quiet", "-m", "initial")
	return f
}

func (f fixture) git(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.repo
	cmd.Env = append(os.Environ(), "HOME="+f.home, "XDG_CONFIG_HOME="+f.home, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// command builds `mythhelm args...` run in the repository against the
// fixture's state directory.
func (f fixture) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], args...) //nolint:gosec // G702: the test binary itself, with the test's own argv
	cmd.Dir = f.repo
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		return strings.HasPrefix(kv, "MYTHHELM_HOME=") || strings.HasPrefix(kv, "HOME=") || strings.HasPrefix(kv, "XDG_CONFIG_HOME=")
	})
	env = append(env, cliEnv+"=1", "MYTHHELM_HOME="+f.state, "HOME="+f.home, "XDG_CONFIG_HOME="+f.home)
	cmd.Env = env
	cmd.WaitDelay = 10 * time.Second
	return cmd
}

// run runs `mythhelm args...` to completion with empty stdin.
func (f fixture) run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cmd := f.command(ctx, args...)
	var out, errOut bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = strings.NewReader(""), &out, &errOut
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
	default:
		t.Fatalf("mythhelm %s: %v", strings.Join(args, " "), err)
	}
	if code != 0 {
		f.logWorkerDiagnostics(t)
	}
	return code, out.String(), errOut.String()
}

// logWorkerDiagnostics preserves the redacted worker log when a fixture run
// fails; the summary's launch_failed label alone cannot identify the cause.
// t.Logf keeps passing runs silent and annotates failing ones.
func (f fixture) logWorkerDiagnostics(t *testing.T) {
	t.Helper()
	logs, err := filepath.Glob(filepath.Join(f.state, "runs", "*", "attempts", "*", "worker.log"))
	if err != nil {
		// A diagnostic helper must never fail the test it annotates.
		t.Logf("worker diagnostic unavailable: %v", err)
		return
	}
	for _, path := range logs {
		file, err := os.Open(path) // #nosec G304 -- test-owned worker diagnostics
		if err != nil {
			t.Logf("worker diagnostic unavailable: %v", err)
			continue
		}
		raw, readErr := io.ReadAll(io.LimitReader(file, 64<<10))
		_ = file.Close()
		t.Logf("redacted worker diagnostic: %s (read error: %v)", raw, readErr)
	}
}

// fakeRun is a complete fake-adapter command line; extra flags are appended.
func (f fixture) fakeRun(extra ...string) []string {
	return append([]string{"run", "--task-file", f.task, "--adapter", "fake", "--billing", "local-scripted",
		"--execution-profile", "trusted-host", "--non-interactive", "--no-checks"}, extra...)
}

func (f fixture) config(t *testing.T, mode string) string {
	t.Helper()
	contents := "schema_version = 1\n[[checks]]\nname = \"test\"\nargv = [" + strconv.Quote(os.Args[0]) + ", \"__check\", " + strconv.Quote(mode) + "]\ntimeout = \"5s\"\n"
	if err := os.WriteFile(filepath.Join(f.repo, "mythhelm.toml"), []byte(contents), 0o600); err != nil { // #nosec G703 -- fixture repo is t.TempDir
		t.Fatal(err)
	}
	f.git(t, "add", "mythhelm.toml")
	f.git(t, "commit", "--quiet", "-m", "add checks")
	_, digest, err := admission.ParseProjectConfig([]byte(contents))
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func (f fixture) checkedRun(digest string, extra ...string) []string {
	args := slices.DeleteFunc(f.fakeRun(), func(s string) bool { return s == "--no-checks" })
	return append(args, append([]string{"--trust-project-config", "sha256:" + digest}, extra...)...)
}

func TestUnknownTOMLKeyExits2(t *testing.T) {
	f := newFixture(t)
	if err := os.WriteFile(filepath.Join(f.repo, "mythhelm.toml"), []byte("schema_version = 1\nunknown = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.git(t, "add", "mythhelm.toml")
	f.git(t, "commit", "--quiet", "-m", "bad config")
	args := slices.DeleteFunc(f.fakeRun(), func(s string) bool { return s == "--no-checks" })
	code, _, stderr := f.run(t, args...)
	if code != 2 || !strings.Contains(stderr, "unknown key") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestUntrustedChecksNonInteractiveBlocked(t *testing.T) {
	f := newFixture(t)
	digest := f.config(t, "pass")
	args := slices.DeleteFunc(f.fakeRun(), func(s string) bool { return s == "--no-checks" })
	code, _, stderr := f.run(t, args...)
	if code != 3 || !strings.Contains(stderr, "untrusted_project_config") || !strings.Contains(stderr, digest) {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestProjectConfigTrustBoundToDigest(t *testing.T) {
	f := newFixture(t)
	digest := f.config(t, "pass")
	if code, _, stderr := f.run(t, f.checkedRun(digest)...); code != 0 {
		t.Fatalf("first exit %d: %s", code, stderr)
	}
	args := slices.DeleteFunc(f.fakeRun(), func(s string) bool { return s == "--no-checks" })
	if code, _, stderr := f.run(t, args...); code != 0 {
		t.Fatalf("stored grant exit %d: %s", code, stderr)
	}
	if err := os.WriteFile(filepath.Join(f.repo, "mythhelm.toml"), []byte("schema_version = 1\n[[checks]]\nname = \"new\"\nargv = [\"go\", \"version\"]\ntimeout = \"1s\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.git(t, "add", "mythhelm.toml")
	f.git(t, "commit", "--quiet", "-m", "change checks")
	if code, _, stderr := f.run(t, args...); code != 3 || !strings.Contains(stderr, "untrusted_project_config") {
		t.Fatalf("changed digest exit %d: %s", code, stderr)
	}
}

func TestNativeSuccessChecksPassExit0ReadyForReview(t *testing.T) {
	f := newFixture(t)
	digest := f.config(t, "pass")
	code, _, stderr := f.run(t, f.checkedRun(digest)...)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	run := f.onlyRun(t)
	if run.State != "ready_for_review" || run.Reason != "" {
		t.Fatalf("run %s/%s", run.State, run.Reason)
	}
	if typeIndex(f.events(t, run.RunID), "verification.completed") < 0 {
		t.Fatal("no verification result event")
	}
}

func TestNativeSuccessChecksFailExit5(t *testing.T) {
	f := newFixture(t)
	digest := f.config(t, "fail")
	code, _, stderr := f.run(t, f.checkedRun(digest)...)
	if code != 5 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	run := f.onlyRun(t)
	if run.State != "failed" || run.Reason != "verification_failed" {
		t.Fatalf("run %s/%s", run.State, run.Reason)
	}
}

func TestMissingExecutableUnavailableExit5(t *testing.T) {
	f := newFixture(t)
	contents := "schema_version = 1\n[[checks]]\nname = \"missing\"\nargv = [\"mythhelm-nonexistent-check-executable\"]\ntimeout = \"1s\"\n"
	if err := os.WriteFile(filepath.Join(f.repo, "mythhelm.toml"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	f.git(t, "add", "mythhelm.toml")
	f.git(t, "commit", "--quiet", "-m", "missing check")
	_, digest, err := admission.ParseProjectConfig([]byte(contents))
	if err != nil {
		t.Fatal(err)
	}
	code, _, stderr := f.run(t, f.checkedRun(digest)...)
	if code != 5 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	run := f.onlyRun(t)
	if run.State != "failed" || run.Reason != "verification_failed" {
		t.Fatalf("run %s/%s", run.State, run.Reason)
	}
}

func TestNoChecksExit5Unverified(t *testing.T) {
	f := newFixture(t)
	code, stdout, stderr := f.run(t, f.fakeRun()...)
	if code != 5 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if !strings.Contains(stdout, "verification: NOT RUN (waived by --no-checks)") {
		t.Fatalf("plain output lacks NOT RUN: %s", stdout)
	}
	run := f.onlyRun(t)
	if run.State != "ready_for_review" || run.Reason != "unverified" {
		t.Fatalf("run %s/%s", run.State, run.Reason)
	}
}

func (f fixture) journal(t *testing.T) *journal.Journal {
	t.Helper()
	j, err := journal.Open(t.Context(), f.state)
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return j
}

// runs lists the recorded runs, or none when no database exists.
func (f fixture) runs(t *testing.T) []journal.RunRow {
	t.Helper()
	j, err := journal.OpenReadOnly(t.Context(), f.state)
	if errors.Is(err, journal.ErrNoDatabase) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	rows, err := j.ListRuns(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func (f fixture) onlyRun(t *testing.T) journal.RunRow {
	t.Helper()
	rows := f.runs(t)
	if len(rows) != 1 {
		t.Fatalf("recorded runs = %d, want 1", len(rows))
	}
	return rows[0]
}

func (f fixture) events(t *testing.T, runID string) []journal.Event {
	t.Helper()
	evs, err := f.journal(t).Events(t.Context(), runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func (f fixture) fingerprint(t *testing.T) string {
	t.Helper()
	fp, err := workspace.SourceFingerprint(f.repo)
	if err != nil {
		t.Fatal(err)
	}
	return fp
}

func typeIndex(evs []journal.Event, typ string) int {
	return slices.IndexFunc(evs, func(ev journal.Event) bool { return ev.Type == typ })
}

func payloadOf(t *testing.T, ev journal.Event) map[string]any {
	t.Helper()
	var p map[string]any
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		t.Fatalf("%s payload: %v", ev.Type, err)
	}
	return p
}

func TestRunNoAdapterFlagExits2(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	base := []string{"run", "--task-file", f.task, "--billing", "local-scripted", "--execution-profile", "trusted-host", "--non-interactive"}
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing adapter", args: base, want: "--adapter is required"},
		{name: "unknown adapter is not a fallback", args: append(slices.Clone(base), "--adapter", "codex"), want: `--adapter must be claudecode or fake, got "codex"`},
		{name: "missing billing", args: []string{"run", "--task-file", f.task, "--adapter", "fake", "--non-interactive"}, want: "--billing is required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := f.run(t, tt.args...)
			if code != 2 || !strings.Contains(stderr, tt.want) {
				t.Fatalf("exit %d, stderr %q; want exit 2 naming %q", code, stderr, tt.want)
			}
		})
	}
	if rows := f.runs(t); len(rows) != 0 {
		t.Fatalf("an invalid command line recorded %d runs", len(rows))
	}
}

// TestInspectProfileAdmission pins inspect's admission: admitted where its
// boundary is recorded (Linux), an exit-7 refusal everywhere else.
func TestInspectProfileAdmission(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	args := slices.DeleteFunc(f.fakeRun(), func(a string) bool { return a == "trusted-host" || a == "--execution-profile" })
	code, _, stderr := f.run(t, append(args, "--execution-profile", "inspect")...)
	if runtime.GOOS == "linux" {
		if !strings.Contains(stderr, "ended ready_for_review") {
			t.Fatalf("exit %d, stderr %q; want inspect admitted and run", code, stderr)
		}
		return
	}
	if code != 7 || !strings.Contains(stderr, "execution_profile_unavailable") {
		t.Fatalf("exit %d, stderr %q; want exit 7 execution_profile_unavailable", code, stderr)
	}
	if rows := f.runs(t); len(rows) != 0 {
		t.Fatalf("a refused profile recorded %d runs", len(rows))
	}
}

// TestMissingProfileConsentNonInteractiveExit3 pins the empty-flag default:
// restricted, with no consent question and never host authority (AC-1.2,
// AC-1.3). Where the boundary is recorded the run is admitted; elsewhere it
// is an exit-7 refusal naming each missing dimension.
func TestMissingProfileConsentNonInteractiveExit3(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	args := slices.DeleteFunc(f.fakeRun(), func(a string) bool { return a == "trusted-host" || a == "--execution-profile" })
	code, _, stderr := f.run(t, args...)
	if strings.Contains(stderr, "consent_required") {
		t.Fatalf("stderr %q: the empty flag asked for consent", stderr)
	}
	if runtime.GOOS == "linux" {
		if !strings.Contains(stderr, "ended ready_for_review") {
			t.Fatalf("exit %d, stderr %q; want the restricted default admitted and run", code, stderr)
		}
		return
	}
	if code != 7 || !strings.Contains(stderr, "execution_profile_unavailable") {
		t.Fatalf("exit %d, stderr %q; want exit 7 execution_profile_unavailable", code, stderr)
	}
	for _, dim := range []string{"filesystem", "process", "network", "credential"} {
		if !strings.Contains(stderr, dim) {
			t.Errorf("stderr %q does not name %s", stderr, dim)
		}
	}
}

func TestHostHerdrExit7(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	code, _, stderr := f.run(t, f.fakeRun("--host", "herdr")...)
	if code != 7 || !strings.Contains(stderr, "host_unavailable") {
		t.Fatalf("exit %d, stderr %q; want exit 7 host_unavailable", code, stderr)
	}
	if rows := f.runs(t); len(rows) != 0 {
		t.Fatalf("--host herdr recorded %d runs", len(rows))
	}
}

func TestDirtyCheckoutNonInteractiveBlocked(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := os.WriteFile(filepath.Join(f.repo, "scratch.txt"), []byte("uncommitted\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	head := f.git(t, "rev-parse", "HEAD")
	before := f.fingerprint(t)

	code, _, stderr := f.run(t, f.fakeRun()...)
	if code != 3 || !strings.Contains(stderr, "dirty_checkout") || !strings.Contains(stderr, "--use-committed") {
		t.Fatalf("non-interactive: exit %d, stderr %q; want exit 3 naming --use-committed", code, stderr)
	}
	interactive := slices.DeleteFunc(f.fakeRun(), func(a string) bool { return a == "--non-interactive" })
	code, _, stderr = f.run(t, interactive...) // stdin is empty: the prompt reads no
	if code != 3 || !strings.Contains(stderr, "Run on the committed revision") {
		t.Fatalf("declined prompt: exit %d, stderr %q; want the prompt and exit 3", code, stderr)
	}
	if rows := f.runs(t); len(rows) != 0 {
		t.Fatalf("a blocked dirty checkout recorded %d runs", len(rows))
	}

	code, _, stderr = f.run(t, f.fakeRun("--use-committed")...)
	if code != 5 {
		t.Fatalf("--use-committed: exit %d, stderr %q; want the run to proceed to exit 5 (verification unavailable)", code, stderr)
	}
	run := f.onlyRun(t)
	if run.BaseRev != head {
		t.Fatalf("base_rev = %s, want HEAD %s", run.BaseRev, head)
	}
	clone := filepath.Join(f.state, "runs", run.RunID, "workspace")
	if _, err := os.Stat(filepath.Join(clone, "scratch.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the uncommitted file reached the snapshot: stat err %v", err)
	}
	if after := f.fingerprint(t); after != before {
		t.Fatalf("the source checkout changed: fingerprint %s -> %s", before, after)
	}
}

func TestRunSecondActiveRunBlocked(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	j := f.journal(t)
	p := supervisor.NewProducer(ids.New("sup"), 1)
	active := ids.New("run")
	if err := supervisor.CreateRun(t.Context(), j, journal.RunRow{RunID: active, AdapterID: "builtin/fake", SourceRepo: f.repo,
		TaskSHA256: strings.Repeat("a", 64), BillingPosture: "local-scripted", ExecutionProfile: "trusted-host"}, p); err != nil {
		t.Fatal(err)
	}
	for _, s := range []supervisor.RunState{supervisor.RunAdmission, supervisor.RunExecuting} {
		if err := supervisor.TransitionRun(t.Context(), j, active, s, "", p); err != nil {
			t.Fatal(err)
		}
	}

	code, _, stderr := f.run(t, f.fakeRun()...)
	if code != 3 || !strings.Contains(stderr, "active_run_exists") || !strings.Contains(stderr, active) {
		t.Fatalf("exit %d, stderr %q; want exit 3 naming active run %s", code, stderr, active)
	}
	var blocked journal.RunRow
	for _, r := range f.runs(t) {
		if r.RunID != active {
			blocked = r
		}
	}
	if blocked.State != "blocked" || blocked.Reason != "active_run_exists" {
		t.Fatalf("second run projected %s/%s, want blocked/active_run_exists", blocked.State, blocked.Reason)
	}
	if evs := f.events(t, blocked.RunID); typeIndex(evs, "admission.decided") >= 0 || typeIndex(evs, "attempt.launch_intent_recorded") >= 0 {
		t.Fatalf("the blocked run went on to admission or launch: %v", evs)
	}

	// Once the first run is no longer active, the same command is admitted.
	if err := supervisor.TransitionRun(t.Context(), j, active, supervisor.RunFailed, "native_failed", p); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := f.run(t, f.fakeRun()...); code != 5 {
		t.Fatalf("after the active run ended: exit %d, stderr %q; want the run to proceed to exit 5", code, stderr)
	}
}

func TestAdmissionDecidedBeforeWorkerSpawn(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	before := f.fingerprint(t)
	code, stdout, stderr := f.run(t, f.fakeRun("--scenario", "happy")...)
	if code != 5 {
		t.Fatalf("exit %d, stdout %q, stderr %q; want 5", code, stdout, stderr)
	}
	run := f.onlyRun(t)
	evs := f.events(t, run.RunID)
	decided, intent, launched := typeIndex(evs, "admission.decided"), typeIndex(evs, "attempt.launch_intent_recorded"), typeIndex(evs, "attempt.launched")
	if decided < 0 || intent <= decided || launched <= intent {
		t.Fatalf("journal order admission.decided=%d launch_intent_recorded=%d launched=%d, want strictly increasing and present", decided, intent, launched)
	}
	adm := payloadOf(t, evs[decided])
	if adm["snapshot"].(map[string]any)["base_rev"] != f.git(t, "rev-parse", "HEAD") || adm["billing"].(map[string]any)["mode"] != "local-scripted" {
		t.Fatalf("admission.decided does not resolve the snapshot and billing posture: %s", evs[decided].Payload)
	}
	if !strings.Contains(stdout, "SCRIPTED: fake adapter, no real agent") || !strings.Contains(stdout, "not contained") {
		t.Fatalf("plain output does not disclose the scripted adapter and the uncontained profile:\n%s", stdout)
	}
	// The agent's edit lands in the snapshot clone, never in the checkout (I08).
	if b, err := os.ReadFile(filepath.Join(f.state, "runs", run.RunID, "workspace", "demo.txt")); err != nil || string(b) != "ok\n" {
		t.Fatalf("demo.txt in the clone = %q, %v; want the agent's edit", b, err)
	}
	if after := f.fingerprint(t); after != before {
		t.Fatalf("the source checkout changed: fingerprint %s -> %s", before, after)
	}
}

func TestRunOutcomeExitCodes(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		scenario            string
		code                int
		state, reason       string
		attempt, attemptWhy string
		category            string
	}{
		{scenario: "happy", code: 5, state: "ready_for_review", reason: "unverified", attempt: "succeeded_native", category: "verification_unavailable"},
		{scenario: "native-fails", code: 4, state: "failed", reason: "native_failed", attempt: "failed_native", attemptWhy: "error_during_execution", category: "native_failed"},
		{scenario: "exit-before-result", code: 4, state: "failed", reason: "native_failed", attempt: "failed_native", attemptWhy: "result_unobserved", category: "native_failed"},
	} {
		t.Run(tt.scenario, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			code, stdout, stderr := f.run(t, f.fakeRun("--scenario", tt.scenario, "--format", "jsonl")...)
			if code != tt.code {
				t.Fatalf("exit %d, stderr %q; want %d", code, stderr, tt.code)
			}
			run := f.onlyRun(t)
			if run.State != tt.state || run.Reason != tt.reason {
				t.Fatalf("run projected %s/%s, want %s/%s", run.State, run.Reason, tt.state, tt.reason)
			}
			res := lastResult(t, stdout)
			if res["exit_code"] != float64(tt.code) || res["error_category"] != tt.category || res["state"] != tt.state {
				t.Fatalf("run.result = %v, want exit %d %s state %s", res, tt.code, tt.category, tt.state)
			}
			var why any
			if tt.attemptWhy != "" {
				why = tt.attemptWhy
			}
			if res["attempt_reason"] != why {
				t.Fatalf("run.result attempt_reason = %v, want %v", res["attempt_reason"], why)
			}
		})
	}
}

func lastResult(t *testing.T, stdout string) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	var ev journal.Event
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &ev); err != nil || ev.Type != "run.result" {
		t.Fatalf("last stdout line %q is not run.result (%v)", lines[len(lines)-1], err)
	}
	return payloadOf(t, ev)
}

// documentedEvents is design §5's event set, which is all run may print.
var documentedEvents = []string{
	"run.created", "run.state_changed", "admission.decided", "workspace.snapshot_created",
	"attempt.launch_intent_recorded", "attempt.launched", "attempt.native_session", "attempt.progress",
	"attempt.permission_denied", "attempt.native_result", "attempt.stop_requested", "attempt.stopped",
	"attempt.state_changed", "attempt.protocol_counters", "candidate.frozen", "verification.started",
	"check.completed", "verification.completed", "receipt.written", "apply.intent_recorded", "apply.completed", "run.result",
}

// decodeEnvelopes parses every stdout line strictly as an Event.
func decodeEnvelopes(stdout string) ([]journal.Event, error) {
	var out []journal.Event
	for i, line := range strings.Split(strings.TrimSuffix(stdout, "\n"), "\n") {
		dec := json.NewDecoder(strings.NewReader(line))
		dec.DisallowUnknownFields()
		var ev journal.Event
		if err := dec.Decode(&ev); err != nil {
			return nil, fmt.Errorf("line %d %q: %w", i+1, line, err)
		}
		if ev.SchemaVersion != journal.EnvelopeVersion || ev.EventID == "" || ev.ProducerID == "" || !slices.Contains(documentedEvents, ev.Type) {
			return nil, fmt.Errorf("line %d is not a documented envelope event: %q", i+1, line)
		}
		out = append(out, ev)
	}
	return out, nil
}

func TestJSONLStdoutOnlyEnvelopes(t *testing.T) {
	f := newFixture(t)
	code, stdout, stderr := f.run(t, f.fakeRun("--format", "jsonl", "--scenario", "denied")...)
	evs, err := decodeEnvelopes(stdout)
	if err != nil {
		t.Fatalf("%v\nstderr: %s", err, stderr)
	}
	last := evs[len(evs)-1]
	if last.Type != "run.result" {
		t.Fatalf("last line is %s, want run.result", last.Type)
	}
	if p := payloadOf(t, last); p["exit_code"] != float64(code) {
		t.Fatalf("run.result exit_code = %v, process exited %d", p["exit_code"], code)
	}
	// Everything before run.result is the run's journal, in order.
	run := f.onlyRun(t)
	journaled := f.events(t, run.RunID)
	var got, want []string
	for _, ev := range evs[:len(evs)-1] {
		got = append(got, ev.EventID)
	}
	for _, ev := range journaled {
		want = append(want, ev.EventID)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("stdout events %v, journal %v", got, want)
	}
	if typeIndex(journaled, "attempt.permission_denied") < 0 {
		t.Fatalf("the denied scenario's permission denial was not journaled")
	}

	// Plain output is not JSONL, so the check above can fail.
	_, plain, _ := f.run(t, f.fakeRun("--scenario", "denied")...)
	if strings.TrimSpace(plain) == "" {
		t.Fatal("plain run printed nothing to stdout")
	}
	if _, err := decodeEnvelopes(plain); err == nil {
		t.Fatalf("decodeEnvelopes accepted plain output %q", plain)
	}
}

func TestIngestResumesFromOffsetWithoutDuplicates(t *testing.T) {
	t.Parallel()
	state := t.TempDir()
	j, err := journal.Open(t.Context(), state)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	p := supervisor.NewProducer(ids.New("sup"), 1)
	ref := supervisor.AttemptRef{StateDir: state, RunID: ids.New("run"), AttemptID: ids.New("att")}
	if err := supervisor.CreateRun(t.Context(), j, journal.RunRow{RunID: ref.RunID, AdapterID: "builtin/fake", SourceRepo: "/tmp/repo",
		TaskSHA256: strings.Repeat("a", 64), BillingPosture: "local-scripted", ExecutionProfile: "trusted-host"}, p); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.RecordLaunchIntent(t.Context(), j, journal.AttemptRow{AttemptID: ref.AttemptID, RunID: ref.RunID, TaskID: "task_1",
		AttemptNumber: 1, LaunchTokenSHA256: strings.Repeat("b", 64), WorkspacePath: "/tmp/ws"}, p); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(ref.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	spool := filepath.Join(ref.Dir(), supervisor.SpoolFile)
	line := func(seq int64, typ, payload string) string {
		b, err := json.Marshal(journal.Event{SchemaVersion: 1, EventID: fmt.Sprintf("evt_%s_%d", ref.AttemptID, seq), RunID: ref.RunID,
			TaskID: "task_1", AttemptID: ref.AttemptID, ProducerID: "wrk_" + ref.AttemptID, ProducerSequence: seq, Generation: 1,
			ObservedAt: time.Now().UTC(), Type: typ, Payload: json.RawMessage(payload)})
		if err != nil {
			t.Fatal(err)
		}
		return string(b) + "\n"
	}
	lines := []string{
		line(1, "attempt.state_changed", `{"state":"launching","reason":null}`),
		line(2, "attempt.launched", `{"worker_pid":10,"worker_start_time":"2026-09-29T00:00:00Z","native_pid":11,"native_pgid":11}`),
		line(3, "attempt.state_changed", `{"state":"running","reason":null}`),
		line(4, "attempt.progress", `{"assistant_turns":1,"tool_uses":{},"retries":0}`),
		line(5, "attempt.state_changed", `{"state":"failed_native","reason":"result_unobserved"}`),
	}
	appendSpool := func(s string) {
		fh, err := os.OpenFile(spool, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600) //nolint:gosec // G304: a test temp path
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = fh.Close() }()
		if _, err := fh.WriteString(s); err != nil {
			t.Fatal(err)
		}
	}
	workerEvents := func() []string {
		evs, err := j.Events(t.Context(), ref.RunID, 0)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, ev := range evs {
			if ev.ProducerID == "wrk_"+ref.AttemptID {
				got = append(got, ev.EventID)
			}
		}
		return got
	}
	ingest := func(wantSeq int64, wantEvents int, wantState string) {
		t.Helper()
		seq, err := supervisor.Ingest(t.Context(), j, ref)
		if err != nil {
			t.Fatalf("Ingest: %v", err)
		}
		a, err := j.Attempt(t.Context(), ref.AttemptID)
		if err != nil {
			t.Fatal(err)
		}
		got := workerEvents()
		if seq != wantSeq || len(got) != wantEvents || a.State != wantState {
			t.Fatalf("Ingest = seq %d, %d worker events, attempt %s; want seq %d, %d events, %s", seq, len(got), a.State, wantSeq, wantEvents, wantState)
		}
		if len(slices.Compact(slices.Sorted(slices.Values(got)))) != len(got) {
			t.Fatalf("duplicate events journaled: %v", got)
		}
	}

	// Three whole lines and a torn fourth: the torn line waits.
	torn := lines[3][:len(lines[3])/2]
	appendSpool(lines[0] + lines[1] + lines[2] + torn)
	ingest(3, 3, "running")
	a, _ := j.Attempt(t.Context(), ref.AttemptID)
	if want := int64(len(lines[0] + lines[1] + lines[2])); a.SpoolOffset != want {
		t.Fatalf("spool_offset = %d, want %d", a.SpoolOffset, want)
	}
	// The worker finishes the line and writes one more.
	appendSpool(lines[3][len(torn):] + lines[4])
	ingest(5, 5, "failed_native")
	// Nothing new: nothing added.
	ingest(5, 5, "failed_native")

	// A stored offset behind the journal (a database restored from a
	// backup, say) replays the spool, and every replayed event is ignored
	// by its event_id.
	db := rawDB(t, state)
	if _, err := db.ExecContext(t.Context(), `UPDATE attempts SET spool_offset = 0 WHERE attempt_id = ?`, ref.AttemptID); err != nil {
		t.Fatal(err)
	}
	ingest(5, 5, "failed_native")

	// Ingestion resumes at the stored offset: bytes before it are never
	// read again, so damage there goes unseen.
	fh, err := os.OpenFile(spool, os.O_WRONLY, 0o600) //nolint:gosec // G304: a test temp path
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fh.WriteAt([]byte("#"), 0); err != nil {
		t.Fatal(err)
	}
	_ = fh.Close()
	ingest(5, 5, "failed_native")

	// A line from another producer is refused and not journaled.
	other := strings.Replace(line(6, "attempt.progress", `{"assistant_turns":2,"tool_uses":{},"retries":0}`), `"wrk_`, `"sup_`, 1)
	appendSpool(other)
	if _, err := supervisor.Ingest(t.Context(), j, ref); !errors.Is(err, supervisor.ErrCorruptSpool) {
		t.Fatalf("Ingest of a foreign producer's line: err = %v, want ErrCorruptSpool", err)
	}
	if got := workerEvents(); len(got) != 5 {
		t.Fatalf("worker events after a corrupt line = %d, want 5", len(got))
	}
}

func rawDB(t *testing.T, state string) *sql.DB {
	t.Helper()
	p := filepath.ToSlash(filepath.Join(state, journal.DBName))
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	u := url.URL{Scheme: "file", Path: p, RawQuery: "_pragma=busy_timeout(5000)"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestOwnerLockExclusive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	release, err := supervisor.AcquireOwner(dir)
	if err != nil {
		t.Fatalf("first AcquireOwner: %v", err)
	}
	if _, err := supervisor.AcquireOwner(dir); !errors.Is(err, supervisor.ErrOwnerHeld) {
		t.Fatalf("second AcquireOwner while held: err = %v, want ErrOwnerHeld", err)
	}
	release()
	release() // idempotent
	again, err := supervisor.AcquireOwner(dir)
	if err != nil {
		t.Fatalf("AcquireOwner after release: %v", err)
	}
	again()
}

// liveRun is a `mythhelm run --format jsonl` child whose stdout and stderr
// are read line by line while it runs.
type liveRun struct {
	cmd    *exec.Cmd
	stdout chan string
	stderr chan string
	done   chan struct{}
	out    []string // every stdout line seen so far
}

func startLive(t *testing.T, f fixture, args ...string) *liveRun {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	t.Cleanup(cancel)
	cmd := f.command(ctx, args...)
	cmd.Stdin = strings.NewReader("")
	so, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	se, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	r := &liveRun{cmd: cmd, stdout: make(chan string, 1024), stderr: make(chan string, 1024), done: make(chan struct{})}
	pump := func(src *bufio.Scanner, dst chan string, closeDone bool) {
		for src.Scan() {
			dst <- src.Text()
		}
		close(dst)
		if closeDone {
			close(r.done)
		}
	}
	go pump(bufio.NewScanner(so), r.stdout, true)
	go pump(bufio.NewScanner(se), r.stderr, false)
	return r
}

// waitStdout reads stdout until a line satisfies match.
func (r *liveRun) waitStdout(t *testing.T, what string, match func(journal.Event) bool) {
	t.Helper()
	timeout := time.After(time.Minute)
	for {
		select {
		case line, ok := <-r.stdout:
			if !ok {
				t.Fatalf("stdout closed before %s; saw %q", what, r.out)
			}
			r.out = append(r.out, line)
			var ev journal.Event
			if json.Unmarshal([]byte(line), &ev) == nil && match(ev) {
				return
			}
		case <-timeout:
			t.Fatalf("timed out waiting for %s; saw %q", what, r.out)
		}
	}
}

// waitStderr reads stderr until a line contains want.
func (r *liveRun) waitStderr(t *testing.T, want string) {
	t.Helper()
	timeout := time.After(time.Minute)
	for {
		select {
		case line, ok := <-r.stderr:
			if !ok {
				t.Fatalf("stderr closed before %q", want)
			}
			if strings.Contains(line, want) {
				return
			}
		case <-timeout:
			t.Fatalf("timed out waiting for %q on stderr", want)
		}
	}
}

// wait collects the remaining output and the exit code.
func (r *liveRun) wait(t *testing.T) (int, []string, string) {
	t.Helper()
	for line := range r.stdout {
		r.out = append(r.out, line)
	}
	var stderr []string
	for line := range r.stderr {
		stderr = append(stderr, line)
	}
	<-r.done
	err := r.cmd.Wait()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0, r.out, strings.Join(stderr, "\n")
	case errors.As(err, &exitErr):
		return exitErr.ExitCode(), r.out, strings.Join(stderr, "\n")
	}
	t.Fatalf("waiting for mythhelm run: %v", err)
	return 0, nil, ""
}

func isType(typ string) func(journal.Event) bool {
	return func(ev journal.Event) bool { return ev.Type == typ }
}

func requireSignals(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Ctrl-C is delivered as SIGINT to the CLI; Windows console control events are not driven by this test")
	}
}

func TestInterruptBeforeWorkerSpawnNeverLaunchesNative(t *testing.T) {
	for _, stage := range []string{"executing", "launch_intent_recorded"} {
		t.Run(stage, func(t *testing.T) {
			f := newFixture(t)
			d, err := admission.Decide(t.Context(), admission.Request{
				StateDir: f.state, Repo: f.repo, TaskFile: f.task,
				Adapter: admission.AdapterFake, Billing: admission.BillingLocalScripted,
				ExecutionProfile: admission.ProfileTrustedHost, Env: os.Environ(),
				NoChecks: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			interrupts := make(chan os.Signal, 1)
			sent := false
			out, err := supervisor.Run(t.Context(), d, supervisor.Hooks{
				Interrupt: interrupts,
				Event: func(ev journal.Event) {
					if sent {
						return
					}
					if stage == "executing" && ev.Type == "run.state_changed" && payloadOf(t, ev)["state"] == "executing" ||
						stage == "launch_intent_recorded" && ev.Type == "attempt.launch_intent_recorded" {
						sent = true
						interrupts <- os.Interrupt
					}
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !sent || out.State != supervisor.RunCancelled {
				t.Fatalf("interrupt sent=%t, run state=%s; want cancelled", sent, out.State)
			}
			if run := f.onlyRun(t); run.State != string(supervisor.RunCancelled) {
				t.Fatalf("projected run state=%s, want cancelled", run.State)
			}
			evs := f.events(t, d.RunID)
			if typeIndex(evs, "attempt.launched") >= 0 {
				t.Fatal("an interrupt before spawn still launched the native")
			}
			if _, err := os.Stat(filepath.Join(workers.AttemptDir(f.state, d.RunID, d.AttemptID), "worker.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("worker identity exists after cancellation: %v", err)
			}
		})
	}
}

// TestRunSeedsRegistry runs the supervisor twice against one fresh state
// dir with admitted fake decisions (cancelled before the worker spawns,
// so no native launches): the first run seeds the seven v2 §7.2 records
// and the second records none (I14: the seven honest labels exist from
// the first admission).
func TestRunSeedsRegistry(t *testing.T) {
	f := newFixture(t)
	decide := func() admission.Decision {
		t.Helper()
		d, err := admission.Decide(t.Context(), admission.Request{
			StateDir: f.state, Repo: f.repo, TaskFile: f.task,
			Adapter: admission.AdapterFake, Billing: admission.BillingLocalScripted,
			ExecutionProfile: admission.ProfileTrustedHost, Env: os.Environ(),
			NoChecks: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	cancelledRun := func(d admission.Decision) {
		t.Helper()
		interrupts := make(chan os.Signal, 1)
		interrupts <- os.Interrupt
		out, err := supervisor.Run(t.Context(), d, supervisor.Hooks{Interrupt: interrupts})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if out.State != supervisor.RunCancelled {
			t.Fatalf("run state = %s, want cancelled", out.State)
		}
	}
	currentRecords := func() []string {
		t.Helper()
		rows, err := f.journal(t).ListCurrentQualificationRecords(t.Context())
		if err != nil {
			t.Fatalf("ListCurrentQualificationRecords: %v", err)
		}
		return rows
	}
	// totalRows counts every qualification row, superseded or not: new
	// revisions superseding the seed would show up here even while the
	// current list still holds seven.
	totalRows := func() int {
		t.Helper()
		var n int
		if err := rawDB(t, f.state).QueryRowContext(t.Context(),
			`SELECT count(*) FROM qualification_records`).Scan(&n); err != nil {
			t.Fatalf("count qualification_records: %v", err)
		}
		return n
	}
	cancelledRun(decide())
	before := currentRecords()
	if len(before) != 7 {
		t.Fatalf("current qualification records after the first run = %d, want 7", len(before))
	}
	if got := totalRows(); got != 7 {
		t.Fatalf("qualification rows after the first run = %d, want 7", got)
	}
	cancelledRun(decide())
	if after := currentRecords(); !slices.Equal(after, before) {
		t.Fatalf("current qualification records changed on the second run:\nbefore: %q\nafter: %q", before, after)
	}
	if got := totalRows(); got != 7 {
		t.Fatalf("qualification rows after the second run = %d, want 7 (no new revisions)", got)
	}
}

// TestSeedFailureFailsClosed drops the qualification table out from under
// the supervisor after admission: the run must fail through
// persistence_unavailable instead of running unseeded, admitting nothing.
func TestSeedFailureFailsClosed(t *testing.T) {
	f := newFixture(t)
	d, err := admission.Decide(t.Context(), admission.Request{
		StateDir: f.state, Repo: f.repo, TaskFile: f.task,
		Adapter: admission.AdapterFake, Billing: admission.BillingLocalScripted,
		ExecutionProfile: admission.ProfileTrustedHost, Env: os.Environ(),
		NoChecks: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Migrate the database the supervisor will open, then drop the table
	// out from under it: the open succeeds (user_version=2) and only the
	// seed fails.
	migrate, err := journal.Open(t.Context(), f.state)
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	if err := migrate.Close(); err != nil {
		t.Fatal(err)
	}
	db := rawDB(t, f.state)
	if _, err := db.Exec("DROP TABLE qualification_records"); err != nil {
		t.Fatalf("DROP TABLE qualification_records: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	interrupts := make(chan os.Signal, 1)
	interrupts <- os.Interrupt
	out, err := supervisor.Run(t.Context(), d, supervisor.Hooks{Interrupt: interrupts})
	if err == nil {
		t.Fatalf("Run on an unseedable database reached state %s; want persistence_unavailable", out.State)
	}
	var blocked *admission.BlockedError
	if !errors.As(err, &blocked) || blocked.Code != "persistence_unavailable" {
		t.Fatalf("Run error = %v, want persistence_unavailable", err)
	}
	if rows := f.runs(t); len(rows) != 0 {
		t.Fatalf("recorded runs = %d, want 0: the failed run admitted nothing", len(rows))
	}
}

func TestCtrlCOnceStopsAndExits130(t *testing.T) {
	t.Parallel()
	requireSignals(t)
	f := newFixture(t)
	r := startLive(t, f, f.fakeRun("--format", "jsonl", "--scenario", "slow")...)
	r.waitStdout(t, "the native session", isType("attempt.native_session"))
	if err := r.cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := r.wait(t)
	if code != 130 {
		t.Fatalf("exit %d, stderr %q; want 130", code, stderr)
	}
	if !strings.Contains(stderr, supervisor.StopRequestedNotice) {
		t.Fatalf("stderr %q does not say the stop was requested", stderr)
	}
	evs, err := decodeEnvelopes(strings.Join(stdout, "\n") + "\n")
	if err != nil {
		t.Fatal(err)
	}
	requested, stopped, cancelled := typeIndex(evs, "attempt.stop_requested"), typeIndex(evs, "attempt.stopped"), -1
	for i, ev := range evs {
		if ev.Type == "run.state_changed" && payloadOf(t, ev)["state"] == "cancelled" {
			cancelled = i
		}
	}
	if requested < 0 || stopped <= requested || cancelled <= stopped {
		t.Fatalf("order stop_requested=%d stopped=%d cancelled=%d, want the confirmed stop before cancelled", requested, stopped, cancelled)
	}
	if p := payloadOf(t, evs[stopped]); p["confirmed"] != true {
		t.Fatalf("attempt.stopped = %v, want confirmed", p)
	}
	if run := f.onlyRun(t); run.State != "cancelled" {
		t.Fatalf("run projected %s, want cancelled", run.State)
	}
	run := f.onlyRun(t)
	c, err := f.journal(t).Candidate(t.Context(), findAttempt(t, f, run.RunID))
	if err != nil || !c.Partial {
		t.Fatalf("cancelled attempt candidate = %+v, %v; want partial", c, err)
	}
}

func TestFreezeWaitsForStopConfirmation(t *testing.T) {
	for _, tc := range []struct {
		scenario string
		partial  bool
	}{
		{"happy", false}, {"native-fails", true},
	} {
		t.Run(tc.scenario, func(t *testing.T) {
			f := newFixture(t)
			_, _, stderr := f.run(t, f.fakeRun("--scenario", tc.scenario, "--format", "jsonl")...)
			if stderr != "" {
				t.Logf("run stderr: %s", stderr)
			}
			run := f.onlyRun(t)
			evs := f.events(t, run.RunID)
			stop, frozen := typeIndex(evs, "attempt.stopped"), typeIndex(evs, "candidate.frozen")
			if stop < 0 || frozen <= stop || payloadOf(t, evs[stop])["confirmed"] != true {
				t.Fatalf("stop=%d frozen=%d: candidate must follow confirmed stop", stop, frozen)
			}
			c, err := f.journal(t).Candidate(t.Context(), findAttempt(t, f, run.RunID))
			if err != nil || c.Partial != tc.partial {
				t.Fatalf("candidate = %+v, %v; want partial=%t", c, err, tc.partial)
			}
		})
	}
}

func TestFreezeRefusedWithUnresolvedDescendants(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("escapee process scan exists on Linux and macOS")
	}
	f := newFixture(t)
	code, _, stderr := f.run(t, f.fakeRun("--scenario", "escapee", "--format", "jsonl")...)
	if code != 6 {
		t.Fatalf("exit = %d, stderr = %q; want 6", code, stderr)
	}
	run := f.onlyRun(t)
	if run.State != "interrupted" || run.Reason != "unresolved_descendants" {
		t.Fatalf("run = %s/%s; want interrupted/unresolved_descendants", run.State, run.Reason)
	}
	evs := f.events(t, run.RunID)
	if typeIndex(evs, "candidate.frozen") >= 0 {
		t.Fatal("unresolved attempt froze a candidate")
	}
	if stop := typeIndex(evs, "attempt.stopped"); stop >= 0 {
		if pids, ok := payloadOf(t, evs[stop])["unresolved_pids"].([]any); ok {
			for _, raw := range pids {
				pid := int(raw.(float64))
				if proc, err := os.FindProcess(pid); err == nil {
					_ = proc.Kill()
				}
			}
		}
	}
	if _, err := f.journal(t).Candidate(t.Context(), findAttempt(t, f, run.RunID)); !errors.Is(err, journal.ErrNotFound) {
		t.Fatalf("candidate row exists or read failed: %v", err)
	}
	// apply is introduced in Task 14. Until then the CLI refuses it and
	// cannot create a branch from an unresolved attempt.
	applyCode, _, _ := f.run(t, "apply", run.RunID, "--to-branch", "unsafe")
	if applyCode == 0 || f.git(t, "branch", "--list", "unsafe") != "" {
		t.Fatal("apply accepted an unresolved attempt")
	}
}

func TestCtrlCTwiceDetachesExit6(t *testing.T) {
	t.Parallel()
	requireSignals(t)
	f := newFixture(t)
	// ignore-sigint holds out through the first rung (3 s), so the stop is
	// still unconfirmed when the second interrupt arrives.
	r := startLive(t, f, f.fakeRun("--format", "jsonl", "--scenario", "ignore-sigint")...)
	r.waitStdout(t, "the native session", isType("attempt.native_session"))
	if err := r.cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	r.waitStderr(t, supervisor.StopRequestedNotice)
	if err := r.cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := r.wait(t)
	if code != 6 {
		t.Fatalf("exit %d, stderr %q; want 6", code, stderr)
	}
	if !strings.Contains(stderr, "stop not yet confirmed") {
		t.Fatalf("stderr %q does not say the stop is unconfirmed", stderr)
	}
	evs, err := decodeEnvelopes(strings.Join(stdout, "\n") + "\n")
	if err != nil {
		t.Fatal(err)
	}
	if typeIndex(evs, "attempt.stopped") >= 0 {
		t.Fatalf("the CLI printed attempt.stopped before detaching, so the stop was already confirmed")
	}
	if res := payloadOf(t, evs[len(evs)-1]); res["exit_code"] != float64(6) || res["state"] != "stopping" {
		t.Fatalf("run.result = %v, want exit 6 in state stopping", res)
	}
	run := f.onlyRun(t)
	if run.State != "stopping" {
		t.Fatalf("run projected %s after detaching, want stopping", run.State)
	}

	// The worker still owns the attempt and confirms the stop on its own.
	a := findAttempt(t, f, run.RunID)
	waitSpool(t, filepath.Join(workers.AttemptDir(f.state, run.RunID, a), supervisor.SpoolFile), `"type":"attempt.stopped","payload":{"confirmed":true`)
}

func findAttempt(t *testing.T, f fixture, runID string) string {
	t.Helper()
	i := typeIndex(f.events(t, runID), "attempt.launch_intent_recorded")
	if i < 0 {
		t.Fatalf("run %s has no attempt", runID)
	}
	return f.events(t, runID)[i].AttemptID
}

// waitSpool polls a spool until it contains want.
func waitSpool(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for {
		b, _ := os.ReadFile(path) //nolint:gosec // G304: a test temp path
		if bytes.Contains(b, []byte(want)) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("spool %s never contained %s:\n%s", path, want, b)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestWorkerKilledRunInterruptedExit6(t *testing.T) {
	t.Parallel()
	requireSignals(t)
	f := newFixture(t)
	r := startLive(t, f, f.fakeRun("--format", "jsonl", "--scenario", "slow")...)
	r.waitStdout(t, "the native session", isType("attempt.native_session"))
	run := f.onlyRun(t)
	id, err := workers.ReadIdentity(workers.AttemptDir(f.state, run.RunID, findAttempt(t, f, run.RunID)))
	if err != nil {
		t.Fatal(err)
	}
	for _, pid := range []int{id.PID, *id.NativePID} {
		proc, err := os.FindProcess(pid)
		if err != nil {
			t.Fatal(err)
		}
		if err := proc.Kill(); err != nil {
			t.Fatalf("killing pid %d: %v", pid, err)
		}
	}
	code, _, stderr := r.wait(t)
	if code != 6 {
		t.Fatalf("exit %d, stderr %q; want 6", code, stderr)
	}
	if run := f.onlyRun(t); run.State != "interrupted" || run.Reason != "worker_lost" {
		t.Fatalf("run projected %s/%s, want interrupted/worker_lost", run.State, run.Reason)
	}
	a, err := f.journal(t).Attempt(t.Context(), findAttempt(t, f, run.RunID))
	if err != nil || a.State != "quarantined" {
		t.Fatalf("attempt = %+v, %v; want quarantined", a, err)
	}
}

func TestIngestFailureInterruptsRunExit6(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	r := startLive(t, f, f.fakeRun("--format", "jsonl", "--scenario", "slow")...)
	// After its progress, the slow scenario's worker spools nothing until
	// it is stopped, so the lines below follow its last one.
	r.waitStdout(t, "the attempt's progress", isType("attempt.progress"))
	run := f.onlyRun(t)
	attemptID := findAttempt(t, f, run.RunID)
	dir := workers.AttemptDir(f.state, run.RunID, attemptID)
	spool := filepath.Join(dir, supervisor.SpoolFile)
	b, err := os.ReadFile(spool) //nolint:gosec // G304: a test temp path
	if err != nil {
		t.Fatal(err)
	}
	// A valid worker state change, journaled in the same ingest as the line
	// no worker writes that follows it.
	evs := f.events(t, run.RunID)
	taskID := evs[typeIndex(evs, "attempt.launch_intent_recorded")].TaskID
	forged, err := json.Marshal(journal.Event{SchemaVersion: 1, EventID: ids.New("evt"), RunID: run.RunID, TaskID: taskID,
		AttemptID: attemptID, ProducerID: "wrk_" + attemptID, ProducerSequence: int64(bytes.Count(b, []byte("\n"))) + 1, Generation: 1,
		ObservedAt: time.Now().UTC(), Type: "attempt.state_changed", Payload: json.RawMessage(`{"state":"failed_native","reason":"forged_reason"}`)})
	if err != nil {
		t.Fatal(err)
	}
	fh, err := os.OpenFile(spool, os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // G304: a test temp path
	if err != nil {
		t.Fatal(err)
	}
	_, err = fh.WriteString(string(forged) + "\n{}\n")
	_ = fh.Close()
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := r.wait(t)
	// The worker still owns the native; stop it through the production path.
	if err := workers.RequestStop(dir, "test-cleanup"); err != nil {
		t.Fatal(err)
	}
	waitSpool(t, spool, `"type":"attempt.stopped"`)

	if code != 6 || !strings.Contains(stderr, "mythhelm recover "+run.RunID) {
		t.Fatalf("exit %d, stderr %q; want exit 6 naming mythhelm recover", code, stderr)
	}
	if run := f.onlyRun(t); run.State != "interrupted" || run.Reason != "ingest_failed" {
		t.Fatalf("run projected %s/%s, want interrupted/ingest_failed", run.State, run.Reason)
	}
	// run.result reports the attempt as journaled, not as last seen.
	if res := lastResult(t, strings.Join(stdout, "\n")+"\n"); res["attempt_reason"] != "forged_reason" {
		t.Fatalf("run.result = %v, want attempt_reason forged_reason", res)
	}
}

// envelopeConfig commits a mythhelm.toml with an [envelopes] table and one
// passing check, and returns its trust digest.
func (f fixture) envelopeConfig(t *testing.T) string {
	t.Helper()
	contents := "schema_version = 1\n[envelopes]\nexecution = \"10m\"\nrepairs = 1\nreplans = 0\n" +
		"[[checks]]\nname = \"test\"\nargv = [" + strconv.Quote(os.Args[0]) + ", \"__check\", \"pass\"]\ntimeout = \"5s\"\n"
	if err := os.WriteFile(filepath.Join(f.repo, "mythhelm.toml"), []byte(contents), 0o600); err != nil { // #nosec G703 -- fixture repo is t.TempDir
		t.Fatal(err)
	}
	f.git(t, "add", "mythhelm.toml")
	f.git(t, "commit", "--quiet", "-m", "add envelopes")
	_, digest, err := admission.ParseProjectConfig([]byte(contents))
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func (f fixture) decideEnvelope(t *testing.T, digest string, flags *billing.Ceilings) admission.Decision {
	t.Helper()
	d, err := admission.Decide(t.Context(), admission.Request{
		StateDir: f.state, Repo: f.repo, TaskFile: f.task,
		Adapter: admission.AdapterFake, Billing: admission.BillingLocalScripted,
		ExecutionProfile: admission.ProfileTrustedHost, Env: os.Environ(),
		TrustProjectConfig: "sha256:" + digest, EnvelopeFlags: flags,
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (f fixture) reservations(t *testing.T, runID string) []string {
	t.Helper()
	rows, err := rawDB(t, f.state).QueryContext(t.Context(), `SELECT reservation_id FROM reservations WHERE run_id = ? ORDER BY reservation_id`, runID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAdmissionWritesInitialEnvelope(t *testing.T) {
	f := newFixture(t)
	flags := &billing.Ceilings{Execution: -1, Repairs: 5, Replans: -1, TransportRetries: -1}
	d := f.decideEnvelope(t, f.envelopeConfig(t), flags)
	var notices []string
	out, err := supervisor.Run(t.Context(), d, supervisor.Hooks{Notice: func(s string) { notices = append(notices, s) }})
	if err != nil || out.State != supervisor.RunReadyForReview {
		t.Fatalf("run = %+v, %v; want ready_for_review", out, err)
	}
	file, err := d.ProjectConfig.Envelopes.ToCeilings()
	if err != nil {
		t.Fatal(err)
	}
	want := billing.ResolveCeilings(flags, file)
	// Flag repairs 5 beat file repairs 1; the file's 10m and 0 replans beat
	// the built-ins; transport retries fall through to the built-in 5.
	if want != (billing.Ceilings{Execution: 10 * time.Minute, Repairs: 5, Replans: 0, TransportRetries: 5}) {
		t.Fatalf("test premise: resolved ceilings = %+v", want)
	}
	row, err := f.journal(t).RunEnvelope(t.Context(), d.RunID)
	if err != nil {
		t.Fatalf("admitted run has no envelope row: %v", err)
	}
	got := billing.Ceilings{Execution: time.Duration(row.ExecutionSeconds) * time.Second,
		Repairs: int(row.Repairs), Replans: int(row.Replans), TransportRetries: int(row.TransportRetries)}
	if got != want {
		t.Fatalf("envelope row ceilings = %+v, want %+v", got, want)
	}
	// The first attempt is neither a repair nor a replan and the fake run
	// retries nothing; FirstStartAt is the launch gate's to set (Task 7).
	if row.RepairsUsed != 0 || row.ReplansUsed != 0 || row.TransportRetriesSeen != 0 {
		t.Fatalf("a one-attempt run carries usage: %+v", row)
	}

	// The admitted run holds exactly one reservation, coupled to the bucket,
	// and the surfaced text says it is local coordination only.
	ids := f.reservations(t, d.RunID)
	if len(ids) != 1 {
		t.Fatalf("reservations = %v, want exactly one", ids)
	}
	res, err := f.journal(t).Reservation(t.Context(), ids[0])
	if err != nil {
		t.Fatal(err)
	}
	// The attempt is terminal by now, so the claim is released with evidence
	// (Task 8); it was held while the attempt ran (TestReleaseOnlyAfterStop).
	if res.Status != "released" || res.ReleaseEvidence == "" || res.Quantity != "unknown" || res.Scope == "" || res.Scope != res.Bucket {
		t.Fatalf("reservation = %+v, want a released, unknown-quantity row with scope = bucket", res)
	}
	if !slices.Contains(notices, admission.ReservationText(res.Bucket)) {
		t.Fatalf("notices %q lack the reservation line", notices)
	}
}

func TestHoldFailureBlocksRun(t *testing.T) {
	f := newFixture(t)
	d := f.decideEnvelope(t, f.envelopeConfig(t), nil)
	// Storage refusing the hold is the failure; the journal must exist first.
	_ = f.journal(t)
	if _, err := rawDB(t, f.state).ExecContext(t.Context(), `CREATE TRIGGER inject_hold BEFORE INSERT ON reservations BEGIN SELECT RAISE(ABORT, 'injected hold failure'); END`); err != nil {
		t.Fatal(err)
	}
	out, err := supervisor.Run(t.Context(), d, supervisor.Hooks{})
	if err == nil || !strings.Contains(err.Error(), "injected hold failure") {
		t.Fatalf("Run err = %v, want the hold failure", err)
	}
	if out.State != supervisor.RunBlocked || out.Reason != "quota_reservation_failed" {
		t.Fatalf("outcome = %s (%s), want blocked (quota_reservation_failed)", out.State, out.Reason)
	}
	if run := f.onlyRun(t); run.State != string(supervisor.RunBlocked) || run.Reason != "quota_reservation_failed" {
		t.Fatalf("projected run = %s (%s), want blocked (quota_reservation_failed)", run.State, run.Reason)
	}
	// Nothing launched and nothing is held: no partial claim survives.
	evs := f.events(t, d.RunID)
	if typeIndex(evs, "attempt.launch_intent_recorded") >= 0 {
		t.Fatal("a run whose hold failed still recorded a launch intent")
	}
	// The hold commits with the admission, so a failed hold leaves neither
	// the admission event nor the envelope behind (AC-3.2, design §4).
	if typeIndex(evs, "admission.decided") >= 0 {
		t.Fatal("admission.decided survived a failed hold: the hold is not in the admission transaction")
	}
	if _, err := f.journal(t).RunEnvelope(t.Context(), d.RunID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("RunEnvelope after a failed hold = %v, want no row", err)
	}
	if got := f.reservations(t, d.RunID); len(got) != 0 {
		t.Fatalf("reservations after a failed hold = %v, want none", got)
	}
}

func TestFailedAdmissionHoldsNothing(t *testing.T) {
	f := newFixture(t)
	// Another run is active, so admission refuses before any hold.
	j := f.journal(t)
	other := ids.New("run")
	now := time.Now()
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.InsertRun(t.Context(), tx, journal.RunRow{RunID: other, State: string(supervisor.RunExecuting), AdapterID: "builtin/fake",
			SourceRepo: f.repo, TaskSHA256: "t", BillingPosture: "local-scripted", ExecutionProfile: "trusted-host", CreatedAt: now, UpdatedAt: now})
	}); err != nil {
		t.Fatal(err)
	}
	d := f.decideEnvelope(t, f.envelopeConfig(t), nil)
	out, err := supervisor.Run(t.Context(), d, supervisor.Hooks{})
	if out.State != supervisor.RunBlocked || out.Reason != "active_run_exists" {
		t.Fatalf("run = %s (%s), %v; want blocked (active_run_exists)", out.State, out.Reason, err)
	}
	if got := f.reservations(t, d.RunID); len(got) != 0 {
		t.Fatalf("a refused admission holds %v, want nothing (AC-3.2)", got)
	}
}

// inspectRegistry is a one-record contain.Registry.
type inspectRegistry struct{ ev contain.Evidence }

func (r inspectRegistry) Lookup(profile, os, route string) (contain.Evidence, bool) {
	if profile == r.ev.Profile && os == r.ev.OS && route == r.ev.Route {
		return r.ev, true
	}
	return contain.Evidence{}, false
}

func TestInspectRequiresEnforcedCoverage(t *testing.T) {
	t.Parallel()
	ev, err := admission.BoundaryConsult("inspect", "linux", "builtin/fake")
	if err != nil {
		t.Fatalf("seeded inspect evidence on linux: %v", err)
	}

	// The same record with the filesystem claim not enforced is refused, and
	// the refusal is exit 7: there is no prompt-only path to accept.
	weak := ev
	weak.Coverage.Filesystem.Enforced = false
	_, err = admission.ConsultRegistry(inspectRegistry{weak}, "inspect", "linux", "builtin/fake")
	var blocked *admission.BlockedError
	if !errors.As(err, &blocked) || !blocked.Capability || !strings.Contains(err.Error(), "filesystem") {
		t.Fatalf("err = %v, want an exit-7 refusal naming filesystem", err)
	}
	if _, err := admission.ConsultRegistry(inspectRegistry{ev}, "inspect", "linux", "builtin/fake"); err != nil {
		t.Fatalf("the unmodified record is refused: %v", err)
	}
}

func TestInspectPolicyIsReadOnly(t *testing.T) {
	t.Parallel()
	workdir := filepath.Join(t.TempDir(), "work")
	binds := []contain.AuthBind{{Source: filepath.Join(t.TempDir(), "token"), Target: filepath.Join(t.TempDir(), "home", "token")}}
	inspect, err := contain.PolicyForProfile("inspect", workdir, binds, "")
	if err != nil {
		t.Fatal(err)
	}
	restricted, err := contain.PolicyForProfile("restricted", workdir, binds, "")
	if err != nil {
		t.Fatal(err)
	}
	if !inspect.ReadOnly || restricted.ReadOnly {
		t.Fatalf("read-only: inspect %v, restricted %v; want true and false", inspect.ReadOnly, restricted.ReadOnly)
	}
	// Shared mechanism: nothing but the profile name and the read-only flag differs.
	inspect.Profile, inspect.ReadOnly = restricted.Profile, restricted.ReadOnly
	if !reflect.DeepEqual(inspect, restricted) {
		t.Fatalf("inspect policy diverges from restricted: %+v vs %+v", inspect, restricted)
	}
	// Read-only never rests on a label: an inspect policy that is not
	// read-only cannot be built.
	if p, err := contain.PolicyFor("inspect", workdir, false, nil, ""); err == nil {
		t.Fatalf("PolicyFor built a writable inspect policy: %+v", p)
	}
}

func TestChecksRefusedUnderInspect(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("inspect is qualified on Linux only")
	}
	f := newFixture(t)
	marker := filepath.Join(t.TempDir(), "check-ran")
	contents := "schema_version = 1\n[[checks]]\nname = \"test\"\nargv = [" + strconv.Quote(os.Args[0]) +
		", \"__check\", \"marker\", " + strconv.Quote(marker) + "]\ntimeout = \"5s\"\n"
	if err := os.WriteFile(filepath.Join(f.repo, "mythhelm.toml"), []byte(contents), 0o600); err != nil { // #nosec G703 -- fixture repo is t.TempDir
		t.Fatal(err)
	}
	f.git(t, "add", "mythhelm.toml")
	f.git(t, "commit", "--quiet", "-m", "add checks")
	_, digest, err := admission.ParseProjectConfig([]byte(contents))
	if err != nil {
		t.Fatal(err)
	}
	args := slices.DeleteFunc(f.checkedRun(digest, "--format", "jsonl"), func(a string) bool { return a == "trusted-host" || a == "--execution-profile" })
	code, stdout, stderr := f.run(t, append(args, "--execution-profile", "inspect")...)
	if code != 5 {
		t.Fatalf("exit %d, stderr %q; want exit 5 (verification unavailable)", code, stderr)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a check ran under inspect")
	}
	evs, err := decodeEnvelopes(stdout)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, ev := range evs {
		if ev.Type != "verification.completed" {
			continue
		}
		found = true
		if p := payloadOf(t, ev); p["result"] != "NOT RUN" || p["reason"] != "checks_refused_under_inspect" {
			t.Errorf("verification.completed = %v, want NOT RUN / checks_refused_under_inspect", p)
		}
	}
	if !found {
		t.Fatal("no verification.completed event")
	}
	if res := lastResult(t, stdout); res["error_category"] != "verification_unavailable" || res["state"] != "ready_for_review" {
		t.Errorf("run.result = %v, want ready_for_review with category verification_unavailable", res)
	}
}

func TestDeadlineExpiryBlocks(t *testing.T) {
	f := newFixture(t)
	// The slow scenario would run for ten minutes; the 2s execution ceiling
	// outlasts the launch (a ceiling that ran out before the intent commits
	// is refused instead, TestLaunchRefusedWhenDeadlineExpiresBeforeCommit)
	// and ends the attempt through the stop ladder. The test never sleeps.
	d, err := admission.Decide(t.Context(), admission.Request{
		StateDir: f.state, Repo: f.repo, TaskFile: f.task,
		Adapter: admission.AdapterFake, Billing: admission.BillingLocalScripted,
		ExecutionProfile: admission.ProfileTrustedHost, Env: os.Environ(),
		NoChecks: true, Scenario: "slow",
		EnvelopeFlags: &billing.Ceilings{Execution: 2 * time.Second, Repairs: -1, Replans: -1, TransportRetries: -1},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	out, err := supervisor.Run(ctx, d, supervisor.Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	if out.State != supervisor.RunBlocked || out.Reason != "envelope_deadline_exceeded" {
		t.Fatalf("outcome = %s (%s), want blocked (envelope_deadline_exceeded)", out.State, out.Reason)
	}
	if out.AttemptState != supervisor.AttemptStopped {
		t.Fatalf("attempt = %s, want stopped through the stop ladder", out.AttemptState)
	}
	evs := f.events(t, d.RunID)
	if typeIndex(evs, "candidate.frozen") < 0 {
		t.Fatal("no candidate was preserved when the deadline stopped the attempt")
	}
	row, err := f.journal(t).RunEnvelope(t.Context(), d.RunID)
	if err != nil || row.FirstStartAt == "" {
		t.Fatalf("envelope after launch = %+v, %v; want the first start recorded", row, err)
	}
}

func TestGateRefusalBlocksRunBeforeAnyIntent(t *testing.T) {
	f := newFixture(t)
	d := f.decideEnvelope(t, f.envelopeConfig(t), nil)
	// A run whose execution clock started long ago, as after a recovery: the
	// trigger stamps the first start when the admission writes the envelope.
	_ = f.journal(t)
	if _, err := rawDB(t, f.state).ExecContext(t.Context(), `CREATE TRIGGER aged_clock AFTER INSERT ON run_envelopes BEGIN
		UPDATE run_envelopes SET first_start_at = '2000-01-01T00:00:00Z' WHERE run_id = NEW.run_id; END`); err != nil {
		t.Fatal(err)
	}
	out, err := supervisor.Run(t.Context(), d, supervisor.Hooks{})
	if err == nil {
		t.Fatal("a refused launch returned no error")
	}
	if out.State != supervisor.RunBlocked || out.Reason != "envelope_deadline_exceeded" {
		t.Fatalf("outcome = %s (%s), want blocked (envelope_deadline_exceeded)", out.State, out.Reason)
	}
	if typeIndex(f.events(t, d.RunID), "attempt.launch_intent_recorded") >= 0 {
		t.Fatal("a launch the envelope refused still journaled an intent")
	}
}
