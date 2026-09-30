# Coding-agent support

MYTHHELM accepts work from any agent that can read repository files, run shell commands and use git. [`AGENTS.md`](../../AGENTS.md) is the common instruction entrypoint; [`WORKFLOW.md`](../../WORKFLOW.md) and the scripts under `scripts/` define the workflow. A client-specific command or hook is an adapter, not the source of policy.

| Client | Project instructions | Project skills | Invocation |
|---|---|---|---|
| Claude Code | `CLAUDE.md` points to `AGENTS.md` | `.claude/skills/` | `/bootstrap`, `/deliver-backlog` |
| Codex | `AGENTS.md` | `.agents/skills/` | `$mythhelm-bootstrap`, `$mythhelm-deliver-backlog` (or ask for them by name) |
| Meta Muse Code | `AGENTS.md` | `.agents/skills/` | Ask for a skill by name or load it with the client's skill command |
| Grok Build | `AGENTS.md` | `.grok/skills/` adapters to `.agents/skills/` | `/mythhelm-bootstrap`, `/mythhelm-deliver-backlog` |
| Kimi Code CLI | `AGENTS.md` | `.agents/skills/` | `/skill:mythhelm-bootstrap`, `/skill:mythhelm-deliver-backlog` |
| OpenCode | `AGENTS.md` | `.agents/skills/` | Ask for a skill by name; the skill tool loads it |
| Other agents | Read `AGENTS.md` explicitly | Read `.agents/skills/<name>/SKILL.md` explicitly | Follow the same script and PR contracts |

Client discovery can vary by version and workspace trust. If a skill is not listed, give the agent the file path. The shared skills use namespaced names so clients that also scan `.claude/skills/` do not see two definitions with the same name.

## Workflow boundary

`mythhelm-bootstrap` and `mythhelm-deliver-backlog` are portable entrypoints. They direct the client to the current spec, scripts and detailed stage documents. The stage documents in `.claude/skills/` include Claude syntax such as `Agent(...)` and `/run-spec`; on another client, load them as procedure documents and use its native tools. A task's `Domain/agent` names a responsibility, not a requirement to launch that particular Claude model. Use a capable native worker in an isolated worktree, or perform the task sequentially in an isolated worktree. Preserve the one-task, one-PR and independent-verification contracts. Never claim that a Claude command or hook ran when it did not.

Shared checks are `scripts/ci/lint-agent-harness.sh`, `scripts/ci/check-public-hygiene.sh`, `scripts/harness/gate.sh`, `scripts/harness/runspec.py`, `scripts/harness/finalize.py` and `scripts/orchestration/delivery.py`. They can be invoked from any shell-capable client. Gate markers and CI are the portable evidence; Claude Stop hooks only add immediate feedback.

## Unattended delivery

Claude Code currently has the session-bound Stop and guard hooks and optional heartbeat described in [`knowledge/autonomy.md`](../../knowledge/autonomy.md). `autonomy.py` also recognizes `CODEX_SESSION_ID` and `CODEX_THREAD_ID`, so an operator can bind a scoped grant to a Codex session from their terminal. Codex still needs operator continuation and checkpoints: its tool calls have no MYTHHELM grant guard. Other clients can do interactive delivery and resume from `orchestration/` state. No client without a tested guard adapter may use a grant for unattended merging or product pre-approval. A grant file or environment variable alone does not supply that enforcement.

## Adding an agent client

Keep policies and stage contracts in the shared files. Add only discovery or hook adapters for a client, test the behavior it claims, and document any client-specific limits here. Do not duplicate the entire lifecycle into a new vendor directory.
