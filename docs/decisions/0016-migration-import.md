# 0016. Supervisor migration and legacy import

- Status: proposed
- Date: 2026-10-09

## Context

The supervisor service (ADR 0014) replaces the legacy per-run writer path,
and the v2 contract vocabulary (stream 1) replaces the v1 rows — but the
shipped state directories are v1. The migration must move each directory
exactly once to the singleton service without losing an accepted write,
without reinterpreting v1 evidence, and with a way back. The spec's design
§10 records seven decisions (D1–D7); this record fixes the migration and
import ones (OQ-1, OQ-2, the import mapping and the drain order) that later
work must not silently change. Each keeps its spec decision ID below.

## Decision

**OQ-1 → D2: new v2 tables beside v1, never altered v1.** The v2 records
land in new tables — `tasks`, `task_revisions`, `policies`, `grants`,
`migration_state` — created by additive migration
`internal/journal/migrations/0007_v2contracts.sql` (spec text says 0004;
0003–0006 were claimed first, so the migration shipped as 0007 with
`SchemaVersion` 7). The 10 v1 tables' DDL is frozen and v1 rows stay for
decode. Rejected: additive columns on the v1 tables (risks silent
reinterpretation of v1 rows) and a second store (competing task stores
violate I23).

**OQ-2 → D3: explicit `mythhelm migrate` with `--preview`/`--apply`.**
`migrate --preview` (bare `migrate` previews) prints the hermetic,
read-only plan — runs with postures, owner sites, schema steps, backup
target — as plain text or `--format jsonl`, and writes nothing
(`internal/migrate/preview/plan.go`, `internal/cli/migrate.go`,
registered in `internal/cli/dispatch.go`). `migrate --apply` previews
first and proceeds only with `--yes`, running Drain → Backup → Import →
Adopt and printing a receipt. Rejected: a first-run prompt (would migrate
implicitly mid-run and cannot show a preview for consent).

**Import mapping: one legacy v1 run becomes one v2 task at revision 1**
(`internal/migrate/import.go`, `ImportRun`, inside the caller's
`control.Mutate` — import never commits). The v2 task id is the earliest
attempt's legacy task id, or the run id when the run has no attempts;
legacy run/attempt ids are preserved verbatim as the v2 `RunID`/
`AttemptID`, and the `migration.imported` envelope carries them under the
deterministic event id `migration-imported-<runID>` (producer `migration`,
generation 0). Billing posture copies the v1 declaration word-for-word
and is never verified; the v2 record stores `sha256:<task_sha256>` as the
acceptance digest with empty prose fields, since v1 carries no
goal/deliverable text. Import never yields `accepted`: `completed`,
`ready_for_review` and `applying` all map to `candidate` (I07), and the
full v1 word map in `import.go` is frozen — an unknown word fails v1
decode and the import refuses `invalid_contract`, never reinterpreting.
v1 rows are read-only to import (SELECTs only); re-import returns
`revision_conflict` and writes nothing. Rejected: promoting completed
runs to `accepted` (v1 completion is an observation, not v2 evidence)
and verifying posture at import (I15 forbids manufacturing entitlement).

**Drain order: interactive writers, run execution, recovery**
(`internal/migrate/drain.go`, `Drain`; the order is pinned as
`preview.Owners`: `internal/cli/apply.go`, `internal/cli/tui.go`,
`internal/supervisor/pipeline.go`, `internal/supervisor/recover.go`).
Migration holds the instance lock plus the state-dir owner lock for its
whole run, so no service writer starts and no new legacy admission lands
mid-migration; the four `AcquireOwner` sites quiesce in order with a
bounded re-enumeration for admission racers, and a lock still held past
the deadline aborts with `ownership_unresolved` — nothing half-adopted.
`Drain` writes no ledger row, so the backup taken next holds every
accepted write including in-flight completions. Adoption reuses the live
launch identity and never relaunches; dead workers quarantine with
launch identity and evidence refs, and their `migration.quarantined`
envelopes persist post-backup from the `DrainReport` (ledger-native per
D4, so stream 4 reconciles them with ordinary machinery). Past
`previewed`, new runs, recovery and apply refuse with
`ownership_unresolved` naming the phase (guards in `pipeline.go`,
`recover.go`, `apply.go`, `tui.go`, wrapping `supervisor.ErrOwnership;
CLI exit 6). Rejected: a sidecar quarantine file (a second task store,
I23) and relaunching dead workers (I12 allows adopt-only-same-launch).

**Backup and restore: `VACUUM INTO` plus an identity sidecar**
(`internal/migrate/backup.go`). The pre-migration backup lands at
`<dir>/mythhelm.db.bak-migration-v<N>` with a `<copy>.json` sidecar
carrying schema version, build version, digest and timestamp — a name
distinct from the `<path>.bak-v<N>` copies `Open` writes during
migration. Restore verifies the digest and the backup's own
`user_version` first; a newer schema refuses `schema_too_new`, a
tampered copy refuses `invalid_contract`, and restore refuses to
overwrite an existing target — each writing nothing. `build_version` is recorded, never
compared (build strings are unordered); only a newer schema refuses.
Rejected: a new copy path (the tested `VACUUM INTO` mechanism already
exists) and silent overwrite of an existing backup.

**Phased v2 `Append` keeps its signature** (`internal/journal/journal.go`,
design §7, stream 1's OQ-9). Before phase `drained`, `schema_version ==
2` is rejected outright; at `drained` only migration-owned envelopes
(`migration.quarantined`, `migration.imported`) append; at `imported` or
later, ordinary v2 envelopes append too — every one through the stream-1
validators in check order with supervisor-allocated `run_sequence`.
`Append` delegates to the unexported transaction-scoped `appendTx`
core; db-level `Append` inside `Mutate` is forbidden (it would contend
with the open transaction), so the import and quarantine envelope writes
mirror its statements until a follow-up consolidates them onto an
exported wrapper. Duplicates ack nil; ID-reuse-with-different-content
conflicts surface at the caller layer. Rejected: a flag-day parameter
(would split every caller) and accepting v2 pre-migration (unknown
critical payloads must never be silently stored).

**Resume by markers, never by replay** (`internal/migrate/apply.go`).
The `migration_state` phase row names the failed step
(`previewed → drained → imported → adopted`); re-apply reuses the backup
by digest, skips imported runs by their `task_revisions` markers,
re-drains from `drained`, adopts directly from `imported`, and no-ops
from `adopted`. A post-drain surprise run aborts fatal. Rejected:
replaying effects (I12) and re-importing without markers.

## Consequences

- One ledger gains v2 tables; v1 decodes under v1 forever
  (`TestMigration0007CreatesV2Tables`, `TestMigration0001Untouched`,
  `TestV1DecodeByteIdentical`, `TestSchemaVersionIs7`).
- Declared posture and unverified evidence stay unverified across the
  import (`TestImportPreservesIDsPostureEvidence`,
  `TestImportReadyForReviewStaysCandidate`, `TestImportTwiceConflicts`,
  `TestImportEventIDReuseConflicts`, `TestImportCorruptV1Refuses`,
  `TestImportRollsBackAtomically`).
- One writer per directory holds across the transition
  (`TestDrainRefusesWhileLockHeld`, `TestDrainAdoptsSameLaunch`,
  `TestDrainWritesNothing`, `TestDrainQuarantinesDeadWorker`,
  `TestRunRefusedPastPreviewed`, `TestApplyRefusedDuringMigration`).
- The way back is tested and refusals write nothing
  (`TestBackupRestoreRoundTrip`, `TestRestoreNewerSchemaRefuses`,
  `TestRestoreDigestMismatchRefuses`, `TestBackupRefusesOverwrite`).
- The migration runs from an explicit, previewed, resumable command
  (`TestPreviewWritesNothing`, `TestPreviewHermetic`,
  `TestPreviewRefusesNewerSchema`, `TestApplyWithoutYesPreviewsFirst`,
  `TestBackupHoldsDrainedWrites`, `TestApplyPersistsQuarantinePostBackup`,
  `TestApplyResumesAfterFailure`, `TestMigrateE2EPackagedBinary`).
- The support matrix (`internal/migrate/SUPPORT.md`) pins exactly these
  rows; `TestSupportMatrixMatchesEvidence` fails on a new deliverable,
  a new export, or any `live-qualified` claim. The NFR-2 posture row is
  explicitly `blocked` until Task 7 ships (OQ-8 still open), and the
  legacy per-run path is drained and refused, not removed — full removal
  waits for stream 4.
