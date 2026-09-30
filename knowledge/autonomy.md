# Autonomous delivery

How a delivery run proceeds without a maintainer watching, what bounds it, and why each mechanism is shaped the way it is. The procedure is `/deliver-backlog`; the rules agents follow are in `.claude/rules/autonomous-delivery.md`; the state files are described in [`orchestration/README.md`](../orchestration/README.md). The execution and finalize machinery it drives is in [`execution.md`](execution.md) and [`finalize.md`](finalize.md).

---

## The model

| Element | Fact | Source |
|---|---|---|
| Grant | The maintainer's time-boxed (1 to 24 hours, renewable), scope-limited (cards and specs) permission for one session to run unattended | `scripts/orchestration/autonomy.py` |
| Who arms it | Only the maintainer, from a terminal. Agents are blocked from `grant` and `renew` and from writing the grant file; anyone may `revoke` | `autonomy.py`, `.claude/hooks/guard-autonomy.sh` |
| Who it binds | The session named in it, and that session's subagents (whose hooks carry the parent's session id). Every other session behaves as if there were no grant | `hh_grant_state` in `.claude/hooks/hook-helpers.sh` |
| Run state | `INTENT.md`, `RUN-LOG.md`, `QUESTIONS.md` in the main checkout's `orchestration/`, written only by `delivery.py` | `scripts/orchestration/delivery.py` |
| Actionable | An item an agent can advance now; building waits for dependencies, specifying does not | `delivery.py actionable` |
| Keeping going | The Stop hook continues the granted session while work is actionable, within the grant's continue budget | `.claude/hooks/continue-run.sh` |
| Asking | Never synchronous in a granted or unattended session; questions go to `QUESTIONS.md` and park only their items | `.claude/hooks/guard-blocking-ask.sh` |
| Merging | Only after a ready `merge-check` for that pull request at that exact head, under this grant, in the last 15 minutes | `autonomy.py merge-check`, `guard-autonomy.sh` |
| Shared resources | Advisory lanes keyed by (main checkout, resource), owned by an explicit token, stale only when the holder process is gone | `scripts/orchestration/lanes.py` |
| Resumption | Optional systemd user timer that resumes the granted session when it stopped with work left | `scripts/orchestration/heartbeat.sh` |

## What a grant permits

A live grant bound to the session permits exactly this:

- **Continuing unattended** over the cards and specs in its scope. `continue-run.sh` turns an idle stop into the next action while `delivery.py actionable` lists work, at most `max_continues` times per grant (default 30), at most three chained continues in a row, and not twice within a minute. A final line `AWAITING MAINTAINER: <reason>` always releases the session.
- **Merging pull requests** that are green, have no unresolved review thread, pass the public-hygiene check, change only files inside their task's scope, and belong to a spec in the grant's scope. `autonomy.py merge-check` decides it with `scripts/harness/runspec.py pr-check` (task and lifecycle pull requests) or `scripts/harness/finalize.py publish-check` (finalize and fix pull requests), records the verdict with the head commit, and the merge must be `gh pr merge <n> --squash --match-head-commit <sha>`. A pull request that needs an accepted scope exception is not mergeable under a grant; it goes to the maintainer.
- **Pre-approved lifecycle syncs**, only when the maintainer granted with `--allow-pm-sync`: a granted card's status moving one step along specced → implementing → shipped (or a Half shipped note while it waits), the `lifecycle-sync` decision entries naming granted cards, and the roadmap regenerated from the backlog. `autonomy.py pm-sync-check` decides it from the current and proposed file. Every other product change still needs the maintainer's signed approval.

It never permits: granting or extending itself, pushing to `main`, tags, releases, workflow runs or settings (`guard-autonomy.sh` blocks the arming script in a granted session, so `guard-publish.sh` and `guard-main-push.sh` stay closed), other product changes, answering its own questions, or installing the heartbeat. When the grant expires, the session may not merge at all until the maintainer renews or revokes it, and a grant file that is unreadable or out of bounds stops merging and arming in every session until it is revoked.

The grant's spec checkpoint is on by default: each spec waits at `approval` after `/spec` until the maintainer approves it. `--no-spec-checkpoint` lets the run go from spec to implementation without that stop.

## Arming, renewing, revoking

```bash
# In Claude Code (the ! prefix runs it in your own shell, with a terminal):
! scripts/orchestration/autonomy.sh grant --hours 8 --scope MH-3,MH-5,spec:status-json \
    --reason "overnight delivery of the status work" --allow-pm-sync
# For Codex, get the ID from the session's environment, then use a real terminal:
#   printenv CODEX_SESSION_ID
# The Codex shell-command tool is noninteractive and cannot issue a grant.
# In a terminal, name that session explicitly:
scripts/orchestration/autonomy.sh grant --hours 8 --scope MH-3 --reason "..." --session <session id>

scripts/orchestration/autonomy.sh status
scripts/orchestration/autonomy.sh renew --hours 8      # a new window of at most 24h; same scope and session
scripts/orchestration/autonomy.sh revoke               # anyone, any time
```

Every grant, renewal, revocation, merge verdict, allowed merge, continue and heartbeat decision is a row in `.claude/data/autonomy-audit.jsonl`.
When invoked in a terminal that carries the session environment, `grant` also reads
`CLAUDE_CODE_SESSION_ID`, `CODEX_SESSION_ID` or `CODEX_THREAD_ID`; `--session` takes precedence.
Codex grants bind by session ID, but the automatic Stop continuation and optional heartbeat
currently use Claude Code hooks and CLI. A Codex run must be continued by its operator.

## Why each piece exists

Each row is a failure seen when agents deliver unattended, and the mechanism that removes it.

| Failure mode | Why it happens | Mechanism |
|---|---|---|
| A session told in prose that it has full authority still stops after every unit of work and waits for someone to type "continue" | Ending a turn is a harness event; an instruction in the conversation cannot change it, and it fades as the context grows | `continue-run.sh` on Stop, bounded by the grant's budget, the chain limit and the interval (`test_continue_run.sh`) |
| An unattended session sat blocked on a question for hours, and most such questions were answered with the option the agent had already recommended | A synchronous question stalls the whole session, with no other work in flight | `guard-blocking-ask.sh`; questions are filed, only their items park, and recommended defaults inside the run's authority are taken and logged |
| An agent granted itself autonomy, or re-armed an expired grant | The grant command was reachable from the agent's shell | `grant` and `renew` need a terminal and are blocked for agents in any spelling the tokenizer sees; the grant file is blocked for Edit, Write and shell writes (`test_guard_autonomy.sh`) |
| A mistyped duration armed a grant for a year | The hours argument was not validated | Whole hours 1 to 24, refused before anything is written; a grant whose window exceeds 24 hours is invalid and binds nothing (`test_autonomy.py`) |
| One global grant silenced questions in the maintainer's own interactive sessions | The grant named no session | The grant binds one session; the hooks compare the payload's session id |
| The Stop hook released a session whose last message merely mentioned the exit sentinel | The sentinel was matched anywhere in the message | Only the last non-empty line counts (`test_continue_run.sh`, "a mention is not an exit") |
| Merges happened on a head nobody checked, or on a spec the maintainer never granted | The merge and the check were separate steps with no link | The check records the head; the merge must pin it with `--match-head-commit`, within 15 minutes, under the same grant |
| A subagent that acquired and released "its" lock deleted the orchestrator's | Locks were keyed by session id, and a subagent carries its parent's | Lanes are owned by an explicit token the orchestrator never hands out; a non-owner release is refused (`test_lane.sh`) |
| A lock of a long-running session was reaped, or a crashed session's lock blocked everyone | Staleness was judged by age | A lane is stale only when its holder process is gone or its pid was reused (start time differs); age never matters |
| Run-log timestamps drifted hours from the truth and corrupted every duration computed from them | Agents estimated times instead of reading the clock | Only `delivery.py` writes the log, stamping each line from the clock; direct writes are blocked |
| File-and-line citations into run state went stale | Archiving or rewriting a state file shifts every line below | The log is append-only and entries are cited by id or timestamp, never by line |
| A worker told to be read-only committed and pushed anyway | A skill's own commit instructions overrode the dispatch prompt | The orchestrator re-derives HEAD, origin and the pull request after every dispatch; workers run in worktrees and never merge |
| Uncommitted files in the shared checkout looked like a peer's live work and were committed, reverting later fixes | They were stale residue of work already merged | Never commit another session's files; compare them with blobs on origin before treating them as work |
| Branches and worktrees of merged pull requests piled up, and the only cleanup was a force delete | Squash merges leave no merged ancestry for `git branch -d` to see | `scripts/harness/prune-merged.sh` deletes only after GitHub reports the pull request merged and the tip is part of its head; `block-destructive.sh` keeps blocking unverified deletes |
| A timer-started session could not continue under the grant | A new session is outside a session-bound grant | The heartbeat resumes the granted session (`claude -p --resume <session>`), marks it unattended, and never skips permissions |

## The heartbeat (optional)

`scripts/orchestration/heartbeat.sh` is opt-in. It resumes the granted session when four things hold: a live grant, actionable work, no live session holding the `run:delivery` lane, and the Claude Code CLI on `PATH`. One tick buys one print-mode turn plus whatever continues the Stop hook grants; it is periodic resumption, not a daemon. Nothing in the repository installs it. The maintainer does, from their own terminal:

```bash
scripts/orchestration/heartbeat.sh --print-install   # prints the commands below for this checkout
mkdir -p ~/.config/systemd/user
cp scripts/orchestration/systemd/mythhelm-heartbeat@.{service,timer} ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now "mythhelm-heartbeat@$(systemd-escape --path "$PWD").timer"
```

The units are templates: the instance is the checkout's escaped path, so they carry no hard-coded path (`%f` is the checkout, `%h` the home directory). Revoking the grant idles every tick; `systemctl --user disable --now` removes the timer.

## Hook payload facts

- `Stop` payloads carry `session_id`, `cwd`, `transcript_path` and `stop_hook_active`; newer versions also carry `last_assistant_message`. `continue-run.sh` falls back to the newest assistant text in the transcript when that field is absent.
- Exit 2 on `Stop` keeps the session working and shows stderr to the model; `stop_hook_active` is true on the stop that follows. That is what the chain limit counts.
- A subagent's tool calls carry its parent's `session_id`, so a grant covers the granted session's workers, and a session id can never identify a lock owner.
- `SessionStart` and `UserPromptSubmit` hooks add context only through `{"hookSpecificOutput": {"hookEventName": "<event>", "additionalContext": "<text>"}}` (or plain stdout). A top-level `additionalContext` is silently ignored, so a hook written that way looks alive in its own tests and does nothing. None of the autonomy hooks uses these events; a future one must use that shape and test it.

## Data

| File | Where | Written by |
|---|---|---|
| `.claude/data/autonomy-grant.json` | main checkout | `autonomy.py grant`, `renew`; removed by `revoke` |
| `.claude/data/autonomy-audit.jsonl` | main checkout | `autonomy.py`, `guard-autonomy.sh`, `continue-run.sh`, `heartbeat.sh` |
| `.claude/data/autonomy-continue.json` | main checkout | `continue-run.sh` (the counters, keyed by grant id) |
| `.claude/data/lanes/*.json`, `lanes/audit.jsonl` | main checkout | `lanes.py` |
| `orchestration/INTENT.md`, `RUN-LOG.md`, `QUESTIONS.md` | main checkout | `delivery.py` |
| `.claude/handoffs/<timestamp>-<task>.md` | main checkout | `/handoff` |

All of it is gitignored.

## Residuals

The terminal test is a cost, not a boundary: a pseudo-terminal passes it. The grant and the run files are ordinary files the same user can write, and the guards read command text, so a write from inside a script or through a computed path is invisible to them. The mechanisms stop the ordinary agent path, a session granting itself autonomy or merging what it did not check; review of every pull request and the maintainer's revoke remain the backstop.

The lifecycle-sync pre-approval takes effect in the product approval guard: when a Write or Edit under `product/` has no matching signed approval, `scripts/orchestration/approvals.py check-write` asks the same question as `autonomy.py pm-sync-check` (the grant covers the session, and the change is a pre-approved sync of granted cards) and releases the write only on a yes, recording it as a `preapproved` row in the approval ledger and in the autonomy audit. Any failure to decide blocks. The roadmap check regenerates the roadmap from the main checkout's backlog, so a sync is written backlog first, then decisions, then roadmap.
