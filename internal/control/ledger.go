package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Mutate runs fn in one sole-writer transaction: it commits only if fn
// returns nil, and a failing fn leaves nothing behind (v2 §5.1). fn must
// not perform network I/O. A busy, locked or failing database returns
// persistence_unavailable; fn's own error is returned unchanged.
func Mutate(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return newError(CodePersistenceUnavailable, "starting transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return newError(CodePersistenceUnavailable, "committing transaction: %v", err)
	}
	return nil
}

// claim reads the operation_id and, if it is absent, claims it, in one
// transaction. It returns the stored Result of a finished repeat; claimed
// reports a fresh claim; neither means another caller holds the claim.
func claim(ctx context.Context, db *sql.DB, in Intent, digest string) (stored *Result, claimed bool, err error) {
	err = Mutate(ctx, db, func(tx *sql.Tx) error {
		var gotDigest, state string
		var result sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT digest, state, result FROM operations WHERE operation_id = ?`, in.OperationID).
			Scan(&gotDigest, &state, &result)
		switch {
		case err == nil:
			if gotDigest != digest {
				return newError(CodeRevisionConflict, "operation_id %q was used with different arguments", in.OperationID)
			}
			if state != "done" {
				return nil
			}
			var res Result
			if err := json.Unmarshal([]byte(result.String), &res); err != nil {
				return newError(CodePersistenceUnavailable, "decoding stored result: %v", err)
			}
			stored = &res
			return nil
		case !errors.Is(err, sql.ErrNoRows):
			return newError(CodePersistenceUnavailable, "reading operation: %v", err)
		}

		if in.Object != "" {
			var current int64
			var inFlight int
			if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(CASE WHEN state='done' THEN revision END), 0),
				COALESCE(SUM(state='claimed'), 0) FROM operations WHERE object = ?`, in.Object).Scan(&current, &inFlight); err != nil {
				return newError(CodePersistenceUnavailable, "reading object revision: %v", err)
			}
			if inFlight > 0 {
				return newError(CodeRevisionConflict, "object %q has an operation in flight", in.Object)
			}
			if in.ExpectedRevision != current {
				return newError(CodeRevisionConflict, "object %q is at revision %d, the intent expected %d", in.Object, current, in.ExpectedRevision)
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO operations (operation_id, method, object, digest, state, claimed_at)
			VALUES (?,?,?,?, 'claimed', ?)`, in.OperationID, in.Method, in.Object, digest, now()); err != nil {
			return newError(CodePersistenceUnavailable, "claiming operation: %v", err)
		}
		claimed = true
		return nil
	})
	return stored, claimed, err
}

// record stores res as the operation's immutable result.
func record(ctx context.Context, db *sql.DB, res Result) error {
	raw, err := json.Marshal(res)
	if err != nil {
		return newError(CodeInvalidContract, "encoding result: %v", err)
	}
	return Mutate(ctx, db, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx, `UPDATE operations SET state='done', revision=?, result=?, finished_at=?
			WHERE operation_id = ? AND state = 'claimed'`, res.Revision, string(raw), now(), res.OperationID)
		if err != nil {
			return newError(CodePersistenceUnavailable, "recording result: %v", err)
		}
		if n, err := r.RowsAffected(); err != nil || n != 1 {
			return newError(CodePersistenceUnavailable, "operation %s was not claimed", res.OperationID)
		}
		return nil
	})
}

// release drops an unfinished claim so the operation_id can be retried.
func release(ctx context.Context, db *sql.DB, operationID string) {
	_ = Mutate(ctx, db, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM operations WHERE operation_id = ? AND state = 'claimed'`, operationID)
		return err
	})
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
