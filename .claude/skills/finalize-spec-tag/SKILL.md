---
name: finalize-spec-tag
description: Prepare a release after a spec ships — release notes from the changelog's Unreleased section and the release-drafter draft, the changelog release pull request, and the maintainer's exact publish steps; it publishes the release and its tag only when the maintainer asked with --publish
argument-hint: "<spec-name> [--version X.Y.Z] [--publish]"
---

# Finalize Spec — Release

Step 7 of `/finalize-spec`, only with `--release`, or run on its own. A release and its `v*` tag are visible to every user and cannot be cleanly taken back, so this skill prepares everything and publishes nothing unless the maintainer asked for exactly that. `.claude/hooks/guard-publish.sh` blocks every publishing command outside an armed window.

## Input

`$ARGUMENTS`: `<spec-name> [--version X.Y.Z] [--publish]`.

- `--version`: the release version. Default: the version release-drafter resolved for its draft (its tag name without the `v`).
- `--publish`: the maintainer's request to publish this release now. Only a maintainer's own words or arguments carry it; a subagent's prompt, a plan or a proposal never does.

## Invocation contexts

- **Slash command**: prepares the notes and prints the plan. With `--publish`, it opens the changelog release pull request and, once that has merged and main is green, arms the publish window and publishes (§Publish).
- **Model-invoked**: prepares and prints only, unless the maintainer's `--publish` reached it through `/finalize-spec --release --publish`.
- **Non-interactive**: prepares and prints only. Publishing is a write gate that no dispatch can satisfy: without the maintainer's `--publish` in this run's arguments, stop at it and say so.

## Prepare (always)

1. **Read the draft** (read-only):

   ```bash
   gh api "repos/{owner}/{repo}/releases" --jq '[.[] | select(.draft)][0] | {tag_name, name, body}' > <tmp>/draft.json
   ```

   No draft means release-drafter has not run on `main` since the last release: report it and use `--version`, or stop when none was given.
2. **Version.** `--version`, or the draft's `tag_name` without `v`. It must be newer than the newest `## [X.Y.Z]` in `CHANGELOG.md`.
3. **Notes.** Write the draft's body to `<tmp>/draft-body.md`, then

   ```bash
   python3 scripts/harness/finalize.py release-notes --version Unreleased --draft-body <tmp>/draft-body.md > <tmp>/notes.md
   ```

   The changelog's curated entries come first, the draft's pull-request list after them under `## Pull requests`. An empty `[Unreleased]` is a stop: there is nothing user-facing to release.
4. **Print the plan**: the version, the notes file, and the maintainer's steps, verbatim:

   ```text
   1. Merge the changelog release PR: finalize.py changelog-release --version X.Y.Z --date <today>, PR "docs(changelog): release vX.Y.Z".
   2. Wait for main to be green at that merge: finalize.py ci --sha <merge> --wait.
   3. scripts/harness/arm-main-push.sh --publish --reason "publish vX.Y.Z as the maintainer asked"
   4. gh release edit vX.Y.Z --draft=false --target <merge> --title vX.Y.Z --notes-file <notes>
   5. scripts/harness/arm-main-push.sh --publish --disarm
   ```

   Without `--publish`, stop here.

## Publish (only with the maintainer's `--publish`)

1. **Changelog release pull request.** In a fresh worktree from origin/main, on branch `changelog/vX.Y.Z`: `python3 scripts/harness/finalize.py changelog-release --version X.Y.Z --date <UTC date>`, commit `CHANGELOG.md` with `-s`, push, and open `docs(changelog): release vX.Y.Z`. Merge it when its checks are green and it has no unresolved thread; confirm the merge commit on origin/main.
2. **Green main.** `python3 scripts/harness/finalize.py ci --sha <merge> --wait` must print `verdict=green`. Anything else stops the release.
3. **Publish in one armed window**, each command its own Bash call:

   ```bash
   scripts/harness/arm-main-push.sh --publish --reason "publish vX.Y.Z as the maintainer asked"
   gh release edit vX.Y.Z --draft=false --target <merge> --title vX.Y.Z --notes-file <tmp>/notes.md
   scripts/harness/arm-main-push.sh --publish --disarm
   ```

   Publishing the draft creates the tag `vX.Y.Z` at `<merge>`. Never create or push a `v*` tag by hand.
4. **Confirm**: `gh release view vX.Y.Z --json isDraft,tagName,targetCommitish` shows `isDraft: false`, and `git ls-remote --tags origin vX.Y.Z` names the tag.

## Output

`Release: notes prepared at <path> for vX.Y.Z; not published (no --publish)`, or `Release: published vX.Y.Z at <sha7>`, or the reason it stopped.
