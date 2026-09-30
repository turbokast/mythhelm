# Claude Code integration

Read [`AGENTS.md`](AGENTS.md) for project instructions and [`WORKFLOW.md`](WORKFLOW.md) for the lifecycle. These are shared by all coding agents; Claude Code has no priority over another client.

Claude Code discovers commands in `.claude/skills/`, agent definitions in `.claude/agents/` and guards in `.claude/hooks/` ([inventory](.claude/hooks/INVENTORY.md)). Use `/bootstrap` at the start of task work. `/deliver-backlog` is the Claude adapter for the shared delivery procedure in `.agents/skills/mythhelm-deliver-backlog/SKILL.md`.

The hook guards bind Claude Code tool calls only. If a hook blocks, follow its Fix line. The Claude model and effort pins in [`knowledge/agent-routing.md`](knowledge/agent-routing.md) apply when dispatching Claude subagents; they do not constrain another client's model choices.
