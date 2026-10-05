---
name: mythhelm-bootstrap
description: Orient a coding-agent session in the MYTHHELM repository before task or spec work.
---

# Bootstrap

Use this when beginning task work or resuming a session in this repository. The input may be a spec name, path or task description.

1. Read `AGENTS.md`, `knowledge/README.md`, `knowledge/domains.md` and `knowledge/invariants.md`. Identify the domain and relevant invariants.
2. Locate the spec in `specs/*/<name>/`, or match the task to `specs/in-progress/` and `specs/todo/`. Read its `requirements.md`, `design.md`, `tasks.md` and `scratchpad.md`, then `docs/spec/README.md` and the cited sections of the named specification revision. New product work uses v2; historical section numbers remain pinned to their original revision.
3. Check which task is ready from its dependencies on `origin/main`. If `orchestration/INTENT.md` exists, run `scripts/orchestration/status.sh --no-fetch` and resume the existing delivery run instead of starting work beside it.
4. Report the domain, task, acceptance checks, relevant invariants and unresolved questions. Ask only when the repository does not answer a choice that changes the result.

Read the affected code next. For workflow commands, use the scripts and procedures in `WORKFLOW.md` and `docs/harness/agent-support.md`. Do not infer that a client-specific slash command or hook ran.
