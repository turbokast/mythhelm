## Supervisor Migration — Tasks

### Dependencies

- Prerequisite specs (both must ship before any task here starts): `specs/*/v2-contract-vocabulary/` provides `v2contract` record types, `Envelope`, sequence/generation validators, the error catalogue and the OQ-9 decision; `specs/*/supervisor-service/` provides the `internal/control` package, migration `0003_supervisor.sql` (`operations`, `reservations`), `Mutate`, and lazy start. Parallel with `specs/*/supervised-stop-recover/` (disjoint files).
- Parallel groups: Tasks 2, 3, 4, 5 may run in parallel once Task 1 lands (disjoint Files; Task 3 consumes Task 1's tables, the rest consume only prerequisite-spec interfaces). Task 6 depends on Tasks 2–5. Task 7 depends on Tasks 5 and 6 (its assertions scan their code). Task 8 depends on Task 6.
- **Gates for every task.** `gofmt -w` on changed Go files first; then `gofmt -l .`, `go vet ./...` (plus `GOOS=windows go vet` and `GOOS=darwin go vet` for platform files), `go test -race ./...`, `go mod tidy -diff`, `scripts/ci/check-public-hygiene.sh`. Files existing or an agent reporting success is not completion — cited gate output is.
- **Completion convention.** Append ` ✅ COMPLETED` to the task heading and add `Status` (`✅ Completed — …; PR #<n>`), `Implementation` (with commit SHAs), `Spec deviations` (`None`, or each deviation with its reason) and `Files modified`, keeping every original field.
- **Commits**: signed off (`git commit -s`); the ADR decision (D7) ships with its task.

---

## Implementation Tasks

### Task 1 — Migration 0004 v2 contract tables

- **Domain/agent**: go-implementer
- **Budget**: complex (persistence schema migration)
- **Change**: Add additive migration `0004_v2contracts.sql` with the new v2 tables and `migration_state` phase row, sequencing after stream-2's 0003, so import and phased `Append` have tables to write.
- **Files**:
  - `internal/journal/migrations/0004_v2contracts.sql`
  - `internal/journal/journal.go` (schema-version bump 3→4 only; stream-2 Task 5 owns the 2→3 bump)
  - `internal/migrate/phase.go` (the `migrate.Phase` constants this task produces; creates the package root)
  - `internal/journal/migrate_tables_test.go`
- **Produces**: `tasks`, `task_revisions`, `policies`, `grants`, `migration_state` tables; `migrate.Phase` constants (`not_started`, `previewed`, `drained`, `imported`, `adopted`)
- **Acceptance**:
  - `TestMigration0004CreatesV2Tables`: a v1+0002+0003 database migrates; the five new tables exist and the v1 tables' DDL is unchanged (`SELECT sql FROM sqlite_master` for the 10 v1 tables matches the pre-migration snapshot).
  - `TestMigration0001Untouched`: SHA-256 of `0001_init.sql` equals the committed hash; any edit fails the test.
  - `TestMigrationStateStartsNotStarted`: fresh migration writes the `not_started` phase row with schema/build identities.
  - `TestSchemaVersionIs4`: `SchemaVersion == 4` after 0004 and `OpenReadOnly` succeeds on a 0004 database.
- **Test plan**: temp databases migrated from a checked-in v1 golden fixture; DDL snapshot comparison.
- **Invariants touched**: I23 (v2 §5.1: single ledger gains v2 tables, no second store); G16 (additive only).

### Task 2 — Append accepts v2 envelopes post-migration

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 1
- **Change**: Extend `Append` to accept `schema_version == 2` envelopes under one phase rule — migration-owned envelopes (`migration.quarantined`, `migration.imported`) at `drained`, ordinary v2 envelopes at `imported` or later, all v2 rejected before `drained` — through the stream-1 validators in check order, keeping the signature and pre-migration behavior identical.
- **Files**:
  - `internal/journal/journal.go`
  - `internal/journal/append_v2_test.go`
- **Produces**: `Append` v2 acceptance phased by `migration_state`; unexported tx-scoped `appendTx(ctx, tx, ev, project)` core that `Append` delegates to and the Import step calls inside `Mutate`
- **Acceptance**:
  - `TestAppendAcceptsV2PostMigration`: a valid `v2contract.Envelope` appends post-`imported` with supervisor-allocated `run_sequence`; duplicates ack with nil error by `event_id`, gaps and stale generations rejected via the stream-1 sentinels.
  - `TestAppendAcceptsMigrationOwnedAtDrained`: `migration.quarantined` and `migration.imported` envelopes append at phase `drained`.
  - `TestAppendRejectsOrdinaryV2AtDrained`: an ordinary v2 envelope at phase `drained` is rejected.
  - `TestAppendRejectsV2PreMigration`: the same envelope pre-`drained` is rejected (unknown critical payloads are never silently stored).
  - `TestV1DecodeByteIdentical`: a golden v1 journal's projections are byte-identical before and after the change (AC-7.5).
- **Test plan**: table-driven envelopes (valid, duplicate, gap, stale generation, v2-pre-migration); golden v1 journal fixture with a `Decision` and every v1 status word.
- **Invariants touched**: I23 (v2 §4.3: contiguity, idempotent duplicates, generation fencing for v2); G16 (v1 decodes under v1 forever).

### Task 3 — Legacy import as one-task runs

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 1
- **Change**: Implement `ImportRun` mapping each legacy v1 run to one v2 task revision 1 with preserved IDs, word-for-word billing posture and untouched v1 evidence, inside the caller's `Mutate` transaction.
- **Files**:
  - `internal/migrate/import.go`
  - `internal/migrate/import_test.go`
- **Produces**: `migrate.ImportRun(ctx context.Context, tx *sql.Tx, runID string) (v2contract.TaskRevision, error)`; `migrate.ImportOptions`; `migrate.ImportResult`
- **Acceptance**:
  - `TestImportPreservesIDsPostureEvidence`: a v1 run with known IDs imports with identical run/attempt IDs, `subscription-declared` posture copied verbatim (never verified), and v1 journal rows untouched.
  - `TestImportReadyForReviewStaysCandidate`: legacy `ready_for_review` with unverified evidence imports as `candidate` TaskState; a variant promoting to `accepted` fails the test.
  - `TestImportTwiceConflicts`: re-importing returns `revision_conflict` and writes nothing new.
  - `TestImportEventIDReuseConflicts`: an existing journal envelope with the same `event_id` but different contents returns `revision_conflict` and commits no v2 rows, import marker or envelope.
  - `TestImportCorruptV1Refuses`: a v1 row failing v1 decode returns `invalid_contract`; nothing is reinterpreted.
  - `TestImportRollsBackAtomically`: when the `migration.imported` append fails (injected ledger fault), `ImportRun` returns `persistence_unavailable` and the caller's transaction rolls back all three writes — no v2 rows, no import marker and no envelope remain.
- **Test plan**: fixture v1 state dirs; golden `TaskRevision` JSON per posture word.
- **Invariants touched**: I15 (v2 §2: declared posture never promoted); I07 (acceptance needs independent evidence); I09 (unverified stays unverified); I12 (no replay of effects).

### Task 4 — Drain, adopt, quarantine

- **Domain/agent**: go-implementer
- **Budget**: complex (process ownership, concurrency across 4 lock sites)
- **Depends on**: Task 1
- **Change**: Implement `Drain` refusing new admissions (via the held migration lock) and quiescing the 4 `AcquireOwner` sites in order without writing any ledger row, reporting adopted and quarantined runs for post-backup persistence, with no service writer starting mid-drain.
- **Files**:
  - `internal/migrate/drain.go`
  - `internal/supervisor/pipeline.go` (admission/drain hooks only)
  - `internal/supervisor/recover.go` (drain hook only)
  - `internal/cli/apply.go` (migration refusal only)
  - `internal/cli/tui.go` (migration refusal only)
  - `internal/migrate/drain_test.go`
- **Produces**: `migrate.Drain(ctx context.Context, db *sql.DB) (DrainReport, error)`; `migrate.DrainReport`
- **Acceptance**:
  - `TestDrainRefusesWhileLockHeld`: with a run lock held past the deadline, `Drain` returns `ownership_unresolved`, nothing is half-adopted, and no service writer starts (asserted via the instance lock still free for migration).
  - `TestDrainAdoptsSameLaunch`: a live worker with matching launch identity adopts; its launch identity is reused, never relaunched (launcher fake shows no new spawn; the adopted run keeps its pid).
  - `TestDrainWritesNothing`: `Drain` leaves the ledger bytes unchanged (journal hash identical before and after); the `DrainReport` carries the outcomes.
  - `TestDrainQuarantinesDeadWorker`: a dead worker's run is reported quarantined with launch identity and evidence refs; no new launch spawns (the `migration.quarantined` envelope is persisted post-backup, asserted in Task 6).
  - `TestApplyRefusedDuringMigration`: `apply` past `previewed` returns `ownership_unresolved` naming the phase.
  - `TestRunRefusedPastPreviewed`: starting a new run past `previewed` returns `ownership_unresolved` naming the phase; in-flight runs drain instead.
- **Test plan**: lock-holding fixtures per owner site; dead-pid simulation; ledger assertions on quarantine envelopes.
- **Invariants touched**: I05 (one writer per dir; no coexisting writers); I18 (one process owner); I06 (stop unconfirmed until reconciled); I12 (reconcile before retry; adopt-only-same-launch-identity).

### Task 5 — Backup and restore with downgrade refusal

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 1
- **Change**: Implement `Backup`/`Restore` around `VACUUM INTO` with a schema/build-identity sidecar, integrity checks, and newer-schema/downgrade refusal that writes nothing.
- **Files**:
  - `internal/migrate/backup.go`
  - `internal/migrate/backup_test.go`
- **Produces**: `migrate.Backup(ctx context.Context, db *sql.DB, dir string) (BackupInfo, error)`; `migrate.Restore(ctx context.Context, info BackupInfo, dir string) error`; `migrate.BackupInfo`
- **Acceptance**:
  - `TestBackupRestoreRoundTrip`: v1 fixture → backup → migrate → restore yields byte-identical v1 projections; sidecar carries schema version, build version, digest, timestamp.
  - `TestRestoreNewerSchemaRefuses`: a backup sidecar naming a newer schema returns `schema_too_new` and the target dir's SHA-256 is unchanged.
  - `TestRestoreDigestMismatchRefuses`: a tampered backup returns `invalid_contract` and writes nothing.
  - `TestBackupRefusesOverwrite`: an existing backup at the target returns `invalid_contract`; the existing bytes are unchanged.
- **Test plan**: fixture state dirs with hashed contents; tampered and future-version sidecars.
- **Invariants touched**: I23 (v2 §5.2: backup + tested restore); AT-42 (refusal without write).

### Task 6 — `mythhelm migrate` with hermetic preview

- **Domain/agent**: go-implementer
- **Budget**: complex (CLI surface, hermetic preview, resumable apply, packaged-binary e2e)
- **Depends on**: Task 2, Task 3, Task 4, Task 5
- **Change**: Add the `migrate --preview`/`--apply` CLI surface: read-only hermetic plan first, then Drain → Backup → Import → Adopt with a receipt (quiesce before the backup so it holds every accepted write; quarantine envelopes persisted post-backup from the `DrainReport`), resumable after step failure.
- **Files**:
  - `internal/migrate/preview/plan.go`
  - `internal/migrate/apply.go`
  - `internal/cli/migrate.go`
  - `internal/cli/dispatch.go` (register `migrate`; stream 2 ships first, so this lands on top of its `supervisor` registration)
  - `internal/cli/migrate_test.go`
  - `tests/migrate_e2e_test.go`
- **Produces**: `preview.PreviewPlan(ctx context.Context, db *sql.DB) (Plan, error)`; `migrate.Apply(ctx context.Context, db *sql.DB, plan preview.Plan) (DrainReport, error)`; `preview.Plan`
- **Acceptance**:
  - `TestPreviewWritesNothing`: `--preview` exits 0 and the state dir hash is unchanged; output names every run, owner site, schema step and backup target (plain and `--format jsonl`).
  - `TestPreviewHermetic`: `go list -deps ./internal/migrate/preview` shows no network packages; the plan output is byte-identical under the full environment and under an emptied environment (fails if any env var changes the plan).
  - `TestPreviewRefusesNewerSchema`: previewing a database newer than the binary returns `schema_too_new` and writes nothing (state dir hash unchanged).
  - `TestApplyWithoutYesPreviewsFirst`: `--apply` without `--yes` prints the preview and writes nothing.
  - `TestBackupHoldsDrainedWrites`: an in-flight run that completes during the drain quiesce is present in the backup (restore after a failed apply shows its writes); quarantine and import envelopes land after the backup.
  - `TestApplyPersistsQuarantinePostBackup`: a quarantined run from the `DrainReport` gains its `migration.quarantined` envelope during apply, after the backup step.
  - `TestMigrateE2EPackagedBinary`: fixture v1 dir → preview → apply → v2 assertions → restore → byte-identical v1 projections, against the built binary.
  - `TestApplyResumesAfterFailure`: a killed mid-import apply resumes without duplicating imported runs (import markers, never replay).
- **Test plan**: `cli.Main` with temp state dirs; packaged-binary e2e; kill-injection resume test.
- **Invariants touched**: I13 (v2 §2: preview needs no credentials or network); I12 (resume by markers, never replay); I23 (adopt hands the ledger to the service once).

### Task 7 — NFR-2 SQLite posture proof

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 5, Task 6
- **Change**: Pin the NFR-2 posture with tests: pragma string, tx scope, named busy timeout, integrity checks on backup/restore, and the OQ-8 engine-version assertion from the built binary. (Needs Task 5's Backup/Restore for the integrity test and Task 6's full package for the Mutate-scope structural assertion.)
- **Files**:
  - `internal/journal/pragmas_test.go`
  - `internal/migrate/nfr2_test.go`
- **Acceptance**:
  - `TestPragmasPinned`: the open pragma string contains `journal_mode(WAL)`, `synchronous(FULL)`, `busy_timeout(5000)`; any drift fails.
  - `TestMigrateWritesRunInMutate`: every ledger-mutating write in `internal/migrate` runs inside `control.Mutate` (structural: no `sql.Open` and no `*sql.DB` `Exec` outside it; read-only `Query` and the `preview` subpackage are excluded — preview hermeticity is pinned by `TestPreviewHermetic`; `Backup`'s single `VACUUM INTO` `Exec` is excluded — a file-creating copy, not a ledger mutation; `tx.*` calls inside the `Mutate` fn are legitimate and allowed; the test fails on a direct write).
  - `TestIntegrityCheckOnBackupRestore`: a corrupt-source backup and a corrupt-copy restore each abort with `persistence_unavailable`.
  - `TestSQLiteEngineHasWALFix`: `SELECT sqlite_version()` from the built binary is ≥3.51.3 or a backport pinned in `scratchpad.md`; records the OQ-8 verdict.
- **Test plan**: assertion tests on the built binary and package structure; corrupt-page fixtures.
- **Invariants touched**: AT-42 (v2 §18.2: failure handling + tested restore posture).

### Task 8 — Migration ADR and support rows

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 6
- **Change**: Record the migration/import ADR and publish the spec's support-matrix rows, describing only the shipped behavior. (Accepted single task spanning test + docs: follows the stream-1/2 support-matrix precedent; the matrix and its assertion ship together.)
- **Files**:
  - `docs/decisions/NNNN-migration-import.md`
  - `internal/migrate/SUPPORT.md`
  - `internal/migrate/support_test.go`
- **Acceptance**:
  - `TestSupportMatrixMatchesEvidence`: the matrix lists exactly the shipped rows (0004 tables, import, drain, backup/restore, migrate CLI, NFR-2 proof), each `fixture-tested` or `blocked`, with zero `live-qualified` claims; adding behavior without a row fails.
  - The ADR names the OQ-1/OQ-2 decisions, the import mapping, and the drain order with file cites; review confirms no claim the code does not keep.
- **Test plan**: matrix-vs-deliverables assertion following the stream-1/2 precedent; reviewer read of the ADR against the merged code.
- **Invariants touched**: I14 (v2 §2: versioned evidence or explicitly unsupported); None beyond evidence (docs describe shipped behavior).
