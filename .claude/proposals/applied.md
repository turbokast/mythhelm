# Applied proposals

Decided proposals, oldest first. `scripts/harness/proposals.py record` moves a proposal
here from `pending.md` with its decision, in the pull request that carries it. Append
only: never edit or remove an entry. The format is in [`README.md`](README.md).

## P-openssf-badge-1 — verify-ci passes the full commit SHA (short SHAs miss)

- **Decision**: approved
- **Date**: 2026-10-05
- **Pull request**: #153
- **Eval**: `verify-ci-full-sha`
- **Rationale**: Maintainer approved 2026-10-05, endorsing the recommendation: removes the false-pending trap at the call site.
- **Source spec**: `openssf-badge`
- **Type**: skill
- **Target**: `.claude/skills/finalize-spec-verify-ci/SKILL.md`
- **Rationale**: `gh run list --commit <short-sha>` silently matches zero runs even when completed runs exist for that commit, and `finalize.py ci` reports the miss as `verdict=pending` ("no push run on main yet") — a false negative that sends the operator down a re-wait loop. Pinning the step to full 40-char SHAs removes the trap at the call site regardless of when the script learns to normalize.
- **Evidence**: openssf-badge retrospective §CI history: `finalize.py ci --sha d77f2e8 --wait` → pending with 6 completed runs present; `gh run list --branch main --commit d77f2e8` → 0 runs; full-SHA invocation → `verdict=green`, runs=6.

**Proposed change:**

In Step 1, change the command template to use a full SHA and add the warning: "Pass the full 40-character SHA (`git rev-parse <sha>` when starting from a short one): `gh run list --commit` does not match short SHAs, and a short SHA yields a false `verdict=pending` even when the runs are green."
