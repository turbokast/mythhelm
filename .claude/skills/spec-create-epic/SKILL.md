---
name: spec-create-epic
description: Write an epic's master plan (plan.md with a machine-read Work Streams table) and one full spec per work stream, each linked back to the plan
argument-hint: "<epic-name | \"epic description\">"
---

# Spec Create Epic

Phase 3b of `/spec`, when `/spec-scope` decided the work needs several specs. It writes the master plan first, confirms the decomposition, then writes each sub-spec with `/spec-create-single`.

## Input

`$ARGUMENTS`: the epic's name or description, with the phase-1 findings and the phase-2 work streams in context.

## Invocation contexts

- **Slash command**: presents the plan (step 2) and writes the sub-specs after "go" or an adjusted decomposition.
- **Model-invoked**: the same.
- **Non-interactive**: the plan confirmation is advisory. Adjustments in the dispatching prompt are applied; otherwise the drafted plan proceeds, and `epic plan auto-confirmed (non-interactive)` is recorded in the plan's Open Questions.

## Steps

1. **Write the plan** at `specs/unrefined/<epic-name>/plan.md`. The directory holds only `plan.md`:

   ```markdown
   ## <Epic Title> — Master Plan

   ### Problem Summary
   <what exists, what is missing, the end state>

   ### Current State
   <investigation findings, cited as file:line>

   ### Work Streams

   | # | Spec | Scope | Domain | Dependencies |
   |---|---|---|---|---|
   | 1 | `<sub-spec-1>` | <one line> | <domain> | None |
   | 2 | `<sub-spec-2>` | <one line> | <domain> | Spec 1 (<the contract it consumes>) |

   ### Dependency Graph
   <which spec blocks which; what can run in parallel>

   ### Implementation Order
   <phases of specs, each with why>

   ### Open Questions
   - <question> — affects `<sub-spec>`; default: <…>; decides: <who>

   ### Risks
   - <risk> — mitigation: <…>
   ```

   The Work Streams table is machine-read: column 2 holds each sub-spec's exact directory name in backticks, and `scripts/ci/lint-agent-harness.sh` (check `specs`) fails when a name has no directory. Every sub-spec depends only on earlier rows, and each one ships something usable on its own.
2. **Present the plan** (advisory step) and revise it on feedback before writing any sub-spec.
3. **Write each sub-spec** in `specs/unrefined/<sub-spec>/` by running `/spec-create-single` in mode C for its work stream, plus:
   - `design.md` ends with Cross-Spec References: the plan (`specs/*/<epic-name>/plan.md`), the specs it depends on, and the specs that depend on it;
   - `tasks.md`'s Dependencies section names the prerequisite specs and what each provides, and the epic it belongs to;
   - its own `scratchpad.md`.
   Sub-specs with no dependency between them may be written by parallel agents.
4. **Check each sub-spec** before the next: its task count fits `/spec-scope`'s single-spec bound; every path is real or a new file in an existing directory; interfaces other specs consume have exact signatures; it bundles no unrelated concern; its cross-references are in place.
5. **Exit gate**: `scripts/ci/lint-agent-harness.sh --only specs` passes.

## Lifecycle of the plan

The plan directory stays in `unrefined/` (or `refined/`) while its sub-specs are validated and moved to `todo/` one by one. It moves to `in-progress/` when the first sub-spec starts and to `done/` when the last one ships; it never enters `todo/` or `unfinalized/` (`specs/README.md`).

## Output

The plan path, one line per sub-spec (name, task count, scope, dependencies), and the lint result.
