# Skill Invocation Contexts

Every skill runs in three contexts and must behave correctly in each:

1. **Slash command**: a person types `/name`.
2. **Model-invoked**: the model calls the skill through the Skill tool in a session where a person can reply.
3. **Non-interactive**: the skill runs in a dispatched subagent, a headless `claude -p` run, a scheduled job, or anywhere no person can reply.

Arguments arrive as `$ARGUMENTS` in all three. Never assume a person typed them.

## How interactive steps behave in context 3

Classify each interactive step, and state its class at the step:

- **Input** ("ask for X"): take the answer from `$ARGUMENTS` and the invoking context first, in every context; ask only for what is still missing. In context 3, stop and return the unanswered questions. Never guess.
- **Write gate** ("confirm before writing"): an explicit pre-approval in `$ARGUMENTS` or the dispatching prompt satisfies it. Without one in context 3, write nothing: return the proposed writes and say they were withheld. Never approve your own write gate.
- **Advisory** (a recommendation with an override, then work continues): apply an override from `$ARGUMENTS` or the prompt; otherwise proceed with the recommendation and record `auto-confirmed (non-interactive)`.
- **Escalation** ("stop and ask for guidance"): stop and report the blockage, the evidence and the open questions to the dispatcher.

You are in context 3 when you were dispatched as a subagent, the session is headless, or no way to ask exists. When unsure, treat write gates as unconfirmed and advisory steps as auto-confirmable.

Every `SKILL.md` has an `## Invocation contexts` section naming its behaviour under **Slash command**, **Model-invoked** and **Non-interactive**. `scripts/ci/lint-agent-harness.sh` fails without it.
