package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/turbokast/mythhelm/internal/acceptance"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// maxProjectionTaskBytes preserves the admission artefact limit without a
// control-to-admission dependency. The boundary test pins their equivalence.
const maxProjectionTaskBytes = 1 << 20

// SelectAttachmentProjection requires a successful mint/validation in this
// same transaction and the trusted caller's retained owner/instance/gate. It
// selects a closed safe view, persists its revision, and returns provisional
// process memory only; the caller must commit before handing it to transport.
func SelectAttachmentProjection(ctx context.Context, tx *sql.Tx, g AttachmentGrant, stateDir string) (Projection, error) {
	s, err := journal.ReadAttachmentSource(ctx, tx, g.RunID)
	if err != nil {
		return Projection{}, attachmentFailure(CodePersistenceUnavailable)
	}
	p := Projection{SchemaVersion: 2, AttachmentID: g.AttachmentID, Generation: g.Generation, RunRevision: s.Revision, RunID: g.RunID, RunState: s.State, ReviewState: "unknown", Attention: "unknown"}
	sum := sha256.Sum256([]byte(g.RunID))
	label := "Run " + hex.EncodeToString(sum[:6])
	p.Label = &label
	p.TaskID = safeProjectionID(s.TaskID)
	p.AttemptID = safeProjectionID(s.AttemptID)
	switch s.Adapter {
	case "builtin/fake":
		harness := "fake"
		p.Harness = &harness
	case "builtin/codex":
		harness := "codex"
		p.Harness = &harness
	case "builtin/claudecode":
		harness := "claude"
		p.Harness = &harness
	}
	if s.Candidate {
		p.ReviewState = "candidate"
		p.Attention = "review"
	}
	// Actual protected verification is intentionally never inferred from run
	// state, native completion, a passed legacy row or arbitrary event payloads.
	if projectionVerified(ctx, tx, g, stateDir, s.AttemptID) {
		p.ReviewState = "verified"
		p.Attention = "review"
	}
	if s.Applied {
		p.ReviewState = "applied"
		p.Attention = "none"
	}
	switch s.State {
	case "blocked", "failed":
		p.Attention = "blocked"
	case "created", "admission", "executing", "verifying", "applying", "stopping", "recovering":
		p.Attention = "none"
	}
	if v2contract.Code(s.Reason).Valid() {
		p.WaitingReason = &s.Reason
	}
	if s.Reason == "stop_unconfirmed" {
		p.Attention = "stop_unconfirmed"
	}
	p.Stop, err = selectProjectionStop(ctx, tx, g.RunID)
	if err != nil {
		return Projection{}, attachmentFailure(CodePersistenceUnavailable)
	}
	if p.Stop != nil && p.Stop.State == "unconfirmed" {
		p.Attention = "stop_unconfirmed"
	}
	p.ProjectionRevision = 1
	if err := p.Validate(); err != nil {
		return Projection{}, attachmentFailure(CodePersistenceUnavailable)
	}
	p.ProjectionRevision = 0
	raw, err := json.Marshal(p)
	if err != nil {
		return Projection{}, attachmentFailure(CodePersistenceUnavailable)
	}
	p.ProjectionRevision, err = journal.CacheAttachmentProjection(ctx, tx, g.AttachmentID, g.Generation, p.RunRevision, raw)
	if err != nil {
		return Projection{}, attachmentFailure(CodePersistenceUnavailable)
	}
	return p, nil
}

func safeProjectionID(value string) *string {
	if value == "" || len(value) > 128 {
		return nil
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return nil
		}
	}
	return &value
}

// projectionVerified reproduces ADR0019's four required binding dimensions
// using this SQL snapshot plus the fixed private task artefact. It never calls
// a journal method opening a second transaction. Missing or malformed evidence
// remains candidate; raw attestation data and diagnostics never leave the core.
func projectionVerified(ctx context.Context, tx *sql.Tx, g AttachmentGrant, stateDir, attemptID string) bool {
	if attemptID == "" || ctx.Err() != nil {
		return false
	}
	var verificationID, commit, attestation, digest string
	err := tx.QueryRowContext(ctx, `SELECT v.verification_id,v.candidate_commit,b.attestation,b.digest FROM verifications v JOIN verification_bindings b ON b.verification_id=v.verification_id WHERE v.run_id=? AND v.result='passed' AND v.verification_id=(SELECT verification_id FROM verifications WHERE run_id=? ORDER BY finished_at DESC,verification_id DESC LIMIT 1) AND length(b.attestation)<=65536 AND length(b.digest)<=128 AND length(v.candidate_commit)<=128 AND length(v.verification_id)<=128`, g.RunID, g.RunID).Scan(&verificationID, &commit, &attestation, &digest)
	if err != nil {
		return false
	}
	att, err := v2contract.Decode[v2contract.Verification]([]byte(attestation))
	if err != nil {
		return false
	}
	canonical, err := json.Marshal(att)
	if err != nil || !bytes.Equal(canonical, []byte(attestation)) || v2contract.Digest(att) != digest || att.VerificationID != verificationID || att.Candidate.Value != commit {
		return false
	}
	var candidate string
	if err := tx.QueryRowContext(ctx, `SELECT candidate_commit FROM candidates WHERE attempt_id=? AND length(candidate_commit)<=128`, attemptID).Scan(&candidate); err != nil {
		return false
	}
	object, err := acceptance.GitObjectOf(candidate)
	if err != nil {
		return false
	}
	var rawPin string
	err = tx.QueryRowContext(ctx, `SELECT json_extract(payload,'$.acceptance_pin') FROM journal WHERE run_id=? AND type='admission.decided' AND run_sequence=(SELECT max(run_sequence) FROM journal WHERE run_id=? AND type='admission.decided') AND length(json_extract(payload,'$.acceptance_pin'))<=65536`, g.RunID, g.RunID).Scan(&rawPin)
	if err != nil {
		return false
	}
	var pin acceptance.Pin
	if err := json.Unmarshal([]byte(rawPin), &pin); err != nil {
		return false
	}
	var recordedTaskDigest string
	if err := tx.QueryRowContext(ctx, `SELECT task_sha256 FROM runs WHERE run_id=? AND length(task_sha256)=64`, g.RunID).Scan(&recordedTaskDigest); err != nil {
		return false
	}
	taskDigest := projectionTaskDigest(ctx, stateDir, g.RunID)
	if taskDigest == "" || acceptance.SHA256Ref(recordedTaskDigest) != taskDigest || pin.Contract != taskDigest || pin.CheckDefinition == "" || pin.EvaluatorDigest == "" {
		return false
	}
	current := acceptance.Binding{Candidate: object, Contract: taskDigest, CheckDefinition: pin.CheckDefinition, Evaluator: pin.EvaluatorDigest}
	return len(acceptance.BindingOf(att).Diff(current)) == 0 && ctx.Err() == nil
}

// projectionTaskDigest opens only a validated run's fixed private artefact.
// Nonblocking open plus before/after identity and regular-file checks prevents
// a FIFO/device substitution from turning a view into an unbounded stream.
// OpenRoot confines resolution; neither source_repo nor caller paths are used.
func projectionTaskDigest(ctx context.Context, stateDir, runID string) string {
	if runID == "" || len(runID) > 64 || strings.ContainsAny(runID, "-/\\") || safeProjectionID(runID) == nil || ctx.Err() != nil {
		return ""
	}
	root, err := os.OpenRoot(stateDir)
	if err != nil {
		return ""
	}
	defer func() { _ = root.Close() }()
	for _, dir := range []string{"runs", filepath.Join("runs", runID)} {
		info, err := root.Lstat(dir)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return ""
		}
	}
	path := filepath.Join("runs", runID, "task.md")
	before, err := root.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() > maxProjectionTaskBytes {
		return ""
	}
	f, err := root.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(before, info) || info.Size() > maxProjectionTaskBytes {
		return ""
	}
	sum := sha256.New()
	n, err := io.Copy(sum, io.LimitReader(f, maxProjectionTaskBytes+1))
	if err != nil || n > maxProjectionTaskBytes || ctx.Err() != nil {
		return ""
	}
	after, err := root.Lstat(path)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(info, after) || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return ""
	}
	return acceptance.SHA256Ref(hex.EncodeToString(sum.Sum(nil)))
}

func selectProjectionStop(ctx context.Context, tx *sql.Tx, runID string) (*StopResult, error) {
	var result StopResult
	var ack string
	err := tx.QueryRowContext(ctx, `SELECT s.operation_id,s.state,s.reason,o.attempt_id,p.result FROM stop_results s JOIN stop_outbox o ON o.operation_id=s.operation_id JOIN operations p ON p.operation_id=s.operation_id WHERE o.run_id=? AND length(s.operation_id)<=128 AND length(s.reason)<=128 AND length(o.attempt_id)<=128 AND length(p.result)<=4096 ORDER BY s.rowid DESC LIMIT 1`, runID).Scan(&result.OperationID, &result.State, &result.Reason, &result.AttemptID, &ack)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if safeProjectionID(result.OperationID) == nil || safeProjectionID(result.AttemptID) == nil {
		return nil, nil
	}
	result.Ack, err = DecodeParams[StopAck]([]byte(ack))
	if err != nil || result.Validate() != nil {
		return nil, nil
	}
	return &result, nil
}
