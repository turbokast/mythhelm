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
	billingpkg "github.com/turbokast/mythhelm/internal/billing"
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

// ledgerReceiptWorld opens a journal with one run in executing state and no
// ledger rows; each test adds the rows its member needs.
func ledgerReceiptWorld(t *testing.T) (*journal.Journal, string) {
	t.Helper()
	j, err := journal.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	runID := ids.New("run")
	prod := supervisor.NewProducer(ids.New("sup"), 1)
	run := journal.RunRow{RunID: runID, AdapterID: "builtin/fake", SourceRepo: "/tmp/repo", TaskSHA256: "t",
		BillingPosture: "local-scripted", ExecutionProfile: "trusted-host"}
	if err := supervisor.CreateRun(t.Context(), j, run, prod); err != nil {
		t.Fatal(err)
	}
	for _, to := range []supervisor.RunState{supervisor.RunAdmission, supervisor.RunExecuting} {
		if err := supervisor.TransitionRun(t.Context(), j, runID, to, "", prod); err != nil {
			t.Fatal(err)
		}
	}
	return j, runID
}

func insertUsage(t *testing.T, j *journal.Journal, o journal.UsageRow) {
	t.Helper()
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.InsertUsageObservation(t.Context(), tx, o)
	}); err != nil {
		t.Fatal(err)
	}
}

func insertEnvelope(t *testing.T, j *journal.Journal, e journal.EnvelopeRow) {
	t.Helper()
	if e.UpdatedAt == "" {
		e.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.UpsertRunEnvelope(t.Context(), tx, e)
	}); err != nil {
		t.Fatal(err)
	}
}

func insertReservation(t *testing.T, j *journal.Journal, r journal.ReservationRow) {
	t.Helper()
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.InsertReservation(t.Context(), tx, r)
	}); err != nil {
		t.Fatal(err)
	}
}

func journalAdmission(t *testing.T, j *journal.Journal, runID, payload string) {
	t.Helper()
	if err := j.Append(t.Context(), journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"), RunID: runID,
		ProducerID: ids.New("sup"), ProducerSequence: 1, Generation: 1,
		ObservedAt: time.Now().UTC(), Type: "admission.decided",
		Payload: json.RawMessage(payload),
	}, nil); err != nil {
		t.Fatal(err)
	}
}

func receiptBilling(t *testing.T, j *journal.Journal, runID string) map[string]any {
	t.Helper()
	r, err := supervisor.BuildReceipt(t.Context(), j, runID)
	if err != nil {
		t.Fatal(err)
	}
	// Round-trip through JSON: the assertions pin the serialised receipt
	// that later tasks and reviewers consume, not Go's number types.
	wire, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	billing, ok := decoded["billing"].(map[string]any)
	if !ok {
		t.Fatalf("receipt billing = %T, want an object", decoded["billing"])
	}
	return billing
}

// TestReceiptCarriesTypedUsage pins billing.usage_observations: one entry per
// row with scope, unit, source, label, quantity and observed_at, the retail
// estimate labelled an estimate, and no summed figure across rows.
func TestReceiptCarriesTypedUsage(t *testing.T) {
	t.Parallel()
	j, runID := ledgerReceiptWorld(t)
	at := time.Now().UTC().Format(time.RFC3339Nano)
	insertUsage(t, j, journal.UsageRow{ObservationID: ids.New("obs"), RunID: runID,
		Scope: "fixture-model", Unit: "tokens", Source: "native-reported",
		Label: "reported", Quantity: "23", ProducerID: "wrk_a1", ProducerSequence: 7, ObservedAt: at})
	insertUsage(t, j, journal.UsageRow{ObservationID: ids.New("obs"), RunID: runID,
		Scope: "retail-equivalent", Unit: "USD", Source: "native-reported",
		Label: "estimated", Quantity: "0.37", ProducerID: "wrk_a1", ProducerSequence: 7, ObservedAt: at})
	billing := receiptBilling(t, j, runID)
	raw, ok := billing["usage_observations"].([]any)
	if !ok {
		t.Fatalf("usage_observations = %#v, want one entry per row", billing["usage_observations"])
	}
	if len(raw) != 2 {
		t.Fatalf("usage_observations has %d entries, want 2 (no summed figure)", len(raw))
	}
	first, ok := raw[0].(map[string]any)
	if !ok {
		t.Fatalf("usage_observations[0] = %#v, want an object", raw[0])
	}
	for k, want := range map[string]string{
		"scope": "fixture-model", "unit": "tokens", "source": "native-reported",
		"label": "reported", "quantity": "23", "observed_at": at,
	} {
		if first[k] != want {
			t.Errorf("usage_observations[0][%s] = %v, want %q", k, first[k], want)
		}
	}
	second, ok := raw[1].(map[string]any)
	if !ok {
		t.Fatalf("usage_observations[1] = %#v, want an object", raw[1])
	}
	if second["scope"] != "retail-equivalent" || second["quantity"] != "0.37" || second["label"] != "estimated" {
		t.Errorf("retail row = %v, want the exact 0.37 estimate", second)
	}
	if second["source"] != "native-reported" || second["note"] != "estimate, not a charge" {
		t.Errorf("retail row source/note = %v/%v, want native-reported and the estimate note",
			second["source"], second["note"])
	}
	// The pre-ledger members stay: the typed rows extend the receipt.
	price, ok := billing["retail_equivalent_estimate_usd"].(map[string]any)
	if !ok || price["note"] != "estimate, not a charge" {
		t.Errorf("retail_equivalent_estimate_usd = %#v, want the existing labelled member", billing["retail_equivalent_estimate_usd"])
	}
}

// TestReceiptUnknownNeverZero pins I09 on the receipt: missing quantities
// render unknown in usage rows and remaining, never 0 or a balance figure.
func TestReceiptUnknownNeverZero(t *testing.T) {
	t.Parallel()
	j, runID := ledgerReceiptWorld(t)
	at := time.Now().UTC().Format(time.RFC3339Nano)
	insertUsage(t, j, journal.UsageRow{ObservationID: ids.New("obs"), RunID: runID,
		Scope: "retail-equivalent", Label: "unknown", Quantity: "unknown", ObservedAt: at})
	billing := receiptBilling(t, j, runID)
	raw, ok := billing["usage_observations"].([]any)
	if !ok || len(raw) != 1 {
		t.Fatalf("usage_observations = %#v, want the one unknown marker", billing["usage_observations"])
	}
	row, ok := raw[0].(map[string]any)
	if !ok {
		t.Fatalf("usage_observations[0] = %#v, want an object", raw[0])
	}
	if row["quantity"] != "unknown" {
		t.Errorf("unknown marker quantity = %v, want unknown, never 0", row["quantity"])
	}
	remaining, ok := billing["remaining"].(map[string]any)
	if !ok {
		t.Fatalf("remaining = %#v, want an object", billing["remaining"])
	}
	if remaining["quantity"] != "unknown" {
		t.Errorf("remaining quantity = %v, want unknown, never 0 or a balance", remaining["quantity"])
	}
	flat, err := json.Marshal(billing)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(flat)), "balance") {
		t.Errorf("billing object carries a balance figure: %s", flat)
	}
}

// TestReceiptReserveIsEstimateOnly pins billing.reserve: the Task 9 note
// verbatim, the envelope repair ceiling, and no token-quantity field that
// could read as a hard claim.
func TestReceiptReserveIsEstimateOnly(t *testing.T) {
	t.Parallel()
	t.Run("no-checks run quantifies the empty pass", func(t *testing.T) {
		t.Parallel()
		j, runID := ledgerReceiptWorld(t)
		insertEnvelope(t, j, journal.EnvelopeRow{RunID: runID,
			ExecutionSeconds: 1800, Repairs: 3, Replans: 2, TransportRetries: 5})
		journalAdmission(t, j, runID, `{"no_checks":true}`)
		billing := receiptBilling(t, j, runID)
		reserve, ok := billing["reserve"].(map[string]any)
		if !ok {
			t.Fatalf("reserve = %#v, want an object", billing["reserve"])
		}
		if reserve["note"] != billingpkg.ReserveNote {
			t.Errorf("reserve note = %v, want the Task 9 note verbatim", reserve["note"])
		}
		if reserve["verify_pass_checks"] != float64(0) || reserve["verify_timeout_sum"] != "0s" {
			t.Errorf("verify pass = %v/%v, want 0 checks and 0s for a waived suite",
				reserve["verify_pass_checks"], reserve["verify_timeout_sum"])
		}
		if reserve["repair_ceiling"] != float64(3) {
			t.Errorf("repair ceiling = %v, want 3 from the envelope row", reserve["repair_ceiling"])
		}
		for k := range reserve {
			switch k {
			case "verify_pass_checks", "verify_timeout_sum", "repair_ceiling", "note":
			default:
				t.Errorf("reserve carries %q, want no token-quantity field", k)
			}
		}
	})
	t.Run("checked run leaves the unjournaled pass unknown", func(t *testing.T) {
		t.Parallel()
		j, runID := ledgerReceiptWorld(t)
		insertEnvelope(t, j, journal.EnvelopeRow{RunID: runID,
			ExecutionSeconds: 1800, Repairs: 3, Replans: 2, TransportRetries: 5})
		journalAdmission(t, j, runID, `{"no_checks":false}`)
		billing := receiptBilling(t, j, runID)
		reserve, ok := billing["reserve"].(map[string]any)
		if !ok {
			t.Fatalf("reserve = %#v, want an object", billing["reserve"])
		}
		if reserve["verify_pass_checks"] != "unknown" || reserve["verify_timeout_sum"] != "unknown" {
			t.Errorf("verify pass = %v/%v, want unknown: check timeouts are not journaled",
				reserve["verify_pass_checks"], reserve["verify_timeout_sum"])
		}
		if reserve["repair_ceiling"] != float64(3) || reserve["note"] != billingpkg.ReserveNote {
			t.Errorf("reserve = %v, want the row ceiling with the estimate note", reserve)
		}
	})
	t.Run("missing envelope leaves the ceiling unknown", func(t *testing.T) {
		t.Parallel()
		j, runID := ledgerReceiptWorld(t)
		billing := receiptBilling(t, j, runID)
		reserve, ok := billing["reserve"].(map[string]any)
		if !ok {
			t.Fatalf("reserve = %#v, want an object", billing["reserve"])
		}
		if reserve["repair_ceiling"] != "unknown" || reserve["note"] != billingpkg.ReserveNote {
			t.Errorf("reserve = %v, want an unknown ceiling with the estimate note", reserve)
		}
	})
}

// TestReceiptNextRetryShown pins billing.next_retry: the exhausted bucket
// renders its reset time (or unknown), used and max retries, while a
// never-exhausted run renders null and never an invented time.
func TestReceiptNextRetryShown(t *testing.T) {
	t.Parallel()
	bucket := `["claude-code","cli","subscription","opaque-ref"]`
	held := func(status string) journal.ReservationRow {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		return journal.ReservationRow{ReservationID: ids.New("rsv"), Bucket: bucket, Scope: bucket,
			Owner: "run", Quantity: "unknown", Status: status,
			ExpiresAt: now, CreatedAt: now}
	}
	t.Run("unknown reset stays unknown", func(t *testing.T) {
		t.Parallel()
		j, runID := ledgerReceiptWorld(t)
		r := held("released")
		r.RunID, r.Owner = runID, runID
		insertReservation(t, j, r)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
			if err := journal.SetBucketExhausted(t.Context(), tx, bucket, now, nil); err != nil {
				return err
			}
			return journal.NoteBucketRetry(t.Context(), tx, bucket)
		}); err != nil {
			t.Fatal(err)
		}
		billing := receiptBilling(t, j, runID)
		next, ok := billing["next_retry"].(map[string]any)
		if !ok {
			t.Fatalf("next_retry = %#v, want the schedule", billing["next_retry"])
		}
		if next["at"] != "unknown" {
			t.Errorf("next retry at = %v, want unknown, never an invented time", next["at"])
		}
		if next["retries_used"] != float64(1) || next["retries_max"] != float64(admission.MaxBucketRetries) {
			t.Errorf("next retry = %v, want used 1 of %d", next, admission.MaxBucketRetries)
		}
		remaining, ok := billing["remaining"].(map[string]any)
		if !ok {
			t.Fatalf("remaining = %#v, want an object", billing["remaining"])
		}
		if remaining["bucket"] != bucket {
			t.Errorf("remaining bucket = %v, want the run's bucket identity", remaining["bucket"])
		}
	})
	t.Run("authoritative reset renders its time", func(t *testing.T) {
		t.Parallel()
		j, runID := ledgerReceiptWorld(t)
		r := held("released")
		r.RunID, r.Owner = runID, runID
		insertReservation(t, j, r)
		reset := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
		if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
			return journal.SetBucketExhausted(t.Context(), tx, bucket,
				time.Now().UTC().Format(time.RFC3339Nano), &reset)
		}); err != nil {
			t.Fatal(err)
		}
		billing := receiptBilling(t, j, runID)
		next, ok := billing["next_retry"].(map[string]any)
		if !ok {
			t.Fatalf("next_retry = %#v, want the schedule", billing["next_retry"])
		}
		if next["at"] != reset || next["retries_used"] != float64(0) {
			t.Errorf("next retry = %v, want the authoritative reset with 0 used", next)
		}
	})
	t.Run("never exhausted renders null", func(t *testing.T) {
		t.Parallel()
		j, runID := ledgerReceiptWorld(t)
		r := held("held")
		r.RunID, r.Owner = runID, runID
		insertReservation(t, j, r)
		billing := receiptBilling(t, j, runID)
		if billing["next_retry"] != nil {
			t.Errorf("next_retry = %#v, want null for a never-exhausted bucket", billing["next_retry"])
		}
		remaining, ok := billing["remaining"].(map[string]any)
		if !ok {
			t.Fatalf("remaining = %#v, want an object", billing["remaining"])
		}
		if remaining["quantity"] != "unknown" || remaining["bucket"] != bucket {
			t.Errorf("remaining = %v, want unknown quantity with the bucket identity", remaining)
		}
	})
}

// TestNoHardLimitAnywhere pins O3 on both S1 surfaces: the receipt billing
// object and the run's ledger notice lines carry no hard-spending or
// hard-token phrasing. The check anchors to those regions, not the file.
func TestNoHardLimitAnywhere(t *testing.T) {
	f := newFixture(t)
	code, plainOut, stderr := f.run(t, f.fakeRun()...)
	if code != 5 {
		t.Fatalf("run exit %d: %s", code, stderr)
	}
	if !strings.Contains(plainOut, "usage:") {
		t.Fatalf("run printed no ledger lines:\n%s", plainOut)
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
	flat, err := json.Marshal(r["billing"])
	if err != nil {
		t.Fatal(err)
	}
	banned := []string{"limit", "cap of", "balance"}
	lowered := strings.ToLower(string(flat))
	for _, phrase := range banned {
		if strings.Contains(lowered, phrase) {
			t.Errorf("receipt billing object carries hard-spending phrasing %q: %s", phrase, flat)
		}
	}
	// The notice lines: the four ledger lines on plain stdout, and the same
	// four as stderr diagnostics on a jsonl run.
	seen := 0
	for line := range strings.Lines(plainOut) {
		for _, prefix := range []string{"usage:", "reserve:", "remaining:", "next retry:"} {
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			seen++
			for _, phrase := range banned {
				if strings.Contains(strings.ToLower(line), phrase) {
					t.Errorf("notice line carries hard-spending phrasing %q: %q", phrase, line)
				}
			}
		}
	}
	if seen < 4 {
		t.Errorf("plain run printed %d ledger notice lines, want at least the four", seen)
	}
	_, _, jsonlErr := f.run(t, f.fakeRun("--format", "jsonl")...)
	seen = 0
	for line := range strings.Lines(jsonlErr) {
		msg, ok := strings.CutPrefix(line, "mythhelm run: ")
		if !ok {
			continue
		}
		for _, prefix := range []string{"usage:", "reserve:", "remaining:", "next retry:"} {
			if !strings.HasPrefix(msg, prefix) {
				continue
			}
			seen++
			for _, phrase := range banned {
				if strings.Contains(strings.ToLower(msg), phrase) {
					t.Errorf("jsonl diagnostic carries hard-spending phrasing %q: %q", phrase, msg)
				}
			}
		}
	}
	if seen < 4 {
		t.Errorf("jsonl run diagnostics hold %d ledger lines, want at least the four", seen)
	}
}
