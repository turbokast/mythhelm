## Supervised Stop and Recover — Tasks

### Dependencies

- Epic `specs/*/v2-contracts-supervisor/plan.md`, stream 4 of 4. Prerequisite specs: `specs/*/v2-contract-vocabulary/` provides `v2contract` lifecycle checks (`CheckAttemptTransition`, `CheckReconcileIdentity`), the error catalogue (`cancel_incomplete`, `ownership_unresolved`, `process_lost`, `revision_conflict`, `external_effect_uncertain`) and generation/sequence validators; `specs/*/supervisor-service/` provides the `internal/control` package with `Intent`/`Result`/`Handler`/`Execute`, sole-writer `Mutate`, capability tokens and generation fencing. No task here starts before `supervisor-service` ships. Parallel-safe with `specs/*/supervisor-migration/` except `internal/supervisor/pipeline.go`, shared with stream-3 Task 4: stream-3 Task 4 lands first; this spec's Tasks 2 and 4 rebase onto it (disjoint regions).
- Task 1 first (contracts). Tasks 2 and 3 may run in parallel after Task 1 (disjoint Files). Task 4 after Tasks 2 and 3 (reconnect handshake consumes `Reconcile`; envelope work edits `worker.go` and `pipeline.go`, also edited by Task 2).
- **Gates for every task.** Format first (`gofmt -w` on changed Go files; `go mod tidy` when needed), then from the tree root: `gofmt -l .` (must print nothing), `go vet ./...`, `go test -race ./...`, `go mod tidy -diff` (must print nothing), plus `GOOS=windows go vet ./...` and `GOOS=darwin go vet ./...` for tasks touching platform files, and `scripts/ci/check-public-hygiene.sh`. Files existing or an agent reporting success is not completion; cited gate output is.
- **Completion convention.** Append ` ✅ COMPLETED` to the task heading and add `Status`, `Implementation`, `Spec deviations` and `Files modified` per `.claude/skills/task-completion/SKILL.md`, keeping every original field; append a scratchpad note.
- **Commits.** Signed off (`git commit -s`); no ADR in this spec (design D5).

---

## Implementation Tasks

### Task 1 — Versioned ladder contract and `stop` intent

- **Domain/agent**: go-implementer
- **Budget**: complex (identity contract and matcher plus the stop Handler across `control` and `workers`)
- **Depends on**: None
- **Change**: Add the versioned `StopLadder`/`StopParams`/`StopReceipt` types, the NFR-5 worker-identity contract (`Identity.Nonce`, `NonceDigest`, `MatchIdentity`, the expected-nonce rule), the admission-pinning reads (`PinnedAdmission`, `CurrentGeneration`), and the idempotent `stop` control method, so stops climb the admission-pinned ladder, match PID/start/nonce, and leave a receipt.
- **Files**:
  - `internal/control/stopladder.go` (StopLadder, StopParams, StopReceipt, Validate, deadline constants)
  - `internal/control/stop.go` (`stop` Handler: version check, two Mutate scopes, receipt)
  - `internal/control/server.go` (register the `stop` method)
  - `internal/control/pinning.go` (`PinnedAdmission`, `CurrentGeneration`)
  - `internal/control/stop_test.go`
  - `internal/control/pinning_test.go`
  - `internal/workers/worker.go` (`Identity.Nonce` field, `NonceDigest` and `MatchIdentity` only — Task 2 owns the rest)
  - `internal/workers/identity_test.go` (matcher table test)
- **Produces**: `control.StopLadder{Version string, Steps []adapter.StopStep}`, `func (l StopLadder) Validate() error`, `control.StopParams{AttemptID, LadderVersion string}`, `control.StopReceipt`, `MaxStopRungGrace`, `MaxStopDeadline`, method `"stop"` with two `Mutate` scopes (persist `stop_requested` with `operation_id`/`started_at`/deadline; journal receipt) and delivery plus bounded wait outside any tx (design §3), delivery via `workers.RequestStop(dir, operationID)` after the `Mutate` #1 commit, the stopped-report schema plus wait semantics both tasks share (spool `attempt.stopped` line with `sent` signal+grace pairs; poll to the recorded deadline, else `cancel_incomplete` — design §3), `workers.Identity` gains `Nonce string` (`json:"nonce"`; raw value in `worker.json`, digest in journaled expected records, empty in pre-change files, never matches), `func NonceDigest(nonce string) string` (lowercase hex sha256), `func MatchIdentity(expected, observed Identity) bool` (PID and start time agree; `NonceDigest(observed.Nonce) == expected.Nonce`; empty nonce on either side never matches), the `attempt.admission_pinned` event with payload `{"ladder_version", "nonce_sha256"}` (design §3), `func PinnedAdmission(ctx context.Context, j *journal.Journal, runID, attemptID string) (ladderVersion, nonceSHA256 string, err error)` (reads via `j.Events` over the same store `RecordAdmissionPinned` writes; no separate database binding), `func CurrentGeneration(dir string) (int64, error)`, and the expected-nonce rule: the nonce is minted at spawn (Task 2), its digest journaled in `attempt.admission_pinned`, and every consumer compares the journaled digest against the observed `worker.json` with `MatchIdentity`
- **Acceptance**:
  - `TestStopLadderValidation`: empty version, zero steps, unknown signal, zero grace, rung grace above `MaxStopRungGrace`, total above `MaxStopDeadline` each return `invalid_contract` naming the field; a ladder with total exactly `MaxStopDeadline` passes.
  - `TestStopIntentWritesRequestFile`: a successful `stop` intent leaves a `stop.request` in the attempt directory whose request ID equals the intent's `operation_id`; a version-mismatched intent writes no request file.
  - `TestStopVersionMismatchWritesNoRequest`: a `stop` intent whose `LadderVersion` differs from the pinned version returns `revision_conflict`, writes no `stop.request` file, and journals no `stop_requested` transition; the same test with the ladder unversioned (pre-change shape) fails to compile, proving the version gate is load-bearing.
  - `TestStopIntentIdempotent`: an identical repeated `operation_id` returns byte-identical `Result.Body`; the same ID with different params returns `revision_conflict` without executing.
  - `TestStopDeadlineQuarantines`: with a scripted worker that never reports stopped, the `stop` Handler's post-commit wait expires and yields `cancel_incomplete` with disposition `after_reconciliation`, a `quarantined` attempt transition, and no further `stop.request` after the deadline.
  - `TestStopAgainstDeadWorkerProcessLost`: with a scripted dead worker (identity matches no live process), the `stop` intent returns `process_lost`, writes no `stop.request`, and signals nothing.
  - `TestStopConfirmedReceiptAssemblesFromStoppedReport`: with a scripted worker that reports stopped on cue, the `stop` intent returns a `StopReceipt` in `Result.Body` with the pinned version, the `sent` signal+grace pairs from the report, and `confirmed=true`; the same test with a wrong version in the report or a missing `sent` fails.
  - `TestStopLedgerUnavailablePersistenceUnavailable`: with a faulted (closed) ledger DB, the `stop` intent returns `persistence_unavailable` and writes no `stop.request`.
  - `TestStopRepeatDuringWaitJoins`: two concurrent identical `operation_id` intents while the worker has not yet reported yield byte-identical `Result.Body` once it reports, with exactly one journaled receipt and one standing `stop.request`.
  - `TestPinnedAdmissionReadsLatestPinning`: against one temp `*journal.Journal`, append two `attempt.admission_pinned` events for the attempt, then the production `PinnedAdmission` on that same store returns the ladder version and nonce digest from the latest; an unknown attempt or one with no such event (pre-pinning shape) returns `invalid_contract`.
  - `TestMatchIdentityRequiresPidStartAndNonce`: a table over PID agree/mismatch, start time agree/mismatch, and nonce (observed raw whose digest equals the journaled digest / different raw / empty on either side) — all agree matches, any one differing fails — plus empty nonce on either side (pre-change shape) never matches.
  - `TestStopIdentityMismatchOwnershipUnresolved`: a `stop` intent whose observed worker identity fails `MatchIdentity` against the journaled digest (observed raw nonce whose digest differs, while PID and start time agree) returns `ownership_unresolved`, writes no `stop.request`, and signals nothing.
- **Test plan**: Table-driven unit tests with a scripted worker (writes `attempt.stopped` on cue, or never); ledger assertions via `Mutate` on a temp DB; pinning reads against fixture `attempt.admission_pinned` events in a temp journal; golden `StopReceipt` JSON. Signal-level assertions live in Task 2.
- **Invariants touched**: I06 (v2 §6.4: unconfirmed stops stay unconfirmed; receipt distinguishes `unknown` from `confirmed`); I09 (v2 §4.2: unknown outcomes never read as confirmed).

### Task 2 — Worker climbs the pinned ladder

- **Domain/agent**: go-implementer
- **Budget**: complex (process signals, cross-platform group semantics)
- **Depends on**: Task 1
- **Change**: Pin the ladder version in the launch handoff and have the worker climb exactly that ladder, recording actual signals and grace in the stop receipt; mint the per-worker nonce at spawn, journal its digest plus the pinned version and generation, and deliver the raw values in the launch handoff. Ordering with stream-3 Task 4 (both edit `pipeline.go`): stream-3 Task 4 lands first; this task rebases (disjoint regions: Spawn call vs drain hooks). Consumes Task 1's stopped-report schema (design §3 stopped-report contract), identity contract (`Identity.Nonce`, `NonceDigest`, `MatchIdentity`, expected-nonce rule) and pinning contract (`attempt.admission_pinned` payload, `PinnedAdmission`, `CurrentGeneration`); emits exactly the stopped-report payload, defines no new field.
- **Files**:
  - `internal/workers/worker.go` (pin `LadderVersion`, `Nonce` and `Generation` in `Launch`; climb on `stop.request`; echo the nonce into `worker.json`; match via Task 1's `MatchIdentity`)
  - `internal/workers/worker_test.go`
  - `internal/workers/worker_unix_test.go` (group-stop confirmation test)
  - `internal/workers/worker_windows_test.go` (job-object confirmation test)
  - `internal/supervisor/pipeline.go` (spawn site `pipeline.go:298` only: mint the worker nonce, read the boot generation via `control.CurrentGeneration`, journal the pinning with `RecordAdmissionPinned`, set `Launch.Nonce`/`Launch.LadderVersion`/`Launch.Generation` — the `MintToken`-at-admission precedent, svc§4; `internal/supervisor` imports `internal/control` for the generation read only, and stream 2's package has no control→supervisor edge (svc§2 layout), so no cycle)
  - `internal/supervisor/pinning_test.go` (mint-and-journal test)
- **Produces**: `workers.Launch` gains `LadderVersion string` (`json:"ladder_version"`), `Nonce string` (`json:"nonce"`, raw value) and `Generation int64` (`json:"generation"`, boot generation) alongside the existing `StopLadder []adapter.StopStep` (`json:"stop_ladder"`), with no `internal/control` import (design §3); `func RecordAdmissionPinned(ctx context.Context, j *journal.Journal, runID, taskID, attemptID, ladderVersion, nonceSHA256 string, producer *Producer) error` (design §3); the spawn site mints the nonce (crypto/rand), journals `attempt.admission_pinned` with the pinned ladder version and `NonceDigest` of the nonce before `workers.Spawn`, and delivers the raw nonce, pinned version and `CurrentGeneration` in the `Launch` for the worker to echo into `worker.json`
- **Acceptance**:
  - `TestWorkerClimbsPinnedLadder`: a worker handed ladder version `stop-ladder/v1` with two rungs against a synthetic owned child records both signals with their graces in `attempt.stopped` and reports `confirmed=true`; deleting one rung from the handoff changes the recorded receipt, proving the receipt reflects the climb, not the default.
  - `TestWorkerSignalsNoStranger`: with a mismatched start identity (simulated PID reuse) the worker sends zero signals and the stop resolves to `ownership_unresolved`, not `confirmed`.
  - `TestStopRequestFirstStands`: a second `stop.request` with a different ID leaves the first request's ID recorded; `RequestStop` still returns nil.
  - `TestNonceMismatchBlocks`: an identity whose raw nonce digests to a different value than the journaled digest fails the match even when PID and start time agree; an empty-nonce legacy identity (pre-change `worker.json` shape, asserted via a fixture with no `nonce` key) likewise never matches.
  - `TestSpawnMintsAndJournalsNonce`: the `pipeline.go:298` spawn path mints a nonce, journals `attempt.admission_pinned` with the pinned ladder version and the nonce digest (read back verbatim via Task 1's `PinnedAdmission`), and delivers the same raw nonce plus the `CurrentGeneration` value in the `Launch`; the worker's `worker.json` echoes the raw nonce, and a second spawn mints a different value.
- **Test plan**: Synthetic sleep-child processes in their own group owned by the test; fake session for the mismatch cases; spawn tests fixture a stream-2 lock file for the generation read (existing pipeline spawn tests updated with the same fixture); build-tagged Unix group test (`worker_unix_test.go`) plus a Windows job-object test (`worker_windows_test.go`) on the Windows runner.
- **Invariants touched**: I06 (v2 §6.4: a sent signal is not a confirmed stop; strangers never signaled); I18 (v2 §2: one process owner — the worker signals only its owned group).

### Task 3 — One-pass recovery through the supervisor

- **Domain/agent**: go-implementer
- **Budget**: complex (process ownership, liveness, persistence)
- **Depends on**: Task 1
- **Change**: Add the idempotent `recover` control method with `Reconcile` choosing exactly one of the four outcomes and persisting the one-pass record, so recovery never re-runs without fresh evidence. Worker-identity matching consumes Task 1's `MatchIdentity` against the `PinnedAdmission` digest (design §3).
- **Files**:
  - `internal/control/recover.go` (`RecoverParams`, `RecoverOutcome`, `Reconcile`, one-pass ledger record)
  - `internal/control/server.go` (register the `recover` method)
  - `internal/control/recover_test.go`
- **Produces**: `control.RecoverParams{RunID string}`, `control.RecoverOutcome` (`reconnected`/`continued`/`partial_candidate`/`quarantined`), `func Reconcile(ctx context.Context, db *sql.DB, runID string) (RecoverOutcome, error)`
- **Acceptance**:
  - `TestReconcileChoosesOneOutcome`: a table over the five preconditions (live same worker; dead worker with clean workspace; dead worker with partial output; unverifiable identity; live same worker with a stale journal generation) yields exactly `reconnected`, `continued`, `partial_candidate`, `quarantined`, `quarantined` respectively — the stale-generation row quarantines with retained evidence and no reconnect — each with its journaled event.
  - `TestReconcileUnknownRunInvalid`: `Reconcile` on an unknown run returns `invalid_contract` and records no outcome.
  - `TestSecondPassWithoutFreshEvidenceReplays`: after a `quarantined` outcome, an identical `recover` returns the stored outcome without re-examining (assert via a generation counter that does not advance); new spool bytes then open a new pass.
  - `TestContinuationKeepsLaunchIdentity`: the continued attempt carries the same `launch_id`; a forged different `launch_id` returns `revision_conflict` and starts nothing.
  - `TestRecoveryNeverReplaysEffects`: a recovery fixture with an ambiguous irreversible effect yields `external_effect_uncertain` and no native launch (assert the launcher fake records zero `Launch` calls).
- **Test plan**: Table-driven tests on a temp ledger with scripted worker liveness (Task 1's `MatchIdentity` over a scripted observed identity), scripted spool contents, and a recording launcher fake; golden outcome events.
- **Invariants touched**: I12 (v2 §6.4: reconcile before retry; replay never replays effects); I06 (v2 §2: ownership unresolved stays out of terminal states via `CheckTerminalEntry`).

### Task 4 — Supervisor-loss envelope and reconnect handshake

- **Domain/agent**: go-implementer
- **Budget**: complex (unsupervised worker behavior, generation fencing)
- **Depends on**: Task 2, Task 3
- **Change**: Add the pinned episode envelope so a supervisor-less worker finishes only its episode and awaits reconnection, and the supervisor reconciles before minting a new-generation token. Loss/return is the `supervisor.beat` file signal (design §5). Lands on Task 2's `pipeline.go`; stream-3 Task 4 lands first per the Task 2 ordering note (same file, disjoint regions). Handshake identity matching consumes Task 1's `MatchIdentity` against the `PinnedAdmission` digest (design §3).
- **Files**:
  - `internal/workers/envelope.go` (`EpisodeEnvelope`, `CheckEnvelope`, `AwaitReconnect`, `ReadSupervisorBeat`)
  - `internal/workers/worker.go` (detect stale `supervisor.beat`; enter `AwaitReconnect`; build `env` from argv `--attempt`, `Launch.LaunchToken`, `Launch.Generation`)
  - `internal/control/reconnect.go` (reconcile-then-mint-token handshake consuming `Reconcile`; `SupervisorBeat`, `MaxSupervisorBeatAge`, `WriteSupervisorBeat`)
  - `internal/supervisor/pipeline.go` (watch-tick `supervisor.beat` write after `Ingest` only; extends the Task 2 supervisor→control import edge, still no cycle per svc§2)
  - `internal/workers/envelope_test.go`
  - `internal/control/reconnect_test.go` (reconcile-before-mint handshake, old-generation rejection)
  - `internal/control/support_test.go` (extend the stream-2 exactness test with the new rows)
  - `internal/control/SUPPORT.md` (one row per new deliverable, design §6)
- **Produces**: `workers.EpisodeEnvelope{AttemptID, LaunchID string, Generation int64}` (built from argv `--attempt` + `Launch.LaunchToken` + `Launch.Generation`; no new handoff beyond §3), `func CheckEnvelope(env EpisodeEnvelope, attemptID, launchID string, generation int64) error`, `func AwaitReconnect(ctx context.Context, dir string, env EpisodeEnvelope) error` (polls `supervisor.beat`), `control.SupervisorBeat{Beat uint64, At string, Generation int64}`, `MaxSupervisorBeatAge` (300ms), `func WriteSupervisorBeat(dir string, generation int64, beat uint64, at time.Time) error`, `func ReadSupervisorBeat(dir string) (at time.Time, generation int64, ok bool)`, `SUPPORT.md` rows for the `stop`/`recover` methods, the pinned ladder, and the envelope/reconnect handshake (each `fixture-tested` with test names, or `blocked`; zero `live-qualified` claims)
- **Acceptance**:
  - `TestSupervisorBeatLossAndReturn`: with a fresh `supervisor.beat` the episode runs supervised; deleting the beat (stale past `MaxSupervisorBeatAge`) moves the worker into `AwaitReconnect` after it finishes and spools its episode; writing a fresh beat again ends the wait.
  - `TestUnsupervisedWorkerStartsNothing`: with `supervisor.beat` stale, the worker finishes its admitted episode, spools the result, and a probe asserting "new task started / new effect approved" finds zero; a misbehaving-worker fixture that starts a new task trips the probe, proving it is load-bearing; `CheckEnvelope` with a wrong `launch_id` returns `revision_conflict` and the worker keeps waiting.
  - `TestUILossHasNoExecutionEffect`: cutting only the UI channel leaves execution unaffected — the admitted episode proceeds and no reconnect wait starts; only a stale `supervisor.beat` enters `AwaitReconnect`.
  - `TestSupportMatrixMatchesEvidence` (extend the stream-2 exactness test): the matrix gains one row per new deliverable (`stop` method, `recover` method, pinned ladder, envelope/reconnect), each `fixture-tested` or `blocked`, with zero `live-qualified` claims; adding behavior without a row fails.
  - `TestSpoolFailureStopsBlindWorker`: a spool write failure while unsupervised concludes the attempt as `interrupted` with the failure recorded; the worker exits instead of continuing.
  - `TestAwaitReconnectCancelLeavesSpool`: cancelling `ctx` ends `AwaitReconnect` without touching the spool — spooled bytes and offsets are unchanged.
  - `TestReconnectReconcilesFirst`: on supervisor return with unacknowledged spool, the handshake reconciles launch identity and spool (duplicates acked, no double-accept) before the new-generation token is minted; budgets and input ownership are unchanged after adoption.
  - `TestOldGenerationNeverFresh`: old-generation evidence presented as fresh control is retained for reconciliation and rejected as completion (assert the completion projection does not advance).
  - `TestHermeticNoCredentialsOrNetwork` (NFR-6, stream-1 credential pattern): with a planted `ANTHROPIC_API_KEY` in the environment, the new tests pass and no golden's decoded string values contain the planted value or any `sk-`/`secret`/`token`/`apiKey` hit (values walked, not keys) — and the walker proves it is load-bearing by tripping on a decoy golden containing a planted `sk-decoy` value, plus asserting values-visited > 0; the new non-test code dials no network service — a structural test fails on any `net.Dial`/`DialContext` call in the new files, and a fixture with a planted `net.Dial` fails it, proving it is load-bearing (the stream-1 `go list -deps` net-absence pattern cannot apply here: `internal/control` necessarily imports `net` for its Unix-socket listener, svc§2).
- **Test plan**: Worker harness with a writable/deletable `supervisor.beat` and a fault-injectable spool; supervisor-side handshake tests on a temp ledger; golden reconnect events.
- **Invariants touched**: I06 (v2 §2: UI/Herdr loss has no execution effect — covered by a test cutting only the UI channel); I12 (v2 §6.4: spool/persist failure handling — AC-3.2's controlled stop); I21 (v2 §2: unsupervised work stays inside the pinned envelope); I23 (v2 §2: the supervisor remains the single ledger writer; the worker only spools).
