# Evidence: finalize-records

Record for `.claude/rules/finalize-records.md`. The facts behind the finalize pipeline are in [`knowledge/finalize.md`](../finalize.md).

## Hazard

- **Shared-checkout commits.** A finalize run in a checkout that other sessions also use stages and commits through a shared index. A commit sweeps their staged work into the spec's record, or a path-limited commit silently leaves the spec's own untracked files (the new retrospective) behind. A commit built from an older view of a shared file reverts whatever a peer landed in between.
- **Remembered numbers.** A retrospective written from the orchestrator's memory of the run reports counts nobody measured. Later analysis then treats them as data.
- **Flake labels.** "The retry went green" proves nondeterminism, not its cause. A flake label with no mechanism hides real hangs and races, and five copies of the same unverified label look like corroboration.
- **Colliding records.** Globally numbered ids, allocated by reading the highest existing id, collide when two sessions allocate at once. Hand-edited ledgers drift apart between concurrent writers.
- **Changelog drift.** A free-form changelog loses its structure, and release notes built from it inherit the damage.
- **Irreversible publishing.** A release or a `v*` tag is seen by every user at once and cannot be cleanly withdrawn.

## Mechanism

- Finalize works in its own worktree and lands one pull request. The worktree's `git status` must be clean after staging, so no untracked file is left behind. `finalize.py publish-check` refuses a pull request that changes anything outside the spec, its epic, the changelog, docs, knowledge and proposals.
- The `finalize` check of `scripts/ci/lint-agent-harness.sh` refuses a spec in `done/` with an incomplete task, without a retrospective, missing a section, with an open critical finding, or with a flake word on a line that has no `Mechanism:`. It also refuses a malformed `CHANGELOG.md`.
- The `proposals` check refuses duplicate ids, missing fields, unknown types, missing targets and empty changes. Ids are numbered per spec, so two open finalize pull requests never pick the same one.
- `finalize.py changelog-add` and `changelog-release` validate their output before writing it.
- `.claude/hooks/guard-publish.sh` blocks releases and `v*` tag pushes outside an armed publish window.

## Instances

None recorded in this repository yet.

## Loosening criteria

- Allow a commit from a shared checkout when concurrent sessions can no longer share an index.
- Relax the flake line check when it has blocked three or more accurate records, each recorded here as an instance.
