package supervisor_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
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

// TestReceiptCarriesBoundary pins the receipt's execution_bundle.boundary: a
// restricted run records the admitted evidence's name, version and coverage,
// an unknown dimension renders unknown (never contained or verified), and a
// trusted-host run with no admitted evidence renders boundary unknown (I09).
func TestReceiptCarriesBoundary(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the restricted boundary is qualified on Linux only")
	}
	f := newFixture(t)
	args := slices.DeleteFunc(f.fakeRun(), func(a string) bool { return a == "trusted-host" || a == "--execution-profile" })
	if code, _, stderr := f.run(t, args...); code != 5 {
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
	ev, err := admission.BoundaryConsult(admission.ProfileRestricted, runtime.GOOS, "builtin/fake")
	if err != nil {
		t.Fatal(err)
	}
	boundary, ok := r["execution_bundle"].(map[string]any)["boundary"].(map[string]any)
	if !ok {
		t.Fatalf("execution_bundle.boundary = %v, want the admitted boundary record", r["execution_bundle"])
	}
	if boundary["name"] != ev.Boundary || boundary["version"] != ev.Version {
		t.Errorf("boundary name/version = %v/%v, want %s/%s from the admitted evidence",
			boundary["name"], boundary["version"], ev.Boundary, ev.Version)
	}
	coverage, ok := boundary["coverage"].(map[string]any)
	if !ok {
		t.Fatalf("boundary.coverage = %v, want one entry per dimension", boundary["coverage"])
	}
	for dim, claim := range map[string]string{
		"filesystem": ev.Coverage.Filesystem.Name, "process": ev.Coverage.Process.Name,
		"network": ev.Coverage.Network.Name, "credential": ev.Coverage.Credential.Name,
	} {
		if coverage[dim] != claim {
			t.Errorf("coverage[%s] = %v, want %q from the admitted evidence", dim, coverage[dim], claim)
		}
	}

	// An unenforced dimension renders unknown, never contained or verified.
	ev.Coverage.Credential.Enforced = false
	doctored, err := json.Marshal(admission.Record{ExecutionProfile: admission.Profile{
		Name: admission.ProfileRestricted, Contained: true, Boundary: &ev}})
	if err != nil {
		t.Fatal(err)
	}
	j, err := journal.Open(t.Context(), f.state)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	if err := j.Append(t.Context(), journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"), RunID: id,
		ProducerID: ids.New("sup"), ProducerSequence: 1, Generation: 1,
		ObservedAt: time.Now().UTC(), Type: "admission.decided", Payload: doctored,
	}, nil); err != nil {
		t.Fatal(err)
	}
	rerendered, err := supervisor.BuildReceipt(t.Context(), j, id)
	if err != nil {
		t.Fatal(err)
	}
	reboundary, ok := rerendered["execution_bundle"].(map[string]any)["boundary"].(map[string]any)
	if !ok {
		t.Fatalf("re-rendered boundary = %v, want the boundary record", rerendered["execution_bundle"])
	}
	recoverage := reboundary["coverage"].(map[string]any)
	if recoverage["credential"] != "unknown" {
		t.Errorf("coverage[credential] = %v, want unknown for the unenforced dimension", recoverage["credential"])
	}
	if flat, _ := json.Marshal(reboundary); strings.Contains(string(flat), "contained") || strings.Contains(string(flat), "verified") {
		t.Errorf("unknown dimension rendered as contained or verified: %s", flat)
	}

	// A trusted-host run admits no boundary evidence: boundary stays unknown.
	_, trusted, _ := receiptRun(t)
	if got := trusted["execution_bundle"].(map[string]any)["boundary"]; got != "unknown" {
		t.Errorf("trusted-host boundary = %v, want unknown", got)
	}
}

// TestReceiptCarriesEvaluator pins the receipt's verification.evaluator: it
// equals the verification row's name and digest, never a recomputation, and a
// run with no verification row renders evaluator unknown.
func TestReceiptCarriesEvaluator(t *testing.T) {
	f := newFixture(t)
	if code, _, stderr := f.run(t, f.fakeRun("--scenario", "native-fails")...); code != 4 {
		t.Fatalf("run exit %d: %s", code, stderr)
	}
	id := f.onlyRun(t).RunID
	j, err := journal.Open(t.Context(), f.state)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	if _, err := j.LatestVerification(t.Context(), id); !errors.Is(err, journal.ErrNotFound) {
		t.Fatalf("LatestVerification err = %v, want no verification row yet", err)
	}
	r, err := supervisor.BuildReceipt(t.Context(), j, id)
	if err != nil {
		t.Fatal(err)
	}
	if got := r["verification"].(map[string]any)["evaluator"]; got != "unknown" {
		t.Fatalf("evaluator with no row = %v, want unknown", got)
	}
	now := time.Now().UTC()
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.InsertVerification(t.Context(), tx, journal.VerificationRow{ID: ids.New("ver"), RunID: id,
			CandidateCommit: strings.Repeat("c", 40), ConfigSHA256: strings.Repeat("d", 64), Result: "passed",
			EvaluatorName: "host", EvaluatorDigest: strings.Repeat("e", 64), StartedAt: now, FinishedAt: now})
	}); err != nil {
		t.Fatal(err)
	}
	row, err := j.LatestVerification(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	rerendered, err := supervisor.BuildReceipt(t.Context(), j, id)
	if err != nil {
		t.Fatal(err)
	}
	evaluator, ok := rerendered["verification"].(map[string]any)["evaluator"].(map[string]any)
	if !ok {
		t.Fatalf("verification.evaluator = %v, want the row's name and digest", rerendered["verification"])
	}
	if evaluator["name"] != row.EvaluatorName || evaluator["digest"] != row.EvaluatorDigest {
		t.Errorf("evaluator = %v, want {name: %s, digest: %s} from the row", evaluator, row.EvaluatorName, row.EvaluatorDigest)
	}
}
