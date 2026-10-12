# Protected Acceptance — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| Q-1 | Adopt MH-21's TaskRevision verbatim or extend? (requirements Q1) | Adopt `v2contract.Verification` and `PolicyVersion` verbatim, extend nothing (D1) | Resolved by design |
| Q-2 | Which check runners exist? (requirements Q2) | One argv runner; definitions are the digest-bound `[[checks]]` | Resolved from main |
| Q-3 | Receipt shape: extend v1 or move to a v2 receipt? (requirements Q3) | Extend additively, `schema_version` unchanged (D8) | Resolved from main |
| Q-4 | Does slice 1 need envelope hooks? (requirements Q4) | No; render the stored bounds with an `enforced` flag (D9) | Resolved by design |
| Q-5 | Unavailable checks end `ready_for_review`/`unverified` (v1 machine) rather than v2 §11.2's `blocked`/`verification_unavailable`. | Keep the v1 representation (D5); migrate with the supervisor run-lifecycle migration | Maintainer; Task 5 |
| Q-6 | AC-4.3 as refined ("admission to the P1 flow shall require the qualified route") contradicts AC-3.1 (declared runs continue alongside P1) and AC-2.3 (the run records its P1 pin). | AC-4.3 is reworded in this spec to the qualified-delivery reading: strict `subscription-only` is the qualified path; declared and scripted runs stay admissible, carry the pin and are labelled unqualified (D11). The alternative refuses them and ends the dogfood flow. Merging the spec PR approves the rewording | Maintainer; Tasks 7, 13 |
| Q-7 | Verified runs recorded before schema 8 are `unbound` and need `--accept-unverified` at apply. | Strict (D7); the alternative is grandfathering them | Maintainer; Task 6 |
| Q-8 | Ship a re-evaluation command for a stale verdict? | No (D13); the stale run downgrades to unverified and the user decides | Maintainer; follow-up card |
| Q-9 | Protect the test files and scripts a check executes (AT-22 residual)? | No mechanism exists; recorded in the honesty register; propose a follow-up card | Maintainer |
| Q-10 | ADR number: 0017 is claimed by `specs/*/windows-process-ownership/` and 0018 by `specs/*/codex-native-adapter/`; this spec uses 0019. | Take the next free number at Task 2's start and update the references | Task 2 |
| Q-12 | A run admitted before the upgrade and recovered afterwards has no pin: it ends `NOT RUN`/`pin_missing` instead of running its checks (D19). | Accept; the alternative pins the evaluator after the candidate exists | Maintainer; Task 5 |
| Q-13 | Check-runner faults other than cancellation (start failure, evidence write failure) now leave the candidate unverified instead of `failed` (D18). | Accept (candidate stays reviewable); the alternative keeps `failed`/`verification_unavailable` | Maintainer; Task 5 |
| Q-14 | A run left `verifying` because the verification record could not be written: if every admitted check has a journaled `check.completed`, recovery records a verdict from them (`record_unavailable` unless one failed) and never re-runs the checks (D21); if not, it stays `ErrOwnership`, exit 6, as today. | Accept; the alternative lets recovery re-run the admitted checks, repeating their effects | Maintainer; Task 5 |
| Q-15 | An unreadable acceptance pin ends the run `failed`/`acceptance_pin_unreadable` (D24), and an `invalid` verdict ends it `failed`/`verification_invalid` (D23); both are not appliable. | Accept; the alternative leaves the run `verifying` or offers `--accept-unverified` | Maintainer; Task 5 |
| Q-16 | Task 2 mixes core files with the ADR (docs domain). | Left as one task: the same agent owns both, and supervisor-migration Task 8 did the same (maintainer decision, recorded here) | Maintainer; Task 2 |
| Q-11 | Task 5 and #328: repair/replan/transport ceilings do not bind a live run. Slice 2 enforces them. | The receipt reports `enforced: false` for those three until then; Task 7's test is rewritten when #328 closes | Slice 2 |

## Research notes

- `refinement-log.md` records a Q-14 about waiting for MH-21; that is resolved (MH-21 shipped, D-90). This table's Q-14 is a new question.

- Re-grounding (2026-10-11, origin/main 31cb991): MH-21 shipped (D-90), MH-16 S1 shipped, MH-13 code merged (11 tasks) with its spec in `unfinalized/`, MH-12 implementing with Tasks 5 and 7 held. Issues #72-#76 all OPEN (`gh issue view <n> --json state`).
- `importRuns` skips any run with a `task_revisions` row (`internal/migrate/apply.go:573`), so a pre-existing row would make a later `migrate --apply` skip that run; `ImportRun` itself writes the row when it imports a run without a marker (`internal/migrate/import.go:170-176`), which is why D2 writes no row for new runs.
- The TUI already withholds "verified" unless the passed row's candidate equals the frozen one (`internal/tui/mission.go:17`); the apply gate did not. Task 6 closes that asymmetry.

## Discoveries

- Task 2 (Q-10 resolved): ADR 0019 is free (`docs/decisions` holds 0001-0016; 0017 and 0018 are claimed by in-flight specs), so every later reference to `docs/decisions/0019-protected-acceptance.md` stands. Migration 0008 also sets `migration_state.schema_version` to 8: that column is pinned to `SchemaVersion` by `TestMigrationStateStartsNotStarted`, and `internal/migrate/SUPPORT.md` names `journal.TestSchemaVersionIs8` as evidence, so a later schema bump renames that test and the SUPPORT.md row again.
- Task 1: `Result("NOT RUN").Unverified()` is false by design (the spec says exactly `not_run` and `unavailable`); renderers label the payload value, gates test the stored `result` column.

- Task 3: a boundary-less host (user namespaces denied) skips the contained-run leg, so the independent read-only digest check runs without the boundary (`EvaluatorFor` needs none) and keeps the `ReadOnly` mutant red everywhere.

- Task 4: a pin is built from the admission `Decision`, so a recovered run rebuilt by `recoveryDecision` must never recompute it; `conclude` reads it with `ReadPin`. A conflicting `policies` row surfaces from the admission transaction's append, not from `NewAdmissionPayload`, so `appendAdmission` tracks the policy error separately from the quota-hold error to block with `acceptance_pin_failed`.

- Task 5: a contained native cannot launch without HOME (`resolveLayout` in `internal/contain/enter_linux.go`), so the runner's "contained checks need HOME" refusal is reached only on a recovery whose process has no HOME: the check environment is rebuilt from the recovering process (`recoveryDecision`), not journaled. A recovery that rebuilds a different check environment from the admitted run also changes the bound `environment` identity; D4 already keeps it out of the apply comparison. A row-less `verifying` run with a complete `check.completed` set is recorded `record_unavailable` without a ran evaluator, so its row has no evaluator digest and no attestation.

- Task 8: the scan found 28 distinct types (`run.result` comes from the renderer, not the journal). `recover --format jsonl` shares `finish`, so the same refusal applies there. The fake `allowance-exhausted` scenario runs under `local-scripted` billing through the CLI and does journal `attempt.native_error`.
- Task 6: the gate trusts the immutable binding row and the append-only admission event; the changeable inputs it compares against are the candidate row (`candidates.candidate_commit`) and `runs/<id>/task.md`, which the tests tamper with. A resumed apply (journaled intent) skips the gate and so cannot record an override; the intent payload does not carry one (hand-off).

- Task 7: the receipt's `unknown_reasons` table (`internal/supervisor/unknown_reasons.go`) lists only leaves a real receipt can render unknown: `requested_outcome.task_file_sha256`, `admitted_snapshot.source_repo`, `execution_bundle.adapter` and `billing.mode` come from required run columns, so they have no entry (an entry no fixture reaches fails `TestEveryUnknownHasAReason`). The walk found `requested_outcome.title` unexplained on its first run, which is the case the test exists for.

- Task 10: `Result("INVALID").Label()` is `unknown` (the label `INVALID` is not a stored value), so a surface that holds a receipt label must print it, never re-label it; only a stored or journaled result goes through `Label()`. The review `Acceptance:` line repeats the waiver reason the `Verification:` line carries, because design §2.7 prints the gate's reason there.

## Validation findings left open (run 3, round 1: Ready for implementation, 0 blocking)

Ten advisory findings are open for the implementing tasks; the first two are worth settling before Tasks 5 and 7 start.

1. Runner-error path (Task 5): use `Decide` when the ran evaluator digest is set and `DecideStatuses` otherwise; state that the path records the verification row, a binding row (reason, no attestation) and the event, and assert the stored row and the receipt's `verification.label` in `TestFailedCheckSurvivesRunnerError`.
2. Unreadable pin (Task 7): `BuildReceipt` renders `acceptance.policy_pin: unknown` with reason `acceptance pin unreadable` instead of failing, so `settleRun`, `review` and recovery's receipt repair still work; add a leg showing that run has a receipt and `review` exits 0.
3. `TestEveryUnknownHasAReason` (Task 7) needs an internal test for the broken-input and fallback legs and an external one over the run fixtures, or an `export_test.go`.
4. ADR number: say "next number neither on disk nor claimed by an in-flight spec", and use the `*-protected-acceptance.md` glob in every command or have Task 2 rewrite the later references.
5. Anchor the ADR checks: `^- Status: proposed`, and the decision IDs only inside the `## Decision` section.
6. NFR-1: add a Task 7 leg for `acceptance.overhead_ms` and `overhead_covers` in the receipt.
7. Honesty register: add a row for v2 §11.3 review contents (native route, fidelity changes, resource usage, authority needed) that plain `review` still omits.
8. Task 6 budget should be `complex` (9 acceptance items, replaces the apply gate).
9. Task 7 Invariants touched should include I21.
10. Task 2: name the test that opens a schema-7 database and asserts the preview step list is exactly `[0008_acceptance.sql]`.
