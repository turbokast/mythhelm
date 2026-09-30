# Skill Invocation Contexts

Every skill runs in three contexts and must behave correctly in each: **slash command** (a person types `/name`), **model-invoked** (through the Skill tool, with a person able to reply) and **non-interactive** (a dispatched subagent, a headless `claude -p` run, a scheduled job, anywhere no person can reply). Arguments arrive as `$ARGUMENTS` in all three; never assume a person typed them.

## How interactive steps behave non-interactively

Classify each interactive step, and state its class at the step:

- **Input** ("ask for X"): take the answer from `$ARGUMENTS` and the invoking context first, in every context; ask only for what is still missing. Non-interactively, stop and return the unanswered questions. Never guess.
- **Write gate** ("confirm before writing"): an explicit pre-approval in `$ARGUMENTS` or the dispatching prompt satisfies it. Without one, non-interactively write nothing: return the proposed writes and say they were withheld. Never approve your own write gate. A per-item decision gate is satisfied only by a decision for that item; a blanket confirmation never covers it.
- **Advisory** (a recommendation with an override, then work continues): apply an override from `$ARGUMENTS` or the prompt; otherwise proceed with the recommendation and record `auto-confirmed (non-interactive)`.
- **Escalation** ("stop and ask for guidance"): stop and report the blockage, the evidence and the open questions to the dispatcher.

You are non-interactive when you were dispatched as a subagent, the session is headless, or no way to ask exists. When unsure, treat write gates as unconfirmed and advisory steps as auto-confirmable.

Every `SKILL.md` states its behaviour in each context (`agent-config-conventions.md`).
