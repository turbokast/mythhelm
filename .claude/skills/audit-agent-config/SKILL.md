---
name: audit-agent-config
description: Read-only audit of the whole agent harness configuration — agents, skills, rules, hooks, settings, routing and knowledge — reported as errors, warnings and info
---

# Audit Agent Config

A readable report on the state of the harness configuration, built on the same checks CI runs plus the judgement checks no script can make. Use it after editing several `.claude/` files, before pushing harness changes, or periodically to catch drift.

## Input

None. `$ARGUMENTS` may name a subset (`agents`, `skills`, `rules`, `hooks`, `knowledge`) to restrict the judgement checks; the mechanical checks always run in full.

## Invocation contexts

- **Slash command**: runs every step and prints the report.
- **Model-invoked**: the same.
- **Non-interactive**: the same; the report is the final message. The skill never writes, so it has no gates.

## Steps

### Mechanical checks

1. Run `scripts/ci/lint-agent-harness.sh` and `.claude/hooks/tests/run-tests.sh`. Every failing lint check or test is an **Error**; quote its finding lines.

### Inventory

Zero-judgment counts. Run them yourself, or dispatch the clerk with no model or effort argument, so its frontmatter pin applies:

```text
Agent(subagent_type: "harness-clerk", description: "Harness inventory",
      prompt: "Run .claude/skills/audit-agent-config/SKILL.md §Inventory steps 2-4 against the working tree and return its report format.")
```

2. Count the files in `.claude/agents/`, `.claude/skills/*/SKILL.md`, `.claude/rules/` (split into always-on and path-conditional) and `.claude/hooks/*.sh`.
3. Report the always-on byte total from the `rule-budget` line of the lint output.
4. List agents present in `.claude/agents/` and rows in `knowledge/agent-routing.md`, side by side.

Report format: one line per step, `N/M` steps run.

### Judgement checks

5. **Contradictions.** Read the always-on rules and the rules loaded for each domain in `knowledge/domains.md`. Two rules giving different instructions for the same situation is an **Error**; name both lines.
6. **Duplication.** The same instruction in two rules, or a rule restated in an agent without a mirror note, is a **Warning**: one copy will drift.
7. **Hedging and history.** "Consider", "optionally", "where practical", dates, ticket IDs or "previously" in a rule is a **Warning** (`.claude/rules/strict-by-default.md`).
8. **Stale facts.** A path, command or agent name in `knowledge/` or an agent that no longer matches the tree is a **Warning** (the lint catches file paths; this catches commands and descriptions).
9. **Scope.** An agent whose domain scope disagrees with `knowledge/domains.md` is an **Error**.
10. **Enforcement.** A rule that claims a check enforces it must name a check that exists and tests that behaviour; otherwise **Warning**.

Never edit anything: this skill reports.

## Output

```markdown
## Agent config audit

### Errors (N)
- [lint:routing-pins] .claude/agents/x.md: ...
### Warnings (N)
- [duplication] .claude/rules/a.md:12 and .claude/agents/b.md:40 state ...
### Info
- 8 agents, 8 skills, 15 rules (6 always-on, 9 path-conditional), 4 hooks
- always-on bytes: N / budget
### Result
N errors, N warnings: PASS | NEEDS ATTENTION
```
