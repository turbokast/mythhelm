## Supervisor Migration — Tasks

### Dependencies

- Prerequisite specs (both must ship before any task here starts): `specs/*/v2-contract-vocabulary/` provides `v2contract` record types, `Envelope`, sequence/generation validators, the error catalogue and the OQ-9 decision; `specs/*/supervisor-service/` provides the `internal/control` package, migration `0003_supervisor.sql` (`operations`, `reservations`), `Mutate`, and lazy start. Parallel with `specs/*/supervised-stop-recover/` (disjoint files).
- Parallel groups: Tasks 2, 3, 4, 5 may run in parallel once Task 1 lands (disjoint Files; Task 3 consumes Task 1's tables, the rest consume only prerequisite-spec interfaces). Task 6 depends on Tasks 2–5. Task 7 depends on Tasks 5 and 6 (its assertions scan their code). Task 8 depends on Task 6.
- **Gates for every task.** `gofmt -w` on changed Go files first; then `gofmt -l .`, `go vet ./...` (plus `GOOS=windows go vet` and `GOOS=darwin go vet` for platform files), `go test -race ./...`, `go mod tidy -diff`, `scripts/ci/check-public-hygiene.sh`. Files existing or an agent reporting success is not completion — cited gate output is.
- **Completion convention.** Append ` ✅ COMPLETED` to the task heading and add `Status` (`✅ Completed — …; PR #<n>`), `Implementation` (with commit SHAs), `Spec deviations` (`None`, or each deviation with its reason) and `Files modified`, keeping every original field.
- **Commits**: signed off (`git commit -s`); the ADR decision (D7) ships with its task.

---

## Implementation Tasks

### Task 1 — Migration 0004 v2 contract tables ✅ COMPLETED

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
- **Status**: ✅ Completed — additive migration 0007 creates the five v2 tables with `migration_state` starting at `not_started`; PR #277.
- **Implementation**: Renumbered 0004→0007 (origin/main already holds 0001–0006); `go:embed` picks the file up with no registry change. Commits 1be31e6, 30a2ecd.
- **Spec deviations**:
  - Migration ships as `0007_v2contracts.sql` with `SchemaVersion` 7, not 0004/4 as the task text says: later specs claimed 0003–0006 first. Test names follow (`TestMigration0007CreatesV2Tables`, `TestSchemaVersionIs7`); later tasks read "0004" as 0007.
  - Extra files beyond the task Files list: `internal/journal/journal_test.go`, `internal/journal/qualification_test.go` and `internal/journal/evaluator_test.go` pin the schema version (incl. `user_version`) and were bumped 6→7.
  - `TestMigration0001Untouched` already exists in `internal/journal/ledger_test.go` (added by budget-ledger-s1) and was reused, not duplicated.
- **Files modified**: `internal/journal/migrations/0007_v2contracts.sql`, `internal/journal/journal.go`, `internal/migrate/phase.go`, `internal/journal/migrate_tables_test.go`, `internal/journal/journal_test.go`, `internal/journal/qualification_test.go`, `internal/journal/evaluator_test.go`, `specs/in-progress/supervisor-migration/tasks.md`, `specs/in-progress/supervisor-migration/handoff.md`, `specs/in-progress/supervisor-migration/scratchpad.md`.

### Task 2 — Append accepts v2 envelopes post-migration ✅ COMPLETED

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
- **Status**: ✅ Completed — `Append` accepts phased v2 envelopes through the stream-1 validators with v1 behavior and decodes byte-identical; PR #286.
- **Implementation**: `Append` delegates to unexported `appendTx` (v1 checks unchanged; v2 branch validates, phase-gates, then CheckDuplicate/CheckGeneration/CheckSequence into a shared `storeTx` tail); v2 failures carry a valid `ControlError` plus the stream-1 cause. V1 golden captured from base `148084b`. Commits 883feb4, 848c270.
- **Spec deviations**:
  - No behavior deviation. Phase refusals also wrap `journal.ErrInvalidEvent`, keeping the existing `TestAppendValidatesEnvelope` "schema version" contract green without touching that file.
  - No behavior deviation. Phase words are string literals in `internal/journal`, not imported `migrate.Phase` constants: `internal/migrate` reaches back into `journal` for the import path, so the import would cycle.
- **Files modified**: `internal/journal/journal.go`, `internal/journal/append_v2_test.go`, `specs/in-progress/supervisor-migration/tasks.md`, `specs/in-progress/supervisor-migration/handoff.md`, `specs/in-progress/supervisor-migration/scratchpad.md`.

### Task 3 — Legacy import as one-task runs ✅ COMPLETED

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
- **Status**: ✅ Completed — `ImportRun` imports one legacy v1 run as a v2 task revision 1 with preserved IDs, verbatim posture and untouched v1 evidence; PR #287.
- **Implementation**: Task id is the earliest attempt's legacy task id (run id when attempt-less); import never yields `accepted` (completed/ready_for_review → `candidate`, I07); envelope statements mirror `Journal.Append` until Task 2's `appendTx` lands. Red-first: stub → `TestImportPreservesIDsPostureEvidence` FAIL; mutants drop-marker-check → `TestImportTwiceConflicts` FAIL, promote-to-accepted → candidate tests FAIL; restored → PASS. Commit 0824820.
- **Spec deviations**:
  - The `migration.imported` envelope is appended with statements mirroring `Journal.Append` rather than Task 2's `appendTx`: Task 2 is parallel and unmerged, and Task 3's file list cannot create it. `import.go` names the seam; the statements stay valid after Task 2 lands.
- **Files modified**: `internal/migrate/import.go`, `internal/migrate/import_test.go`, `specs/in-progress/supervisor-migration/tasks.md`, `specs/in-progress/supervisor-migration/handoff.md`, `specs/in-progress/supervisor-migration/scratchpad.md`.

### Task 4 — Drain, adopt, quarantine ✅ COMPLETED

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
- **Status**: ✅ Completed — Drain quiesces the four legacy owner sites into a drain/adopt/quarantine report with no ledger write, and every owner site refuses new work while migration holds the state directory or the ledger is past previewed; PR #292.
- **Implementation**: Drain holds the instance lock plus the state-dir owner lock as the migration lock, waits each run's owner lock to the context/default deadline, and verifies launch identity (token, journaled observations, live pid) before adopting; hook refusals wrap supervisor.ErrOwnership (CLI exit 6) while Drain returns v2 ControlError codes. Commits a0b675d, b6ae6e8.
- **Spec deviations**:
  - `internal/journal/migrate_tables_test.go` (outside Files): phase literals instead of `migrate.Phase` — the import cycled once migrate imports control/supervisor/journal as design §2 mandates; bound back by `TestPhaseMatchesMigrationVocabulary`.
  - The migration lock is the state directory's owner lock held alongside the instance lock (same flock mechanism, hermetic under parallel package suites) rather than the per-user instance lock alone.
  - No checkpoint-stop signalling: in-flight runs finish their admitted work or abort at the deadline; no acceptance criterion requires forced early stops.
  - The TUI palette Apply call site shares `apply.go`'s guard helper and is covered by construction (no headless path drives the palette action itself).
  - `internal/supervisor/migration_guard_test.go` (outside Files): pins the held-lock contract of the new `checkMigrationClear` guard (lock held on success, phase read under the lock, every refusal releases it); lives with the guard rather than in `drain_test.go`.
  - `internal/supervisor/pipeline_test.go` (outside Files): extends `TestSeedFailureFailsClosed` with a release-on-seed-failure assertion (review round 2) — the seed-failure exit lives in `Run`, so the pin belongs to the existing `Run` seed test.
- **Files modified**: `internal/migrate/drain.go`, `internal/migrate/drain_test.go`, `internal/supervisor/pipeline.go`, `internal/supervisor/recover.go`, `internal/supervisor/migration_guard_test.go`, `internal/supervisor/pipeline_test.go`, `internal/cli/apply.go`, `internal/cli/tui.go`, `internal/journal/migrate_tables_test.go`, `specs/in-progress/supervisor-migration/tasks.md`, `specs/in-progress/supervisor-migration/handoff.md`, `specs/in-progress/supervisor-migration/scratchpad.md`.

### Task 5 — Backup and restore with downgrade refusal ✅ COMPLETED

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
- **Status**: ✅ Completed — Backup/Restore around `VACUUM INTO` with a schema/build-identity sidecar, integrity checks and write-free newer-schema/downgrade refusal; PR #285.
- **Implementation**: `mythhelm.db.bak-migration-v<N>` target beside Open's `.bak-v<N>`; journal constants mirrored (import cycle) with a pin test; every failure a validated `ControlError`. Commit bb16b26.
- **Spec deviations**:
  - Backup target named `mythhelm.db.bak-migration-v<N>` with a `<copy>.json` sidecar: the design pins no filename, and Open already writes `<path>.bak-v<N>` during migration — a distinct name keeps the two from colliding.
  - `backup.go` mirrors `journal.DBName`/`journal.SchemaVersion` as unexported constants instead of importing `internal/journal`: journal's tests import `internal/migrate`, so the import would cycle in test builds. `TestBackupPinsMatchJournal` fails on drift.
  - `Restore` drops stale `-wal`/`-shm` sidecars and installs via write-then-rename: both follow from "copies back" onto a live state dir and keep a failed restore from leaving a half-written ledger.
- **Files modified**: `internal/migrate/backup.go`, `internal/migrate/backup_test.go`, `specs/in-progress/supervisor-migration/tasks.md`, `specs/in-progress/supervisor-migration/handoff.md`, `specs/in-progress/supervisor-migration/scratchpad.md`.

### Task 6 — `mythhelm migrate` with hermetic preview ✅ COMPLETED

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
- **Status**: ✅ Completed — `migrate --preview`/`--apply` CLI with hermetic read-only plan, Drain → Backup → Import → Adopt receipt flow and marker-based resume; PR #297.
- **Implementation**: `PreviewPlan` derives runs/postures, owner sites, pending schema steps and the backup target from a read-only handle (tolerates pre-v2 schemas; refuses newer with `schema_too_new`); `Apply` reuses `Drain`/`Backup`/`ImportRun`, persists `migration.quarantined` envelopes post-backup, records `previewed → drained → imported → adopted`, reuses the backup by digest on resume and aborts fatal on post-drain surprise runs; CLI prints plan + receipt (plain/jsonl) with exit mapping (newer-schema → 2, ownership → 6). Fixtures are true v1 databases (0001 only). Commits 18532a5, df8a3a5.
- **Spec deviations**:
  - `preview.Plan.Runs` uses the preview-local `Run` type (same JSON as `migrate.ImportResult`): importing `internal/migrate` from `preview` would cycle, since `Apply` consumes the plan.
  - Quarantine envelopes append with statements mirroring `appendTx` (same reason as Task 3's `appendImported` deviation): Task 2's `appendTx` is unexported and unreachable cross-package, and db-level `Append` inside `Mutate` would contend. The consolidation follow-up now covers both mirrors.
  - `internal/migrate/drain_test.go` (outside Files): moved `TestApplyRefusedDuringMigration` + `runApplyMain` to `internal/cli/migrate_test.go`, assertions unchanged — the migrate command made `cli` import `migrate`, so migrate's internal test files can no longer drive `cli.Main` (import cycle in test).
  - `Plan.BackupTo` always names `mythhelm.db.bak-migration-v7`: apply migrates the schema first, so the backup is always post-migration version (reads "0004" as 0007 per Task 1).
  - Build-identity downgrade refusal is schema-only: `build_version` is recorded in `migration_state` but never compared (build strings are unordered); only a newer schema refuses.
  - Bare `migrate` previews; `--apply` prints the preview first and requires `--yes` (exit 0 without it, nothing written).
  - The migration lock spans post-Drain through Adopt, not the `Drain` call itself (`Drain` acquires the same pair internally and flock re-acquire would refuse); a post-Drain surprise run aborts fatal with `ownership_unresolved` per the Task 4 handoff.
  - Resume short-circuits: `imported` adopts directly, `adopted` succeeds as a no-op; `drained` re-drains for a fresh quiesce and report.
  - `TestApplyResumesAfterFailure` simulates the kill with a mid-import `invalid_contract` failure (corrupt v1 row) + fix + re-apply through `cli.Main`, not a literal SIGKILL: the resume path (phase + markers + backup reuse) is identical and deterministic.
- **Files modified**: `internal/migrate/preview/plan.go`, `internal/migrate/apply.go`, `internal/cli/migrate.go`, `internal/cli/dispatch.go`, `internal/cli/migrate_test.go`, `tests/migrate_e2e_test.go`, `internal/migrate/drain_test.go`, `specs/in-progress/supervisor-migration/tasks.md`, `specs/in-progress/supervisor-migration/handoff.md`, `specs/in-progress/supervisor-migration/scratchpad.md`.

### Task 7 — NFR-2 SQLite posture proof ✅ COMPLETED

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
- **Status**: ✅ Completed — NFR-2 posture pinned: pragma string, Mutate tx scope, named busy timeout, backup/restore integrity checks, and the OQ-8 engine verdict (3.53.4 ≥ 3.51.3) from the built binary; PR #301.
- **Implementation**: `TestPragmasPinned` asserts the `pragmas` const carries WAL/FULL/5000 and scans the package sources so no second busy_timeout lurks; `TestMigrateWritesRunInMutate` walks top-level `internal/migrate` ASTs (preview/ excluded by construction, `tx.*` and read-only `Query` allowed; `sql.Open` allowed only for `readOnlyDSN`/`mode=ro` opens, `Exec` only for `VACUUM INTO`); `TestIntegrityCheckOnBackupRestore` flips one byte past page 1 (header intact, `user_version` still answers) and requires `persistence_unavailable` naming `integrity_check` in `migrate/cause` with write-free refusals; `TestSQLiteEngineHasWALFix` builds `cmd/mythhelm` and reads `sqlite_version` from `version --format jsonl` against the 3.51.3 floor. Teeth: pragma drift, read-only-DSN drift, direct `db.Exec`/`sql.Open` mutants and a raised floor each fail for the right reason. Commit 92342f0.
- **Spec deviations**:
  - No behavior deviation. The `sql.Open` exclusion covers the read-only `mode=ro` integrity opens per the Task 5 handoff ("exclude those opens from `TestMigrateWritesRunInMutate` exactly as the design excludes Backup's single `VACUUM INTO` `Exec`"); the entry's exclusion list names only the `VACUUM INTO` `Exec`.
  - No behavior deviation. The structural pin also flags `Begin`/`BeginTx`/`Prepare`/`PrepareContext` on non-`tx` receivers: a direct write through those handles would otherwise evade a literal `Exec`-only check while violating the one-transaction-per-mutation scope the test pins.
  - Extra files beyond the task Files list: `internal/journal/engine.go` (new `journal.SQLiteVersion` helper — the binary needs a surface exposing `SELECT sqlite_version()` for the OQ-8 assertion), `internal/cli/dispatch.go` (`version` reports `sqlite_version` in plain and jsonl), `internal/cli/dispatch_test.go` and `internal/cli/tui_test.go` (version-object and version-template assertions move with the new field).
- **Files modified**: `internal/journal/pragmas_test.go`, `internal/migrate/nfr2_test.go`, `internal/journal/engine.go`, `internal/cli/dispatch.go`, `internal/cli/dispatch_test.go`, `internal/cli/tui_test.go`, `specs/in-progress/supervisor-migration/tasks.md`, `specs/in-progress/supervisor-migration/handoff.md`, `specs/in-progress/supervisor-migration/scratchpad.md`.

### Task 8 — Migration ADR and support rows ✅ COMPLETED

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
- **Status**: ✅ Completed — Migration/import ADR recorded and the spec's support matrix published with its assertion; PR #302.
- **Implementation**: ADR 0016 (proposed — acceptance is the maintainer's) pins OQ-1/OQ-2, the import mapping and the drain order with file cites; `SUPPORT.md` lists exactly the 6 shipped rows, all `fixture-tested` (`nfr2-proof` cites Task 7's merged pins); `support_test.go` resolves every evidence ref to a real test, checks platform claims against build tags, maps every `migrate`/`preview` export to a row, and self-mutates each defect class; review round 1 hardened the build-constraint scan and the ADR status check. Commits 4f7989a, 638222b, 11c63da and the origin/main merge that flipped `nfr2-proof` once Task 7 landed.
- **Spec deviations**: None.
- **Files modified**: `docs/decisions/0016-migration-import.md`, `internal/migrate/SUPPORT.md`, `internal/migrate/support_test.go`, `specs/in-progress/supervisor-migration/tasks.md`, `specs/in-progress/supervisor-migration/handoff.md`, `specs/in-progress/supervisor-migration/scratchpad.md`.
