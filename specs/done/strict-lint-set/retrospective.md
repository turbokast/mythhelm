# strict-lint-set — Retrospective

## Review Summary

- **Range**: dee8695..0aef6db (PRs #170, #171, #172, #173, #174, #175, #176, #177; fix PRs none)
- **Reviewer**: code-reviewer, architect
- **Findings**: critical 0, important 0, suggestion 1 (confirmed); rejected 0
- **Open critical**: 0
- **Vendor review**: skipped (codex/muse/jev all unavailable — disabled)

| Severity | Finding | Anchor | Disposition |
|---|---|---|---|
| suggestion | Two red-baseline count nits in task entries: task 6 "15 hits" vs 16 listed revive locations; task 6 "6 more on Task 7 paths" vs task 7's measured 5 (already self-noted in the task 7 entry) | `tasks.md:180,207` | no edit — frozen shipped record; per-linter totals reconcile to design's 48/3/10/2/32 and task 8's zero-findings run is ground truth |

Foreign changes in range: none.

## Acceptance

| Criterion | Result | Evidence |
|---|---|---|
| AC-1.1 trial matrix | met | Design §2: 10 linters at v2.13.2, uncapped command, counts/verdicts |
| AC-1.2 costed verdicts | met | 8 enables + testpackage/err113 rejects with costs, checkpoint-confirmed |
| AC-2.1 winners + zero findings | met | 13 enables (5+8); 0 issues at v2.13.2 (PR #177) |
| AC-2.2 scoped exclusions only | met | Config diff is exactly the 8 enable lines; G204 pre-existing |
| AC-2.3 CI lint green | met | ubuntu-latest, green in 44s (run 37500309996) |
| AC-3.1 fixed at source | met | 0 new //nolint; red→green per task recorded (PRs #170-176) |
| AC-3.2 tests + behavior | met | Full suite green 247.2s; every semantic hunk mapped; no follow-ups |
| AC-4.1 justification text | met | Design §6; PR #177 quotes verbatim incl. placeholder |
| AC-4.2 badge untouched | met | No assessment files in range |
| NFR-1 time budget | met | 5-minute budget in scratchpad Q1; timeout-minutes: 5 on lint job only |
| NFR-2 offline free tooling | met | Free local v2.13.2 binary; documented command |

## Deviations

- Tasks 1–7: None recorded; review confirmed (47 Go files match design §4 exactly, fallout-first ordering honored).
- Task 8: scratchpad Q1 answered ("5 minutes (maintainer checkpoint 2026-10-06)") — recorded, checkpoint-sanctioned under NFR-1 itself.
- Review found one cosmetic class: red-baseline count nits (task 6 "15 hits" vs 16 locations; task 6 "6 more" vs task 7's measured 5, self-noted) — frozen record stands; task 8's zero-findings run is ground truth.

## CI history

- CI + zizmor + PR title + OSV-Scanner on PR #170 @3863cad: infra, runs cancelled — superseded by push; full set success on the later head.
- CI + OSV-Scanner + PR title on PR #171 @1bca71f: infra, runs cancelled — superseded by push; full set success on the later head.
- CI on PR #172 @629ef0b: infra, run cancelled — superseded by push; full set success on the later head.
- CI + zizmor + OSV-Scanner on PR #173 @c4ae48c: infra, runs cancelled — superseded by push; full set success on the later head.
- CI + OSV-Scanner on PR #174 @3cfc8cc: infra, runs cancelled — superseded by push; full set success on the later head.
- CI + OSV-Scanner + PR title on PR #175 @c4fa746/@568b205: infra, runs cancelled — superseded by push; full set success on the later head.
- CI + OSV-Scanner on PR #176 @779f08f: infra, runs cancelled — superseded by push; full set success on the later head.
- PR #177: no non-success runs on any head.
- Run-events fail/lost rows (worker-level, not CI): 11:54 tasks 2+4 fail (environmental) and tasks 1+3 lost (orchestrator-cancelled), 12:00 tasks 1+2 lost — box-wide EMFILE (file-descriptor exhaustion) killed the 6-worker parallel wave mid-run; serial re-dispatch from 13:55 ran clean. Mechanism: parallel subagents × Go builds/tests exhausted box FDs; recovery was cancellation to one worker. See P-strict-lint-set-1.
- No failure+success on the same headSha: no nondeterministic failure. `finalize.py verify` emitted no `note=history:` lines.

## Effort

dispatched=14 returned=8 failed=6; attempts 14 over 8 tasks; first-pass 4/8; review rounds 5; wall-clock 2026-10-06T11:29:49Z → 2026-10-06T17:16:32Z

| Task | Agent | Attempts | Review rounds | PR | Merged | First pass |
|---|---|---|---|---|---|---|
| 1 | go-implementer | 3 | 0 | #170 | yes | no |
| 2 | go-implementer | 3 | 1 | #171 | yes | no |
| 3 | go-implementer | 2 | 0 | #172 | yes | no |
| 4 | go-implementer | 2 | 1 | #173 | yes | no |
| 5 | go-implementer | 1 | 1 | #174 | yes | yes |
| 6 | tui-implementer | 1 | 0 | #175 | yes | yes |
| 7 | tui-implementer | 1 | 1 | #176 | yes | yes |
| 8 | release-engineer | 1 | 1 | #177 | yes | yes |

## Lessons

- What worked: task-file-scoped lint verification (per-task paths vs tree-wide) proved zero findings per task before the config named the linters — the fallout-first order (D4) kept main green at every merge.
- What worked: evidence-backed review rebuttals settled 3 vendor threads with zero code churn (newexpr trial-clean, vacuous-escape pre-existing, specs-domain premise wrong), while 2 architect reviews covered the journal/billing-adjacent tasks.
- What worked: independent parallel renames converged (real→canonical, max→limit) — the domain split did not fragment conventions.
- What to change: the 6-worker parallel wave collapsed the box (EMFILE) and cost 6 failed attempts plus a 2-hour serialize-and-retry — dispatch needs an FD-headroom gate before parallel waves (P-strict-lint-set-1).
- Follow-up (not a proposal): runspec.py silently drops result-less merge rows from Merged/First-pass (issue #188); two rows needed provenance backfills during this finalize wave.

## Proposals

- P-strict-lint-set-1 — gate parallel run-spec waves on FD headroom

Acceptance (spec-wide, adjudicated at head 0aef6db): AC-1.1 met (design §2: 10 linters at exactly v2.13.2, uncapped command, counts/verdicts); AC-1.2 met (8 enables + 2 costed rejects, checkpoint-confirmed); AC-2.1 met (13 enables = 5+8, each once in the block; 0 issues at v2.13.2); AC-2.2 met (config diff is exactly the 8 enable lines — no new settings/exclusions; G204 pre-existing); AC-2.3 met (lint job ubuntu-latest, green in 44s); AC-3.1 met (0 new //nolint; red→green per task recorded); AC-3.2 met (full suite green 247.2s; every semantics-touching hunk mapped to its finding; no follow-ups needed); AC-4.1 met (design §6 text; PR #177 quotes it verbatim incl. placeholder); AC-4.2 met (no assessment files touched); NFR-1 met (5-minute budget in scratchpad Q1, timeout-minutes: 5 once on the lint job, local 5.08s / CI 44s); NFR-2 met (free offline v2.13.2 binary; documented command); DoD met. Seams: conventions converge across all 7 fallout tasks (Errorf→errors.New verbatim, AsType uniform, SplitSeq/range-int element-identical, real→canonical + max→limit agreed independently, identical empty-block idiom, godot-clean comments); exactly one helper extraction (recordApplyEvent, same test package); no smuggled behavior (appendAssign in test fixtures with revert-retrip proof; one proactive range conversion recorded). Invariants touched: None ×8 — confirmed (comments/renames/binds/equivalent simplifications + config-only enablement). Design vs shipped: task 8's scratchpad Q1 answer (recorded, checkpoint-sanctioned); the suggestion-1 count nits; task 6's proactive conversion (recorded); otherwise none — 47 Go files match design §4 exactly, fallout-first ordering honored. Stale documentation: none (MH-20 card refresh belongs to pm-sync; automation row names all eight; remaining golangci mentions are generic).
