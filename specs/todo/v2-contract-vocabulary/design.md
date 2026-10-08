## V2 Contract Vocabulary — Design

Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0.

## 1. Current state

- No v2 contract package exists. `ls internal/` shows 14 packages (`adapter`, `admission`, `buildinfo`, `cli`, `ids`, `integration`, `journal`, `qualify`, `security`, `statedir`, `supervisor`, `tui`, `workers`, `workspace`; none for contracts); `grep -rn "TaskRevision\|task_revision" internal/ cmd/ adapters/ --include='*.go' | grep -v _test` prints nothing.
- The v1 event envelope is `journal.Event` (`internal/journal/journal.go:80-94`): `schema_version`, `event_id`, `run_id`, `task_id?`, `attempt_id?`, `producer_id`, `producer_sequence`/`run_sequence`/`generation` (all `int64`), `caused_by?`, `observed_at`, `type`, `payload` (`json.RawMessage`). Only `schema_version = 1` is accepted (`EnvelopeVersion` at `journal.go:31-32`; hard reject at `journal.go:367-368`).
- `Append` (`internal/journal/journal.go:281-345`) already implements the v1 form of AC-4.1: duplicates acked by `event_id` (lines 291-298, before the sequence check), stale generations rejected (`ErrStaleGeneration`, lines 309-312), non-contiguous sequences rejected (`ErrSequenceGap`, lines 313-316), `run_sequence` allocated as per-run MAX+1 (lines 318-322). Idempotency is by `event_id`, not by sequence: a new `event_id` reusing an old sequence is a gap, not a duplicate.
- v1 run/attempt machines (`internal/supervisor/state.go:41-96`) are smaller than v2 §6: no `planning`/`integrating` run phases, no task-revision machine at all, no `reserved`/`waiting_native`/`waiting_approval` attempt states. `ErrIllegalTransition` names from→to (state.go, `grep ErrIllegalTransition internal/supervisor/state.go`).
- The closest v2 precedent is `internal/qualify`: `KeyHash` (`qualify.go:130`), `CanonicalDigest` (`qualify.go:147`), `DecodeRecord` (`qualify.go:176`) — content-addressed records, canonical JSON, `sha256:<hex>` digests, missing→`unknown` defaults.
- IDs are opaque prefixed ULIDs from `ids.New(prefix)` (`internal/ids/ids.go:18`); prefixes are display aids, matching v2 §4.2.
- The receipt-file `schema_version = 2` (`internal/supervisor/apply.go:195`) is a separate namespace from the envelope; a contract-version check must never match a receipt (AC-1.3).
- TOML is available without a new dependency: `github.com/BurntSushi/toml v1.6.0` is already direct in `go.mod:8` (used by `internal/admission/projectconfig.go`, `internal/tui/theme/theme.go`).

## 2. Package and shared kernel (v2 §4.2; FR-1)

New package `internal/v2contract` (D1). It imports stdlib only (`encoding/json`, `errors`, `fmt`, `strings`, `time`); tests additionally use `BurntSushi/toml`. It must never import `journal` (which links the SQLite driver) or `qualify` (which imports `journal`): the hermeticity test pins this (NFR-4).

```go
// Package v2contract is the canonical v2 contract vocabulary (master spec
// v2 §4): record types, lifecycle tables, the error catalogue and the
// event-v2 envelope. Pure data and pure validators; no I/O, no time source,
// no global state.
package v2contract

// SchemaVersion is the schema_version every canonical v2 payload carries.
const SchemaVersion = 2

// Frame limits (v2 §4.3; NFR-1). Constants only: enforcement points live in
// specs 2-4, which reference these values.
const (
    MaxFrameBytes   = 1 << 20 // 1 MiB encoded frame
    MaxNestingDepth = 64
    MaxArtifactRefs = 128 // bounded artifact references per frame (D2)
)
```

Shared shapes used by several records:

```go
// GitObject carries a Git object ID with its format and full value (v2 §4.2:
// never assume SHA-1 length).
type GitObject struct {
    Format string `json:"format" toml:"format"` // e.g. "sha1", "sha256"
    Value  string `json:"value" toml:"value"`   // full hex value
}

// Budget is one finite envelope (v2 §4.1 Run/Experiment; I21). Limit "unknown"
// means unmeasured, never zero (I09).
type Budget struct {
    Name  string `json:"name" toml:"name"`
    Limit string `json:"limit" toml:"limit"`
    Unit  string `json:"unit" toml:"unit"`
}

// RevisionRef pins a dependency to an exact artifact or contract revision
// (v2 §4.1 TaskRevision).
type RevisionRef struct {
    Kind     string `json:"kind" toml:"kind"`         // "artifact" | "contract"
    ID       string `json:"id" toml:"id"`
    Revision int    `json:"revision" toml:"revision"` // >= 1
    Digest   string `json:"digest,omitempty" toml:"digest,omitempty"`
}

// Validate rejects a Kind outside {"artifact", "contract"} (naming the
// value), an empty ID, and a Revision below 1. TaskRevision.Validate
// calls it for every dependency.
func (r RevisionRef) Validate() error

// RequestEnvelope is the v2 §4.2 envelope every mutating control request
// carries: the client-chosen idempotency key, the target object identity,
// the ledger revision the client read, and the controller generation where
// applicable (absent omits it, I09). Stream 2's Intent embeds this shape;
// no consumer redefines these fields.
type RequestEnvelope struct {
    OperationID      string `json:"operation_id" toml:"operation_id"`
    Object           string `json:"object,omitempty" toml:"object,omitempty"`
    ExpectedRevision int64  `json:"expected_revision" toml:"expected_revision"`
    Generation       *int64 `json:"generation,omitempty" toml:"generation,omitempty"`
}

// Validate requires a non-empty OperationID and a non-negative
// ExpectedRevision. Object and Generation are carried opaquely: they are
// required where the owning method defines them ("where applicable").
func (r RequestEnvelope) Validate() error
```

IDs are opaque strings (v2 §4.2): validated non-empty only. No prefix check — a prefix check would make prefixes authority, which §4.2 forbids.

### 2.1 Vocabularies (v2 §4.2; AC-1.4)

Three distinct string scales, each with `Valid()` and `String()`; the words are master-spec text, duplicated here rather than imported from `qualify` (D3):

```go
type Capability string // "supported" | "unsupported" | "unknown"
type Progress string   // "planned" | "documented-candidate" | "fixture-tested" |
                       // "live-qualified" | "experimental" | "blocked" | "unsupported"
type Eligibility string // "eligible" | "blocked" | "unsupported"

func (c Capability) Valid() bool
func (p Progress) Valid() bool
func (e Eligibility) Valid() bool
```

Values travel with evidence, scope and expiry in one generic claim (§4.2; I09; I14):

```go
// ScaleValue is a vocabulary word that can validate itself.
type ScaleValue interface {
    ~string
    Valid() bool
}

// Claim binds a vocabulary value to its evidence, scope and expiry.
type Claim[T ScaleValue] struct {
    Value     T          `json:"value" toml:"value"`
    Evidence  string     `json:"evidence" toml:"evidence"`
    Scope     string     `json:"scope" toml:"scope"`
    ExpiresAt *time.Time `json:"expires_at,omitempty" toml:"expires_at,omitempty"`
}

func (c Claim[T]) Validate() error // Value in-scale (naming the value), Evidence and Scope non-empty, ExpiresAt non-nil and non-zero (v2 §4.2: every value travels with evidence, scope and expiry)
```

### 2.2 Strict decode and digest

Every record, the envelope and `ControlError` implement `Validator`; one generic `Decode` gives all of them unknown-key rejection (v2 §4.2: unknown keys in authority/configuration contracts fail validation; applied uniformly as the conservative default, D4):

```go
type Validator interface{ Validate() error }

// Decode strictly decodes canonical JSON into T: unknown fields rejected,
// then T.Validate enforced. Failure cases: syntax error (offset named),
// unknown field (field named), validation error (field named).
func Decode[T Validator](data []byte) (T, error)

// Digest returns "sha256:<hex>" over the canonical JSON encoding of v
// (encoding/json marshal of the struct; declaration order is deterministic).
// No field exclusions: no v2 record carries its own digest (D5).
func Digest(v any) string
```

## 3. Record types (v2 §4.1; FR-1, AC-1.1–AC-1.2)

Fourteen structs, one per §4.1 row except optional `Release` (deferred, honesty register). Every struct carries `SchemaVersion int` first, validated `== 2`, and snake_case `json`/`toml` tags throughout. Every `Validate()` names the offending field; every record has a golden JSON fixture (§7). `time.Time` fields use `*time.Time` with `omitempty` where absence is meaningful (I09: missing is never zero).

```go
type Run struct {
    SchemaVersion int      `json:"schema_version" toml:"schema_version"`
    RunID         string   `json:"run_id" toml:"run_id"`
    RepoID        string   `json:"repo_id" toml:"repo_id"`
    ExecutionHost string   `json:"execution_host" toml:"execution_host"`
    Goal          string   `json:"goal" toml:"goal"`
    GoalRevision  int      `json:"goal_revision" toml:"goal_revision"`
    RequestedDeliverable string `json:"requested_deliverable" toml:"requested_deliverable"`
    SourceSnapshot GitObject `json:"source_snapshot" toml:"source_snapshot"`
    PlanRevision  int      `json:"plan_revision" toml:"plan_revision"`
    PinnedPolicy  string   `json:"pinned_policy" toml:"pinned_policy"`
    GrantIDs      []string `json:"grants" toml:"grants"`
    Budgets       []Budget `json:"budgets" toml:"budgets"`
    State         RunState `json:"lifecycle" toml:"lifecycle"`
    Reasons       []string `json:"reasons" toml:"reasons"`
}
```

(`RunState` in §4. `Goal` is the versioned request/acceptance data within the run, per §4.1; `Run` is what the UI calls Mission.)

```go
// TaskRevision is immutable once dispatched (I20): a material change is a new
// Revision, never an edit. MH-22 adopts this shape verbatim. Validate runs
// RevisionRef.Validate on every dependency before accepting the task.
type TaskRevision struct {
    SchemaVersion          int           `json:"schema_version" toml:"schema_version"`
    TaskID                 string        `json:"task_id" toml:"task_id"`
    Revision               int           `json:"revision" toml:"revision"` // >= 1, increasing within TaskID
    Deliverable            string        `json:"deliverable" toml:"deliverable"`
    AcceptanceContractDigest string      `json:"acceptance_contract_digest" toml:"acceptance_contract_digest"`
    Dependencies           []RevisionRef `json:"dependencies" toml:"dependencies"`
    WriteScope             []string      `json:"write_scope" toml:"write_scope"`
    Risk                   string        `json:"risk" toml:"risk"`
    ResourceClaims         []string      `json:"resource_claims" toml:"resource_claims"`
    IntegrationDestination string        `json:"integration_destination" toml:"integration_destination"`
    State                  TaskState     `json:"lifecycle" toml:"lifecycle"`
}

type Attempt struct {
    SchemaVersion   int         `json:"schema_version" toml:"schema_version"`
    AttemptID       string      `json:"attempt_id" toml:"attempt_id"`
    TaskID          string      `json:"task_id" toml:"task_id"`
    TaskRevision    int         `json:"task_revision" toml:"task_revision"`
    Number          int         `json:"number" toml:"number"`
    RouteBundle     string      `json:"route_bundle" toml:"route_bundle"` // exact admitted bundle reference
    WorkerID        string      `json:"worker_id" toml:"worker_id"`
    LaunchID        string      `json:"launch_id" toml:"launch_id"` // one-use launch identity (v2 §6.3)
    Workspace       string      `json:"workspace" toml:"workspace"`
    NativeSessionRef string     `json:"native_session_ref" toml:"native_session_ref"`
    ContextManifestID string    `json:"context_manifest_id" toml:"context_manifest_id"`
    GrantIDs        []string    `json:"grants" toml:"grants"`
    ReservationIDs  []string    `json:"reservations" toml:"reservations"`
    State           AttemptState `json:"lifecycle" toml:"lifecycle"`
    CreatedAt       time.Time   `json:"created_at" toml:"created_at"`
    UpdatedAt       time.Time   `json:"updated_at" toml:"updated_at"`
}

type RoutingDecision struct {
    SchemaVersion        int               `json:"schema_version" toml:"schema_version"`
    DecisionID           string            `json:"decision_id" toml:"decision_id"`
    RunID                string            `json:"run_id" toml:"run_id"`
    Eligible             []string          `json:"eligible" toml:"eligible"`
    Exclusions           map[string]string `json:"exclusions" toml:"exclusions"` // route -> reason
    Features             map[string]string `json:"features" toml:"features"`     // pre-assignment features
    SelectedRoute        string            `json:"selected_route" toml:"selected_route"`
    SelectedPolicy       string            `json:"selected_policy" toml:"selected_policy"`
    Rationale            string            `json:"rationale" toml:"rationale"`
    Scores               map[string]float64 `json:"scores,omitempty" toml:"scores,omitempty"`
    Uncertainty          string            `json:"uncertainty,omitempty" toml:"uncertainty,omitempty"`
    SelectionProbability *float64          `json:"selection_probability,omitempty" toml:"selection_probability,omitempty"`
    OverrideProvenance   string            `json:"override_provenance,omitempty" toml:"override_provenance,omitempty"`
}

// DesignDisposition is proposed | accepted | superseded. Never confused with
// route ranking (v2 §4.1).
type DesignDisposition string

type DesignDecision struct {
    SchemaVersion int               `json:"schema_version" toml:"schema_version"`
    DecisionID    string            `json:"decision_id" toml:"decision_id"`
    Disposition   DesignDisposition `json:"disposition" toml:"disposition"`
    Rationale     string            `json:"rationale" toml:"rationale"`
    Alternatives  []string          `json:"alternatives" toml:"alternatives"`
    Owner         string            `json:"owner" toml:"owner"` // authorised decision owner
    Scope         string            `json:"scope" toml:"scope"`
    Sources       []string          `json:"sources" toml:"sources"`
    Revision      int               `json:"revision" toml:"revision"`
    Supersedes    *int              `json:"supersedes,omitempty" toml:"supersedes,omitempty"`
}

type Artifact struct {
    SchemaVersion  int       `json:"schema_version" toml:"schema_version"`
    ArtifactID     string    `json:"artifact_id" toml:"artifact_id"` // repository-scoped opaque ID
    RepoID         string    `json:"repo_id" toml:"repo_id"`
    SHA256         string    `json:"sha256" toml:"sha256"` // exactly 64 lowercase hex chars; Validate rejects empty, non-hex and wrong-length values
    SizeBytes      int64     `json:"size_bytes" toml:"size_bytes"`
    MediaType      string    `json:"media_type" toml:"media_type"`
    ProducerID     string    `json:"producer_id" toml:"producer_id"`
    Snapshot       GitObject `json:"snapshot" toml:"snapshot"`
    Sensitivity    string    `json:"sensitivity" toml:"sensitivity"`
    Dependencies   []string  `json:"dependencies" toml:"dependencies"`
    ValidUntil     *time.Time `json:"valid_until,omitempty" toml:"valid_until,omitempty"`
    RetentionRoots []string  `json:"retention_roots" toml:"retention_roots"`
}

// ObservationKind is controller | tool | native. Agent prose is a claim, never
// an Observation with native authority (v2 §4.1).
type ObservationKind string

type Observation struct {
    SchemaVersion int             `json:"schema_version" toml:"schema_version"`
    ObservationID string          `json:"observation_id" toml:"observation_id"`
    Kind          ObservationKind `json:"kind" toml:"kind"`
    SourceID      string          `json:"source_id" toml:"source_id"`
    SourceVersion string          `json:"source_version" toml:"source_version"`
    ObservedAt    time.Time       `json:"observed_at" toml:"observed_at"`
    ValueKind     string          `json:"value_kind" toml:"value_kind"`
    Value         string          `json:"value" toml:"value"`
    Uncertainty   string          `json:"uncertainty" toml:"uncertainty"`
}

type Verification struct {
    SchemaVersion            int               `json:"schema_version" toml:"schema_version"`
    VerificationID           string            `json:"verification_id" toml:"verification_id"`
    Candidate                GitObject         `json:"candidate" toml:"candidate"` // exact candidate/tree
    Environment              string            `json:"environment" toml:"environment"`
    AcceptanceContractDigest string            `json:"acceptance_contract_digest" toml:"acceptance_contract_digest"`
    CheckDefinition          string            `json:"check_definition" toml:"check_definition"`
    CheckVersion             string            `json:"check_version" toml:"check_version"`
    VerifierID               string            `json:"verifier_id" toml:"verifier_id"`
    Results                  map[string]string `json:"results" toml:"results"`
    Logs                     []string          `json:"logs" toml:"logs"`
}

// ContextManifest is the immutable handoff record; Handoff is its sending-use
// name (v2 §4.1 lists one "Handoff / ContextManifest" row).
type ContextManifest struct {
    SchemaVersion              int      `json:"schema_version" toml:"schema_version"`
    ManifestID                 string   `json:"manifest_id" toml:"manifest_id"`
    Requirements               []string `json:"requirements" toml:"requirements"`
    DecisionIDs                []string `json:"decisions" toml:"decisions"`
    Inputs                     []string `json:"inputs" toml:"inputs"`
    Claims                     []string `json:"claims" toml:"claims"`
    Evidence                   []string `json:"evidence" toml:"evidence"`
    Unresolved                 []string `json:"unresolved" toml:"unresolved"`
    Disclosures                []string `json:"disclosures" toml:"disclosures"`
    LastAcknowledgedRevision   int      `json:"last_acknowledged_revision" toml:"last_acknowledged_revision"`
}

type Handoff = ContextManifest

type Message struct {
    SchemaVersion int                 `json:"schema_version" toml:"schema_version"`
    MessageID     string              `json:"message_id" toml:"message_id"`
    Sender        string              `json:"sender" toml:"sender"`
    Recipient     string              `json:"recipient" toml:"recipient"`
    Kind          string              `json:"kind" toml:"kind"`
    CausedBy      string              `json:"caused_by,omitempty" toml:"caused_by,omitempty"`
    References    []string            `json:"references" toml:"references"`
    Sensitivity   string              `json:"sensitivity" toml:"sensitivity"`
    ExpiresAt     *time.Time          `json:"expires_at,omitempty" toml:"expires_at,omitempty"`
    Delivery      DeliveryDisposition `json:"delivery" toml:"delivery"`
}

// DeliveryDisposition is an open string (values grow with integrations; the
// v2 §4.3 example uses "queued_for_next_turn"), validated non-empty (D6).
type DeliveryDisposition string

// Grant is user-issued standing or one-use authority; EffectIntent is its
// single-effect-use name (v2 §4.1 lists one "Grant / EffectIntent" row).
// Unknown keys fail validation (v2 §4.2 authority contracts).
type Grant struct {
    SchemaVersion   int        `json:"schema_version" toml:"schema_version"`
    GrantID         string     `json:"grant_id" toml:"grant_id"`
    Use             string     `json:"use" toml:"use"` // "standing" | "one_use"
    Effect          string     `json:"effect" toml:"effect"` // exact bounded effect
    Target          string     `json:"target" toml:"target"`
    ArtifactID      string     `json:"artifact_id,omitempty" toml:"artifact_id,omitempty"`
    ExpectedState   string     `json:"expected_state" toml:"expected_state"`
    ExpiresAt       *time.Time `json:"expires_at,omitempty" toml:"expires_at,omitempty"`
    Revoked         bool       `json:"revoked" toml:"revoked"`
    OperationID     string     `json:"operation_id" toml:"operation_id"`
    Reconciliation  string     `json:"reconciliation,omitempty" toml:"reconciliation,omitempty"`
}

type EffectIntent = Grant

type Reservation struct {
    SchemaVersion   int        `json:"schema_version" toml:"schema_version"`
    ReservationID   string     `json:"reservation_id" toml:"reservation_id"`
    Bucket          string     `json:"bucket" toml:"bucket"` // typed resource/billing bucket
    Scope           string     `json:"scope" toml:"scope"`
    Owner           string     `json:"owner" toml:"owner"`
    Generation      int64      `json:"generation" toml:"generation"`
    Quantity        string     `json:"quantity" toml:"quantity"` // quantity or "unknown", never 0 for unknown (I09)
    Status          string     `json:"status" toml:"status"`     // open string, validated non-empty (D6)
    ExpiresAt       *time.Time `json:"expires_at,omitempty" toml:"expires_at,omitempty"`
    HeartbeatAt     *time.Time `json:"heartbeat_at,omitempty" toml:"heartbeat_at,omitempty"`
    ReleaseEvidence string     `json:"release_evidence,omitempty" toml:"release_evidence,omitempty"`
}

type PolicyVersion struct {
    SchemaVersion int               `json:"schema_version" toml:"schema_version"`
    PolicyID      string            `json:"policy_id" toml:"policy_id"`
    Version       int               `json:"version" toml:"version"` // immutable once referenced (I20)
    Parent        *int              `json:"parent,omitempty" toml:"parent,omitempty"`
    ActionFamily  string            `json:"action_family" toml:"action_family"`
    Prompts       map[string]string `json:"prompts" toml:"prompts"`
    Context       map[string]string `json:"context" toml:"context"`
    Review        map[string]string `json:"review" toml:"review"`
    Decomposition map[string]string `json:"decomposition" toml:"decomposition"`
    EligibleCohorts []string        `json:"eligible_cohorts" toml:"eligible_cohorts"`
    Evidence      []string          `json:"evidence" toml:"evidence"`
    PromotedFrom  string            `json:"promoted_from,omitempty" toml:"promoted_from,omitempty"`
    RollbackTo    string            `json:"rollback_to,omitempty" toml:"rollback_to,omitempty"`
}

type Experiment struct {
    SchemaVersion int               `json:"schema_version" toml:"schema_version"`
    ExperimentID  string            `json:"experiment_id" toml:"experiment_id"`
    Hypothesis    string            `json:"hypothesis" toml:"hypothesis"`
    Population    string            `json:"population" toml:"population"`
    Unit          string            `json:"unit" toml:"unit"`
    Variants      []string          `json:"variants" toml:"variants"`
    Splits        map[string]float64 `json:"splits" toml:"splits"`
    Assignment    string            `json:"assignment" toml:"assignment"`
    Budgets       []Budget          `json:"budgets" toml:"budgets"`
    Metrics       []string          `json:"metrics" toml:"metrics"`
    Margins       map[string]string `json:"margins" toml:"margins"`
    StoppingRule  string            `json:"stopping_rule" toml:"stopping_rule"`
    Evidence      []string          `json:"evidence" toml:"evidence"`
    Disposition   string            `json:"disposition" toml:"disposition"` // open string (D6)
}
```

Immutability (AC-1.2) is structural: `TaskRevision.Revision`, `PolicyVersion.Version`, `DesignDecision.Revision` are required `>= 1`, and `Digest` differs across revisions (tested). The no-in-place-rewrite enforcement at rest belongs to the ledger (specs 2–3); this package provides the revision fields, the digest, and the rule in one place.

## 4. Lifecycles (v2 §6.1–§6.2; FR-2, AC-2.1–AC-2.3)

New state types in `v2contract`, distinct from the v1 `supervisor.RunState`/`supervisor.AttemptState` (spec 3 migrates the supervisor onto these; the two types coexist during the migration period only):

```go
type RunState string

const (
    RunCreated       RunState = "created"
    RunAdmission     RunState = "admission"
    RunPlanning      RunState = "planning"
    RunExecuting     RunState = "executing"
    RunIntegrating   RunState = "integrating"
    RunVerifying     RunState = "verifying"
    RunReadyForReview RunState = "ready_for_review"
    RunApplying      RunState = "applying"
    RunBlocked       RunState = "blocked"
    RunStopping      RunState = "stopping"
    RunInterrupted   RunState = "interrupted"
    RunRecovering    RunState = "recovering"
    RunCompleted     RunState = "completed"
    RunCancelled     RunState = "cancelled"
    RunFailed        RunState = "failed"
)

type TaskState string

const (
    TaskPending    TaskState = "pending"
    TaskReady      TaskState = "ready"
    TaskRunning    TaskState = "running"
    TaskCandidate  TaskState = "candidate"
    TaskVerifying  TaskState = "verifying"
    TaskBlocked    TaskState = "blocked"
    TaskAccepted   TaskState = "accepted"
    TaskFailed     TaskState = "failed"
    TaskCancelled  TaskState = "cancelled"
    TaskSuperseded TaskState = "superseded"
)

type AttemptState string

const (
    AttemptReserved            AttemptState = "reserved"
    AttemptLaunchIntentRecorded AttemptState = "launch_intent_recorded"
    AttemptLaunching           AttemptState = "launching"
    AttemptRunning             AttemptState = "running"
    AttemptWaitingNative       AttemptState = "waiting_native"
    AttemptWaitingApproval     AttemptState = "waiting_approval"
    AttemptStopRequested       AttemptState = "stop_requested"
    AttemptInterrupted         AttemptState = "interrupted"
    AttemptQuarantined         AttemptState = "quarantined"
    AttemptStopped             AttemptState = "stopped"
    AttemptSucceededNative     AttemptState = "succeeded_native"
    AttemptFailedNative        AttemptState = "failed_native"
)
```

### 4.1 Transition checks (AC-2.1)

```go
// ErrIllegalTransition names the offending from→to pair, e.g.
// "v2contract: illegal run transition created -> verifying".
var ErrIllegalTransition = errors.New("v2contract: illegal transition")

// CheckRunTransition enforces the v2 §6.1 table. saved is the phase recorded
// when from == blocked; resuming to any other phase is illegal, and saved
// must itself be an eligible saved phase (a nonterminal phase with a
// →blocked edge in the table below). The caller revalidates the blocker,
// authority and revisions before resuming (master §6.1: "never resume from
// a remembered enum alone"); the contract checks to == saved plus
// saved-eligibility.
func CheckRunTransition(from, to, saved RunState) error

// CheckTaskTransition enforces the v2 §6.2 task table; saved as above,
// with the task-table eligible set.
func CheckTaskTransition(from, to, saved TaskState) error

// CheckAttemptTransition enforces the v2 §6.2 attempt table (no saved phase).
func CheckAttemptTransition(from, to AttemptState) error
```

Tables, transcribed exactly from v2 §6.1–§6.2 (the `blocked` row's "saved eligible phase" is the `to == saved` rule plus saved-eligibility; D8). Eligible saved phases are exactly the nonterminal phases with a →blocked edge: Run `{admission, planning, executing, integrating, verifying, ready_for_review, applying, recovering}`; Task `{pending, ready, running, candidate, verifying}`. A `saved` outside its set (terminal, unknown, or edge-less like `stopping`) fails even when `to == saved`.

- Run: `created→{admission}`; `admission→{planning, executing, blocked, failed}`; `planning→{executing, blocked, failed}`; `executing→{integrating, verifying, planning, blocked, failed}`; `integrating→{verifying, executing, blocked, failed}`; `verifying→{ready_for_review, executing, blocked, failed}`; `ready_for_review→{applying, blocked, completed, cancelled}`; `applying→{completed, blocked, failed, interrupted}`; `blocked→{stopping, cancelled, failed}` plus `to == saved`; any nonterminal active phase `→{stopping, interrupted}`; `stopping→{cancelled, interrupted}`; `interrupted→{recovering}`; `recovering→{<reconciled active phase>, ready_for_review, blocked, cancelled, failed, interrupted}` (the reconciled phase is runtime-chosen; the contract accepts any nonterminal active phase); terminal `→{}`.
- Task: `pending→{ready, blocked, cancelled, superseded}`; `ready→{running, blocked, cancelled, superseded}`; `running→{candidate, ready, blocked, failed, cancelled, superseded}`; `candidate→{verifying, ready, blocked, cancelled, superseded}`; `verifying→{accepted, ready, blocked, failed, cancelled, superseded}`; `blocked→{failed, cancelled, superseded}` plus `to == saved`; `accepted→{superseded}`; terminal `→{}`.
- Attempt: `reserved→{launch_intent_recorded, stopped}`; `launch_intent_recorded→{launching, stopped, interrupted}`; `launching→{running, succeeded_native, failed_native, stop_requested, interrupted}`; `running|waiting_native|waiting_approval→{each other, stop_requested, succeeded_native, failed_native, interrupted}`; `stop_requested→{stopped, succeeded_native, failed_native, interrupted, quarantined}`; `interrupted|quarantined→{running, waiting_native, waiting_approval, stop_requested, stopped, succeeded_native, failed_native, quarantined}`; terminal `→{}`.

Unknown states (not in the tables) are rejected as illegal with the state named.

### 4.2 Terminal-entry and reconcile guards (AC-2.2, AC-2.3)

```go
func IsTerminalRunState(s RunState) bool // completed | cancelled | failed

// CheckTerminalEntry refuses a terminal run state while ownership is
// unresolved (v2 §6.1; I06). Maps to ownership_unresolved at the caller.
func CheckTerminalEntry(to RunState, ownershipResolved bool) error

// CheckReconcileIdentity requires the same launch identity when reconciling
// an interrupted/quarantined attempt (v2 §6.2; I12); replay of effects is
// never a reconcile disposition (all reconcile dispositions are
// after_reconciliation with next actions that name the same launch).
// Maps to revision_conflict at the caller.
func CheckReconcileIdentity(oldLaunchID, newLaunchID string) error
```

Lifecycle sentinels map to catalogue codes at the runtime call site (specs 2–4 wrap them into `ControlError`): `ErrIllegalTransition→invalid_contract`, terminal-entry refusal `→ownership_unresolved`, launch mismatch `→revision_conflict`. The pure functions stay free of request context (`owner`, `operation_id`).

## 5. Error catalogue (v2 §4.5; FR-3, AC-3.1–AC-3.2)

24 required codes, counted from v2 §4.5 (the phase-1 "26" is refuted; see requirements Context):

```go
type Code string

const (
    CodeInvalidContract       Code = "invalid_contract"
    CodeRevisionConflict      Code = "revision_conflict"
    CodeDependencyStale       Code = "dependency_stale"
    CodeAuthUnavailable       Code = "auth_unavailable"
    CodeEntitlementUnknown    Code = "entitlement_unknown"
    CodeEntitlementIneligible Code = "entitlement_ineligible"
    CodeAllowanceExhausted    Code = "allowance_exhausted"
    CodeCapabilityUnsupported Code = "capability_unsupported"
    CodePermissionDenied      Code = "permission_denied"
    CodeProviderThrottled     Code = "provider_throttled"
    CodeProtocolMismatch      Code = "protocol_mismatch"
    CodeProcessLost           Code = "process_lost"
    CodeOwnershipUnresolved   Code = "ownership_unresolved"
    CodeToolFailed            Code = "tool_failed"
    CodeCandidateRejected     Code = "candidate_rejected"
    CodeIntegrationConflict   Code = "integration_conflict"
    CodeVerificationFailed    Code = "verification_failed"
    CodeVerificationUnavailable Code = "verification_unavailable"
    CodePersistenceUnavailable Code = "persistence_unavailable"
    CodeCancelIncomplete      Code = "cancel_incomplete"
    CodeExternalEffectUncertain Code = "external_effect_uncertain"
    CodeBudgetExhausted       Code = "budget_exhausted"
    CodePolicyIneligible      Code = "policy_ineligible"
    CodeSchemaTooNew          Code = "schema_too_new"
)

func (c Code) Valid() bool // in the 24-code catalogue

type Disposition string

const (
    DispositionNever               Disposition = "never"
    DispositionAfterUserAction     Disposition = "after_user_action"
    DispositionAfterReconciliation Disposition = "after_reconciliation"
    DispositionAfterCooldown       Disposition = "after_cooldown"
    DispositionBoundedTransient    Disposition = "bounded_transient"
)
```

Default disposition per code (authored here; v2 §4.5 does not assign them — D9). Ranks order retry permissiveness `never < after_user_action < after_reconciliation < after_cooldown < bounded_transient`; `Validate` rejects any disposition ranking above the code's default, which is the machine-checked form of "never widens authority":

| Code | Default disposition | Why this rank |
|---|---|---|
| `invalid_contract` | `never` | Retrying the identical invalid contract cannot succeed |
| `revision_conflict` | `after_reconciliation` | Re-read, then retry at the new revision |
| `dependency_stale` | `after_reconciliation` | Revalidate dependencies first |
| `auth_unavailable` | `after_user_action` | User must restore auth |
| `entitlement_unknown` | `after_user_action` | Unknown blocks until declared/verified (I02) |
| `entitlement_ineligible` | `never` | Ineligibility does not change by retry |
| `allowance_exhausted` | `after_user_action` | User adds allowance or waits out the window |
| `capability_unsupported` | `never` | No retry invents a capability |
| `permission_denied` | `after_user_action` | A grant, not a retry, unblocks |
| `provider_throttled` | `after_cooldown` | Time-gated by the provider |
| `protocol_mismatch` | `after_user_action` | Versions must change first |
| `process_lost` | `after_reconciliation` | Reconcile before any retry (I12) |
| `ownership_unresolved` | `after_reconciliation` | Establish the result first (I06) |
| `tool_failed` | `bounded_transient` | Finite scheduler retries (§3.2) |
| `candidate_rejected` | `never` | Repair is a new operation, not a retry |
| `integration_conflict` | `after_reconciliation` | Bounded repair first |
| `verification_failed` | `never` | Repair uses a new attempt (§6.2) |
| `verification_unavailable` | `bounded_transient` | Finite retries, then blocked |
| `persistence_unavailable` | `bounded_transient` | Finite retries (busy/unavailable) |
| `cancel_incomplete` | `after_reconciliation` | Reconcile uncertainty (§6.4) |
| `external_effect_uncertain` | `after_reconciliation` | Never blindly repeated (I12) |
| `budget_exhausted` | `after_user_action` | User raises the envelope |
| `policy_ineligible` | `never` | A promotion is a different operation |
| `schema_too_new` | `after_user_action` | Upgrade the binary |

```go
func (c Code) DefaultDisposition() Disposition

// ControlError is the v2 §4.5 error: code drives transitions, the message
// only explains. Revision is a pointer so absence omits it (I09).
type ControlError struct {
    Code        Code              `json:"code" toml:"code"`
    Owner       string            `json:"owner" toml:"owner"`
    OperationID string            `json:"operation_id" toml:"operation_id"`
    Object      string            `json:"object,omitempty" toml:"object,omitempty"`
    Revision    *int              `json:"revision,omitempty" toml:"revision,omitempty"`
    Disposition Disposition       `json:"disposition" toml:"disposition"`
    NextAction  string            `json:"next_action" toml:"next_action"`
    Evidence    []string          `json:"evidence_refs,omitempty" toml:"evidence_refs,omitempty"`
    Detail      map[string]string `json:"detail,omitempty" toml:"detail,omitempty"`
}

func (e *ControlError) Error() string // "v2contract: <code>: <next_action>"
func (e *ControlError) Validate() error
// Code in catalogue; Owner/OperationID/NextAction non-empty; Disposition
// rank <= code default rank; every Detail key namespaced ("<ns>/<key>").
// ControlError has pointer-receiver Validate, so Decode[*ControlError] is the
// strict-decode form (records use value receivers and Decode[T]).
```

Adapter mapping (AC-3.2):

```go
// MapAdapterFailure maps an adapter failure to a required code with its
// default (never-widening) disposition and namespaced detail.
func MapAdapterFailure(code Code, owner, operationID, namespace string, cause error) *ControlError
// Detail key is namespace + "/cause". A namespace without effect authority
// (any adapter string) can never produce a disposition above the default.
```

## 6. Event v2 envelope (v2 §4.3; FR-4, AC-4.1–AC-4.2)

`Envelope` mirrors `journal.Event` field-for-field (same JSON names and `int64` sequences) so the spec-3 `Append` extension is mechanical; the types differ only by required `schema_version`:

```go
// Envelope is the v2 canonical event (§4.3). Payload is JSON-canonical and
// excluded from TOML (toml:"-"): TOML tags on records cover configuration
// shapes; event payloads are JSON only.
type Envelope struct {
    SchemaVersion    int             `json:"schema_version" toml:"schema_version"` // must be 2
    EventID          string          `json:"event_id" toml:"event_id"`
    RunID            string          `json:"run_id" toml:"run_id"`
    TaskID           string          `json:"task_id,omitempty" toml:"task_id,omitempty"`
    AttemptID        string          `json:"attempt_id,omitempty" toml:"attempt_id,omitempty"`
    ProducerID       string          `json:"producer_id" toml:"producer_id"`
    ProducerSequence int64           `json:"producer_sequence" toml:"producer_sequence"`
    RunSequence      int64           `json:"run_sequence" toml:"run_sequence"`
    Generation       int64           `json:"generation" toml:"generation"`
    CausedBy         string          `json:"caused_by,omitempty" toml:"caused_by,omitempty"`
    ObservedAt       time.Time       `json:"observed_at" toml:"observed_at"`
    Type             string          `json:"type" toml:"type"`
    Payload          json.RawMessage `json:"payload" toml:"-"`
}

func (e Envelope) Validate() error
// schema_version == 2; EventID/RunID/ProducerID/Type non-empty; sequences and
// generation >= 0; ObservedAt non-zero (an omitted observed_at fails, never
// decodes as year 1); Payload valid JSON object. Event-type vocabulary is open
// (D6): unknown critical types cannot be ignored, which is an ingestion
// behavior in spec 3, not a closed enum here.
```

Pure sequence/generation validators, mirroring `Append`'s v1 rules exactly (§1):

```go
var (
    // ErrDuplicateEvent: event_id already journaled → ack without append.
    ErrDuplicateEvent = errors.New("v2contract: duplicate event")
    ErrSequenceGap    = errors.New("v2contract: producer sequence gap")
    ErrStaleGeneration = errors.New("v2contract: stale generation")
)

// CheckSequence: got == last+1 → nil, else ErrSequenceGap. Duplicates are NOT
// detected here: idempotency is by event_id at the ledger (CheckDuplicate),
// matching Append's check order (event_id first, then generation, then
// sequence).
func CheckSequence(last, got int64) error

// CheckDuplicate: seen → ErrDuplicateEvent (ack, no append); else nil.
func CheckDuplicate(seen bool) error

// CheckGeneration: got >= current → nil; else ErrStaleGeneration, meaning
// retain as quarantined evidence at most, never an accepted result (AC-4.2).
func CheckGeneration(current, got int64) error
```

OQ-9 is decided here (D7): the `journal` table stores both versions; envelope `schema_version = 2` is accepted post-migration; v1 rows decode under the v1 contract forever. The `Append` acceptance change itself is a `supervisor-migration` task consuming `Envelope` and these validators; this spec ships the types, the validators, and the decision. Nothing in this spec edits `Append` or any migration file.

## 7. Fixtures and support matrix (FR-9; NFR-1, NFR-4)

### 7.1 Golden fixtures (AC-9.1)

`internal/v2contract/testdata/`:

- `records/<snake_name>.golden.json` — one valid golden per record (14), each with `schema_version: 2`. Round-trip tests decode each golden, re-encode, and compare bytes modulo key order; flipping one byte changes `Digest`.
- `envelope.golden.json` — a v2 envelope shaped like the §4.3 example.
- `error.golden.json` — a `ControlError` with all fields set.
- `invalid/unknown_authority_key.json` — a `Grant` with an unknown key: `Decode[Grant]` fails naming the key (AC-9.1 malformed/unknown authority keys).
- `invalid/null_measurement.json` — a `Reservation` with `"quantity": null` and an `Artifact` with `"size_bytes": null`: both rejected (decode/validate error naming the field), never read as `0` or `""` (AC-9.1 null measurements; I09). The honest unknown is the explicit string `"unknown"`, which the `reservation` golden uses; optional numerics use pointer fields where absence is meaningful.
- `policy_version.golden.toml` — TOML golden proving the `toml` tags (AC-1.1, AC-9.1); decoded with `BurntSushi/toml` (already required, no new dependency).
- ID/sequence round-trips (AC-9.1): `ids.New("run")`-shaped synthetic IDs survive JSON round-trips; `int64` sequences survive `1<<53` boundary values (encoded as JSON numbers, decoded exactly; a test pins `9223372036854775807`).

Synthetic IDs use the `xxx_demo_nnnn` shape from the §4.3 example or `ids.New` output; fixtures never carry credentials (the NFR-4 test decodes every golden and walks string values — not keys — failing on any `sk-`/`secret`/`token`/`apiKey` hit).

### 7.2 Support matrix (AC-9.2)

`internal/v2contract/SUPPORT.md`: one row per deliverable (14 records, 3 lifecycle tables, error catalogue, envelope, frame limits), each `fixture-tested` with its test names, plus `blocked` rows for JSON Schema artefacts (N4 follow-up) and unresolved auth/OS facts. No row claims `live-qualified`: nothing here has run against a live supervisor. `TestSupportMatrixMatchesEvidence` asserts the matrix lists exactly the shipped deliverables (adding a record without a matrix row fails) and contains zero `live-qualified` claims.

### 7.3 NFR tests

- NFR-1: constants test pins `MaxFrameBytes == 1<<20`, `MaxNestingDepth == 64`, `MaxArtifactRefs == 128`; `CheckFrameLimits(encodedLen, depth, refs int) error` validates a frame against them (enforcement call sites are specs 2–4). `CheckFrameLimits` lives in `v2contract.go` beside the constants.
- NFR-4: `TestContractHasNoNetworkDependency` runs `go list -deps ./internal/v2contract` and fails on any `net` or `modernc.org/sqlite` line (assertable here because the package imports stdlib only — unlike `qualify`, which imports `journal`); tests set no credentials and the fixture-content test above pins no-secret fixtures.

## 8. Tests and CI

No CLI, no TUI, no migrations in this spec: no `cli.Main`, e2e, or live tests. Tests are table-driven unit tests plus golden round-trips in `internal/v2contract/*_test.go`, run by the standard Go gates (`gofmt -l .`, `go vet ./...`, `go test -race ./...`, `go mod tidy -diff`) and `scripts/ci/check-public-hygiene.sh`. No OS-specific files, so no cross-`GOOS` vet. A task is complete when its named tests fail on the pre-change tree (new files: fail as missing) and pass after, with gate output cited.

## 9. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | New package `internal/v2contract` (not an extension of `journal`/`supervisor`) | The vocabulary is an independent contract consumed by three later specs and MH-22; v2 §3.2 splits modules when they acquire independent contracts. Extending `journal` would couple pure types to the SQLite driver and break NFR-4. |
| D2 | `MaxArtifactRefs = 128` per frame | v2 §4.3 says "bounded" without a number; a contract needs an exact bound. 128 bounds memory while exceeding every current spool use (spool lines are single observations). Spec 2 may tighten per-integration; widening needs a revision. |
| D3 | Duplicate the §4.2 scale words in `v2contract` instead of importing `qualify` | `qualify` imports `journal`, which links SQLite — importing it would poison NFR-4 hermeticity for every consumer. The words are stable master-spec text; a test pins word-equality with `qualify`'s scales. |
| D4 | Strict decode (unknown-key rejection) for all records, not just authority/config contracts | Conservative default: a misspelled field failing loudly beats a silently dropped one, and v2 §4.2 requires strictness at least for authority/config. Relaxing per-type later is a compatible change. |
| D5 | `Digest` covers the full canonical JSON with no field exclusions | No v2 record carries its own digest (unlike `qualify.Record`'s `Digest`/`SupersededAt`), so exclusions would be dead complexity. If a self-digest field is ever added, the golden tests fail until exclusions are specified. |
| D6 | Open strings (validated non-empty) for `DeliveryDisposition`, `Reservation.Status`, `Experiment.Disposition`, event `Type` | The master spec does not enumerate these vocabularies; inventing a closed enum risks mismatch with the runtime specs that own them. Documented values guide; the owning spec may tighten. |
| D7 | OQ-9: same `journal` table, v2 accepted post-migration (default (a)) | Matches v2 §5.2 ("Decode historical event v1 separately. Write v2 for new payload contracts") and reuses `Append`'s allocation/fencing. The alternative (reject v2 until migration completes, then a flag day) complicates the migration tasks without narrowing any invariant. Implemented by `supervisor-migration`. |
| D8 | `blocked`-resume checks `to == saved` plus saved-phase eligibility; the runtime revalidates blocker, authority and revisions | The contract owns what the tables decide: target equals the recorded saved phase, and saved is a phase that could have blocked (master §6.1 "Saved eligible phase"). Only the runtime knows which phase the entity actually occupied and whether the blocker still holds, so the caller revalidates those before resuming (master §6.1: "never resume from a remembered enum alone"); the stream-4 caller test for that revalidation is assigned in the honesty register. |
| D9 | Authored per-code default dispositions with rank-ordered widening check | v2 §4.5 requires a disposition per mapping but assigns none; without defaults, "never widens authority" is untestable. The rank order (never < user < reconciliation < cooldown < bounded-transient) makes widening machine-checkable in `Validate`. |
| D10 | One generic `Decode[T Validator]` instead of 14 per-type decode functions | The `qualify.DecodeRecord` precedent covers one type; 14 copies would be boilerplate with 14 chances to diverge. The generic is one strict path, tested once plus per-type `Validate` tests. |
| D11 | No ADR in this spec | Nothing here changes billing, persistence, process ownership, or a frozen public contract: these are internal Go types. v2 §19 freezes a public protocol only after implementations exercise it — the freeze ADR belongs to `supervisor-service`. |

## 10. Honesty register

| Spec demand | Position |
|---|---|
| v2 §4.1 `Release` record | Deferred: optional delivery-effect record (S6); no MH-21 consumer needs it. Not in AC-1.1's list; a follow-up adds it when the first producer exists. |
| Epic DoD "JSON Schemas" | Partially met: Go types + strict decode + golden fixtures are the machine-checkable contract (N4); JSON Schema files are an explicit MH-21 follow-up with no consumer yet. |
| NFR-1 "malformed mandatory frames stop the affected integration" | Partially met: constants + `CheckFrameLimits` ship here; enforcement call sites are specs 2–4 (no runtime exists in this spec to stop). |
| NFR-4 "migration preview" hermeticity | Deferred to `supervisor-migration`, which owns the preview surface; this spec pins contract-validation hermeticity only. |
| AC-2.3 "never relaunch by replay" enforcement | Partially met: same-launch check + reconcile-only dispositions ship; launch-path enforcement is `supervisor-service`/`supervised-stop-recover` behavior. |
| Blocked-resume blocker revalidation (master §6.1) | Caller duty, assigned to stream 4 (`supervised-stop-recover`): it revalidates blocker, authority and revisions before resuming and ships the caller-level test (stale blocker refused); this spec checks `to == saved` plus saved-eligibility only (D8). |
| AC-4.1 supervisor-side allocation behavior | Already holds for v1 in `Append` (`journal.go:318-322`); the v2 acceptance extension is a `supervisor-migration` task consuming this spec. |
| AC-9.2 `live-qualified` rows | None claimed: nothing here has run against a live supervisor; unresolved auth/OS facts stay `blocked`. |

## 11. Cross-Spec References

- Epic plan: `specs/*/v2-contracts-supervisor/plan.md` (this spec is stream 1; scope FR-1–FR-4, FR-9; NFR-1, NFR-4; OQ-9).
- Specs depending on this one: `supervisor-service` (record types, schemas, error codes), `supervisor-migration` (table/contract shapes, `Envelope`, OQ-9 decision), `supervised-stop-recover` (lifecycle tables, error codes). Downstream MH-22 adopts `TaskRevision` verbatim.
- No spec delivers before this one: it has no prerequisite specs.
