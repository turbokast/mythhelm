package supervisor

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/journal"
)

// unknownResetBackoff is the wait before each scheduled retry when the
// native gave no reset time (AC-6.2): the first retry waits 1m after the
// exhaustion, the second 5m after the next, the third 15m.
var unknownResetBackoff = [admission.MaxBucketRetries]time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute}

// RetrySchedule is the retry plan for an exhausted bucket: at most
// admission.MaxBucketRetries retries. With an authoritative reset time the
// retry waits for that time itself (wait 0 here); with none it backs off
// 1m, 5m and 15m after the exhaustion. After the last retry it gives up,
// and its timing is then unknown, never invented.
func RetrySchedule(resetAt *time.Time, retriesUsed int) (wait time.Duration, giveUp bool) {
	if retriesUsed >= admission.MaxBucketRetries {
		return 0, true
	}
	if resetAt != nil {
		return 0, false
	}
	return unknownResetBackoff[retriesUsed], false
}

// EvaluateBucket enforces the schedule at re-admission: admit iff now has
// reached the next scheduled time (the reset time, or the exhaustion plus
// the backoff), otherwise refuse with the wait remaining. A time the row
// cannot read refuses, so a damaged row never admits (I02). It has no
// effect on the row; the caller counts an admission.
func EvaluateBucket(row journal.BucketRow, now time.Time) (admit bool, wait time.Duration, giveUp bool) {
	var reset *time.Time
	if row.ResetAt != nil {
		t, err := time.Parse(time.RFC3339Nano, *row.ResetAt)
		if err != nil {
			return false, 0, false
		}
		reset = &t
	}
	backoff, giveUp := RetrySchedule(reset, int(row.RetriesUsed))
	if giveUp {
		return false, 0, true
	}
	next := time.Time{}
	if reset != nil {
		next = *reset
	} else {
		exhausted, err := time.Parse(time.RFC3339Nano, row.ExhaustedAt)
		if err != nil {
			return false, 0, false
		}
		next = exhausted.Add(backoff)
	}
	if now.Before(next) {
		return false, next.Sub(now), false
	}
	return true, 0, false
}

// HandleExhaustion records the exhausted bucket, blocks the run with reason
// allowance_exhausted and releases the run's reservation, in one
// transaction. It runs only once the attempt is terminal and its candidate
// frozen (AC-3.4, I12). The reservation names the bucket. The reset time
// stays unknown unless the native gave one (I09). A journal failure returns
// an error and journals nothing.
func HandleExhaustion(ctx context.Context, j *journal.Journal, p *Producer, runID, reservationID string, resetAt *time.Time) error {
	res, err := j.Reservation(ctx, reservationID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	var reset *string
	if resetAt != nil {
		reset = new(resetAt.UTC().Format(time.RFC3339Nano))
	}
	return transitionRun(ctx, j, runID, RunBlocked, "allowance_exhausted", p, func(tx *sql.Tx) error {
		if err := journal.SetBucketExhausted(ctx, tx, res.Bucket, now.Format(time.RFC3339Nano), reset); err != nil {
			return err
		}
		return journal.SetReservationReleased(ctx, tx, reservationID, "attempt terminal: allowance_exhausted", now)
	})
}

// heldReservations returns the ids of the run's held reservations.
func heldReservations(ctx context.Context, j *journal.Journal, runID string) ([]string, error) {
	var out []string
	err := j.Transact(ctx, func(tx *sql.Tx) error {
		var err error
		out, err = heldReservationsTx(ctx, tx, runID)
		return err
	})
	return out, err
}

func heldReservationsTx(ctx context.Context, tx *sql.Tx, runID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT reservation_id FROM reservations WHERE run_id = ? AND status = 'held' ORDER BY reservation_id`, runID)
	if err != nil {
		return nil, fmt.Errorf("listing held reservations of %s: %w", runID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("reading a held reservation of %s: %w", runID, err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing held reservations of %s: %w", runID, err)
	}
	return out, nil
}

// releaseHeldReservations releases the run's held reservations with
// evidence, once its attempt is terminal.
func releaseHeldReservations(ctx context.Context, j *journal.Journal, runID, evidence string) error {
	return j.Transact(ctx, func(tx *sql.Tx) error {
		held, err := heldReservationsTx(ctx, tx, runID)
		if err != nil {
			return err
		}
		for _, id := range held {
			if err := journal.SetReservationReleased(ctx, tx, id, evidence, time.Now().UTC()); err != nil {
				return err
			}
		}
		return nil
	})
}

// orphanHeldReservations marks the run's held reservations orphaned: its
// attempt could not be reconciled, so nothing proves the claim ended. A
// later hold mints a new id and never reuses an orphan (AC-3.4).
func orphanHeldReservations(ctx context.Context, j *journal.Journal, runID string) error {
	return j.Transact(ctx, func(tx *sql.Tx) error {
		held, err := heldReservationsTx(ctx, tx, runID)
		if err != nil {
			return err
		}
		for _, id := range held {
			if err := journal.MarkReservationOrphaned(ctx, tx, id); err != nil {
				return err
			}
		}
		return nil
	})
}
