# Applied proposals

Decided proposals, oldest first. `scripts/harness/proposals.py record` moves a proposal
here from `pending.md` with its decision, in the pull request that carries it. Append
only: never edit or remove an entry. The format is in [`README.md`](README.md).

## P-docs-site-demos-1 — spec-decomposition: user-visible design claims need a task owner

- **Decision**: approved
- **Date**: 2026-10-05
- **Pull request**: pending
- **Eval**: `decomposition-owns-visible-claims`
- **Rationale**: Maintainer approved 2026-10-05, endorsing the recommendation: prevents user-visible claims dropping silently.
- **Source spec**: `docs-site-demos`
- **Type**: skill
- **Target**: `.claude/skills/spec-decomposition/SKILL.md`
- **Rationale**: design §5 required the docs site to show the word "planned" where the TUI recording will go, but no task's Files list owned a site page for that claim — Task 7 owned only the tape scaffold and the (unrendered) demos README row. No task built it, and no task recorded skipping it, so the gap surfaced only at finalize review. A decomposition check that every user-visible design claim has a task owner prevents silent drops.
- **Evidence**: docs-site-demos retrospective §Review Summary (suggestion 2) and §Deviations (Review row); `specs/done/docs-site-demos/design.md` §5 vs Task 7 Files in `tasks.md`; PR #137 merged without it.

**Proposed change:**

Add to the decomposition steps, after task Files lists are drafted: "Coverage pass: for every sentence in design.md that states what a user sees (page text, labels, placeholders, error wording), name the task whose Files list contains the file carrying that text. A user-visible claim with no owning task is a gap: assign it to a task's Files or record it as an explicit follow-up with its card or issue. The finalize review re-checks this mapping."
