---
name: run-spec-completion
description: Finish or stop a /run-spec run — confirm every task is complete on origin/main, move the spec to unfinalized through a lifecycle pull request, and print the success or stop report from the run's event log with dispatch accounting
argument-hint: "<spec-name>"
---

# Run Spec — Completion

The last phase of `/run-spec`, and the report every stop prints.

## Input

`$ARGUMENTS`: the spec name. `/run-spec` reads this page inline.

## Invocation contexts

- **Slash command**: prints the report for the spec's run so far; it opens the lifecycle pull request only when every task is complete and the person confirms (write gate) or passed `--finish`.
- **Model-invoked**: the same.
- **Non-interactive**: inside `/run-spec`, the run is the approval to open and merge the lifecycle pull request. Otherwise it prints the report and withholds the pull request, saying so.

## On success

1. **Confirm from origin/main, not the local checkout.** `git fetch origin`, then `python3 scripts/harness/runspec.py status <(git show origin/main:specs/in-progress/<spec>/tasks.md)`: `incomplete` is empty and `marker_mismatch` is empty. For each task the run merged, `runspec.py verify-merged` already printed `verdict=merged`; re-run it for any task the run adopted from an earlier session.

   **Deferrals.** Collect every "Task N ..." deferral named in a **Spec deviations** entry of that `tasks.md`. Each must resolve to a completing change: the promised edit is present in the referenced diff (`gh pr diff <n>`), and the task or fix pull request that carries it has merged. A **Files modified** entry alone, or an open fix pull request, is not proof. An unresolved deferral stops the run (§On stop) with the promise named in `Reason:`.
2. **Lifecycle pull request.** From a fresh worktree of origin/main: `scripts/harness/spec-lifecycle.sh move <spec> unfinalized`, commit both paths, push `docs/<alias>-implemented`, and open `docs(spec): <spec> implemented, awaiting finalize`. Merge it through `/run-spec-worktree-merge` with `pr-check --lifecycle`, then `git fetch origin` and confirm `specs/unfinalized/<spec>/` on origin/main. Record `python3 scripts/harness/runspec.py event --spec <spec> --kind lifecycle --detail "in-progress -> unfinalized" --result ok`.
3. **Report**, then record `run_end`.

```text
## /run-spec complete: <spec>

dispatched=<N> returned=<M> failed=<K>
<the table from: python3 scripts/harness/runspec.py summary --spec <spec>>

Retries: <tasks that needed more than one attempt, with the digest's one-line cause>
Review rounds: <total>, <the most common finding class>
Stop-hook overrides: <rows of .claude/data/stop-gate-audit.jsonl with event override_* from this run, or none>
Next: the spec is in specs/unfinalized/ and awaits the finalize review.
```

## On stop

Printed at every escalation (`/run-spec` §Escalations); the spec stays in its lifecycle directory, and re-running `/run-spec <spec>` resumes.

```text
## /run-spec stopped: <spec>

Reason: <the escalation, one line>
Task: <N> — <name>; attempts <K>/3; PR #<n> (<state>, <pr-check verdict>)
Evidence: <the failing lines, the reason= lines or the question, verbatim, at most 10 lines>
In flight: <open pull requests of this spec, with verdicts>
dispatched=<N> returned=<M> failed=<K>   unaccounted: <task list or none>
Progress: <complete>/<total> complete on origin/main
To resume: <the decision or action needed, then /run-spec <spec>>
```

Every number in either report comes from `runspec.py summary` and `runspec.py status`, never from memory of the run: a count that cannot be produced is printed as `unknown`.

## Output

One of the two reports.
