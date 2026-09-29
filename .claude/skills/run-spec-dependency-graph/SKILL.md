---
name: run-spec-dependency-graph
description: Parse a spec's tasks.md into its task graph — numbers, agents, budgets, Files, dependencies, completion state — validate it, and report progress, the ready set and deadlock; the mechanism is scripts/harness/runspec.py
argument-hint: "<spec-name>"
---

# Run Spec — Dependency Graph

How `/run-spec` reads the task graph. The parser is `scripts/harness/runspec.py`; this page says what it extracts and what each field means for scheduling.

## Input

`$ARGUMENTS`: the spec name. `/run-spec` also reads this page inline.

## Invocation contexts

- **Slash command**: parses the spec's `tasks.md` and prints the graph and progress.
- **Model-invoked**: the same.
- **Non-interactive**: the same. Read-only; a parse failure is returned, never guessed around.

## What the parser extracts

`python3 scripts/harness/runspec.py tasks specs/<state>/<spec>/tasks.md` prints one object per `### Task N — <name>` block outside code fences:

| Field | Source | Scheduling meaning |
|---|---|---|
| `number`, `name` | the heading | the task's identity in branches, titles and events |
| `agent` | first word of `Domain/agent` | the dispatch's `subagent_type`; `maintainer` is the operator's |
| `budget` | first word of `Budget` | `standard` or `complex`; sets the dispatch's budget line |
| `depends` | `Depends on` (task numbers outside parenthesised notes; `None` is empty) | edges; a task with no `Depends on` depends on the task listed before it, the first task on none |
| `files` | backticked paths in `Files`, outside parenthesised notes | the overlap check for parallel dispatch; globs and directories count |
| `heading_complete`, `status_complete`, `complete` | ` ✅ COMPLETED` in the heading; a Status starting with `✅` | complete when either holds |
| `blocked` | a `Blocked` field | never dispatched; reported |
| `pr` | `PR #<n>` in Status | the task's merged pull request |

## Validation

`scripts/ci/lint-agent-harness.sh --only specs` is the gate: required fields present, `Depends on` naming existing tasks, no cycle. `runspec.py status` repeats the cycle check (`"cycle": true`) and lists tasks whose dependencies it cannot read under `unresolvable`; either stops the run as a spec defect.

## Progress and the ready set

`python3 scripts/harness/runspec.py status <tasks.md> [--in-flight N,M]` prints:

- `complete`, `incomplete`, `in_flight` (the tasks you passed that are still incomplete);
- `ready`: incomplete, not blocked, not in flight, every dependency complete;
- `waiting`, `blocked`, `unresolvable`;
- `deadlock`: tasks remain, none is ready and none is in flight;
- `marker_mismatch`: tasks complete by only one of heading and Status, for the completion clerk.

Scheduling reads `tasks.md` **on origin/main** for completion (`runspec.py deps-merged`): a dependency is satisfied when its merged pull request put the ✅ there, because every worker starts from origin/main. `python3 scripts/harness/runspec.py closure <tasks.md> <N>` gives a task's transitive dependencies, whose hand-off sections go into its dispatch.

## Worked example

```text
Task 1  (no Depends on: first task)        → depends []
Task 2  Depends on: Task 1                 → depends [1]
Task 3  Depends on: Task 1                 → depends [1]
Task 4  (no Depends on)                    → depends [3]   the task before it
Task 5  Depends on: Task 2, Task 4 (after both land) → depends [2, 4]
```

Nothing complete: ready `[1]`. After Task 1 merges: ready `[2, 3]`, a parallel batch when their Files are disjoint. After 2 and 3: ready `[4]`. After 4: ready `[5]`. If Task 3 were `Blocked`, then 4 and 5 could never start: after Task 2, nothing is ready or in flight, so `deadlock` is true and the run stops naming Task 3.

## Output

The `status` JSON, or the parser's error.
