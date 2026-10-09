# supervised-stop-recover — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Versioned ladder contract and `stop` intent

- **Produces**: `control.StopLadder{Version, Steps}` + `Validate`, `StopParams{AttemptID, LadderVersion}`, `StopReceipt`, `MaxStopRungGrace`/`MaxStopDeadline`, `StopHandler(StopDeps{DB, Journal, StateDir, WaitDeadline, PollInterval})`, `Server.RegisterStop`, `PinnedAdmission`, `CurrentGeneration`, `AdmissionPinnedPayload`, `EventAdmissionPinned`, `workers.NonceDigest`, `workers.MatchIdentity`, `Identity.Nonce`; codes `cancel_incomplete`/`ownership_unresolved` in `stop.go`, `process_lost` in `control.go`.
- **Produces, differs from design**: the stopped report carries `ladder_version` (reject wrong versions); the wait deadline is `started_at + MaxStopDeadline` (no ladder store exists yet — the pinned steps arrive in Task 2's `Launch`); `stop_requested` records the `operation_id` as the attempt reason.
- **For dependents (Task 2)**: emit `attempt.stopped` with `confirmed`, `sent` (required, `[{signal, grace-ns}]`), `ladder_version` (required, must equal the pinned version or the report is skipped), `unresolved_pids`; reuse `control.AdmissionPinnedPayload` and `control.EventAdmissionPinned` in `RecordAdmissionPinned`; echo the raw nonce into `worker.json`.
- **For dependents (Task 2)**: `CurrentGeneration` reads the lock file and refuses a missing lock or foreign root, but does no PID-liveness probe; a stale lock file from a dead supervisor still reads. Decide at the spawn site how to treat the value when no supervisor holds the lock.
- **For dependents (Task 3)**: a handler that must persist writes alongside a failure must return the failure in the `Result` with a nil error — `Execute` rolls back writes on a returned `*Error` (`recordQuarantine` is the pattern; your one-pass record needs it too).
- **For dependents (Task 4)**: fold `stop`/`recover` into `NewSupervisorServer` with the matrix rows; production needs a `*journal.Journal` beside the ledger handle for `StopDeps` (the tests keep both open on one file). Land `Execute` long-handler support (the Task 1 deviation) before serving `stop`: until the wait leaves the transaction, every stop wedges mutating intents past `busy_timeout` for up to `MaxStopDeadline`.
- **Traps**: `Execute` serialises concurrent identical intents on the write lock (repeats block-then-replay; no independent join); `worker.json` without `nonce` never matches (empty fails closed); unknown `attempt_id` fails before any journal read.
- **Deviations changing later inputs (Tasks 2, 4)**: `RegisterStop` keeps `stop` out of `NewSupervisorServer` until Task 4 wires it; OQ-SR1 resolved as the opaque-string default (mismatch is `revision_conflict`, never a compatibility guess).

## Task 2 — Worker climbs the pinned ladder

<!-- pending -->

## Task 3 — One-pass recovery through the supervisor

- **Produces**: `control.RecoverParams{RunID}`, `control.RecoverOutcome` (`reconnected`/`continued`/`partial_candidate`/`quarantined`), `control.RecoverDeps{DB, StateDir, Launch, ExamineCount}`, `control.RecoverHandler`, `control.Reconcile(ctx, deps, runID) (RecoverOutcome, error)`, `control.RecoveryReport`/`RecoveryMarkers`, `control.EventRecoveryDecided` (`attempt.recovery_decided`), `Server.RegisterRecover`, code `external_effect_uncertain` in `recover.go`.
- **Produces, differs from design**: `Reconcile` takes `RecoverDeps`, not `(db, runID)` — the pinned signature cannot see worker.json nonces, spool bytes, or the launcher; `RecoverDeps` has no `Journal` (the pass reads the journal table through its own tx); quarantine reasons are `ownership_unresolved`/`stale_generation`/`spool_unverifiable`/`external_effect_uncertain` with stale and spool mapping to the `ownership_unresolved` code; an eventless continuation past `MaxRecoveryContinuationAge` (15min) is re-examined, not replayed.
- **For dependents (Task 4)**: call `Reconcile(ctx, deps, runID)` for the ledger pass — it never launches, so the handshake stays spawn-free; fold `recover` (with `stop`) into `NewSupervisorServer` plus the matrix rows; production needs `DB` + `StateDir` + a `Launch` func (nil fails a fresh `continued` closed, rolling the admission back). Land the durable post-commit launch handoff (the Task 3 review-round-1 deferral) before serving `recover`: until the launch leaves the transaction, a commit failure after a successful spawn orphans a worker with no durable admission.
- **For dependents (Task 4)**: the one-pass record is the latest `attempt.recovery_decided` event per run; quarantine/uncertainty return the failure in the `Result` with nil error so the record commits (Task 1 pattern); refusals (`invalid_contract`, `revision_conflict`) record nothing.
- **For dependents (Task 4)**: continuations reuse the recovered attempt's `launch_token_sha256` digest; when the handshake mints new-generation tokens at spawn, decide whether the attempts digest rotates (breaking same-launch lineage) or `capability_tokens` carries the live token (the reconnect launch check reads the attempts digest and may need to follow).
- **Traps**: the record describes the world as the pass leaves it (attempts re-read post-transition) — comparing pre-write markers would make every pass look fresh; liveness itself is a marker, so a worker dying between passes reopens one; concurrent fresh passes serialise on `Execute`'s write lock and losers replay, never relaunch.
- **Deviations changing later inputs (Task 4)**: `RegisterRecover` keeps `recover` out of `NewSupervisorServer` until Task 4 wires it; `Reconcile`'s deps signature; a continued-from-quarantined attempt stays `quarantined` beside its continuation.

## Task 4 — Supervisor-loss envelope and reconnect handshake

<!-- pending -->
