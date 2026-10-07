package journal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// InsertQualificationRecord stores revision for keyHash, superseding the
// previous current row for the key inside its own transaction. Old revisions
// are immutable (I20): the insert touches only the previous row's
// superseded_at metadata. Empty keyHash, digest or recordJSON, or a
// revision below 1, is refused without writing anything.
func (j *Journal) InsertQualificationRecord(ctx context.Context, keyHash string, revision int, digest, recordJSON string) error {
	if !validQualificationInput(keyHash, revision, digest, recordJSON) {
		return errors.New("journal: invalid qualification record")
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("journal: starting qualification insert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := formatTime(time.Now())
	if _, err := tx.ExecContext(ctx, `UPDATE qualification_records SET superseded_at = ? WHERE key_hash = ? AND superseded_at IS NULL`,
		now, keyHash); err != nil {
		return fmt.Errorf("journal: superseding qualification record: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO qualification_records (key_hash, revision, digest, record_json, recorded_at) VALUES (?, ?, ?, ?, ?)`,
		keyHash, revision, digest, recordJSON, now); err != nil {
		return fmt.Errorf("journal: inserting qualification record: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("journal: committing qualification record: %w", err)
	}
	return nil
}

// CurrentQualificationRecord returns the current record JSON and revision
// for keyHash, or ErrNotFound wrapped as "journal: qualification record"
// when the key has no current row (I09: absence is not a zero record).
func (j *Journal) CurrentQualificationRecord(ctx context.Context, keyHash string) (recordJSON string, revision int, err error) {
	err = j.db.QueryRowContext(ctx, `SELECT record_json, revision FROM qualification_records WHERE key_hash = ? AND superseded_at IS NULL`,
		keyHash).Scan(&recordJSON, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, fmt.Errorf("journal: qualification record: %w", ErrNotFound)
	}
	if err != nil {
		return "", 0, fmt.Errorf("journal: reading qualification record: %w", err)
	}
	return recordJSON, revision, nil
}

// ListCurrentQualificationRecords returns every current record JSON,
// ordered by key hash.
func (j *Journal) ListCurrentQualificationRecords(ctx context.Context) (records []string, err error) {
	rows, err := j.db.QueryContext(ctx, `SELECT record_json FROM qualification_records WHERE superseded_at IS NULL ORDER BY key_hash`)
	if err != nil {
		return nil, fmt.Errorf("journal: listing qualification records: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("journal: reading qualification record: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("journal: reading qualification records: %w", err)
	}
	return out, nil
}

// InsertQualificationRecordIfAbsent inserts the row only when no
// (key_hash, revision) row exists, in a single INSERT OR IGNORE statement;
// it never supersedes. It reports whether it inserted.
func (j *Journal) InsertQualificationRecordIfAbsent(ctx context.Context, keyHash string, revision int, digest, recordJSON string) (inserted bool, err error) {
	if !validQualificationInput(keyHash, revision, digest, recordJSON) {
		return false, errors.New("journal: invalid qualification record")
	}
	res, err := j.db.ExecContext(ctx, `INSERT OR IGNORE INTO qualification_records (key_hash, revision, digest, record_json, recorded_at) VALUES (?, ?, ?, ?, ?)`,
		keyHash, revision, digest, recordJSON, formatTime(time.Now()))
	if err != nil {
		return false, fmt.Errorf("journal: inserting qualification record: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("journal: inserting qualification record: %w", err)
	}
	return rows == 1, nil
}

// validQualificationInput reports whether an insert carries a usable key,
// revision, digest and payload.
func validQualificationInput(keyHash string, revision int, digest, recordJSON string) bool {
	return keyHash != "" && digest != "" && recordJSON != "" && revision >= 1
}
