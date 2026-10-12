package supervisor_test

import (
	"context"
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
	"sync"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/acceptance"
	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/admission"
	billingpkg "github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/contain"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/integration"
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
	t.Parallel()
	r := waivedRun(t).file
	want := []string{"schema_version", "run_id", "state", "exit_code", "requested_outcome", "admitted_snapshot",
		"execution_bundle", "fidelity_differences", "native_configuration", "billing", "routing", "native_result",
		"candidate", "verification", "external_effects", "execution_host", "host_integration", "unknowns", "remaining_human_action"}
	for _, k := range want {
		if _, ok := r[k]; !ok {
			t.Errorf("receipt lacks section 9 key %q", k)
		}
	}
	if r["requested_outcome"].(map[string]any)["title"] != "Demo task" {
		t.Fatal("admitted title missing")
	}
}

// TestReceiptTopLevelKeys pins D8: the receipt grows by exactly the keys
// acceptance and unknown_reasons, and its schema version does not move.
func TestReceiptTopLevelKeys(t *testing.T) {
	t.Parallel()
	r := waivedRun(t).file
	want := []string{"schema_version", "run_id", "state", "exit_code", "requested_outcome", "admitted_snapshot",
		"execution_bundle", "fidelity_differences", "native_configuration", "billing", "routing", "native_result",
		"candidate", "verification", "external_effects", "execution_host", "host_integration", "unknowns", "remaining_human_action",
		"acceptance", "unknown_reasons"}
	got := make([]string, 0, len(r))
	for k := range r {
		got = append(got, k)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("receipt keys = %v; want %v", got, want)
	}
	if r["schema_version"] != float64(1) {
		t.Fatalf("schema_version = %v, want 1", r["schema_version"])
	}
}

func TestReceiptUnknownsNeverZero(t *testing.T) {
	t.Parallel()
	r := waivedRun(t).file
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
	t.Parallel()
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
	// Sequential: process environment (ANTHROPIC_API_KEY via t.Setenv).
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
	t.Parallel()
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
	t.Parallel()
	if runtime.GOOS != "linux" {
		t.Skip("the restricted boundary is qualified on Linux only")
	}
	requireBoundary(t)
	f := newFixture(t)
	f.home = outsideTmpDir(t)
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
	t.Parallel()
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
	return receiptWorldTo(t, supervisor.RunExecuting)
}

// receiptWorldTo is ledgerReceiptWorld with a run walked through admission to
// the given state (executing, or a terminal one admission can reach).
func receiptWorldTo(t *testing.T, to supervisor.RunState) (*journal.Journal, string) {
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
	if err := supervisor.TransitionRun(t.Context(), j, runID, supervisor.RunAdmission, "", prod); err != nil {
		t.Fatal(err)
	}
	reason := ""
	if to == supervisor.RunBlocked {
		reason = "admission_blocked"
	}
	if err := supervisor.TransitionRun(t.Context(), j, runID, to, reason, prod); err != nil {
		t.Fatal(err)
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
	t.Run("divergent scope resolves by bucket", func(t *testing.T) {
		t.Parallel()
		j, runID := ledgerReceiptWorld(t)
		r := held("released")
		r.RunID, r.Owner = runID, runID
		r.Scope = "another-scope"
		insertReservation(t, j, r)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
			return journal.SetBucketExhausted(t.Context(), tx, bucket, now, nil)
		}); err != nil {
			t.Fatal(err)
		}
		billing := receiptBilling(t, j, runID)
		next, ok := billing["next_retry"].(map[string]any)
		if !ok {
			t.Fatalf("next_retry = %#v, want the schedule looked up by bucket, not scope",
				billing["next_retry"])
		}
		if next["at"] != "unknown" {
			t.Errorf("next retry at = %v, want unknown", next["at"])
		}
		remaining, ok := billing["remaining"].(map[string]any)
		if !ok {
			t.Fatalf("remaining = %#v, want an object", billing["remaining"])
		}
		if remaining["bucket"] != bucket {
			t.Errorf("remaining bucket = %v, want %q, not the scope", remaining["bucket"], bucket)
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
	t.Parallel()
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

// realRun is what the receipt tests read of a finished fake-adapter run. It is
// captured once for the test binary, because a real run costs seconds and its
// fixture is removed when the test that built it ends. Tests only read it.
type realRun struct {
	file   map[string]any          // the receipt file the run wrote
	built  supervisor.Receipt      // the receipt the journal renders now
	row    journal.VerificationRow // the run's verification, zero when none
	digest string                  // the check configuration's digest, empty when waived
}

var (
	passedOnce, waivedOnce sync.Once
	passedSnapshot         realRun
	waivedSnapshot         realRun
)

func captureRun(t *testing.T, f fixture, runID, digest string) realRun {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(f.state, "runs", runID, "receipt.json")) // #nosec G304 -- fixture state and projected ID
	if err != nil {
		t.Fatal(err)
	}
	run := realRun{digest: digest}
	if err := json.Unmarshal(b, &run.file); err != nil {
		t.Fatal(err)
	}
	j, err := journal.OpenReadOnly(t.Context(), f.state)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	if run.built, err = supervisor.BuildReceipt(t.Context(), j, runID); err != nil {
		t.Fatal(err)
	}
	if run.row, err = j.LatestVerification(t.Context(), runID); err != nil && !errors.Is(err, journal.ErrNotFound) {
		t.Fatal(err)
	}
	return run
}

// passedRun is a fake task whose check passed: exit 0, bound and attested.
func passedRun(t *testing.T) realRun {
	t.Helper()
	passedOnce.Do(func() {
		f := newFixture(t)
		digest := f.config(t, "pass")
		if code, _, stderr := f.run(t, f.checkedRun(digest)...); code != 0 {
			t.Fatalf("run exit %d: %s", code, stderr)
		}
		passedSnapshot = captureRun(t, f, f.onlyRun(t).RunID, digest)
	})
	if passedSnapshot.file == nil {
		t.Fatal("the shared passed run failed in an earlier test")
	}
	return passedSnapshot
}

// waivedRun is a fake task run with --no-checks: unverified, exit 5.
func waivedRun(t *testing.T) realRun {
	t.Helper()
	waivedOnce.Do(func() {
		f := newFixture(t)
		if code, _, stderr := f.run(t, f.fakeRun()...); code != 5 {
			t.Fatalf("run exit %d: %s", code, stderr)
		}
		waivedSnapshot = captureRun(t, f, f.onlyRun(t).RunID, "")
	})
	if waivedSnapshot.file == nil {
		t.Fatal("the shared waived run failed in an earlier test")
	}
	return waivedSnapshot
}

// object returns m[key] as an object.
func object(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	v, ok := m[key].(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want an object", key, m[key])
	}
	return v
}

// worldReceipt renders the world run's receipt as its consumers read it: JSON.
func worldReceipt(t *testing.T, j *journal.Journal, runID string) map[string]any {
	t.Helper()
	r, err := supervisor.BuildReceipt(t.Context(), j, runID)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func unknownReason(t *testing.T, r map[string]any, path string) string {
	t.Helper()
	got, _ := object(t, r, "unknown_reasons")[path].(string)
	return got
}

// verifiedWorld is a world run with a pin-less admission and one recorded
// verification of the given verdict, bound without an attestation.
func verifiedWorld(t *testing.T, result, reason string) (*journal.Journal, string) {
	t.Helper()
	j, runID := ledgerReceiptWorld(t)
	journalAdmission(t, j, runID, mustJSON(t, admission.Record{}))
	now := time.Now().UTC()
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.InsertVerification(t.Context(), tx, journal.VerificationRow{ID: ids.New("ver"), RunID: runID,
			CandidateCommit: strings.Repeat("c", 40), ConfigSHA256: strings.Repeat("d", 64), Result: result, Reason: reason,
			StartedAt: now, FinishedAt: now, Checks: []journal.CheckRow{{Name: "lint", Status: "unavailable", Argv: []string{"lint"}}}})
	}); err != nil {
		t.Fatal(err)
	}
	return j, runID
}

// TestReceiptCarriesEntitlementStatement pins AC-3.1 on the receipt: a
// declared posture is never worded as verified (I15).
func TestReceiptCarriesEntitlementStatement(t *testing.T) {
	t.Parallel()
	t.Run("a declared-posture receipt carries the declared sentence", func(t *testing.T) {
		t.Parallel()
		j, runID := ledgerReceiptWorld(t)
		journalAdmission(t, j, runID, mustJSON(t, admission.Record{Billing: adapter.BillingPosture{
			Mode: admission.BillingSubscriptionDeclared, CredentialProvenance: "native-login", EntitlementClass: "included-plan",
			EntitlementSource: "user_declared+native_status(subscriptionType=max)", PaidContinuation: "unknown",
			PaidContinuationUserDeclaration: "disabled", G05: "not-passed"}}))
		want := "entitlement: user-declared, NOT verified by MYTHHELM; paid continuation: unknown (user declares disabled)"
		if got := receiptBilling(t, j, runID)["entitlement_statement"]; got != want {
			t.Errorf("entitlement_statement = %q, want %q", got, want)
		}
	})
	t.Run("a fake-route receipt carries the not-applicable form", func(t *testing.T) {
		want := "entitlement: not applicable (local-scripted: no inference, no network)"
		if got := object(t, waivedRun(t).file, "billing")["entitlement_statement"]; got != want {
			t.Errorf("entitlement_statement = %q, want %q", got, want)
		}
	})
	t.Run("an unadmitted run is unknown, never declared", func(t *testing.T) {
		t.Parallel()
		j, runID := ledgerReceiptWorld(t)
		if got := receiptBilling(t, j, runID)["entitlement_statement"]; got != "entitlement: unknown" {
			t.Errorf("entitlement_statement = %q, want entitlement: unknown", got)
		}
	})
}

// TestReceiptVerificationLabelAndBinding pins AC-3.2 and AC-1.1 on the
// receipt: the label, its reason and what the verdict is bound to (I07, I09).
func TestReceiptVerificationLabelAndBinding(t *testing.T) {
	t.Parallel()
	t.Run("a passed run is bound to its candidate", func(t *testing.T) {
		run := passedRun(t)
		v := object(t, run.file, "verification")
		if v["result"] != "passed" || v["label"] != "passed" || v["reason"] != "" {
			t.Errorf("verification = %v/%v/%q, want passed/passed with no reason", v["result"], v["label"], v["reason"])
		}
		binding := object(t, v, "binding")
		candidate := object(t, binding, "candidate")
		commit := object(t, run.file, "candidate")["commit"]
		if candidate["value"] != commit || candidate["format"] != "sha1" || commit == "unknown" {
			t.Errorf("binding.candidate = %v, want sha1 %v (candidate.commit)", candidate, commit)
		}
		if binding["check_definition"] != "sha256:"+run.digest || binding["evaluator"] != "sha256:"+run.row.EvaluatorDigest ||
			binding["attestation_digest"] != run.row.AttestationDigest || binding["attestation_digest"] == "" {
			t.Errorf("binding = %v, want check definition sha256:%s, evaluator sha256:%s and the row's attestation digest %s",
				binding, run.digest, run.row.EvaluatorDigest, run.row.AttestationDigest)
		}
	})
	t.Run("a waived run is NOT RUN with the waiver and no binding", func(t *testing.T) {
		v := object(t, waivedRun(t).file, "verification")
		if v["label"] != "NOT RUN" || v["reason"] != "waived by --no-checks" || v["binding"] != "unknown" {
			t.Errorf("verification = %v/%v/%v, want NOT RUN, waived by --no-checks, unknown binding", v["label"], v["reason"], v["binding"])
		}
	})
	for _, tt := range []struct{ name, result, reason, label string }{
		{"an unavailable check is NOT RUN with its reason", "unavailable", "checks_unavailable", "NOT RUN"},
		{"an invalid verdict is INVALID, never passed", "invalid", "evaluator_digest_mismatch", "INVALID"},
		{"a failed verdict stays failed", "failed", "checks_failed", "failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			j, runID := verifiedWorld(t, tt.result, tt.reason)
			v := object(t, worldReceipt(t, j, runID), "verification")
			if v["result"] != tt.result || v["label"] != tt.label || v["reason"] != tt.reason {
				t.Errorf("verification = %v/%v/%v, want %s/%s/%s", v["result"], v["label"], v["reason"], tt.result, tt.label, tt.reason)
			}
			if v["binding"] != "unknown" {
				t.Errorf("binding = %v, want unknown for a verdict with no attestation", v["binding"])
			}
		})
	}
	t.Run("a run with no verification renders unknown with a reason", func(t *testing.T) {
		t.Parallel()
		j, runID := ledgerReceiptWorld(t)
		r := worldReceipt(t, j, runID)
		v := object(t, r, "verification")
		if v["result"] != "unknown" || v["label"] != "unknown" || v["binding"] != "unknown" {
			t.Errorf("verification = %v, want unknown result, label and binding", v)
		}
		if got := unknownReason(t, r, "verification.label"); got != "no verification was recorded" {
			t.Errorf("reason of verification.label = %q, want no verification was recorded", got)
		}
	})
}

// TestReceiptPolicyPinAndDeterminism pins AC-2.3 on the receipt: the run's P1
// pin with the evaluator that ran, and a pre-schema-8 run's pin as unknown.
func TestReceiptPolicyPinAndDeterminism(t *testing.T) {
	t.Parallel()
	t.Run("a pinned run renders its policy and evaluator", func(t *testing.T) {
		run := passedRun(t)
		a := object(t, run.file, "acceptance")
		pin := object(t, a, "policy_pin")
		evaluator := object(t, pin, "evaluator")
		if pin["policy_id"] != "P1" || pin["policy_version"] != float64(1) || pin["check_definition"] != "sha256:"+run.digest {
			t.Errorf("policy_pin = %v, want P1 version 1 over check definition sha256:%s", pin, run.digest)
		}
		if evaluator["name"] != run.row.EvaluatorName || evaluator["digest"] != "sha256:"+run.row.EvaluatorDigest {
			t.Errorf("policy_pin.evaluator = %v, want the verification's %s sha256:%s", evaluator, run.row.EvaluatorName, run.row.EvaluatorDigest)
		}
		det := object(t, a, "determinism")
		if det["confined_to"] != "native session" || det["note"] != acceptance.DeterminismNote {
			t.Errorf("determinism = %v, want confined to the native session with the documented note", det)
		}
	})
	t.Run("a waived run pins no evaluator", func(t *testing.T) {
		pin := object(t, object(t, waivedRun(t).file, "acceptance"), "policy_pin")
		if pin["policy_id"] != "P1" || pin["evaluator"] != "unknown" || pin["check_definition"] != "unknown" {
			t.Errorf("policy_pin = %v, want P1 with an unknown evaluator and check definition", pin)
		}
	})
	t.Run("a pre-schema-8 run renders the pin unknown with that reason", func(t *testing.T) {
		t.Parallel()
		j, runID := ledgerReceiptWorld(t)
		journalAdmission(t, j, runID, mustJSON(t, admission.Record{}))
		r := worldReceipt(t, j, runID)
		if got := object(t, r, "acceptance")["policy_pin"]; got != "unknown" {
			t.Errorf("policy_pin = %v, want unknown", got)
		}
		if got := unknownReason(t, r, "acceptance.policy_pin"); got != "recorded before acceptance binding existed" {
			t.Errorf("reason = %q, want recorded before acceptance binding existed", got)
		}
	})
	t.Run("an unadmitted run says it was not admitted", func(t *testing.T) {
		t.Parallel()
		j, runID := ledgerReceiptWorld(t)
		r := worldReceipt(t, j, runID)
		if got := unknownReason(t, r, "acceptance.policy_pin"); got != "the run was not admitted" {
			t.Errorf("reason = %q, want the run was not admitted", got)
		}
	})
	t.Run("an unreadable pin renders unknown and still builds the receipt", func(t *testing.T) {
		t.Parallel()
		j, runID := ledgerReceiptWorld(t)
		journalAdmission(t, j, runID, `{"acceptance_pin":{"policy_version":"one"}}`)
		r := worldReceipt(t, j, runID)
		if got := object(t, r, "acceptance")["policy_pin"]; got != "unknown" {
			t.Errorf("policy_pin = %v, want unknown", got)
		}
		if got := unknownReason(t, r, "acceptance.policy_pin"); got != "acceptance pin unreadable" {
			t.Errorf("reason = %q, want acceptance pin unreadable", got)
		}
	})
}

// TestRunWithUnreadablePinHasReceiptAndReviews pins the D24 outcome: the run
// failed before any verification, its receipt exists with the pin unknown and
// review still exits 0 (validation finding 2).
func TestRunWithUnreadablePinHasReceiptAndReviews(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	digest := f.commitChecks(t, testCheck("test", "pass"))
	d, _, wait := seedLaunchWith(t, f, "happy", corruptPinPayload, "sha256:"+digest)
	wait()
	if code, _, stderr := f.run(t, "recover", d.RunID); code != 1 {
		t.Fatalf("recover exit %d: %s", code, stderr)
	}
	if got := f.onlyRun(t); got.State != "failed" || got.Reason != "acceptance_pin_unreadable" {
		t.Fatalf("run %s/%s, want failed/acceptance_pin_unreadable", got.State, got.Reason)
	}
	r := captureRun(t, f, d.RunID, digest).file
	if got := object(t, r, "acceptance")["policy_pin"]; got != "unknown" {
		t.Errorf("policy_pin = %v, want unknown", got)
	}
	if got := object(t, r, "verification")["label"]; got != "unknown" {
		t.Errorf("verification.label = %v, want unknown for a run with no verification", got)
	}
	if code, out, stderr := f.run(t, "review", d.RunID); code != 0 {
		t.Errorf("review exit %d, want 0: %s%s", code, out, stderr)
	}
}

// TestReceiptBoundsEnforcedFlags pins AC-4.4 and I10: the four envelope values
// with the one that binds a live run flagged, the other three not (the three
// flags are rewritten by the task that closes issue #328).
func TestReceiptBoundsEnforcedFlags(t *testing.T) {
	t.Parallel()
	j, runID := ledgerReceiptWorld(t)
	insertEnvelope(t, j, journal.EnvelopeRow{RunID: runID, ExecutionSeconds: 1800, Repairs: 3, Replans: 2, TransportRetries: 5})
	pin := acceptance.NewPin("tsk", strings.Repeat("a", 64)).WithChecks(strings.Repeat("b", 64), "host", strings.Repeat("c", 64),
		map[string]string{"test": "5s"})
	journalAdmission(t, j, runID, mustJSON(t, supervisor.AdmissionPayload{AcceptancePin: pin}))
	bounds := object(t, object(t, worldReceipt(t, j, runID), "acceptance"), "bounds")
	for key, want := range map[string]struct {
		value    float64
		enforced bool
	}{
		"execution_seconds": {1800, true}, "repairs": {3, false}, "replans": {2, false}, "transport_retries": {5, false},
	} {
		got := object(t, bounds, key)
		if got["value"] != want.value || got["enforced"] != want.enforced {
			t.Errorf("bounds.%s = %v, want value %v enforced %v", key, got, want.value, want.enforced)
		}
	}
	timeouts := object(t, bounds, "check_timeouts")
	if len(timeouts) != 1 || timeouts["test"] != "5s" {
		t.Errorf("bounds.check_timeouts = %v, want the pin's {test: 5s}", timeouts)
	}
	if bounds["evidence_bytes_per_check"] != float64(integration.MaxEvidenceBytes) {
		t.Errorf("bounds.evidence_bytes_per_check = %v, want %d", bounds["evidence_bytes_per_check"], integration.MaxEvidenceBytes)
	}
}

func TestReceiptBoundsUnknownWithoutEnvelope(t *testing.T) {
	t.Parallel()
	j, runID := ledgerReceiptWorld(t)
	r := worldReceipt(t, j, runID)
	if got := object(t, r, "acceptance")["bounds"]; got != "unknown" {
		t.Errorf("bounds = %v, want unknown for a run with no envelope", got)
	}
	if unknownReason(t, r, "acceptance.bounds") == "" {
		t.Error("acceptance.bounds is unknown with no registered reason")
	}
}

// TestReceiptOverheadCoversBindingStep pins NFR-1: the binding step's duration
// is the receipt's acceptance overhead, and it says what it covers.
func TestReceiptOverheadCoversBindingStep(t *testing.T) {
	t.Parallel()
	t.Run("an executed verification carries the measured overhead", func(t *testing.T) {
		run := passedRun(t)
		if run.row.OverheadMS == nil {
			t.Fatal("the verification row has no overhead")
		}
		a := object(t, run.file, "acceptance")
		if a["overhead_ms"] != float64(*run.row.OverheadMS) || a["overhead_covers"] != "binding step" {
			t.Errorf("acceptance overhead = %v covering %v, want %d covering the binding step", a["overhead_ms"], a["overhead_covers"], *run.row.OverheadMS)
		}
	})
	t.Run("a waived run measured nothing and says so", func(t *testing.T) {
		if got := object(t, waivedRun(t).file, "acceptance")["overhead_ms"]; got != "unknown" {
			t.Errorf("overhead_ms = %v, want unknown when no binding step ran", got)
		}
	})
	t.Run("a run recorded before schema 8 has no measurement", func(t *testing.T) {
		t.Parallel()
		j, runID := ledgerReceiptWorld(t)
		journalAdmission(t, j, runID, mustJSON(t, admission.Record{}))
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := rawDB(t, j.StateDir()).ExecContext(t.Context(), `INSERT INTO verifications
			(verification_id, run_id, candidate_commit, config_sha256, result, started_at, finished_at)
			VALUES (?, ?, ?, ?, 'passed', ?, ?)`, ids.New("ver"), runID, strings.Repeat("c", 40), strings.Repeat("d", 64), now, now); err != nil {
			t.Fatal(err)
		}
		r := worldReceipt(t, j, runID)
		if got := object(t, r, "acceptance")["overhead_ms"]; got != "unknown" {
			t.Errorf("overhead_ms = %v, want unknown", got)
		}
		want := "recorded before acceptance binding existed"
		for _, path := range []string{"acceptance.overhead_ms", "verification.binding"} {
			if got := unknownReason(t, r, path); got != want {
				t.Errorf("reason of %s = %q, want %q", path, got, want)
			}
		}
	})
}

// TestBuildReceiptWithinExpires pins NFR-1's render limit: a limit that has
// already passed fails the build with its cause and renders no receipt.
func TestBuildReceiptWithinExpires(t *testing.T) {
	t.Parallel()
	j, runID := ledgerReceiptWorld(t)
	r, err := supervisor.BuildReceiptWithin(t.Context(), j, runID, -time.Second)
	if !errors.Is(err, context.DeadlineExceeded) || r != nil {
		t.Fatalf("BuildReceiptWithin = receipt %t, error %v; want no receipt and an error wrapping context.DeadlineExceeded", r != nil, err)
	}
	if _, err := os.Stat(filepath.Join(j.StateDir(), "runs", runID, "receipt.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a receipt file exists after an expired build: stat = %v", err)
	}
	if r, err := supervisor.BuildReceiptWithin(t.Context(), j, runID, time.Minute); err != nil || r["run_id"] != runID {
		t.Errorf("BuildReceiptWithin within its limit = %v, %v; want the run's receipt", r, err)
	}
}

// journalEvent appends an event of typ with payload to the world run.
func journalEvent(t *testing.T, j *journal.Journal, runID, typ, payload string) {
	t.Helper()
	if err := j.Append(t.Context(), journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"), RunID: runID,
		ProducerID: ids.New("sup"), ProducerSequence: 1, Generation: 1,
		ObservedAt: time.Now().UTC(), Type: typ, Payload: json.RawMessage(payload),
	}, nil); err != nil {
		t.Fatal(err)
	}
}

// unknownFixtures builds the receipts that together reach every producer of an
// unknown leaf: the real passed and waived runs and journaled runs for the
// rest (the unavailable-check, blocked, failed-native, exhausted-bucket,
// restricted-profile and Claude native-auth runs among them).
func unknownFixtures(t *testing.T) map[string]supervisor.Receipt {
	t.Helper()
	out := map[string]supervisor.Receipt{"passed": passedRun(t).built, "waived": waivedRun(t).built}
	add := func(name string, j *journal.Journal, runID string) {
		r, err := supervisor.BuildReceipt(t.Context(), j, runID)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out[name] = r
	}
	j, runID := ledgerReceiptWorld(t)
	add("unadmitted", j, runID)

	j, runID = receiptWorldTo(t, supervisor.RunBlocked)
	add("blocked", j, runID)

	j, runID = ledgerReceiptWorld(t)
	journalAdmission(t, j, runID, mustJSON(t, admission.Record{}))
	journalEvent(t, j, runID, "attempt.native_result", `{"subtype":"error"}`)
	if err := supervisor.TransitionRun(t.Context(), j, runID, supervisor.RunFailed, "native_failed", supervisor.NewProducer(ids.New("sup"), 1)); err != nil {
		t.Fatal(err)
	}
	add("failed native", j, runID)

	j, runID = verifiedWorld(t, "unavailable", "checks_unavailable")
	add("check that never started", j, runID)

	j, runID = ledgerReceiptWorld(t)
	bucket := `["claude-code","cli","subscription","opaque-ref"]`
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	insertReservation(t, j, journal.ReservationRow{ReservationID: ids.New("rsv"), RunID: runID, Bucket: bucket, Scope: bucket,
		Owner: runID, Quantity: "unknown", Status: "released", ExpiresAt: stamp, CreatedAt: stamp})
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.SetBucketExhausted(t.Context(), tx, bucket, stamp, nil)
	}); err != nil {
		t.Fatal(err)
	}
	insertUsage(t, j, journal.UsageRow{ObservationID: ids.New("obs"), RunID: runID, Scope: "retail-equivalent",
		Label: "unknown", Quantity: "unknown", ObservedAt: stamp})
	add("exhausted bucket with no reset", j, runID)

	j, runID = ledgerReceiptWorld(t)
	journalAdmission(t, j, runID, mustJSON(t, admission.Record{ExecutionProfile: admission.Profile{
		Name: admission.ProfileRestricted, Contained: true,
		Boundary: &contain.Evidence{Boundary: "boundary", Version: "1", Coverage: contain.Coverage{Filesystem: contain.Claim{Name: "fs", Enforced: true}}}}}))
	add("restricted profile", j, runID)

	j, runID = ledgerReceiptWorld(t)
	journalAdmission(t, j, runID, mustJSON(t, admission.Record{ExecutionProfile: admission.Profile{
		Name: admission.ProfileRestricted, Contained: true, Boundary: &contain.Evidence{Boundary: "boundary", Version: "1"}}}))
	add("restricted profile enforcing nothing", j, runID)

	j, runID = ledgerReceiptWorld(t)
	journalAdmission(t, j, runID, mustJSON(t, admission.Record{NativeAuth: &admission.AuthEvidence{LoggedIn: true}}))
	add("claude native auth", j, runID)

	j, runID = ledgerReceiptWorld(t)
	journalAdmission(t, j, runID, mustJSON(t, admission.Record{}))
	if _, err := rawDB(t, j.StateDir()).ExecContext(t.Context(), `INSERT INTO verifications
		(verification_id, run_id, candidate_commit, config_sha256, result, started_at, finished_at)
		VALUES (?, ?, ?, ?, 'failed', ?, ?)`, ids.New("ver"), runID, strings.Repeat("c", 40), strings.Repeat("d", 64), stamp, stamp); err != nil {
		t.Fatal(err)
	}
	add("recorded before schema 8", j, runID)

	j, runID = ledgerReceiptWorld(t)
	journalAdmission(t, j, runID, `{"acceptance_pin":{"policy_version":"one"}}`)
	add("unreadable pin", j, runID)
	return out
}

// TestEveryUnknownHasAReason pins AC-3.4 (I09): every unknown leaf of every
// receipt the producers can render has a registered reason, and every
// registered reason names a leaf some receipt renders unknown.
func TestEveryUnknownHasAReason(t *testing.T) {
	t.Parallel()
	fixtures := unknownFixtures(t)
	reasons := supervisor.UnknownReasons()
	seen := map[string]bool{}
	for name, r := range fixtures {
		rendered, _ := r["unknown_reasons"].(map[string]string)
		if len(rendered) == 0 {
			t.Errorf("%s: the receipt carries no unknown_reasons", name)
		}
		for path, reason := range rendered {
			seen[path] = true
			if reason == "no reason recorded" {
				t.Errorf("%s: %s is unknown with no registered reason", name, path)
			}
		}
		if missing := supervisor.AttachUnknownReasons(r, reasons); len(missing) != 0 {
			t.Errorf("%s: unknown leaves without a registered reason: %v", name, missing)
		}
	}
	for path := range reasons {
		if !seen[path] {
			t.Errorf("registered reason %q names a leaf no fixture renders unknown (an orphan, or a fixture is missing)", path)
		}
	}
}
