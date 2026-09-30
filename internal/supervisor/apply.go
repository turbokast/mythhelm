package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/workspace"
)

// ErrApplyBlocked is a policy refusal that leaves the run and user repository
// available for a later, correctly authorized apply.
var ErrApplyBlocked = errors.New("apply blocked")

type applyRecord struct {
	Branch          string `json:"branch"`
	TargetRepo      string `json:"target_repo"`
	CandidateCommit string `json:"candidate_commit"`
}

func applyEvent(ctx context.Context, j *journal.Journal, p *Producer, runID, typ string, record applyRecord) error {
	ev, err := newEvent(runID, "", "", typ, record, time.Now().UTC())
	if err != nil {
		return err
	}
	return p.append(ctx, j, ev, nil)
}

// ApplyRun journals the irreversible branch creation intent, reconciles a
// branch left by a crash, and writes the completed version 2 receipt.
// The caller must hold the run's owner lock throughout this call.
func ApplyRun(ctx context.Context, j *journal.Journal, runID, branch string, acceptFlags, acceptUnverified bool) (Receipt, error) {
	run, err := j.Run(ctx, runID)
	if err != nil {
		return nil, err
	}
	if err := workspace.CheckBranch(ctx, run.SourceRepo, branch); err != nil {
		return nil, err
	}
	if run.State != string(RunReadyForReview) && run.State != string(RunApplying) && run.State != string(RunCompleted) {
		return nil, fmt.Errorf("%w: run is %s, expected ready_for_review", ErrApplyBlocked, run.State)
	}
	attempt, err := j.LatestAttempt(ctx, runID)
	if err != nil {
		return nil, err
	}
	candidate, err := j.Candidate(ctx, attempt.AttemptID)
	if err != nil {
		return nil, err
	}
	record := applyRecord{Branch: branch, TargetRepo: run.SourceRepo, CandidateCommit: candidate.Commit}
	events, err := j.Events(ctx, runID, 0)
	if err != nil {
		return nil, err
	}
	var intent, completed *applyRecord
	for _, ev := range events {
		if ev.Type != "apply.intent_recorded" && ev.Type != "apply.completed" {
			continue
		}
		var m applyRecord
		if err := json.Unmarshal(ev.Payload, &m); err != nil {
			return nil, err
		}
		if ev.Type == "apply.intent_recorded" {
			intent = &m
		} else {
			completed = &m
		}
	}
	if intent != nil && *intent != record {
		return nil, fmt.Errorf("%w: prior apply intent targets another branch or commit", ErrApplyBlocked)
	}
	if run.State == string(RunApplying) && intent == nil {
		return nil, fmt.Errorf("%w: applying run has no journaled intent", ErrApplyBlocked)
	}
	if completed != nil && *completed != record {
		return nil, fmt.Errorf("%w: completed apply targets another branch or commit", ErrApplyBlocked)
	}
	current, err := workspace.BranchCommit(ctx, run.SourceRepo, branch)
	if err != nil {
		return nil, err
	}
	if current != "" && current != candidate.Commit {
		return nil, fmt.Errorf("%w: %s", workspace.ErrBranchExists, branch)
	}
	if run.State == string(RunCompleted) {
		if current != candidate.Commit {
			return nil, fmt.Errorf("%w: completed branch no longer matches candidate", ErrApplyBlocked)
		}
		r, _, err := ReadReceipt(ctx, j, runID)
		if err == nil && r["state"] == string(RunCompleted) {
			return r, nil
		}
		// A crash after the state transition but before the receipt write is
		// repaired below from the still-journaled version 1 receipt.
	}
	var flags []any
	if err := json.Unmarshal(candidate.Flags, &flags); err != nil {
		return nil, err
	}
	if len(flags) > 0 && !acceptFlags && intent == nil {
		return nil, fmt.Errorf("%w: candidate flags require --accept-flags", ErrApplyBlocked)
	}
	if run.Reason == "unverified" && !acceptUnverified && intent == nil {
		return nil, fmt.Errorf("%w: unverified candidate requires --accept-unverified", ErrApplyBlocked)
	}
	if _, err := workspace.Git(ctx, run.SourceRepo, true, "cat-file", "-e", run.BaseRev+"^{commit}"); err != nil {
		return nil, fmt.Errorf("%w: admitted base commit is unavailable in source repository: %w", ErrApplyBlocked, err)
	}
	if current == "" && run.State == string(RunCompleted) {
		return nil, fmt.Errorf("%w: completed branch is absent", ErrApplyBlocked)
	}
	r, raw, err := ReadReceipt(ctx, j, runID)
	if err != nil {
		return nil, err
	}
	if r["schema_version"] != float64(1) {
		return nil, fmt.Errorf("%w: expected version 1 receipt", ErrApplyBlocked)
	}
	runDir := filepath.Join(j.StateDir(), "runs", runID)
	if _, err := WriteReceiptBytes(runDir, "receipt.v1.json", raw); err != nil {
		return nil, err
	}
	p := NewProducer(ids.New("sup"), 1)
	if intent == nil {
		if err := applyEvent(ctx, j, p, runID, "apply.intent_recorded", record); err != nil {
			return nil, err
		}
	}
	if run.State == string(RunReadyForReview) {
		if err := TransitionRun(ctx, j, runID, RunApplying, "", p); err != nil {
			return nil, err
		}
	}
	ref := "refs/mythhelm/candidates/" + attempt.AttemptID
	if err := workspace.ApplyBranch(ctx, run.SourceRepo, attempt.WorkspacePath, ref, branch, candidate.Commit); err != nil {
		return nil, err
	}
	if completed == nil {
		if err := applyEvent(ctx, j, p, runID, "apply.completed", record); err != nil {
			return nil, err
		}
	}
	if run.State != string(RunCompleted) {
		if err := TransitionRun(ctx, j, runID, RunCompleted, "", p); err != nil {
			return nil, err
		}
	}
	r["schema_version"], r["state"], r["exit_code"] = 2, string(RunCompleted), 0
	effect := "branch_created"
	if current == candidate.Commit {
		effect = "branch_reconciled"
	}
	r["external_effects"] = []any{map[string]any{"type": effect, "target_repo": run.SourceRepo,
		"branch": branch, "commit": candidate.Commit}}
	r["remaining_human_action"] = []string{"review the branch", "sign off (DCO) after review"}
	sha, err := WriteReceipt(runDir, r)
	if err != nil {
		return nil, err
	}
	ev, err := newEvent(runID, "", "", "receipt.written", map[string]any{"schema_version": 2, "sha256": sha, "path": "receipt.json"}, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if err := p.append(ctx, j, ev, nil); err != nil {
		return nil, err
	}
	return r, nil
}
