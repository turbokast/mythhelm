# Agent workflow

How work moves from an idea to merged code in MYTHHELM, for the maintainers who direct the agents. Where this guide and [`docs/harness/charter.md`](docs/harness/charter.md) disagree, this guide wins. Agents find the procedures in the skills it names; the reasons behind them are in [`knowledge/`](knowledge/README.md).

## From idea to spec

A change larger than a single fix becomes a spec: a directory under `specs/<state>/<name>/` that moves through lifecycle states. [`specs/README.md`](specs/README.md) has the states and who moves a spec between them.

| Step | Command | Result |
|---|---|---|
| Capture | `/create-spec` | `unrefined/<name>/requirements.md` |
| Refine | `/refine-spec <name>` | `refined/`: requirements a design can be written from |
| Design and plan | `/spec <name>` | `todo/`: `design.md`, `tasks.md` and `scratchpad.md`, validated by a fresh reviewer |
| Review an old spec | `/evaluate-spec <name>` | the spec updated against the code, or archived |

## From spec to merged code

`/run-spec <name>` implements a spec in `todo/` or `in-progress/`. It runs in your session as the orchestrator:

1. **Start.** It moves the spec to `in-progress/` and seeds `handoff.md` in one small lifecycle pull request, and merges it once CI is green.
2. **Schedule.** It parses `tasks.md` and picks the tasks whose dependencies are merged on `origin/main`. Tasks whose `Files` lists do not overlap run in parallel, up to four at a time.
3. **Dispatch.** Each task goes to the agent its `Domain/agent` line names, so the agent's model pin applies, in its own git worktree cut from `origin/main`. The prompt carries the hand-off notes of the tasks it depends on. The worker follows `/implement`: it writes the tests first, runs the gates through `scripts/harness/gate.sh`, and opens one pull request. That pull request contains the code, the task's completion entry in `tasks.md` and its section of `handoff.md`.
4. **Verify.** The orchestrator trusts no report. It reads the pull request from GitHub, checks that the worktree's gate markers match the exact tree that was pushed, checks the completion entry, and re-runs the task's acceptance tests itself.
5. **Review and merge.** CodeRabbit and Sourcery review every pull request. The orchestrator sends each round of findings back to the same worker, which fixes or rebuts every thread and resolves it. The orchestrator merges when `scripts/harness/runspec.py pr-check` reports `verdict=ready`: every check green, no unresolved thread, the entry in place, and nothing in the title, body or diff that must not be published. It then confirms the merge on `origin/main`.
6. **Finish.** When the last task has merged, it moves the spec to `unfinalized/` with a second lifecycle pull request and prints a summary: tasks, attempts, review rounds, and the `dispatched / returned / failed` accounting.

`/implement <name> [--task N]` is the same work for one task, driven by hand: you or an agent take one task to an open pull request.

### What stops a run

The orchestrator stops and reports to you when:

- a task fails three attempts;
- three review rounds leave its pull request not ready;
- nothing can start (a blocked task, a dependency cycle, or a `maintainer` task waiting for you);
- a worker asks a question the spec does not answer;
- the spec has a duplicate copy or is in the wrong state.

The stop report names the task, the evidence and what is needed. Re-running `/run-spec <name>` resumes: completed tasks are read from `origin/main`, and open pull requests are picked up where they stand.

Tasks whose `Domain/agent` is `maintainer` are yours. The run hands them over when they become ready and continues with everything that does not depend on them.

## Gates, markers and the completion check

- **Gate markers.** `scripts/harness/gate.sh <gate|go|harness|all>` runs a gate's fixed command and, only when it exits 0 over an unchanged tree, writes `.claude/data/gate-marker-<gate>.json` with a fingerprint of the files it checked. Editing a file afterwards makes the marker stale. `python3 scripts/harness/gatelib.py status` shows what the current change set needs and whether each marker is fresh.
- **The completion check.** When a Claude Code session marks a task complete in `tasks.md`, the Stop hook (`.claude/hooks/verify-task-completion.sh`) keeps it working until the entry is well-formed and every needed marker is fresh. Other sessions' work in the same checkout never triggers it. If it blocks wrongly, the session can record an override with a reason: `python3 scripts/harness/gatelib.py override --session <id> --reason "<why>"`. The override lasts only until the next file change, and every block and override is logged in `.claude/data/stop-gate-audit.jsonl`.
- **Dispatch pins.** While `/run-spec` or `/implement` runs, `.claude/hooks/guard-dispatch-pin.sh` refuses a dispatch that does not name a project agent.

## Where the state lives

| What | Where |
|---|---|
| Which tasks are complete | `tasks.md` on `origin/main` |
| Work in flight | the spec's open pull requests |
| Notes for dependent tasks | `specs/<state>/<name>/handoff.md` |
| Open questions and research | `specs/<state>/<name>/scratchpad.md` |
| Attempts, retries, review rounds | `.claude/data/run-events.jsonl` (local, gitignored) |
| Gate evidence | `.claude/data/gate-marker-*.json` in each worktree (local) |
