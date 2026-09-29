---
name: run-spec
description: Orchestrate every task of a spec to merged pull requests — parse the dependency graph, dispatch each ready task to its pinned agent in its own worktree from origin/main, verify each result against GitHub and the tree, run the review loop, merge, retry within bounds, and move the spec through its lifecycle
argument-hint: "<spec-name> [--alias <word>] [--max-parallel N]"
---

# Run Spec

Automates the scheduling a person does by hand: read `tasks.md`, start a fresh agent per ready task, wait, verify, merge, repeat. Each task lands as its own pull request, reviewed by CodeRabbit and Sourcery and merged by this orchestrator once it is green, has no unresolved review thread and passes the leak check. The durable state is `tasks.md` on origin/main plus the open pull requests, so a stopped run resumes where it stopped.

The orchestrator decides only from mechanical facts: `scripts/harness/runspec.py` parses the task graph, checks entries, reads GitHub and folds the event log; `scripts/harness/gatelib.py` reports gate markers. A worker's prose is never evidence.

## Input

`$ARGUMENTS`: `<spec-name> [--alias <word>] [--max-parallel N]`. `--alias` is the short name in branch names and pull-request titles (default: the spec name; reuse the alias earlier pull requests of the spec used). `--max-parallel` caps a batch (default 4).

## Invocation contexts

- **Slash command**: runs Phases 0–3. Escalations (below) stop the run and report to the person.
- **Model-invoked**: the same.
- **Non-interactive** (a headless run or a dispatched orchestrator): the same, with no questions. Every escalation stops the run and returns the stop report. An orchestrator that is itself a subagent receives no background notifications: it waits on CI, reviews and workers in the foreground (`gh pr checks <n> --watch` with a raised Bash timeout, or a bounded poll loop), never by ending its turn.

## Sub-skills

| Phase | Skill |
|---|---|
| Task graph | `/run-spec-dependency-graph` |
| What can start now | `/run-spec-parallel-detection` |
| Dispatch prompts | `/run-spec-dispatch` |
| Review loop and merge | `/run-spec-worktree-merge` |
| Retries, stalls, deadlock | `/run-spec-error-handling` |
| Completion | `/run-spec-completion` |

## Phase 0 — Resolve and preflight

1. `git fetch origin`. Resolve the spec: `scripts/harness/spec-lifecycle.sh resolve <spec>`. It refuses a spec with two copies across the working tree and the git index; that is an escalation, never a guess. Refuse `unrefined/` and `refined/` (point to `/refine-spec` or `/spec`) and `unfinalized/`, `done/`, `archived/` (already implemented).
2. Resolve it on origin/main too: `git ls-tree -r --name-only origin/main -- specs/ | grep "/<spec>/tasks.md"` names exactly one path. Workers start from origin/main, so that copy is the one they read.
3. `scripts/ci/lint-agent-harness.sh --only specs` passes: a missing field or a dependency cycle is a spec defect to fix through `/evaluate-spec`, not something to schedule around.
4. Record the start: `python3 scripts/harness/runspec.py event --spec <spec> --kind run_start --detail "alias=<alias>"`.

## Phase 1 — Lifecycle start and resume

1. **Lifecycle pull request.** When the spec is in `todo/` on origin/main, or its `handoff.md` lacks a section for some task, open one lifecycle pull request from a fresh worktree of origin/main:

   ```bash
   git worktree add -b docs/<alias>-start ../<repo>-<alias>-start origin/main
   (cd ../<repo>-<alias>-start && scripts/harness/spec-lifecycle.sh move <spec> in-progress \
     && python3 scripts/harness/runspec.py handoff-seed specs/in-progress/<spec>)
   ```

   Commit the move (both paths) and `handoff.md`, push, and open it titled `docs(spec): start <spec> implementation`. Merge it through `/run-spec-worktree-merge` with `runspec.py pr-check --lifecycle`, then `git fetch origin`. Task pull requests already open against the old path merge cleanly after it: git's rename detection carries their edits.
2. **Resume.** For every incomplete task, look for its open pull request (`gh pr list --state open --search "<alias> task <N> in:title" --json number,headRefName`). A task with one is **in flight**: it goes straight to the review loop (`/run-spec-worktree-merge`), never to a second dispatch.
3. Record the baseline: `python3 scripts/harness/runspec.py status specs/in-progress/<spec>/tasks.md --in-flight <in-flight tasks>`. A non-empty `marker_mismatch` (a task complete by heading or by Status, not both) is bookkeeping to backfill with `completion-clerk` through a small pull request; it never blocks scheduling.

## Phase 2 — Execution loop

Repeat until every task is complete on origin/main, or the run stops:

1. **Select.** `/run-spec-parallel-detection` returns the batch: ready tasks whose dependencies are merged on origin/main and whose `Files` overlap neither each other nor an in-flight task. Empty batch, nothing in flight, tasks left: deadlock (`/run-spec-error-handling`). A `maintainer` task is handed to the operator, not dispatched.
2. **Dispatch.** `git fetch origin`, then for each task `python3 scripts/harness/runspec.py deps-merged <spec> <N>` exits 0. Render each prompt with `/run-spec-dispatch` and send the batch in one message: one Agent call per task, `subagent_type` = the task's `Domain/agent`, `isolation: "worktree"`, no `model`. Record one `dispatch` event per call before any result arrives, then print `dispatched=N returned=M failed=K` whenever results are consumed (`.claude/rules/agent-behavioral-posture.md` §5).
3. **Verify each return** — never the worker's claims, always their evidence:
   1. Parse the report: `python3 scripts/harness/runspec.py report --file <final message>`. `task_report_missing` or `task_report_malformed` is not yet a failure; go to substep 3.
   2. `gh pr view <pr> --json headRefName,headRefOid,state` exists, is open, and its head branch is the one the worktree has checked out; `git -C <worktree> status --porcelain` is empty and `git -C <worktree> rev-parse HEAD` equals `headRefOid`. A report of `pr_open` with no such pull request is a failed dispatch, whatever its prose says. With no report at all, look for the branch and pull request before deciding: a worker can open the pull request and still die before reporting.
   3. `python3 scripts/harness/gatelib.py status --root <worktree>` exits 0: every gate the change set needs has a marker for the exact tree that was pushed.
   4. `python3 scripts/harness/runspec.py entry-check <worktree>/specs/in-progress/<spec>/tasks.md <N> --pr <pr>` exits 0, and the task's `handoff.md` section is no longer `<!-- pending -->`.
   5. Re-run the task's Acceptance yourself, verbatim, in the worktree: the named tests (`go test -race -run '<names>' <pkgs> -v`, each `--- PASS` seen) and every presence or absence `git grep` the criteria state. A worker has written the banned token into its own negative test before; the orchestrator's grep is the one that counts.
   6. Classify: code, tests or acceptance wrong → **real failure** (`/run-spec-error-handling`); only the entry, hand-off, report or markers wrong → **bookkeeping-only** (resume the worker to fix it; it costs no attempt). Record a `verify` event with the outcome.
4. **Review and merge** each verified pull request through `/run-spec-worktree-merge`: poll `runspec.py pr-check`, resume the same worker for each review round, merge on `verdict=ready`, then `runspec.py verify-merged` must print `verdict=merged` before the task counts as complete.
5. **Report progress** after each merge or failure: `[<done>/<total>] Task N — <name>: MERGED (PR #n)`, `FAILED (attempt K/3)` or `RETRYING (attempt K/3)`.

## Phase 3 — Completion

`/run-spec-completion`: confirm every task is complete on origin/main, open and merge the lifecycle pull request that moves the spec to `unfinalized/`, print the summary, and record `run_end`.

## Escalations (the run stops)

1. A task failed three attempts, or three review rounds left its pull request not ready.
2. Deadlock: nothing ready, nothing in flight, tasks left; or a dependency cycle.
3. A duplicate spec copy, or a spec in the wrong lifecycle state.
4. A worker reported `blocked` with a question the spec does not answer.
5. A guard blocked the orchestrator itself.

Every stop prints the `/run-spec-completion` stop report. The spec stays where it is; re-running `/run-spec <spec>` resumes.

## Constraints

- One task per worker, one worktree per task, one pull request per task. The shared checkout is never a worker's working tree: its index is shared, and one `git stash` or `git add -A` there sweeps other sessions' work.
- Every dispatch names `subagent_type`; `.claude/hooks/guard-dispatch-pin.sh` blocks one that does not while this skill runs.
- Workers never merge, never edit another task's entry or hand-off section, and never write outside their worktree.
- The orchestrator never edits a worker's code. It may commit bookkeeping (entry, hand-off) into a worker's branch only through `/run-spec-error-handling` §Bookkeeping-only.

## Output

The progress lines as the run goes, then the `/run-spec-completion` report.
