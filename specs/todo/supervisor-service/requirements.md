## Supervisor Service — Requirements

> One lazily started per-user supervisor per execution host owns the canonical ledger, global reservations and recovery, behind an authenticated local control protocol with idempotent intents — replacing the one-process-per-run topology with run ownership as assignments under the service. A slice of the MH-21 epic (`specs/*/v2-contracts-supervisor/plan.md`, stream 2 of 4). Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it.

## Context

- **Backlog card**: MH-21
- **Issue**: [#127](https://github.com/turbokast/mythhelm/issues/127) (records planned work; no live-test authority)
- **Epic plan**: `specs/*/v2-contracts-supervisor/plan.md` (maintainer-approved 2026-10-08, 4-way split). This spec is stream 2; FR-5/FR-6 numbers are inherited from the archived epic requirements (`specs/archived/v2-contracts-supervisor-requirements/requirements.md`) and are not renumbered.
- **Prerequisite**: `specs/*/v2-contract-vocabulary/` (stream 1) provides `internal/v2contract`: record types, lifecycle checks, the error catalogue, the event-v2 envelope and frame-limit constants. This spec consumes those signatures verbatim and defines no contract vocabulary of its own.
- **Grounding** (verified 2026-10-08 against the working tree):
  - Per-run owners acquired at 4 sites — HOLDS (`grep -rn 'AcquireOwner' internal cmd --include='*.go' | grep -v _test` → def + `pipeline.go:130`, `recover.go:47`, `cli/tui.go:185`, `cli/apply.go:67`).
  - No daemon or control server — HOLDS (`grep -rn -E 'daemon|UnixListener|net\.Listen' internal cmd adapters --include='*.go' | grep -v _test` → 0 hits).
  - No `operation_id`/`expected_revision` machinery — HOLDS (same-shaped grep → 0 hits).
  - `golang.org/x/sys v0.48.0` direct in `go.mod:13`; `BurntSushi/toml v1.6.0` direct in `go.mod:8`.

### Objectives

- **O1**: Work survives client closure: at most one supervisor per user per host holds the instance lock and serves control requests.
- **O2**: Every control mutation is an authenticated, idempotent intent: repeats return the prior result, conflicting reuses fail as conflicts.
- **O3**: The supervisor is the sole logical writer: reservations and ledger mutations happen in one transaction each; workers never write SQLite.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Contract vocabulary, schemas, fixtures (stream 1) | Consume `v2contract` signatures exactly; no parallel error codes or envelope |
| N2 | Legacy migration, backup/restore, v1 import (stream 3, `supervisor-migration`) | No coexisting writers by design: the instance lock and root-conflict refusal this spec ships are the mechanism stream 3 drains onto |
| N3 | Stop ladder and recovery (stream 4, `supervised-stop-recover`) | `I06 (v2 §2)`: UI exit transfers nothing; unconfirmed stops stay unconfirmed |
| N4 | Router/scheduler intelligence | `I05 (v2 §2)`: reservation transaction shape and idempotency envelope only |
| N5 | TCP or remote control | `I03 (v2 §2)`: control stays on private local transport; no listener on TCP by default |

## Functional Requirements

### FR-5 — One per-user supervisor (v2 §3.1, I05, I18, G04)

- **AC-5.1** [I05 (v2 §2), I18 (v2 §2)] The system shall run at most one supervisor per user per execution host, holding an OS instance lock for its lifetime, with run ownership as assignments under it rather than a second election.
- **AC-5.2** [G04 (v2 §18.3)] When a control request arrives, the system shall authenticate peer identity where available, bind it to an `operation_id` and `expected_revision`, return the prior result for an identical repeat, and reject an ID reused with different arguments as a conflict.
- **AC-5.3** [I18 (v2 §2), I03 (v2 §2)] The system shall expose control only over a private Unix socket or user-restricted Windows named pipe with attempt-scoped capability tokens, and shall not listen on TCP by default.
- **AC-5.4** [I05 (v2 §2)] If an alternative `MYTHHELM_HOME` is presented while work is active under another root, then the system shall refuse the conflicting root rather than silently run a second authority.

### FR-6 — Sole-writer ledger and reservations (v2 §5.1, I05, I23, G04)

- **AC-6.1** [I23 (v2 §2)] The system shall make the supervisor the sole logical writer: each mutation validates preconditions, appends an event, updates projections and enqueues an outbox item in one transaction; workers never write SQLite.
- **AC-6.2** [I05 (v2 §2)] The system shall key global reservations by execution host and resource identity in the ledger, and shall prevent new work when bounds fail rather than invent allowance.
- **AC-6.3** [I09 (v2 §2)] The system shall filter authority before returning content, counts, names or index metadata, so separate repositories cannot discover each other through shared endpoints.

## Non-Functional Requirements

- **NFR-1** [G04 (v2 §18.3)] Control frames shall respect the §4.3 default limits (1 MiB encoded frame, nesting depth 64, bounded artifact references); malformed mandatory frames stop the affected integration.
- **NFR-3** [AT-40 (v2 §18.2), G04 (v2 §18.3)] Advertised OS/arch combinations shall pass their own filesystem/process/detach tests with `CGO_ENABLED=0`.

NFR-2 belongs to stream 3 (`supervisor-migration`, SQLite posture); the numbering gap is intentional.

## Definition of Done

- [ ] Supervisor singleton per user/host with instance lock and root-conflict refusal (AC-5.1, AC-5.4)
- [ ] Authenticated local IPC, no default TCP listener (AC-5.3)
- [ ] Idempotent intents: repeat returns prior result, conflicting reuse is a conflict (AC-5.2)
- [ ] Global reservations keyed by host and resource; bounds failures prevent work (AC-6.2)
- [ ] Sole-writer transactions; workers never write SQLite (AC-6.1)
- [ ] Authority filtered before any returned content, counts, names or metadata (AC-6.3)
- [ ] Frame limits enforced at every control ingress; malformed mandatory frames stop the integration (NFR-1)
- [ ] Advertised OS/arch combinations pass filesystem/process/detach tests with `CGO_ENABLED=0` (NFR-3)

## Open Questions

Tracked with defaults and deciders in `scratchpad.md`: OQ-3 (Windows supervisor form), OQ-6 (Unix peer-auth mechanism), OQ-7 (Windows pipe transport), OQ-10 (control-protocol package shape), OQ-11 (idempotency store location).
