package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type txKey struct{}

// Mutate runs fn in one sole-writer transaction: it commits only if fn
// returns nil, and a failing fn leaves nothing behind (v2 §5.1). fn must
// not perform network I/O. A busy, locked or failing database returns
// persistence_unavailable; fn's own error is returned unchanged. Inside an
// Execute handler the context carries Execute's transaction, so Mutate joins
// it and the handler's writes commit with the operation's result.
func Mutate(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return fn(tx)
	}
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

// execute runs one intent in a single transaction: the stored result of a
// repeat, a refusal, or the handler's writes together with the operation's
// result row. The write lock taken at BEGIN serialises concurrent duplicates,
// so the loser finds the winner's committed result.
func execute(ctx context.Context, db *sql.DB, h Handler, peer Peer, in Intent, digest string) (Result, error) {
	var out Result
	err := Mutate(ctx, db, func(tx *sql.Tx) error {
		stored, err := lookup(ctx, tx, in, digest)
		if err != nil {
			return err
		}
		if stored != nil {
			out = *stored
			return nil
		}
		if err := checkRevision(ctx, tx, in); err != nil {
			return err
		}
		res, err := runHandler(ctx, tx, h, peer, in)
		if err != nil {
			return err
		}
		res.OperationID = in.OperationID
		raw, err := json.Marshal(res)
		if err != nil {
			return newError(CodeInvalidContract, "encoding result: %v", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO operations (operation_id, method, object, digest, state, revision, result, claimed_at, finished_at)
			VALUES (?,?,?,?, 'done', ?,?,?,?)`, in.OperationID, in.Method, in.Object, digest, res.Revision, string(raw), now(), now()); err != nil {
			return newError(CodePersistenceUnavailable, "recording result: %v", err)
		}
		out = res
		return nil
	})
	return out, err
}

// lookup returns the stored result of in.OperationID, nil if it is new, and
// revision_conflict if the id was used with different arguments.
func lookup(ctx context.Context, tx *sql.Tx, in Intent, digest string) (*Result, error) {
	var gotDigest, result string
	err := tx.QueryRowContext(ctx, `SELECT digest, result FROM operations WHERE operation_id = ?`, in.OperationID).Scan(&gotDigest, &result)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, newError(CodePersistenceUnavailable, "reading operation: %v", err)
	case gotDigest != digest:
		return nil, newError(CodeRevisionConflict, "operation_id %q was used with different arguments", in.OperationID)
	}
	var res Result
	if err := json.Unmarshal([]byte(result), &res); err != nil {
		return nil, newError(CodePersistenceUnavailable, "decoding stored result: %v", err)
	}
	return &res, nil
}

// checkRevision requires the intent's expected_revision to be the object's
// latest recorded revision.
func checkRevision(ctx context.Context, tx *sql.Tx, in Intent) error {
	if in.Object == "" {
		return nil
	}
	var current int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision), 0) FROM operations WHERE object = ?`, in.Object).Scan(&current); err != nil {
		return newError(CodePersistenceUnavailable, "reading object revision: %v", err)
	}
	if in.ExpectedRevision != current {
		return newError(CodeRevisionConflict, "object %q is at revision %d, the intent expected %d", in.Object, current, in.ExpectedRevision)
	}
	return nil
}

// runHandler runs h with the transaction attached to its context. A
// *Error from h is a result to store: the handler's writes are rolled back
// to a savepoint and the failure is recorded. Any other error is transient
// and aborts the whole transaction, leaving nothing behind.
func runHandler(ctx context.Context, tx *sql.Tx, h Handler, peer Peer, in Intent) (Result, error) {
	if _, err := tx.ExecContext(ctx, `SAVEPOINT handler`); err != nil {
		return Result{}, newError(CodePersistenceUnavailable, "starting handler scope: %v", err)
	}
	res, herr := h(context.WithValue(ctx, txKey{}, tx), peer, in)
	var ce *Error
	switch {
	case herr == nil:
	case errors.As(herr, &ce):
		if _, err := tx.ExecContext(ctx, `ROLLBACK TO handler`); err != nil {
			return Result{}, newError(CodePersistenceUnavailable, "undoing the failed handler: %v", err)
		}
		res = Result{Error: ce}
	default:
		return Result{}, herr
	}
	if _, err := tx.ExecContext(ctx, `RELEASE handler`); err != nil {
		return Result{}, newError(CodePersistenceUnavailable, "closing handler scope: %v", err)
	}
	return res, nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
