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

// ErrDuplicateHold reports a second live reservation for one run: S1 admits
// one bucket per run and preauthorises no alternative route (AC-6.4).
var ErrDuplicateHold = errors.New("run already holds a reservation")

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

type journalReserver struct{ c billing.Ceilings }

// NewJournalReserver returns the production Reserver, writing the
// reservations table through the run owner's transaction. A hold expires at
// its creation plus the execution ceiling plus one hour of grace.
func NewJournalReserver(c billing.Ceilings) Reserver { return journalReserver{c: c} }

func (r journalReserver) Reserve(ctx context.Context, tx *sql.Tx, runID, bucket, owner string) (string, error) {
	now := time.Now().UTC()
	id := ids.New("rsv")
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
