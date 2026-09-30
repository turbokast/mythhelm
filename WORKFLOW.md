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

## Learning loop

The harness learns from its own runs. A finalize retrospective turns lessons that point at the harness into **proposals** in `.claude/proposals/pending.md`; `/research-practices` can add cited proposals from other projects' practice. Nothing in a proposal changes the harness until you decide it.

1. **Notice.** When proposals are pending, each Claude Code session starts with one line: how many, and how old the oldest is.
2. **Decide.** `/apply-proposals` shows each proposal and asks you to approve, reject or defer it, with your reason. It asks one proposal at a time; "approve all" decides nothing, and in a headless run it only returns the queue.
3. **Apply.** Each decision becomes its own pull request from `origin/main`. An approved change touches only the proposal's target, and a rule, skill or hook change comes with an eval case that fails without the change and passes with it (you can waive the case, with a reason). The decision moves from `pending.md` to the end of `applied.md`, which is append-only. The pull request goes through the normal checks and review, and merges when you pass `--merge` or merge it yourself.
4. **Lane 0 (off by default).** If you enable it in `.claude/proposals/auto-apply.json` and allow some files under `knowledge/`, a knowledge proposal that only appends to one of those files is applied and merged without asking you. To veto one, revert its pull request.
5. **Stay honest.** The eval cases in `.claude/evals/cases/` pin behaviours of the harness itself (guards still block, templates still route, the always-on budget still holds) and run in CI with the harness lint. `/health-check` prints a read-only report: budget headroom, evals, pending proposals, specs stuck in flight, stale gate markers, vendor opt-in, required checks, `main`'s CI, pull requests with unresolved threads, and local branches whose pull request merged.

[`knowledge/learning-loop.md`](knowledge/learning-loop.md) explains the reason for each mechanism.

## Autonomous delivery

`/deliver-backlog` takes a set of backlog cards and specs through the whole lifecycle, from `/create-spec` to `/finalize-spec`, in dependency order: specifying runs ahead, building waits until what it depends on is done. It orchestrates the skills above and never implements. Its state lives in the main checkout's `orchestration/` ([`orchestration/README.md`](orchestration/README.md)), so a new session resumes where the last one stopped.

With you present, it asks you at each spec checkpoint and when it is stuck. To let it run while you are away, grant it autonomy from your own terminal:

```bash
! scripts/orchestration/autonomy.sh grant --hours 8 --scope MH-3,MH-5 --reason "overnight run"   # in the session to grant
scripts/orchestration/autonomy.sh status        # also: renew --hours 8, revoke
```

The grant binds that one session and its workers for at most 24 hours (renewable), over the cards and specs you name. Agents cannot grant or renew it. While it is live:

- the session keeps working instead of stopping after each step, within a continue budget, until nothing is left that an agent can do; it then ends with `AWAITING MAINTAINER: <reason>`;
- it never asks you directly: questions go to `orchestration/QUESTIONS.md`, only the affected items wait, and you answer with `python3 scripts/orchestration/delivery.py answer Q-<n> --text "..."` or in the file;
- it merges a pull request only when it is green, has no unresolved thread, passes the leak check, stays inside its task's scope and belongs to a granted spec, pinned to the exact head it checked;
- each new spec still waits for your approval (`/deliver-backlog approve-spec <spec>`) unless you granted with `--no-spec-checkpoint`;
- backlog changes still need your signed approval. `--allow-pm-sync` marks the specced → implementing → shipped moves of granted cards as pre-approved (`autonomy.py pm-sync-check`), but the approval queue does not consult it yet, so until it does those moves are filed as ordinary requests for you to sign;
- it never pushes to `main`, tags, releases or runs workflows.

`scripts/orchestration/status.sh` shows the grant, the run, the lanes, the open pull requests with their unresolved threads, and `main`'s CI on one screen. An optional systemd user timer (`scripts/orchestration/heartbeat.sh --print-install`) resumes the granted session when it stopped with work left; nothing installs it for you. `/handoff` writes a continuation package when a session must end mid-work, and `scripts/harness/prune-merged.sh` removes the local branches and worktrees of merged pull requests. [`knowledge/autonomy.md`](knowledge/autonomy.md) explains each mechanism.

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
| Decided proposals | `.claude/proposals/applied.md` |
| Config regression cases | `.claude/evals/cases/` |
| User-facing changes | `CHANGELOG.md` |
| Attempts, retries, review rounds | `.claude/data/run-events.jsonl` (local, gitignored) |
| Gate evidence | `.claude/data/gate-marker-*.json` in each worktree (local) |
| A delivery run: items, stages, log, questions | `orchestration/` in the main checkout (local) |
| The autonomy grant and its audit log | `.claude/data/autonomy-grant.json`, `autonomy-audit.jsonl` (local) |
| Continuation packages | `.claude/handoffs/` (local) |
