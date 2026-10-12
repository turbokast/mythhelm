package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// codexFixtureMain is the synthetic native Codex: a copy of this test binary
// installed as `codex`. It appends its argv to invocations.log beside itself,
// then answers --version with the synthetic fixture version. It never contacts
// a model and has no other behaviour: any other argv exits 9, so a run that
// reached a native launch would fail loudly (D13).
func codexFixtureMain() int {
	exe, err := os.Executable()
	if err != nil {
		return 9
	}
	log, err := os.OpenFile(filepath.Join(filepath.Dir(exe), "invocations.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 -- fixture log beside the copied test binary
	if err != nil {
		return 9
	}
	_, _ = fmt.Fprintln(log, strings.Join(os.Args[1:], " "))
	_ = log.Close()
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("codex-cli 0.0.0")
		return 0
	}
	return 9
}

// installFakeCodex copies the test binary to a fresh directory as `codex` and
// returns that directory; TestMain dispatches the copy to codexFixtureMain.
func installFakeCodex(t *testing.T) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(self) // #nosec G304 -- the test copies its own binary from os.Executable
	if err != nil {
		t.Fatal(err)
	}
	name := "codex"
	if runtime.GOOS == "windows" {
		name = "codex.exe"
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o700); err != nil { // #nosec G306 G703 -- executable test fixture under t.TempDir
		t.Fatal(err)
	}
	return dir
}

// fixtureInvocations returns the argv lines the fixture native logged, empty
// when it never ran.
func fixtureInvocations(t *testing.T, dir string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "invocations.log")) // #nosec G304 -- fixture log under t.TempDir
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

// TestCodexBillingModesAllRefuseOrBlock runs the packaged binary with
// --adapter codex under trusted-host in every billing mode against a fixture
// native: none is admitted (I02, I15 (v2 §2)). subscription-only reaches the
// probe and blocks on the unknown auth category (D13); the other two modes
// block before any native process starts (D3).
func TestCodexBillingModesAllRefuseOrBlock(t *testing.T) {
	tests := []struct {
		name       string
		billing    string
		wantReason string
		wantNative []string
	}{
		{name: "subscription-only blocks on the unknown auth category after the probe", billing: "subscription-only",
			wantReason: "no_qualification_record", wantNative: []string{"--version"}},
		{name: "subscription-declared is unsupported for Codex before any native process", billing: "subscription-declared",
			wantReason: "billing_unsupported_for_adapter"},
		{name: "local-scripted is unsupported for Codex before any native process", billing: "local-scripted",
			wantReason: "billing_unsupported_for_adapter"},
	}
	for _, tc := range tests {
		t.Run(tc.billing, func(t *testing.T) {
			e := newEnv(t)
			repo, digest := mkRepo(t, e, t.TempDir(), "pass")
			fakeDir := installFakeCodex(t)
			code, stdout, stderr := runBin(t, childEnvWithPath(t, e, fakeDir), repo, mythhelmBin, "run",
				"--task-file", "task.md", "--adapter", "codex",
				"--billing", tc.billing, "--execution-profile", "trusted-host",
				"--non-interactive", "--trust-project-config", "sha256:"+digest,
				"--format", "jsonl")
			if code != 3 {
				t.Fatalf("exit %d, want 3 (blocked) for %s; stderr %q\nstdout:\n%s", code, tc.name, stderr, stdout)
			}
			if _, res := runResultOf(t, stdout); res["reason"] != tc.wantReason {
				t.Errorf("run.result reason = %v, want %s", res["reason"], tc.wantReason)
			}
			if !strings.Contains(stderr, tc.wantReason) {
				t.Errorf("stderr lacks %s:\n%s", tc.wantReason, stderr)
			}
			if got := fixtureInvocations(t, fakeDir); strings.Join(got, "|") != strings.Join(tc.wantNative, "|") {
				t.Errorf("fixture native invocations = %q, want %q", got, tc.wantNative)
			}
			if entries, err := os.ReadDir(filepath.Join(e.state, "runs")); err == nil && len(entries) != 0 {
				t.Errorf("a blocked run recorded %d runs", len(entries))
			}
		})
	}
}

// TestCodexDoctorRowEndToEnd runs the packaged binary's plain doctor against a
// state directory seeded through qualify.EnsureSeeded: the Codex row stays
// planned, names every column that is not proven, and is followed by its
// authorised next test (v2 §7.2).
func TestCodexDoctorRowEndToEnd(t *testing.T) {
	e := newEnv(t)
	seedQualificationRecords(t, e)
	code, stdout, stderr := run(t, e, t.TempDir(), "doctor")
	if code != 0 {
		t.Fatalf("doctor exit %d, stderr %q", code, stderr)
	}
	lines := strings.Split(stdout, "\n")
	row := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "codex × ") })
	if row < 0 {
		t.Fatalf("doctor output has no Codex row:\n%s", stdout)
	}
	if !strings.Contains(lines[row], ": planned (") {
		t.Errorf("Codex row is not planned: %q", lines[row])
	}
	if !strings.HasSuffix(lines[row], " failing: fidelity, entitlement, lifecycle") {
		t.Errorf("Codex row does not end with its failing columns: %q", lines[row])
	}
	const wantNext = "  next_test: Qualify native-managed sign-in, approvals and effort metadata; authority: maintainer live-test grant"
	if row+1 >= len(lines) || lines[row+1] != wantNext {
		t.Errorf("the line after the Codex row is not its seeded next_test:\n%s", stdout)
	}

}
