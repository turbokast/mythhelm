package journal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// RunRevision reads the canonical execution revision inside the caller's snapshot.
// Source-table triggers advance it atomically with projection changes. Reads,
// authority metadata and duplicate spool acknowledgments do not advance it.
func RunRevision(ctx context.Context, tx *sql.Tx, runID string) (uint64, error) {
	if tx == nil {
		return 0, errors.New("journal: run revision requires a transaction")
	}
	var revision uint64
	err := tx.QueryRowContext(ctx, `SELECT revision FROM run_control_revisions WHERE run_id = ?`, runID).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("journal: run %s revision: %w", runID, ErrNotFound)
	}
	if err != nil {
		return 0, fmt.Errorf("journal: reading run %s revision: %w", runID, err)
	}
	return revision, nil
}
