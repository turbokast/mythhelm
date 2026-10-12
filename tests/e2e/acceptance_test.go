package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/admission"
)

// credentialVars are the agent credential variables the credential-free
// flow runs without (I13, v2 §2).
var credentialVars = []string{"ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN", "OPENAI_API_KEY"}

const (
	declaredSentence = "entitlement: user-declared, NOT verified by MYTHHELM; paid continuation: unknown (user declares disabled)"
	scriptedSentence = "entitlement: not applicable (local-scripted: no inference, no network)"
)

// credentialFreeEnv is e's child environment with every credential
// variable removed, asserted absent before any child starts.
func credentialFreeEnv(t *testing.T, e env) []string {
	t.Helper()
	out := slices.DeleteFunc(e.withParent(t), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return slices.Contains(credentialVars, name)
	})
	for _, kv := range out {
		name, _, _ := strings.Cut(kv, "=")
		if slices.Contains(credentialVars, name) {
			t.Fatalf("child environment still carries %s", name)
		}
	}
	return out
}

// acceptanceRepo is mkRepo with the check configuration replaced by argv
// (committed), returning the repository and the digest the run must trust.
func acceptanceRepo(t *testing.T, e env, argv ...string) (repo, digest string) {
	t.Helper()
	repo, _ = mkRepo(t, e, t.TempDir(), "pass")
	quoted := make([]string, len(argv))
	for i, a := range argv {
		quoted[i] = strconv.Quote(a)
	}
	config := "schema_version = 1\n[[checks]]\nname = \"e2e\"\nargv = [" + strings.Join(quoted, ", ") + "]\ntimeout = \"30s\"\n"
	if err := os.WriteFile(filepath.Join(repo, "mythhelm.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "mythhelm.toml"}, {"commit", "--quiet", "-m", "checks"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = e.withParent(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	_, digest, err := admission.ParseProjectConfig([]byte(config))
	if err != nil {
		t.Fatal(err)
	}
	return repo, digest
}

// fakeRunArgs is the packaged fake-route run: scripted billing, trusted
// host, the repository's trusted checks, the plain stream.
func fakeRunArgs(digest string) []string {
	return []string{"run", "--task-file", "task.md", "--adapter", "fake",
		"--billing", "local-scripted", "--execution-profile", "trusted-host",
		"--non-interactive", "--trust-project-config", "sha256:" + digest, "--plain"}
}

// receiptOf reads the run's receipt.json as a generic document.
func receiptOf(t *testing.T, e env, runID string) map[string]any {
	t.Helper()
	var r map[string]any
	if err := json.Unmarshal(ledgerReceiptBytes(t, e, runID), &r); err != nil {
		t.Fatalf("receipt of %s: %v", runID, err)
	}
	return r
}

// at walks a receipt by object keys and requires every step to be an object
// except the last, so a missing key names itself in the failure.
func at(t *testing.T, doc map[string]any, path ...string) any {
	t.Helper()
	var cur any = doc
	for _, key := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("receipt path %v: %q is not under an object", path, key)
		}
		if cur, ok = m[key]; !ok {
			t.Fatalf("receipt has no %v", path)
		}
	}
	return cur
}

// lineWithPrefix returns the first line of out that starts with prefix.
func lineWithPrefix(t *testing.T, out, prefix string) string {
	t.Helper()
	for line := range strings.Lines(out) {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimRight(line, "\r\n")
		}
	}
	t.Fatalf("no line starting %q in:\n%s", prefix, out)
	return ""
}

// hasLine reports whether out has a line exactly equal to want.
func hasLine(out, want string) bool {
	for line := range strings.Lines(out) {
		if strings.TrimRight(line, "\r\n") == want {
			return true
		}
	}
	return false
}

// candidateOf reads the candidate commit a plain review prints.
func candidateOf(t *testing.T, review string) string {
	t.Helper()
	commit := strings.TrimSpace(strings.TrimPrefix(lineWithPrefix(t, review, "Candidate:"), "Candidate:"))
	if len(commit) != 40 {
		t.Fatalf("candidate %q is not a 40-hex commit", commit)
	}
	return commit
}

// TestE2EP1FlowNoCredentials runs the whole P1 flow on the packaged binary
// with no agent credential in the child environment (I13).
func TestE2EP1FlowNoCredentials(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	env := credentialFreeEnv(t, e)
	repo, digest := mkRepo(t, e, t.TempDir(), "pass")

	code, stdout, stderr := runBin(t, env, repo, mythhelmBin, fakeRunArgs(digest)...)
	if code != 0 {
		t.Fatalf("run exit %d, want 0; stderr %q\nstdout:\n%s", code, stderr, stdout)
	}
	runID := ledgerRunIDOf(t, stdout)
	code, review, stderr := runBin(t, env, repo, mythhelmBin, "review", runID)
	if code != 0 {
		t.Fatalf("review exit %d, want 0; stderr %q", code, stderr)
	}
	commit := candidateOf(t, review)
	if want := "Verification: passed (bound to " + commit + ")"; !hasLine(review, want) {
		t.Errorf("review has no line %q:\n%s", want, review)
	}
	if want := "Acceptance: bound"; !hasLine(review, want) {
		t.Errorf("review has no line %q:\n%s", want, review)
	}
	code, stdout, stderr = runBin(t, env, repo, mythhelmBin, "apply", runID, "--to-branch", "accepted")
	if code != 0 {
		t.Fatalf("apply exit %d, want 0; stderr %q\nstdout:\n%s", code, stderr, stdout)
	}

	receipt := receiptOf(t, e, runID)
	if got := at(t, receipt, "verification", "label"); got != "passed" {
		t.Errorf("receipt verification.label = %v, want passed", got)
	}
	if got := at(t, receipt, "acceptance", "policy_pin", "policy_id"); got != "P1" {
		t.Errorf("receipt acceptance.policy_pin.policy_id = %v, want P1", got)
	}
	cmd := exec.Command("git", "rev-parse", "--verify", "refs/heads/accepted")
	cmd.Dir = repo
	cmd.Env = e.withParent(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("apply left no accepted branch: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != commit {
		t.Errorf("accepted branch is at %s, want the reviewed candidate %s", got, commit)
	}
}

// TestE2EUnavailableChecksKeepCandidate: a check that cannot start leaves
// the candidate reviewable but unverified, and apply needs the user's
// explicit acceptance (I09: NOT RUN is never verified).
func TestE2EUnavailableChecksKeepCandidate(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	repo, digest := acceptanceRepo(t, e, filepath.Join(t.TempDir(), "no-such-check"))

	code, stdout, stderr := run(t, e, repo, fakeRunArgs(digest)...)
	if code != 5 {
		t.Fatalf("run exit %d, want 5; stderr %q\nstdout:\n%s", code, stderr, stdout)
	}
	runID := ledgerRunIDOf(t, stdout)
	code, review, stderr := run(t, e, repo, "review", runID)
	if code != 0 {
		t.Fatalf("review exit %d, want 0; stderr %q", code, stderr)
	}
	commit := candidateOf(t, review)
	if want := "Verification: NOT RUN (checks_unavailable)"; !hasLine(review, want) {
		t.Errorf("review has no line %q:\n%s", want, review)
	}
	if want := "Acceptance: unverified (checks_unavailable)"; !hasLine(review, want) {
		t.Errorf("review has no line %q:\n%s", want, review)
	}
	code, _, stderr = run(t, e, repo, "apply", runID, "--to-branch", "refused")
	if code != 3 {
		t.Fatalf("apply without --accept-unverified exit %d, want 3; stderr %q", code, stderr)
	}
	code, stdout, stderr = run(t, e, repo, "apply", runID, "--to-branch", "accepted", "--accept-unverified")
	if code != 0 {
		t.Fatalf("apply --accept-unverified exit %d, want 0; stderr %q\nstdout:\n%s", code, stderr, stdout)
	}
	cmd := exec.Command("git", "rev-parse", "--verify", "refs/heads/accepted")
	cmd.Dir = repo
	cmd.Env = e.withParent(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("apply left no accepted branch: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != commit {
		t.Errorf("accepted branch is at %s, want the kept candidate %s", got, commit)
	}
}

// TestE2EStaleVerdictBlocksApply: a verdict bound to one task contract does
// not apply after the task file changes (I07).
func TestE2EStaleVerdictBlocksApply(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	repo, digest := mkRepo(t, e, t.TempDir(), "pass")

	code, stdout, stderr := run(t, e, repo, fakeRunArgs(digest)...)
	if code != 0 {
		t.Fatalf("run exit %d, want 0; stderr %q\nstdout:\n%s", code, stderr, stdout)
	}
	runID := ledgerRunIDOf(t, stdout)
	task := filepath.Join(e.state, "runs", runID, "task.md")
	if err := os.WriteFile(task, []byte("# E2E task\n\nWrite something else.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, _, stderr = run(t, e, repo, "apply", runID, "--to-branch", "refused")
	if code != 3 {
		t.Fatalf("apply exit %d, want 3; stderr %q", code, stderr)
	}
	if !strings.Contains(stderr, "changed: contract") {
		t.Errorf("apply stderr does not name the changed dimension %q:\n%s", "changed: contract", stderr)
	}
	code, review, stderr := run(t, e, repo, "review", runID)
	if code != 0 {
		t.Fatalf("review exit %d, want 0; stderr %q", code, stderr)
	}
	if got := lineWithPrefix(t, review, "Acceptance:"); !strings.HasPrefix(got, "Acceptance: STALE (changed: contract)") {
		t.Errorf("review acceptance line = %q, want STALE (changed: contract)", got)
	}
}

// TestE2EDeclaredClaudeRunCarriesSentence: the user-declared Claude route
// states, on every surface, that MYTHHELM did not verify the entitlement
// (I15).
func TestE2EDeclaredClaudeRunCarriesSentence(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the claudecode probe refuses Windows by design; the unit resolve() legs cover the mapping there")
	}
	e := newEnv(t)
	repo, digest := mkRepo(t, e, t.TempDir(), "pass")
	configDir := filepath.Join(e.home, "claude-config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	env := childEnvWithPath(t, e, installFakeClaude(t, configDir))
	args := []string{"run", "--task-file", "task.md", "--adapter", "claudecode",
		"--billing", "subscription-declared", "--execution-profile", "trusted-host",
		"--non-interactive", "--trust-project-config", "sha256:" + digest,
		"--declare-entitlement", "plan=max,extra-usage=disabled",
		"--strip-credential-env", "--plain"}
	code, stdout, stderr := runBin(t, env, repo, mythhelmBin, args...)
	if code != 0 {
		t.Fatalf("declared run exit %d, want 0; stderr %q\nstdout:\n%s", code, stderr, stdout)
	}
	runID := ledgerRunIDOf(t, stdout)
	if !hasLine(stdout, "  "+declaredSentence) {
		t.Errorf("run stream admission block has no line %q:\n%s", "  "+declaredSentence, stdout)
	}
	if got := at(t, receiptOf(t, e, runID), "billing", "entitlement_statement"); got != declaredSentence {
		t.Errorf("receipt billing.entitlement_statement = %v, want %q", got, declaredSentence)
	}
	code, review, stderr := runBin(t, env, repo, mythhelmBin, "review", runID)
	if code != 0 {
		t.Fatalf("review exit %d, want 0; stderr %q", code, stderr)
	}
	if !hasLine(review, declaredSentence) {
		t.Errorf("review has no line %q:\n%s", declaredSentence, review)
	}
}

// TestE2EScriptedRunEntitlementNotApplicable: a scripted run makes no
// entitlement claim and says so (I15).
func TestE2EScriptedRunEntitlementNotApplicable(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	repo, digest := mkRepo(t, e, t.TempDir(), "pass")

	code, stdout, stderr := run(t, e, repo, fakeRunArgs(digest)...)
	if code != 0 {
		t.Fatalf("run exit %d, want 0; stderr %q\nstdout:\n%s", code, stderr, stdout)
	}
	runID := ledgerRunIDOf(t, stdout)
	if got := at(t, receiptOf(t, e, runID), "billing", "entitlement_statement"); got != scriptedSentence {
		t.Errorf("receipt billing.entitlement_statement = %v, want %q", got, scriptedSentence)
	}
}
