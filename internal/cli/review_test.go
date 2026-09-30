package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

func reviewFixture(t *testing.T, malicious string) (string, string) {
	t.Helper()
	dir := stateHome(t)
	j := openJournal(t, dir)
	defer func() { _ = j.Close() }()
	runID := createRun(t, j, supervisor.NewProducer(ids.New("sup"), 1), "/tmp/repo")
	runDir := filepath.Join(dir, "runs", runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	r := supervisor.Receipt{
		"schema_version": 1, "run_id": runID, "state": "ready_for_review", "exit_code": 0,
		"requested_outcome": map[string]any{"title": "Demo task"},
		"candidate":         map[string]any{"commit": "unknown", "flags": []any{map[string]any{"path": malicious, "flag": "large_file"}}},
		"verification":      map[string]any{"checks": []any{map[string]any{"name": "test", "status": "passed", "evidence": malicious, "sha256": "abc"}}},
	}
	sha, err := supervisor.WriteReceipt(runDir, r)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"schema_version": 1, "sha256": sha, "path": "receipt.json"})
	if err != nil {
		t.Fatal(err)
	}
	err = j.Append(t.Context(), journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"), RunID: runID,
		ProducerID: ids.New("sup"), ProducerSequence: 1, Generation: 1,
		ObservedAt: time.Now().UTC(), Type: "receipt.written", Payload: payload,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return dir, runID
}

func TestReviewSanitisesMaliciousFilename(t *testing.T) {
	_, runID := reviewFixture(t, "evil\x1b]8;;https://example.test\a.txt")
	code, out, stderr := runMain("review", runID, "--no-diff")
	if code != 0 {
		t.Fatalf("review exit %d: %s", code, stderr)
	}
	if strings.Contains(out, "\x1b") || strings.Contains(out, "\a") {
		t.Fatalf("terminal control reached output: %q", out)
	}
	if !strings.Contains(out, "evil") || !strings.Contains(out, "Checks:") {
		t.Fatalf("review omitted evidence: %q", out)
	}
}

func TestReviewJSONLSingleObject(t *testing.T) {
	_, runID := reviewFixture(t, "safe.txt")
	code, out, stderr := runMain("review", "--format", "jsonl", runID)
	if code != 0 {
		t.Fatalf("review exit %d: %s", code, stderr)
	}
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("JSONL must have one line: %q", out)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	if m["run_id"] != runID || m["schema_version"] != float64(1) {
		t.Fatalf("wrong receipt: %v", m)
	}
}

func TestReviewRejectsTamperedReceipt(t *testing.T) {
	dir, runID := reviewFixture(t, "safe.txt")
	p := filepath.Join(dir, "runs", runID, "receipt.json")
	if err := os.WriteFile(p, []byte(`{"run_id":"forged"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runMain("review", runID, "--format", "jsonl")
	if code == 0 || !strings.Contains(stderr, "does not match") {
		t.Fatalf("tampered receipt exit %d: %s", code, stderr)
	}
}
