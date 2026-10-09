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

<!-- pending -->

## Task 4 — Supervisor-loss envelope and reconnect handshake

<!-- pending -->
