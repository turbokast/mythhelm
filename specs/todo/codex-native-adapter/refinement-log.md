# Refinement log — codex-native-adapter

dispatched=4 returned=3 failed=1

The failed dispatch was a round-2 assessor stopped by a container restart before it reported (no report, no verdict). It was not read as "no issues". A replacement round-2 assessor was dispatched on the same file and returned. The loop therefore ran three returned rounds; the strict "failed=0" condition of `/refine-spec` step 6 is not met, and the maintainer should weigh that when approving.

## Round 1 — Needs human input (D8 FAIL)

Fixes applied: O1 aligned to registry vocabulary; doctor, flag, seed and test-pin carve-outs; AC-1.3 and O4 exceptions; N4/AC-6.2 contradiction; shared consult block codes in AC-4.1; usage non-overlap rule; exhaustion classification AC; Claude-bound call-site list derived with its command; docs target moved to `docs/limitations.md`; non-goal N7; dependencies on `contained-execution-profiles` Task 11 and `budget-ledger-s1`.
Human-required items recorded as OQ6 to OQ9 with defaults and a decider (non-interactive; not guessed).

## Round 2 (second dispatch) — Needs work

Fixes applied: O1/O4/N2/AC-1.3/DoD contradictions; AC-1.5 versus AC-2.3/AC-6.2 (method-call clauses dropped); three-field interrupt report (AC-5.2); testable AC-7.3 and AC-7.5; closed auxiliary list (AC-4.2); non-native auth key AC-4.7; OQ10 for vendor usage and error shapes; OQ defaults and deciders; `supervised-stop-recover` Task 4 and `claude-strict-subscription` Task 5/7 sequencing.

## Round 3 — Needs work, no FAIL (good enough)

Fixes applied: AC-4.1 `stop_at_exhaustion_unproven`; AC-4.3 code and field; AC-1.5 wording against existing `InterruptReport` fields; `acknowledged` kept off the journal event; AC-5.3 sole-writer structure; per-flag decisions in AC-1.3; AC-3.4 sentinel test; `adapter.Capabilities` disambiguation; OQ3 decider.

## Remaining notes for /spec

- OQ1, OQ2, OQ6, OQ7, OQ8, OQ9, OQ10 are vendor or policy facts held at conservative defaults; the design must not assume them.
- Scope: spans core, adapters and docs across the `internal/`/`adapters/` seam; `/spec` scope step should consider an epic split (FR-1 plus seam changes, then the Codex adapter) and an `architect` review.

## Post-refinement edits during /spec validation round 1

Three requirement wordings changed so they match what can be built, each recorded here rather than silently: AC-3.1 (the bounded `--version` probe is the one launch before trust; the zero-launch check counts model-task launches), AC-5.3 (second `Start` on the same adapter instance, refused before launch; cross-worker guarantee rests on no resume path), AC-4.6 (`rate_limit` is the internal class for v2 `provider_throttled`). The stale "Unrefined" banner was removed.

## Reversal during /spec validation round 2

AC-3.1 was reverted to forbid any native process before trust (design §4 inventories and trusts before the probe); the earlier note above about allowing the `--version` probe first no longer applies. AC-2.2 was reworded so `Probe`, not `Prepare`, blocks an untested version.

## /spec validation record

Counts below are validator dispatches only; the refine-loop dispatch accounting (dispatched=4 returned=3 failed=1) is at the top of this file.

| Round | dispatched/returned/failed | Verdict | Findings |
|---|---|---|---|
| 1 | 1/1/0 | Needs revision | 20 (8 blocking); applied |
| 2 | 1/1/0 | Needs revision | 16 (4 blocking); applied |
| 3 | 1/1/0 | Needs revision | 14 (1 blocking, 13 advisory); applied, not re-validated |

The three-round cap of `/spec-fix-and-report` was reached with a "Needs revision" verdict on round 3. All round-3 findings were fixed in the text, but no fourth validator ran, so the spec has no "Ready for implementation" verdict and stays in `specs/refined/` instead of moving to `todo/`. A maintainer decides whether to run `/spec codex-native-adapter` again for a fresh validation.

## Fresh validation round (authorised by the maintainer, Q-19)

| Round | dispatched/returned/failed | Verdict | Findings |
|---|---|---|---|
| 4 (fresh context, worktree at origin/main `fd68167` merged) | 1/1/0 | Ready for implementation | 18 (0 blocking, 18 advisory) |

Lint (specs) exit 0 in the same round. The ADR in Task 12 was renumbered from 0016 to 0018 before the round (0016 is `0016-migration-import.md` on main; the MH-11 spec claims 0017). No selector dry-match applied: the spec embeds no `go test -run` command. The 18 advisory findings were not applied in this round (one validation round was authorised, and an edit after a Ready verdict would be unvalidated); they are listed in the pull request description. The validator recommended fixing findings 1 to 4 (OQ4 wait on `claude-strict-subscription`, Task 2 e2e refusal file, stale dependency statuses and baseline, Task 11/8 fixture file) before `/run-spec`.
