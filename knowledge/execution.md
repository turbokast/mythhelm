# Execution

How a spec's tasks become merged code, and why the machinery is shaped the way it is. The procedures are in the skills (`/run-spec`, `/implement`, `/task-completion`); this page holds the facts and reasons they rest on.

---

## The model

| Element | Fact | Source |
|---|---|---|
| Unit of work | One task, one worker, one linked worktree cut from `origin/main`, one pull request | `.claude/skills/run-spec/SKILL.md` |
| Complete | The task's pull request is merged, its merge commit is on `origin/main`, and `tasks.md` there carries its entry naming the pull request | `scripts/harness/runspec.py verify-merged` |
| Ready to merge | Every latest check passing or skipped (`CI OK` and `CodeRabbit` present), zero unresolved review threads, not conflicting or behind, entry present and well-formed, scope recorded, no leak in title, body or added lines | `runspec.py pr-check` |
| Dependency satisfied | The dependency is complete in `tasks.md` **on origin/main** | `runspec.py deps-merged` |
| Durable state | `tasks.md` on `origin/main` plus the spec's open pull requests; the local event log only adds attempts and accounting | `.claude/data/run-events.schema.json` |
| Hand-off | `handoff.md` in the spec directory, one seeded section per task, pasted into dependants' dispatch prompts | `runspec.py handoff-seed`, `runspec.py handoff` |
| Gate evidence | A marker per gate per tree, written by the wrapper only for an exit-0 run over an unchanged tree | `scripts/harness/gatelib.py`, `.claude/data/gate-marker.schema.json` |

## Why each piece exists

Each row is a failure mode seen when agents run a spec, and the mechanism that removes it.

| Failure mode | Why it happens | Mechanism |
|---|---|---|
| A gate "passed" but no marker was written, or a marker attests a run that failed | A `PostToolUse` Bash payload may carry no exit code; a backgrounded command's `PostToolUse` fires at launch, before the gate finishes; a hook that pattern-matches the command text misses `cd … &&`, absolute paths and pipes | The wrapper `scripts/harness/gate.sh` runs the gate itself and writes the marker from the real exit status, whatever the launch form (`scripts/harness/tests/test_gatelib.sh`). No `PostToolUse` hook records passes. |
| Markers never agree with the checker | Two components hashed different file sets, one including build output that churns | One fingerprint function over tracked and untracked, non-ignored files (`gatelib.fingerprint`), used by the wrapper and the Stop hook alike |
| A marker vouches for another tree | Hook environments name the session's launch directory, not the worktree a subagent works in | Markers live in, and record, the tree they attest; a copied marker is stale (`test_gatelib.sh`, "a marker attests only its own tree") |
| A Stop hook blocks a session for another session's work in the same tree | Whole-tree checks cannot tell whose change it is | A claim counts only when this session's own transcript (the subagent's own on `SubagentStop`) edited the `tasks.md` (`.claude/hooks/tests/test_verify_task_completion.sh`) |
| A wrong block traps a session | A Stop hook that always blocks loops until the harness gives up | Three blocks on an unchanged tree, then a released and audited stop; an audited override bound to session, tree and bytes |
| A worker "worked around" a guard with `git stash`, wiping everyone's uncommitted work | Workers shared the main checkout and its index | Workers never use the shared checkout; `block-destructive.sh` blocks `git stash` and index-wide adds |
| A dependent task started without its dependency's output | Worktrees are cut from `origin/main`, not from local, unpushed state | Dependencies must be merged on `origin/main` before dispatch; the worker switches to a branch from `origin/main` explicitly |
| Worktree results "merged" as a no-op | Workers left their changes uncommitted, and the merge of an unchanged branch reports success | The result is a pushed pull request; the orchestrator checks the worktree is clean and its `HEAD` equals the pull request head |
| A worker read a stale duplicate of the spec | A plain `mv` left a second copy in the git index | `spec-lifecycle.sh resolve` refuses two copies; lifecycle moves are their own pull requests |
| Work reported done was not done | Summaries were trusted; empty or garbled tool output was filled in | Every claim is re-derived: report parsed, pull request read from GitHub, markers checked, acceptance re-run by the orchestrator |
| A subagent idled "waiting" for a background command | A subagent is not notified when its own background command ends | Foreground-only gates in every dispatch; stalled workers are resumed with evidence |
| Implementation ran on the orchestrator's model | A dispatch without `subagent_type` falls back to the session model | `.claude/hooks/guard-dispatch-pin.sh` blocks it while an implementation or finalize skill runs |
| Bookkeeping rows collided between concurrent runs | Per-task files were keyed by task number only | One append-only `run-events.jsonl` in the main checkout, every row stamped with its spec |
| An absence criterion failed on the agent's own test | Negative tests and comments carried the banned token | Dispatch rule, and the orchestrator re-runs the acceptance greps itself |
| Parallel pull requests conflicted in the spec files | Every task appended to the same place | Each task writes only its own `tasks.md` block and its own seeded `handoff.md` section (`scripts/harness/tests/test_runspec.py`, the two-branch merge) |

## Hook payload facts

What the hooks rely on, from the [Claude Code hooks reference](https://code.claude.com/docs/en/hooks) and the tests that pin the behaviour here:

- `Stop` and `SubagentStop` payloads carry `session_id`, `cwd`, `stop_hook_active` and `transcript_path`; `SubagentStop` also carries `agent_transcript_path`, the subagent's own transcript. A subagent's tool calls carry its parent's `session_id`, so a session id alone cannot tell a worker from its orchestrator. The Stop hook treats a missing or unreadable transcript as "this session edited the file", so an absent field never waves a claim through.
- Exit 2 on `Stop` keeps the session working and shows stderr to the model; `stop_hook_active` is true on the stop that follows such a block.
- A `systemMessage` in a hook's JSON output is shown to the user; stderr on exit 0 is shown to no one.
- An agent dispatched with `isolation: "worktree"` works in its worktree, so `gate.sh` and the `SubagentStop` check resolve that tree from the working directory. The orchestrator's own check of the worker's markers (`gatelib.py status --root <worktree>`) names the tree explicitly and does not depend on it.

## Data files

All under `.claude/data/`, gitignored except the schemas:

| File | Where | Written by |
|---|---|---|
| `gate-marker-<gate>.json` | the tree the gate ran in | `gate.sh` |
| `run-events.jsonl` | the main checkout | `runspec.py event` (orchestrator only) |
| `stop-gate-<session>.json`, `stop-gate-override-<session>.json`, `stop-gate-audit.jsonl` | the main checkout | the Stop hook and `gatelib.py override` |

The main checkout's copy outlives every linked worktree, so anything that must survive a worktree's removal is written there.
