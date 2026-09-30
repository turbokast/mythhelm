package integration_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/integration"
	"github.com/turbokast/mythhelm/internal/workspace"
)

func TestMain(m *testing.M) {
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
			child := exec.Command(os.Args[0], "__verify_helper", "delayed", os.Args[3]) // #nosec G702 -- test binary re-exec with test-owned path
			if err := child.Start(); err != nil {
				os.Exit(2)
			}
			time.Sleep(10 * time.Second)
		case "delayed":
			time.Sleep(time.Second)
			_ = os.WriteFile(os.Args[3], []byte("survived"), 0o600) // #nosec G703 -- marker path is a test-owned temporary file
		case "linger_parent":
			child := exec.Command(os.Args[0], "__verify_helper", "linger_child") // #nosec G702 -- test binary re-exec
			child.Stdout, child.Stderr = os.Stdout, os.Stderr
			if err := child.Start(); err != nil {
				os.Exit(2)
			}
		case "linger_child":
			time.Sleep(10 * time.Second)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func verifyFixture(t *testing.T) (integration.Candidate, admission.ProjectConfig) {
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
	marker := filepath.Join(t.TempDir(), "child-survived")
	cfg.Checks[0] = admission.CheckConfig{Name: "timeout", Argv: []string{os.Args[0], "__verify_helper", "spawn", marker}, Timeout: "200ms"}
	v, err := integration.RunChecks(context.Background(), c, cfg, os.Environ())
	if err != nil || v.Checks[0].Status != "timed_out" {
		t.Fatalf("verification = %+v, err = %v", v, err)
	}
	time.Sleep(1200 * time.Millisecond)
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
	if err != nil || v.Checks[0].Status != "timed_out" || time.Since(start) > 5*time.Second {
		t.Fatalf("verification = %+v, duration = %s, err = %v", v, time.Since(start), err)
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
