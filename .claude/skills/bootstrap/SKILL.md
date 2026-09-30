---
name: bootstrap
description: Session-start orientation — read CLAUDE.md, the knowledge index, the domain map, the invariants and the active spec before starting task work
argument-hint: "[spec-name | path | task description]"
---

# Bootstrap

A short orientation before task work, so the session starts from the project's actual conventions, invariants and plan rather than from assumptions.

## Input

`$ARGUMENTS`: optional. A spec name, a path, or a description of the task.

## Invocation contexts

- **Slash command**: runs the steps and states readiness.
- **Model-invoked**: the same, at the start of a task.
- **Non-interactive**: the same; when the task is ambiguous (step 5), returns the questions instead of proceeding.

## Steps

1. **Project and conventions.** Read `CLAUDE.md` if it is not already in context, then `knowledge/README.md`.
2. **Domains.** Read `knowledge/domains.md` and map the task's paths to domains and owning agents.
3. **Invariants.** Read `knowledge/invariants.md`. Note which invariants the task can reach.
4. **Active spec.** Find the spec:
   - `$ARGUMENTS` names one: look for `specs/*/<name>/`.
   - Otherwise list `specs/in-progress/` and `specs/todo/`, and match the task description to a spec directory.
   - When found, read `requirements.md`, `design.md`, `tasks.md` and `scratchpad.md`, and identify the next task whose heading lacks `✅ COMPLETED` and whose dependencies are complete.
   - Read the master-spec sections (`docs/spec/master-spec.md`) that the task cites.
   - When `orchestration/INTENT.md` exists, run `scripts/orchestration/status.sh --no-fetch`: a delivery run in progress, and any autonomy grant this session holds, decide what to work on. Resume the run with `/deliver-backlog` rather than starting work beside it.
5. **Confirm readiness** (escalation step when unclear). State in a few lines: the domains and agent, the invariants in play, the spec task and its acceptance checks, and any open questions from `scratchpad.md` that affect it. If the task is ambiguous after these reads, ask; in a non-interactive run, return the questions and stop.

Read nothing speculatively: a single-domain task needs only these files and the code it touches.

## Output

A readiness summary: domain and agent, spec task, invariants, acceptance checks, open questions.
