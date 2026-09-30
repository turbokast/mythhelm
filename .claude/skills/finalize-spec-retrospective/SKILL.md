---
name: finalize-spec-retrospective
description: Write a finalized spec's retrospective — acceptance criteria against what shipped, deviations, review findings, CI history with a mechanism for every nondeterministic failure, effort from the run-events log — and emit evidence-backed improvement proposals into .claude/proposals/pending.md
argument-hint: "<spec-name>"
---

# Finalize Spec — Retrospective

Step 3 of `/finalize-spec`. The retrospective is the spec's permanent record of how it went, and the input to the harness's learning loop: every lesson that points at a mechanism becomes a proposal a maintainer can accept.

## Input

`$ARGUMENTS`: the spec name. `specs/unfinalized/<spec>/retrospective.md` already holds the `## Review Summary` from `/finalize-spec-review`.

## Invocation contexts

- **Slash command**: writes the retrospective sections and appends proposals in the working tree it runs in (the finalize worktree under `/finalize-spec`); run alone, it writes the files and commits nothing.
- **Model-invoked**: the same.
- **Non-interactive**: the same. Choosing which lessons become proposals is an advisory step: apply the criteria below and record `auto-confirmed (non-interactive)`. Nothing here writes outside the spec directory and `.claude/proposals/pending.md`.

## Sources

Every number comes from one of these, never from memory of the run; a value that cannot be produced is written `unknown`.

| Section | Source |
|---|---|
| Acceptance | `requirements.md` criteria; the reviewers' spec-wide acceptance map; each task's Acceptance and its merged tests |
| Deviations | every task's `Spec deviations` in origin/main's `tasks.md`; the Review Summary's design divergences |
| CI history | `finalize.py verify` `note=history:` lines; for each task pull request, `gh run list --branch <its head branch> --json headSha,workflowName,conclusion,createdAt` (a failure and a success on the **same** `headSha` is a nondeterministic failure); `fail` and `retry` rows of the run-events log |
| Effort | `python3 scripts/harness/runspec.py summary --spec <spec>`; the first `run_start` and last `merge` rows of `.claude/data/run-events.jsonl` (in the main checkout) for wall-clock |

## Steps

1. **Write the sections** after the Review Summary, in this order, each always present (`None` or `No data` when empty):

   ```markdown
   ## Acceptance

   | Criterion | Result | Evidence |
   |---|---|---|
   | <requirement criterion, short> | met / partial / unmet | <test name, PR #n, or why not> |

   ## Deviations

   - Task N: <deviation> — <recorded in the task entry | found by review>; <impact>

   ## CI history

   - <workflow/job> on <PR #n | merge sha7>: <real | infra | unknown>, <what failed>. <For a nondeterministic failure: Mechanism: <the mechanism>, failing value <x>, passing value <y> — or "unexplained nondeterminism" with both values when no mechanism is known.>

   ## Effort

   dispatched=N returned=M failed=K; attempts <total> over <tasks> tasks; first-pass <n>/<tasks>; review rounds <n>; wall-clock <first run_start> → <last merge>
   <the runspec.py summary table>

   ## Lessons

   - What worked: <practice, with the task or PR that shows it>
   - What to change: <problem, with its evidence>

   ## Proposals

   - <P-<spec>-<n> — title>, or None
   ```

   **Flake discipline.** Never label a failure a flake from "a retry went green": that proves nondeterminism, not its cause. The words flake or flaky appear only on a line that also gives `Mechanism:` with the failing and the passing value (`.claude/rules/agent-behavioral-posture.md` §7). The `finalize` lint check enforces this.

2. **Choose proposals.** A lesson becomes a proposal when its evidence points at the harness, not at one task: a spec-format ambiguity two tasks resolved differently; a gate that passed a defect the review found; a skill step runs skipped or improvised; a CI failure class seen more than once; a review finding class the per-task review should have caught. A one-off slip with no mechanism stays under Lessons. Also turn into proposals the important review findings the Review Summary routed here.

3. **Append each proposal** to `.claude/proposals/pending.md` in the format of `.claude/proposals/README.md`, one change and one target each. Take every id from `python3 scripts/harness/finalize.py proposal-id --spec <spec>`, run again after each append. Types: `rule`, `skill`, `hook`, `knowledge`, `product`. Proposals are public: generic hazard, no private detail.

4. **Check** in the working tree: `scripts/ci/lint-agent-harness.sh --only proposals,references` passes. (The `finalize` check reads `specs/done/`, so it runs in `/finalize-spec-publish` after the move.)

## Output

The path written, the proposal ids (or `none`), and one line per section with its count: criteria met/partial/unmet, deviations, CI failures by class, dispatched/returned/failed.
