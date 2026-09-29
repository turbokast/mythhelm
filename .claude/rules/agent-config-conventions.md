---
paths:
  - ".claude/agents/**"
  - ".claude/skills/**"
  - ".claude/rules/**"
  - ".claude/settings.json"
  - "knowledge/agent-routing.md"
---

# Agent Configuration Conventions

`validate-agent-config.sh` checks each edit before it is written; `scripts/ci/lint-agent-harness.sh` checks the whole set. Run the lint before pushing any change under `.claude/` or `knowledge/`.

## Agents: `.claude/agents/<name>.md`

- Frontmatter: `name` (equals the file name), `description` (when to use it, one line), `model` (`opus`, `sonnet` or `haiku`, never a dated model ID), `effort` as `knowledge/agent-routing.md` requires for the tier (none for haiku), and `tools` as an allowlist when the agent is read-only or mechanical.
- Body sections, in order: `## Role`, `## Domain scope`, `## Before you begin` (the reading list), `## Workflow`, `## Gates`, `## Boundaries`, `## Escalation`. Implementers add `## Completion checklist` and `## Review threads`; reviewers add `## Output`.
- Add or change the agent's row in `knowledge/agent-routing.md` in the same change.
- Reference `knowledge/` instead of copying it. Duplicate a rule into an agent only when the agent must see it and the rule is path-conditional (subagents do not reliably receive path-conditional rules); mark the copy with the rule it mirrors.

## Skills: `.claude/skills/<name>/SKILL.md`

- Frontmatter: `name` (equals the directory), `description` (what it does and when to use it), optional `argument-hint`, `allowed-tools`, `model`. Every skill stays model-invocable: never set `disable-model-invocation`.
- Body: `## Input`, `## Invocation contexts` (the **Slash command**, **Model-invoked** and **Non-interactive** behaviours, per `skill-invocation-contexts.md`), `## Steps`, `## Output`. State each interactive step's class at the step.
- Never write a bare `$0` in a skill body: skill text is argument-interpolated when loaded, so `$0` becomes the arguments. Write `\$0` or `${0}`.
- Never pass `effort` when a skill dispatches a haiku agent.
- Reference only files that exist. Never point at a skill, script or knowledge file that a later change will add.

## Rules: `.claude/rules/<name>.md`

- One concern per rule, imperative, current policy only (`strict-by-default.md`). Evidence goes in `knowledge/rule-evidence/<name>.md`, with a one-line pointer in the rule.
- Default to path-conditional: `paths:` frontmatter listing repository-relative globs for the files whose editing needs the rule. A rule with no `paths:` loads in every session and counts against the always-on budget in `scripts/ci/lint-agent-harness.sh`; raise that budget only in the change that needs it, and say why in the pull request.
- Every `paths:` glob matches at least one path in the tree. A glob for a directory that does not exist yet is preceded by a comment line starting `# future` with the reason; delete the marker once the path exists.

## Hooks and settings

Hooks follow `.claude/hooks/README.md`. A new or changed hook updates `.claude/settings.json`, `.claude/hooks/INVENTORY.md` (event, matcher, posture) and its test in the same change; the lint checks that registrations and inventory rows agree.
