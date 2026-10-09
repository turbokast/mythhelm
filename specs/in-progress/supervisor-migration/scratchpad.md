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
  implementing task evidence (Task 7). Status: open — Task 7 records the
  observed version and the backport pin (if any) here.

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

- **Task 1**: the migration shipped as `0007_v2contracts.sql`
  (`SchemaVersion` 7), not 0004/4 as the task text says: origin/main
  already carries 0001–0006 (budget-ledger-s1's 0003, supervisor
  0004, reservation-host 0005, evaluator 0006). Later tasks read
  "0004" as 0007. `go:embed migrations/*.sql` picks the new file up
  with no registry change. Three pre-existing tests pin the schema
  version (`journal_test.go` pragmas, `qualification_test.go`
  `TestMigration0002Applies`, `evaluator_test.go`) and were bumped
  6→7; future schema bumps must sweep those files too.
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
