# Supervisor migration support matrix

One row per deliverable of the supervisor migration (spec
`supervisor-migration`; decision record
[ADR 0016](../../docs/decisions/0016-migration-import.md)).
`support_test.go` reads this file: it must list exactly the shipped
deliverables, every exported declaration of `internal/migrate` and its
`preview` subpackage must map to a row, and each row's evidence must be
tests that exist in the named packages, built for the platforms the row
claims.

Evidence refs are package-qualified (`migrate`, `journal`, `cli`, `tests`).
Statuses are `fixture-tested` and `blocked`. Nothing here is
`live-qualified`: no row has run against a live multi-client deployment.
`fixture-tested` means the named tests run in the CI matrix
(`.github/workflows/ci.yml`: Linux, macOS and Windows runners) for the
platforms in the row; this file does not claim a passing run, which is
CI's to show. Platforms are `all` (the test files carry no build tag).

| ID | Deliverable | Status | Platforms | Evidence |
|---|---|---|---|---|
| `v2-tables` | v2 contract tables (migration `0007_v2contracts.sql`, spec 0004 renumbered) with the `migration_state` phase row, plus phased v2 `Append` acceptance (nothing pre-`drained`, migration-owned at `drained`, all v2 at `imported`) and byte-identical v1 decodes | fixture-tested | all | `journal.TestMigration0007CreatesV2Tables`, `journal.TestMigration0001Untouched`, `journal.TestMigrationStateStartsNotStarted`, `journal.TestSchemaVersionIs7`, `journal.TestAppendAcceptsV2PostMigration`, `journal.TestAppendAcceptsMigrationOwnedAtDrained`, `journal.TestAppendRejectsOrdinaryV2AtDrained`, `journal.TestAppendRejectsV2PreMigration`, `journal.TestV1DecodeByteIdentical` |
| `import` | One legacy v1 run imports as one v2 task revision 1 with preserved IDs, verbatim billing posture and untouched v1 evidence; never `accepted`; conflicts and corrupt rows refuse; the marker, v2 rows and `migration.imported` envelope commit or roll back together | fixture-tested | all | `migrate.TestImportPreservesIDsPostureEvidence`, `migrate.TestImportReadyForReviewStaysCandidate`, `migrate.TestImportTwiceConflicts`, `migrate.TestImportEventIDReuseConflicts`, `migrate.TestImportCorruptV1Refuses`, `migrate.TestImportRollsBackAtomically` |
| `drain` | `Drain` quiesces the four legacy owner sites in drain order into a drain/adopt/quarantine report with no ledger write; new admissions and recovery refuse past `previewed` | fixture-tested | all | `migrate.TestDrainRefusesWhileLockHeld`, `migrate.TestDrainAdoptsSameLaunch`, `migrate.TestDrainWritesNothing`, `migrate.TestDrainQuarantinesDeadWorker`, `migrate.TestRunRefusedPastPreviewed`, `migrate.TestRecoverRefusedPastPreviewed`, `cli.TestApplyRefusedDuringMigration` |
| `backup-restore` | `VACUUM INTO` backup with a schema/build-identity sidecar, integrity-checked restore, and newer-schema/digest/overwrite refusals that write nothing | fixture-tested | all | `migrate.TestBackupRestoreRoundTrip`, `migrate.TestRestoreNewerSchemaRefuses`, `migrate.TestRestoreDigestMismatchRefuses`, `migrate.TestBackupRefusesOverwrite` |
| `migrate-cli` | `mythhelm migrate --preview` (hermetic, read-only plan) and `--apply --yes` (Drain → Backup → Import → Adopt with receipt, marker-based resume, packaged-binary e2e) | fixture-tested | all | `cli.TestPreviewWritesNothing`, `cli.TestPreviewHermetic`, `cli.TestPreviewRefusesNewerSchema`, `cli.TestApplyWithoutYesPreviewsFirst`, `cli.TestBackupHoldsDrainedWrites`, `cli.TestApplyPersistsQuarantinePostBackup`, `cli.TestApplyResumesAfterFailure`, `tests.TestMigrateE2EPackagedBinary` |
| `nfr2-proof` | NFR-2 SQLite posture proof: the open pragma string (WAL, synchronous FULL, busy_timeout 5000), migrate ledger writes confined to `control.Mutate` apart from Backup's `VACUUM INTO` and read-only integrity opens, `integrity_check` on backup and restore, and the shipped engine's WAL-fix version (OQ-8) read from the built binary | fixture-tested | all | `journal.TestPragmasPinned`, `migrate.TestMigrateWritesRunInMutate`, `migrate.TestIntegrityCheckOnBackupRestore`, `migrate.TestSQLiteEngineHasWALFix` |

## Not claimed

- No row is `live-qualified`.
- The NFR-2 row is fixture-tested (corrupt-page fixtures and an engine
  assertion on the built binary); it claims no behaviour under a live
  multi-client load.
- The legacy per-run path is drained and refused during migration, not
  removed; full removal is a follow-up after stream 4 (design §11).
- Quarantined runs are persisted with evidence, never reconciled;
  reconciliation belongs to stream 4 (`supervised-stop-recover`).
