package supervisor

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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
	return p.recordSkip(ctx, candidate.Commit, reason)
}

// recordSkip records the NOT RUN verification of commit and settles the run
// ready_for_review/unverified. Recovery reaches it for a skip whose record
// step failed.
func (p *pipeline) recordSkip(ctx context.Context, commit, reason string) error {
	at, id := time.Now().UTC(), ids.New("ver")
	row := journal.VerificationRow{ID: id, RunID: p.d.RunID, CandidateCommit: commit,
		Result: string(acceptance.ResultNotRun), Reason: reason, StartedAt: at, FinishedAt: at}
	payload := map[string]any{"verification_id": id, "candidate_commit": commit, "result": "NOT RUN",
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
		// The cancelled context could not commit the transition, leaving the
		// run verifying with no one to settle it.
		settle, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.timeouts().Record)
		defer cancel()
		state, reason := verifyOutcome(runErr, acceptance.ResultFailed)
		return p.runTo(settle, state, reason)
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
		// The checks it left without a result are journaled too, though not
		// recorded with the verification, so a failed record step leaves
		// recovery a complete set to settle from (design §2.9, D21).
		for i, check := range p.d.ProjectConfig.Checks[len(v.Checks):] {
			result := integration.CheckResult{Name: check.Name, Status: "not_run"}
			if i == 0 {
				result.Status = "unavailable"
			}
			if err := p.append(ctx, "check.completed", result); err != nil {
				return err
			}
		}
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

// AcceptanceState is how the latest verification relates to the run as it
// stands now (design §2.5).
type AcceptanceState string

// The states, in the order CheckAcceptance decides them.
const (
	AcceptanceBound      AcceptanceState = "bound"
	AcceptanceUnverified AcceptanceState = "unverified"
	AcceptanceStale      AcceptanceState = "stale"
	AcceptanceUnbound    AcceptanceState = "unbound"
	AcceptanceRejected   AcceptanceState = "rejected"
)

// AcceptanceStatus is the gate's answer. Changed names the binding
// dimensions of a stale verdict.
type AcceptanceStatus struct {
	State   AcceptanceState
	Reason  string
	Changed []string
}

// CheckAcceptance is CheckAcceptanceWithin under the default gate limit.
func CheckAcceptance(ctx context.Context, j *journal.Journal, runID string) (AcceptanceStatus, error) {
	return CheckAcceptanceWithin(ctx, j, runID, acceptance.DefaultTimeouts().Gate)
}

// CheckAcceptanceWithin compares the run's latest verification with what the
// run is now, under limit: the candidate row, the task file and the pinned
// check definition and evaluator against the attestation (design §2.5). A
// read, decode or timeout failure is an error and never bound (I02).
func CheckAcceptanceWithin(ctx context.Context, j *journal.Journal, runID string, limit time.Duration) (AcceptanceStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	status, err := checkAcceptance(ctx, j, runID)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return AcceptanceStatus{}, fmt.Errorf("checking acceptance of %s: %w", runID, err)
	}
	return status, nil
}

func checkAcceptance(ctx context.Context, j *journal.Journal, runID string) (AcceptanceStatus, error) {
	v, err := j.LatestVerification(ctx, runID)
	if errors.Is(err, journal.ErrNotFound) {
		return AcceptanceStatus{State: AcceptanceUnverified, Reason: "no verification recorded"}, nil
	}
	if err != nil {
		return AcceptanceStatus{}, err
	}
	result := acceptance.Result(v.Result)
	switch {
	case result == acceptance.ResultFailed || result == acceptance.ResultInvalid:
		return AcceptanceStatus{State: AcceptanceRejected, Reason: reasonOr(v.Reason, v.Result)}, nil
	case result.Unverified():
		return AcceptanceStatus{State: AcceptanceUnverified, Reason: reasonOr(v.Reason, "no reason recorded")}, nil
	case !v.Bound:
		return AcceptanceStatus{State: AcceptanceUnbound, Reason: "verification recorded before acceptance binding existed"}, nil
	case result != acceptance.ResultPassed:
		return AcceptanceStatus{}, fmt.Errorf("verification %s has unrecognised result %q", v.ID, v.Result)
	}
	attested, err := attestedBinding(v)
	if err != nil {
		return AcceptanceStatus{}, err
	}
	current, err := currentBinding(ctx, j, runID)
	if err != nil {
		return AcceptanceStatus{}, err
	}
	if changed := attested.Diff(current); len(changed) > 0 {
		return AcceptanceStatus{State: AcceptanceStale, Reason: "a bound dimension changed since the verdict", Changed: changed}, nil
	}
	return AcceptanceStatus{State: AcceptanceBound}, nil
}

func reasonOr(reason, fallback string) string {
	if reason == "" {
		return fallback
	}
	return reason
}

// attestedBinding decodes what a passed verification was bound to. A passed
// verdict always carries an attestation whose digest is the stored one;
// anything else is unreadable evidence, not a verdict to apply (I02).
func attestedBinding(v journal.VerificationRow) (acceptance.Binding, error) {
	if len(v.Attestation) == 0 {
		return acceptance.Binding{}, fmt.Errorf("passed verification %s has no attestation", v.ID)
	}
	att, err := v2contract.Decode[v2contract.Verification](v.Attestation)
	if err != nil {
		return acceptance.Binding{}, fmt.Errorf("decoding the attestation of verification %s: %w", v.ID, err)
	}
	if v2contract.Digest(att) != v.AttestationDigest {
		return acceptance.Binding{}, fmt.Errorf("attestation of verification %s does not match its stored digest", v.ID)
	}
	return acceptance.BindingOf(att), nil
}

// currentBinding is what a verdict would have to be bound to now: the
// candidate row, the task file on disk and the check definition and
// evaluator pinned at admission. The environment stays empty, so it is not
// compared (D4). A missing task file leaves the contract empty, which differs
// from any attested one; any other read failure is an error.
func currentBinding(ctx context.Context, j *journal.Journal, runID string) (acceptance.Binding, error) {
	attempt, err := j.LatestAttempt(ctx, runID)
	if err != nil {
		return acceptance.Binding{}, err
	}
	candidate, err := j.Candidate(ctx, attempt.AttemptID)
	if err != nil {
		return acceptance.Binding{}, err
	}
	object, err := acceptance.GitObjectOf(candidate.Commit)
	if err != nil {
		return acceptance.Binding{}, fmt.Errorf("candidate of %s: %w", runID, err)
	}
	pin, _, err := ReadPin(ctx, j, runID)
	if err != nil {
		return acceptance.Binding{}, err
	}
	contract, err := taskDigest(filepath.Join(j.StateDir(), "runs", runID, taskFile))
	if err != nil {
		return acceptance.Binding{}, err
	}
	return acceptance.Binding{Candidate: object, Contract: contract, CheckDefinition: pin.CheckDefinition, Evaluator: pin.EvaluatorDigest}, nil
}

// taskDigest is the "sha256:<hex>" identity of the task file, read up to the
// admission size limit plus one byte so a grown file cannot match. A missing
// file gives "".
func taskDigest(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- run ID from the journal under the private state directory
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, copyErr := io.Copy(h, io.LimitReader(f, admission.MaxTaskBytes+1))
	if err := errors.Join(copyErr, f.Close()); err != nil {
		return "", fmt.Errorf("reading the task file: %w", err)
	}
	return acceptance.SHA256Ref(hex.EncodeToString(h.Sum(nil))), nil
}
