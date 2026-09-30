---
paths:
  - ".claude/proposals/**"
  - ".claude/evals/**"
---

# Proposals and Evals

- Decide proposals one at a time with a maintainer (`/apply-proposals`). Never decide one yourself, and never read a blanket confirmation as a decision on any item.
- Land every decision as its own pull request from a worktree of `origin/main`, changing only the proposal's target and the files `scripts/harness/proposals.py apply-check` allows.
- Write `.claude/proposals/applied.md` only through `proposals.py record`. Never edit or remove an entry; `apply-check` refuses it.
- An approved rule, skill or hook proposal lands with an eval case that fails at the base and passes with the change. Only a maintainer waives it, and the waiver's reason is recorded.
- Never enable lane 0 or widen its allowlist in `.claude/proposals/auto-apply.json`: that is a maintainer's change.
- An eval case pins one behaviour. Its targets exist, and its grader goes red when the behaviour is reverted: show it red before you rely on it. Change a case only in the pull request that deliberately changes the behaviour it pins, and say so in that pull request.

Evidence: `knowledge/rule-evidence/proposals-and-evals.md`.
