---
name: run-spec-parallel-detection
description: Decide which ready tasks start now — dependencies merged on origin/main, Files disjoint from each other and from in-flight pull requests, at most four — using runspec.py batch
argument-hint: "<spec-name> [--in-flight N,M]"
---

# Run Spec — Parallel Detection

Which tasks `/run-spec` dispatches together. Parallelism is an optimisation: when in doubt a task waits for the next batch.

## Input

`$ARGUMENTS`: the spec name and, optionally, the in-flight tasks (open pull requests not yet merged).

## Invocation contexts

- **Slash command**: prints the batch and the deferred tasks with their reasons.
- **Model-invoked**: the same; `/run-spec` also reads this page inline.
- **Non-interactive**: the same. Read-only.

## Steps

1. `git fetch origin`, then compute the batch from the spec's `tasks.md`:

   ```bash
   python3 scripts/harness/runspec.py batch specs/in-progress/<spec>/tasks.md --in-flight <N,M> --max <max-parallel>
   ```

   It takes the ready tasks in task order and keeps each whose `Files` overlaps neither an earlier pick nor an in-flight task (same path, a directory containing it, or a matching glob). A ready task with no `Files` list runs alone. The rest are `deferred` with the reason.
2. For each task in the batch, `python3 scripts/harness/runspec.py deps-merged <spec> <N>` must exit 0. Workers start from origin/main, so a dependency whose pull request is not merged yet is invisible to them; such a task waits, whatever `tasks.md` in the local checkout says.
3. Drop `maintainer` tasks from the batch and report them to the operator.

## Why these rules

- **Disjoint Files, including in-flight pull requests.** Two open pull requests editing one file conflict at merge, and the second needs a merge of main and a re-run of its gates. The spec's own `tasks.md` and `handoff.md` are exempt: each task edits only its own block and its own seeded section, so those edits merge cleanly (`scripts/harness/tests/test_runspec.py` merges two such branches).
- **Merged, not just committed.** A worker's worktree is cut from origin/main. Output that exists only in another worktree or an unmerged branch is not there.
- **At most four.** Beyond four concurrent workers, review and merge become the bottleneck, and an early failure that changes the plan wastes more work.

## Output

The `batch` JSON: `batch`, `deferred` (task and reason) and `deadlock`, plus any task dropped at step 2 or 3.
