# Supervised Stop and Recover — Design

Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0.
Consumes `specs/*/v2-contract-vocabulary/design.md` (cited `v2c§N`) and
`specs/*/supervisor-service/design.md` (cited `svc§N`) verbatim. Nothing here
redefines a record type, error code, transition table, frame or intent shape.

## 1. Current state

- Stop is a request file plus an unversioned ladder: `supervisor.Stop`
  verifies identity then writes `stop.request`
  (`internal/supervisor/stop.go:24-76`); `workers.RequestStop` at
  `internal/workers/worker.go:263` ("the first request stands"); the ladder is
  `StopLadder []StopStep` with no version field
  (`internal/adapter/adapter.go:85`, `internal/workers/worker.go:81`); rungs are
  `StopStep{Signal, Grace}` (`adapter.go:115-118`); both adapter sessions share
  `ClimbLadder` (`adapter.go:199-221`).
- Identity matching already uses PID + start time + launch token
  (`stop.go:81-135`); no nonce participates yet (NFR-5 gap).
- Recovery predates the service: `RecoverWithHooks` takes the per-run owner
  lock (`internal/supervisor/recover.go:47`), reattaches or quarantines
  (`quarantineRecovery`, `recover.go:331-346`), then watches and may conclude —
  with no one-pass bound and no supervisor-owned generation.
- Worker liveness primitives exist: heartbeats assert liveness only
  (`worker.go:911-917`); no supervisor-loss envelope, no reconnect handshake.
  Structural grounding: the `workers` exported API is `AttemptDir`, `Spawn`,
  `Main`, `ReadIdentity`, `RequestStop` (`worker.go:100-263`),
  `ProcessStartTime`, `OrphanPIDs`, `AttemptMarker` (`proc_*.go`,
  `orphan.go`) and the `Launch`/`Identity`/`ProcessIdentity` types — no
  envelope type or reconnect entry point; the spool event set is the ten
  `attempt.*` constants (`spool.go:14-25`), none a loss/reconnect envelope;
  the supervise loop only spools observations, beats, and polls
  `stop.request` (`worker.go:541-608`). (The keyword sweep `grep -rn
  'supervisor.*lost\|AwaitReconnect\|pinned envelope' internal
  --include='*.go' | grep -v _test` also prints nothing.)
- Stream 2 leaves exactly this hole: `stop`/`recover` are unknown intent
  methods returning `capability_unsupported` (svc§6); the intent, ledger and
  token machinery they must ride on ships there.

## 2. Consumed contracts (exact; not redefined)

From stream 2 (`supervisor-service/design.md`):

- `control.Intent{OperationID, ExpectedRevision, Method, Params,
  CapabilityToken}`; `control.Result{OperationID, Revision, Body, Error}`;
  `control.Handler func(ctx, peer, intent) (Result, error)`; `control.Execute`
  (identical repeat replays the stored `Result`; reused ID with different args
  returns `revision_conflict`) (svc§6).
- `control.Mutate(ctx, db, fn)` sole-writer transaction: preconditions → append
  event → projections → outbox, no network I/O inside (svc§7).
- `control.Peer` authenticated identity; attempt-scoped capability tokens minted
  at admission (svc§4). Generation fencing via `Append` (svc§1).
- New methods register beside `reserve`/`release`/`heartbeat`/`status`/
  `assign`/`read` (svc§6).

From stream 1 (`v2-contract-vocabulary/design.md`):

- Lifecycle: `AttemptState` constants including `stop_requested`,
  `interrupted`, `quarantined`, `stopped` (v2c§4); `CheckAttemptTransition`,
  `CheckTerminalEntry` (→`ownership_unresolved`), `CheckReconcileIdentity`
  (→`revision_conflict`) (v2c§4.1–§4.2).
- Errors: `Code` catalogue (v2c§5) — this spec uses `cancel_incomplete`,
  `ownership_unresolved`, `process_lost`, `revision_conflict`,
  `external_effect_uncertain`, `capability_unsupported`; `ControlError` with
  `Validate()`; `MapAdapterFailure`; default dispositions
  (`cancel_incomplete`→`after_reconciliation`, `process_lost`→
  `after_reconciliation`, `capability_unsupported`→`never`).
- `CheckGeneration` / `CheckSequence` / `CheckDuplicate` (v2c§6);
  `MaxFrameBytes == 1<<20` (v2c§7.3) bounds stop/recover params (epic NFR-1:
  limit constants ship in stream 1, enforcement lands at every ingress —
  the new `stop`/`recover` intents are ingresses, so they carry the
  enforcement half here).

## 3. Versioned stop ladders (v2 §6.4; FR-1)

New types in `internal/control` (the svc§2 package; D1):

```go
// StopLadder is one versioned stop ladder. Version is pinned at admission
// and recorded in the stop receipt; Steps climb in order.
type StopLadder struct {
    Version string             `json:"version"` // e.g. "stop-ladder/v1"
    Steps   []adapter.StopStep `json:"steps"`
}

// MaxStopRungGrace bounds one rung; MaxStopDeadline bounds the whole ladder.
const (
    MaxStopRungGrace = 30 * time.Second
    MaxStopDeadline  = 2 * time.Minute
)

// Validate checks the ladder shape. Failure cases: invalid_contract for an
// empty version, zero steps, an unknown signal, a non-positive grace, a
// rung grace above MaxStopRungGrace, or a total above MaxStopDeadline.
func (l StopLadder) Validate() error

// StopParams is the params object of the "stop" intent.
type StopParams struct {
    AttemptID     string `json:"attempt_id"`
    LadderVersion string `json:"ladder_version"` // must equal the pinned version
}

// StopReceipt records what the ladder actually did (AC-1.2). Sent holds the
// signals delivered with the grace each was given; Confirmed is true only
// when the group was observed gone; UnresolvedPIDs names the remainder.
type StopReceipt struct {
    AttemptID      string             `json:"attempt_id"`
    LadderVersion  string             `json:"ladder_version"`
    Sent           []adapter.StopStep `json:"sent"`
    Confirmed      bool               `json:"confirmed"`
    UnresolvedPIDs []int              `json:"unresolved_pids,omitempty"`
    Quarantined    bool               `json:"quarantined"`
}
```

Method `stop` (a `control.Handler`, idempotent via `control.Execute`).
It runs in two short `Mutate` scopes with delivery and the bounded wait
outside any transaction — the sole-writer transaction is never held across
the wait, which would wedge every other intent past `busy_timeout` for up
to `MaxStopDeadline`:

- Reads the attempt's pinned ladder version and expected nonce digest via
  `PinnedAdmission` (pinning contract below); a `LadderVersion` mismatch
  returns `revision_conflict` without signalling (D2).
- `Mutate` #1 (request): the `stop_requested` transition is checked with
  `v2contract.CheckAttemptTransition` and persisted with the intent's
  `operation_id`, `started_at`, and the recorded ladder deadline.
  Re-entrant: if `stop_requested` is already persisted for this
  `operation_id`, the transaction is a no-op returning the recorded
  deadline.
- After the commit, delivery: `workers.RequestStop` with the attempt
  directory and the intent's `operation_id` as the request ID (the first
  request stands; repeats are no-ops). The worker climbs the pinned
  ladder when it observes `stop.request` (Task 2, same `ClimbLadder` both
  sessions share); the receipt reflects that climb, not the request.
- Then the Handler waits outside any transaction for the worker's stopped
  report up to the recorded deadline (poll discipline below).
- `Mutate` #2 (receipt): on the stopped report, the `StopReceipt` is
  journaled as the `Result.Body` with the checked terminal transition for
  the reported outcome; on deadline expiry, `cancel_incomplete` with
  disposition `after_reconciliation`, attempt to `quarantined` (checked
  transition), no further signals (AC-1.3). Exactly one execution
  journals: `Mutate` #2 first reads the operations row for the
  `operation_id` (svc§6); if a `Result` is already stored, it returns
  that without journaling.
- Repeat-during-wait: an identical repeat while the first execution still
  waits finds no stored `Result`, re-enters `Mutate` #1 as a no-op, finds
  `stop.request` already standing (no second delivery), and joins an
  independent bounded wait for the same report up to the recorded
  deadline; every execution returns the identical stored `Result`. A
  repeat after a supervisor restart behaves the same, waiting only the
  remaining time past the recorded deadline before taking the quarantine
  path.
- Failure cases: `invalid_contract` (bad params, unknown attempt);
  `revision_conflict` (ID reuse, version mismatch, stale `expected_revision`);
  `ownership_unresolved` (identity unverifiable — PID/start/nonce per NFR-5);
  `process_lost` (worker already gone; reconcile, don't signal);
  `persistence_unavailable` (ledger I/O).

Delivery to the worker reuses the legacy request file as the
supervisor→worker stop channel (D7):

```go
// Already exists at internal/workers/worker.go:263; the first request
// stands, a later one returns nil without replacing it.
func RequestStop(dir, requestID string) error
```

The import runs control→workers only; the worker never imports `control`.

Stopped-report contract (Task 1 implements the wait, Task 2 the emission):

- Medium: one `attempt.stopped` spool line (`evStopped`,
  `internal/workers/spool.go:24`) in the attempt's `spool.jsonl`, each line
  a `journal.Event` envelope written by `spool.emit` (`spool.go:72`).
- Payload: the current shape (`worker.go:740-744`) plus `sent`, the
  signal+grace pairs AC-1.2 needs for `StopReceipt.Sent`: `confirmed`
  (bool), `unresolved_pids` ([]int), `sent` ([]`adapter.StopStep` as
  `[{"signal","grace"}]` in climb order, grace in nanoseconds per the
  `time.Duration` encoding), `signals_sent` (kept for the legacy path),
  `descendant_scan` (string), `unresolved_identities`
  ([]`workers.ProcessIdentity`). Unknown outcomes stay `unknown`: no
  report by the deadline means `cancel_incomplete`, never `confirmed`.
- Observation: the Handler polls `spool.jsonl` for a complete
  `attempt.stopped` line for the attempt, on the `ingestInterval` (100ms)
  precedent (`internal/supervisor/pipeline.go:30`), with the
  `supervisor.Ingest` line discipline — complete lines past a byte offset,
  a torn last line left for the next poll
  (`internal/supervisor/ingest.go:51-80`).

The worker side carries the pinned ladder without importing `control` (the
reverse import would cycle with the delivery call above). `workers.Launch`
(`worker.go:70-82`) gains exactly three fields; the steps stay in the existing
`StopLadder` field:

```go
type Launch struct {
    // ... existing fields unchanged ...
    LadderVersion string             `json:"ladder_version"` // pinned at admission; must equal StopParams.LadderVersion
    Nonce         string             `json:"nonce"`          // raw worker nonce, minted at spawn (Task 2); echoed into worker.json
    Generation    int64              `json:"generation"`     // supervisor boot generation (svc§3), pinned at admission; the §5 envelope's generation
    StopLadder    []adapter.StopStep `json:"stop_ladder"`    // steps of the pinned version, in climb order
}
```

NFR-5 matching needs a nonce the launch token cannot serve (the token is
compared by hash, never read back). `workers.Identity` (`worker.go:87-97`)
gains exactly one field (Task 1, the contracts task):

```go
type Identity struct {
    // ... existing fields unchanged ...
    // Nonce pins the worker to its admission. worker.json carries the raw
    // value echoed from Launch.Nonce (0600 file, the LaunchToken precedent);
    // the journaled expected record carries NonceDigest of it, never the raw
    // value (the launch_token_sha256 precedent). Empty in pre-change files,
    // which therefore never match.
    Nonce string `json:"nonce"`
}

// NonceDigest is the lowercase hex sha256 of a raw worker nonce. It lives in
// workers (not control) because MatchIdentity is the consumer and workers
// must not import control.
func NonceDigest(nonce string) string

// MatchIdentity reports whether observed is the worker expected names:
// PID and start time must agree, and the nonce must agree. expected.Nonce
// is the journaled digest; observed.Nonce is the raw worker.json value;
// agreement is NonceDigest(observed.Nonce) == expected.Nonce. An empty
// nonce on either side never matches, so pre-change identity files fail
// closed.
func MatchIdentity(expected, observed Identity) bool
```

Admission pinning (the `MintToken`-at-admission precedent, svc§4) is a
journal event, not new columns — stream 3 owns later migrations (svc D2),
so this spec adds no migration file. At the `pipeline.go:298` spawn site
(Task 2), the supervisor mints the nonce (crypto/rand), reads the boot
generation, journals `attempt.admission_pinned`, and delivers the raw
nonce plus the pinned ladder version and generation in the `Launch`; the
worker echoes the raw nonce into `worker.json`:

```go
// AdmissionPinnedPayload is the payload of attempt.admission_pinned.
type AdmissionPinnedPayload struct {
    LadderVersion string `json:"ladder_version"`
    NonceSHA256   string `json:"nonce_sha256"` // NonceDigest of the raw nonce in Launch.Nonce
}

// RecordAdmissionPinned journals attempt.admission_pinned for the attempt,
// in the same admission step as RecordLaunchIntent (state.go:226) and
// before workers.Spawn. Task 2, in pipeline.go next to the spawn site.
func RecordAdmissionPinned(ctx context.Context, j *journal.Journal, runID, taskID, attemptID, ladderVersion, nonceSHA256 string, producer *Producer) error

// PinnedAdmission returns the admission-pinned ladder version and expected
// nonce digest for attemptID from its latest attempt.admission_pinned
// event. Failure cases: invalid_contract (unknown attempt, or an attempt
// admitted before pinning, which has no such event);
// persistence_unavailable (ledger I/O). Task 1; consumed verbatim by
// Tasks 2/3/4.
func PinnedAdmission(ctx context.Context, db *sql.DB, attemptID string) (ladderVersion, nonceSHA256 string, err error)

// CurrentGeneration reports this supervisor's boot generation from the
// stream-2 lock-file metadata for dir (svc§3). Failure cases: no live lock
// or unreadable metadata (plain error; the spawn site fails the admission
// rather than pinning a guessed generation). Task 1.
func CurrentGeneration(dir string) (int64, error)
```

The stop Handler, `Reconcile` (§4) and the reconnect handshake (§5) read
the journaled digest via `PinnedAdmission` and compare with
`MatchIdentity` against the observed `worker.json`; any mismatch is
`ownership_unresolved` (stop) or evidence for quarantine (recovery).

The per-run `supervisor.Stop` stays for the migration period (stream 3 drains
it); the service path never calls it.

## 4. One-pass recovery (v2 §6.4; FR-2)

Method `recover` (a `control.Handler`):

```go
// RecoverParams is the params object of the "recover" intent.
type RecoverParams struct {
    RunID string `json:"run_id"`
}

// RecoverOutcome is exactly one of the four v2 §6.4 dispositions.
type RecoverOutcome string

const (
    RecoverReconnected      RecoverOutcome = "reconnected"
    RecoverContinued        RecoverOutcome = "continued"
    RecoverPartialCandidate RecoverOutcome = "partial_candidate"
    RecoverQuarantined      RecoverOutcome = "quarantined"
)

// Reconcile examines launch intent, process/session identity, unacknowledged
// spool and outstanding effects, then returns exactly one outcome. It performs
// at most one reconciliation pass: when ownership stays unresolved it returns
// RecoverQuarantined with an ownership_unresolved ControlError and records
// the pass in the ledger so a repeat without fresh evidence replays the
// stored outcome instead of re-running. Failure cases: invalid_contract
// (unknown run); ownership_unresolved (one pass done, still unresolved);
// revision_conflict (launch identity mismatch via CheckReconcileIdentity);
// external_effect_uncertain (ambiguous irreversible effect — never retried).
func Reconcile(ctx context.Context, db *sql.DB, runID string) (RecoverOutcome, error)
```

Rules:

- Reconnect requires the same live worker: PID **and** start identity **and**
  nonce match via `MatchIdentity` against the `PinnedAdmission` digest
  (§3, NFR-5); fencing via `v2contract.CheckGeneration` on the journal
  generation (stale → quarantined evidence at most, v2c§6 — this is the
  journal generation, not the §5 boot generation).
- A continuation is a new attempt admitted after the old writer is provably
  gone (lease expiry is not proof, v2 §6.4); `CheckReconcileIdentity` binds the
  same launch identity; replay of native tools/prompts/effects is never a
  disposition (AC-2.3, v2c D9 `after_reconciliation` only).
- `recover` is idempotent: the ledger's one-pass record makes an identical
  repeat return the stored outcome; fresh evidence (new spool offset,
  user action) opens a new pass (D3).

## 5. Supervisor-loss worker envelope (v2 §6.4; FR-3)

The worker builds its `EpisodeEnvelope` from values it already holds plus
the one new handoff field from §3 — no separate delivery path, and the
worker never opens SQLite (I23). `AttemptID` comes from argv `--attempt`
(`worker.go:155-166`, held as `w.attemptID`); `LaunchID` is
`Launch.LaunchToken`, the one-use launch identity already in the held
`Launch`; `Generation` is `Launch.Generation`, the supervisor boot
generation (svc§3 instance-lock generation, bumped on stale-lock adoption)
pinned at admission:

```go
// EpisodeEnvelope pins what the worker may do without a supervisor.
type EpisodeEnvelope struct {
    AttemptID  string `json:"attempt_id"`
    LaunchID   string `json:"launch_id"` // one-use launch identity: the LaunchToken value
    Generation int64  `json:"generation"` // supervisor boot generation (svc§3), pinned at admission
}

// CheckEnvelope allows only the pinned episode. Failure cases:
// revision_conflict (attempt/launch/generation mismatch); the worker treats
// any failure as "await reconnection", never as permission to improvise.
func CheckEnvelope(env EpisodeEnvelope, attemptID, launchID string, generation int64) error

// AwaitReconnect completes the currently admitted episode within env, spools
// results, then polls supervisor.beat until the supervisor returns or ctx
// ends. It starts no new task and approves no new effect. Failure cases: a
// spool/persistence failure triggers a controlled stop (the attempt is
// concluded as interrupted with the failure recorded, not continued blind);
// ctx cancellation ends the wait without touching the spool.
func AwaitReconnect(ctx context.Context, dir string, env EpisodeEnvelope) error
```

Supervisor loss and return are file signals in the attempt directory (D4).
The supervisor's watch loop writes `supervisor.beat` after every successful
`Ingest` (`pipeline.go:533` tick); the worker polls it in the supervise
loop (`worker.go:543`) and in `AwaitReconnect`:

```go
// SupervisorBeat is the supervisor.beat payload, mirroring the worker
// heartbeat shape (worker.go:911-917) plus the boot generation.
type SupervisorBeat struct {
    Beat       uint64 `json:"beat"`
    At         string `json:"at"` // RFC3339Nano, UTC
    Generation int64  `json:"generation"`
}

// MaxSupervisorBeatAge bounds staleness: 300ms (3x the 100ms watch-tick
// beat interval, OQ-SR3).
const MaxSupervisorBeatAge = 300 * time.Millisecond

// WriteSupervisorBeat writes dir/supervisor.beat (0600). The beat counter
// is owned by the caller (the watch loop, like w.beats).
func WriteSupervisorBeat(dir string, generation int64, beat uint64, at time.Time) error

// ReadSupervisorBeat reports the latest supervisor beat. ok is false when
// the file is missing or unparsable, which the caller treats as stale.
func ReadSupervisorBeat(dir string) (at time.Time, generation int64, ok bool)
```

- Loss: no beat fresher than `MaxSupervisorBeatAge` while the episode runs
  → the supervise loop enters `AwaitReconnect`, which finishes the admitted
  episode within `env`, spools results, and waits. UI/Herdr loss never
  triggers this (v2 §6.4).
- Return: a beat fresher than `MaxSupervisorBeatAge`, any generation →
  `AwaitReconnect` returns and normal supervised operation resumes. A
  changed generation means the supervisor restarted; the handshake below
  re-homes the worker — the worker never treats the mismatch as
  permission to improvise.
- On return, the supervisor reconciles exact launch identity plus
  unacknowledged spool (v2c `CheckDuplicate`/`CheckSequence`) before minting a
  new-generation capability token; budgets and input ownership are not reset;
  old-generation evidence is retained, never accepted as fresh control (AC-3.3).
- Heartbeats keep their current meaning: worker→file liveness only
  (`worker.go:911-917`), not progress, not authority; the supervisor beat
  is the reverse direction and asserts supervisor presence only.

## 6. Tests and CI (NFR-6, NFR-5)

- Unit + table-driven tests in `internal/control/*_test.go` and
  `internal/workers/*_test.go`: ladder validation (empty version, zero steps,
  over-deadline total → `invalid_contract`); version mismatch →
  `revision_conflict` with no request file and no ledger transition;
  deadline expiry (scripted worker never reports stopped) →
  `cancel_incomplete` + quarantined, with signal-level assertions in the
  worker tests (Task 2); recovery outcome table
  (one outcome each; second pass without fresh evidence replays stored
  outcome); envelope checks (wrong launch ID → wait, never improvise).
- Process tests use synthetic processes the test owns (sleep children in their
  own group); PID-reuse is simulated by mismatched start identity, asserting
  no signal to the stranger. NFR-5's nonce: identity files gain a nonce field;
  a test mismatches it and asserts `ownership_unresolved`.
- No agent credentials, no network: tests reuse the stream-1 hermeticity
  pattern for credentials, with a dial-side structural check in place of the
  deps pattern (`internal/control` necessarily imports `net` for its
  Unix-socket listener, svc§2), pinned by Task 4's
  `TestHermeticNoCredentialsOrNetwork`.
  Gates: `gofmt -l .`, `go vet ./...` (plus `GOOS=windows` and
  `GOOS=darwin` vet for the platform ladder/process files), `go test -race
  ./...`, `go mod tidy -diff`, `scripts/ci/check-public-hygiene.sh`.
- `internal/control/SUPPORT.md` gains one row per new deliverable
  (`fixture-tested` or `blocked`; no `live-qualified` claims), following the
  svc§11 precedent.

## 7. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | Ladder/recover/envelope types live in `internal/control` (stream 2's package), worker execution stays in `internal/workers` | The intent methods are control-surface contracts; stream 2's package is their home. Putting them in `supervisor` would fork the control surface during the migration window. No new package: nothing here is a new contract surface, only new methods on the existing one. |
| D2 | Ladder version pinned at admission; mismatch returns `revision_conflict` without signalling | A stop must climb what was admitted, not what the requester claims; signalling under a guessed ladder risks the wrong process. Alternative (requester-supplied ladder) rejected: it lets any client pick signals. |
| D3 | One-pass record persisted in the ledger, not in memory | The supervisor may restart between passes; an in-memory flag would re-run the pass and double-reconcile. Same-transaction with the outcome event keeps repeat-vs-fresh-evidence exact. |
| D4 | Supervisor loss = stale `supervisor.beat` (no beat fresher than `MaxSupervisorBeatAge`) | Worker heartbeats are worker→file writes asserting worker liveness (`worker.go:911-917`); the worker cannot "miss" its own window, so they cannot signal supervisor loss. The supervisor beat reverses the direction over the same file mechanism with no new transport. Watching the supervisor PID instead would break across the lazy-spawn/adoption design (svc§3, §10). |
| D5 | No ADR in this spec | Ladder versioning, one-pass recovery and the worker envelope refine behavior inside the service topology whose ADR stream 2 writes (svc D8); nothing here changes billing, persistence layout, process ownership or a frozen public contract. |
| D6 | `MaxStopRungGrace` 30s, `MaxStopDeadline` 2min | A rung must outlast a native's graceful shutdown (SIGINT handlers flushing state take seconds, not milliseconds); 30s caps one misbehaving rung without stalling the ladder. The 2min total bounds a worst-case stop so recovery and quarantine proceed instead of hanging the supervisor; both are fixture-tested defaults the ladder version pins, not live-qualified per-surface timings (I14). |
| D7 | Supervisor→worker stop delivery reuses the legacy `stop.request` file | The file channel already exists, is first-stands idempotent, and keeps the worker free of control-client code during the migration window (N2). Alternative (worker as a control client receiving `stop` over RPC) rejected: stream 2 defines no worker↔supervisor control connection (per svc D7, control clients are CLI-internal), so the worker would need a new transport, auth, and reconnect story just to receive stops. |

## 8. Honesty register

| Spec demand | Position |
|---|---|
| v2 §6.4 "actual signals and grace periods qualified per surface" | Partially met: versions, deadlines and receipts ship with fixture-tested per-surface defaults; no row claims `live-qualified` (I14). |
| v2 §6.4 detached descendants, containers, ports | Partially met: owned process group/session (Unix) and Job Object (Windows) handling ships; launched containers and held ports are recorded as unresolved and quarantine, not force-cleaned. |
| v2 §6.4 three bounded transient transport retries per operation | Deferred: transport retry policy belongs to the control client as a whole; this spec's intents are safe to retry (idempotent) but the client does not yet retry them. Explicit follow-up. |
| v2 §6.4 "Match PID and start identity and nonce" | Met for the service path (nonce added to identity files); the legacy per-run `recoveryIdentity` keeps PID+start+token until stream 3 drains it. |
| NFR-5 Windows Job Object confirmation semantics | `fixture-tested` on the Windows runner; `blocked` where the runner cannot model job nesting. No guessed rows. |

## Cross-Spec References

- Epic plan: `specs/*/v2-contracts-supervisor/plan.md` (this spec is stream 4; scope FR-8: versioned stop ladders, one-pass recovery, supervisor-loss worker envelope).
- Depends on `specs/*/v2-contract-vocabulary/` (stream 1): lifecycle tables and transition checks, error catalogue and dispositions, generation/sequence validators, frame limits.
- Depends on `specs/*/supervisor-service/` (stream 2): `internal/control` package, intent/Handler/Execute machinery, sole-writer `Mutate`, capability tokens, generation fencing. No implementation task in this spec starts before stream 2 ships.
- Parallel-safe with `specs/*/supervisor-migration/` (stream 3) except one shared file: `internal/supervisor/pipeline.go` is edited here (Task 2 spawn site ~:298 + `RecordAdmissionPinned`; Task 4 watch-tick beat write ~:533) and by stream-3 Task 4 (admission/drain hooks). Ordering: stream-3 Task 4 lands first; this spec's Tasks 2 and 4 rebase onto it. The three regions are disjoint; a rebase conflict keeps all three. All other files are disjoint (control/worker paths here; journal/migrations/import there).
