package journal

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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

// AttachmentSource is a closed, bounded set of canonical presentation inputs.
// Raw journal/native payloads, paths, task text and provider metadata are never
// selected. Revision triggers cover every table used here (schema 9).
type AttachmentSource struct {
	Revision                                  uint64
	State, Reason, Adapter, AttemptID, TaskID string
	Candidate                                 bool
	Applied                                   bool
}

// ReadAttachmentSource selects execution inputs from the caller's authority
// transaction. Empty optional identities remain unknown. A passed native or
// legacy verification alone never establishes protected acceptance.
func ReadAttachmentSource(ctx context.Context, tx *sql.Tx, runID string) (AttachmentSource, error) {
	var s AttachmentSource
	var reason sql.NullString
	if tx == nil {
		return s, errors.New("journal: attachment source requires transaction")
	}
	if err := tx.QueryRowContext(ctx, `SELECT substr(state,1,128),substr(reason,1,128),substr(adapter_id,1,128) FROM runs WHERE run_id=?`, runID).Scan(&s.State, &reason, &s.Adapter); err != nil {
		return s, err
	}
	s.Reason = reason.String
	var err error
	s.Revision, err = RunRevision(ctx, tx, runID)
	if err != nil {
		return s, err
	}
	err = tx.QueryRowContext(ctx, `SELECT substr(attempt_id,1,256),substr(task_id,1,256) FROM attempts WHERE run_id=? ORDER BY attempt_number DESC,attempt_id DESC LIMIT 1`, runID).Scan(&s.AttemptID, &s.TaskID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return s, err
	}
	if s.AttemptID != "" {
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM candidates WHERE attempt_id=?)`, s.AttemptID).Scan(&s.Candidate); err != nil {
			return s, err
		}
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM applies a JOIN candidates c ON c.attempt_id=? AND c.candidate_commit=a.candidate_commit WHERE a.run_id=? AND a.state='applied')`, s.AttemptID, runID).Scan(&s.Applied); err != nil {
			return s, err
		}
	}
	return s, nil
}

// CacheAttachmentProjection persists only sanitised complete content. Its hash
// includes the run revision; identical content keeps the existing revision.
// The caller supplies JSON without its own projection revision to avoid a
// circular hash, then serialises that revision into the transport result.
func CacheAttachmentProjection(ctx context.Context, tx *sql.Tx, attachmentID string, generation int64, runRevision uint64, content []byte) (uint64, error) {
	if tx == nil || generation <= 0 || runRevision == 0 || len(content) > 64*1024 || !json.Valid(content) {
		return 0, errors.New("journal: invalid attachment projection")
	}
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	var previous uint64
	var old string
	err := tx.QueryRowContext(ctx, `SELECT projection_revision,content_sha256 FROM attachment_projections WHERE attachment_id=? AND generation=?`, attachmentID, generation).Scan(&previous, &old)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if err == nil && old == digest {
		return previous, nil
	}
	if previous >= math.MaxInt64 {
		return 0, errors.New("journal: projection revision exhausted")
	}
	next := previous + 1
	// The content digest excludes its own revision, but persisted content is the
	// complete sanitised projection rather than a revision-zero template.
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(content, &stored); err != nil || stored == nil {
		return 0, errors.New("journal: projection must be an object")
	}
	stored["projection_revision"], err = json.Marshal(next)
	if err != nil {
		return 0, err
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO attachment_projections(attachment_id,generation,projection_revision,source_run_revision,content_sha256,content) VALUES(?,?,?,?,?,?) ON CONFLICT(attachment_id,generation) DO UPDATE SET projection_revision=excluded.projection_revision,source_run_revision=excluded.source_run_revision,content_sha256=excluded.content_sha256,content=excluded.content`, attachmentID, generation, next, runRevision, digest, string(encoded))
	return next, err
}
