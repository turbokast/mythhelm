---
paths:
  - "specs/done/**"
  - ".claude/proposals/**"
  - ".claude/skills/finalize-spec*/**"
  - "scripts/harness/finalize.py"
  # future: the first finalize pull request creates the changelog
  - "CHANGELOG.md"
---

# Finalize Records

- A spec reaches `specs/done/` only through `/finalize-spec`: one finalize pull request from a worktree of origin/main, merged on `finalize.py publish-check` printing `verdict=ready`. Never commit a finalize record in the shared checkout.
- Take every number in a retrospective from `finalize.py` or `runspec.py` output. Write `unknown` for a number no command produced.
- Label a failure a flake only on a line that also names its `Mechanism:` with the failing and passing values. Otherwise write `unexplained nondeterminism` with both values.
- Write proposals in the format of `.claude/proposals/README.md`, with an id from `finalize.py proposal-id`. Append at the end of `pending.md`. Never edit or remove another spec's proposal.
- Change `CHANGELOG.md` only through `finalize.py changelog-add` and `changelog-release`. Write each entry for a user of MYTHHELM and end it with the pull request that shipped it.
- Never publish a release or push a `v*` tag unless a maintainer asked for that release in this session (`/finalize-spec-tag` §Publish).

Evidence: `knowledge/rule-evidence/finalize-records.md`.
