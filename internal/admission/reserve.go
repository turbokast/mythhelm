package admission

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/qualify"
)

// ReservationNote is the wording every reservation surface carries: a hold
// is a local claim, never a statement about the provider's quota (I10).
const ReservationNote = "local coordination only — not provider availability"

// reservationGrace is how long a hold outlives the execution ceiling before
// it can be treated as abandoned.
const reservationGrace = time.Hour

// MaxBucketRetries is how many scheduled retries an exhausted bucket gets
// before its schedule is spent (AC-6.2).
const MaxBucketRetries = 3

// ErrDuplicateHold reports a second live reservation for one run: S1 admits
// one bucket per run and preauthorises no alternative route (AC-6.4).
var ErrDuplicateHold = errors.New("run already holds a reservation")

// BucketEvaluator decides a re-admission against an exhausted bucket's
// retry schedule: admit iff the next scheduled time has come, otherwise
// refuse with the wait remaining; giveUp is terminal (AC-6.2). The
// supervisor supplies its EvaluateBucket.
type BucketEvaluator func(row journal.BucketRow, now time.Time) (admit bool, wait time.Duration, giveUp bool)

// ExhaustedError reports a new hold refused because the bucket is live in
// bucket_state and its retry schedule has not come due (AC-6.1). It wraps
// billing.ErrAllowanceExhausted.
type ExhaustedError struct {
	Bucket string
	Wait   time.Duration
	GiveUp bool
}

func (e *ExhaustedError) Error() string {
	if e.GiveUp {
		return fmt.Sprintf("%s: bucket %s used its scheduled retries; next retry time unknown", billing.CodeAllowanceExhausted, e.Bucket)
	}
	return fmt.Sprintf("%s: bucket %s is exhausted; next scheduled admission in %s", billing.CodeAllowanceExhausted, e.Bucket, e.Wait.Round(time.Second))
}

func (e *ExhaustedError) Unwrap() error { return billing.ErrAllowanceExhausted }

// Reserver holds quota reservations inside the caller's transaction, so a
// hold commits or rolls back with the admission it belongs to (AC-3.2). It
// is declared here, where admission consumes it, so tests can use a fake.
type Reserver interface {
	// Reserve holds one reservation for runID on bucket, owned by owner.
	// A second live hold for the same run returns ErrDuplicateHold; a
	// storage failure wraps its cause.
	Reserve(ctx context.Context, tx *sql.Tx, runID, bucket, owner string) (reservationID string, err error)
}

// QuotaBucket names the admitted billing bucket from the record's harness,
// surface and entitlement class and the account identity reference. The
// fields are JSON-encoded, so no field value can move a boundary. An empty
// field is "unknown", never an empty segment (I09).
func QuotaBucket(rec qualify.Record, identityRef string) string {
	parts := []string{rec.Key.Harness, rec.Key.Surface, rec.Key.EntitlementClass, identityRef}
	for i, p := range parts {
		if p == "" {
			parts[i] = "unknown"
		}
	}
	b, err := json.Marshal(parts)
	if err != nil {
		// Unreachable: a slice of strings always marshals.
		return ""
	}
	return string(b)
}

// ReservationText is the line a surface shows for a held reservation. It
// names the bucket, not the reservation id, so a run's output stays stable
// across runs.
func ReservationText(bucket string) string {
	return fmt.Sprintf("reservation held on bucket %s: %s", bucket, ReservationNote)
}

type journalReserver struct {
	c        billing.Ceilings
	evaluate BucketEvaluator
}

// NewJournalReserver returns the production Reserver, writing the
// reservations table through the run owner's transaction. A hold expires at
// its creation plus the execution ceiling plus one hour of grace.
func NewJournalReserver(c billing.Ceilings, evaluate BucketEvaluator) Reserver {
	return journalReserver{c: c, evaluate: evaluate}
}

func (r journalReserver) Reserve(ctx context.Context, tx *sql.Tx, runID, bucket, owner string) (string, error) {
	now := time.Now().UTC()
	id := ids.New("rsv")
	if err := r.checkBucket(ctx, tx, bucket, now); err != nil {
		return "", err
	}
	var held int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM reservations WHERE run_id = ? AND status = 'held'`, runID).Scan(&held); err != nil {
		return "", fmt.Errorf("counting held reservations of %s: %w", runID, err)
	}
	if held > 0 {
		return "", fmt.Errorf("%w: %s", ErrDuplicateHold, runID)
	}
	if err := journal.InsertReservation(ctx, tx, journal.ReservationRow{
		ReservationID: id, RunID: runID, Bucket: bucket, Scope: bucket, Owner: owner,
		Quantity: "unknown", Status: "held",
		ExpiresAt: now.Add(r.c.Execution + reservationGrace).Format(time.RFC3339Nano),
		CreatedAt: now.Format(time.RFC3339Nano),
	}); err != nil {
		return "", err
	}
	return id, nil
}

// checkBucket refuses new model work on a bucket that is live in
// bucket_state until its schedule comes due. A due re-admission is counted
// here, in the admission transaction, and the row is cleared once its
// retries are spent or its reset time has passed; if the bucket is still
// exhausted the native signal re-blocks the next attempt. A refusal changes
// nothing. A bucket with no evaluator never admits (I02).
func (r journalReserver) checkBucket(ctx context.Context, tx *sql.Tx, bucket string, now time.Time) error {
	row := journal.BucketRow{Bucket: bucket}
	var reset sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT exhausted_at, reset_at, retries_used FROM bucket_state WHERE bucket = ?`, bucket).
		Scan(&row.ExhaustedAt, &reset, &row.RetriesUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading bucket state of %s: %w", bucket, err)
	}
	if reset.Valid {
		row.ResetAt = &reset.String
	}
	if r.evaluate == nil {
		return &ExhaustedError{Bucket: bucket}
	}
	admit, wait, giveUp := r.evaluate(row, now)
	if !admit {
		return &ExhaustedError{Bucket: bucket, Wait: wait, GiveUp: giveUp}
	}
	if err := journal.NoteBucketRetry(ctx, tx, bucket); err != nil {
		return err
	}
	resetPassed := false
	if row.ResetAt != nil {
		if t, err := time.Parse(time.RFC3339Nano, *row.ResetAt); err == nil {
			resetPassed = !now.Before(t)
		}
	}
	if row.RetriesUsed+1 >= MaxBucketRetries || resetPassed {
		return journal.ClearBucket(ctx, tx, bucket)
	}
	return nil
}

// HoldQuotaReservation holds the run's reservation coupled to the admitted
// bucket inside tx and returns its id. The quantity is unknown: S1 cannot
// read provider quota (I09). A failure holds nothing; the caller rolls tx
// back and blocks the run (I02).
func HoldQuotaReservation(ctx context.Context, tx *sql.Tx, r Reserver, runID string, rec qualify.Record, identityRef string) (string, error) {
	id, err := r.Reserve(ctx, tx, runID, QuotaBucket(rec, identityRef), runID)
	if err != nil {
		return "", fmt.Errorf("holding the quota reservation of %s: %w", runID, err)
	}
	return id, nil
}
