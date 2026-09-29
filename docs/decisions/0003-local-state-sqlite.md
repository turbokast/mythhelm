# 0003. Local state: SQLite driver, schema v1 and migration policy

- Status: accepted
- Date: 2026-09-29

## Context

MYTHHELM must remember runs, attempts, candidates, checks, trust grants and billing declarations across crashes. It must also keep an event journal that recovery and receipts can rely on. The master specification asks for SQLite through a pure-Go driver (§6.4). It asks for transactional state transitions with a durable journal (§7.5), a deduplicated, per-producer-ordered event envelope (§7.6), and a refusal to downgrade across an incompatible schema (§7.5). Release builds and CI must work with `CGO_ENABLED=0` on Linux, macOS and Windows (G01).

This slice has no per-user daemon (N11). Each run's supervisor writes through SQLite transactions, and different runs are serialised by SQLite's own locking (ADR 0004 covers process ownership).

## Decision

**Driver.** Use `modernc.org/sqlite`, pinned exactly at v1.59.0. That was the newest release at least 7 days old on 2026-09-29, per the repository's dependency cooldown. It is BSD-3-Clause, stable v1 and CGO-free, and it uses SQLite's own VFS and locking. We rejected `ncruces/go-sqlite3` because it is pre-1.0 and its custom VFS is less proven. The two drivers are never mixed on one file. The driver has a history of retracted releases, so upgrades are deliberate, reviewed bumps and are never taken as a floating minimum.

**Location and file.** The state directory is `$MYTHHELM_HOME` (absolute) or the per-OS default in design §5. It is created with mode 0700 (`statedir.Ensure`), and an existing directory with wider permissions is narrowed on Unix, through a handle verified to be the directory that was checked. A state directory that is itself a symlink is refused, so MYTHHELM never changes the permissions of a directory someone else pointed it at. Only the final component is checked. The ancestors are the user's home or a path the user configured, and choosing a `MYTHHELM_HOME` whose parents only the user can write is the user's responsibility. On Windows, the directory inherits its parent's ACL. The default, `%LocalAppData%`, is private to the user profile. Enforcing an owner-only ACL on a custom `MYTHHELM_HOME` is deferred along with the rest of Windows ownership (N7). The database is `mythhelm.db` in that directory. The directory must be on a local filesystem, because WAL requires it.

**Connection settings.** Every pooled connection applies `busy_timeout(5000)`, `foreign_keys(1)`, `journal_mode(WAL)` and `synchronous(FULL)`, and begins transactions with `BEGIN IMMEDIATE` (`_txlock=immediate`). The path is percent-encoded into a `file:` URI, so paths with spaces, `?`, `#`, `%` or non-ASCII characters open the intended file.

**Schema v1.** `internal/journal/migrations/0001_init.sql`, exactly as in design §5. `PRAGMA user_version` records the schema version. Tables are `STRICT`. The `journal` table is append-only: triggers abort every `UPDATE` and `DELETE`, whichever connection issues them.

**Append contract.** `(*Journal).Append(ctx, ev, project)` runs in one transaction:

1. If `event_id` is already journaled, it does nothing and returns nil. The projection does not run again.
2. It rejects a `generation` older than the producer's newest (`ErrStaleGeneration`).
3. It rejects any `producer_sequence` other than the producer's last plus one (`ErrSequenceGap`). A new producer starts at 1. Sequences continue across generations, because `(producer_id, producer_sequence)` is unique.
4. It assigns `run_sequence` as the run's maximum plus one, which is ingestion order (§7.6), and ignores any caller-supplied value.
5. It applies the caller's projection update in the same transaction. A projection error rolls everything back (AC-9.3).

The envelope is validated before any write. `schema_version` must be 1, and every required field must be present. The payload must be a JSON object, so every value is a named field and a bare number can never be stored as a measurement of unknown kind (I09). The payload is stored byte for byte, so decimal strings and explicit `null`s survive unchanged.

**Migration policy.**

- `Open` first reads `user_version` over a read-only connection that sets no pragmas. The WAL pragma alone would rewrite the header of a rollback-journal file.
- If the version is newer than the binary supports, `Open` returns `ErrSchemaTooNew`, with a restore instruction, and writes nothing. The CLI maps this error to exit 2 (AC-9.4).
- Before upgrading from any non-zero version, `Open` copies the database with `VACUUM INTO mythhelm.db.bak-v<N>`.
  - The copy is taken while the migrating connection holds the write lock (`BEGIN IMMEDIATE`). It runs on a second connection that reads the committed database as a WAL reader. Concurrent openers therefore take turns, and only the first one backs up and migrates.
  - A backup left by an interrupted migration is reused only if it is an intact database (`quick_check`) at the same version.
  - Anything else at the backup path is never overwritten, and `Open` fails, naming that path.
- The migrations then run in one transaction, which re-checks the version first in case another process migrated concurrently.
- A released migration file is never edited. Each change is a new numbered file.

## Consequences

- The binary builds and tests without cgo. `TestBuildWithoutCgo` runs `CGO_ENABLED=0 go build ./...` on every CI OS.
- The driver adds about ten transitive modules, including `golang.org/x/sys`, which D5 names as a direct dependency for later tasks.
- `golang.org/x/sys` is BSD-3-Clause plus the Go project's additional patent grant. Dependency review reports that grant as `LicenseRef-scancode-google-patent-license-golang`. The grant only adds rights, so it is compatible with Apache-2.0, and it is added to the dependency-review allow-list (`.github/workflows/dependency-review.yml`). Every `golang.org/x` module carries it.
- Opening a database left by a newer MYTHHELM is safe: the file's bytes are unchanged (`TestOpenRefusesNewerSchema`).
- Concurrent writers from different runs queue on the 5-second busy timeout. If a lock is held longer than that, the write fails with `SQLITE_BUSY`; turning that into an admission stop (AC-9.5) is Task 15.
- Linux open-file-description locks in the driver stay off (`MODERNC_SQLITE_OFD_LOCK` unset), keeping SQLite's default POSIX locking (scratchpad Q10).
- Tests that pin this behaviour: `TestPragmasApplied`, `TestJournalRejectsUpdateAndDelete`, `TestAppendIsIdempotentOnEventID`, `TestAppendRejectsProducerSequenceGap`, `TestAppendRejectsStaleGeneration`, `TestRunSequenceIsPerRunMonotonic`, `TestConcurrentAppendsFromTwoHandles`, `TestAppendProjectionErrorRollsBack`, `TestOpenRefusesNewerSchema`, `TestMigrationBacksUpBeforeUpgrade`, `TestConcurrentOpensMigrateOnce`, `TestMigrationReusesCompleteBackup`, `TestMigrationRefusesUnusableBackup`, `TestOpenPathWithSpacesAndUnicode`, `TestStateDirMode0700` and `TestEnsureRefusesSymlink`.
