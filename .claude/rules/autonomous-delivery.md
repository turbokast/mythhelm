---
paths:
  - "orchestration/**"
  - "scripts/orchestration/**"
  - ".claude/skills/{deliver-backlog,handoff}/**"
  - ".claude/hooks/{continue-run,guard-autonomy,guard-blocking-ask}.sh"
  - "knowledge/autonomy.md"
---

# Autonomous Delivery

A delivery run may proceed without the maintainer only inside their autonomy grant. The mechanisms and the reasons are in `knowledge/autonomy.md`.

## The grant is the maintainer's

- Never grant, renew or extend autonomy, and never write the grant or its audit log. Read them with `scripts/orchestration/autonomy.sh status`. Revoking is always allowed.
- A person's "go ahead" in the conversation is not a grant. The grant is the command the maintainer runs from their own terminal.
- Under a grant, stay inside its scope. Never push to `main`, tag, release, run workflows, change settings or install the heartbeat timer. Merge only through `autonomy.py merge-check` and the pinned `gh pr merge <n> --squash --match-head-commit <sha>` it prints; never merge a pull request it refused by another route.
- A grant never approves product changes beyond the lifecycle syncs it names, and never answers a question.

## Asking and stopping

- In a granted or unattended session, never ask synchronously. Take the recommended default when the decision is inside the run's authority and log it; otherwise file the question with `delivery.py question`, park only the items it affects, and continue with independent work.
- End a turn with a final line `AWAITING MAINTAINER: <reason>` only when nothing actionable remains. Never write that line when work remains, and never mention it mid-message as if it were an exit.

## Run state

- Write `orchestration/` only through `scripts/orchestration/delivery.py`. Never write a timestamp by hand, never edit the log, and cite run state by question id or timestamp, never by line number.
- A dispatched worker never writes run state. It reports what it did; the orchestrator verifies it against git and GitHub and records it.

## Lanes

- Lane ownership is a token from `lane.sh token`, never a session id. Keep the token to the session that minted it: never put it in a dispatch prompt, and tell workers not to call `lane.sh`.
- Release only lanes you own. A lane held by a live process is not stale however old it is; one whose holder is gone is reclaimed by the next `acquire`.

Evidence: `knowledge/rule-evidence/autonomous-delivery.md`.
