## V2 Contract Vocabulary — Requirements

> Ships the canonical v2 contract vocabulary of MH-21 as reviewed, machine-checkable Go artefacts: the §4.1 record types, the §6.1–§6.2 transition tables, the §4.5 error catalogue, the §4.3 event-v2 envelope contract, and golden fixtures with a support matrix. No runtime, migration, IPC or stop/recover behavior ships here; those belong to the sibling specs. Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it.

## Context

- **Backlog card**: MH-21
- **Issue**: [#127](https://github.com/turbokast/mythhelm/issues/127) (records planned work; no live-test authority)
- **Epic plan**: `specs/*/v2-contracts-supervisor/plan.md` (stream 1 of 4). FR/NFR numbers below follow the epic's numbering for traceability; FR-5–FR-8 and NFR-2–NFR-3 belong to the sibling specs.
- **Problem**: The journal schema is v1 with no v2 task/policy/authority contract tables and no v2 contract event payloads, so nothing can yet express a `TaskRevision`, a revisioned policy, or an idempotent control intent. Every consumer (supervisor service, migration, stop/recover, MH-22 protected acceptance) needs one reviewed vocabulary first (epic plan: contracts are pure artefacts; spec 1 unblocks everything).
- **Grounding** (verified 2026-10-08 against the working tree):
  - No v2 contract package or `TaskRevision` exists — HOLDS (`ls internal/` shows 14 packages, none for contracts; `grep -rn "TaskRevision\|task_revision" internal/ cmd/ adapters/ --include='*.go' | grep -v _test` prints nothing).
  - `journal.Event` is the v1 envelope with int64 sequences — HOLDS (`internal/journal/journal.go:80-94`).
  - `Append` implements idempotent event_id, contiguity and generation fencing for v1 — HOLDS (`internal/journal/journal.go:281-345`; duplicates acked by `event_id` at lines 291-298, before the sequence check).
  - v1 run/attempt machines lack `planning`/`integrating`, task revisions, and `reserved`/`waiting_*` states — HOLDS (`internal/supervisor/state.go:41-96`; `runTransitions`/`attemptTransitions` enumerate the smaller tables).
  - `internal/qualify` is the closest v2-contract precedent — HOLDS (`KeyHash` at `internal/qualify/qualify.go:130`, `CanonicalDigest` at :147, `DecodeRecord` at :176).
  - The post-apply receipt-file `schema_version = 2` is a separate namespace from the envelope — HOLDS (uniform envelope v1 per `internal/journal/journal.go:31-32`; receipt-file write at `internal/supervisor/apply.go:195`).
  - "26 required error codes" (phase-1 findings) — REFUTED: v2 §4.5 lists 24 codes (counted from source); this spec builds on the counted 24.

### Objectives

- **O1**: Implementers of the MH-21 sibling specs and MH-22 work from one reviewed v2 contract vocabulary with machine-checkable transitions, errors and fixtures.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Supervisor singleton, IPC, reservations runtime (FR-5–FR-6, `supervisor-service`) | The `operation_id`/`expected_revision` envelope shape (v2 §4.2) and reservation record ship here |
| N2 | Migrations, legacy import, drain, backup/restore (FR-7, `supervisor-migration`) | v1 decode contract untouched; the OQ-9 coexistence decision is recorded here |
| N3 | Stop ladders and recovery behavior (FR-8, `supervised-stop-recover`) | The lifecycle tables and error codes it consumes ship here |
| N4 | JSON Schema artefacts for the records | Go types with strict decode plus golden fixtures are the machine-checkable contract (I14); JSON Schema files are an explicit MH-21 follow-up (no external consumer exists yet) |
| N5 | Frame-limit and generation-fence enforcement in running code | NFR-1 limit constants and pure validators ship here; enforcement points are in specs 2–4 (G04) |

## Functional Requirements

Acceptance criteria use EARS. Tags name the version-qualified invariants, gates and sections each one serves.

### FR-1 — Canonical contract types (v2 §4.1–§4.2, I20, I23)

- **AC-1.1** [I20 (v2 §2), I23 (v2 §2), v2 §4.1, v2 §4.2] The system shall define `Run`, `TaskRevision`, `Attempt`, `RoutingDecision`, `DesignDecision`, `Artifact`, `Observation`, `Verification`, `Handoff`/`ContextManifest`, `Message`, `Grant`/`EffectIntent`, `Reservation`, `PolicyVersion`, `Experiment` records with the §4.1 identity and contents, as Go types with snake_case JSON/TOML tags and `schema_version = 2`.
- **AC-1.2** [I20 (v2 §2)] When a material contract or plan change occurs, the system shall require a new immutable revision rather than an in-place rewrite of the admitted contract.
- **AC-1.3** [G16 (v2 §18.3), v2 §4.2] The system shall version new canonical event payloads as `schema_version = 2`, keeping schema v1 as a separately decoded historical contract; the contract version shall be distinguishable by type name from the post-apply receipt-file `schema_version = 2`.
- **AC-1.4** [I09 (v2 §2), v2 §4.2] The system shall keep capabilities (`supported | unsupported | unknown`), qualification progress, and admission verdicts (`eligible | blocked | unsupported`) as distinct vocabularies, each value carried with evidence, scope and expiry.

### FR-2 — Machine-checkable lifecycles (v2 §6.1–§6.2, G04)

- **AC-2.1** [G04 (v2 §18.3), v2 §6.1, v2 §6.2] The system shall reject any run/task/attempt transition outside the §6.1/§6.2 tables, with the offending transition named in the error.
- **AC-2.2** [I06 (v2 §2), v2 §6.1] If local mutation ownership is unresolved, then the system shall not enter a terminal run state, using `interrupted` or `blocked` instead.
- **AC-2.3** [I12 (v2 §2), v2 §6.2] When reconciling an `interrupted`/`quarantined` attempt, the system shall require the same launch identity, and no reconcile disposition shall authorize relaunching native tools, prompts or external effects by replay.

### FR-3 — Error and retry contract (v2 §4.5, G04)

- **AC-3.1** [G04 (v2 §18.3), v2 §4.5] The system shall carry `code`, `owner`, `operation_id`, object/revision, retry disposition and safe next action on control errors, with code — not message — driving transitions.
- **AC-3.2** [G04 (v2 §18.3), v2 §4.5] If an adapter reports a failure, then the system shall map it to a required §4.5 code with a disposition that never widens authority, with adapter detail namespaced.

### FR-4 — Event v2 and ingestion contract (v2 §4.3, I23)

- **AC-4.1** [I23 (v2 §2), v2 §4.3] The system shall define the v2 envelope with supervisor-allocated `run_sequence`, contiguous per-producer sequence validation with idempotent duplicates, and stale-generation fencing from canonical state.
- **AC-4.2** [I12 (v2 §2), v2 §4.3] If a late observation arrives on an old generation, then the system shall classify it as quarantined evidence at most, never as an accepted result.

### FR-9 — Contract evidence and compatibility matrix (I14)

- **AC-9.1** [I14 (v2 §2)] The system shall publish golden JSON/TOML fixtures and a machine-checkable transition/event contract with an error-code catalogue, covering malformed/unknown authority keys, ID/sequence round-trips and null measurements.
- **AC-9.2** [I14 (v2 §2)] The system shall publish each support-matrix row as documented, fixture-tested, blocked or live-qualified, with unresolved auth/OS facts left blocked rather than guessed.

## Non-Functional Requirements

- **NFR-1** [G04 (v2 §18.3), v2 §4.3] The contract shall define the §4.3 default limits as exact constants: 1 MiB encoded frame, nesting depth 64, bounded artifact references (128 per frame).
- **NFR-4** [I13 (v2 §2)] Contract validation and fixtures shall run with no agent credentials and no network service; the contract package's dependency closure shall contain no network code.

## Definition of Done

- [ ] Go record types for all §4.1 records (minus optional `Release`) with `schema_version = 2` and snake_case JSON/TOML tags (FR-1)
- [ ] Transition tables enforced in code; illegal transitions rejected with names (FR-2)
- [ ] Error catalogue with the 24 required codes and dispositions; no authority-widening retry (FR-3)
- [ ] v2 envelope plus pure sequence/generation/quarantine validators; OQ-9 decided (FR-4)
- [ ] Golden fixtures, error-code catalogue, support matrix; v1 decode untouched (FR-9)
- [ ] Limit constants exact; contract tests hermetic with no network dependency (NFR-1, NFR-4)

## Open Questions

- **OQ-9** (v2 envelope coexistence): same journal table, envelope `schema_version = 2` accepted post-migration, v1 rows decoded under v1 forever. Decided at design (see `design.md` D7); implementation of the acceptance change belongs to `supervisor-migration`.

## Dependencies

- **Prerequisite**: none. This spec creates new files only and imports no sibling package.
- **Downstream**: `supervisor-service`, `supervisor-migration`, `supervised-stop-recover` consume these contracts; MH-22 protected acceptance adopts `TaskRevision` verbatim (epic plan Q-14).
- **Conflicting**: none. In-flight `claude-strict-subscription` edits `internal/admission/*` and `internal/qualify/registry.go`; this spec touches neither (new package only).

## Impacted components

- `internal/v2contract/` (new package: records, lifecycles, error catalogue, envelope, fixtures, support matrix). No existing file is edited; no `go.mod` change (stdlib plus the already-required `BurntSushi/toml` in tests only).
