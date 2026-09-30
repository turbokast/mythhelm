---
name: handoff
description: Write an evidence-grounded continuation package so a fresh session can resume this work exactly where it stands — git state, open pull requests, the delivery run's position, the next action — saved as a timestamped file under .claude/handoffs/ with a copy-paste prompt for the next session
argument-hint: "[focus for the next session]"
---

# Handoff

Prepares the next session to continue this one's work without rediscovering it. Run it in the session that did the work: the synthesis needs context only this session holds, so it is never delegated to a subagent.

Use it when the maintainer asks for a handoff, or when this session must end before its work does (its context is nearly full, or the maintainer is leaving and no autonomy grant will carry the run). It is not a way to stop early: work that can finish now finishes now.

`$ARGUMENTS` is an optional focus for the next session. It may reorder the priorities; it never drops unfinished work, which the handoff still lists.

**Read-only toward the project.** The skill writes exactly one file, the handoff. It never commits, pushes, stashes, resets, switches branches, removes worktrees, kills processes, re-runs long suites or continues the implementation.

## Invocation contexts

- **Slash command**: runs Steps 1–5 and prints the output.
- **Model-invoked**: the same, when the maintainer asked for a handoff or the session must end with work left.
- **Non-interactive**: the same; nothing here asks a question. When the mission cannot be recovered from the conversation and the files (Step 1), the handoff says so and names the best recovery source instead of guessing.

## Step 1: Recover the mission

From the conversation, the spec, the delivery run and the project instructions, capture:

- the objective and its definition of done, including merges, finalize, docs and any release step in scope; "done" never quietly shrinks to "code written";
- every requirement, constraint and exclusion still in force, and the authority the work runs under (an autonomy grant: its scope and deadline from `scripts/orchestration/autonomy.sh status`);
- decisions with their reasons, approaches rejected and why, and lessons from failed attempts;
- changes of direction: the earlier instruction and what replaced it;
- every outstanding commitment across the whole scope, not only the latest subtask.

When important history is gone (the context was compacted, say), write that down and name where it can be recovered from: the spec, `orchestration/RUN-LOG.md`, a commit range, a pull request. Never invent it.

## Step 2: Establish the current state

Look only at what the handoff needs, for each checkout or worktree in play:

```bash
git -C <path> rev-parse --abbrev-ref HEAD
git -C <path> rev-parse --short HEAD
git -C <path> status --short
git -C <path> log --oneline -5
git -C <path> rev-list --left-right --count HEAD...@{upstream}
git -C <path> worktree list
```

Then:

- `scripts/orchestration/status.sh --no-fetch` when a delivery run exists: the grant, the run's items and stages, lanes, open pull requests with their unresolved-thread counts and main's CI.
- `gh pr list --author @me --state open` for pull requests outside the run.
- Which uncommitted files are this work's and which are another session's. Never claim or sweep the others.
- The exact task in progress, how far it got, and the next concrete action.
- Test and gate results: what ran, on which tree, with what result. A result from before a later edit is stale; say so. Gate markers: `python3 scripts/harness/gatelib.py status`.
- Delegated agents and background jobs: finished or not, and whether their output was collected. One that was stopped or never reported is **UNKNOWN**, and its partial work is inspected in its worktree, not assumed.
- Pending maintainer actions: open questions in `orchestration/QUESTIONS.md`, approval requests, anything needing credentials. Environment variables by name only.

Label every claim **VERIFIED**, **REPORTED**, **ASSUMED** or **UNKNOWN**. Code existing is not done, and a test that passed earlier is not passing now.

Never copy a secret, token or credential value into the handoff.

## Step 3: Write the handoff

Write `.claude/handoffs/<UTC timestamp>-<short-task-id>.md` in the main checkout (the directory is gitignored), with the timestamp from `date -u +%Y-%m-%dT%H%MZ`, never estimated. Never overwrite an existing file. Fill [`template.md`](template.md). Keep it short: link the authoritative files instead of copying them, and preserve anything that exists only in this conversation.

## Step 4: Check it before presenting it

1. Read the file back: it exists and every section is filled or marked not applicable.
2. Every path it names exists (`test -e`), or is marked MISSING or EXTERNAL.
3. Compare it with the Step 1 list: no unfinished requirement fell out.
4. Verified and unverified claims are visibly separate.
5. The next action is specific and executable: a command, a skill invocation, or a file and the change to make.
6. "Lost when this session ends" names what does not survive: uncommitted work in a place the next session will not look, running background agents or jobs, temporary files outside the repository, credentials held only by this session.

## Step 5: Write the next session's prompt

One prompt, about 150 to 300 words, addressed to the next session. It:

- names the exact checkout and worktree paths and the exact handoff file (never "the latest");
- states the objective and the first substantive task;
- tells it to read the project instructions and the handoff, then check the live state (git status, the worktrees, the pull requests, `scripts/orchestration/status.sh`) before editing, since the handoff is a snapshot;
- says this continues existing work: keep valid decisions and working code, do not repeat finished discovery, and use the project's skills (`/deliver-backlog`, `/run-spec`, `/finalize-spec`);
- asks for the shortest credible path to done, with verification proportionate to the change and no lowered acceptance bar;
- tells it to decide ordinary technical questions itself, respect the authority it runs under, and ask the maintainer only for a real blocker while continuing unblocked work;
- ends with: "Do not answer with another handoff or a plan. Execute."

## Output

Only:

1. the handoff file's path;
2. warnings about work that will be lost, and actions the maintainer must take (omit when there are none);
3. the next-session prompt in one fenced code block.
