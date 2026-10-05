# Agent instructions

MYTHHELM is an open-source command deck for native coding agents. Its planned stack is a Go core, Bubble Tea TUI, local SQLite state and a versioned plugin protocol. It is pre-alpha; read the [specification index](docs/spec/README.md) and relevant sections of the current master before implementing product behavior.

This file is the common entrypoint for every coding agent. [`WORKFLOW.md`](WORKFLOW.md) defines the development lifecycle and takes precedence over [`docs/harness/charter.md`](docs/harness/charter.md). Client-specific files such as [`CLAUDE.md`](CLAUDE.md) add only client integration details. No agent vendor is the default owner of work.

## Start work

- Read [`knowledge/README.md`](knowledge/README.md), [`knowledge/domains.md`](knowledge/domains.md) and [`knowledge/invariants.md`](knowledge/invariants.md). Locate the active spec and its next ready task. The portable `mythhelm-bootstrap` skill under `.agents/skills/` gives the short procedure; clients without skill discovery can read it directly.
- Use the spec lifecycle in [`WORKFLOW.md`](WORKFLOW.md). The portable `mythhelm-deliver-backlog` skill under `.agents/skills/` coordinates a backlog run. Stage procedures currently in `.claude/skills/` are repository documents: read the relevant `SKILL.md` and translate tool-specific calls to the tools your client actually has. Never assume a named Claude subagent, model pin, Stop hook or slash command exists in another client.
- Route by domain and task, using [`knowledge/domains.md`](knowledge/domains.md). The model pins in [`knowledge/agent-routing.md`](knowledge/agent-routing.md) apply only to Claude agent definitions; other clients choose a capable agent or work directly and record who did the work.
- Inspect [`docs/harness/agent-support.md`](docs/harness/agent-support.md) for client discovery, workflow invocation and unattended-delivery limits.

## Repository rules

- Land changes on `main` through a pull request after required CI checks pass. Never push directly to `main`.
- Sign every commit with `git commit -s`. Commit named paths only; never use `git add -A`, `git add .`, `git commit -a` or `git stash` in a shared checkout. Use an isolated worktree for task work.
- Releases, version tags, workflow runs, secrets, variables and repository settings are the operator's actions unless explicitly requested. Public-repo hygiene applies to every file and PR description; see [`.claude/rules/public-repo-hygiene.md`](.claude/rules/public-repo-hygiene.md).
- Product changes need a maintainer's signed approval through [`scripts/orchestration/approve.sh`](scripts/orchestration/approve.sh). Do not edit `product/` or `orchestration/` state directly; use the repository scripts.
- Format first (`gofmt -w` on changed Go files; `go mod tidy` when needed), then run [`scripts/harness/gate.sh`](scripts/harness/gate.sh) with `go`, `harness` or `all`. [`scripts/harness/gatelib.py`](scripts/harness/gatelib.py) reports whether gate markers still match the tree.
- Never treat a client hook as a cross-client safety boundary. Run the shared checks explicitly. Unattended delivery requires the exact client session and enforcement conditions in [`knowledge/autonomy.md`](knowledge/autonomy.md); otherwise use interactive checkpoints and do not self-grant.
