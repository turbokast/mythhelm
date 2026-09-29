---
name: add-agent
description: Create a new agent definition at .claude/agents/<name>.md and its routing-table row in knowledge/agent-routing.md
argument-hint: "<agent-name> [one-line purpose]"
---

# Add Agent

Scaffold an agent definition that follows `.claude/rules/agent-config-conventions.md` and pin it in the routing table in the same change.

## Input

`$ARGUMENTS`: the agent name in kebab-case, optionally followed by a one-line purpose.

## Invocation contexts

- **Slash command**: asks for any missing answer in step 3, then writes after confirmation (step 5).
- **Model-invoked**: the same, sourcing answers from the conversation first.
- **Non-interactive**: returns the unanswered step 3 questions if any required answer is missing; writes only with explicit pre-approval (`--confirm` in `$ARGUMENTS` or the dispatching prompt), otherwise returns both drafts and states that writes were withheld.

## Steps

1. **Check for a clash.** If `.claude/agents/<name>.md` exists, stop and offer to edit it instead. The name must be lowercase letters, digits and hyphens.
2. **Read references.** `knowledge/agent-routing.md` §Adding an agent, and the closest existing agent: `.claude/agents/go-implementer.md` for an implementer, `.claude/agents/code-reviewer.md` for a read-only reviewer, `.claude/agents/harness-clerk.md` for a mechanical clerk.
3. **Gather requirements** (input step): source each from `$ARGUMENTS` and the context first; ask only for what is missing.
   - What does it do, and when should it be used? (the `description`; required)
   - Which paths does it own? Every path must belong to one domain in `knowledge/domains.md`; a new domain means editing that map too.
   - Which class of work: implementation, review, configuration or release, or mechanical? This fixes `model`, `effort` and `tools` per the routing page.
4. **Draft the files.**
   - `.claude/agents/<name>.md`:

     ```markdown
     ---
     name: <name>
     description: <what it does and when to use it>
     model: <opus|sonnet|haiku>
     effort: <xhigh for opus, high for sonnet; omit the line for haiku>
     tools: [<allowlist, only for read-only or mechanical agents>]
     ---

     # <Title>

     ## Role
     ## Domain scope
     ## Before you begin
     ## Workflow
     ## Gates
     ## Boundaries
     ## Escalation
     ```

     Implementers add `## Completion checklist` and `## Review threads`; reviewers add `## Output` and a `tools:` allowlist without Edit, Write or NotebookEdit; mechanical agents never get the Agent tool.
   - A row in the `## Canonical agent → model mapping` table of `knowledge/agent-routing.md`: `` | `<name>` | `<model>` | <Tier> | <rationale> | ``.
5. **Write** (write gate): show both drafts and write on confirmation.
6. **Verify.** Run `scripts/ci/lint-agent-harness.sh` and confirm the `frontmatter` and `routing-pins` checks pass.

## Output

The two paths written (or the drafts, when writes were withheld) and the lint result.
