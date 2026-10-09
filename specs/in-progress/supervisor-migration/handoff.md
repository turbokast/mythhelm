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

- **Produces**: `Journal.Append` with the same signature, now accepting
  `schema_version == 2` under the §7 phase rule (nothing pre-`drained`;
  only `migration.quarantined`/`migration.imported` at `drained`;
  everything at `imported`/`adopted`); unexported
  `appendTx(ctx, tx, ev, project)` core (`internal/journal/journal.go`)
  that `Append` delegates to after `BeginTx`; v2 failures as a valid
  `*v2contract.ControlError` plus the stream-1 cause
  (`invalid_contract`, `ownership_unresolved` on stale generation,
  `persistence_unavailable` on ledger I/O). Tests in
  `internal/journal/append_v2_test.go`, including the v1 golden.
- **For dependents**: Task 3 landed first (PR #287) with its own
  mirrored `appendImported` in `internal/migrate/import.go`, since
  `appendTx` is unexported per this task's spec and unreachable
  cross-package — consolidating that mirror onto `appendTx` is a
  follow-up needing a thin exported wrapper in `internal/journal`
  (one-line deviation). Never call db-level `Append` from inside
  `Mutate` (it takes its own `BeginTx`). A duplicate `event_id` acks
  nil with no append — ID-reuse-with-different-content conflicts are
  the caller's layer (import markers), never `Append`'s. v1 behavior,
  messages and sentinels are byte-identical, and the v1 golden pins
  decodes; phase refusals also wrap `journal.ErrInvalidEvent`.
- **Deviations that change a later task's inputs**: none — the phase
  vocabulary, migration number (0007) and `migration_state` shape from
  Task 1 are unchanged. Note the consolidation follow-up above, and
  that phase words live as literals in `internal/journal` mirroring
  `migrate.Phase` (no import either way between the packages, or it
  cycles).

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
  (including rowless dirs) quarantine. Drain loops `drainStabilizePasses`
  re-enumerations for admission racers, with the final pass aborting on a
  fresh run. The `spawnWorker` seam
  in drain.go is test-only. `Run` maps guard I/O failures to the existing
  `persistence_unavailable` refusal (see `TestLockedDatabaseStopsAdmission`).
- **Deviations that change a later task's inputs**: the migration lock is the
  state-dir `owner.lock` held alongside the instance lock (Tasks 5–6: probe
  or hold the same pair, never the instance lock alone for admissions); the
  journal phase test uses literals — never re-add a migrate import to
  journal's internal tests (Tasks 2, 6). `Run` holds the migration lock
  through `MkdirAll` + `AcquireOwner`, so no hook-check-to-mkdir race
  remains: any post-drain run is a genuine surprise, and Apply (Task 6)
  must treat it as fatal, not silent.

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

- **Produces**: `preview.Plan` + `PreviewPlan(ctx, db)` in
  `internal/migrate/preview/plan.go` (runs with derived task ids and
  verbatim postures, 4 owner sites in drain order, pending schema steps,
  backup target); `migrate.Apply(ctx, db, plan)` in
  `internal/migrate/apply.go` (Drain → Backup → Import → Adopt with
  `previewed → drained → imported → adopted` phase records);
  `mythhelm migrate --preview/--apply [--yes] [--format]` in
  `internal/cli/migrate.go` (+ `dispatch.go` registration) printing plan
  and receipt (plain/jsonl); tests in `internal/cli/migrate_test.go`
  and `tests/migrate_e2e_test.go`.
- **For dependents**: every `internal/migrate` ledger write runs inside
  `control.Mutate` (Task 7: no `sql.Open` in `apply.go`; `*sql.DB` use
  is read-only `Query`; the only `Exec` paths are `tx.*` inside
  `Mutate` plus `Backup`'s excluded `VACUUM INTO`). Quarantine
  envelopes (`migration.quarantined`, deterministic
  `migration-quarantined-<runID>`, producer `migration` generation 0)
  persist in one post-backup `Mutate`; dedupe strips `quarantined_at`
  before comparing. Resume: `drained` re-drains and reuses the backup
  by digest, `imported` adopts directly, `adopted` no-ops; imports skip
  by `task_revisions` markers. `migration_state` carries `started_at`,
  `backup_path`, `build_version` after the first apply.
- **For Task 7**: `preview` imports only stdlib + `v2contract`
  (hermeticity pinned by `TestPreviewHermetic`); mirror constants
  (`dbName`, schema version, migration filenames) are pinned by
  `TestPreviewMirrorsPinned` — the next schema bump must sweep
  `preview/plan.go` too. The e2e restores and compares v1 projections
  byte-identically.
- **Deviations that change a later task's inputs**: `Plan.Runs` is
  `[]preview.Run`, not `[]migrate.ImportResult` (import cycle — same
  JSON shape); quarantine append mirrors `appendTx` like Task 3's
  `appendImported` (consolidation follow-up covers both); Task 4's
  `TestApplyRefusedDuringMigration` now lives in
  `internal/cli/migrate_test.go` (import cycle — assertions unchanged);
  build-identity refusal is schema-only (build strings unordered).

## Task 7 — NFR-2 SQLite posture proof

- **Produces**: `TestPragmasPinned` in
  `internal/journal/pragmas_test.go` (open pragma string pins WAL +
  synchronous FULL + busy_timeout 5000; every busy_timeout in the
  package's non-test sources names the design-time 5000);
  `TestMigrateWritesRunInMutate`, `TestIntegrityCheckOnBackupRestore`
  and `TestSQLiteEngineHasWALFix` in `internal/migrate/nfr2_test.go`;
  OQ-8 verdict (engine 3.53.4 from the built binary) recorded in
  `scratchpad.md`.
- **For dependents**: `mythhelm version` (plain and `--format jsonl`)
  now reports `sqlite_version` from `journal.SQLiteVersion`
  (`internal/journal/engine.go`, `:memory:` open, no state touched) —
  the OQ-8 surface. The Mutate-scope pin walks top-level
  `internal/migrate/*.go` (preview/ excluded by construction):
  `tx.*` calls allowed; `*sql.DB` Exec/Begin/Prepare allowed only for
  Backup's `VACUUM INTO`; `sql.Open` allowed only for the read-only
  integrity opens (`readOnlyDSN` or a `mode=ro` literal) — a new
  direct write fails the test by name and line, so future migrate
  writes must go through `control.Mutate` or extend the exclusion
  with a cited reason.
- **Deviations that change a later task's inputs**: `version --format
  jsonl` gains the `sqlite_version` field; Task 8's support matrix
  NFR-2 row is fixture-tested (corrupt-page fixtures, built-binary
  engine assertion), not live-qualified.

## Task 8 — Migration ADR and support rows

- **Produces**: ADR `docs/decisions/0016-migration-import.md`
  (`Status: proposed` — acceptance is the maintainer's; flip line 3 on
  approval) pinning OQ-1/OQ-2, the import mapping and the drain order;
  `internal/migrate/SUPPORT.md` with exactly the 6 shipped rows, all
  `fixture-tested` (`nfr2-proof` cites Task 7's four pins); the matrix
  assertion `TestSupportMatrixMatchesEvidence` plus
  `TestMigrationADRNamesDecisions` in
  `internal/migrate/support_test.go`.
- **For dependents**: the matrix test resolves every evidence ref to a
  real test function in `migrate`/`journal`/`cli`/`tests`, checks the
  platform claims against build tags, and maps every exported
  declaration of `internal/migrate` and `preview` to a row — adding
  behaviour without a row fails. Nothing here claims
  `live-qualified`.
- **Deviations that change a later task's inputs**: none — docs plus
  assertion only, no behaviour changed. Task 7 merged first, so the
  `nfr2-proof` row ships `fixture-tested` here; no later task owes a
  flip.
