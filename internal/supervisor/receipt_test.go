package supervisor_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

func receiptRun(t *testing.T) (fixture, supervisor.Receipt, string) {
	t.Helper()
	f := newFixture(t)
	if code, _, stderr := f.run(t, f.fakeRun()...); code != 5 {
		t.Fatalf("run exit %d: %s", code, stderr)
	}
	id := f.onlyRun(t).RunID
	b, err := os.ReadFile(filepath.Join(f.state, "runs", id, "receipt.json")) // #nosec G304 -- fixture state and projected ID
	if err != nil {
		t.Fatal(err)
	}
	var r supervisor.Receipt
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	j, err := journal.OpenReadOnly(t.Context(), f.state)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	events, err := j.Events(t.Context(), id, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		if ev.Type == "admission.decided" && strings.Contains(string(ev.Payload), "Demo task") {
			t.Fatal("task title leaked into admission journal")
		}
		if ev.Type != "receipt.written" {
			continue
		}
		var m struct {
			SHA256 string `json:"sha256"`
		}
		if err := json.Unmarshal(ev.Payload, &m); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		if m.SHA256 != hex.EncodeToString(sum[:]) {
			t.Fatalf("journaled hash %s differs from receipt", m.SHA256)
		}
		return f, r, id
	}
	t.Fatal("receipt.written was not journaled")
	return fixture{}, nil, ""
}

func TestReceiptHasAllSection9Keys(t *testing.T) {
	_, r, _ := receiptRun(t)
	want := []string{"schema_version", "run_id", "state", "exit_code", "requested_outcome", "admitted_snapshot",
		"execution_bundle", "fidelity_differences", "native_configuration", "billing", "routing", "native_result",
		"candidate", "verification", "external_effects", "execution_host", "host_integration", "unknowns", "remaining_human_action"}
	got := make([]string, 0, len(r))
	for k := range r {
		got = append(got, k)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("section 9 keys = %v; want %v", got, want)
	}
	if r["schema_version"] != float64(1) {
		t.Fatalf("schema_version = %v", r["schema_version"])
	}
	if r["requested_outcome"].(map[string]any)["title"] != "Demo task" {
		t.Fatal("admitted title missing")
	}
}

func TestReceiptUnknownsNeverZero(t *testing.T) {
	_, r, _ := receiptRun(t)
	billing := r["billing"].(map[string]any)
	price := billing["retail_equivalent_estimate_usd"].(map[string]any)
	if price["value"] != "unknown" || price["source"] != "unknown" {
		t.Fatalf("missing cost = %v", price)
	}
	tokens := billing["tokens"].(map[string]any)
	if tokens["by_model"] != "unknown" || tokens["source"] != "unknown" {
		t.Fatalf("missing usage = %v", tokens)
	}
}

func TestReceiptCostLabelledEstimate(t *testing.T) {
	f, _, id := receiptRun(t)
	j, err := journal.Open(t.Context(), f.state)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	err = j.Append(t.Context(), journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"), RunID: id,
		ProducerID: ids.New("sup"), ProducerSequence: 1, Generation: 1,
		ObservedAt: time.Now().UTC(), Type: "attempt.native_result",
		Payload: json.RawMessage(`{"retail_equivalent_estimate_usd":"0.42","usage_native_reported":{"fixture-model":{"input":23}}}`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, err := supervisor.BuildReceipt(t.Context(), j, id)
	if err != nil {
		t.Fatal(err)
	}
	price := r["billing"].(map[string]any)["retail_equivalent_estimate_usd"].(map[string]any)
	if price["value"] != "0.42" || price["source"] != "native-reported" || price["note"] != "estimate, not a charge" {
		t.Fatalf("cost label = %v", price)
	}
	tokens := r["billing"].(map[string]any)["tokens"].(map[string]any)
	if tokens["source"] != "native-reported" {
		t.Fatalf("tokens source = %v", tokens)
	}
}

func TestNoCredentialValuesPersisted(t *testing.T) {
	f := newFixture(t)
	secret := strings.Join([]string{"sk", "ant", "test-receipt-never-persist-this-value"}, "-")
	t.Setenv("ANTHROPIC_API_KEY", secret)
	if err := os.MkdirAll(filepath.Join(f.home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.home, ".claude", "settings.json"), []byte(`{"env":{"API_KEY":"`+secret+`"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := f.run(t, f.fakeRun()...); code != 5 {
		t.Fatalf("run exit %d: %s", code, stderr)
	}
	root, err := os.OpenRoot(f.state)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	err = filepath.WalkDir(f.state, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(f.state, path)
		if err != nil {
			return err
		}
		b, err := root.ReadFile(rel)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), secret) {
			t.Errorf("credential value persisted in %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReviewReadsRealCandidateDiff(t *testing.T) {
	f, _, id := receiptRun(t)
	code, out, stderr := f.run(t, "review", id)
	if code != 0 {
		t.Fatalf("review exit %d: %s", code, stderr)
	}
	if !strings.Contains(out, "Candidate diff:") || !strings.Contains(out, "demo.txt") || !strings.Contains(out, "+ok") {
		t.Fatalf("review omitted the frozen candidate diff: %q", out)
	}
}
