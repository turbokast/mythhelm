package supervisor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/turbokast/mythhelm/internal/acceptance"
	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/contain"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/integration"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// AdmissionPayload is the admission.decided payload: the admission record
// plus the run's acceptance pin (design §2.2). Decoders that predate the pin
// read the embedded record and ignore the extra key.
type AdmissionPayload struct {
	admission.Record
	AcceptancePin acceptance.Pin `json:"acceptance_pin"`
}

// checkOptions is the option set the verification stage runs the checks
// with: the restricted policy for a contained run, host authority otherwise.
// Admission-time pinning and conclude both call it, so the pinned evaluator
// is the one that runs (I07).
func checkOptions(d admission.Decision) integration.RunOptions {
	opts := integration.RunOptions{KeepGoing: d.KeepGoing}
	if d.Profile.Contained {
		opts.Policy = &contain.Policy{Profile: admission.ProfileRestricted}
	}
	return opts
}

// NewAdmissionPayload builds the record admission journals: d.Record() and the
// run's P1 pin. The check definition, evaluator and timeouts are pinned only
// when checks will run (not waived, not refused under inspect); otherwise they
// stay empty and render as unknown (I09).
func NewAdmissionPayload(d admission.Decision) (AdmissionPayload, error) {
	pin := acceptance.NewPin(d.TaskID, d.Task.SHA256)
	if !d.NoChecks && d.Profile.Name != admission.ProfileInspect {
		ev, err := integration.EvaluatorFor(checkOptions(d), d.ProjectConfig.Checks)
		if err != nil {
			return AdmissionPayload{}, fmt.Errorf("computing the evaluator to pin: %w", err)
		}
		timeouts := make(map[string]string, len(d.ProjectConfig.Checks))
		for _, c := range d.ProjectConfig.Checks {
			timeouts[c.Name] = c.Timeout
		}
		pin = pin.WithChecks(d.ConfigDigest, ev.Name, ev.Digest, timeouts)
	}
	return AdmissionPayload{Record: d.Record(), AcceptancePin: pin}, nil
}

// ReadPin returns the pin journaled with the run's admission. It reports false
// with no error when the admission predates the pin or the run has none; an
// error is only a read or decode failure, which callers never treat as a
// missing pin (I02).
func ReadPin(ctx context.Context, j *journal.Journal, runID string) (acceptance.Pin, bool, error) {
	events, err := j.Events(ctx, runID, 0)
	if err != nil {
		return acceptance.Pin{}, false, err
	}
	var pin *acceptance.Pin
	for _, ev := range events {
		if ev.Type != "admission.decided" {
			continue
		}
		var payload struct {
			AcceptancePin *acceptance.Pin `json:"acceptance_pin"`
		}
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			return acceptance.Pin{}, false, fmt.Errorf("decoding the acceptance pin of %s: %w", runID, err)
		}
		pin = payload.AcceptancePin
	}
	if pin == nil {
		return acceptance.Pin{}, false, nil
	}
	return *pin, true, nil
}

// verifyOutcome is the run outcome of a verification (design §2.3). A
// cancelled check run is the user stopping the run, so it fails whatever the
// verdict; an unrecognised verdict fails closed and never reads as passed
// (I02, I09).
func verifyOutcome(err error, result acceptance.Result) (RunState, string) {
	if errors.Is(err, context.Canceled) {
		return RunFailed, "verification_unavailable"
	}
	switch result {
	case acceptance.ResultPassed:
		return RunReadyForReview, ""
	case acceptance.ResultInvalid:
		return RunFailed, "verification_invalid"
	case acceptance.ResultUnavailable, acceptance.ResultNotRun:
		return RunReadyForReview, "unverified"
	default:
		return RunFailed, "verification_failed"
	}
}

// timeouts returns the acceptance step limits: the defaults, with every
// non-zero field of Hooks.Timeouts taking its place.
func (p *pipeline) timeouts() acceptance.Timeouts {
	t := acceptance.DefaultTimeouts()
	h := p.h.Timeouts
	if h == nil {
		return t
	}
	if h.Bind != 0 {
		t.Bind = h.Bind
	}
	if h.Record != 0 {
		t.Record = h.Record
	}
	if h.Render != 0 {
		t.Render = h.Render
	}
	if h.Gate != 0 {
		t.Gate = h.Gate
	}
	return t
}

// skipVerification journals a verification that ran no check: the candidate
// stays frozen and unverified, labelled NOT RUN with its reason (I09).
func (p *pipeline) skipVerification(ctx context.Context, reason string) error {
	candidate, err := p.j.Candidate(ctx, p.d.AttemptID)
	if err != nil {
		return errors.Join(err, p.runTo(ctx, RunFailed, "verification_unavailable"))
	}
	at, id := time.Now().UTC(), ids.New("ver")
	row := journal.VerificationRow{ID: id, RunID: p.d.RunID, CandidateCommit: candidate.Commit,
		Result: string(acceptance.ResultNotRun), Reason: reason, StartedAt: at, FinishedAt: at}
	payload := map[string]any{"verification_id": id, "candidate_commit": candidate.Commit, "result": "NOT RUN",
		"reason": reason, "baseline": "not-run"}
	if err := p.recordVerification(ctx, row, payload); err != nil {
		return err
	}
	return p.runTo(ctx, RunReadyForReview, "unverified")
}

// recordVerification journals verification.completed and projects the row
// with its binding in one transaction, under the record step's limit.
func (p *pipeline) recordVerification(ctx context.Context, row journal.VerificationRow, payload map[string]any) error {
	rctx, cancel := context.WithTimeout(ctx, p.timeouts().Record)
	defer cancel()
	ev, err := newEvent(p.d.RunID, p.d.TaskID, p.d.AttemptID, "verification.completed", payload, row.FinishedAt)
	if err != nil {
		return err
	}
	if err := p.prod.append(rctx, p.j, ev, func(tx *sql.Tx) error { return journal.InsertVerification(rctx, tx, row) }); err != nil {
		return fmt.Errorf("recording verification %s: %w", row.ID, err)
	}
	return p.flush(ctx)
}

// verifyChecks runs the admitted checks against the pin journaled at
// admission, binds the verdict to what it judged and records it (design §2.3).
func (p *pipeline) verifyChecks(ctx context.Context) error {
	pin, ok, err := ReadPin(ctx, p.j, p.d.RunID)
	if err != nil {
		return errors.Join(err, p.runTo(ctx, RunFailed, "acceptance_pin_unreadable"))
	}
	if !ok {
		return p.skipVerification(ctx, "pin_missing")
	}
	candidate, err := p.j.Candidate(ctx, p.d.AttemptID)
	if err != nil {
		return errors.Join(err, p.runTo(ctx, RunFailed, "verification_unavailable"))
	}
	// restricted checks run in the boundary with a check policy (design
	// §2.7); trusted-host keeps host checks.
	v, runErr := integration.RunChecksWithPolicy(ctx, integration.Candidate{
		Commit: candidate.Commit, Workspace: p.d.Workdir,
	}, p.d.ProjectConfig, p.d.Proposal.Spec.Env, checkOptions(p.d))
	if runErr != nil {
		p.h.Notice(fmt.Sprintf("verification could not complete: %v", runErr))
	}
	if errors.Is(runErr, context.Canceled) {
		state, reason := verifyOutcome(runErr, acceptance.ResultFailed)
		return p.runTo(ctx, state, reason)
	}
	v.ConfigSHA256 = p.d.ConfigDigest
	statuses := make([]string, 0, len(v.Checks)+1)
	for _, check := range v.Checks {
		if err := p.append(ctx, "check.completed", check); err != nil {
			return err
		}
		statuses = append(statuses, check.Status)
	}
	if runErr != nil {
		// A runner fault leaves the check it interrupted unavailable; a
		// failure already seen still wins (D6, D18).
		statuses = append(statuses, "unavailable")
	}
	verdict, reason := acceptance.DecideStatuses(statuses)
	if v.Evaluator.Digest != "" {
		verdict, reason = acceptance.Decide(pin.EvaluatorDigest, acceptance.SHA256Ref(v.Evaluator.Digest), statuses)
	}
	id := ids.New("ver")
	var attestation []byte
	var attestationDigest string
	var overhead *int64
	if runErr == nil {
		started := time.Now()
		var berr error
		attestation, attestationDigest, berr = p.bind(ctx, pin, v, candidate.Commit, id)
		ms := time.Since(started).Milliseconds()
		overhead = &ms
		if berr != nil {
			// The checks ran; only the attestation could not be built, so a
			// failure or an invalid evaluator stays and nothing else passes.
			p.h.Notice(fmt.Sprintf("verification binding unavailable: %v", berr))
			attestation, attestationDigest = nil, ""
			if verdict == acceptance.ResultPassed || verdict == acceptance.ResultUnavailable {
				verdict, reason = acceptance.ResultUnavailable, "binding_unavailable"
			}
		}
	}
	finished := v.FinishedAt
	if finished.IsZero() {
		finished = time.Now().UTC()
	}
	row := journal.VerificationRow{ID: id, RunID: p.d.RunID, CandidateCommit: v.CandidateCommit, ConfigSHA256: v.ConfigSHA256,
		Result: string(verdict), StartedAt: v.StartedAt, FinishedAt: finished, Checks: checkRows(v.Checks),
		Reason: reason, Attestation: attestation, AttestationDigest: attestationDigest, OverheadMS: overhead}
	payload := map[string]any{"verification_id": id, "candidate_commit": v.CandidateCommit, "config_sha256": v.ConfigSHA256,
		"result": string(verdict), "checks": v.Checks, "baseline": "not-run"}
	if v.Evaluator.Digest != "" {
		row.EvaluatorName, row.EvaluatorDigest = v.Evaluator.Name, v.Evaluator.Digest
		payload["evaluator"] = v.Evaluator
	}
	if reason != "" {
		payload["reason"] = reason
	}
	if attestationDigest != "" {
		payload["attestation_digest"] = attestationDigest
	}
	if err := p.recordVerification(ctx, row, payload); err != nil {
		return err
	}
	state, why := verifyOutcome(nil, verdict)
	return p.runTo(ctx, state, why)
}

// bind builds the canonical attestation of an executed verification under the
// bind step's limit: what the verdict judged, from the pin and the ran
// evaluator (I07). It returns the encoded record and its digest.
func (p *pipeline) bind(ctx context.Context, pin acceptance.Pin, v integration.Verification, commit, verificationID string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeouts().Bind)
	defer cancel()
	object, err := acceptance.GitObjectOf(commit)
	if err != nil {
		return nil, "", err
	}
	b := acceptance.Binding{
		Candidate: object, Contract: pin.Contract, CheckDefinition: pin.CheckDefinition,
		Evaluator:   acceptance.SHA256Ref(v.Evaluator.Digest),
		Environment: acceptance.EnvironmentID(runtime.GOOS, runtime.GOARCH, v.Evaluator.Name, envNames(p.d.Proposal.Spec.Env)),
	}
	if p.h.BindStep != nil {
		if err := p.h.BindStep(ctx, b); err != nil {
			return nil, "", err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	results := make(map[string]string, len(v.Checks))
	var logs []string
	for _, c := range v.Checks {
		results[c.Name] = c.Status
		if c.EvidenceSHA256 != "" {
			logs = append(logs, acceptance.SHA256Ref(c.EvidenceSHA256))
		}
	}
	record := acceptance.Attest(verificationID, v.Evaluator.Name, b, results, logs)
	encoded, err := json.Marshal(record)
	if err != nil {
		return nil, "", err
	}
	return encoded, v2contract.Digest(record), nil
}

// envNames lists the variable names of KEY=VALUE entries, never the values.
func envNames(env []string) []string {
	names := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		names = append(names, name)
	}
	return names
}

func checkRows(checks []integration.CheckResult) []journal.CheckRow {
	rows := make([]journal.CheckRow, 0, len(checks))
	for _, c := range checks {
		rows = append(rows, journal.CheckRow{Name: c.Name, Argv: c.Argv, Status: c.Status, ExitCode: c.ExitCode,
			DurationMS: c.DurationMS, EvidencePath: c.EvidencePath, EvidenceSHA256: c.EvidenceSHA256})
	}
	return rows
}
