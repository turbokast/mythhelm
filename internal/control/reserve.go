package control

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// reservationTTL is how long a reservation holds before it is a reconcile
// input; no reaper ships with this task.
const reservationTTL = time.Hour

// unknownQuantity is the quantity of a reservation whose amount is unknown;
// it is never stored as 0 (I09).
const unknownQuantity = "unknown"

var decimalText = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// ReserveOptions carries the idempotency key, the key of the reservation
// (Host, Bucket, Scope) and its bounds.
type ReserveOptions struct {
	OperationID string
	Host        string // execution host; the handlers fill in this machine's name
	Bucket      string
	Scope       string
	Quantity    string // a positive decimal, or "unknown"; never "" or "0"
	Owner       string // the attempt or run holding the reservation
	Bound       string // the most one reservation may hold, a decimal; "" is no known bound
}

// Reserve records a reservation in the ledger, in the caller's mutation
// transaction when ctx carries one. Failure cases: allowance_exhausted when
// the bucket is exhausted or the quantity cannot be shown to be within
// Bound (new work is prevented, never force-admitted); revision_conflict
// when the host, bucket and scope are already held or the operation_id was
// used; invalid_contract for an empty field, a zero, negative or malformed
// quantity, or an unknown owner.
func Reserve(ctx context.Context, db *sql.DB, opts ReserveOptions) (v2contract.Reservation, error) {
	if err := opts.validate(); err != nil {
		return v2contract.Reservation{}, err
	}
	var out v2contract.Reservation
	err := Mutate(ctx, db, func(tx *sql.Tx) error {
		runID, err := ownerRun(ctx, tx, opts.Owner)
		if err != nil {
			return err
		}
		if err := checkBounds(ctx, tx, opts); err != nil {
			return err
		}
		id := "res_" + opts.OperationID
		var one int
		switch err := tx.QueryRowContext(ctx, `SELECT 1 FROM reservations WHERE reservation_id = ?`, id).Scan(&one); {
		case err == nil:
			return newError(CodeRevisionConflict, "operation_id %q already made a reservation", opts.OperationID)
		case !errors.Is(err, sql.ErrNoRows):
			return newError(CodePersistenceUnavailable, "reading reservation: %v", err)
		}
		var holder string
		switch err := tx.QueryRowContext(ctx, `SELECT owner FROM reservations WHERE execution_host = ? AND bucket = ? AND scope = ? AND status = 'held'`,
			opts.Host, opts.Bucket, opts.Scope).Scan(&holder); {
		case err == nil:
			return newError(CodeRevisionConflict, "bucket %q scope %q on host %q is held by %s", opts.Bucket, opts.Scope, opts.Host, holder)
		case !errors.Is(err, sql.ErrNoRows):
			return newError(CodePersistenceUnavailable, "reading held reservations: %v", err)
		}
		created, expires := time.Now().UTC(), time.Now().UTC().Add(reservationTTL)
		if _, err := tx.ExecContext(ctx, `INSERT INTO reservations
			(reservation_id, run_id, execution_host, bucket, scope, owner, quantity, status, expires_at, created_at)
			VALUES (?,?,?,?,?,?,?, 'held', ?, ?)`, id, runID, opts.Host, opts.Bucket, opts.Scope, opts.Owner, opts.Quantity,
			expires.Format(time.RFC3339Nano), created.Format(time.RFC3339Nano)); err != nil {
			return newError(CodePersistenceUnavailable, "recording reservation: %v", err)
		}
		out = v2contract.Reservation{SchemaVersion: v2contract.SchemaVersion, ReservationID: id, Bucket: opts.Bucket, Scope: opts.Scope,
			Owner: opts.Owner, Quantity: opts.Quantity, Status: "held", ExpiresAt: &expires}
		return nil
	})
	return out, err
}

func (o ReserveOptions) validate() error {
	for name, v := range map[string]string{"operation_id": o.OperationID, "host": o.Host, "bucket": o.Bucket, "scope": o.Scope, "owner": o.Owner} {
		if v == "" {
			return newError(CodeInvalidContract, "reservation %s is empty", name)
		}
	}
	if !validQuantity(o.Quantity) {
		return newError(CodeInvalidContract, "reservation quantity %q must be a positive decimal or %q", o.Quantity, unknownQuantity)
	}
	if o.Bound != "" && !decimalText.MatchString(o.Bound) {
		return newError(CodeInvalidContract, "reservation bound %q is not a decimal", o.Bound)
	}
	return nil
}

// validQuantity accepts "unknown" or a decimal above zero: unknown is not
// zero, and nothing is reserved for zero.
func validQuantity(q string) bool {
	if q == unknownQuantity {
		return true
	}
	if !decimalText.MatchString(q) {
		return false
	}
	r, _ := new(big.Rat).SetString(q)
	return r.Sign() > 0
}

// ownerRun resolves the run an owner (an attempt or a run) belongs to.
func ownerRun(ctx context.Context, tx *sql.Tx, owner string) (string, error) {
	var runID string
	err := tx.QueryRowContext(ctx, `SELECT run_id FROM attempts WHERE attempt_id = ?`, owner).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `SELECT run_id FROM runs WHERE run_id = ?`, owner).Scan(&runID)
	}
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", newError(CodeInvalidContract, "owner %q is neither an attempt nor a run", owner)
	case err != nil:
		return "", newError(CodePersistenceUnavailable, "resolving owner: %v", err)
	}
	return runID, nil
}

// checkBounds refuses a reservation against an exhausted bucket, and one
// whose quantity is not shown to be within the bound. An unset bound is
// unknown: it is never invented, so it refuses nothing by itself (I02).
func checkBounds(ctx context.Context, tx *sql.Tx, o ReserveOptions) error {
	var one int
	switch err := tx.QueryRowContext(ctx, `SELECT 1 FROM bucket_state WHERE bucket = ?`, o.Bucket).Scan(&one); {
	case err == nil:
		return newError(CodeAllowanceExhausted, "bucket %q is exhausted", o.Bucket)
	case !errors.Is(err, sql.ErrNoRows):
		return newError(CodePersistenceUnavailable, "reading bucket state: %v", err)
	}
	if o.Bound == "" {
		return nil
	}
	if o.Quantity == unknownQuantity {
		return newError(CodeAllowanceExhausted, "an unknown quantity cannot be shown to be within the bound %s", o.Bound)
	}
	q, _ := new(big.Rat).SetString(o.Quantity)
	bound, _ := new(big.Rat).SetString(o.Bound)
	if q.Cmp(bound) > 0 {
		return newError(CodeAllowanceExhausted, "quantity %s exceeds the bound %s", o.Quantity, o.Bound)
	}
	return nil
}

// Release releases a held or orphaned reservation and records the evidence.
// operationID is the caller's idempotency key and must be set; Execute
// supplies the replay. Failure cases: invalid_contract (unknown reservation,
// empty evidence), revision_conflict (already released).
func Release(ctx context.Context, db *sql.DB, operationID, reservationID, evidence string) error {
	if operationID == "" || reservationID == "" || evidence == "" {
		return newError(CodeInvalidContract, "release needs an operation_id, a reservation_id and evidence")
	}
	return Mutate(ctx, db, func(tx *sql.Tx) error {
		if err := requireReservation(ctx, tx, reservationID); err != nil {
			return err
		}
		return transition(journal.SetReservationReleased(ctx, tx, reservationID, evidence, time.Now()), reservationID)
	})
}

// Heartbeat refreshes heartbeat_at on a held reservation.
func Heartbeat(ctx context.Context, db *sql.DB, reservationID string) error {
	if reservationID == "" {
		return newError(CodeInvalidContract, "heartbeat needs a reservation_id")
	}
	return Mutate(ctx, db, func(tx *sql.Tx) error {
		if err := requireReservation(ctx, tx, reservationID); err != nil {
			return err
		}
		return transition(journal.TouchReservation(ctx, tx, reservationID, time.Now()), reservationID)
	})
}

func requireReservation(ctx context.Context, tx *sql.Tx, id string) error {
	var one int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM reservations WHERE reservation_id = ?`, id).Scan(&one)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return newError(CodeInvalidContract, "reservation %q is unknown", id)
	case err != nil:
		return newError(CodePersistenceUnavailable, "reading reservation: %v", err)
	}
	return nil
}

// transition maps a refused status change to revision_conflict.
func transition(err error, id string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sql.ErrNoRows):
		return newError(CodeRevisionConflict, "reservation %q is not in a state that allows this change", id)
	default:
		return newError(CodePersistenceUnavailable, "updating reservation: %v", err)
	}
}

// authorize requires the capability token of the attempt that owns a
// reservation; a run-owned reservation needs only the peer authentication
// the transport already did (AC-5.3).
func authorize(ctx context.Context, db *sql.DB, token, owner string) error {
	var one int
	err := db.QueryRowContext(ctx, `SELECT 1 FROM attempts WHERE attempt_id = ?`, owner).Scan(&one)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return newError(CodePersistenceUnavailable, "reading attempt: %v", err)
	}
	return CheckToken(ctx, db, token, owner)
}

// strictParams decodes an intent's params into dst, rejecting unknown keys.
func strictParams(in Intent, dst any) error {
	if len(in.Params) == 0 {
		return newError(CodeInvalidContract, "%s needs params", in.Method)
	}
	dec := json.NewDecoder(bytes.NewReader(in.Params))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return newError(CodeInvalidContract, "%s params: %v", in.Method, err)
	}
	return nil
}

func hostName() (string, error) {
	h, err := os.Hostname()
	if err != nil || strings.TrimSpace(h) == "" {
		return "", newError(CodePersistenceUnavailable, "cannot name this execution host: %v", err)
	}
	return h, nil
}

type reserveParams struct {
	Bucket   string `json:"bucket"`
	Scope    string `json:"scope"`
	Quantity string `json:"quantity"`
	Owner    string `json:"owner"`
	Bound    string `json:"bound"`
}

// ReserveHandler answers the reserve intent for this execution host. The
// owner's capability token is required when the owner is an attempt.
func ReserveHandler(db *sql.DB) Handler {
	return func(ctx context.Context, _ Peer, in Intent) (Result, error) {
		var p reserveParams
		if err := strictParams(in, &p); err != nil {
			return Result{}, err
		}
		host, err := hostName()
		if err != nil {
			return Result{}, err
		}
		if err := authorize(ctx, db, in.CapabilityToken, p.Owner); err != nil {
			return Result{}, err
		}
		res, err := Reserve(ctx, db, ReserveOptions{OperationID: in.OperationID, Host: host, Bucket: p.Bucket, Scope: p.Scope,
			Quantity: p.Quantity, Owner: p.Owner, Bound: p.Bound})
		if err != nil {
			return Result{}, err
		}
		body, err := json.Marshal(res)
		return Result{Body: body}, err
	}
}

// ReleaseHandler answers the release intent.
func ReleaseHandler(db *sql.DB) Handler {
	return reservationHandler(db, func(ctx context.Context, in Intent, id string, p reservationParams) error {
		return Release(ctx, db, in.OperationID, id, p.Evidence)
	})
}

// HeartbeatHandler answers the heartbeat intent.
func HeartbeatHandler(db *sql.DB) Handler {
	return reservationHandler(db, func(ctx context.Context, _ Intent, id string, _ reservationParams) error {
		return Heartbeat(ctx, db, id)
	})
}

type reservationParams struct {
	ReservationID string `json:"reservation_id"`
	Evidence      string `json:"evidence"`
}

func reservationHandler(db *sql.DB, apply func(context.Context, Intent, string, reservationParams) error) Handler {
	return func(ctx context.Context, _ Peer, in Intent) (Result, error) {
		var p reservationParams
		if err := strictParams(in, &p); err != nil {
			return Result{}, err
		}
		var owner string
		err := db.QueryRowContext(ctx, `SELECT owner FROM reservations WHERE reservation_id = ?`, p.ReservationID).Scan(&owner)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return Result{}, newError(CodeInvalidContract, "reservation %q is unknown", p.ReservationID)
		case err != nil:
			return Result{}, newError(CodePersistenceUnavailable, "reading reservation: %v", err)
		}
		if err := authorize(ctx, db, in.CapabilityToken, owner); err != nil {
			return Result{}, err
		}
		if err := apply(ctx, in, p.ReservationID, p); err != nil {
			return Result{}, err
		}
		body, err := json.Marshal(map[string]string{"reservation_id": p.ReservationID})
		return Result{Body: body}, err
	}
}
