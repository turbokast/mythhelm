# tui-slice — Retrospective

## Review Summary

- **Range**: 41e38d5..3962954 (PRs #89, #91, #93, #90, #94, #95, #97, #98, #99, #100, #101, #102, #103, #104; fix PRs none)
- **Reviewer**: code-reviewer, architect
- **Findings**: critical 0, important 6, suggestion 12 (confirmed); rejected 1
- **Open critical**: 0
- **Vendor review**: skipped (all vendors disabled)

| Severity | Finding | Anchor | Disposition |
|---|---|---|---|
| important | Exit-options dialog rows never wired; `q` inert, users cannot exit | `internal/tui/dialogs.go:96` | issue #106 |
| important | Mid-run Ctrl-C cancels (exit 130) instead of detaching; detach unobserved on real runs | `internal/cli/tui.go:250` | issue #107 |
| important | TUI startup failure injects stop+detach into a healthy run (no `tuiErr` guard) | `internal/cli/tui.go:240` | issue #107 |
| important | Reattach line names `review`, pipeline names `recover`; stopping runs have no available action | `internal/cli/tui.go:257` | issue #107 |
| important | Blocked/failed runs told to "recover" but gate allows only `interrupted`; accessible says "no next step" | `internal/tui/mission.go:40` | issue #107 |
| important | Quit from working state silently abandons confirmed recover/apply | `internal/tui/dialogs.go:87` | issue #107 |
| suggestion | `SearchHistory` implemented and tested but unreachable from the UI | `internal/tui/history.go:21` | none required |
| suggestion | Traceability omits `TestHistorySearchComplete` for AC-6.2 | `docs/tui-slice-g09-evidence.md` §4 | none required |
| suggestion | UX session: 1.5/8 checks completed; states/billing/task-start/receipt undiscoverable | `docs/tui-slice-g09-evidence.md` §2 | none required |
| suggestion | Every 200 ms tick re-runs `git diff` + full parse unconditionally | `internal/tui/model.go:206` | none required |
| suggestion | `decodeLiveProgress` duplicates `decodeProgress` field-for-field | `internal/tui/motion.go:170` | none required |
| suggestion | `applyActionResult` clears `actCancel` without calling it (uncancelled ctx) | `internal/tui/actions.go:198` | none required |
| suggestion | Design launch order lists `--accessible` after non-TTY/dumb; code (correctly) precedes | `specs/done/tui-slice/design.md:25` | none required |
| suggestion | `theme.Load` has no non-test callers; no flag wires custom themes | `internal/tui/theme/theme.go:62` | none required |
| suggestion | Negative `--after` silently accepted (non-numeric correctly exits 2) | `internal/cli/tui.go:98` | none required |
| suggestion | `demo --accessible` prints review/done banners after the `next-after` trailer | `internal/cli/demo.go:150` | none required |
| suggestion | `verification: waiting` shown on terminal failed/blocked runs with nil verification | `internal/tui/mission.go:243` | none required |
| suggestion | Accessible next-action omits the unverified distinction for ready-for-review | `internal/cli/accessible.go:350` | none required |
| rejected | "Range holds 50 files, not 46" | range derivation | rejected: raw `base..head` diff includes out-of-spec commits; the 46 spec files in `range.json` are the reviewed set |

Foreign changes in range: 3eeabceb docs(spec): tui-slice task 1 pin file for tidy-clean requires (#92): specs/in-progress/tui-slice/tasks.md.

## Acceptance

| Criterion | Result | Evidence |
|---|---|---|
| AC-1.1 mission facts | met | `TestMissionShowsRequiredFacts`, `TestNextActionTable` (PR #97) |
| AC-1.2 real width-safe diff | met | Task-5 diff tests + `TestTruncationFooterNamesExactCommand` (PRs #94, #97) |
| AC-1.3 lane labels | met | `TestLaneLabels` (PR #97) |
| AC-1.4 text+shape/icon, ASCII alternatives | met | `TestStatusStripKindLabels`, `TestRenderAsciiFallback`, adversarial-theme test (PRs #94, #97, #100) |
| AC-1.5 no font requirement | met | `TestNoFontDependentGlyphs` (PR #97) |
| AC-2.1–2.4 layout ladder | met | `TestLayoutBreakpoints`, `TestShortHeightCompact` (PR #97) |
| AC-2.5 stable linear output | met | `TestLaunchRuleMatrix` + e2e linear/plain/jsonl tests (PRs #102, #103) |
| AC-2.6 resize safety | met | `TestResizePreservesIdentity` (PR #97) |
| AC-3.1 keyboard flows | partial | Key/palette/help tests pass; `q` opens the exit dialog and Esc closes it, but the dialog's detach/request-stop rows are inert (F-1, issue #106) |
| AC-3.2 mouse parity | met | `TestNoMouseOffered` (vacuous: mouse not offered, PR #98) |
| AC-3.3 labelled confirmations | met | Task-9 dialog tests, Cancel-first focus (PR #100) |
| AC-4.1 signature moments | met | Task-8 truth-rule tests, applicable subset (PR #99) |
| AC-4.2 reduced motion | met | `TestReducedMotionStatic` (PR #99) |
| AC-4.3 flags | met | Caps tests + `TestInvalidModesExit2`, `TestColorAlias` (PRs #89, #102) |
| AC-4.4 overrides, never COLORFGBG | met | `TestExplicitNeverWins`, `TestColorFgBgNeverConsulted` (PR #89) |
| AC-5.1 linear SR mode | met | Accessible label/chatter/cursor/order tests (PR #95) |
| AC-5.2 focus/help/colour | met | Focus-stability, full-name help, adversarial-theme tests (PRs #98, #100) |
| AC-5.3 paging/export/wide-safe | met | Resume-cursor, wide-safe, export tests (PRs #95, #100) |
| AC-5.4 claimed-only-if-tested | met | Evidence §3 marks all SR combinations experimental; README mirrors it (PR #104) |
| AC-6.1 coalesced, no idle loop | met | `TestCoalescedReload`, `TestNoPermanentLoop` (PR #101) |
| AC-6.2 virtualisation + searchable history | partial | `TestViewportVirtualises`, `TestHistorySearchComplete` pass, but `SearchHistory` has no UI caller |
| AC-7.1 requested≠stopped | met | `TestStopLabelsRequested` (PR #100) |
| AC-7.2 revision-attached verification | met | Mismatch + native-result tests (PR #97) |
| AC-7.3 kind-labelled figures | met | `TestStatusStripKindLabels` (PR #97) |
| NFR-1 G09 matrix recorded | met | `docs/tui-slice-g09-evidence.md` §1 (PR #104) |
| NFR-2 UX session | partial | Session run with a non-Vim engineer and fully recorded, but 1.5/8 checks completed; states/billing/task-start/receipt undiscoverable (evidence §2, PR #104) |
| DoD traceability + README status | met | Evidence §4 maps every AC to named merged tests; README names exactly the recorded combination (PR #104) |

## Deviations

- Task 2: `Snapshot` gains a `Goal` field the design §3 snippet omits — recorded in the task entry; feeds task 6.
- Task 5: `Diff` gains a `TotalLines()` accessor the §8 snippet omits — recorded; task 6's footer needs the pre-cap total.
- Task 6: truncation notice spans full view width; `NoticeBacklog` lands in `model.go`, moves to `live.go` in task 10 — recorded; layout necessity + planned move.
- Task 7: `Config` gains a `ThemeName` field — recorded; initialises toggle state from supplied tokens.
- Task 8: attempt-scoped launch-evidence folding + startup-cancel clean shutdown — recorded; fixes a retry-inheritance defect and an accessible race review/CI found.
- Task 9: off-loop supervisor calls via `tea.Cmd`, payload-confirmed stop-label retirement, wiring through mission/motion/nav/palette — recorded; review-driven correctness.
- Task 10: poll chain bootstraps from `Run`, compact notice ordering, `cancelAction` join — recorded; preserves task 8's motion assertions.
- Task 11: no `approval:` summary line (pipeline has no approval state); `EventsLimit`+`EventsSinceLimit` cap — recorded; truthfulness + overflow fix.
- Task 12: `--accessible` beats non-TTY/dumb legs (design §2 reads the other way); detach skips demo review; `Started` channel — recorded; forced by acceptance tests, sound.
- Task 13: `waitForRunJournaled` + detached-context shutdown drain (task-12 defects caught by e2e); load-bearing normalisation guards — recorded; fixes real races.
- Review-found: exit-options dialog rows never wired (design §2 breach) — found by review and G09, in no task's deviations; the one seam that fell between tasks 7 and 9/12 (issue #106).

## CI history

- PR #89 merge 5931af5: CI/(run) + OSV-Scanner/(run): infra — jobs never judged the code; tip green covers.
- PR #95 merge 5e2129d, push run 37064602507: real — `TestAccessibleResumeFromCursor`: "context canceled" reading the schema version. Same code passed on the branch, so timing-dependent. Mechanism: cancel landing in the accessible startup queries errored instead of shutting down clean; fixed by PR #99's clean-shutdown fix. Failing value: push run on 5e2129d; passing value: branch runs + all later runs.
- PR #103 head 8d24b94, `Go (windows-latest)`: infra — "hosted runner lost communication" after 48m, no test output; rerun green on the same sha. Mechanism: GitHub runner communication loss. Failing value: run 37108583857 job 111161887697; passing value: rerun job 111169672377.
- PR #103 push f8f6cd4 (docs-only entry commit): real — all five Go legs failed; the failure belongs to the parent code commit (first CI signal on the branch). Fixed by f6187da; logs expired, exact error unavailable.
- PR #102 branch heads 2b78e3a/31b27e7/31b4548: DCO sign-off failures, real and deterministic — worker commits unsigned. Fixed by signoff rebase 40ee90c.
- PR-title duplicate success runs across branches: re-trigger noise, never a code signal.

## Effort

dispatched=5 returned=0 failed=1 (local run-events log, partial: covers only the tasks 1–4 dispatches); attempts: task 1 took 2 (log `fail` event: go.mod pin blocked-decision, retried after spec PR #92), other tasks unknown (not in log); first-pass unknown; review threads 27 across 14 PRs, all resolved (from GitHub); review rounds unknown (log holds 2 rows); wall-clock 2026-10-02T18:17:52Z → 2026-10-03T23:00:11Z

runspec.py summary (same partial log):

dispatched=5 returned=0 failed=1
| Task | Agent | Attempts | Review rounds | PR | Merged | First pass |
| 1 | go-implementer | 2 | 0 | - | no | no |
| 2 | tui-implementer | 1 | 0 | #91 | no | no |
| 3 | tui-implementer | 1 | 0 | #89 | no | no |
| 4 | tui-implementer | 1 | 0 | #90 | no | no |
| 13 | - | 0 | 1 | #103 | yes | no |
| 14 | - | 0 | 1 | #104 | yes | no |

(Ground truth from origin/main: 14/14 tasks merged via PRs #89–#104; the table's `Merged: no` rows are missing log rows, not missing merges.)

## Lessons

- What worked: red-green review fixes with regression tests — e.g. the stranded-run shutdown hang was proven on old code (30 s timeout) before the fix (PR #103).
- What worked: G09 human sessions caught what automation missed — F-1 (inert exit dialog) and F-2 (cancel-vs-detach) both surfaced in maintainer/UX sessions, never in CI (PR #104).
- What worked: reply-and-resolve review discipline — 27/27 threads resolved with evidence, zero carried across merges.
- What worked: load-bearing e2e normalisation — byte comparisons guard that raw streams differ, so the normalisation cannot silently stop asserting (PR #103).
- What to change: stubs without owners fall between tasks — the exit dialog's "a later task wires its rows" named no task, and none did (proposal P-tui-slice-1).
- What to change: stubbed launch tests mask real lifecycle behaviour — `TestQuitLiveDetaches` passes against a fake while real Ctrl-C cancels (issue #107).
- What to change: worker commits missed sign-off and broke the DCO gate (PR #102; proposal P-tui-slice-2).
- What to change: run-events logging went incomplete across sessions, so effort numbers are unknown — dispatch/merge/round events must be recorded as they happen, never batched from memory.
- What to change: the first CI signal on a branch (PR #103 f8f6cd4) reads as belonging to the push commit even when the breakage is in the parent code commit — attribute early failures to the branch, not the push.

## Proposals

- P-tui-slice-1 — Tasks name the owner of every stub they leave
- P-tui-slice-2 — Dispatch template requires signed-off worker commits
- P-tui-slice-3 — Implementation skill carries the same DCO exception
