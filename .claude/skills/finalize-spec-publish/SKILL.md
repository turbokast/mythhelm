---
name: finalize-spec-publish
description: Land a spec's finalize record as one pull request — move the spec to done, roll up its epic, update the docs it changed and the changelog, commit only named paths from the finalize worktree, open the pull request, answer its review, merge it on a ready verdict and confirm it on origin/main
argument-hint: "<spec-name> [--alias <word>] [--no-changelog \"<reason>\"]"
---

# Finalize Spec — Publish

Step 4 of `/finalize-spec`. Nothing reaches `main` except through a pull request with every required check green, so the record of a finished spec is one pull request, `docs(spec): finalize <spec>`, reviewed like any other.

## Input

`$ARGUMENTS`: `<spec-name> [--alias <word>] [--no-changelog "<reason>"]`. `--no-changelog` is for a spec that changed nothing a user of MYTHHELM can notice (harness or internal-only work); the reason goes into the pull-request body.

Run it in the finalize worktree `/finalize-spec` Step 0 created (`<wt>`), after the review and the retrospective have written `retrospective.md` there.

## Invocation contexts

- **Slash command**: runs Steps 1–7. Merging the finalize pull request (Step 7) is a write gate: it proceeds when `/finalize-spec` invoked this skill or the person passed `--merge`; otherwise stop at it and report the verdict.
- **Model-invoked**: the same.
- **Non-interactive**: inside `/finalize-spec`, its invocation is the approval to merge its own finalize pull request on `verdict=ready`. Standalone without `--merge`, stop at the gate, report, and merge nothing.

## Steps

1. **Move the spec.** `(cd <wt> && scripts/harness/spec-lifecycle.sh move <spec> done)`. It refuses any move other than `unfinalized/` → `done/` and any duplicate copy.
2. **Epic.** `/finalize-spec-epic-rollup <spec>` in `<wt>`. Keep the epic name it prints for Step 6.
3. **Docs.** `/update-docs <spec>` in `<wt>`.
4. **Changelog.** For each change a user of MYTHHELM can notice, one entry, written for that user, ending with the pull request that shipped it:

   ```bash
   (cd <wt> && python3 scripts/harness/finalize.py changelog-add --section Added --entry "<what a user can now do> (#<pr>)")
   ```

   Sections: `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`, `Security`. The script creates `CHANGELOG.md` in Keep a Changelog form when it is absent, adds under `## [Unreleased]`, and refuses to write a file that would not validate. With `--no-changelog`, add nothing.
5. **Check, then commit named paths only.**

   ```bash
   cd <wt>
   scripts/ci/lint-agent-harness.sh --only specs,finalize,proposals,references,abs-paths
   scripts/ci/check-public-hygiene.sh
   git add -- specs/done/<spec> <each other path this run changed: CHANGELOG.md, .claude/proposals/pending.md, docs>
   git status --porcelain            # must print nothing: every change is staged, nothing is forgotten
   git commit -s -m "docs(spec): finalize <spec>"
   git push origin docs/<alias>-finalize
   ```

   `git mv` already staged the deletion of `specs/unfinalized/<spec>/` and, with a rollup, both sides of the epic's move. Name the new directory in `git add` so the untracked `retrospective.md` is staged: a commit that names only tracked paths silently leaves untracked files behind, and the clean `git status` is the check that none was. Name only paths that exist: one pathspec that matches nothing aborts the whole `git add`. Never `git add -A` or `.`.
6. **Open the pull request.**

   ```bash
   gh pr create --base main --head docs/<alias>-finalize --title "docs(spec): finalize <spec>" --body-file <file>
   ```

   The body is public and its edit history permanent (`.claude/rules/public-repo-hygiene.md`): what the spec delivered in two sentences, the Review Summary's five lines, the changelog entries (or `Changelog: none — <reason>`), the proposal ids, and the `finalize.py verify` verdict line.
7. **Review loop and merge.**

   ```bash
   gh pr checks <n> --watch --interval 30
   python3 scripts/harness/finalize.py publish-check --spec <spec> --pr <n> [--epic <epic>] [--no-changelog]
   ```

   `publish-check` requires what `runspec.py pr-check` requires of any pull request (open, not a draft, not conflicting or behind, every check's latest run green with `CI OK` and `CodeRabbit` present, no unresolved thread, no leak in title, body or added lines) plus the finalize scope: only the spec's directories, its epic's, `CHANGELOG.md`, root docs, `docs/`, `knowledge/` and `.claude/proposals/pending.md` change; `specs/done/<spec>/retrospective.md` is added; `CHANGELOG.md` changes unless `--no-changelog`.
   - Exit 3: poll again. Exit 2: retry, then escalate.
   - Exit 1: answer each reason in `<wt>`. For a review thread, verify the finding, fix it or rebut it with evidence, reply and resolve it (the GraphQL calls in `/implement` Step 9). `behind` or `conflict`: `git merge origin/main` (never rebase); a conflict in `pending.md` keeps both sides. Commit named paths, push, go back to the top. After three rounds still not ready, escalate.
   - Exit 0: merge, then confirm.

   ```bash
   gh pr merge <n> --squash --delete-branch
   git fetch origin
   gh pr view <n> --json state,mergeCommit        # MERGED, and its oid is the finalize merge commit
   git merge-base --is-ancestor <oid> origin/main
   git ls-tree -r --name-only origin/main -- specs/ | grep "/<spec>/"   # every path under specs/done/<spec>/
   ```

   Anything else is not a merge: escalate.

## Output

`Publish: PR #<n> merged as <oid7>; epic <result>; docs <files | none>; changelog <entries | none — reason>`, and the merge commit id for `/finalize-spec-verify-ci`.
