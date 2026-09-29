---
name: finalize-spec-review
description: Review a whole spec's cumulative diff — the range from the first task's base to the last merge, derived from the merged pull requests — with the code-reviewer agent against requirements, design and invariants, plus an optional advisory vendor review; adjudicate every finding and write the Review Summary
argument-hint: "<spec-name> [--extra-pr N]..."
---

# Finalize Spec — Review

Step 2 of `/finalize-spec`. Each task's pull request was reviewed on its own; this review looks at what no single pull request shows: the seams between tasks, whether the spec's requirements are met as a whole, invariants that only the combined change can break, and drift between the design and what shipped.

## Input

`$ARGUMENTS`: the spec name, and `--extra-pr N` for each fix pull request of this spec merged since the tasks (critical findings from an earlier run of this review, or CI fixes), so the range covers them.

## Invocation contexts

- **Slash command**: runs the review and writes the Review Summary into the working tree it is run in: the finalize worktree when `/finalize-spec` runs it, or the current checkout when run alone, where it writes the file and commits nothing.
- **Model-invoked**: the same.
- **Non-interactive**: the same. Adjudication is an advisory step: every finding is confirmed or rejected at its anchor and recorded, `auto-confirmed (non-interactive)`. A finding that shows the spec itself is wrong is an escalation.

## Steps

1. **Derive the range from the record, never from the local branch.**

   ```bash
   python3 scripts/harness/finalize.py range --spec <spec> [--extra-pr <n>]... > <tmp>/range.json
   ```

   `base` is the parent of the spec's first merge commit, `head` its last, `files` the union of what its pull requests changed, and `foreign` lists commits by other work inside `base..head` that touched those files. The diff to review is `git diff <base>..<head> -- <files>`. A foreign commit's lines in that diff belong to its own change: attribute them in the summary and draw no finding from them against this spec.

2. **Read** `requirements.md`, `design.md` and `tasks.md` of the spec (origin/main's copy), and `knowledge/invariants.md`. From `tasks.md` take every task's `Spec deviations` and `Invariants touched`.

3. **Dispatch the reviewers in one message**, each with `subagent_type` and no `model` or `effort` (their pins apply):
   - `code-reviewer`, always.
   - `architect` too, when `files` spans more than one domain of `knowledge/domains.md`, or touches a seam its agent file lists.

   The prompt gives the range and the diff command, the file list, the foreign attributions, the spec files to read, and this brief: *"Every task pull request was already reviewed. Review the spec as one change: (1) map every acceptance criterion in requirements.md to the merged code or test that meets it, or mark it unmet; (2) check the seams between tasks: interfaces one task produced and another consumed, duplicated helpers, error and exit-code conventions that diverge between tasks; (3) check each invariant under `Invariants touched` across the combined change; (4) compare design.md with what shipped and name each divergence and whether a task recorded it; (5) report stale documentation the change left behind. Report in your standard format, with an 'Acceptance criteria (spec-wide)' section. Read-only: never edit, commit or comment on GitHub."* Record `dispatched=N returned=M failed=K` before reading the results. A reviewer that returned nothing is re-dispatched once; a second failure is an escalation, never a pass.

4. **Optional vendor review** (advisory, `.claude/rules/vendor-usage.md`). When `python3 scripts/vendors/vendors.py status` shows Codex or Muse enabled, run `/vendor-consult <vendor> change-review <context file> --diff-base <base>`, where the context file holds the spec's requirements and the file list, and the snapshot is taken with `--only` for each file in `files`. `unavailable` means `skipped (<reason>)`; a vendor never blocks.

5. **Adjudicate every finding at its anchor.** Findings from agents and vendors are candidates: open `file:line` on `head` and mark each `confirmed` or `rejected` with the evidence. Then give each confirmed finding a disposition:
   - **critical**: open until a fix pull request merges (`/finalize-spec` §Fix pull requests); a spec never ships with one.
   - **important**: fixed in a pull request, filed as a GitHub issue, or turned into a proposal by `/finalize-spec-retrospective`. Name which.
   - **suggestion**: kept in the list; no action required.

6. **Write the Review Summary** at the top of `specs/unfinalized/<spec>/retrospective.md` in the working tree (create the file with the title `# <spec> — Retrospective`; replace an existing `## Review Summary` section, keep every other section):

   ```markdown
   ## Review Summary

   - **Range**: <base7>..<head7> (PRs #a, #b, ...; fix PRs #x or none)
   - **Reviewer**: code-reviewer[, architect]
   - **Findings**: critical <n>, important <n>, suggestion <n> (confirmed); rejected <n>
   - **Open critical**: <n; 0 before the spec can ship>
   - **Vendor review**: <codex|muse: <confirmed>/<findings> confirmed | skipped (<reason>)>

   | Severity | Finding | Anchor | Disposition |
   |---|---|---|---|
   | important | <one line> | `path:line` | <fixed in #n / issue #n / proposal / rejected: evidence> |

   Foreign changes in range: <sha7 subject: files, or none>.
   ```

   `scripts/ci/lint-agent-harness.sh` (check `finalize`) refuses a spec in `done/` whose `Open critical` is not 0 or whose summary lacks one of the five fields.

7. **Report** to the caller: the counts, and each open critical finding with its anchor. `/finalize-spec` stops while any is open.

## Output

The five summary lines, then one line per open critical finding, or `Open critical: 0`.
