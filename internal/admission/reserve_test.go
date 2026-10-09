package admission_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/qualify"
)

func bucketRecord(harness, surface, entitlement string) qualify.Record {
	return qualify.Record{Key: qualify.Key{Harness: harness, Surface: surface, EntitlementClass: entitlement}}
}

var admittedRecord = bucketRecord("claude-code", "cli", "included-plan")

const identityRef = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func journalWithRun(t *testing.T) (*journal.Journal, string) {
	t.Helper()
	j, err := journal.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	runID := ids.New("run")
	now := time.Now()
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.InsertRun(t.Context(), tx, journal.RunRow{RunID: runID, State: "admission", AdapterID: "builtin/fake", SourceRepo: "/tmp/repo",
			TaskSHA256: "t", BillingPosture: "local-scripted", ExecutionProfile: "trusted-host", CreatedAt: now, UpdatedAt: now})
	}); err != nil {
		t.Fatal(err)
	}
	return j, runID
}

func reservationRows(t *testing.T, j *journal.Journal, runID string) int {
	t.Helper()
	var n int
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM reservations WHERE run_id = ?`, runID).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestHoldCouplesAdmittedBucket(t *testing.T) {
	t.Parallel()
	j, runID := journalWithRun(t)
	reserver := admission.NewJournalReserver(j, billing.BuiltInCeilings())
	id, err := admission.HoldQuotaReservation(t.Context(), reserver, runID, admittedRecord, identityRef)
	if err != nil {
		t.Fatal(err)
	}
	row, err := j.Reservation(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	want := admission.QuotaBucket(admittedRecord, identityRef)
	if row.Scope != want || row.Bucket != want {
		t.Fatalf("reservation scope %q bucket %q, want both %q", row.Scope, row.Bucket, want)
	}
	if row.RunID != runID || row.Status != "held" || row.Owner == "" {
		t.Fatalf("reservation = %+v, want a held row for %s with an owner", row, runID)
	}
	// Another route, entitlement class or identity is another bucket.
	for name, other := range map[string]string{
		"harness":     admission.QuotaBucket(bucketRecord("codex", "cli", "included-plan"), identityRef),
		"surface":     admission.QuotaBucket(bucketRecord("claude-code", "sdk", "included-plan"), identityRef),
		"entitlement": admission.QuotaBucket(bucketRecord("claude-code", "cli", "metered"), identityRef),
		"identity":    admission.QuotaBucket(admittedRecord, strings.Repeat("f", 64)),
	} {
		if other == want {
			t.Errorf("a different %s maps to the same bucket %q", name, want)
		}
	}
}

func TestQuotaBucketJoinsUnambiguously(t *testing.T) {
	t.Parallel()
	a := admission.QuotaBucket(bucketRecord("a|b", "c", "e"), identityRef)
	b := admission.QuotaBucket(bucketRecord("a", "b|c", "e"), identityRef)
	if a == b {
		t.Fatalf("field boundary moved without changing the bucket: %q", a)
	}
	// An unknown identity is its own bucket label, never an empty segment.
	if got := admission.QuotaBucket(admittedRecord, ""); !strings.Contains(got, "unknown") {
		t.Fatalf("bucket without identity = %q, want an explicit unknown", got)
	}
}

type fakeReserver struct {
	runID, bucket, owner string
	err                  error
}

func (f *fakeReserver) Reserve(_ context.Context, runID, bucket, owner string) (string, error) {
	f.runID, f.bucket, f.owner = runID, bucket, owner
	return "rsv_fake", f.err
}

func TestHoldPassesBucketAndPropagatesFailure(t *testing.T) {
	t.Parallel()
	f := &fakeReserver{}
	id, err := admission.HoldQuotaReservation(t.Context(), f, "run_1", admittedRecord, identityRef)
	if err != nil || id != "rsv_fake" {
		t.Fatalf("hold = %q, %v", id, err)
	}
	if f.runID != "run_1" || f.bucket != admission.QuotaBucket(admittedRecord, identityRef) || f.owner == "" {
		t.Fatalf("reserver saw %+v", f)
	}
	boom := errors.New("storage down")
	f.err = boom
	if id, err := admission.HoldQuotaReservation(t.Context(), f, "run_1", admittedRecord, identityRef); !errors.Is(err, boom) || id != "" {
		t.Fatalf("failed hold = %q, %v; want the storage error and no id", id, err)
	}
}

func TestOneBucketPerRun(t *testing.T) {
	t.Parallel()
	j, runID := journalWithRun(t)
	reserver := admission.NewJournalReserver(j, billing.BuiltInCeilings())
	if _, err := admission.HoldQuotaReservation(t.Context(), reserver, runID, admittedRecord, identityRef); err != nil {
		t.Fatal(err)
	}
	// A second hold for the same run is refused, on the same bucket or any other.
	for _, rec := range []qualify.Record{admittedRecord, bucketRecord("codex", "cli", "included-plan")} {
		if _, err := admission.HoldQuotaReservation(t.Context(), reserver, runID, rec, identityRef); !errors.Is(err, admission.ErrDuplicateHold) {
			t.Fatalf("second hold err = %v, want ErrDuplicateHold", err)
		}
	}
	if n := reservationRows(t, j, runID); n != 1 {
		t.Fatalf("reservation rows = %d, want exactly 1", n)
	}
}

func TestUnknownQuantityNeverZero(t *testing.T) {
	t.Parallel()
	j, runID := journalWithRun(t)
	id, err := admission.HoldQuotaReservation(t.Context(), admission.NewJournalReserver(j, billing.BuiltInCeilings()), runID, admittedRecord, identityRef)
	if err != nil {
		t.Fatal(err)
	}
	row, err := j.Reservation(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if row.Quantity != "unknown" {
		t.Fatalf("held quantity = %q, want exactly \"unknown\" (never 0, never empty)", row.Quantity)
	}
}

func TestHoldExpiresAfterCeilingPlusGrace(t *testing.T) {
	t.Parallel()
	j, runID := journalWithRun(t)
	c := billing.Ceilings{Execution: 10 * time.Minute, Repairs: 1, Replans: 1, TransportRetries: 1}
	before := time.Now()
	id, err := admission.HoldQuotaReservation(t.Context(), admission.NewJournalReserver(j, c), runID, admittedRecord, identityRef)
	if err != nil {
		t.Fatal(err)
	}
	row, err := j.Reservation(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	created, err1 := time.Parse(time.RFC3339Nano, row.CreatedAt)
	expires, err2 := time.Parse(time.RFC3339Nano, row.ExpiresAt)
	if err1 != nil || err2 != nil {
		t.Fatalf("times %q %q: %v %v", row.CreatedAt, row.ExpiresAt, err1, err2)
	}
	if created.Before(before.Add(-time.Second)) || expires.Sub(created) != 10*time.Minute+time.Hour {
		t.Fatalf("created %v expires %v, want expiry = hold + 10m ceiling + 1h grace", created, expires)
	}
}

func TestReservationWordingIsLocalOnly(t *testing.T) {
	t.Parallel()
	const golden = "reservation held on bucket b1: local coordination only — not provider availability"
	if got := admission.ReservationText("b1"); got != golden {
		t.Fatalf("reservation text = %q, want %q", got, golden)
	}
	if admission.ReservationNote != "local coordination only — not provider availability" {
		t.Fatalf("ReservationNote = %q", admission.ReservationNote)
	}
}
