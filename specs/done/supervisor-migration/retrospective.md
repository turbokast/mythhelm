# supervisor-migration — Retrospective

## Review Summary

- **Range**: 3e5907c..4888161 (PRs #277, #285, #286, #287, #292, #297, #301, #302; fix PRs #324)
- **Reviewer**: code-reviewer, architect
- **Findings**: critical 2, important 8, suggestion 8 (confirmed); rejected 0
- **Open critical**: 0
- **Vendor review**: skipped (not run: maintainer skipped the optional vendor review)

The two critical findings of the first review (on 83f72ca) were fixed by PR #324 (4888161) and re-adjudicated on that head: both reviewers read the fix at its anchors and the code-reviewer reproduced the red-first result (`TestApplyRefusesStaleBackupAtDefaultPath` and `TestApplyHoldsLocksAcrossDrainAndBackup` fail against the pre-fix `apply.go` and `drain.go`; `TestApplyRefusesMissingRecordedBackup` passes on both, so it pins existing behaviour). #324 added no critical defect: every lock acquire is non-blocking, every `drainLocked` failure releases the run locks, and the lock order (instance, state dir, runs) agrees with the legacy order (state dir, run).

| Severity | Finding | Anchor | Disposition |
|---|---|---|---|
| critical | A stale backup at the default path is reused as the pre-migration backup after a restore or crash, so a later restore loses work (AC-7.4, design §5) | `internal/migrate/apply.go:261-263`, `internal/migrate/backup.go:70-79` | fixed in #324 (`backupOrReuse` takes a fresh `Backup`, which refuses an existing copy) |
| critical | The migration lock is released between `Drain` and `Apply`; legacy `recover`/`apply` pass the guards while the backup and import run (AC-7.3, I05, I18) | `internal/migrate/apply.go:114-123`, `internal/migrate/drain.go:119-164` | fixed in #324 (`Apply` holds the instance, state-dir and every run's owner lock from the drain to adoption) |
| important | Adopt outcomes are not persisted; adopted workers have no owner (design §4, I18) | `internal/migrate/apply.go:150-164`, `internal/migrate/drain.go:270` | issue #319 |
| important | `migrate --apply` is one-way and `Restore` has no production caller (O1, AC-7.4) | `internal/cli/dispatch.go:43`, `internal/migrate/backup.go:165` | issue #320 |
| important | State-dir lock refuses concurrent legacy runs outside any migration | `internal/supervisor/pipeline.go:191-211`, `internal/supervisor/recover.go:53-58` | issue #321 |
| important | Migration journal writers bypass the v2 phase rule; quarantine lacks evidence refs; quarantined runs import as `running` | `internal/migrate/import.go:368-433`, `internal/migrate/apply.go:509-572` | issue #322 |
| important | Evidence that cannot fail, NFR-2 pin gap, preview shows source files not owners, stale SUPPORT/doc text | `internal/migrate/drain_test.go:430-436`, `internal/migrate/preview/plan.go:47-54` | issue #323 |
| important | A recorded backup is reused while the phase is still `previewed`: `recordBackup` and `recordPhase(drained)` are two transactions, legacy writes are admitted between them (AC-7.4, design §5). The part of the first critical that #324 did not cover | `internal/migrate/apply.go:141-148`, `internal/migrate/apply.go:264-273`, `internal/supervisor/pipeline.go:245-251` | issue #341 |
| important | A run that finishes during the drain is classified on the pre-lock state and journaled as quarantined (design §4, I09, I23) | `internal/migrate/drain.go:130-157`, `internal/migrate/drain.go:293-296` | issue #342 |
| important | ADR 0016 stale after #324: lock-span wording and "post-drain surprise run aborts fatal" / "re-apply reuses the backup by digest" | `docs/decisions/0016-migration-import.md:62-70`, `docs/decisions/0016-migration-import.md:109-112` | fixed in the finalize pull request (ADR status stays `proposed`) |
| suggestion | No update/delete-abort triggers on `task_revisions`, `policies`, `grants` (I20) | `internal/journal/migrations/0007_v2contracts.sql:11-31` | kept |
| suggestion | `migration_state.schema_version` is a literal 7, never advanced | `internal/journal/migrations/0007_v2contracts.sql:42` | kept |
| suggestion | Backups land at the state root, not `backups/` (master §5.1) | `internal/migrate/backup.go:68` | kept |
| suggestion | Restore runs `integrity_check` after the rename | `internal/migrate/backup.go:261-270` | kept |
| suggestion | v1 readers do not filter by schema version | `internal/supervisor/receipt.go:65-100` | kept |
| suggestion | `SUPPORT.md` and the ADR cite none of #324's three tests as evidence for the lock-span claim (I14); the ADR now cites two | `internal/migrate/SUPPORT.md:24-26` | kept |
| suggestion | Exported `Drain` has no production caller and releases every lock on return; the Task 4 hand-off still says "call Drain before Backup" | `internal/migrate/drain.go:67-91`, `specs/unfinalized/supervisor-migration/handoff.md:91` | kept |
| suggestion | `markPreviewed` and the `imported` to `adopted` resume write the ledger before any migration lock is held | `internal/migrate/apply.go:93-103` | kept |

Foreign changes in range: 312d3ff (#317), 20d1ccc (#311), 4cb2c4a (#298), 8334337 (#294): internal/supervisor/pipeline.go, internal/supervisor/pipeline_test.go, internal/supervisor/recover.go; 5166fac (#315): the spec's own tracking files.

Open question for the maintainers (not a finding against this spec): `internal/control` never reads `migration_state`, so the supervisor service can own the ledger before `adopted`; per-run fencing on main (#336) covers writes to the same run only.

## Acceptance

| Criterion | Result | Evidence |
|---|---|---|
| AC-7.1 additive migrations, `0001_init.sql` untouched | met | `TestMigration0001Untouched`, `TestMigration0007CreatesV2Tables`; empty diff of `0001_init.sql` over the range |
| AC-7.2 import preserves IDs, posture, evidence; unverified stays unverified | met | `TestImportPreservesIDsPostureEvidence`, `TestImportReadyForReviewStaysCandidate`, `TestImportTwiceConflicts`; caveat: quarantined runs import as `running` (#322) |
| AC-7.3 exclusive lock, no new legacy admissions, drain or quarantine every owner | met | `TestApplyHoldsLocksAcrossDrainAndBackup` (#324), `TestRunRefusedPastPreviewed`, `TestApplyRefusedDuringMigration`, `TestDrain*`; caveats: adoptions not persisted (#319), run finishing during drain misclassified (#342) |
| AC-7.4 backup with identities, tested restore, refusal without write | partial | identities and refusals: `TestBackupRestoreRoundTrip`, `TestRestoreNewerSchemaRefuses`, `TestApplyRefusesStaleBackupAtDefaultPath`; restore is unreachable for a user (#320) and a recorded backup can miss writes at `previewed` (#341) |
| AC-7.5 v1 events decode under v1 | met | `TestV1DecodeByteIdentical` |
| AC-7.6 hermetic preview, nothing written until confirmed | met | `TestPreviewWritesNothing`, `TestPreviewHermetic`, `TestApplyWithoutYesPreviewsFirst`; caveat: preview lists owner source files, not owners (#323) |
| NFR-2 WAL, `synchronous=FULL`, one transaction per mutation, WAL-reset fix verified | met | `TestPragmasPinned`, `TestMigrateWritesRunInMutate`, `TestIntegrityCheckOnBackupRestore`, `TestSQLiteEngineHasWALFix`; caveat: `OpenLedger` DSN not pinned (#323) |

## Deviations

- Task 1: migration ships as `0007_v2contracts.sql` with schema version 7, not 0004/4 — recorded in the task entry; later specs claimed 0003-0006 first.
- Task 2, Task 4, Task 6: phase words are string literals in `internal/journal` and `internal/supervisor`, not `migrate.Phase`, to avoid import cycles — recorded; bound back by `TestPhaseMatchesMigrationVocabulary`.
- Task 3, Task 6: import and quarantine envelopes mirror `appendTx` statements instead of calling it — recorded; consolidation left as follow-up.
- Task 4: the migration lock is the state directory's owner lock held beside the instance lock, and there is no checkpoint-stop signalling — recorded; the lock was released between `Drain` and `Apply` until #324 (found by review, critical).
- Task 5: backup named `mythhelm.db.bak-migration-v<N>` with a JSON sidecar; the stale-backup reuse in `Apply` was a defect found by review (critical, fixed in #324).
- Task 6: build-identity downgrade refusal is schema-only; bare `migrate` previews and `--apply` needs `--yes` — recorded.
- Task 6 and Task 4 text on "lock spans post-Drain through Adopt" and "a post-Drain surprise run aborts fatal" (`tasks.md` Task 6 deviations, `handoff.md`) was superseded by #324, which deleted the surprise-run check; recorded in the #324 pull request body only, and the ADR is corrected in this finalize.
- Adoption not persisted, restore exercised in-process only, and quarantine records without evidence refs diverge from design §4/§5/D4 and were recorded by no task — found by review (#319, #320, #322).
- Task 7, Task 8: extra files and test helpers beyond the Files lists — recorded; no behaviour impact.

## CI history

- CI on PR #285 head faa1fe3: real, `Go (windows-latest)` and `Go (windows-11-arm)` failed `TestBackupRestoreRoundTrip` in `internal/migrate`; the task merged on a later head. Mechanism not established in this run (unknown); no run on faa1fe3 passed.
- CI on PR #292 head 12018e2: unknown, `Go (windows-11-arm)` failed `TestSeedFailureFailsClosed` in `internal/supervisor` with `TempDir RemoveAll cleanup ... owner.lock: The process cannot access the file because it is being used by another process`; no run on 12018e2 passed and the cause of the held handle was not diagnosed in this run.
- CI on PR #297 heads 7ae41d6, a53e156, 9b2b064: real, `TestBackupHoldsDrainedWrites` in `internal/cli` failed on Windows runners in all three (7ae41d6 also failed `TestPreviewMirrorsPinned` and on macOS); the task merged on a later head. Three consecutive pushes failed the same test before it passed.
- CI on merge 4888161 (PR #324): `Go (ubuntu-24.04-arm)` failed in `internal/workers` `TestUnsupervisedWorkerStartsNothing`, a package outside this spec's range (foreign, supervised-stop-recover); main's tip was green at verify (`finalize.py verify` printed `verdict=ready`).
- All other non-success runs on the task branches are `cancelled` (superseded by a later push). No failure and success on the same head SHA exists, so there is no nondeterministic failure to explain.

## Effort

dispatched=8 returned=5 failed=4 (runspec.py summary; `unaccounted=task 7 (1 dispatch)`); attempts 8 over 8 tasks; first-pass 6/8; review rounds 8; wall-clock 2026-10-08T23:31:47Z → 2026-10-10T01:29:58Z (last task merge); fix PR #324 merged 2026-10-10T07:28:28Z; finalize started five times from 2026-10-10T06:28:55Z (run-events lifecycle rows; the first stopped at review on the two criticals).

| Task | Agent | Attempts | Review rounds | PR | Merged | First pass |
|---|---|---|---|---|---|---|
| 1 | go-implementer | 1 | 0 | #277 | yes | no |
| 2 | - | 1 | 1 | #286 | yes | yes |
| 3 | - | 1 | 1 | #287 | yes | yes |
| 4 | - | 1 | 2 | #292 | yes | yes |
| 5 | - | 1 | 1 | #285 | yes | yes |
| 6 | mig-t6-implement | 1 | 2 | #297 | yes | yes |
| 7 | - | 1 | 1 | #301 | yes | yes |
| 8 | - | 1 | 0 | #302 | yes | no |

## Lessons

- What worked: pinning the phase vocabulary with `TestPhaseMatchesMigrationVocabulary` let three tasks use literals across an import cycle without drifting (Tasks 2, 4, 6).
- What worked: the whole-spec review found both criticals that eight per-task reviews and gates passed, and #324 landed with red-first tests the re-review reproduced.
- What to change: a guarantee that spans two tasks' functions (the lock held from `Drain` through `Apply`) was acceptance-tested in neither: Task 4 tested `Drain` releasing correctly, Task 6 tested `Apply` calling it. Evidence: critical finding 2, Task 4 deviation on the lock, #324. Proposal P-supervisor-migration-1.
- What to change: three of eight tasks (Tasks 4, 5, 6) failed CI on Windows runners (and Task 6 once on macOS), once for three consecutive pushes of one test (`TestBackupHoldsDrainedWrites`). No mechanism was established for #285 or #297; the one diagnosed signature (#292) is a file handle held past `TempDir` cleanup. No proposal: the evidence does not yet point at one harness change.
- What to change: the finalize range artifact was written to a fixed filename in the session scratchpad, and a concurrent finalize of another spec overwrote it; both reviewers were handed another spec's range. Evidence: the code-reviewer and architect reports each name a base `0ffc42d` and head `82b6fd3` for the file; the file later failed to parse as JSON. Proposal P-supervisor-migration-2.

## Proposals

- P-supervisor-migration-1 — Test a cross-task guarantee at the composed entry point
- P-supervisor-migration-2 — Name the finalize range artifact after its spec
