## Supervisor Migration — Requirements

> Migrates the dogfood slice to the per-user supervisor service through additive migrations, a legacy import that preserves v1 runs, IDs, billing posture and evidence, and an explicit backup, drain/adopt/quarantine and restore procedure — with no coexisting writers, no silent reinterpretation of v1 events, and refusal of newer schemas and downgrades. Stream 3 of the MH-21 epic. Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it.

## Context

- **Backlog card**: MH-21
- **Issue**: [#127](https://github.com/turbokast/mythhelm/issues/127) (records planned work; no live-test authority)
- **Epic**: `specs/*/v2-contracts-supervisor/plan.md` (stream 3; depends on streams 1 and 2, which must ship before any implementation task here starts)
- **Problem**: the journal schema is v1 with no v2 task/policy/authority contract tables; every `mythhelm run` is its own supervisor proved by a per-run OS lock. The move to one lazily started per-user supervisor must preserve v1 runs, IDs and evidence with no window where legacy and service writers coexist.
- **Grounding** (verified 2026-10-08 against the working tree):
  - `0001_init.sql` has 10 v1 tables and no v2 task/policy/authority/reservation tables — HOLDS (`grep -n 'CREATE TABLE'` lists runs, attempts, journal, producers, candidates, verifications, check_results, trust_grants, declarations, applies; 0002 adds `qualification_records` only).
  - Per-run owner lock acquired at 4 non-test sites — HOLDS (`grep -rn 'AcquireOwner' internal cmd --include='*.go' | grep -v _test` → `ownerlock.go:23` def plus `pipeline.go:130`, `recover.go:47`, `cli/apply.go:67`, `cli/tui.go:185`).
  - `Append` hard-rejects envelopes with `schema_version != 1` — HOLDS (`internal/journal/journal.go:368`).
  - Read-only probe-first `Open` with `ErrSchemaTooNew` without writing — HOLDS (`internal/journal/journal.go:108-170`).
  - The receipt-file `schema_version = 2` is a separate namespace from the envelope — HOLDS (`internal/supervisor/apply.go:195` vs `journal.go:31-32`).

### Objectives

- **O1**: A user with v1 runs can migrate to the supervisor service with original IDs, billing posture and evidence intact, and can restore the pre-migration backup.
- **O2**: At no point do a legacy writer and the service writer both run; every run-scoped owner is drained, adopted or quarantined.
- **O3**: Historical v1 events stay decodable under the v1 contract forever; nothing silently reinterprets them.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | The supervisor service runtime itself (stream 2: singleton, IPC, intents, reservations) | The migration targets the shipped service interfaces exactly as specified; it defines no parallel control surface (I05, I18) |
| N2 | Stop ladders and one-pass recovery (stream 4) | Runs left `interrupted`/`quarantined` by migration reconcile under stream-4 rules later; migration never replays effects (I12) |
| N3 | New v2 record shapes beyond storage (stream 1) | Table columns store stream-1 records verbatim; no second vocabulary (I20, I23) |
| N4 | Router/scheduler intelligence, learner, export/erasure | Append-only retained history; purge needs its own contract (v2 §5.3) |

## Functional Requirements

Acceptance criteria use EARS. Tags in brackets name the invariants, gates and sections each one serves.

### FR-7 — Migration from the dogfood slice (v2 §5.2, G16, AT-41)

- **AC-7.1** [G16 (v2 §18.3)] The system shall migrate with additive numbered migrations only and shall never edit the released `0001_init.sql`.
- **AC-7.2** [AT-41 (v2 §18.2)] When importing legacy runs, the system shall preserve original IDs, billing posture and evidence as one-task runs; `subscription-declared` stays unverified and legacy `ready_for_review/unverified` displays as an unverified candidate, never promoted.
- **AC-7.3** [AT-41 (v2 §18.2)] The system shall acquire an exclusive migration/instance lock, prevent new legacy admissions, and drain or quarantine every run-scoped owner; no service writer starts while any legacy writer runs.
- **AC-7.4** [AT-41 (v2 §18.2), AT-42 (v2 §18.2)] The system shall keep a pre-migration backup with schema/build identities and a tested restoration procedure, and shall refuse a newer schema or downgrade without writing.
- **AC-7.5** [G16 (v2 §18.3)] If historical v1 events are read, then the system shall decode them under the v1 contract and shall never silently reinterpret an old `Decision` or status.
- **AC-7.6** [I13 (v2 §2)] When the user invokes migration, the system shall first show a preview (runs to import, owners to drain/quarantine, schema steps) computed with no agent credentials and no network service, and shall write nothing until the user confirms.

## Non-Functional Requirements

- **NFR-2** [AT-42 (v2 §18.2)] Critical rows shall use WAL with `synchronous=FULL`, one-transaction-per-mutation scope (preconditions, event, projections, outbox; no network I/O inside), a busy timeout named at design time, and integrity checks; the shipped SQLite engine's WAL-reset fix is verified from the actual binary.

## Definition of Done

- [ ] Additive migrations after 0003; `0001_init.sql` byte-identical (FR-7, AC-7.1)
- [ ] Legacy import preserving IDs/posture/evidence as one-task runs; unverified stays unverified (FR-7, AC-7.2)
- [ ] Exclusive migration lock; drain/adopt/quarantine of all 4 owner sites; no coexisting writers (FR-7, AC-7.3)
- [ ] Pre-migration backup with schema/build identities; tested restore; newer-schema/downgrade refusal without write (FR-7, AC-7.4)
- [ ] v1 events decode under v1 forever (FR-7, AC-7.5)
- [ ] Explicit `migrate` command with hermetic preview and confirmed apply (FR-7, AC-7.6)
- [ ] NFR-2 SQLite posture: WAL/FULL, tx scope, named busy timeout, integrity checks, engine fix verified from the built binary

## Open Questions

- **OQ-1**: New tables with import (default (a)) vs in-place evolution via additive columns. Decides: designer.
- **OQ-2**: User-facing migration trigger and preview surface (default (a): explicit `mythhelm migrate` with preview). Decides: designer.
- **OQ-8**: Shipped SQLite engine WAL-fix status — unverified until `SELECT sqlite_version()` asserted from the built binary. Decides: implementing task evidence.

## Dependencies

- **Prerequisite**: `specs/*/v2-contract-vocabulary/` (stream 1) — record types, `Envelope`, sequence/generation validators, error catalogue; OQ-9 decision (same `journal` table, v2 accepted post-migration).
- **Prerequisite**: `specs/*/supervisor-service/` (stream 2) — the service to migrate to: `internal/control` package, migration `0003_supervisor.sql` (`operations`, `reservations` tables), sole-writer transaction, lazy start.
- **Prerequisite**: MH-10 qualification-registry records — migration must preserve `qualification_records` and sequence after 0002.
- **Downstream**: `specs/*/supervised-stop-recover/` (stream 4) — runs in parallel; owns stop/recover of migrated runs.
- **Conflicting**: in-flight `specs/*/claude-strict-subscription/` touches `internal/cli/run.go`, which the migrate-command work neighbors; coordinate or land after.

## Impacted components

- `internal/journal/` + `internal/journal/migrations/` (new migrations after 0003; v2-accepting `Append`; v1 decode preserved)
- `internal/supervisor/` (legacy owner sites drained: `pipeline.go:130`, `recover.go:47`, `ownerlock.go`)
- `internal/cli/` (migration command; `apply.go:67`, `tui.go:185` owner sites drained)
- New migration package (design names it)
- `docs/decisions/` (ADR for the migration/import approach — a persistence choice)
