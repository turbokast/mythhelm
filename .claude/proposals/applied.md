# Applied proposals

Decided proposals, oldest first. `scripts/harness/proposals.py record` moves a proposal
here from `pending.md` with its decision, in the pull request that carries it. Append
only: never edit or remove an entry. The format is in [`README.md`](README.md).

## P-tui-slice-3 — Implementation skill carries the same DCO exception

- **Decision**: approved
- **Date**: 2026-10-05
- **Pull request**: pending
- **Eval**: `implement-dco-retry-exception`
- **Rationale**: Maintainer approved 2026-10-05, endorsing the recommendation: keeps implement consistent with the P-tui-slice-2 DCO exception.
- **Source spec**: `tui-slice`
- **Type**: skill
- **Target**: `.claude/skills/implement/SKILL.md`
- **Rationale**: P-tui-slice-2 exempts open-PR DCO retries from the dispatch template's no-force-push rule, but retry workers also follow the implementation skill, whose Step 7 forbids force pushes and whose Step 9 forbids rebases outright. Without the same exception there, a worker ordered to `rebase --signoff` plus force-with-lease faces two skills in direct conflict. (Applying both proposals must also reconcile `.claude/hooks/block-destructive.sh`, which blocks force pushes; that hook change rides with whichever proposal a maintainer accepts first.)
- **Evidence**: P-tui-slice-2; `.claude/skills/implement/SKILL.md` Steps 7 and 9; PR #102 DCO cycle.

**Proposed change:**

In Step 7's rules, append to the no-force-push sentence: "Exception: an open-PR DCO retry signs off via `git rebase --signoff` and pushes with `git push --force-with-lease` (see P-tui-slice-2); this is the only rebase or force-push a worker ever performs." In Step 9's behind/conflict rule, append: "The DCO retry is the exception: it rebases with `--signoff` instead of merging."
