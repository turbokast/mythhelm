---
name: run-spec-error-handling
description: What /run-spec does when a task goes wrong — classify real versus bookkeeping-only failures, the three-attempt retry ladder with failure digests, lost and stalled workers, blocked and maintainer tasks, deadlock and cycles, and when the run stops
argument-hint: "<spec-name> --task N"
---

# Run Spec — Error Handling

A failed task is classified before anything is re-dispatched, because the cheapest correct response differs by class, and a wrong class wastes an attempt or merges broken work.

## Input

`$ARGUMENTS`: the spec and the task. `/run-spec` reads this page inline.

## Invocation contexts

- **Slash command**: classifies the named task's last result and prints the next action.
- **Model-invoked**: the same.
- **Non-interactive**: the same. Every escalation stops the run with the stop report (`/run-spec-completion`); nothing waits for an answer.

## Classes

| Class | Signal | Response | Costs an attempt |
|---|---|---|---|
| Real failure | acceptance re-run fails, gates red, code absent, or the worker reported `blocked` on its own defect | retry ladder | yes |
| Bookkeeping-only | code, tests and acceptance verified; only the entry, hand-off, report or markers are wrong | fix it through the worker (below) | no |
| Lost | the Agent call returned nothing, or no report and no branch | look before retrying (below) | yes, once confirmed |
| Stalled | the worker says it is waiting on something | resume it with the evidence (below) | no |
| Blocked on a decision | `blocked` with a question the spec does not answer | stop: escalation | no |
| Not-ready after review | three review rounds (`/run-spec-worktree-merge`) | stop: escalation | no |

Record each classification: `python3 scripts/harness/runspec.py event --spec <spec> --kind fail --task <N> --attempt <K> --detail "<class>: <one line>"`.

## Retry ladder

At most three attempts per task, each a fresh agent of the same `Domain/agent`:

1. **Attempt 1**: the first-attempt template.
2. **Attempt 2**: the retry template with the attempt-1 digest.
3. **Attempt 3**: the retry template with both digests.

The digest is at most ten lines: the error class, the failing check with its exact failing lines (at most three), what was tried, the pull request and branch if any, and a one-line hypothesis. It comes from the verification output and the `fail` event, never from pasted logs. A pull request an earlier attempt opened is continued, not duplicated. After the third failure the run stops: escalation. Budgets never grow on retry; a task that keeps overrunning is re-tiered in `tasks.md` by a spec change.

## Bookkeeping-only

The work is right; only its record is not. First, `SendMessage` the same worker with the bookkeeping-fix template (`/run-spec-dispatch`): it has the context to write the entry and hand-off correctly. When the worker cannot be resumed:

1. Dispatch `completion-clerk` (no `effort`) with the worktree path, the task number and the facts it may not recompute: the pull request number, the commit SHAs, the files the pull request changes (`gh pr view <pr> --json files`), the deviations and the gate lines you observed. It edits only the task's entry.
2. Fill the hand-off section yourself from the verified diff, or mark it `- No hand-off recorded; see the pull request.` when there is nothing a dependent needs.
3. In the worktree, refresh and commit: `(cd <worktree> && scripts/harness/gate.sh all)`, then `git -C <worktree> commit -s -m "docs(spec): <alias> task <N> completion entry" -- specs/in-progress/<spec>/tasks.md specs/in-progress/<spec>/handoff.md` and `git -C <worktree> push origin <branch>`.

At most one bookkeeping fix per task; a second means a real failure.

## Lost and stalled workers

- **Lost.** A dispatch that came back empty is unknown, not failed. Before retrying, look: `gh pr list --state all --search "<alias> task <N> in:title"` and `git ls-remote origin '<type>/<alias>-t<N>'`. A pull request or branch means the work happened: verify it as a return (`/run-spec` Phase 2 step 3) and record `return`. Nothing at all: record `lost`, then retry.
- **Stalled.** A worker that ends its turn "waiting for" a command, CI or a notification is not failed. A subagent is never notified when its own background command ends. Check the evidence (the branch, the pull request's checks, the gate markers in its worktree), then `SendMessage` the same worker with what you found and the remaining steps, restating that every command runs in the foreground. Never dispatch a duplicate while the first worker can still be resumed.
- **Long-running.** The Agent tool cannot abort a worker. One that has not returned an hour after dispatch is checked the same way; if it is still working, let it finish and report the overrun.

## Blocked and maintainer tasks

- A task with a `Blocked` field is skipped and listed in the report; its dependants cannot start.
- A `maintainer` task is the operator's. Hand it over with its acceptance criteria when it becomes ready, and continue with every task that does not depend on it.

## Deadlock and cycles

`runspec.py status` reports `deadlock: true` when tasks remain, none is ready and none is in flight, and `cycle: true` for a dependency cycle. Either stops the run. The report names each stuck task with the dependencies it waits on and why each is not complete (blocked, maintainer, failed, not merged). Pass every task with an open pull request as `--in-flight`: a pull request waiting on review or bookkeeping is progress, and must never read as a deadlock.

## Output

The class, the event recorded and the next action: `retry attempt K`, `resume worker <id>`, `bookkeeping fix`, or `STOP: <escalation> — <reason>`.
