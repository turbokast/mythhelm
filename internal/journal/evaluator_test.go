package journal

import (
	"testing"
	"time"
)

func TestVerificationRowCarriesEvaluator(t *testing.T) {
	ctx := t.Context()
	j, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	if SchemaVersion != 6 {
		t.Fatalf("SchemaVersion = %d, want 6", SchemaVersion)
	}
	var table string
	err = j.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'verification_evaluators'`).Scan(&table)
	if err != nil {
		t.Fatalf("verification_evaluators missing after migration 0006: %v", err)
	}

	now := time.Now().UTC()
	if _, err := j.db.ExecContext(ctx, `INSERT INTO runs (run_id, state, reason, adapter_id, source_repo, source_branch, base_rev, task_sha256, billing_posture, execution_profile, created_at, updated_at)
		VALUES ('run_1', 'verifying', '', 'builtin/fake', '/r', 'main', 'abc', 'sha', 'local-scripted', 'restricted', ?, ?)`, formatTime(now), formatTime(now)); err != nil {
		t.Fatalf("runs table shape changed, adjust the fixture: %v", err)
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	row := VerificationRow{ID: "ver_1", RunID: "run_1", CandidateCommit: "c", Result: "passed", StartedAt: now, FinishedAt: now,
		EvaluatorName: "mythhelm-restricted", EvaluatorDigest: "abc123"}
	if err := InsertVerification(ctx, tx, row); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	got, err := j.LatestVerification(ctx, "run_1")
	if err != nil {
		t.Fatal(err)
	}
	if got.EvaluatorName != "mythhelm-restricted" || got.EvaluatorDigest != "abc123" {
		t.Fatalf("row = %+v, want the evaluator it was written with", got)
	}
}

func TestVerificationWithoutEvaluatorReadsUnknown(t *testing.T) {
	ctx := t.Context()
	j, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	now := time.Now().UTC()
	if _, err := j.db.ExecContext(ctx, `INSERT INTO runs (run_id, state, reason, adapter_id, source_repo, source_branch, base_rev, task_sha256, billing_posture, execution_profile, created_at, updated_at)
		VALUES ('run_1', 'verifying', '', 'builtin/fake', '/r', 'main', 'abc', 'sha', 'local-scripted', 'trusted-host', ?, ?)`, formatTime(now), formatTime(now)); err != nil {
		t.Fatalf("runs table shape changed, adjust the fixture: %v", err)
	}
	tx, err := j.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := InsertVerification(ctx, tx, VerificationRow{ID: "ver_1", RunID: "run_1", CandidateCommit: "c", Result: "passed", StartedAt: now, FinishedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	got, err := j.LatestVerification(ctx, "run_1")
	if err != nil || got.EvaluatorName != "" || got.EvaluatorDigest != "" {
		t.Fatalf("row = %+v, %v; want an unrecorded evaluator to read as empty, never a claimed one", got, err)
	}
}
