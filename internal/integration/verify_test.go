package integration_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/contain"
	"github.com/turbokast/mythhelm/internal/integration"
	"github.com/turbokast/mythhelm/internal/workspace"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == contain.Command {
		os.Exit(contain.Main(os.Args[2:]))
	}
	if len(os.Args) > 2 && os.Args[1] == "__verify_helper" {
		switch os.Args[2] {
		case "pass":
			os.Exit(0)
		case "output":
			fmt.Print("not formatted\n")
			os.Exit(0)
		case "secret":
			fmt.Print("token=pretend-secret-value\n")
			os.Exit(0)
		case "spawn":
			// Optional readiness probe: spawn <marker> [ready sleep]. When
			// given, the grandchild writes ready at startup so the test can
			// prove the grandchild exists before the kill fires.
			args := []string{"__verify_helper", "delayed", os.Args[3]}
			if len(os.Args) > 5 {
				args = append(args, os.Args[4], os.Args[5])
			}
			child := exec.Command(os.Args[0], args...) // #nosec G702 -- test binary re-exec with test-owned path
			if err := child.Start(); err != nil {
				os.Exit(2)
			}
			time.Sleep(60 * time.Second)
		case "delayed":
			// Optional readiness probe: delayed <marker> [ready sleep].
			// Defaults preserve the pre-probe timing for existing callers.
			sleep := time.Second
			if len(os.Args) > 5 {
				if d, err := time.ParseDuration(os.Args[5]); err == nil && d > 0 {
					sleep = d
				}
				_ = os.WriteFile(os.Args[4], []byte("started"), 0o600) // #nosec G703 -- marker path is a test-owned temporary file
			}
			time.Sleep(sleep)
			_ = os.WriteFile(os.Args[3], []byte("survived"), 0o600) // #nosec G703 -- marker path is a test-owned temporary file
		case "linger_parent":
			child := exec.Command(os.Args[0], "__verify_helper", "linger_child") // #nosec G702 -- test binary re-exec
			child.Stdout, child.Stderr = os.Stdout, os.Stderr
			if err := child.Start(); err != nil {
				os.Exit(2)
			}
		case "linger_failed_parent":
			child := exec.Command(os.Args[0], "__verify_helper", "delayed", os.Args[3]) // #nosec G702 -- test binary re-exec with test-owned path
			child.Stdout, child.Stderr = os.Stdout, os.Stderr
			if err := child.Start(); err != nil {
				os.Exit(2)
			}
			os.Exit(1)
		case "linger_child":
			time.Sleep(3 * time.Second)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func verifyFixture(t *testing.T) (integration.Candidate, admission.ProjectConfig) {
	t.Helper()
	return verifyFixtureWith(t, nil)
}

// verifyFixtureWith is verifyFixture with extra files added to the candidate
// before it is frozen.
func verifyFixtureWith(t *testing.T, files map[string]string) (integration.Candidate, admission.ProjectConfig) {
	t.Helper()
	f := fixture(t)
	f.write(t, "mythhelm.toml", "schema_version = 1\n[[checks]]\nname = \"admitted\"\nargv = [\"true\"]\ntimeout = \"2s\"\n")
	f.git(t, "add", "mythhelm.toml")
	f.git(t, "commit", "--quiet", "-m", "config")
	f.base = f.git(t, "rev-parse", "HEAD")
	runDir := t.TempDir()
	clone := filepath.Join(runDir, "workspace")
	if err := workspace.Snapshot(t.Context(), f.dir, f.base, clone); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clone, "mythhelm.toml"), []byte("schema_version = 1\n[[checks]]\nname = \"injected\"\nargv = [\"missing-injected-tool\"]\ntimeout = \"2s\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(clone, name), []byte(body), 0o700); err != nil { //nolint:gosec // G306: an executable fixture
			t.Fatal(err)
		}
	}
	c, err := integration.Freeze(t.Context(), clone, f.base, integration.CommitMeta{
		RunID: "run_verify", AttemptID: "att_verify", Title: "Demo", Name: "Test User", Email: "test@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := admission.ProjectConfig{Checks: []admission.CheckConfig{{Name: "admitted", Argv: []string{os.Args[0], "__verify_helper", "pass"}, Timeout: "2s"}}}
	return c, cfg
}

func TestChecksReadFromSnapshotNotCandidate(t *testing.T) {
	c, cfg := verifyFixture(t)
	if !flagExists(c, "check_config_changed") {
		t.Fatal("candidate edit did not flag check_config_changed")
	}
	v, err := integration.RunChecks(t.Context(), c, cfg, os.Environ())
	if err != nil || v.Result != "passed" || len(v.Checks) != 1 || v.Checks[0].Name != "admitted" {
		t.Fatalf("verification = %+v, err = %v", v, err)
	}
}

func TestGofmtFailOnOutput(t *testing.T) {
	c, cfg := verifyFixture(t)
	cfg.Checks[0] = admission.CheckConfig{Name: "gofmt", Argv: []string{os.Args[0], "__verify_helper", "output"}, FailOnOutput: true, Timeout: "2s"}
	v, err := integration.RunChecks(t.Context(), c, cfg, os.Environ())
	if err != nil || v.Result != "failed" || v.Checks[0].Status != "failed" || *v.Checks[0].ExitCode != 0 {
		t.Fatalf("verification = %+v, err = %v", v, err)
	}
}

func TestMissingExecutableUnavailable(t *testing.T) {
	c, cfg := verifyFixture(t)
	cfg.Checks = []admission.CheckConfig{
		{Name: "missing", Argv: []string{filepath.Join(t.TempDir(), "missing-check")}, Timeout: "1s"},
		{Name: "later", Argv: []string{os.Args[0], "__verify_helper", "pass"}, Timeout: "5s"},
	}
	v, err := integration.RunChecks(t.Context(), c, cfg, os.Environ())
	if err != nil || v.Result != "failed" || v.Checks[0].Status != "unavailable" || v.Checks[1].Status != "not_run" {
		t.Fatalf("verification = %+v, err = %v", v, err)
	}
}

func TestKeepGoingAfterUnavailable(t *testing.T) {
	c, cfg := verifyFixture(t)
	cfg.Checks = []admission.CheckConfig{
		{Name: "missing", Argv: []string{"mythhelm-tool-that-does-not-exist"}, Timeout: "1s"},
		{Name: "later", Argv: []string{os.Args[0], "__verify_helper", "pass"}, Timeout: "5s"},
	}
	v, err := integration.RunChecksWithOptions(t.Context(), c, cfg, os.Environ(), true)
	if err != nil || v.Result != "failed" || v.Checks[0].Status != "unavailable" || v.Checks[1].Status != "passed" {
		t.Fatalf("verification = %+v, err = %v", v, err)
	}
}

func TestCheckTimeoutKillsGroup(t *testing.T) {
	c, cfg := verifyFixture(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "child-survived")
	ready := filepath.Join(dir, "child-started")
	// The kill fires only after the grandchild proves it exists (ready file),
	// so a slow spawn on a loaded runner cannot slip past the tree-snapshot
	// kill on Windows. The 30s check timeout is a backstop that must never
	// fire; the spawn parent sleeps 60s to outlive it.
	cfg.Checks[0] = admission.CheckConfig{Name: "timeout", Argv: []string{os.Args[0], "__verify_helper", "spawn", marker, ready, "3s"}, Timeout: "30s"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		// Cancel only on observed readiness: when ready never arrives the
		// 30s check timeout stays the kill trigger, and the explicit
		// readiness assertion below fails the test as inconclusive.
		deadline := time.Now().Add(25 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(ready); err == nil {
				cancel()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	v, err := integration.RunChecks(ctx, c, cfg, os.Environ())
	if err != nil || v.Checks[0].Status != "timed_out" {
		t.Fatalf("verification = %+v, err = %v", v, err)
	}
	if _, err := os.Stat(ready); err != nil {
		t.Fatalf("grandchild never started before the kill; test inconclusive: %v", err)
	}
	// The grandchild writes the marker 3s after startup when it survives, so
	// a 5s watch after the kill still fails the test on a missed kill.
	time.Sleep(5 * time.Second)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("check child survived process-group kill: %v", err)
	}
}

func TestCheckWaitDelayBounded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows cannot signal a process group after its leader exits")
	}
	c, cfg := verifyFixture(t)
	cfg.Checks[0] = admission.CheckConfig{Name: "linger", Argv: []string{os.Args[0], "__verify_helper", "linger_parent"}, Timeout: "8s"}
	start := time.Now()
	v, err := integration.RunChecks(t.Context(), c, cfg, os.Environ())
	if err != nil || v.Checks[0].Status != "passed" || time.Since(start) < 3*time.Second || time.Since(start) > 6*time.Second {
		t.Fatalf("verification = %+v, duration = %s, err = %v", v, time.Since(start), err)
	}
	c, cfg = verifyFixture(t)
	marker := filepath.Join(t.TempDir(), "failed-child-survived")
	cfg.Checks[0] = admission.CheckConfig{Name: "failed_linger", Argv: []string{os.Args[0], "__verify_helper", "linger_failed_parent", marker}, Timeout: "200ms"}
	v, err = integration.RunChecks(t.Context(), c, cfg, os.Environ())
	if err != nil || v.Checks[0].Status != "timed_out" {
		t.Fatalf("failed leader = %+v, err = %v", v, err)
	}
	time.Sleep(1200 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("failed leader's child survived: %v", err)
	}
}

func TestEvidenceRedactedAndHashed(t *testing.T) {
	c, cfg := verifyFixture(t)
	cfg.Checks[0] = admission.CheckConfig{Name: "secret", Argv: []string{os.Args[0], "__verify_helper", "secret"}, Timeout: "2s"}
	v, err := integration.RunChecks(t.Context(), c, cfg, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	r := v.Checks[0]
	b, err := os.ReadFile(r.EvidencePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "pretend-secret-value") || !strings.Contains(string(b), "[REDACTED]") {
		t.Fatalf("evidence was not redacted: %q", b)
	}
	sum := sha256.Sum256(b)
	if r.EvidenceSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("evidence hash %s != %x", r.EvidenceSHA256, sum)
	}
}

func TestEvidenceDirectorySymlinkRejected(t *testing.T) {
	c, cfg := verifyFixture(t)
	runDir := filepath.Dir(c.Workspace)
	if err := os.Symlink(t.TempDir(), filepath.Join(runDir, "evidence")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := integration.RunChecks(t.Context(), c, cfg, os.Environ()); err == nil {
		t.Fatal("verification followed an agent-planted evidence symlink")
	}
}

func requireBoundary(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the check boundary is Linux-only in v1")
	}
	if a := contain.ProbeLinux(); !a.Supported {
		t.Skipf("boundary unavailable here: %s", a.Reason)
	}
}

// outsideTmp makes a directory outside /tmp, which the boundary replaces.
func outsideTmp(t *testing.T) string {
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

func shCheck(name, script string, args ...string) admission.CheckConfig {
	return admission.CheckConfig{Name: name, Argv: append([]string{"/bin/sh", "-c", script, "sh"}, args...), Timeout: "10s"}
}

func TestRestrictedChecksRunContained(t *testing.T) {
	requireBoundary(t)
	c, cfg := verifyFixture(t)
	outside, home := outsideTmp(t), outsideTmp(t)
	marker := filepath.Join(outside, "pwned")
	cfg.Checks = []admission.CheckConfig{shCheck("write-outside", `echo x > "$1"`, marker)}
	env := []string{"PATH=/usr/bin:/bin", "HOME=" + home}

	v, err := integration.RunChecks(t.Context(), c, cfg, env)
	if err != nil || v.Result != "passed" {
		t.Fatalf("host policy: %+v, %v; want the write to pass", v, err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("the host check did not write outside the worktree; the fixture proves nothing: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}

	// One verification per candidate: the contained run gets its own.
	c, _ = verifyFixture(t)
	v, err = integration.RunChecksWithPolicy(t.Context(), c, cfg, env, integration.RunOptions{Policy: &contain.Policy{Profile: "restricted"}})
	if err != nil {
		t.Fatal(err)
	}
	if v.Result != "failed" || v.Checks[0].Status != "failed" {
		t.Fatalf("contained verification = %s / %s, want the candidate failed, never accepted", v.Result, v.Checks[0].Status)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a contained check wrote outside the worktree")
	}
	if v.Evaluator.Name != contain.BoundaryName {
		t.Errorf("evaluator = %+v, want the boundary's name", v.Evaluator)
	}

	// Control: the boundary runs an ordinary check, so the failure above came
	// from the confinement and not from a broken setup.
	c, _ = verifyFixture(t)
	cfg.Checks = []admission.CheckConfig{shCheck("noop", "true")}
	v, err = integration.RunChecksWithPolicy(t.Context(), c, cfg, env, integration.RunOptions{Policy: &contain.Policy{Profile: "restricted"}})
	if err != nil || v.Result != "passed" {
		t.Fatalf("contained no-op check: %+v, %v; want passed", v, err)
	}
}

func TestCheckArgvStillAdmittedOnly(t *testing.T) {
	requireBoundary(t)
	outside, home := outsideTmp(t), outsideTmp(t)
	marker := filepath.Join(outside, "candidate-script-ran")
	// The candidate adds a script of its own and rewrites mythhelm.toml
	// (verifyFixture); neither is an admitted definition.
	c, cfg := verifyFixtureWith(t, map[string]string{"evil.sh": "#!/bin/sh\necho x > " + marker + "\n"})
	env := []string{"PATH=/usr/bin:/bin", "HOME=" + home}

	v, err := integration.RunChecks(t.Context(), c, cfg, env)
	if err != nil || len(v.Checks) != len(cfg.Checks) || v.Checks[0].Name != "admitted" {
		t.Fatalf("verification = %+v, %v; want exactly the admitted checks", v, err)
	}
	if !reflect.DeepEqual(v.Checks[0].Argv, cfg.Checks[0].Argv) {
		t.Errorf("executed argv = %q, want the admitted %q", v.Checks[0].Argv, cfg.Checks[0].Argv)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a script the candidate added ran without being admitted")
	}

	// Even an admitted check that points at the candidate's script cannot
	// escape the check policy: the script's write outside the worktree fails.
	cfg.Checks = []admission.CheckConfig{{Name: "candidate-script", Argv: []string{"./evil.sh"}, Timeout: "10s"}}
	c, _ = verifyFixtureWith(t, map[string]string{"evil.sh": "#!/bin/sh\necho x > " + marker + "\n"})
	v, err = integration.RunChecksWithPolicy(t.Context(), c, cfg, env, integration.RunOptions{Policy: &contain.Policy{Profile: "restricted"}})
	if err != nil || v.Result != "failed" {
		t.Fatalf("contained candidate script: %+v, %v; want failed", v, err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the candidate's script wrote outside the worktree")
	}
}

func TestEvaluatorDigestRecorded(t *testing.T) {
	_, cfg := verifyFixture(t)
	run := func(cfg admission.ProjectConfig) contain.Evaluator {
		t.Helper()
		c, _ := verifyFixture(t) // one verification per candidate
		v, err := integration.RunChecks(t.Context(), c, cfg, os.Environ())
		if err != nil || v.Evaluator.Digest == "" {
			t.Fatalf("verification = %+v, %v; want an evaluator digest", v, err)
		}
		return v.Evaluator
	}
	first := run(cfg)
	if again := run(cfg); again != first {
		t.Errorf("same checks, different evaluators: %+v vs %+v", first, again)
	}
	changed := admission.ProjectConfig{Checks: []admission.CheckConfig{cfg.Checks[0]}}
	changed.Checks[0].Timeout = "3s"
	if other := run(changed); other.Digest == first.Digest {
		t.Errorf("a changed check definition kept digest %s: an old row would still apply", first.Digest)
	}
}

func TestEvaluatorDigestRepeatable(t *testing.T) {
	base := contain.Policy{Profile: "restricted", Workdir: "/runs/a/verify", ReadOnly: true, ProxyAddr: "127.0.0.1:1111",
		AuthBinds: []contain.AuthBind{{Source: "/srv/a/.token", Target: "/scratch/token"}, {Source: "/x/y", Target: "/scratch/aa"}}}
	checks := []string{"b", "a"}
	want := contain.EvaluatorDigest("b", "1", base, checks)

	moved := base
	moved.Workdir, moved.ProxyAddr = "/runs/b/verify", "127.0.0.1:2222"
	moved.AuthBinds = []contain.AuthBind{{Source: "/other/1", Target: "/scratch/aa"}, {Source: "/other/2", Target: "/scratch/token"}}
	if got := contain.EvaluatorDigest("b", "1", moved, []string{"a", "b"}); got != want {
		t.Errorf("runtime-specific values changed the digest: %+v vs %+v", got, want)
	}

	rw := base
	rw.ReadOnly = false
	for name, got := range map[string]contain.Evaluator{
		"read-only flag":   contain.EvaluatorDigest("b", "1", rw, checks),
		"one check digest": contain.EvaluatorDigest("b", "1", base, []string{"a", "c"}),
		"boundary version": contain.EvaluatorDigest("b", "2", base, checks),
	} {
		if got.Digest == want.Digest {
			t.Errorf("changing the %s kept the digest", name)
		}
	}
}

func TestUnavailableChecksStayUnverified(t *testing.T) {
	c, cfg := verifyFixture(t)
	cfg.Checks = []admission.CheckConfig{{Name: "missing", Argv: []string{filepath.Join(t.TempDir(), "no-such-check")}, Timeout: "2s"}}
	v, err := integration.RunChecks(t.Context(), c, cfg, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if v.Result == "passed" || v.Checks[0].Status != "unavailable" || v.Checks[0].ExitCode != nil {
		t.Fatalf("verification = %+v; want an unavailable check and no acceptance", v)
	}
	if v.CandidateCommit != c.Commit {
		t.Errorf("verification names %s, want the preserved candidate %s", v.CandidateCommit, c.Commit)
	}
}
