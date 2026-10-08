package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/qualify"
)

// seedQualificationRecords migrates the env's state dir and seeds the seven
// v2 §7.2 records. Test-side seeding covers display only: the production
// trigger (supervisor seeding on admitted runs) is Task 9's work.
func seedQualificationRecords(t *testing.T, e env) {
	t.Helper()
	j, err := journal.Open(context.Background(), e.state)
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	if err := qualify.EnsureSeeded(context.Background(), j); err != nil {
		_ = j.Close()
		t.Fatalf("EnsureSeeded: %v", err)
	}
	if err := j.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// wantQualificationProgress is the honest seed reading: claude-code blocked
// behind its next authorised test, every other harness planned, none
// fixture-tested or better.
var wantQualificationProgress = map[string]string{
	"claude-code": "blocked",
	"codex":       "planned",
	"opencode":    "planned",
	"muse":        "planned",
	"kimi":        "planned",
	"cursor":      "planned",
	"antigravity": "planned",
}

// checkDoctorRecord requires the per-record JSONL shape Task 6 ships:
// progress, per-column verdict + evidence ids, evidence revisions and drift
// triggers.
func checkDoctorRecord(m map[string]any) (harness, progress string, err error) {
	for _, key := range []string{"harness", "surface", "progress", "drift", "latest_evidence", "next_test"} {
		s, ok := m[key].(string)
		if !ok || s == "" {
			return "", "", &fieldError{field: key}
		}
	}
	for _, col := range []string{"fidelity", "entitlement", "lifecycle"} {
		cm, ok := m[col].(map[string]any)
		if !ok {
			return "", "", &fieldError{field: col}
		}
		v, ok := cm["verdict"].(string)
		if !ok || (v != "proven" && v != "not-proven" && v != "unknown") {
			return "", "", &fieldError{field: col + ".verdict"}
		}
		ev, ok := cm["evidence"].([]any)
		if !ok {
			return "", "", &fieldError{field: col + ".evidence"}
		}
		for _, id := range ev {
			if _, ok := id.(string); !ok {
				return "", "", &fieldError{field: col + ".evidence"}
			}
		}
	}
	if _, ok := m["evidence_revisions"].(float64); !ok {
		return "", "", &fieldError{field: "evidence_revisions"}
	}
	dt, ok := m["drift_triggers"].(map[string]any)
	if !ok {
		return "", "", &fieldError{field: "drift_triggers"}
	}
	for _, key := range []string{"executable_digest", "config_digest"} {
		if _, ok := dt[key].(string); !ok {
			return "", "", &fieldError{field: "drift_triggers." + key}
		}
	}
	return m["harness"].(string), m["progress"].(string), nil
}

type fieldError struct{ field string }

func (e *fieldError) Error() string { return "record lacks " + e.field }

// TestE2EDoctorShowsSevenRecords runs the packaged binary's doctor against
// a seeded home: JSONL lists the seven v2 §7.2 records with honest
// progress values.
func TestE2EDoctorShowsSevenRecords(t *testing.T) {
	e := newEnv(t)
	seedQualificationRecords(t, e)
	code, stdout, stderr := run(t, e, t.TempDir(), "doctor", "--format", "jsonl")
	if code != 0 {
		t.Fatalf("doctor exit %d, stderr %q", code, stderr)
	}
	if strings.Count(stdout, "\n") != 1 {
		t.Fatalf("jsonl output is not one object: %q", stdout)
	}
	var rep map[string]any
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatalf("doctor jsonl: %v\n%s", err, stdout)
	}
	if rep["type"] != "doctor" {
		t.Fatalf("jsonl type = %v, want doctor", rep["type"])
	}
	qual, ok := rep["qualification"].(map[string]any)
	if !ok {
		t.Fatalf("jsonl object has no qualification map: %q", stdout)
	}
	raw, ok := qual["records"].([]any)
	if !ok || len(raw) != 7 {
		t.Fatalf("qualification records = %v, want the 7 seeded records", qual["records"])
	}
	seen := map[string]string{}
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			t.Errorf("record is not an object: %v", item)
			continue
		}
		harness, progress, err := checkDoctorRecord(m)
		if err != nil {
			t.Errorf("record fails the field check: %v (harness %v)", err, m["harness"])
			continue
		}
		seen[harness] = progress
	}
	if len(seen) != 7 {
		t.Fatalf("record harnesses = %v, want the 7 v2 §7.2 ids", seen)
	}
	for harness, want := range wantQualificationProgress {
		if seen[harness] != want {
			t.Errorf("%s progress = %q, want honest %q", harness, seen[harness], want)
		}
	}
}

// TestE2EStrictStillBlocked runs the packaged binary's strict claudecode
// path against a fixture native and a missing registry: it still refuses,
// exit 3, now with the consult's no_qualification_record (a missing registry
// skips the source-gap check, so the pin holds before and after the gap
// resolves).
func TestE2EStrictStillBlocked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the claudecode probe refuses Windows by design")
	}
	e := newEnv(t)
	repo, digest := mkRepo(t, e, t.TempDir(), "pass")
	configDir := filepath.Join(e.home, "claude-config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	fakeDir := installFakeClaude(t, configDir)
	env := append(childEnvWithPath(t, e, fakeDir), "MYTHHELM_HOME="+filepath.Join(e.home, "absent-state"))
	code, stdout, stderr := runBin(t, env, repo, mythhelmBin, "run",
		"--task-file", "task.md", "--adapter", "claudecode",
		"--billing", "subscription-only", "--execution-profile", "trusted-host",
		"--non-interactive", "--trust-project-config", "sha256:"+digest,
		"--strip-credential-env", "--format", "jsonl")
	if code != 3 {
		t.Fatalf("strict run exit %d, want 3; stderr %q\nstdout:\n%s", code, stderr, stdout)
	}
	_, res := runResultOf(t, stdout)
	if res["reason"] != "no_qualification_record" {
		t.Errorf("run.result reason = %v, want no_qualification_record", res["reason"])
	}
	if res["exit_code"] != float64(3) {
		t.Errorf("run.result exit_code = %v, want 3", res["exit_code"])
	}
	if strings.TrimSpace(stderr) == "" {
		t.Error("stderr is empty; the refusal must surface there")
	}
}

// installFakeClaude writes an executable `claude` fixture answering
// --version and `auth status` from baked-in fixture values, and the native
// launch with a minimal successful stream-json session: an init frame on
// the subscription route (apiKeySource none, per AC-4.4) then a success
// result. The launch leg validates the adapter's fixed argv (Argv in
// adapters/claudecode/launch.go) before emitting the stream, permitting
// the optional --allowedTools tail, and exits nonzero on anything else,
// so the test fails if the binary ever misconstructs the native command.
// It performs no live call and reads no secret.
// It returns the directory to prepend to PATH.
func installFakeClaude(t *testing.T, configDir string) string {
	t.Helper()
	if strings.ContainsAny(configDir, "'\\") {
		t.Fatalf("config dir %q is not safe to bake into the fixture script", configDir)
	}
	script := "#!/bin/sh\n" +
		"# Fixture native for TestE2EDeclaredUnaffected: fixture answers only.\n" +
		"if [ \"$1\" = \"--version\" ]; then\n" +
		"    printf '%s\\n' '2.1.284 (Claude Code)'\n" +
		"    exit 0\n" +
		"fi\n" +
		"if [ \"$1\" = \"auth\" ] && [ \"$2\" = \"status\" ]; then\n" +
		"    printf '%s\\n' '{\"loggedIn\":true,\"authMethod\":\"claude.ai\",\"apiProvider\":\"firstParty\",\"subscriptionType\":\"max\",\"configDirectory\":\"" + configDir + "\",\"orgId\":\"org-e2e-declared-unaffected\"}'\n" +
		"    exit 0\n" +
		"fi\n" +
		"if [ \"$1\" != \"-p\" ] || [ \"$2\" != \"--output-format\" ] || [ \"$3\" != \"stream-json\" ] || [ \"$4\" != \"--verbose\" ] || [ \"$5\" != \"--input-format\" ] || [ \"$6\" != \"text\" ] || [ \"$7\" != \"--permission-mode\" ] || [ \"$8\" != \"acceptEdits\" ] || [ \"$9\" != \"--permission-prompts\" ] || [ \"${10}\" != \"none\" ]; then\n" +
		"    printf '%s\\n' 'unexpected claude arguments' >&2\n" +
		"    exit 2\n" +
		"fi\n" +
		"if [ \"$#\" -gt 10 ] && { [ \"${11}\" != \"--allowedTools\" ] || [ \"$#\" -lt 12 ]; }; then\n" +
		"    printf '%s\\n' 'unexpected claude arguments' >&2\n" +
		"    exit 2\n" +
		"fi\n" +
		"printf '%s\\n' '{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"sess-e2e-declared\",\"apiKeySource\":\"none\",\"model\":\"e2e-fixture\",\"claude_code_version\":\"2.1.284\",\"permissionMode\":\"acceptEdits\"}'\n" +
		"printf '%s\\n' '{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false}'\n" +
		"exit 0\n"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o700); err != nil { // #nosec G306 -- executable fixture native under t.TempDir
		t.Fatal(err)
	}
	return dir
}

// childEnvWithPath returns e's child environment with dir prepended to PATH,
// replacing the inherited PATH entries so the fake native resolves first.
func childEnvWithPath(t *testing.T, e env, dir string) []string {
	t.Helper()
	var out []string
	for _, kv := range e.withParent(t) {
		if name, _, _ := strings.Cut(kv, "="); !strings.EqualFold(name, "PATH") {
			out = append(out, kv)
		}
	}
	return append(out, "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// runBin executes bin to completion in dir with env, mirroring run() for
// invocations that need more than the default child environment.
func runBin(t *testing.T, env []string, dir, bin string, args ...string) (exit int, stdout, stderr string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = env
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	_ = cmd.Run()
	if cmd.ProcessState == nil {
		t.Fatalf("%s %s did not run", bin, strings.Join(args, " "))
	}
	return cmd.ProcessState.ExitCode(), out.String(), errOut.String()
}

// TestE2EDeclaredUnaffected runs the packaged binary's user-declared
// dogfood path (claudecode with subscription-declared) against a fixture
// native: admission still admits and the run completes exactly as before
// this spec — exit 0, ready_for_review. The registry consult attaches no
// record (seed rows never stable-match a real probe) and blocks nothing.
func TestE2EDeclaredUnaffected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the claudecode probe refuses Windows by design; the unit resolve() legs cover the mapping there")
	}
	e := newEnv(t)
	repo, digest := mkRepo(t, e, t.TempDir(), "pass")
	configDir := filepath.Join(e.home, "claude-config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	fakeDir := installFakeClaude(t, configDir)
	args := []string{"run", "--task-file", "task.md", "--adapter", "claudecode",
		"--billing", "subscription-declared", "--execution-profile", "trusted-host",
		"--non-interactive", "--trust-project-config", "sha256:" + digest,
		"--declare-entitlement", "plan=max,extra-usage=disabled",
		"--strip-credential-env", "--format", "jsonl"}
	code, stdout, stderr := runBin(t, childEnvWithPath(t, e, fakeDir), repo, mythhelmBin, args...)
	if code != 0 {
		t.Fatalf("declared run exit %d, want 0; stderr %q\nstdout:\n%s", code, stderr, stdout)
	}
	runID, res := runResultOf(t, stdout)
	if runID == "" {
		t.Fatal("run.result has no run_id")
	}
	if res["state"] != "ready_for_review" {
		t.Fatalf("run.result state = %v, want ready_for_review", res["state"])
	}
}
