---
name: run-spec-worktree-merge
description: Take a verified task pull request to merged — poll CI and review with runspec.py pr-check, run bounded review rounds through the same worker, keep the branch current by merging origin/main, merge only on verdict=ready, confirm the merge on origin/main, and clean up the task's worktree
argument-hint: "<spec-name> --task N --pr P --worktree <path>"
---

# Run Spec — Review Loop and Merge

Every task's work reaches `main` through its own pull request, merged by the orchestrator only when GitHub, not the worker, says it is ready. This is the step that turns "the worker says it is done" into "it is done".

## Input

`$ARGUMENTS`: the spec, the task, its pull request and the worker's worktree path (absent when resuming a pull request from an earlier run). A lifecycle pull request passes `--lifecycle` instead of a task.

## Invocation contexts

- **Slash command**: runs the loop for one pull request and reports the verdict; it merges only when the person confirms (write gate) or passed `--merge`.
- **Model-invoked**: the same.
- **Non-interactive**: inside `/run-spec`, the run's start is the pre-approval to merge its own task and lifecycle pull requests on `verdict=ready`. Without that context, the merge is a write gate: stop at it and return the verdict.

## Steps

1. **Wait for GitHub, then read it.**

   ```bash
   gh pr checks <pr> --watch --interval 30          # waits for CI; its exit status is not the verdict
   python3 scripts/harness/runspec.py pr-check --spec <spec> --task <N> --pr <pr>
   ```

   The verdict is the last line. `pr-check` requires: the pull request open and not a draft; every check's latest run passing or skipped, `CI OK` and `CodeRabbit` present; zero unresolved review threads; a mergeable state that is not conflicting, behind or blocked; the task's completion entry in the pull request's own `tasks.md`, naming this pull request; every changed file inside the task's `Files` or named in its `Spec deviations`; and no leak in the title, body or added lines (`scripts/ci/check-public-hygiene.sh`).
   - Exit 3, `verdict=unknown` (a check pending, the review not posted, merge state not computed): wait and poll again. Never merge on 3. A review bot that has not started after ten minutes gets a `@coderabbitai review` comment.
   - Exit 1, `verdict=not-ready`: go to step 2.
   - Exit 0, `verdict=ready`: go to step 3.
   - Exit 2: GitHub or git could not be read. That is missing data, not a verdict: retry, then escalate.
2. **Review round.** Send the `reason=` lines to the same worker with the review-round template (`/run-spec-dispatch`). Record `review_round`. When it returns, re-run `/run-spec` Phase 2 step 3 (verification) on the new head, then step 1 here. After three rounds that still end `not-ready`, stop the run: escalation 1.
   - `behind` or `conflict`: the worker merges `origin/main` into its branch in its worktree (never a rebase, never a force push), re-runs its gates and pushes.
   - Reviewing a branch whose base trails origin/main: review the branch's own diff, `git diff <merge-base> HEAD` with `<merge-base>` from `git merge-base origin/main HEAD`, for scope and deletions. A tree-diff against origin/main shows other merged work as deletions the branch never made; use it only to confirm the merge result after the branch is updated.
   - `scope:` for a file the task legitimately needed: the worker names it in `Spec deviations` with the reason. The orchestrator passes `--accept-scope <path>` only for a file the spec itself sanctions elsewhere, and records why in a `verify` event.
   - `leak:` in the diff: the worker removes it and pushes. A credential (`token`, `private-key`) that was pushed at all stays exposed in the branch history: stop the run and have the operator revoke it. A leak in the title or body stops the run and goes to the operator: the body's edit history stays public, and a `gh api` write to the pull request is a publishing action (`.claude/hooks/guard-publish.sh`).
3. **Merge.**

   ```bash
   git -C <worktree> status --porcelain           # must print nothing: no work left behind
   git worktree remove <worktree>                 # refuses a dirty worktree; never --force past that
   gh pr merge <pr> --squash --delete-branch
   python3 scripts/harness/runspec.py verify-merged --spec <spec> --task <N> --pr <pr>
   python3 scripts/harness/runspec.py event --spec <spec> --kind merge --task <N> --pr <pr> --result ok
   ```

   Under an autonomy grant (`scripts/orchestration/autonomy.sh status` reports it live), `.claude/hooks/guard-autonomy.sh` refuses that merge line: record the verdict under the grant with `python3 scripts/orchestration/autonomy.py merge-check --pr <pr> --spec <spec> --task <N>` (`--lifecycle` for a lifecycle pull request), and merge with the `gh pr merge <pr> --squash --match-head-commit <sha>` it prints (add `--delete-branch`).

   `verify-merged` fetches origin and requires the pull request merged, its merge commit on origin/main, and the task's entry there naming the pull request. Anything else is not a merge: record `--result fail` and stop the run. A dirty worktree at the first command means the pushed branch is not the whole work: resume the worker to commit or discard it, then start again at step 1.
4. **Report** `[<done>/<total>] Task N — <name>: MERGED (PR #<pr>)`, then `git fetch origin` so the next batch's workers start from the new tip.

## Conflict prevention

Parallel tasks have disjoint `Files` by construction (`/run-spec-parallel-detection`). The spec files every task edits merge without conflict because each task writes only its own `tasks.md` block and its own seeded `handoff.md` section. The conflicts that remain come from a file outside the Files lists, usually one named as a deviation: the second pull request to merge picks it up at step 2 as `behind` or `conflict`.

## Output

One line per pull request: `PR #<pr> task <N>: merged (<merge sha>)`, or the verdict and reasons at the stop.
