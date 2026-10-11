# Protected Acceptance — Design

> Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0 (resolved through `docs/spec/README.md`); § numbers, I-IDs, G-IDs and AT-IDs refer to it. Grounded against origin/main 31cb991 on 2026-10-11. Scope: single spec, `scope auto-confirmed (non-interactive)`; why it is not split is in §9.

## 1. Current state

Every line is a fact read on origin/main 31cb991.

**Verification today**

- A run reaches `verifying` from a native success. `conclude` journals `verification.started`, then either skips checks (`--no-checks`, or `inspect`) as `NOT RUN` or runs them: `internal/supervisor/pipeline.go:1049` (`conclude`), `:1105` (start), `:1115-1147` (skip path), `:1148-1204` (run path).
- The skip path journals `verification.completed` with `result: "NOT RUN"` and a `reason` (`pipeline.go:1129-1132`), projects a `verifications` row with `result = "not_run"` and **no reason column** (`internal/journal/projections.go:67` `VerificationRow`; `internal/journal/migrations/0001_init.sql` `verifications`), and ends `ready_for_review` / `unverified` (`pipeline.go:1147`).
- The run path calls `integration.RunChecksWithPolicy` (`internal/integration/verify.go:79`). It sets `Result = "failed"` whenever any check status is not `passed`, including `unavailable` (`verify.go:137-142`), and a hard error from it ends the run `failed` / `verification_unavailable` (`pipeline.go:1164`). A passing run ends `ready_for_review` with no reason (`pipeline.go:1205`); any other result ends `failed` (`pipeline.go:1196-1203`).
- Check definitions come only from the admitted configuration: `RunChecksWithPolicy` takes `cfg admission.ProjectConfig` and never reads the candidate's `mythhelm.toml` (`verify.go:75-79`); the snapshot is checked against the admitted digest before launch (`pipeline.go:430-441`). `evaluatorOf` digests the boundary, canonical policy and every check definition (`verify.go:150`, `internal/contain/evaluator.go:30`); the digest is stored in `verification_evaluators` (`internal/journal/migrations/0006_evaluator.sql`).
- The run state machine still is the v1 one: `verifying` leaves only to `ready_for_review` or `failed` (`internal/supervisor/state.go:45`), `blocked` and `failed` are terminal, and `ready_for_review` takes only the reasons empty or `unverified` (`state.go:103`). `internal/v2contract/lifecycle.go` holds the v2 table but states that a later spec migrates the supervisor onto it.
- Nothing binds a verdict to what it judged. `grep -rn "v2contract\.Verification\|v2contract\.PolicyVersion" --include='*.go' internal adapters cmd | grep -v "_test.go\|^internal/v2contract/"` finds nothing: the canonical attestation and policy records have no producer. The only writer of a `TaskRevision` is the legacy import (`internal/migrate/import.go:118`, `AcceptanceContractDigest: "sha256:" + run.taskSHA256` at `:142`).
- The apply gate never reads a verification: `grep -n "LatestVerification" internal/supervisor/apply.go` finds nothing, and the unverified check is `run.Reason == "unverified"` (`apply.go:143`). The TUI already compares the verification's candidate with the frozen one for display (`internal/tui/mission.go:17`, `:243-254`); the gate that can refuse does not.
- `importRuns` skips any run that already has a `task_revisions` row as imported (`internal/migrate/apply.go:573`; `checkImportMarker`, `import.go:306`). A new run that wrote `task_revisions` rows would be silently skipped by a later `migrate --apply` (D2).

**Receipts and surfaces today**

- `BuildReceipt` (`internal/supervisor/receipt.go:60`) renders `verification` as `{config_sha256, baseline, checks, evaluator}` (`receipt.go:220`, `:268`): no label, no reason, no binding. `receiptExit` maps `ready_for_review`/`unverified` and the two `verification_*` reasons to exit 5 (`receipt.go:372`).
- `TestReceiptHasAllSection9Keys` pins the 19 top-level receipt keys (`internal/supervisor/receipt_test.go:72`). Receipt unknowns print `unknown` with no reason (`receipt.go:204-215`, `:325`).
- The entitlement sentence exists only in the capability record (`adapters/claudecode/probe.go:137`). Summaries print `billing: <mode> (qualified: <bool>)` (`internal/cli/render.go:295`), the accessible line prints `paid continuation: <x>` (`internal/cli/accessible.go:318`), the TUI strip prints `qualified:`/`paid continuation:` (`internal/tui/mission.go:302`), and plain `review` prints no billing at all (`internal/cli/review.go:104`). `ResolveBilling` sets `EntitlementSource` `user_declared+native_status(...)`, `Qualified: false`, `G05: "not-passed"` for declared, and `registry:live-qualified`, `Qualified: true`, `G05: "passed"`, `PaidContinuation: "prevented"` for strict (`internal/admission/billing.go:45`, `:62`); the fake route sets `Mode: "local-scripted"`, `EntitlementSource: "not-applicable"` (`adapters/fake/fake.go:155`).
- `NOT RUN` renders in three places with the reason hard-coded to `(waived by --no-checks)`: `internal/cli/render.go:231`, `internal/cli/accessible.go:298`, `internal/tui/mission.go:268`. A waived run's plain `review` prints `Checks: none` with no label or reason (`review.go:122-133`); a checkless verified run cannot exist, since admission refuses an empty check list (`internal/admission/trust.go:74-76`).
- The accessible view tests `result == "NOT RUN"` (`accessible.go:297`) but the projected row holds `not_run` (`internal/tui/viewmodel/viewmodel.go:93`; the pipeline writes `Result: "not_run"`, `pipeline.go:1139`), so that branch never fires and the view prints `verification: not_run (candidate ...)`. The renderers will use `Result.Label()`.
- Streams: `run` and `recover` print every journaled event through `renderer.event` (`internal/cli/render.go:62`, `internal/cli/recover.go:51`). The documented set is a test-local list of 23 types (`internal/supervisor/pipeline_test.go:809`); the worker may also spool `attempt.native_error` (`internal/supervisor/ingest.go:42`, emitted at `internal/workers/worker.go:828`) and recovery journals `recovery.ownership_resolved` (`internal/supervisor/recover.go:158`).
- Bounds: `run_envelopes` stores the resolved ceilings (`internal/journal/ledger.go:148`, `:172`); the receipt shows `repair_ceiling` only (`receipt.go:338`). Only the execution deadline binds a live run (`internal/supervisor/envelope.go:330`; budget-ledger-s1 retrospective; issue #328 OPEN). Check timeouts are in `ProjectConfig` (`projectconfig.go:101`) and are not journaled (`receipt.go:322-324`). Evidence per check is capped at `maxEvidence = 1 << 20` (`verify.go:27`).
- Schema: `SchemaVersion = 7` (`internal/journal/journal.go:31`) with mirrors at `internal/migrate/backup.go:31` and `internal/migrate/preview/plan.go:30`; the preview's `migrationFiles` list (`internal/migrate/preview/plan.go:36-44`); version pins at `internal/journal/qualification_test.go:31,75`, `migrate_tables_test.go:123`, `evaluator_test.go:15`, `journal_test.go:99`, and backup-name pins `mythhelm.db.bak-migration-v7` at `internal/cli/migrate_test.go:274,301,311` (plus `len(decoded.Steps) != 6` at `:296`), `tests/migrate_e2e_test.go:256` and `internal/migrate/apply_test.go:142`, found with `grep -rn "bak-migration-v7\|len(decoded.Steps)\|SchemaVersion != 7\|user_version.*7" --include='*.go' .`. Migrations 0001-0007 exist (`ls internal/journal/migrations`).

**Conflicts**: `specs/*/codex-native-adapter/` edits `internal/supervisor/pipeline.go` (Task 9) and `docs/limitations.md` (Task 12); `specs/*/windows-process-ownership/` edits `internal/supervisor/pipeline.go` (Task 4), `recover.go` (Task 6) and `docs/limitations.md` (Tasks 10-11); `specs/*/claude-strict-subscription/` Task 5 adds under `tests/e2e/` and its `TestE2EDeclaredUnaffectedStrict` byte-compares declared receipts and labels, which Tasks 7 and 10 here change additively. Whichever spec lands second rebases and updates the goldens; no task here edits `internal/admission/admission.go`. Tasks rebase onto the merges and never run in parallel on a shared file.

## 2. Design by area

### 2.1 The `internal/acceptance` package (v2 §4.1, §11.2; I07, I09, I20)

A new pure package (imports only `internal/v2contract`; no I/O, no clock). It owns the vocabulary the other tasks share, so a verdict, its label and its binding are defined once.

```go
package acceptance

import (
	"time"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

const (
	PolicyID      = "P1"
	PolicyVersion = 1
)

// Result is a verification verdict as journaled in verifications.result.
type Result string

const (
	ResultPassed      Result = "passed"
	ResultFailed      Result = "failed"
	ResultNotRun      Result = "not_run"
	ResultUnavailable Result = "unavailable"
	ResultInvalid     Result = "invalid"
)

// Label is the verbatim text receipts and summaries print: "passed",
// "failed", "NOT RUN" (not_run, unavailable and the journaled payload value
// "NOT RUN"), "INVALID"; any other value is "unknown". Stream renderers call
// it on the payload's result, so a skip path's "NOT RUN" prints as NOT RUN.
func (r Result) Label() string

// Unverified reports whether the candidate is left unverified: not_run and
// unavailable.
func (r Result) Unverified() bool

// Timeouts are the per-step limits of NFR-1; see §2.9. They are values
// passed to the steps, never package state, so a test passes a short one.
type Timeouts struct{ Bind, Record, Render, Gate time.Duration }

// DefaultTimeouts is 10 s to bind and 30 s each to record, render and gate.
func DefaultTimeouts() Timeouts

// Receipt text shared with the docs (AC-4.1).
const (
	DeterminismConfinedTo = "native session"
	DeterminismNote       = "acceptance inputs (candidate, contract, check definitions, environment) are pinned; each admitted check runs once with no retry; a check's own determinism is not asserted"
)

// P1 is the canonical PolicyVersion for policy P1, "one native session, one
// writer, protected checks" (v2 §10.2): SchemaVersion 2, PolicyID "P1",
// Version 1, no Parent, ActionFamily "implement", Prompts
// {"mode":"native-session"}, Context {"source":"task-file"}, Review
// {"reviewer":"none"}, Decomposition {"tasks":"single"}, EligibleCohorts
// [], Evidence ["v2 §10.2"]. Its digest is pinned by a test: a change is a
// new PolicyVersion, never an edit (I20).
func P1() v2contract.PolicyVersion

// SHA256Ref renders a bare hex digest as "sha256:<hex>", the form every
// digest in a pin or binding takes.
func SHA256Ref(hex string) string

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

// NewPin returns the P1 pin for a task: policy P1 version 1 with its digest
// (v2contract.Digest, already "sha256:"-prefixed), task revision 1, and the
// contract digest SHA256Ref(taskSHA256) from the bare hex task digest (the
// form the legacy import records).
func NewPin(taskID, taskSHA256 string) Pin

// WithChecks adds what only an admitted check list defines. configDigest and
// evaluatorDigest are the bare hex values admission and contain.Evaluator
// carry; WithChecks stores both as SHA256Ref(...). timeouts maps check name
// to its admitted timeout.
func (p Pin) WithChecks(configDigest, evaluatorName, evaluatorDigest string, timeouts map[string]string) Pin

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
func (b Binding) Diff(want Binding) []string

// GitObjectOf derives the object format from the hex length: 40 is sha1, 64
// is sha256; anything else is an error.
func GitObjectOf(hex string) (v2contract.GitObject, error)

// EnvironmentID is "sha256:<hex>" over goos, goarch, verifier and the sorted
// names (never values) of the variables the checks ran with. Tool versions of
// the checks' own commands are not identified (honesty register).
func EnvironmentID(goos, goarch, verifier string, envNames []string) string

// Attest builds the canonical attestation of an executed verification:
// Candidate, Environment, AcceptanceContractDigest (b.Contract),
// CheckDefinition (b.CheckDefinition), CheckVersion (b.Evaluator), VerifierID,
// Results (check name to status) and Logs (one "sha256:<hex>" evidence
// identity per check that has an EvidenceSHA256; a check without one
// contributes no entry, never an empty digest). VerifierID is the
// evaluator name ("host" or the boundary name).
func Attest(verificationID, verifierID string, b Binding, results map[string]string, logs []string) v2contract.Verification

// BindingOf reads a Binding back from an attestation.
func BindingOf(v v2contract.Verification) Binding

// Decide returns the verdict of an executed verification and its reason.
// Order: a missing or different evaluator digest is invalid
// (evaluator_pin_missing, evaluator_digest_mismatch); any failed or timed_out
// check is failed (a failure is never hidden by an unavailable check); any
// unavailable or not_run check, or no check at all, is unavailable
// (checks_unavailable, no_checks_ran); otherwise passed only if every status is exactly `passed` (an unrecognised status is `unavailable`, never `passed`). statuses are
// integration.CheckResult.Status values.
func Decide(pinEvaluator, ranEvaluator string, statuses []string) (Result, string)

// DecideStatuses is Decide without the evaluator comparison: the verdict over
// check statuses alone, for the paths that have no ran evaluator (a runner
// error before the evaluator is computed, `verify.go:81-99`; recovery from
// check.completed events, which carry none). It orders statuses exactly as
// Decide does.
func DecideStatuses(statuses []string) (Result, string)
```

`Decide` never returns `passed` for an empty check list (empty universal: a zero-check run must not read as verified).

### 2.2 Admission-time pin (v2 §4.1 `Run.pinned policy`; AC-2.1, AC-2.3; I07, I20)

- `integration.EvaluatorFor(opts RunOptions, checks []admission.CheckConfig) (contain.Evaluator, error)` is `evaluatorOf` with the policy normalisation `RunChecksWithPolicy` applies (`ReadOnly = true`, no auth binds) extracted so the pin and the ran digest are computed by one function (D12). `integration.MaxEvidenceBytes = 1 << 20` replaces the unexported `maxEvidence`.
- `func checkOptions(d admission.Decision) integration.RunOptions` returns the options the verification stage uses (restricted policy when `Profile.Contained`, none for trusted-host); admission-time pinning and `conclude` both call it.
- The admission payload becomes an exported struct, so the production path and test fixtures build it one way:

```go
type AdmissionPayload struct {
	admission.Record
	AcceptancePin acceptance.Pin `json:"acceptance_pin"`
}

// NewAdmissionPayload builds d.Record() plus the pin: acceptance.NewPin(d.TaskID,
// d.Task.SHA256), with WithChecks(d.ConfigDigest, ev.Name, ev.Digest, timeouts)
// when checks will run (not --no-checks, not inspect). A failure of
// EvaluatorFor is returned.
func NewAdmissionPayload(d admission.Decision) (AdmissionPayload, error)

// ReadPin returns the run's pin; (zero, false, nil) when the admission predates
// the pin or the run has no admission.decided event (for example a run blocked
// with active_run_exists); an error only for a read or decode failure.
func ReadPin(ctx context.Context, j *journal.Journal, runID string) (acceptance.Pin, bool, error)
```

  Existing readers decode into `admission.Record` and ignore the extra key (`internal/tui/viewmodel/viewmodel.go:215`; `BuildReceipt`, `receipt.go:75`). `appendAdmission` journals `NewAdmissionPayload(d)`. `conclude` always takes the pin from `ReadPin`, never from memory, so a recovering supervisor and a launching one read the same journaled value.
- The same admission transaction runs `journal.EnsurePolicy(ctx, tx, acceptance.P1())`. Two runs leave one row.
- Failure: an error from `NewAdmissionPayload` or `EnsurePolicy` rolls the admission back and blocks the run before any worker exists with reason `acceptance_pin_failed`, as a failed quota hold does: `errors.Join(&GateError{Reason: "acceptance_pin_failed", Err: err}, p.runTo(ctx, RunBlocked, reason))` (`pipeline.go:370`, `GateError` at `internal/supervisor/envelope.go:122`), which exits 3 (I02).
- A run records its pin whatever its route: P1 is the only policy there is to pin (v2 §10.2). Qualified P1 delivery is a P1 run whose posture is `Qualified` (D11).

### 2.3 Verification binding and verdicts (v2 §11.2; AC-1.1, AC-1.3, AC-2.2, AC-4.1; I07, I09)

In `conclude`'s run path, step 0 comes before the checks and steps 1 to 3 after `RunChecksWithPolicy`:

0. **Pin**: `ReadPin`. A run whose admission has no pin (recovered across the upgrade) takes the skip path with reason `pin_missing`: no check runs and the candidate is unverified, because nothing was fixed at admission to compare with.
1. **Bind** (timeout `Timeouts.Bind`; after the `Binding` is computed, `Hooks.BindStep` (when set) runs inside the timed region and receives it, so a test substitutes one that blocks until its context ends; both come from `Hooks`, see below): `GitObjectOf(candidate.Commit)`; `Binding{Candidate, Contract: pin.Contract, CheckDefinition: pin.CheckDefinition, Evaluator: SHA256Ref(v.Evaluator.Digest), Environment: EnvironmentID(runtime.GOOS, runtime.GOARCH, v.Evaluator.Name, envNames(p.d.Proposal.Spec.Env))}`. A bind error or expiry never overrides what the checks showed: `Decide` runs first over the statuses, and only a would-be `passed` or `unavailable` verdict becomes `unavailable` / `binding_unavailable` with no attestation; a `failed` or `invalid` verdict is kept, also without an attestation (D17).
2. **Decide**: `acceptance.Decide(pin.EvaluatorDigest, SHA256Ref(v.Evaluator.Digest), statuses)`; the pin's evaluator and the ran one are both `sha256:` forms.
3. **Record** (timeout `Timeouts.Record`, applied to the append's context) in the existing verification transaction: the `verifications` row with `Result` = the verdict string, and the binding row (§2.4): `Reason`, the canonical attestation JSON and its `v2contract.Digest`, and `OverheadMS` (the bind step's duration). `verification.completed`'s payload gains `reason` and `attestation_digest`, and its `result` is the verdict (`passed`, `failed`, `unavailable` or `invalid`; the skip paths keep `NOT RUN`), never the raw check result, so the stream prints `acceptance.Result.Label()` of it and an `invalid` or `binding_unavailable` verdict cannot stream as `verification: passed`.

`RunChecksWithPolicy` now sets `Result` to `failed` only when a check failed or timed out, `unavailable` when none failed and any was `unavailable` or `not_run`, `passed` otherwise; its callers read `Decide` for the verdict.

The run outcome is one table in `func verifyOutcome(err error, result acceptance.Result) (RunState, string)`:

| Verdict | Run state / reason | Exit |
|---|---|---|
| `passed` | `ready_for_review` / empty | 0 |
| `failed` | `failed` / `verification_failed` | 5 |
| `invalid` | `failed` / `verification_invalid` | 5 |
| `unavailable`, `not_run` | `ready_for_review` / `unverified` | 5 |
| (recovery only) `verifying` run with no verification row and a pin with non-empty `check_timeouts`, `verification.started` with `no_checks=false`, and a complete set of `check.completed` events after the last `verification.started` (one per check name in the pin's `check_timeouts`) | `DecideStatuses` over those events' statuses: `failed` if any failed or timed out, otherwise `unavailable` / `record_unavailable`; checks are never re-run (D21) | 5 |
| (recovery only) `verifying` run with no verification row or no complete set of `check.completed` events (including every pin-less or `no_checks` run) | unchanged: `ErrOwnership`, exit 6, the run stays `verifying` (a check may still be running in its own process group) | 6 |
| `RunChecksWithPolicy` error, other than cancellation (start failure, unsafe managed directory, evidence write failure) | `DecideStatuses` over the checks already recorded in the partial result plus `unavailable` for the one that errored: `failed` if any recorded check failed or timed out, otherwise `unavailable` / `checks_unavailable` (D18) | 5 |
| cancellation (`errors.Is(err, context.Canceled)`) | `failed` / `verification_unavailable` (today's behaviour) | 5 |

`receiptExit` (`receipt.go:372`) maps `verification_invalid` to 5, and so does `runExit` (`internal/cli/exit.go:101-130`, the code `run`, `recover` and the TUI return): its `RunFailed` switch gains `case "verification_invalid": return ExitVerify, "verification_invalid"`; without it the process would exit 1 while the receipt says 5. Recovery applies the same table in both places it reaches a verdict: a `verifying` run reads its row (`internal/supervisor/recover.go:113-124`; `not_run` and `unavailable` become `ready_for_review`/`unverified`, `invalid` becomes `failed`/`verification_invalid`), and a run whose surviving worker finishes enters `conclude` (`recover.go:242`), which reads the pin from the journal as above.

The skip paths keep their journaled reasons verbatim: `waived by --no-checks` and `checks_refused_under_inspect` (`pipeline.go:1118-1121`); the new codes are `checks_unavailable`, `binding_unavailable`, `pin_missing` and `record_unavailable`. They write a binding row with `Reason` and no attestation.

### 2.4 Persistence: schema 8 (v2 §5.1, I20)

`internal/journal/migrations/0008_acceptance.sql` (additive; no earlier table altered, per ADR 0016):

```sql
CREATE TABLE verification_bindings (
  verification_id TEXT PRIMARY KEY REFERENCES verifications(verification_id),
  reason TEXT NOT NULL DEFAULT '',
  attestation TEXT CHECK (attestation IS NULL OR json_valid(attestation)),
  digest TEXT,
  overhead_ms INTEGER,
  CHECK ((attestation IS NULL) = (digest IS NULL))
) STRICT;
CREATE TRIGGER verification_bindings_no_update BEFORE UPDATE ON verification_bindings
BEGIN SELECT RAISE(ABORT, 'verification_bindings is immutable'); END;
CREATE TRIGGER verification_bindings_no_delete BEFORE DELETE ON verification_bindings
BEGIN SELECT RAISE(ABORT, 'verification_bindings is immutable'); END;
CREATE TRIGGER verification_bindings_no_replace BEFORE INSERT ON verification_bindings
WHEN EXISTS (SELECT 1 FROM verification_bindings WHERE verification_id = NEW.verification_id)
BEGIN SELECT RAISE(ABORT, 'verification_bindings is immutable'); END;
CREATE TRIGGER policies_no_replace BEFORE INSERT ON policies
WHEN EXISTS (SELECT 1 FROM policies WHERE policy_id = NEW.policy_id AND version = NEW.version)
BEGIN SELECT RAISE(ABORT, 'policies is immutable'); END;
CREATE TRIGGER policies_no_update BEFORE UPDATE ON policies
BEGIN SELECT RAISE(ABORT, 'policies is immutable'); END;
CREATE TRIGGER policies_no_delete BEFORE DELETE ON policies
BEGIN SELECT RAISE(ABORT, 'policies is immutable'); END;
```

The BEFORE INSERT triggers close `INSERT OR REPLACE`, which fires no delete trigger without `recursive_triggers` (`internal/journal/journal.go:39` does not set it). A row exists for every verification a schema-8 build records, so its absence means "recorded before binding existed". Journal surface:

```go
// VerificationRow gains these fields.
type VerificationRow struct {
	// ... existing fields ...
	Bound             bool   // a verification_bindings row exists
	Reason            string // why the verdict is NOT RUN, INVALID or not passed; "" when none recorded
	Attestation       []byte // canonical v2contract.Verification JSON; nil when no verifier ran
	AttestationDigest string
	OverheadMS        *int64 // nil when unmeasured
}

var ErrPolicyConflict = errors.New("journal: policy version stored with different content")

// EnsurePolicy inserts p, or returns nil when an identical row (same
// v2contract.Digest) exists (read first, then insert, never INSERT OR IGNORE); a row with the same id and version and another
// digest returns ErrPolicyConflict and changes nothing.
func EnsurePolicy(ctx context.Context, tx *sql.Tx, p v2contract.PolicyVersion) error
```

`InsertVerification` always writes the binding row (an all-empty one for a verdict with no reason, attestation or overhead), so `Bound == false` means only "recorded before schema 8"; `LatestVerification` left-joins it. `SchemaVersion` becomes 8 and every mirror and pin moves with it: `internal/migrate/backup.go:31`, `internal/migrate/preview/plan.go:30` and its `migrationFiles` list, and every test pin listed in §1 (the backup-name pins derive the name from `journal.SchemaVersion` so the next schema bump cannot break them silently). No `tasks` or `task_revisions` row is written for a new run (D2).

### 2.5 Apply and review gate (v2 §11.3; AC-1.2, AC-2.2; I07, I08, I20)

```go
type AcceptanceState string

const (
	AcceptanceBound      AcceptanceState = "bound"      // passed, and every binding dimension matches
	AcceptanceUnverified AcceptanceState = "unverified" // NOT RUN or unavailable
	AcceptanceStale      AcceptanceState = "stale"      // passed, but a bound dimension changed
	AcceptanceUnbound    AcceptanceState = "unbound"    // verification recorded before binding existed
	AcceptanceRejected   AcceptanceState = "rejected"   // failed or invalid
)

type AcceptanceStatus struct {
	State   AcceptanceState
	Reason  string   // row reason, or what is missing
	Changed []string // Binding.Diff names, for stale
}

// CheckAcceptance compares the latest verification's attestation with the
// run's current candidate, task file, admitted config digest and pinned
// evaluator, under Timeouts.Gate. A read or decode failure is an error, never
// bound (I02).
func CheckAcceptance(ctx context.Context, j *journal.Journal, runID string) (AcceptanceStatus, error) // CheckAcceptanceWithin with DefaultTimeouts().Gate

func CheckAcceptanceWithin(ctx context.Context, j *journal.Journal, runID string, limit time.Duration) (AcceptanceStatus, error)
```

The current values are: the candidate row's commit (`j.Candidate`), `SHA256Ref` of the SHA-256 of `runs/<id>/task.md` (a missing file is a mismatch on `contract`), and the pin's check definition and evaluator (`ReadPin`). The attested values come from the immutable binding row, and the pin from the append-only journal, so a drift is detectable where the changeable inputs are the candidate row and the task file; a verification whose attestation carries another check definition or evaluator than the pin reports `check_definition` or `evaluator`. A run with no verification row is `AcceptanceUnverified` with reason `no verification recorded`. The environment is not compared here (D4). Precedence, first match wins: a `failed` or `invalid` row is `AcceptanceRejected`; a `not_run` or `unavailable` row is `AcceptanceUnverified`; a row with no binding row is `AcceptanceUnbound`; a passed row whose attestation differs from the current values is `AcceptanceStale`; otherwise, only for a `passed` row, `AcceptanceBound`; any other result string is an error (I02).

`ApplyRun` (`apply.go:143`) replaces `run.Reason == "unverified"` with `status.State != AcceptanceBound`; with `intent != nil` (a resumed apply) the gate is skipped as today. The refusal names the cause and keeps the substring `--accept-unverified`:

- unverified: `unverified candidate requires --accept-unverified (NOT RUN: <reason>)`
- stale: `verification does not apply to the candidate (changed: <dims>); re-evaluation is required; --accept-unverified applies it without verification`
- unbound: `verification was recorded before acceptance binding existed; --accept-unverified applies it without verification`

When apply proceeds over a status other than bound (the user passed `--accept-unverified`), the version-2 receipt records it: `ApplyRun` sets `acceptance.apply_override = {state, reason, changed}` in the receipt map it already rewrites (`apply.go:195-199`), creating the `acceptance` map when absent; a bound apply has no such key. The receipt's `verification.label` still describes the verdict as recorded (`passed`), and the override key says it did not apply to what was applied (AT-35).

No re-evaluation command ships (D13): a stale verdict downgrades the run to unverified and the user decides. `review` prints the same status (§2.7).

### 2.6 Receipts (v2 §17.2, §15.3; AC-2.3, AC-3.1, AC-3.2, AC-3.4, AC-4.1, AC-4.4; I09)

Additive to the v1 shape (D8); the top-level key set grows by exactly `acceptance` and `unknown_reasons`.

- `billing.entitlement_statement`: `admission.EntitlementStatement(adm.Billing)` (§2.8).
- `verification` keeps `config_sha256`, `baseline`, `checks`, `evaluator` and adds `result` (`passed|failed|not_run|unavailable|invalid|unknown`), `label` (`Result.Label()`), `reason`, and `binding`: `{candidate: {format, value}, contract, check_definition, evaluator, environment, attestation_digest}` from the attestation, or `unknown`. `baseline: "not-run"` stays: it names the baseline run, not the verdict. A run with no verification row has `result`/`label` `unknown`.
- `acceptance.policy_pin`: the pin with `evaluator: {name, digest}`, or `unknown`. `acceptance.determinism`: `{confined_to: DeterminismConfinedTo, note: DeterminismNote}`. `acceptance.overhead_ms`: the row's `OverheadMS` or `unknown`, with `overhead_covers: "binding step"`. `acceptance.bounds`: `{execution_seconds, repairs, replans, transport_retries}` from `RunEnvelope`, each as `{value, enforced}`; `check_timeouts` from the pin; `evidence_bytes_per_check: integration.MaxEvidenceBytes`; or `unknown` for an unadmitted run. `enforced` is `true` for the execution deadline and `false` for the other three while issue #328 stands (re-checked when Task 7 starts).
- `unknown_reasons`: every `unknown` leaf of the receipt maps to a reason, keyed by its normalised path (array indices replaced by `[]`). `attachUnknownReasons(r Receipt, reasons map[string]string) (missing []string)` walks the receipt, stores each registered reason and returns the paths whose leaf is `unknown` with none, so a new unknown field cannot ship unexplained: the fixtures' `missing` must be empty. The reason table is the deliverable of the receipt task and covers every leaf that can render `unknown`. It is derived from the producers in `receipt.go` (each use of the `unknown` literal, `known`, `eventValue` and `boundaryClaim`), with wildcard paths, and is exercised by fixtures that reach every producer branch: unadmitted, blocked, failed-native and passed runs, plus an unavailable-check run (`verification.checks[].exit_code|evidence|sha256`), an exhausted bucket with no reset (`billing.next_retry.at`), a restricted-profile run (`execution_bundle.boundary.coverage.*`) and a Claude native-auth run (`billing.native_auth.*`). `attachUnknownReasons` returns the unregistered paths and never fails the receipt: `BuildReceipt` renders the fallback reason `no reason recorded` for each, because a receipt is built after the terminal transition commits and must not fail on a missing label; the tests fail when any fixture falls back. the classes include cost (`billing.retail_equivalent_estimate_usd.*`), usage (`billing.tokens.*`), the candidate before freeze, the binding, the pin, the model and permission mode, and the retry schedule. `verification.baseline` is the string `not-run` (it names the baseline run that was not made), never `unknown`, so it has no entry.
- Runs recorded before schema 8 render `verification.binding`, `acceptance.policy_pin` and `acceptance.overhead_ms` as `unknown` with the reason `recorded before acceptance binding existed`.

### 2.7 Surfaces

- `review` plain: the entitlement statement verbatim as its own line, a `Verification:` line (`passed (bound to <candidate>)`, `NOT RUN (<reason>)`, `INVALID (<reason>)`, `failed`, or `unknown (<reason>)`), the `Checks:` block, and an `Acceptance:` line (a `failed` or `invalid` verdict prints `rejected (<verdict>)`) from `CheckAcceptance`: `bound`, `STALE (changed: <dims>); re-evaluation required`, `unbound (...)`, or `unverified (<reason>)`.
- `run`/`recover` plain stream: `verification: NOT RUN (<reason>)` with the payload's reason verbatim, so the waived text stays `(waived by --no-checks)`; `admissionLines` gains a line that is two spaces plus the statement verbatim (the statement already begins `entitlement: `).
- `--accessible` and the TUI: the same label and reason from `VerificationRow.Reason`, and the entitlement statement from `viewmodel.Admission.EntitlementStatement` (computed from the decoded `admission.Record.Billing` by `admission.EntitlementStatement`), set by both the snapshot decoder (`viewmodel.go:215`) and the live-event decoder (`internal/tui/motion.go:196`). The TUI task precedes the CLI task because `accessible.go` reads the viewmodel. The TUI and accessible views still decide "verified" from the candidate commit alone (`mission.go:17`); applying `CheckAcceptance` there is deferred (honesty register).
- JSONL `review` prints the receipt, so the new keys appear there unchanged.

### 2.8 Entitlement statement (AC-3.1; I09, I15)

```go
// EntitlementStatement renders the verbatim entitlement sentence for a posture.
func EntitlementStatement(b adapter.BillingPosture) string
```

- `Qualified == false` and `EntitlementSource` begins `user_declared`: `entitlement: user-declared, NOT verified by MYTHHELM; paid continuation: unknown (user declares disabled)`.
- `Qualified == true` and `G05 == "passed"`: `entitlement: verified by MYTHHELM (<EntitlementSource>); paid continuation: <PaidContinuation>`.
- `Mode == "local-scripted"`: `entitlement: not applicable (local-scripted: no inference, no network)`.
- Anything else, including an inconsistent posture: `entitlement: unknown`.

The qualified sentence renders posture fields; the consulted record's identity is not journaled with admission (honesty register).

### 2.9 Timeouts and overhead (NFR-1; §17.2)

| Step | Runs in | Timeout | On expiry |
|---|---|---|---|
| bind | `conclude`, after the checks | `Timeouts.Bind` 10 s | verification journaled `unavailable` / `binding_unavailable`, no attestation, run unverified |
| record | the verification append | `Timeouts.Record` 30 s | `conclude` returns the error and the run stays `verifying`; `recover` finds the complete `check.completed` set (the checks had returned) and applies D21 |
| render | `BuildReceipt` at its three callers (`settleRun`, recovery's repair, `writeApplyReceipt`'s `blockApply`) | `Timeouts.Render` 30 s, inside `BuildReceiptWithin` | an error as today (`building receipt`) |
| gate | `CheckAcceptance` in apply and review | `Timeouts.Gate` 30 s, inside `CheckAcceptanceWithin` | an error; apply is refused, never defaulted to bound |

The step values travel as `acceptance.Timeouts`: `Hooks` (`internal/supervisor/pipeline.go:67`) gains `Timeouts *acceptance.Timeouts` (nil means `DefaultTimeouts()`; a zero field in a non-nil value means that step's default, so a test shortening one step leaves the others at their defaults) and `BindStep func(ctx context.Context, b acceptance.Binding) error` (nil means the real bind), both used by tests that call `supervisor.Run` in process. `BuildReceiptWithin(ctx, j, runID, limit)` is `BuildReceipt` under a limit, and `BuildReceipt` calls it with the default, so the three callers share the bound. The checks' own timeouts are the admitted per-check values; this slice adds none. The recorded overhead is the bind step's duration; record and render cannot include their own duration (honesty register).

### 2.10 JSONL stream contract (AC-3.3, issue #74)

`supervisor.StreamEventTypes() []string` returns a sorted copy of the closed set: the 23 types of `documentedEvents` plus the six other types any run's journal can hold (29): `attempt.native_error` (worker, `internal/workers/spool.go:20`), `recovery.ownership_resolved` (`recover.go:158`), `attempt.recovery_decided` (`internal/control/recover.go:51`), `run.extension_granted` (`internal/supervisor/envelope.go:18`), `migration.imported` (`internal/migrate/import.go:32`) and `migration.quarantined` (`internal/migrate/apply.go:23`). `recover` replays a run's whole history (`pipeline.go:1276` reads from sequence 0), so an imported or service-recovered run streams the last four. `supervisor.IsStreamEvent(typ string) bool` tests membership.

`jsonlRenderer.event` refuses a type outside the set: it writes nothing and keeps an `*UndocumentedEventError` naming the type. `finish` (`internal/cli/run.go:322`) folds that error into the command's result before it builds `run.result`, so the exit code and `run.result.exit_code` agree: it folds only when the outcome is otherwise successful (`err == nil`), giving exit 1 (`ExitInternal`) with category `undocumented_event` (`exit.go` maps the new error type); when the outcome already carries an error (for example exit 5), that error's code stands and the undocumented-type error goes to stderr as a notice. Today `finish` returns `r.err()` only after `run.result` was written with the outcome's code (D10). The legitimate events are documented rather than filtered (D10).

The residual: an emitter that builds its event type in an expression, or passes it through a function the scan does not name, is not collected; the behavioural legs and the renderer's refusal are what catch it. The pin has three legs: the exact sorted set in a test; an AST scan (`go/parser`) of the non-test files of `internal/supervisor`, `internal/workers`, `internal/control` and `internal/migrate` that collects (a) the string value of every constant or variable named `Event*`, `ev*`, `*Event` or `*EventType` (e.g. `stoppedEvent` at `internal/control/stop.go:37`), and (b) the string literal at the event-type position of `newEvent` (4th argument), `p.append`/`pipeline.append` (2nd) and `applyEvent` (5th), asserts at least 25 types were found, and requires each to be in the set (a fixture source with an undocumented emitter, kept in the test, must be flagged); and two behavioural runs (a fake `allowance-exhausted` scenario for `attempt.native_error`, the recover fixture for `recovery.ownership_resolved`) whose every streamed line is in the set. `apply --format jsonl` prints one `apply.result` object and `review --format jsonl` one receipt; neither is a journal event stream and neither changes.

### 2.11 Tests and CI

Contributor tests need no credentials (NFR-2): fake adapter, `local-scripted`, temp state directories, real git and real SQLite. Contained-boundary cases stay Linux-only as in `specs/*/contained-execution-profiles/`; everything else runs on Linux, macOS and Windows. Tamper fixtures edit only what is mutable: the candidate row and the run's `task.md` (as `breakWorkspace` edits an attempt row, `internal/supervisor/faults_test.go:740`), and journal rows the test appends itself (a verification with another attestation, an admission payload built from `NewAdmissionPayload` with an altered pin). The journal is append-only by trigger (`0001_init.sql:26-27`); no fixture updates or deletes a journaled event. The packaged-binary flow lives in `tests/e2e/`.

### 2.12 Sequencing against prerequisites

| Prerequisite | State on origin/main | Effect on the tasks |
|---|---|---|
| MH-10 registry | shipped | none |
| MH-16 S1 | shipped | Task 7 reads `run_envelopes`; #328 re-checked at its start |
| MH-21 | shipped (D-90) | `v2contract` types, schema 7, `ImportRun` consumed as shipped |
| MH-13 | all 11 tasks merged; spec awaiting finalize; card `specced` | Tasks 3-5 consume `contain.Evaluator`/`EvaluatorDigest` as shipped; contained checks Linux-only; no task waits on its finalize |
| MH-12 | implementing; Tasks 5 and 7 held on Q1 | Task 13 is blocked-on MH-12 Task 5 (and its Q1); Task 14 is blocked-on MH-12 Task 7; both sit last and nothing depends on them |

## 3. Requirements to tasks

| Requirement | Task |
|---|---|
| AC-1.1 bind | 1, 4, 5 |
| AC-1.2 stale verdict | 1, 2, 6, 10 |
| AC-1.3 unavailable stays unverified | 3, 5 |
| AC-2.1 candidate cannot change the evaluator | 3, 4, 5 |
| AC-2.2 digest mismatch is invalid | 1, 5 |
| AC-2.3 policy pin | 1, 2, 4, 7 |
| AC-3.1 entitlement sentence | 7, 9, 10, 11 |
| AC-3.2 NOT RUN everywhere | 5, 7, 9, 10, 11 |
| AC-3.3 documented events | 8 |
| AC-3.4 unknown with reason | 7 |
| AC-4.1 determinism | 5, 7 |
| AC-4.2 credential-free flow | 11 |
| AC-4.3 qualified admission | blocked path already true, pinned by `TestE2EStrictStillBlocked` (`tests/e2e/qualification_test.go:155`); 13 (positive path and registry reasons); 14 (live) |
| AC-4.4 bounds as data | 4, 7 |
| NFR-1 | 1, 5, 6, 7 |
| AC-2.1 (continued) | 3: the check argv cannot come from the candidate |
| NFR-2 | 11 |

## 4. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | Adopt `v2contract.Verification` as the attestation and `v2contract.PolicyVersion` for P1, verbatim, stored as canonical JSON with their `v2contract.Digest`; nothing is extended (Q1). | Verification is the sole canonical attestation record (v2 §4.1). A bespoke binding struct would be a second attestation shape. The four bound items map onto its fields (`Candidate`, `AcceptanceContractDigest`, `CheckDefinition`/`CheckVersion`, `Environment`). |
| D2 | Write no `tasks`/`task_revisions` row for a new run; record the contract revision as `{task_id, revision 1, contract digest}` in the pin and attestation. | `importRuns` and `checkImportMarker` treat any `task_revisions` row as "already imported" (`apply.go:573`, `import.go:306`), so a new run's row would make `migrate --apply` silently skip it. Rejected: writing the rows and changing the migrate marker logic (reopens finalized supervisor-migration). The lifecycle migration owns the TaskRevision rows for new runs. |
| D3 | Seed one global `policies` row for P1 version 1 (idempotent, digest-checked) and carry the per-run pin in `admission.decided`. | A pin naming a policy that is stored nowhere dangles; the row is what a later policy compares against. The per-run evaluator is per repository, so it cannot live in the global row. |
| D4 | Environment identity is OS, architecture, verifier and the names of the check environment's variables; tool versions are `unknown`. It is recorded and bound, and compared only where a verdict would be reused (no reuse exists in this slice), not at apply. | Comparing at apply would invalidate every pending run when the user upgrades the binary or moves the state directory, with no input to the verdict changed. AC-1.2 lists candidate, contract and check definitions as the apply-time changes. |
| D5 | Unavailable checks end `ready_for_review` / `unverified` with a `NOT RUN` label and reason, not `failed` and not v2 §11.2's `blocked`. | The unverified representation, its exit 5 and the `--accept-unverified` gate already exist and MH-21 import preserves it (`specs/*/supervisor-migration/requirements.md` AC-7.2). The v1 machine's `blocked` is terminal and not appliable (`state.go:41-52`), so `blocked` would strand the candidate. Recorded in the honesty register and Q-5. |
| D6 | A failed or timed-out check dominates an unavailable one. | An unavailable check must never hide a failure; the verdict is the worst of the checks. |
| D7 | The apply gate decides on the binding, not the run reason; a verified run recorded before schema 8 is `unbound` and needs `--accept-unverified`. | Missing evidence is never verified (I09); new enforcement starts strict (`strict-by-default.md`). Rejected: grandfathering old verified runs, which would apply an unbound verdict. Q-7. |
| D8 | Extend the v1 receipt additively: one new `acceptance` key and one `unknown_reasons` key, `schema_version` unchanged (Q3). | `internal/v2contract` defines no receipt type (`grep -n "Receipt" internal/v2contract/*.go` finds nothing); the post-apply receipt already moves to 2 for effects (`apply.go:196`). |
| D9 | Render the run's bounds from `run_envelopes` and the pin, flagging which are enforced; add no enforcement (Q4). | MH-16 stores the fields; #328 (OPEN) says three of the four never bind. The receipt must not imply otherwise (I10). |
| D10 | Document the six types a run's journal can hold beyond the 23 (`attempt.native_error`, `recovery.ownership_resolved`, `attempt.recovery_decided`, `run.extension_granted`, `migration.imported`, `migration.quarantined`) and refuse any other undocumented type in the JSONL renderer; do not filter. | All six are real journal facts (an unrecognised native error; resolved ownership; a service recovery decision; a granted extension; an import; a quarantine) a consumer needs. Filtering would hide recovery evidence. A refusal makes a future undocumented event a loud defect rather than silent contract drift. |
| D11 | Every run records the P1 pin; "P1 admission requires a qualified route" is enforced by strict `subscription-only` admission, and declared and scripted runs stay admissible but are labelled unqualified (Q-6). | AC-3.1 keeps declared-mode runs alongside P1, so a flag that refused them would break the dogfood flow. Strict admission already blocks with the registry reason (`internal/admission/billing.go:35`; `TestE2EStrictStillBlocked`). |
| D12 | One function, `integration.EvaluatorFor`, computes the evaluator for both the admission pin and the ran verification. | Two computations could drift; a mismatch is then a real signal (G07), not a bug. |
| D13 | No re-evaluation command in this slice; a stale verdict downgrades the run to unverified and apply needs `--accept-unverified` (Q-8). | A `verify` command is new surface the requirements do not ask for. AC-1.2 requires that the prior verdict not apply, which the gate enforces. |
| D14 | Schema 8 adds one side table rather than altering `verifications`; triggers make it and `policies` immutable. | ADR 0016 freezes the v1 DDL, and CEP used a side table for the evaluator (`0006_evaluator.sql`). Immutability is I20; the supervisor-migration review noted its absence as a kept suggestion. |
| D15 | Keep the existing `NOT RUN` reason strings (`waived by --no-checks`, `checks_refused_under_inspect`) and add `checks_unavailable`. | The plain stream and its tests print the waived text today (`pipeline_test.go:426`); renaming it would break the stream contract for no gain. |
| D16 | `GitObjectOf` derives the object format from the hex length. | The candidate row stores only the commit string; 40 and 64 are the only formats Git ships. Asking Git per run adds a process for no new information. |
| D17 | A bind failure leaves a `passed` or `unavailable` verdict `unavailable`/`binding_unavailable` with no attestation, and keeps a `failed` or `invalid` verdict. | The checks ran; only the attestation could not be built. The candidate is preserved and honestly unverified, and a bind fault never hides a failure (D6). |
| D18 | Any check-runner error other than cancellation (start failure, unsafe managed directory, evidence write failure) is `unavailable`/`checks_unavailable`, not `failed`. | Today every such error ends `failed`/`verification_unavailable` (`pipeline.go:1164`), which discards the candidate's reviewability for a fault in the evaluator, not the candidate. The candidate is preserved and unverified; a cancellation stays `failed` because the user stopped the run. The partial result keeps any failure already seen (D6). Rejected: `failed` for the fault alone, which labels a candidate bad for an environment fault. |
| D19 | `conclude` reads the pin from the journal, never from memory; a pin-less admission skips the checks as `NOT RUN`/`pin_missing`. | A recovering supervisor rebuilds its decision from the journal (`recoveryDecision`, `recover.go:329`) and has no in-memory pin. Rejected: rebuilding a pin at recovery, which would pin the evaluator at verification time, after the candidate exists. |
| D24 | An unreadable pin (read or decode failure) ends the run `failed`/`acceptance_pin_unreadable` before any check runs. | The v1 machine allows `verifying` to leave only to `ready_for_review` or `failed` (`state.go:45`), and a run left `verifying` blocks every later admission with no CLI recovery path (the trap issue #76(b) describes). `failed` is fail-closed (I02) and not appliable. Rejected: leaving it `verifying`. |
| D23 | An `invalid` verdict (evaluator digest missing or different from the pin) ends `failed`/`verification_invalid`, not unverified. | v2 §11.2 and G07 want a changed evaluator reported invalid, never accepted; making it appliable with `--accept-unverified` would offer a candidate judged by an evaluator nobody pinned. The candidate is still frozen and reviewable. Rejected: `ready_for_review`/`unverified`, which D18 uses for faults that say nothing about the evaluator's integrity. |
| D22 | `verification.baseline` stays the string `not-run`, with no `unknown_reasons` entry. | It names the baseline run that was not made, which is a fact, not a missing value; AC-3.4's "missing baseline" is satisfied by that explicit label rather than `unknown`. Rejected: `unknown`, which would imply a baseline exists. |
| D21 | Recovery of a `verifying` run with no verification row records a verdict from the journaled `check.completed` events (`record_unavailable` unless one failed) only when the pin names the admitted checks and every one has one, and never re-runs the checks; otherwise (including a pin-less run, where nothing proves the checks finished) it keeps today's `ErrOwnership`. | A run in `verifying` with no row is also what a crash in the middle of the checks leaves, and a check runs in its own process group with no parent-death signal (`internal/integration/process_unix.go:10`; `recover.go:111`), so moving it out of the active states early would admit a new run beside a still-writing check (I05, I24). `check.completed` is appended only after the checks return (`pipeline.go:1168`), so a complete set proves they finished. A failed record step today blocks every later admission (`pipeline.go:61-63`, `:293-299`). Q-14. |
| D20 | An apply over a non-bound status records `acceptance.apply_override` in the version-2 receipt. | The version-1 receipt says `passed` for a run whose verdict no longer applies; the override key is the only truthful record of that (AT-35, I09). |

An ADR (`docs/decisions/0019-protected-acceptance.md`) records D1, D2, D5, D7, D8, D10, D14, D18, D19, D20, D21 and D23, because they change persistence and public contracts. Task 2 creates it (status `proposed`) in the schema change's pull request, at the next free number (0019 provisionally: 0017 and 0018 are claimed by other specs); Task 12 completes its consequences. The number is provisional: `specs/*/windows-process-ownership/` claims 0017.

## 5. Honesty register

| Spec demand | Position |
|---|---|
| v2 §11.2 / §6.1: an unverified preview is `blocked` with `verification_unavailable` | Partially met. The candidate is preserved, labelled `NOT RUN`, exit 5, never a verified badge; the state is the v1 `ready_for_review`/`unverified` (D5). Moves with the supervisor run-lifecycle migration. |
| v2 §11.2: legitimate test/spec change is a new contract revision | Partially met. A changed task file or admitted config changes the bound digest and the verdict stops applying; no `TaskRevision` row or supersession record exists for new runs (D2). |
| AT-22: removed, skipped or weakened protected checks cannot yield accepted software | Partially met. Check definitions, timeouts and the evaluator are protected and pinned; the test files and scripts a check executes are candidate content and are not protected. No protected-paths mechanism exists; follow-up card proposed. |
| AT-23: environment and cache reuse | Partially met. Environment identity is recorded and bound (D4); there is no verification cache, so no reuse path exists to guard. Any cache must key on the whole `Binding`. |
| I07: toolchain identity | Not met. The identity of the checks' own tools (compiler, test runner versions) is `unknown`, and the environment identity hashes variable names only, so a changed value (for example `PATH` pointing at another toolchain) leaves it unchanged. |
| v2 §11.2 "required coverage, risk-specific review" | Deferred. Checks are the admitted argv list; coverage and risk review are not modelled. |
| I20: immutable revisions | Partially met. `policies` and `verification_bindings` are immutable by trigger; `task_revisions` has no trigger (kept suggestion of supervisor-migration) and gets no row for new runs (D2). |
| I21 / v2 §10.3: finite envelopes | Partially met. Bounds are recorded with an `enforced` flag; only the execution deadline binds a live run. Repair, replan and transport ceilings are slice 2 with #328. |
| NFR-1: acceptance overhead measured per run | Met as reworded: the bind step's duration is recorded; record and render are bounded by their timeouts but not included, since neither can measure itself. |
| v2 §17.2: the receipt carries grants, policy and context lineage, failures and interventions | Partially met. The receipt gains the pin, the verdict binding, bounds and unknown reasons; grants, context lineage and interventions are not added here. |
| AC-3.1 "the verified entitlement record" | Partially met. The qualified sentence renders posture fields; the consulted qualification record's identity is not journaled with admission. |
| AC-4.3 / I15 / G05: qualified route | Blocked. The positive path (a qualified strict admission completing the flow) needs MH-12 Task 5 and its Q1; the live evidence needs MH-12 Task 7. The negative path holds today. |
| v2 §15.3 JSONL final result | Partially met. `run.result` carries state, reason and exit (`render.go:91`); the candidate and verification references and known gaps are in the receipt, not the final line. |
| AT-35 submitted/candidate/verified/applied/deployed | Partially met. Candidate, verified and applied are distinct on every surface; deployment does not exist. |
| Verdict binding on a different host or after a binary upgrade | Partially met. Candidate, contract, check definitions and evaluator are compared at apply; the environment is not (D4). |
| v2 §11.2: no "verified" badge when the verdict no longer applies | Partially met. The apply gate and `review` use `CheckAcceptance`; the TUI and accessible views decide "verified" from the commit alone (`internal/tui/mission.go:17`) and can still show a stale verdict as verified. Follow-up. |
| v2 §4.2: new payload contracts use `schema_version = 2` | Partially met. The new journal payload keys ride version-1 envelopes; the receipt stays version 1 (D8). |
| v2 §11.2: nondeterministic checks record seeds/repetition and a declared policy | Not met. Checks run once, with no retry; a flaky check's own nondeterminism is not detected or recorded (the receipt's determinism note says so). |
| AT-24 / v2 §11.2: a deliberately limited draft contract is visibly distinct ("draft accepted under limited checks") | Not met. `--no-checks` renders `NOT RUN (waived by --no-checks)` and needs `--accept-unverified`; there is no limited-contract draft mode or label. |
| I07 / v2 §8.1 / AT-47: evaluator out of reach of same-user code | Partially met. O3 holds against candidate content in every profile; only the restricted profile (Linux, MH-13) runs checks inside a boundary. Under trusted-host a same-user process can rewrite the state directory, and the receipt says the profile is not adversarially contained. |
| G06 / AT-18: justified revalidation after a contract change | Deferred. D13 ships no re-evaluation command; a stale verdict is only refused or overridden. |
| AC-2.1: task text and plugin messages cannot change the evaluator | Partially met. The evaluator is built from the admitted configuration only (no task or model text reaches it, pinned by Task 3 and the existing `TestTaskBytesNeverReachArgv`); there is no plugin surface in `internal/` to test. |
| AC-1.3 "no check definitions" | Reading recorded: admission refuses an empty or absent check list with exit 3 before any candidate exists (`internal/admission/trust.go:74-76`), so the only route to an unverified candidate without checks is `--no-checks`; unavailable checks and an unavailable runner are the other routes. |
| AT-05: dirty originals and stale expected state at apply | Unchanged. The dogfood apply checks stay as shipped; this slice adds the binding gate only. |
| Git execution over the agent's workspace | See MH-13. |
| #73, #76 | Deferred to slice 2; not closed or duplicated here. |

## 6. Interfaces consumed from shipped code

`journal.Journal.Append(ctx, ev Event, project func(*sql.Tx) error) error` (`internal/journal/journal.go:290`); `journal.CandidateRow{AttemptID, BaseRev, Commit, Tree, PatchSHA256, ChangedPaths, Flags, Partial}` via `Journal.Candidate(ctx, attemptID)`; `journal.EnvelopeRow` via `Journal.RunEnvelope`; `contain.EvaluatorDigest(boundary, version string, policy Policy, checkDigests []string) Evaluator` and `contain.Evaluator{Name, Digest}` (digest is bare hex); `v2contract.Digest(v any) string` (returns `sha256:<hex>`, `""` when unencodable); `v2contract.Decode[T Validator](data []byte) (T, error)` (strict); `v2contract.Verification.Validate` requires verification id, candidate format and value, verifier id and check definition; `ids.New(prefix string) string`.

## 7. Failure cases

A `ReadPin` read or decode failure in `conclude` ends the run `failed` / `acceptance_pin_unreadable` before any check runs (D24; the candidate stays frozen and reviewable; `runExit` maps the reason to `ExitInternal`), never a default (I02). `EnsurePolicy` returns `ErrPolicyConflict`. `GitObjectOf` returns an error for a length other than 40 or 64 or a non-hex byte. `CheckAcceptance` returns the underlying error for a journal read or attestation decode failure and never `bound`. `attachUnknownReasons` returns the unregistered paths, and `BuildReceipt` renders `no reason recorded` for each without failing. `jsonlRenderer.event` keeps an error naming an undocumented type. `ApplyRun` returns `ErrApplyBlocked` wrapped with the cause for stale, unbound and unverified.

## 8. Security and hygiene

No secret reaches the attestation: environment identity uses variable names only, logs are evidence digests, and evidence paths stay in the check rows as today. Docs and tests use synthetic repositories and `example.com`; no real transcript is copied.

## 9. Why one spec of 14 tasks

The tasks share one contract: the pin, the binding row and the verdicts (Tasks 1-5) feed the apply gate (6), the receipt (7) and every surface (9, 10). The stream contract (8) is independent in content but edits the same renderer and pipeline tests, and the card absorbs #74 with the other receipt gaps, so a one-task spec would add a lifecycle for no independent value. Nothing is usable before Task 5. Tasks 13 and 14 are blocked-on tasks of another spec and are last, so the 12 buildable tasks ship without them. 14 is not above the epic threshold of `/spec-scope` rule 2; the count is above 12, hence this paragraph.
