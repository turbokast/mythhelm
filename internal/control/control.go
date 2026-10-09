package control

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

// Code is a control error code. ControlError and the 24-code catalogue ship
// with v2-contract-vocabulary task 6, which is not merged; until it lands
// these carry the exact v2 §4.5 strings for the codes this package returns,
// and Error is replaced by *v2contract.ControlError (design §6).
type Code string

// The codes the intent server returns.
const (
	CodeInvalidContract        Code = "invalid_contract"
	CodeRevisionConflict       Code = "revision_conflict"
	CodePermissionDenied       Code = "permission_denied"
	CodeCapabilityUnsupported  Code = "capability_unsupported"
	CodePersistenceUnavailable Code = "persistence_unavailable"
	CodeAllowanceExhausted     Code = "allowance_exhausted"
)

// Error is a control failure carrying its code. It matches another Error
// with the same code under errors.Is.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return fmt.Sprintf("control: %s: %s", e.Code, e.Message) }

// Is reports whether target is an Error with e's code.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

func newError(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Intent is one control mutation: the stream-1 request envelope plus the
// method call. Its wire keys are exactly Frame's.
type Intent struct {
	v2contract.RequestEnvelope
	Method          string          `json:"method"`
	Params          json.RawMessage `json:"params,omitempty"`
	CapabilityToken string          `json:"capability_token,omitempty"`
}

// Validate enforces the envelope, a method name, and Params that, when
// present, are valid JSON.
func (i Intent) Validate() error {
	if err := i.RequestEnvelope.Validate(); err != nil {
		return err
	}
	if i.Method == "" {
		return errors.New("control: intent method is empty")
	}
	if len(i.Params) > 0 && !json.Valid(i.Params) {
		return errors.New("control: intent params are not valid JSON")
	}
	return nil
}

// Result is the durable outcome of an executed intent, replayed verbatim.
type Result struct {
	OperationID string          `json:"operation_id"`
	Revision    int64           `json:"revision"`
	Body        json.RawMessage `json:"body,omitempty"`
	Error       *Error          `json:"error,omitempty"`
}

// Handler executes one intent. A failure the caller should see and that a
// repeat must replay is an *Error; any other error is a transient failure
// that releases the operation_id for a retry.
type Handler func(ctx context.Context, peer Peer, intent Intent) (Result, error)

type ledgerKey struct{}

// WithLedger returns ctx carrying the ledger Execute claims operations in.
// The design gives Execute no ledger parameter, so the supervisor attaches
// it once per request context.
func WithLedger(ctx context.Context, db *sql.DB) context.Context {
	return context.WithValue(ctx, ledgerKey{}, db)
}

func ledgerFrom(ctx context.Context) (*sql.DB, error) {
	db, _ := ctx.Value(ledgerKey{}).(*sql.DB)
	if db == nil {
		return nil, newError(CodePersistenceUnavailable, "no ledger attached to the request context")
	}
	return db, nil
}

// Execute runs h at most once per operation_id (I12). The handler's writes
// and the operation's result commit in one transaction (Mutate joins it from
// the handler's context), so a failure before the commit leaves nothing
// behind and the operation_id can be retried. An identical repeat (same
// method, object, params, expected_revision and generation) returns the
// stored Result; a reused operation_id with different arguments, and an
// expected_revision that is not the object's current revision, return
// revision_conflict without running h. Concurrent duplicates serialise on
// the write lock: the loser returns the winner's stored Result. A malformed
// intent returns invalid_contract.
func Execute(ctx context.Context, h Handler, peer Peer, intent Intent) (Result, error) {
	if err := intent.Validate(); err != nil {
		return Result{}, newError(CodeInvalidContract, "%v", err)
	}
	db, err := ledgerFrom(ctx)
	if err != nil {
		return Result{}, err
	}
	res, err := execute(ctx, db, h, peer, intent, intentDigest(intent))
	if err != nil {
		return Result{}, err
	}
	return replay(res)
}

// replay returns a stored result; a stored failure is also the error.
func replay(res Result) (Result, error) {
	if res.Error != nil {
		return res, res.Error
	}
	return res, nil
}

func intentDigest(i Intent) string {
	var params bytes.Buffer
	if len(i.Params) > 0 {
		_ = json.Compact(&params, i.Params) // Validate has checked it
	}
	sum := sha256.New()
	generation := "-"
	if i.Generation != nil {
		generation = strconv.FormatInt(*i.Generation, 10)
	}
	for _, part := range []string{i.Method, i.Object, params.String(), strconv.FormatInt(i.ExpectedRevision, 10), generation} {
		_, _ = fmt.Fprintf(sum, "%d:%s;", len(part), part)
	}
	return hex.EncodeToString(sum.Sum(nil))
}

// MintToken mints an opaque capability token for attemptID and stores its
// digest. Minting again rotates the token. An unknown attempt returns
// invalid_contract and stores nothing.
func MintToken(ctx context.Context, db *sql.DB, attemptID string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", newError(CodePersistenceUnavailable, "reading randomness: %v", err)
	}
	token := hex.EncodeToString(raw)
	err := Mutate(ctx, db, func(tx *sql.Tx) error {
		var one int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM attempts WHERE attempt_id = ?`, attemptID).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return newError(CodeInvalidContract, "attempt %q is unknown", attemptID)
		}
		if err != nil {
			return newError(CodePersistenceUnavailable, "reading attempt: %v", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO capability_tokens (attempt_id, token_sha256, minted_at) VALUES (?,?,?)
			ON CONFLICT(attempt_id) DO UPDATE SET token_sha256=excluded.token_sha256, minted_at=excluded.minted_at`,
			attemptID, tokenDigest(token), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return newError(CodePersistenceUnavailable, "storing token digest: %v", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// CheckToken authorises token for attemptID. A token minted for another
// attempt, or never minted, returns permission_denied.
func CheckToken(ctx context.Context, db *sql.DB, token, attemptID string) error {
	var stored string
	err := db.QueryRowContext(ctx, `SELECT token_sha256 FROM capability_tokens WHERE attempt_id = ?`, attemptID).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return newError(CodePermissionDenied, "attempt %q has no capability token", attemptID)
	}
	if err != nil {
		return newError(CodePersistenceUnavailable, "reading token digest: %v", err)
	}
	if subtle.ConstantTimeCompare([]byte(stored), []byte(tokenDigest(token))) != 1 {
		return newError(CodePermissionDenied, "capability token is not valid for attempt %q", attemptID)
	}
	return nil
}

func tokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
