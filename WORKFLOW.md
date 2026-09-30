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
6. **Finish.** When the last task has merged, it moves the spec to `unfinalized/` with a second lifecycle pull request and prints a summary: tasks, attempts, review rounds, and the `dispatched / returned / failed` accounting. `/finalize-spec` takes it from there.

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

## From merged tasks to a finished spec

`/finalize-spec <name>` closes a spec that `/run-spec` left in `unfinalized/`. The tasks' code is already on `main`; finalizing reviews it as one change, records how it went, and checks that `main` stays green. It works in its own worktree of `origin/main` and lands its record as one pull request.

1. **Verify.** `scripts/harness/finalize.py verify` reads `origin/main` and GitHub: every task complete and merged, no unresolved review thread, no open task or fix pull request, `main` green at its tip, the required checks and the `CI OK` job's dependencies not reduced. Anything else stops the run with the reason.
2. **Review the whole spec.** The `code-reviewer` agent (and `architect` when the change spans domains) reviews the cumulative diff, from the parent of the first task's merge to the last merge, against the requirements, the design and the invariants. The focus is what no single task's review could see: the seams between tasks, the acceptance criteria as a whole, the design's drift. Codex or Muse add an advisory review when you have enabled them. A confirmed critical finding stops the run until a fix pull request merges.
3. **Retrospective and proposals.** `specs/<state>/<name>/retrospective.md` records the review summary, each acceptance criterion against what shipped, deviations, CI history, effort from the run log, and lessons. Lessons that point at the harness itself become proposals in [`.claude/proposals/pending.md`](.claude/proposals/README.md), for you to accept or reject.
4. **Publish.** One pull request, `docs(spec): finalize <name>`, moves the spec to `done/` (and its epic, when it was the last work stream), updates the docs the spec made stale (`/update-docs`), and adds `CHANGELOG.md` entries for what users can notice. It goes through the normal checks and review, and the finalizer merges it when `finalize.py publish-check` reports `verdict=ready`.
5. **Verify CI.** `finalize.py ci` watches `main`'s runs for the merge commit. A failed job is classed `real`, `infra` or `unknown`, never "flaky". A real failure gets up to two fix pull requests. An infra or unknown failure comes to you, since re-running a job is yours to decide.
6. **Backlog and release.** The spec's backlog cards move to `shipped` through approval requests you sign. With `--release`, `/finalize-spec-tag` prepares the release notes from the changelog and the release draft. It publishes the release only when you pass `--publish`.

The finalizer stops and reports when verification fails for a reason no fix can remove, a critical finding cannot be fixed, three review rounds leave the finalize pull request not ready, or `main` stays red. Re-running `/finalize-spec <name>` resumes. [`knowledge/finalize.md`](knowledge/finalize.md) explains the reason for each mechanism.

## Gates, markers and the completion check

- **Gate markers.** `scripts/harness/gate.sh <gate|go|harness|all>` runs a gate's fixed command and, only when it exits 0 over an unchanged tree, writes `.claude/data/gate-marker-<gate>.json` with a fingerprint of the files it checked. Editing a file afterwards makes the marker stale. `python3 scripts/harness/gatelib.py status` shows what the current change set needs and whether each marker is fresh.
- **The completion check.** When a Claude Code session marks a task complete in `tasks.md`, the Stop hook (`.claude/hooks/verify-task-completion.sh`) keeps it working until the entry is well-formed and every needed marker is fresh. Other sessions' work in the same checkout never triggers it. If it blocks wrongly, the session can record an override with a reason: `python3 scripts/harness/gatelib.py override --session <id> --reason "<why>"`. The override lasts only until the next file change, and every block and override is logged in `.claude/data/stop-gate-audit.jsonl`.
- **Dispatch pins.** While `/run-spec`, `/implement` or `/finalize-spec` runs, `.claude/hooks/guard-dispatch-pin.sh` refuses a dispatch that does not name a project agent.

## Where the state lives

| What | Where |
|---|---|
| Which tasks are complete | `tasks.md` on `origin/main` |
| Work in flight | the spec's open pull requests |
| Notes for dependent tasks | `specs/<state>/<name>/handoff.md` |
| Open questions and research | `specs/<state>/<name>/scratchpad.md` |
| How a finished spec went | `specs/done/<name>/retrospective.md` |
| Proposed harness improvements | `.claude/proposals/pending.md` |
| User-facing changes | `CHANGELOG.md` |
| Attempts, retries, review rounds | `.claude/data/run-events.jsonl` (local, gitignored) |
| Gate evidence | `.claude/data/gate-marker-*.json` in each worktree (local) |
