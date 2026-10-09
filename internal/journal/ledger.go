package journal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// UsageRow is one durable usage observation. A blank ProducerID stores NULL:
// the observation has no event identity (an unknown marker, or an
// identity-less delta).
type UsageRow struct {
	ObservationID       string
	RunID               string
	Scope, Unit, Source string
	Label               string // reported|observed|estimated|user-declared|unknown
	Quantity            string // decimal text or exactly "unknown"
	ProducerID          string
	ProducerSequence    int64
	ObservedAt          string
}

// EnvelopeRow is a run's finite envelope and what it has used.
type EnvelopeRow struct {
	RunID                              string
	ExecutionSeconds, Repairs, Replans int64
	TransportRetries                   int64
	FirstStartAt                       string
	RepairsUsed, ReplansUsed           int64
	TransportRetriesSeen               int64
	PauseSpans                         string // JSON array, default "[]"
	UpdatedAt                          string
}

// ReservationRow is a typed quota reservation coupled to a billing bucket.
type ReservationRow struct {
	ReservationID        string
	RunID                string
	Bucket, Scope, Owner string
	Quantity             string // decimal text or exactly "unknown"
	Status               string // held|released|orphaned
	ExpiresAt            string
	HeartbeatAt          string
	ReleaseEvidence      string
	CreatedAt            string
}

// BucketRow is an exhausted bucket. A nil ResetAt is an unknown reset time,
// never an invented one.
type BucketRow struct {
	Bucket      string
	ExhaustedAt string
	ResetAt     *string
	RetriesUsed int64
}

var (
	usageLabels         = []string{"reported", "observed", "estimated", "user-declared", "unknown"}
	reservationStatuses = []string{"held", "released", "orphaned"}
)

// Transact runs fn in one transaction on the run owner's connection, and
// commits only if fn returns nil.
func (j *Journal) Transact(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("journal: starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("journal: committing transaction: %w", err)
	}
	return nil
}

func nullInt(v int64, valid bool) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: valid} }

// InsertUsageObservation stores o. An empty or out-of-scale label and an
// empty quantity are errors, never defaulted (I09).
func InsertUsageObservation(ctx context.Context, tx *sql.Tx, o UsageRow) error {
	switch {
	case o.ObservationID == "" || o.RunID == "" || o.Scope == "" || o.ObservedAt == "":
		return errors.New("journal: usage observation needs an id, run, scope and time")
	case !slices.Contains(usageLabels, o.Label):
		return fmt.Errorf("journal: usage observation label %q is not one of %s", o.Label, strings.Join(usageLabels, ", "))
	case o.Quantity == "":
		return errors.New("journal: usage observation quantity is empty; use \"unknown\" for an unreported value")
	case o.ProducerID == "" && o.ProducerSequence != 0:
		return errors.New("journal: usage observation has a producer sequence but no producer")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO usage_observations
		(observation_id, run_id, scope, unit, source, label, quantity, producer_id, producer_sequence, observed_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		o.ObservationID, o.RunID, o.Scope, o.Unit, o.Source, o.Label, o.Quantity,
		nullable(o.ProducerID), nullInt(o.ProducerSequence, o.ProducerID != ""), o.ObservedAt); err != nil {
		return fmt.Errorf("journal: inserting usage observation %s: %w", o.ObservationID, err)
	}
	return nil
}

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// UsageObservations returns runID's observations in insertion order.
func (j *Journal) UsageObservations(ctx context.Context, runID string) ([]UsageRow, error) {
	return usageObservations(ctx, j.db, runID)
}

// UsageObservationsTx is UsageObservations inside tx.
func UsageObservationsTx(ctx context.Context, tx *sql.Tx, runID string) ([]UsageRow, error) {
	return usageObservations(ctx, tx, runID)
}

func usageObservations(ctx context.Context, q queryer, runID string) ([]UsageRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT observation_id, run_id, scope, unit, source, label, quantity,
		producer_id, producer_sequence, observed_at FROM usage_observations WHERE run_id = ? ORDER BY rowid`, runID)
	if err != nil {
		return nil, fmt.Errorf("journal: reading usage observations of %s: %w", runID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []UsageRow
	for rows.Next() {
		var o UsageRow
		var producer sql.NullString
		var seq sql.NullInt64
		if err := rows.Scan(&o.ObservationID, &o.RunID, &o.Scope, &o.Unit, &o.Source, &o.Label, &o.Quantity,
			&producer, &seq, &o.ObservedAt); err != nil {
			return nil, fmt.Errorf("journal: scanning usage observation: %w", err)
		}
		o.ProducerID, o.ProducerSequence = producer.String, seq.Int64
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("journal: reading usage observations of %s: %w", runID, err)
	}
	return out, nil
}

// UpsertRunEnvelope stores e, replacing the run's earlier envelope.
func UpsertRunEnvelope(ctx context.Context, tx *sql.Tx, e EnvelopeRow) error {
	if e.RunID == "" || e.UpdatedAt == "" {
		return errors.New("journal: run envelope needs a run and an update time")
	}
	if e.PauseSpans == "" {
		e.PauseSpans = "[]"
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO run_envelopes
		(run_id, execution_seconds, repairs, replans, transport_retries, first_start_at,
		 repairs_used, replans_used, transport_retries_seen, pause_spans, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(run_id) DO UPDATE SET execution_seconds=excluded.execution_seconds, repairs=excluded.repairs,
		 replans=excluded.replans, transport_retries=excluded.transport_retries, first_start_at=excluded.first_start_at,
		 repairs_used=excluded.repairs_used, replans_used=excluded.replans_used,
		 transport_retries_seen=excluded.transport_retries_seen, pause_spans=excluded.pause_spans,
		 updated_at=excluded.updated_at`,
		e.RunID, e.ExecutionSeconds, e.Repairs, e.Replans, e.TransportRetries, nullable(e.FirstStartAt),
		e.RepairsUsed, e.ReplansUsed, e.TransportRetriesSeen, e.PauseSpans, e.UpdatedAt); err != nil {
		return fmt.Errorf("journal: storing envelope of %s: %w", e.RunID, err)
	}
	return nil
}

// RunEnvelope returns runID's envelope, or an error wrapping sql.ErrNoRows.
func (j *Journal) RunEnvelope(ctx context.Context, runID string) (EnvelopeRow, error) {
	var e EnvelopeRow
	var first sql.NullString
	err := j.db.QueryRowContext(ctx, `SELECT run_id, execution_seconds, repairs, replans, transport_retries,
		first_start_at, repairs_used, replans_used, transport_retries_seen, pause_spans, updated_at
		FROM run_envelopes WHERE run_id = ?`, runID).Scan(&e.RunID, &e.ExecutionSeconds, &e.Repairs, &e.Replans,
		&e.TransportRetries, &first, &e.RepairsUsed, &e.ReplansUsed, &e.TransportRetriesSeen, &e.PauseSpans, &e.UpdatedAt)
	if err != nil {
		return EnvelopeRow{}, fmt.Errorf("journal: reading envelope of %s: %w", runID, err)
	}
	e.FirstStartAt = first.String
	return e, nil
}

// InsertReservation stores r. An empty quantity or out-of-scale status is an
// error.
func InsertReservation(ctx context.Context, tx *sql.Tx, r ReservationRow) error {
	switch {
	case r.ReservationID == "" || r.RunID == "" || r.Bucket == "" || r.Scope == "" || r.Owner == "" || r.ExpiresAt == "" || r.CreatedAt == "":
		return errors.New("journal: reservation needs an id, run, bucket, scope, owner and times")
	case r.Quantity == "":
		return errors.New("journal: reservation quantity is empty; use \"unknown\" for an unknown amount")
	case !slices.Contains(reservationStatuses, r.Status):
		return fmt.Errorf("journal: reservation status %q is not one of %s", r.Status, strings.Join(reservationStatuses, ", "))
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO reservations
		(reservation_id, run_id, bucket, scope, owner, quantity, status, expires_at, heartbeat_at, release_evidence, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		r.ReservationID, r.RunID, r.Bucket, r.Scope, r.Owner, r.Quantity, r.Status, r.ExpiresAt,
		nullable(r.HeartbeatAt), nullable(r.ReleaseEvidence), r.CreatedAt); err != nil {
		return fmt.Errorf("journal: inserting reservation %s: %w", r.ReservationID, err)
	}
	return nil
}

// Reservation returns the reservation, or an error wrapping sql.ErrNoRows.
func (j *Journal) Reservation(ctx context.Context, reservationID string) (ReservationRow, error) {
	var r ReservationRow
	var heartbeat, evidence sql.NullString
	err := j.db.QueryRowContext(ctx, `SELECT reservation_id, run_id, bucket, scope, owner, quantity, status,
		expires_at, heartbeat_at, release_evidence, created_at FROM reservations WHERE reservation_id = ?`,
		reservationID).Scan(&r.ReservationID, &r.RunID, &r.Bucket, &r.Scope, &r.Owner, &r.Quantity, &r.Status,
		&r.ExpiresAt, &heartbeat, &evidence, &r.CreatedAt)
	if err != nil {
		return ReservationRow{}, fmt.Errorf("journal: reading reservation %s: %w", reservationID, err)
	}
	r.HeartbeatAt, r.ReleaseEvidence = heartbeat.String, evidence.String
	return r, nil
}

// updateReservation runs one constant UPDATE of a single reservation (its
// last placeholder is the id) and reports an error wrapping sql.ErrNoRows
// when the reservation is not in a status the update accepts.
func updateReservation(ctx context.Context, tx *sql.Tx, id, accepts, query string, args ...any) error {
	res, err := tx.ExecContext(ctx, query, append(args, id)...)
	if err != nil {
		return fmt.Errorf("journal: updating reservation %s: %w", id, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("journal: updating reservation %s: %w", id, err)
	} else if n == 0 {
		return fmt.Errorf("journal: reservation %s is not %s: %w", id, accepts, sql.ErrNoRows)
	}
	return nil
}

// SetReservationReleased releases a held or orphaned reservation, recording
// the evidence and the time.
func SetReservationReleased(ctx context.Context, tx *sql.Tx, reservationID, evidence string, at time.Time) error {
	return updateReservation(ctx, tx, reservationID, "held or orphaned",
		`UPDATE reservations SET status='released', release_evidence=?, heartbeat_at=? WHERE status IN ('held','orphaned') AND reservation_id = ?`,
		evidence, formatTime(at))
}

// TouchReservation records a heartbeat on a held reservation.
func TouchReservation(ctx context.Context, tx *sql.Tx, reservationID string, at time.Time) error {
	return updateReservation(ctx, tx, reservationID, "held",
		`UPDATE reservations SET heartbeat_at=? WHERE status='held' AND reservation_id = ?`, formatTime(at))
}

// MarkReservationOrphaned marks a held reservation whose run owner is gone.
func MarkReservationOrphaned(ctx context.Context, tx *sql.Tx, reservationID string) error {
	return updateReservation(ctx, tx, reservationID, "held",
		`UPDATE reservations SET status='orphaned' WHERE status='held' AND reservation_id = ?`)
}

// SetBucketExhausted records bucket as exhausted. A nil resetAt stays unknown;
// retries already counted are kept.
func SetBucketExhausted(ctx context.Context, tx *sql.Tx, bucket, exhaustedAt string, resetAt *string) error {
	if bucket == "" || exhaustedAt == "" {
		return errors.New("journal: exhausted bucket needs a name and a time")
	}
	var reset sql.NullString
	if resetAt != nil {
		reset = sql.NullString{String: *resetAt, Valid: true}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO bucket_state (bucket, exhausted_at, reset_at) VALUES (?,?,?)
		ON CONFLICT(bucket) DO UPDATE SET exhausted_at=excluded.exhausted_at, reset_at=excluded.reset_at`,
		bucket, exhaustedAt, reset); err != nil {
		return fmt.Errorf("journal: recording exhausted bucket: %w", err)
	}
	return nil
}

// BucketState returns the bucket's exhaustion, or an error wrapping
// sql.ErrNoRows when it is not exhausted.
func (j *Journal) BucketState(ctx context.Context, bucket string) (BucketRow, error) {
	b := BucketRow{}
	var reset sql.NullString
	err := j.db.QueryRowContext(ctx, `SELECT bucket, exhausted_at, reset_at, retries_used FROM bucket_state WHERE bucket = ?`,
		bucket).Scan(&b.Bucket, &b.ExhaustedAt, &reset, &b.RetriesUsed)
	if err != nil {
		return BucketRow{}, fmt.Errorf("journal: reading bucket state: %w", err)
	}
	if reset.Valid {
		b.ResetAt = &reset.String
	}
	return b, nil
}

// NoteBucketRetry counts one retry against an exhausted bucket.
func NoteBucketRetry(ctx context.Context, tx *sql.Tx, bucket string) error {
	res, err := tx.ExecContext(ctx, `UPDATE bucket_state SET retries_used = retries_used + 1 WHERE bucket = ?`, bucket)
	if err != nil {
		return fmt.Errorf("journal: counting bucket retry: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("journal: counting bucket retry: %w", err)
	} else if n == 0 {
		return fmt.Errorf("journal: bucket is not exhausted: %w", sql.ErrNoRows)
	}
	return nil
}

// ClearBucket removes the bucket's exhaustion record.
func ClearBucket(ctx context.Context, tx *sql.Tx, bucket string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM bucket_state WHERE bucket = ?`, bucket); err != nil {
		return fmt.Errorf("journal: clearing bucket: %w", err)
	}
	return nil
}
