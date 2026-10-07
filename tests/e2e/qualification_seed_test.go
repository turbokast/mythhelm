package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestE2EFreshHomeSeedsOnRun runs the packaged binary's fake-adapter run
// against a fresh home with no test-side seeding: the admitted run seeds
// the registry itself (Task 9's production trigger), and doctor then
// lists the seven v2 §7.2 records with honest progress values.
func TestE2EFreshHomeSeedsOnRun(t *testing.T) {
	e := newEnv(t)
	repo, digest := mkRepo(t, e, t.TempDir(), "pass")
	args := []string{"run", "--task-file", "task.md", "--adapter", "fake",
		"--billing", "local-scripted", "--execution-profile", "trusted-host",
		"--non-interactive", "--trust-project-config", "sha256:" + digest,
		"--format", "jsonl"}
	code, stdout, stderr := run(t, e, repo, args...)
	if code != 0 {
		t.Fatalf("run exit %d, stderr %q\nstdout:\n%s", code, stderr, stdout)
	}
	if _, res := runResultOf(t, stdout); res["state"] != "ready_for_review" {
		t.Fatalf("run.result state = %v, want ready_for_review", res["state"])
	}
	code, stdout, stderr = run(t, e, repo, "doctor", "--format", "jsonl")
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
