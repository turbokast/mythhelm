## Supervisor Migration — Scratchpad

Seeded at spec creation. Every task appends its discoveries; nothing here
overrides `design.md` — conflicts go back to the designer.

## Open questions

- **OQ-1** (new tables vs additive columns): default (a) new tables; decided in
  design D2 (`0004_v2contracts.sql`). Decider: designer. Status: decided.
- **OQ-2** (migration trigger surface): default (a) explicit `mythhelm migrate`
  with `--preview`/`--apply`; decided in design D3. Decider: designer.
  Status: decided.
- **OQ-8** (shipped SQLite engine WAL-fix status): default unverified until
  `SELECT sqlite_version()` is asserted from the built binary. Decider:
  implementing task evidence (Task 7). Status: decided — the built
  binary reports engine 3.53.4 (`modernc.org/sqlite v1.60.1`), meeting
  the ≥3.51.3 floor; no backport pin needed. Evidence:
  `TestSQLiteEngineHasWALFix` (`internal/migrate/nfr2_test.go`) builds
  `cmd/mythhelm` and reads `sqlite_version` from `version --format
  jsonl`.

## Prerequisite-spec notes

- Stream 1 (`specs/*/v2-contract-vocabulary/`): consume `v2contract` signatures
  verbatim (§1 of design lists them). OQ-9 already decided there (same
  `journal` table, v2 accepted post-migration). NFR-4's migration-preview half
  was deferred here — AC-7.6 and Task 6 carry it.
- Stream 2 (`specs/*/supervisor-service/`): consume `control` signatures
  verbatim; `0003_supervisor.sql` (`operations`, `reservations`) must exist
  before Task 1 runs. Windows pipe transport may still be blocked on the
  go-winio maintainer decision — migration tasks stay platform-agnostic and
  must not depend on it.
- Stream 4 (`specs/*/supervised-stop-recover/`) runs in parallel on disjoint
  files; it reconciles the `migration.quarantined` envelopes this spec writes.

## Research notes

- `VACUUM INTO` cannot run inside a transaction (`journal.go:226`); Backup
  follows the existing off-transaction pattern.
- `modernc.org/sqlite` version in `go.mod` is the engine under test for OQ-8;
  the module cache does not reveal its SQLite version — only the built binary
  does (Task 7).
- In-flight `specs/*/claude-strict-subscription/` touches
  `internal/cli/run.go`, neighboring the migrate-command work (Task 6):
  coordinate or land after.

## Task discoveries

- **Task 5**: the pre-migration backup lands at
  `mythhelm.db.bak-migration-v<N>` (+ `<copy>.json` sidecar), not
  journal's `<path>.bak-v<N>` (Open writes those during migration;
  the names must not collide). `internal/migrate` cannot import
  `internal/journal` (journal's tests import migrate — cycle), so
  `backup.go` mirrors `DBName`/`SchemaVersion` with
  `TestBackupPinsMatchJournal`; schema bumps sweep that file too.
  Task 7's Mutate-scope assertion must exclude Restore's read-only
  `mode=ro` integrity opens alongside Backup's `VACUUM INTO`.
- **Task 1**: the migration shipped as `0007_v2contracts.sql`
  (`SchemaVersion` 7), not 0004/4 as the task text says: origin/main
  already carries 0001–0006 (budget-ledger-s1's 0003, supervisor
  0004, reservation-host 0005, evaluator 0006). Later tasks read
  "0004" as 0007. `go:embed migrations/*.sql` picks the new file up
  with no registry change. Three pre-existing tests pin the schema
  version (`journal_test.go` pragmas, `qualification_test.go`
  `TestMigration0002Applies`, `evaluator_test.go`) and were bumped
  6→7; future schema bumps must sweep those files too.
- **Task 2**: the v1 golden in `append_v2_test.go` was captured from
  base `148084b` via the test's `GOLDEN_OUT` hook (deterministic across
  runs); regenerate from the base, never from the branch, if the
  fixture ever changes. Sandbox note for the whole spec: the full
  suite needs a clean `HOME` here — `TestStrictMainBlocksWriteNothing`
  inventories native config from `$HOME` and fails identically on the
  untouched base (it passes in CI); run gates with `HOME` pointed at
  an empty dir and the real python on `PATH` (`python3` is an asdf
  shim and breaks under a fake `HOME`).
- **T3 (import, PR #287)**: v1 runs carry no goal/deliverable/write-scope text (only
  `task_sha256`), so the v2 record stores `sha256:<task_sha256>` as
  `AcceptanceContractDigest` with empty prose fields (I09). Task-id rule: earliest
  attempt's legacy task id, run id fallback. Import never yields `accepted`, including
  for `completed` runs (I07). Local gate note: `TestStrictMainBlocksWriteNothing`
  (`internal/cli`) inventories the developer's real `$HOME` native config and fails on
  machines with untrusted native settings; it passes with a clean `HOME` and on clean
  CI runners — pre-existing on origin/main, unrelated to this spec. (`python3` here is
  an asdf shim that needs the real `HOME`; run gatelib with the resolved interpreter
  when overriding `HOME`.)
- **Task 4**: journal's internal tests must never import
  `internal/migrate` (import cycle once migrate imports
  control/supervisor/journal per design §2); the phase vocabulary
  there stays as literals bound by
  `TestPhaseMatchesMigrationVocabulary`. Drain holds the instance
  lock plus the state-dir owner lock; quarantine envelopes persist
  post-backup from re-read evidence, and adopted runs are never
  relaunched.
- **Task 6**: `internal/cli` now imports `internal/migrate` (the
  migrate command), so migrate's *internal* test files cannot import
  `cli` back — `TestApplyRefusedDuringMigration` moved to
  `internal/cli/migrate_test.go` (external `_test` packages may still
  import across, per the admission/supervisor precedent). Quarantine
  payload re-reads the latest attempt row at persist time (attempt +
  task + token hash, empty when never launched); the dedupe strips
  `quarantined_at` before comparing. `Apply` never spawns the
  supervisor — adopted hands the ledger over on next lazy start. The
  e2e and `TestBackupHoldsDrainedWrites` close every handle before
  `Restore` and rely on last-close checkpointing (the backup_test.go
  pattern; no extra checkpoint handle — less file churn for Windows
  runners). A resume whose recorded backup vanished refuses with
  `invalid_contract` instead of re-taking (a fresh copy would hold
  committed migration rows, not pre-migration state).
- **Task 7**: `ControlError.Error()` renders code + next action only —
  integrity-path pins must assert on `Detail["migrate/cause"]`, not the
  message. Single-flipped-byte corruption past page 1 trips
  `integrity_check` on the first candidate tried (page 2 + 64); the
  `corruptCopy` helper still hunts across pages/offsets so the
  fixture fails loudly instead of silently passing if the engine ever
  stops detecting it. Local gate note from earlier tasks still
  applies: run the full suite with a clean `HOME`.
