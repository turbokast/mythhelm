package journal

import (
	"context"
	"database/sql"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

// AppendCanonicalTx joins a caller-owned transaction without committing it.
// The existing append path owns validation, phase, sequence and projection.
func AppendCanonicalTx(ctx context.Context, tx *sql.Tx, envelope v2contract.Envelope, project func(*sql.Tx) error) error {
	ev := Event(envelope)
	if envelope.SchemaVersion != v2contract.SchemaVersion {
		code := v2contract.CodeInvalidContract
		if envelope.SchemaVersion > v2contract.SchemaVersion {
			code = v2contract.CodeCapabilityUnsupported
		}
		return v2Error(code, ev, "use canonical schema 2", ErrInvalidEvent)
	}
	if tx == nil {
		return v2Error(v2contract.CodeInvalidContract, ev, "supply a transaction", ErrInvalidEvent)
	}
	return appendTx(ctx, tx, ev, project)
}
