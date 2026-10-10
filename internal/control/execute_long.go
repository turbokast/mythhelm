package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

const (
	// longPollInterval is how often a duplicate long intent re-reads a
	// live claim while waiting for its result.
	longPollInterval = 20 * time.Millisecond
	// maxClaimAge bounds one long execution: a claim older than this is
	// adopted (its execution died with its supervisor). It exceeds
	// MaxStopDeadline plus delivery and receipt margins; runLong cancels a
	// handler at handlerBudget.
	maxClaimAge = 5 * time.Minute
	// handlerBudget bounds a handler's context below maxClaimAge, so a
	// live handler's claim is never old enough for a duplicate to adopt.
	handlerBudget = maxClaimAge - time.Minute
)

// ExecuteLong runs h at most once per operation_id (I12), like Execute,
// but for handlers that wait on the world: the stop wait and the recover
// launch run outside any ledger transaction, so a waiting stop never
// wedges other mutating intents past busy_timeout. It claims the
// operation in one short transaction, runs the handler with no
// transaction attached (its Mutate calls commit on their own), then
// stores the result in a second short transaction.
//
// A duplicate of a live claim polls for the winner's stored Result and
// replays it byte-identically (block-then-replay, the Task 1 join
// semantics); a duplicate of a stale claim adopts it and executes, so a
// repeat after a supervisor restart proceeds instead of hanging. A reused
// operation_id with different arguments, and a stale expected_revision at
// claim time, return revision_conflict without running h. A transient
// handler failure releases the claim for a retry; a stored *Error replays
// like any result. Concurrent same-operation finishers converge: the
// first stores, the rest replay what is stored.
//
// A handler's own Mutate calls commit independently, so unlike Execute a
// long failure can leave earlier writes behind (a stop_requested wait
// that never reports still stands, recorded). Long handlers are written
// for that: same-operation repeats re-enter idempotently.
func ExecuteLong(ctx context.Context, h Handler, peer Peer, in Intent) (Result, error) {
	if err := in.Validate(); err != nil {
		return Result{}, newError(CodeInvalidContract, "%v", err)
	}
	db, err := ledgerFrom(ctx)
	if err != nil {
		return Result{}, err
	}
	digest := intentDigest(in)
	for {
		claim, err := readClaim(ctx, db, in.OperationID)
		if err != nil {
			return Result{}, err
		}
		switch {
		case claim.state == "done":
			return replayClaim(claim, digest)
		case claim.state == "claimed" && claim.digest != digest:
			return Result{}, newError(CodeRevisionConflict, "operation_id %q was used with different arguments", in.OperationID)
		case claim.state == "claimed" && claimFresh(claim.at):
			res, done, err := pollClaim(ctx, db, h, peer, in, digest)
			if done {
				return res, err
			}
		case claim.state == "claimed":
			if adopted, err := adoptClaim(ctx, db, in.OperationID, claim.at); err != nil {
				return Result{}, err
			} else if adopted {
				return runLong(ctx, db, h, peer, in, digest)
			}
		default:
			claimed, err := insertClaim(ctx, db, in, digest)
			if err != nil {
				return Result{}, err
			}
			if claimed {
				return runLong(ctx, db, h, peer, in, digest)
			}
		}
	}
}

// longClaim is one operations row as the long path reads it.
type longClaim struct {
	state  string
	digest string
	result string
	at     string
}

// readClaim returns the operation's row, or the zero claim when no row
// exists. A database failure is persistence_unavailable.
func readClaim(ctx context.Context, db *sql.DB, operationID string) (longClaim, error) {
	var c longClaim
	var result sql.NullString
	err := db.QueryRowContext(ctx, `SELECT state, digest, result, claimed_at FROM operations WHERE operation_id = ?`,
		operationID).Scan(&c.state, &c.digest, &result, &c.at)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return longClaim{}, nil
	case err != nil:
		return longClaim{}, newError(CodePersistenceUnavailable, "reading operation: %v", err)
	}
	c.result = result.String
	return c, nil
}

// replayClaim returns a done row's stored result, or revision_conflict
// when the id was reused with different arguments.
func replayClaim(c longClaim, digest string) (Result, error) {
	if c.digest != digest {
		return Result{}, newError(CodeRevisionConflict, "operation_id was used with different arguments")
	}
	var res Result
	if err := json.Unmarshal([]byte(c.result), &res); err != nil {
		return Result{}, newError(CodePersistenceUnavailable, "decoding stored result: %v", err)
	}
	return replay(res)
}

// claimFresh reports whether a claim is too recent to adopt. An
// unparsable timestamp adopts: it heals to the adopter's time.
func claimFresh(at string) bool {
	claimed, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return false
	}
	return time.Since(claimed) <= maxClaimAge
}

// insertClaim revision-checks and claims a new operation. It reports
// false when a concurrent claimant won the insert, so the caller
// re-reads and polls; every other failure is returned. A checkRevision
// refusal is revision_conflict.
func insertClaim(ctx context.Context, db *sql.DB, in Intent, digest string) (bool, error) {
	claimed := false
	err := Mutate(ctx, db, func(tx *sql.Tx) error {
		if err := checkRevision(ctx, tx, in); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO operations (operation_id, method, object, digest, state, claimed_at)
			VALUES (?, ?, ?, ?, 'claimed', ?) ON CONFLICT (operation_id) DO NOTHING`, in.OperationID, in.Method, in.Object, digest, now())
		if err != nil {
			return newError(CodePersistenceUnavailable, "claiming operation: %v", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return newError(CodePersistenceUnavailable, "claiming operation: %v", err)
		}
		claimed = n == 1
		return nil
	})
	return claimed, err
}

// adoptClaim steals a stale claim with a compare-and-swap on its
// timestamp, so concurrent adopters converge on one winner.
func adoptClaim(ctx context.Context, db *sql.DB, operationID, at string) (bool, error) {
	res, err := db.ExecContext(ctx, `UPDATE operations SET claimed_at = ? WHERE operation_id = ? AND claimed_at = ?`,
		now(), operationID, at)
	if err != nil {
		return false, newError(CodePersistenceUnavailable, "adopting operation: %v", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, newError(CodePersistenceUnavailable, "adopting operation: %v", err)
	}
	return n == 1, nil
}

// pollClaim waits for a live claim to resolve: done replays, a released
// claim re-loops to a fresh claim, a stale claim is adopted, and ctx
// cancellation ends the wait. It reports done=false to re-read and
// re-enter the claim loop.
func pollClaim(ctx context.Context, db *sql.DB, h Handler, peer Peer, in Intent, digest string) (Result, bool, error) {
	for {
		claim, err := readClaim(ctx, db, in.OperationID)
		if err != nil {
			return Result{}, true, err
		}
		switch {
		case claim.state == "done":
			res, err := replayClaim(claim, digest)
			return res, true, err
		case claim.state == "":
			return Result{}, false, nil
		case claim.digest != digest:
			return Result{}, true, newError(CodeRevisionConflict, "operation_id %q was used with different arguments", in.OperationID)
		case !claimFresh(claim.at):
			if adopted, err := adoptClaim(ctx, db, in.OperationID, claim.at); err != nil {
				return Result{}, true, err
			} else if adopted {
				res, err := runLong(ctx, db, h, peer, in, digest)
				return res, true, err
			}
		}
		timer := time.NewTimer(longPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return Result{}, true, ctx.Err()
		case <-timer.C:
		}
	}
}

// runLong runs h with no transaction attached, then stores its result. A
// *Error from h is a result to store; any other error is transient and
// releases the claim. A panic releases the claim too, then propagates,
// so a retry runs at once instead of polling a stuck claim.
func runLong(ctx context.Context, db *sql.DB, h Handler, peer Peer, in Intent, digest string) (Result, error) {
	defer func() {
		if p := recover(); p != nil {
			releaseClaim(ctx, db, in.OperationID)
			panic(p)
		}
	}()
	hctx, cancel := context.WithTimeout(ctx, handlerBudget)
	defer cancel()
	res, herr := h(hctx, peer, in)
	var ce *Error
	switch {
	case herr == nil:
	case errors.As(herr, &ce):
		res = Result{Error: ce}
	default:
		releaseClaim(ctx, db, in.OperationID)
		return Result{}, herr
	}
	return finishLong(ctx, db, in, digest, res)
}

// finishLong stores res as the operation's done result. Concurrent
// same-operation finishers converge: the UPDATE lands for exactly one of
// them, and the rest replay what is stored.
func finishLong(ctx context.Context, db *sql.DB, in Intent, digest string, res Result) (Result, error) {
	res.OperationID = in.OperationID
	raw, err := json.Marshal(res)
	if err != nil {
		releaseClaim(ctx, db, in.OperationID)
		return Result{}, newError(CodeInvalidContract, "encoding result: %v", err)
	}
	stored, err := storeResult(ctx, db, in.OperationID, res.Revision, string(raw))
	if err != nil {
		return Result{}, err
	}
	if stored {
		return replay(res)
	}
	claim, err := readClaim(ctx, db, in.OperationID)
	if err != nil {
		return Result{}, err
	}
	if claim.state == "done" {
		return replayClaim(claim, digest)
	}
	return Result{}, newError(CodePersistenceUnavailable, "the operation's claim was lost before its result was stored")
}

// storeResult flips one claimed operation to done with its result,
// reporting whether this caller stored it. It is the long path's single
// compare-and-swap: the UPDATE lands only on a claimed row, so the first
// finisher stores and every later one replays.
func storeResult(ctx context.Context, db *sql.DB, operationID string, revision int64, result string) (bool, error) {
	res, err := db.ExecContext(ctx, `UPDATE operations SET state = 'done', result = ?, finished_at = ?, revision = ?
		WHERE operation_id = ? AND state = 'claimed'`, result, now(), revision, operationID)
	if err != nil {
		return false, newError(CodePersistenceUnavailable, "recording result: %v", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, newError(CodePersistenceUnavailable, "recording result: %v", err)
	}
	return n == 1, nil
}

// releaseClaim frees a claim after a transient failure, so the
// operation_id stays retryable. It never touches a done row, and a lost
// race heals through stale adoption, so its error is safe to drop.
func releaseClaim(ctx context.Context, db *sql.DB, operationID string) {
	_, _ = db.ExecContext(ctx, `DELETE FROM operations WHERE operation_id = ? AND state = 'claimed'`, operationID)
}
