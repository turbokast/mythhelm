## Supervised Stop and Recover — Requirements

> Stop and recovery run through the per-user supervisor: stop requests escalate through a versioned adapter/platform ladder with a finite deadline and a recorded receipt, recovery chooses exactly one outcome in one reconciliation pass, and a worker that loses its supervisor finishes only its pinned episode and awaits reconnection. Stream 4 of the MH-21 epic (`specs/*/v2-contracts-supervisor/plan.md`), delivering epic FR-8 (AC-8.1–AC-8.3). Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it.

### Context

- **Backlog card**: MH-21
- **Epic plan**: `specs/*/v2-contracts-supervisor/plan.md` (stream 4; scope FR-8; branch from stream 2, parallel-safe with stream 3 except `internal/supervisor/pipeline.go`, shared with stream-3 Task 4 — stream-3 Task 4 lands first, Tasks 2 and 4 rebase)
- **Prerequisite specs**: `specs/*/v2-contract-vocabulary/` (lifecycle tables, error catalogue, envelope), `specs/*/supervisor-service/` (control protocol, ownership records, intent methods). No implementation task here starts before `supervisor-service` ships.
- **Grounding** (verified 2026-10-08 against the working tree):
  - Stop today is a request file, not a ladder climb: `Stop` verifies identity then writes `stop.request` (`internal/supervisor/stop.go:24-76`); `RequestStop` at `internal/workers/worker.go:263` — HOLDS.
  - The ladder exists but is unversioned: `StopLadder []StopStep` with no version field (`internal/adapter/adapter.go:85`, `internal/workers/worker.go:81`); `ClimbLadder` at `adapter.go:199` — HOLDS.
  - Recovery predates the supervisor service: `Recover` takes the per-run owner lock (`internal/supervisor/recover.go:47`) and can loop through watch/reconcile without a one-pass bound — HOLDS.
  - No `stop`/`recover` control methods exist: stream 2 reports them as unknown methods returning `capability_unsupported` (`specs/*/supervisor-service/design.md` §6) — HOLDS.
  - Worker heartbeats assert liveness only (`internal/workers/worker.go:911-917`); no supervisor-loss envelope exists — HOLDS.

### Objectives

- **O1**: A stop request deterministically escalates through the ladder version pinned at admission and leaves a receipt naming what was actually signaled.
- **O2**: Recovery after client closure, worker loss or supervisor restart picks exactly one outcome and stops after one unresolved pass instead of retrying blindly.
- **O3**: A worker that loses its supervisor cannot start new tasks or approve new effects while unsupervised.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Singleton, transport, idempotent intents, reservations (stream 2) | This spec's `stop`/`recover` methods are `control.Handler`s with `operation_id` idempotency; no second control surface |
| N2 | Migration, legacy import, backup/restore (stream 3) | Pre-migration, the per-run `Stop`/`Recover` path keeps working; this spec adds the service path, it does not delete the legacy one |
| N3 | Router/scheduler intelligence | G04 (v2 §18.3): stop/recovery transitions are code-driven (`cancel_incomplete`, `ownership_unresolved`); no ranking or fairness logic |
| N4 | Live qualification of per-surface signals | I14: each ladder row is `fixture-tested` or `blocked`, never claimed `live-qualified` without evidence |

## Functional Requirements

Acceptance criteria use EARS. Tags name the invariants, gates and sections each one serves. Epic AC-8.x is the parent of each FR.

### FR-1 — Versioned stop ladders (v2 §6.4, epic AC-8.1, I06)

- **AC-1.1** [I06 (v2 §2), v2 §6.4] When a stop is requested, the system shall escalate through the stop-ladder version pinned at admission for that attempt, rung by rung, each rung with its qualified signal and grace period.
- **AC-1.2** [I06 (v2 §2), I09 (v2 §2), v2 §6.4] When the ladder finishes, whether confirmed or not, the system shall record a stop receipt naming the ladder version, the signals actually sent with their grace periods, and the unresolved PIDs, if any; unknown signal outcomes stay `unknown`, never `confirmed`.
- **AC-1.3** [I06 (v2 §2), G04 (v2 §18.3), v2 §6.4] If the ladder's finite deadline expires without confirmed termination, then the system shall report incomplete cancellation (`cancel_incomplete`), quarantine the attempt, and signal no further processes.

### FR-2 — One-pass recovery (v2 §6.4, epic AC-8.2, I12, I06)

- **AC-2.1** [v2 §6.4, I06 (v2 §2)] When recovering, the system shall choose exactly one outcome: reconnect to the same live worker, admit a continuation as a new attempt, expose a partial candidate, or leave ownership quarantined.
- **AC-2.2** [v2 §6.4, I06 (v2 §2)] If a reconciliation pass leaves ownership unresolved, then the system shall stop after that one pass until fresh evidence or user action arrives; it shall not start a second pass on its own.
- **AC-2.3** [I12 (v2 §2), v2 §6.2] When recovery admits a continuation, the system shall bind it to the same launch identity and shall never relaunch native tools, prompts or external effects by replay.

### FR-3 — Supervisor-loss worker envelope (v2 §6.4, epic AC-8.3, I06, I21)

- **AC-3.1** [I06 (v2 §2), I21 (v2 §2), v2 §6.4] If the supervisor is lost (no `supervisor.beat` fresher than `MaxSupervisorBeatAge`, design §5), then the worker shall complete only its currently admitted episode within its pinned envelope, spool results, and await reconnection, starting no new tasks and approving no new effects.
- **AC-3.2** [I12 (v2 §2), v2 §6.4] If the worker's spool or persistence fails while unsupervised, then the worker shall perform a controlled stop rather than continue blind.
- **AC-3.3** [I06 (v2 §2), v2 §6.4] When the supervisor returns, it shall reconcile the surviving worker's exact launch identity and unacknowledged spool before issuing a new-generation control token; old-generation evidence is retained for reconciliation, never accepted as fresh control or completion.

## Non-Functional Requirements

Numbers follow the epic's: NFR-1–NFR-4 belong to the archived epic requirements and streams 1–3 (NFR-1 frame limits, NFR-2 SQLite posture, NFR-3 OS/arch matrix, NFR-4 hermetic contract validation). This spec's own NFRs start at NFR-5. Epic NFR-1 still binds here in its enforcement half: the stream-1 limit constants bound this spec's new `stop`/`recover` intents (design §2).

- **NFR-5** [v2 §6.4] Process matching shall use PID **and** start identity **and** nonce, never PID alone; an expired lease or heartbeat never releases a resource a possibly live process still uses.
- **NFR-6** [I13 (v2 §2)] Stop-ladder, recovery and envelope tests shall run with no agent credentials and no network service; live-signal tests use synthetic processes owned by the test.

## Definition of Done

- [ ] Versioned ladders pinned at admission; stop escalates rung by rung with per-surface deadlines (FR-1)
- [ ] Stop receipts record version, actual signals/grace, unresolved PIDs; incomplete cancellation quarantines (FR-1)
- [ ] Recovery chooses exactly one of the four outcomes and stops after one unresolved pass (FR-2)
- [ ] Continuations reuse the launch identity; no effect replay (FR-2)
- [ ] Supervisor loss pins the worker to its episode; spool failure stops it; reconnect reconciles before a new token (FR-3)
- [ ] `stop` and `recover` are idempotent control intents; no second control surface (N1)
