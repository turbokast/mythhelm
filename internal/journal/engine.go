package journal

import (
	"context"
	"database/sql"
	"fmt"
)

// SQLiteVersion reports the SQLite engine version linked into this binary
// (SELECT sqlite_version()): the OQ-8 evidence for the shipped WAL-reset
// fix floor (supervisor-migration design §8). It opens an in-memory
// database, touching no state directory.
func SQLiteVersion(ctx context.Context) (string, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return "", fmt.Errorf("opening an in-memory database for the engine version: %w", err)
	}
	defer func() { _ = db.Close() }()
	var version string
	if err := db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&version); err != nil {
		return "", fmt.Errorf("reading sqlite_version(): %w", err)
	}
	return version, nil
}
