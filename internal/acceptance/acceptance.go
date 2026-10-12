// Package acceptance defines the vocabulary of protected acceptance: what a
// verification verdict is, what it is bound to and how it is labelled. It is
// pure: no I/O and no clock (v2 §11.2; I07, I09, I20).
package acceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

// PolicyID and PolicyVersion identify the P1 policy revision.
const (
	PolicyID      = "P1"
	PolicyVersion = 1
)

// Result is a verification verdict as journaled in verifications.result.
type Result string

// The verdicts a verification can have.
const (
	ResultPassed      Result = "passed"
	ResultFailed      Result = "failed"
	ResultNotRun      Result = "not_run"
	ResultUnavailable Result = "unavailable"
	ResultInvalid     Result = "invalid"
)

// notRunPayload is the value a skip path journals in verification.completed.
const notRunPayload Result = "NOT RUN"

// Label is the verbatim text receipts and summaries print: passed, failed,
// "NOT RUN" (not_run, unavailable and the journaled payload value "NOT RUN"),
// "INVALID", and "unknown" for any other value.
func (r Result) Label() string {
	switch r {
	case ResultPassed, ResultFailed:
		return string(r)
	case ResultNotRun, ResultUnavailable, notRunPayload:
		return "NOT RUN"
	case ResultInvalid:
		return "INVALID"
	default:
		return "unknown"
	}
}

// Unverified reports whether the candidate is left unverified: not_run and
// unavailable.
func (r Result) Unverified() bool {
	return r == ResultNotRun || r == ResultUnavailable
}

// Timeouts are the per-step limits of NFR-1. They are values passed to the
// steps, never package state, so a test passes a short one.
type Timeouts struct{ Bind, Record, Render, Gate time.Duration }

// DefaultTimeouts is 10 s to bind and 30 s each to record, render and gate.
func DefaultTimeouts() Timeouts {
	return Timeouts{Bind: 10 * time.Second, Record: 30 * time.Second, Render: 30 * time.Second, Gate: 30 * time.Second}
}

// Receipt text shared with the documentation.
const (
	DeterminismConfinedTo = "native session"
	DeterminismNote       = "acceptance inputs (candidate, contract, check definitions) are pinned; the check environment is recorded as a fingerprint and is not pinned, so a run recovered after a supervisor crash can bind a different one; each admitted check runs once with no retry; a check's own determinism is not asserted"
)

// P1 is the canonical PolicyVersion for policy P1, "one native session, one
// writer, protected checks" (v2 §10.2). Its digest is pinned by a test: a
// change is a new PolicyVersion, never an edit (I20).
func P1() v2contract.PolicyVersion {
	return v2contract.PolicyVersion{
		SchemaVersion:   v2contract.SchemaVersion,
		PolicyID:        PolicyID,
		Version:         PolicyVersion,
		ActionFamily:    "implement",
		Prompts:         map[string]string{"mode": "native-session"},
		Context:         map[string]string{"source": "task-file"},
		Review:          map[string]string{"reviewer": "none"},
		Decomposition:   map[string]string{"tasks": "single"},
		EligibleCohorts: []string{},
		Evidence:        []string{"v2 §10.2"},
	}
}

// SHA256Ref renders a bare hex digest as "sha256:<hex>", the form every
// digest in a pin or binding takes.
func SHA256Ref(hex string) string { return "sha256:" + hex }

// Pin is the run's acceptance pin, journaled inside admission.decided. Every
// digest in a pin and in a Binding has the form "sha256:<hex>". A field that
// admission could not know (no checks admitted, or checks refused) is empty
// and renders as unknown (I09).
type Pin struct {
	PolicyID        string            `json:"policy_id"`
	PolicyVersion   int               `json:"policy_version"`
	PolicyDigest    string            `json:"policy_digest"`
	TaskID          string            `json:"task_id"`
	TaskRevision    int               `json:"task_revision"`
	Contract        string            `json:"contract"`
	CheckDefinition string            `json:"check_definition,omitempty"`
	EvaluatorName   string            `json:"evaluator_name,omitempty"`
	EvaluatorDigest string            `json:"evaluator_digest,omitempty"`
	CheckTimeouts   map[string]string `json:"check_timeouts,omitempty"`
}

// NewPin returns the P1 pin for a task: policy P1 version 1 with its digest,
// task revision 1, and the contract digest from the bare hex task digest.
func NewPin(taskID, taskSHA256 string) Pin {
	return Pin{
		PolicyID:      PolicyID,
		PolicyVersion: PolicyVersion,
		PolicyDigest:  v2contract.Digest(P1()),
		TaskID:        taskID,
		TaskRevision:  1,
		Contract:      SHA256Ref(taskSHA256),
	}
}

// WithChecks adds what only an admitted check list defines. configDigest and
// evaluatorDigest are bare hex values; timeouts maps check name to its
// admitted timeout.
func (p Pin) WithChecks(configDigest, evaluatorName, evaluatorDigest string, timeouts map[string]string) Pin {
	p.CheckDefinition = SHA256Ref(configDigest)
	p.EvaluatorName = evaluatorName
	p.EvaluatorDigest = SHA256Ref(evaluatorDigest)
	p.CheckTimeouts = timeouts
	return p
}

// Binding is what a verdict is bound to (I07).
type Binding struct {
	Candidate       v2contract.GitObject
	Contract        string
	CheckDefinition string
	Evaluator       string
	Environment     string
}

// Diff names, in the order candidate, contract, check_definition, evaluator,
// environment, each dimension of b that differs from want. A required
// dimension (every one but environment) that is empty on either side differs:
// missing evidence never matches (I09). environment is compared only when
// want.Environment is non-empty.
func (b Binding) Diff(want Binding) []string {
	var diff []string
	if b.Candidate == (v2contract.GitObject{}) || b.Candidate != want.Candidate {
		diff = append(diff, "candidate")
	}
	for _, d := range []struct{ name, got, want string }{
		{"contract", b.Contract, want.Contract},
		{"check_definition", b.CheckDefinition, want.CheckDefinition},
		{"evaluator", b.Evaluator, want.Evaluator},
	} {
		if d.got == "" || d.got != d.want {
			diff = append(diff, d.name)
		}
	}
	if want.Environment != "" && b.Environment != want.Environment {
		diff = append(diff, "environment")
	}
	return diff
}

// GitObjectOf derives the object format from the hex length: 40 is sha1, 64
// is sha256; anything else is an error.
func GitObjectOf(value string) (v2contract.GitObject, error) {
	var format string
	switch len(value) {
	case 40:
		format = "sha1"
	case 64:
		format = "sha256"
	default:
		return v2contract.GitObject{}, fmt.Errorf("acceptance: git object id has %d characters, want 40 or 64", len(value))
	}
	if _, err := hex.DecodeString(value); err != nil {
		return v2contract.GitObject{}, fmt.Errorf("acceptance: git object id is not hex: %w", err)
	}
	return v2contract.GitObject{Format: format, Value: value}, nil
}

// EnvironmentID is "sha256:<hex>" over goos, goarch, verifier and the sorted
// names (never values) of the variables the checks ran with. Tool versions of
// the checks' own commands are not identified, so this is a partial,
// non-secret fingerprint and receipts describe it as such.
func EnvironmentID(goos, goarch, verifier string, envNames []string) string {
	h := sha256.New()
	for _, field := range append([]string{goos, goarch, verifier}, slices.Sorted(slices.Values(envNames))...) {
		h.Write([]byte(strconv.Itoa(len(field)) + ":" + field + ";"))
	}
	return SHA256Ref(hex.EncodeToString(h.Sum(nil)))
}

// Attest builds the canonical attestation of an executed verification.
// results maps check name to status; logs holds one "sha256:<hex>" evidence
// identity per check that has one. verifierID is the evaluator name.
func Attest(verificationID, verifierID string, b Binding, results map[string]string, logs []string) v2contract.Verification {
	return v2contract.Verification{
		SchemaVersion:            v2contract.SchemaVersion,
		VerificationID:           verificationID,
		Candidate:                b.Candidate,
		Environment:              b.Environment,
		AcceptanceContractDigest: b.Contract,
		CheckDefinition:          b.CheckDefinition,
		CheckVersion:             b.Evaluator,
		VerifierID:               verifierID,
		Results:                  results,
		Logs:                     logs,
	}
}

// BindingOf reads a Binding back from an attestation.
func BindingOf(v v2contract.Verification) Binding {
	return Binding{
		Candidate:       v.Candidate,
		Contract:        v.AcceptanceContractDigest,
		CheckDefinition: v.CheckDefinition,
		Evaluator:       v.CheckVersion,
		Environment:     v.Environment,
	}
}

// Decide returns the verdict of an executed verification and its reason. A
// missing or different evaluator digest is invalid (evaluator_pin_missing,
// evaluator_digest_mismatch); otherwise the verdict is DecideStatuses.
func Decide(pinEvaluator, ranEvaluator string, statuses []string) (Result, string) {
	switch {
	case pinEvaluator == "":
		return ResultInvalid, "evaluator_pin_missing"
	case pinEvaluator != ranEvaluator:
		return ResultInvalid, "evaluator_digest_mismatch"
	}
	return DecideStatuses(statuses)
}

// DecideStatuses is Decide without the evaluator comparison, for the paths
// that have no ran evaluator. Any failed or timed_out check is failed (an
// unavailable check never hides a failure); any other status but passed, or
// no check at all, is unavailable (checks_unavailable, no_checks_ran);
// otherwise passed. A zero-check run never reads as verified (I09).
func DecideStatuses(statuses []string) (Result, string) {
	if len(statuses) == 0 {
		return ResultUnavailable, "no_checks_ran"
	}
	unavailable := false
	for _, s := range statuses {
		switch s {
		case "failed", "timed_out":
			return ResultFailed, "checks_failed"
		case "passed":
		default:
			unavailable = true
		}
	}
	if unavailable {
		return ResultUnavailable, "checks_unavailable"
	}
	return ResultPassed, ""
}
