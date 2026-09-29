---
name: add-skill
description: Create a new skill at .claude/skills/<name>/SKILL.md with the required invocation-contexts section
argument-hint: "<skill-name> [one-line purpose]"
---

# Add Skill

Scaffold a skill that follows `.claude/rules/agent-config-conventions.md` and behaves correctly in all three invocation contexts (`.claude/rules/skill-invocation-contexts.md`).

## Input

`$ARGUMENTS`: the skill name in kebab-case, optionally followed by a one-line purpose.

## Invocation contexts

- **Slash command**: asks for any missing answer in step 3, then writes after confirmation (step 6).
- **Model-invoked**: the same, sourcing answers from the conversation first.
- **Non-interactive**: returns the unanswered step 3 questions if any required answer is missing; writes only with explicit pre-approval (`--confirm` in `$ARGUMENTS` or the dispatching prompt), otherwise returns the draft and states that writes were withheld.

## Steps

1. **Check for a clash.** If `.claude/skills/<name>/SKILL.md` exists, or an existing skill does the same job, stop and propose editing it instead.
2. **Read references.** `.claude/skills/add-rule/SKILL.md` (a skill with a write gate) and `.claude/skills/quality-gates/SKILL.md` (a reference skill).
3. **Gather requirements** (input step): from `$ARGUMENTS` and the context first; ask only for what is missing.
   - What does it do, and when should it be used? (the `description`; required)
   - What arguments does it take? (the `argument-hint`)
   - Which of its steps are interactive, and of which class: input, write gate, advisory or escalation?
   - Does it dispatch agents? Name each by `subagent_type`. Never pass `effort` to a haiku agent.
4. **Draft** `.claude/skills/<name>/SKILL.md`:

   ```markdown
   ---
   name: <name>
   description: <what it does and when to use it>
   argument-hint: "<hint, if it takes arguments>"
   ---

   # <Title>

   <One paragraph: what it does and when.>

   ## Input

   <What `$ARGUMENTS` carries, or "None".>

   ## Invocation contexts

   - **Slash command**: <behaviour>
   - **Model-invoked**: <behaviour>
   - **Non-interactive**: <what each interactive step does when no one can reply>

   ## Steps

   1. **<Step>** (<class, if interactive>): <instruction>

   ## Output

   <What it returns or writes.>
   ```

5. **Check the body.** No unescaped dollar-zero: write `\$0` or `${0}` because skill text is argument-interpolated when loaded. Every referenced file exists now. Never set `disable-model-invocation`.
6. **Write** (write gate): show the draft and write on confirmation.
7. **Verify.** Run `scripts/ci/lint-agent-harness.sh`: `frontmatter`, `skill-contexts`, `dollar-zero`, `haiku-effort` and `references` must pass.

## Output

The path written (or the draft) and the lint result.
