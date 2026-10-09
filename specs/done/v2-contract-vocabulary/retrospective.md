# v2-contract-vocabulary — Retrospective

## Review Summary

- **Reviewer**: read-only code-reviewer subagent; findings adjudicated against the code by the delivery shepherd.
- **Open critical**: 0.
- **Range**: `9f2d770..6717345` (tasks 1,4,5,3,7,2,6,8 via PRs #234, #237, #238, #239, #241, #244, #243, #264). **Foreign**: none.
- **Package tests at head**: `go test ./internal/v2contract/` → ok. Lifecycle tables and the 24-code catalogue verified line-by-line against `MYTHHELM_Master_Spec_v2.md` §6.1/§6.2/§4.5 — all match.
- **Findings**: critical 0 / important 2 / suggestion 3. Open critical: 0.
  - F1 (important): `Attempt.Validate` never required `LaunchID`, so reconcile-with-no-identity was accepted against AC-2.3 — fixed in #268 (`TestAttemptRequiresLaunchID`).
  - F2 (important): `Run`/`TaskRevision`/`Attempt.State` still `string`, not the lifecycle types; Task 2's entry deferred the retype to "Task 5 or 8" and neither did it — fixed in #268 (`TestRecordLifecycleMustBeInVocabulary`).
  - F3 (suggestion): `MapAdapterFailure` validated nothing (empty namespace, unknown code) — folded into #268 (`TestMapAdapterFailureRejectsBadInput`).
  - F4 (suggestion): AC-1.3's "distinguishable by type name" clause is ambiguous about which type names; kept as a Lessons note.
  - F5 (suggestion): reconcile refusal message wording; kept as a Lessons note.
- **Vendor review**: not run (single-package pure-contract change; per-task CodeRabbit rounds covered each PR).

## Acceptance

| Criterion | Result | Evidence |
|---|---|---|
| AC-1.1 14 records, snake_case tags, schema_version = 2 | met | Tasks 1, 2, 8; PRs #234, #244, #264 |
| AC-1.2 new immutable revision on material change | met | Task 2; PR #244 |
| AC-1.3 v2 payloads versioned, distinguishable from receipt schema_version | met | Task 4; PR #237 (F4 notes the clause wording) |
| AC-1.4 distinct capability/qualification/verdict vocabularies | met | Task 1; PR #234 |
| AC-2.1 reject transitions outside §6.1/§6.2 tables | met | Task 3; PR #239 (`TestRunTableMatchesSpec`) |
| AC-2.2 no terminal state on unresolved ownership | met | Task 5; PR #238 |
| AC-2.3 same launch identity on reconcile | met | Task 5 + fix; PRs #238, #268 |
| AC-3.1 control-error shape, code-driven transitions | met | Task 6; PR #243 |
| AC-3.2 adapter mapping, never-widening disposition | met | Task 6 + fix; PRs #243, #268 |
| AC-4.1 v2 envelope, sequences, generation fencing | met | Task 7; PR #241 |
| AC-4.2 late observations quarantined at most | met | Task 7; PR #241 |
| AC-9.1 golden fixtures, malformed/unknown keys, nulls | met | Tasks 7, 8; PRs #241, #264 |
| AC-9.2 support matrix, no guessed live-qualified claims | met | Task 8; PR #264 |

13 met, 0 partial, 0 unmet.

## Deviations

- Task 1: None.
- Task 2: `codec_test.go` lost the `//nolint:unused` on `loadGolden` (now called); `State` fields left `string` with a "Task 5 or 8 retypes them" promise that neither kept — closed by the fix PR #268.
- Task 3: `codec_test.go` `//nolint:unused` removal (called for the first time).
- Task 4: `codec_test.go` `//nolint:unused` removal; comment-only.
- Task 5: "active phase" read as the 7 forward phases admission…applying; pinned in `TestRunTableMatchesSpec`.
- Task 6: None.
- Task 7: `codec_test.go` `//nolint:unused` removal (called for the first time).
- Task 8: `run.golden.json` budget renamed `tokens` → `gpu-hours`/`hours` (the NFR-4 value scan flags `token` in values); no test changed.
- Fix #268: F1/F2/F3 closures above; no stated requirement contradicted.

## CI history

- t1..t5, t7, t8, fix branches: every workflow run green; `cancelled` entries are superseded pushes, not failures.
- Merge 85049b2 (PR #241): CI/(run) job-failed, class=infra (from `finalize.py verify` history note).
- No same-`headSha` failure+success pair on any branch: no nondeterministic failure to explain.

## Effort

dispatched=8 (one attempt-1 dispatch per task; the 9th dispatch row is the post-review seams fix, not a task attempt), returned=7 (t8 has no return row: it went dispatch → verify → maintainer merge #264 without one), failed=0 (no fail rows, no attempt-2 dispatch anywhere); first-pass 8/8 (all 8 tasks merged on attempt 1: #234 #237 #238 #239 #241 #243 #244 #264); review rounds 3 (+1 finalize-review fix PR); wall-clock 2026-10-08T23:31:47Z → 2026-10-09T09:05:46Z. Event-log gaps: the t8 return row and the t2 merge row (#244 is on main at 01f718f) were never recorded.

| Task | Agent | Attempts | Review rounds | PR | Merged | First pass |
|---|---|---|---|---|---|---|
| 1 | cloud-sonnet | 1 | 1 | #234 | yes | yes |
| 2 | cloud-sonnet | 1 | 1 | #244 | yes | yes |
| 3 | cloud-sonnet | 1 | 0 | #239 | yes | yes |
| 4 | go-implementer | 1 | 0 | #237 | yes | yes |
| 5 | go-implementer | 1 | 0 | #238 | yes | yes |
| 6 | go-implementer | 1 | 0 | #243 | yes | yes |
| 7 | go-implementer | 1 | 1 | #241 | yes | yes |
| 8 | cloud-sonnet | 1 | 1 | #264 | yes | yes |

(`runspec.py summary` prints task 2 as merged=no and task 8 as unaccounted — event-log gaps across the overnight sessions; both merged on main as #244 and #264. Task 8's agent shows `-` for the same reason; it was cloud-sonnet. Task 8's TOML review round postdates the summary snapshot.)

## Lessons

- What worked: one acceptance test per criterion, run before merge, caught nothing late — every task passed first attempt; the finalize review's contract-seam sweep (F1/F2/F3) caught exactly the cross-task gaps per-task review cannot see; `pr-check`'s Files-vs-deviations scope rule forced three undeclared files into the open on sibling specs the same night.
- What worked: cloud parallelization — the spec's tasks ran concurrently with four other specs' tasks with no cross-spec conflict.
- What to change: a completion entry that defers work to a later task ("Task 5 or 8 retypes them") with no owner rots silently until finalize (F2) → P-v2-contract-vocabulary-1.
- What to change: reviewing a stale branch with a tree-diff against main shows other specs' merges as deletions; the branch's own diff (merge-base..HEAD) is the reviewable unit — misread twice this run → P-v2-contract-vocabulary-2.
- What to change (notes): F4 (AC-1.3 "distinguishable by type name" names no names) and F5 (reconcile refusal message wording) are spec-wording suggestions with no behaviour gap; left for the next spec touching those files.

## Proposals

- P-v2-contract-vocabulary-1 — Track deferred cross-task promises to completion
- P-v2-contract-vocabulary-2 — Review stale branches by their own diff
