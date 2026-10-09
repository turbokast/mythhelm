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
- **For dependents**: Task 3 calls the envelope path inside `Mutate` —
  but `appendTx` is unexported per this task's spec, so Task 3 must add
  a thin exported wrapper in `internal/journal` (one-line deviation) to
  reach it cross-package; never call db-level `Append` from inside
  `Mutate` (it takes its own `BeginTx`). A duplicate `event_id` acks
  nil with no append — ID-reuse-with-different-content conflicts are
  the caller's layer (import markers), never `Append`'s. v1 behavior,
  messages and sentinels are byte-identical, and the v1 golden pins
  decodes; phase refusals also wrap `journal.ErrInvalidEvent`.
- **Deviations that change a later task's inputs**: none — the phase
  vocabulary, migration number (0007) and `migration_state` shape from
  Task 1 are unchanged. Note the wrapper need above (Task 3), and that
  phase words live as literals in `internal/journal` mirroring
  `migrate.Phase` (no import either way between the packages, or it
  cycles).

## Task 3 — Legacy import as one-task runs

<!-- pending -->

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
