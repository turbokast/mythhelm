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

- **Produces**: `migrate.ImportRun(ctx, tx, runID) (v2contract.TaskRevision, error)` in
  `internal/migrate/import.go`; `migrate.ImportOptions{RunIDs}` (empty means all v1 runs;
  consumed by Task 6, not by `ImportRun`); `migrate.ImportResult{RunID, TaskID, Revision,
  Posture}` receipt row; failures are `*v2contract.ControlError` (`revision_conflict`,
  `invalid_contract`, `persistence_unavailable`, each `Validate`-clean). Tests in
  `internal/migrate/import_test.go`.
- **For dependents**: call inside `control.Mutate` — `ImportRun` never commits. The v2
  task id is the run's earliest attempt's legacy task id, or the run id when the run has
  no attempts; the `migration.imported` envelope carries legacy run/attempt ids verbatim
  under deterministic event id `migration-imported-<runID>` (producer `migration`,
  generation 0). Import never yields `accepted` (`completed`/`ready_for_review`/
  `applying` → `candidate`, I07); v1 rows are read-only to import.
- **Deviations that change a later task's inputs**: the envelope append mirrors
  `Journal.Append` inline instead of Task 2's `appendTx` (Tasks 2, 6: no action needed —
  the statements stay valid after `appendTx` lands; a follow-up may refactor
  `appendImported` onto it).

## Task 4 — Drain, adopt, quarantine

<!-- pending -->

## Task 5 — Backup and restore with downgrade refusal

<!-- pending -->

## Task 6 — `mythhelm migrate` with hermetic preview

<!-- pending -->

## Task 7 — NFR-2 SQLite posture proof

<!-- pending -->

## Task 8 — Migration ADR and support rows

<!-- pending -->
