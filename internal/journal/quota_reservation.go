package journal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/ids"
)

// QuotaReservationInput is trusted storage data, not execution authority.
type QuotaReservationInput struct {
	RunID, Bucket  string
	ExecutionNanos int64
	ObservedAt     time.Time
}

// QuotaRefusal reports a fixed-schedule refusal.
type QuotaRefusal struct {
	Bucket string
	Wait   time.Duration
	GiveUp bool
}

func (e *QuotaRefusal) Error() string {
	return fmt.Sprintf("%s: exhausted quota bucket %s", billing.CodeAllowanceExhausted, e.Bucket)
}
func (e *QuotaRefusal) Unwrap() error { return billing.ErrAllowanceExhausted }

// ErrDuplicateQuotaHold reports a second held reservation for a run.
var ErrDuplicateQuotaHold = errors.New("duplicate quota hold")

// ReserveRunQuota applies the fixed policy and writes a checked local hold.
func ReserveRunQuota(ctx context.Context, tx *sql.Tx, in QuotaReservationInput) (string, error) {
	if err := validateQuotaInput(tx, in); err != nil {
		return "", err
	}
	var row BucketRow
	var reset sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT exhausted_at, reset_at, retries_used FROM bucket_state WHERE bucket = ?`, in.Bucket).Scan(&row.ExhaustedAt, &reset, &row.RetriesUsed)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("reading bucket state of %s: %w", in.Bucket, err)
	}
	if err == nil {
		if reset.Valid {
			row.ResetAt = &reset.String
		}
		// A passed authoritative reset ends even a spent episode, matching
		// legacy admission's reset override before the retry policy.
		cleared := false
		if row.ResetAt != nil {
			if at, parseErr := time.Parse(time.RFC3339Nano, *row.ResetAt); parseErr == nil && !in.ObservedAt.Before(at) && row.RetriesUsed >= 0 {
				if err := ClearBucket(ctx, tx, in.Bucket); err != nil {
					return "", err
				}
				cleared = true
			}
		}
		if !cleared {
			admit, wait, giveUp := billing.EvaluateQuotaSchedule(row.ExhaustedAt, row.ResetAt, row.RetriesUsed, in.ObservedAt)
			if !admit {
				return "", &QuotaRefusal{Bucket: in.Bucket, Wait: wait, GiveUp: giveUp}
			}
			if err := NoteBucketRetry(ctx, tx, in.Bucket); err != nil {
				return "", err
			}
		}
	}
	return InsertRunQuotaHold(ctx, tx, in)
}

// InsertRunQuotaHold checks duplicates and inserts a local hold. It deliberately
// does not evaluate scheduling: legacy admission checks its evaluator first.
// Neither this storage effect nor its input grants execution permission.
func InsertRunQuotaHold(ctx context.Context, tx *sql.Tx, in QuotaReservationInput) (string, error) {
	if err := validateQuotaInput(tx, in); err != nil {
		return "", err
	}
	var held int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM reservations WHERE run_id = ? AND status = 'held'`, in.RunID).Scan(&held); err != nil {
		return "", fmt.Errorf("counting held reservations of %s: %w", in.RunID, err)
	}
	if held > 0 {
		return "", fmt.Errorf("%w: %s", ErrDuplicateQuotaHold, in.RunID)
	}
	now := in.ObservedAt.UTC()
	id := ids.New("rsv")
	if err := InsertReservation(ctx, tx, ReservationRow{
		ReservationID: id, RunID: in.RunID, Bucket: in.Bucket, Scope: in.Bucket, Owner: in.RunID,
		Quantity: "unknown", Status: "held",
		ExpiresAt: now.Add(time.Duration(in.ExecutionNanos) + time.Hour).Format(time.RFC3339Nano),
		CreatedAt: now.Format(time.RFC3339Nano),
	}); err != nil {
		return "", err
	}
	return id, nil
}

func validateQuotaInput(tx *sql.Tx, in QuotaReservationInput) error {
	if tx == nil || in.RunID == "" || in.Bucket == "" || in.ObservedAt.IsZero() || in.ExecutionNanos < 0 || in.ExecutionNanos > math.MaxInt64-int64(time.Hour) {
		return errors.New("invalid quota reservation input")
	}
	return nil
}
