# supervisor-migration — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Migration 0004 v2 contract tables

- **Produces**: migration `internal/journal/migrations/0007_v2contracts.sql`
  (renumbered from 0004 — see deviation below) creating `tasks`,
  `task_revisions` (+ `task_revisions_run` index on `run_id`),
  `policies`, `grants`, `migration_state`; `SchemaVersion == 7`;
  `internal/migrate/phase.go` with `migrate.Phase` constants
  (`not_started`, `previewed`, `drained`, `imported`, `adopted`);
  tests in `internal/journal/migrate_tables_test.go`.
- **For dependents**: `migration_state` holds exactly one row (`id = 1`);
  fresh databases start at phase `not_started` with `schema_version = 7`
  and empty `started_at`/`backup_path`/`build_version` (Task 6's Apply
  records the start marker and build identity). The `phase` CHECK
  accepts exactly the five `migrate.Phase` values. v1 table DDL is
  frozen — no migration may alter the 10 v1 tables (guarded by
  `TestMigration0007CreatesV2Tables` and `TestMigration0001Untouched`).
- **Deviations that change a later task's inputs**: the migration number
  is 0007, not 0004 (Tasks 2–8: read every "0004" in the spec as 0007;
  `SchemaVersion` is 7, not 4). The phase vocabulary is unchanged, so
  Task 2's phase rule and Tasks 4–6's phase reads are unaffected.

## Task 2 — Append accepts v2 envelopes post-migration

<!-- pending -->

## Task 3 — Legacy import as one-task runs

<!-- pending -->

## Task 4 — Drain, adopt, quarantine

<!-- pending -->

## Task 5 — Backup and restore with downgrade refusal

- **Produces**: `migrate.Backup(ctx, db, dir) (BackupInfo, error)` and
  `migrate.Restore(ctx, info, dir) error` in `internal/migrate/backup.go`
  with `migrate.BackupInfo` exactly as designed (`path`,
  `schema_version`, `build_version`, `sha256`, `created_at`);
  tests in `internal/migrate/backup_test.go`.
- **For dependents**: `dir` is the state dir in both calls. Backup lands
  at `<dir>/mythhelm.db.bak-migration-v<N>` with the sidecar at
  `<backup>.json` (Task 6: expect this name in `Plan.BackupTo`).
  Every failure is a `*v2contract.ControlError` passing `Validate` —
  assert codes with `errors.As`: `invalid_contract` (overwrite,
  digest mismatch), `schema_too_new` (backup newer than the binary),
  `persistence_unavailable` (I/O, integrity failures). All refusals
  return before writing anything to the target dir.
- **For dependents**: `Restore` refuses with `persistence_unavailable`
  when the target holds a non-empty `-wal` — close every handle on
  the state database first (last close checkpoints the WAL). The
  backup file's own `user_version` is authoritative: a sidecar
  disagreeing with it refuses `invalid_contract`, a lowered sidecar
  over a newer file still refuses `schema_too_new`.
- **For Task 7**: `Restore` opens the backup and the restored copy
  read-only (`mode=ro`) for `PRAGMA integrity_check` and the
  backup's `user_version` read; exclude those opens from
  `TestMigrateWritesRunInMutate` exactly as the design excludes
  Backup's single `VACUUM INTO` `Exec`. Corrupt-source backup and
  corrupt-copy restore (matching digest) both abort with
  `persistence_unavailable` — verified by a throwaway probe, not a
  committed test, so Task 7 owns that pin.
- **Deviations that change a later task's inputs**: backup.go mirrors
  `journal.DBName`/`journal.SchemaVersion` (import cycle — journal's
  tests import migrate); the next schema bump must sweep
  `supportedSchemaVersion` (Task 6+, `TestBackupPinsMatchJournal`
  fails otherwise).

## Task 6 — `mythhelm migrate` with hermetic preview

<!-- pending -->

## Task 7 — NFR-2 SQLite posture proof

<!-- pending -->

## Task 8 — Migration ADR and support rows

<!-- pending -->
