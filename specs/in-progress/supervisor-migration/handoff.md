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

- **Produces**: `migrate.Drain(ctx, db) (DrainReport, error)` and
  `DrainReport{Drained, Adopted, Quarantined []string}` in
  `internal/migrate/drain.go`; migration guards in
  `internal/supervisor/pipeline.go` (`checkMigrationClear`, called by `Run`),
  `internal/supervisor/recover.go` (`RecoverWithHooks`),
  `internal/cli/apply.go` (`runApply` plus the shared helper) and
  `internal/cli/tui.go` (palette Apply); tests in
  `internal/migrate/drain_test.go`. Drain errors are `*v2contract.ControlError`
  (`ownership_unresolved`/`persistence_unavailable`); hook refusals wrap
  `supervisor.ErrOwnership` naming the code and phase (CLI exit 6).
- **For dependents**: call Drain before Backup (Task 6) — it holds the
  instance lock and the state-dir owner lock throughout and writes no ledger
  row. Persist each quarantined run's `migration.quarantined` envelope
  post-backup by re-reading its worker.json and journal evidence (the report
  carries run IDs only). Adopted runs keep live pids under their existing
  launch identity — never relaunch. Terminal states are the complement of
  `supervisor.activeStates`; active runs without a live verified worker
  (including rowless dirs) quarantine. Drain re-enumerates twice for
  admission racers and aborts if runs still appear. The `spawnWorker` seam
  in drain.go is test-only. `Run` maps guard I/O failures to the existing
  `persistence_unavailable` refusal (see `TestLockedDatabaseStopsAdmission`).
- **Deviations that change a later task's inputs**: the migration lock is the
  state-dir `owner.lock` held alongside the instance lock (Tasks 5–6: probe
  or hold the same pair, never the instance lock alone for admissions); the
  journal phase test uses literals — never re-add a migrate import to
  journal's internal tests (Tasks 2, 6); a residual sub-millisecond
  hook-check-to-mkdir race remains — Apply (Task 6) must treat unexpected
  post-drain runs as fatal, not silent.

## Task 5 — Backup and restore with downgrade refusal

<!-- pending -->

## Task 6 — `mythhelm migrate` with hermetic preview

<!-- pending -->

## Task 7 — NFR-2 SQLite posture proof

<!-- pending -->

## Task 8 — Migration ADR and support rows

<!-- pending -->
