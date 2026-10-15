# TUI Observatory View — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| Q1 (resolved) | Distinguish lost-terminal client result from successful canonical run outcome. | Maintainer approved 2026-10-15: preserve canonical outcome; map client0 to6 after terminal loss and retain nonzero run failures. Deliberate second Ctrl-C keeps AC-6.9's outcome. AC-6.13 amended narrowly to state this. | Resolved; concrete ADR approval still belongs to Task3. |
| Q2 | Is an independent non-locking owner observation qualified on this deployment? | No. Production stale/missing beat renders supervision unknown. Test the conditional unsupervised path without claiming a platform probe. | Future platform owner; nonblocking because AC-4.12 explicitly permits unknown. |
| Q3 | Who records human terminal/accessibility evidence? | Task20 is the maintainer's synthetic demo re-record only; NVDA/VoiceOver/Orca and real light/dark qualification stay slice4. | Maintainer; finalization waits Task20, no agent evidence substitution. |

## Research notes

- Author: native Codex agent in the portable spec workflow, 2026-10-15; base origin/main `30062ec`. No native paid inference, account switch or live benchmark was used for specification authoring.
- Scope: single spec, auto-confirmed in non-interactive dispatch, honoring explicitly confirmed OQ-21. Twenty bounded tasks preserve contract-before-consumer and one-domain ownership; Tasks1–10 are the lifecycle prefix. Requirements changed only at AC-6.13 to record the maintainer-approved client/run exit distinction; refinement log remains historical and unchanged.
- Grounding: read pipeline/watch/conclude/recover, worker handoff/supervise/requestStop/envelope, ownerlock Acquire, TUI model/nav/dialogs/actions/palette/help/live/mission/styles/theme/viewmodel, journal row types, source frames1a/1f/1g/1h/1i, ADRs0004/0005/0007, current master lifecycle/UI/extensions/gates and authoring/domain rules.
- `rg -n 'Launch\{' internal tests` identified actual readLaunch/spawn fixtures in worker_test.go, supervisor faults_test.go and pinning_test.go, and admission launch_fixture_test.go; all are in Task4. Containment/config helper-only literals do not deserialize a handoff and are not blindly migrated.
- `rg -n 'Tokens\.Warning|t\.Warning|Warning:' internal/tui` identified mission.go, diff.go, actions_test.go and theme implementation/tests; Task11 migrates them together so schema removal compiles.
- `rg --files docs/demos` confirms existing tui.tape, tui.gif, tui.transcript.txt, tui-poster.png and manifest.json; Task20 owns all changed derived artifacts.
- Supervision research: [Linux man-pages proc_locks(5)](https://www.man7.org/linux/man-pages/man5/proc_locks.5.html), read 2026-10-15, documents FLOCK entries and PID-namespace filtering. Absence is insufficient for a universal no-owner claim. No probe takes the exclusive owner lock, even momentarily.
- Deadline interpretation: AC-6.11 explicitly requires fenced and unfenced deadline workers to exit. Its more specific condition exempts deadline-terminal workers from the general await-reconnect residual; the honesty register preserves that residual for non-deadline episodes. No requirement text changed for this interpretation.
- Ordinary controls and human approval checkpoints remain distinct. The user delegated routine design judgment; no delegation creates a signed product approval or approves the ADR's future exact contents.
- Benchmark scope here is renderer performance only (ten non-race samples, median ns/op/allocations, bounded-row acceptance). It is not apples-for-apples native harness evidence and cannot support a MYTHHELM-vs-CLI advantage claim.

## Validation

Mechanical spec lint passed on 2026-10-15. Embedded go-test selectors: none in the authored design/tasks/scratchpad (named future tests are acceptance contracts). Round1 fresh-context verdict: Needs revision (references PASS, requirements PASS, design FAIL, tasks FAIL, conflicts PASS, epic n/a). Two blocking findings fixed: demo retention/recovery coverage and missing exit.go in Task5. Added lifecycle Task9, shifted subsequent task IDs, and added packaged demo continuity/explicit cleanup proof to Task10. Round2 fresh-context verdict: Ready for implementation, all checks PASS (epic n/a), with one advisory to raise Tasks17/18/20 from standard to complex because each owns six files. Those budget corrections are applied; final mechanical lint and round3 confirmation both passed; the spec is Ready for implementation.

- Round1 grounding: demo.go unconditionally defers RemoveAll for its private repo/state and reviewOK checks only Detached; recover reads statedir.Resolve, so a bare command points at caller state instead. Added Task9 retention and shell-safe explicit state-root guidance; operator cleanup remains separate from ordinary recover, as coordinated with the delivery agent.
- Round1 sweep: reindexed all singular/plural task references, lifecycle prefix now1–10 and total20; Task5 now owns exit.go where runExit is defined. Spec lint rerun after the sweep.

### Round 2 — Fresh validator report

Lint (specs): exit 0; clean. Embedded test selectors: none; no test binaries executed. Vendor: not run. Files edited: none.

### Spec review: tui-observatory-view

Verdict: Ready for implementation

| Check | Result | Findings |
|---|---|---|
| 1 References | PASS | Existing interfaces and task paths support the design. |
| 2 Requirements | PASS | Numbered requirements, binding non-goals and DoD coverage present. |
| 3 Design | PASS | Lifecycle contracts, demo retention, recovery destination and deferred capabilities addressed. |
| 4 Tasks | PASS | One budget advisory below; no blocking implementation gap found. |
| 5 Conflicts | PASS | Overlapping active specs have serialization/revalidation notes. |
| 6 Epic | n/a | Explicitly retained as one spec under OQ-21. |

Findings: 1. [advisory] Check 4 — tasks.md, Tasks 17, 18 and 20: Each lists six files but declares Budget: standard. The decomposition procedure assigns six-or-more-file tasks complex. Change these budget declarations to complex; this does not require a scope split.

No blocking findings. The demo retention/recovery coverage and Task 5’s internal/cli/exit.go ownership resolve the identified lifecycle gaps.

Author disposition: all three budgets changed to complex; spec lint exit 0 after the correction. Round 3 checked the corrected files before the move.

### Round 3 — Final fresh validator report

Lint (specs): exit 0; clean. Embedded test selectors: static extraction found no go test commands in task text; no test binaries executed. Vendor: not run. Files edited: none.

### Spec review: tui-observatory-view

Verdict: Ready for implementation

| Check | Result | Findings |
|---|---|---|
| 1 References | PASS | Task paths exist or create files in existing directories. Runtime, theme and view-model contracts agree with the inspected source. |
| 2 Requirements | PASS | Numbered requirements, binding non-goals, concrete acceptance criteria and Definition of Done cover the slice. |
| 3 Design | PASS | Ownership, deadline enforcement, demo retention, recovery guidance and presentation contracts are specified; remaining limitations are recorded. |
| 4 Tasks | PASS | Dependencies precede consumers, ownership is separated, larger file sets are justified, and Tasks 17/18/20 now use complex budgets. |
| 5 Conflicts | PASS | Active worker and CLI overlaps have serialization and revalidation instructions. |
| 6 Epic | n/a | Single-spec scope is explicitly retained under OQ-21. |

Findings: None. No blocking or advisory findings identified. Concrete ADR approval remains Task3 implementation checkpoint; verdict does not replace it.

Accounting across three fresh-context reviewers: dispatched=3 returned=3 failed=0. Final mechanical spec lint exit 0. No native-account trial or implementation was performed. Lifecycle move uses the shared script only after this Ready verdict.

## Hosted review round 1

- Recovery-command finding confirmed: existing review argument validation is broader than a shell-safe token. Design §3 and Task9 now require bounded full run grammar, rejection of controls/metacharacters, and independent target-shell quoting of both arguments. Kept malicious-ID/path fixtures test the executable effect.
- Terminal-loss apply finding confirmed: current tui.Run calls cancelAction unconditionally after Tea returns; actionCtx derives from view lifetime. Design §3 and Tasks7/8 now specify independently owned/joined action lifetime, headless completion callback and canonical action-result mapping in both live and inspect modes; explicit double-quit retains cancellation/join.
- Mixed-version finding is a real compatibility limit; accepting absent mandatory execution_deadline is rejected as a fix because AC-6.11 explicitly requires fail-closed rejection and I21 forbids inventing an unlimited envelope. Design §2, Task4 and Task19 now test/document both strict-decoder directions, retained running-worker evidence and the unsupported admission-to-spawn binary-replacement window. No automatic upgrade compatibility or new authority is claimed; the future ADR still requires maintainer approval.
- Source anchors re-read: internal/cli/review.go validation, internal/ids/ids.go generated grammar, internal/tui/model.go Run cleanup, internal/tui/actions.go actionCtx/cancelAction, internal/cli/tui.go wireActions, internal/workers/worker.go Spawn/readLaunch, ADR0004 private stdin handoff and v2 §17.2 upgrade demand.
- These are hosted-review corrections after the completed three-round specification validation. Independent review of the changed design/tasks is arranged by the delivery coordinator; no implementation started.

## Discoveries

None yet; implementation has not started.
