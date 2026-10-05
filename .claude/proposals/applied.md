# Applied proposals

Decided proposals, oldest first. `scripts/harness/proposals.py record` moves a proposal
here from `pending.md` with its decision, in the pull request that carries it. Append
only: never edit or remove an entry. The format is in [`README.md`](README.md).

## P-tui-slice-2 — Dispatch template requires signed-off worker commits

- **Decision**: approved
- **Date**: 2026-10-05
- **Pull request**: #151
- **Eval**: `dispatch-commits-signed-off`
- **Rationale**: Maintainer approved 2026-10-05, endorsing the recommendation: stops the repeated unsigned-commit red-gate cycle.
- **Source spec**: `tui-slice`
- **Type**: skill
- **Target**: `.claude/skills/run-spec-dispatch/SKILL.md`
- **Rationale**: Task 12's worker committed unsigned, breaking the required DCO gate on three consecutive heads; the fix needed a signoff rebase and force-push cycle. The dispatch template's commit rule ("Commit named paths only") never mentions sign-off, so every new worker rediscovers it through a red gate.
- **Evidence**: PR #102 DCO failures on 2b78e3a/31b27e7/31b4548; fix commit 40ee90c; retrospective CI history.

**Proposed change:**

In the first-attempt template's Rules, change "Commit named paths only." to "Commit named paths only, signed off (`git commit -s`)." Add to the retry template's digest rules: a DCO failure is fixed by `git rebase --signoff` plus push, never by an empty sign-off commit. When the retry continues an open PR (branch checked out from `origin/<branch>`), the rebase rewrites published commits, so the push must be `git push --force-with-lease` — the narrow, explicitly named exception to the template's no-force-push rule for this case only. A retry without an open PR starts from `origin/main` and must not force-push.
