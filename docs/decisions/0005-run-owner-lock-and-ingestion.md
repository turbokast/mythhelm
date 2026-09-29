# 0005. Run owner lock, spool ingestion and foreground stop

- Status: accepted
- Date: 2026-09-29

## Context

ADR 0004 gives each attempt a detached worker that writes a spool and never opens SQLite, and says that whichever process holds the run's owner lock ingests that spool. The slice has no per-user daemon (N11), so `mythhelm run` is the run's supervisor. It must accept only its own worker (§7.3, AC-5.1), journal the spool exactly once in order (§7.5, §7.6), and never present a requested stop as a confirmed one (I06, AC-5.5). This record fixes the supervisor side (design §3, decision D6).

## Decision

**Owner lock.** `runs/<run_id>/owner.lock` is held with `flock(LOCK_EX|LOCK_NB)` on Unix and `LockFileEx(LOCKFILE_EXCLUSIVE_LOCK|LOCKFILE_FAIL_IMMEDIATELY)` on Windows (`supervisor.AcquireOwner`). The operating system drops it when the holder dies, so it never goes stale while the holder lives and is never pre-empted. A lock already held, by any open file in any process, is `ErrOwnerHeld` (exit 6). `mythhelm run` takes it before it records anything about the run and holds it until it returns.

**Order of a run.** `run.created`, `run.state_changed{admission}`, then the active-run check, `admission.decided`, `workspace.snapshot_created`, `run.state_changed{executing}`, `attempt.launch_intent_recorded`, and only then the worker spawn. The active-run check runs after the run is recorded, so of two runs admitted at the same moment at least one sees the other. Both may block; both never proceed (I05). A run in `created`, `admission`, `executing`, `verifying`, `applying`, `stopping`, `interrupted` or `recovering` is active. Refusals decided from the command line and the repository alone (flags, profile, host, billing, dirty checkout, preflight) record no run.

**Accepting the worker.** The supervisor waits for `worker.json` and accepts it only if its PID is the spawned PID, the SHA-256 of its launch token equals the journaled one, and the PID's start time equals the recorded one. A worker that has already exited and been reaped has no start time to compare. Its PID is still the one just spawned, and its token authenticates its files, so it is accepted; nothing is ever signalled on that basis. A worker that is not accepted is lost: it is never signalled (AC-10.3).

**Ingestion.** `supervisor.Ingest` reads complete lines from `attempts.spool_offset` onward. Each line must be an event of this run and attempt, from producer `wrk_<attempt_id>`, of a type a worker emits; anything else is `ErrCorruptSpool`, and ingestion stops there. Each event is appended with its projection update (attempt state through the design §4 state machine, process IDs, session ID) and the new offset in one transaction. A torn last line waits for the next call. An event already journaled is ignored by its `event_id`, and the offset still moves past it.

**Outcomes.** A worker that exits before its attempt's terminal state is journaled leaves the attempt `interrupted` then `quarantined` and the run `interrupted`, all with reason `worker_lost` (exit 6). An attempt the worker ends `interrupted` (`stop_unconfirmed`, `unresolved_descendants`, `worker_persistence_failed`) is quarantined in the same way. `stopped` ends the run `cancelled` (exit 130). `failed_native` ends it `failed`/`native_failed` (exit 4). Freezing and verification do not exist yet, so `succeeded_native` goes through `verifying` to `failed`/`verification_unavailable` (exit 5). It is never reported as a verified deliverable.

**Foreground stop.** The first interrupt writes `stop.request` through `workers.RequestStop`, moves the run to `stopping`, and prints `stop requested — waiting for worker confirmation`. The command exits 130 only after the worker's `attempt.stopped` and terminal state are journaled. A second interrupt detaches: the command prints `stop not yet confirmed; run 'mythhelm recover <id>'`, leaves the run `stopping`, and exits 6. The worker keeps owning the attempt and finishes the stop on its own.

## Consequences

- One supervising process per run, proved by an OS lock, with no daemon. `recover` (Task 15) takes the same lock and resumes ingestion from the stored offset.
- `--format jsonl` stdout is exactly the run's journaled events in `run_sequence` order, then one `run.result`, which is not journaled and comes from its own one-event `cli_` producer.
- **Known limitations.** Until `recover` exists, a detached or interrupted run stays active and blocks new runs. Windows interrupts are not covered by tests.
- **Tests that pin this behaviour** (`internal/supervisor`): `TestOwnerLockExclusive`, `TestRunSecondActiveRunBlocked`, `TestAdmissionDecidedBeforeWorkerSpawn`, `TestIngestResumesFromOffsetWithoutDuplicates`, `TestRunOutcomeExitCodes`, `TestJSONLStdoutOnlyEnvelopes`, `TestCtrlCOnceStopsAndExits130`, `TestCtrlCTwiceDetachesExit6` and `TestWorkerKilledRunInterruptedExit6` (the last three on Unix).
