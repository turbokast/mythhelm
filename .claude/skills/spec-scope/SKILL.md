---
name: spec-scope
description: Decide whether investigated work fits one spec or needs an epic of several, present the determination with its reasons, and take a confirmation or override
argument-hint: "<spec-name | \"feature description\">"
---

# Spec Scope

Phase 2 of `/spec`. After the investigation, decides single spec or epic. Both failure directions are costly: an oversized spec stalls mid-implementation and is hard to review; a needless epic adds coordination and cross-spec drift.

## Input

`$ARGUMENTS`: the spec name or description, with the phase-1 investigation findings in context.

## Invocation contexts

- **Slash command**: presents the determination and waits for "go", "make this an epic" or "make this one spec".
- **Model-invoked**: the same.
- **Non-interactive**: advisory step. A directive in `$ARGUMENTS` or the dispatching prompt ("single spec", "epic") wins; otherwise the computed determination proceeds, recorded as `scope auto-confirmed (non-interactive)`.

## Steps

1. **Estimate** the task count with `/spec-decomposition`'s sizing rules, and list the work streams: groups of tasks that could be built by different agents at the same time, and could ship on their own.
2. **Decide**, in this order:
   - **Epic** when any holds: more than 12 estimated tasks; two or more independent work streams; work across domains (`knowledge/domains.md`) whose parts are independent subsystems; parts that deliver value shipped alone.
   - **Single spec** otherwise: one work stream, tightly coupled changes, 12 or fewer tasks.
   - **Vertical-slice exception.** Work above 12 tasks stays one spec when no part can ship alone (every task is needed before anything is usable, as in a first end-to-end slice). `design.md` then states why it was not split.
   - **Ambiguous** (10 to 14 tasks, partly independent streams): split when two streams could be implemented in parallel by different agents; otherwise stay single. Give the reasoning.
3. **Present** the determination (advisory step):

   ```text
   Single spec: ~<N> tasks, one work stream (<area>), domains: <list>.
   Why not an epic: <the coupling that stops the parts shipping alone>.
   Proceeding in specs/unrefined/<name>/ (or refined/ for mode A). Override: "make this an epic".
   ```

   ```text
   Epic of <N> specs: ~<N> tasks across <M> work streams.
   - <stream 1>: <scope> (~<N> tasks, <domain>)
   - <stream 2>: <scope> (~<N> tasks, <domain>) — depends on <stream 1> for <contract>
   Proceeding with a plan in specs/unrefined/<epic-name>/. Override: "make this one spec".
   ```

4. **Apply** the answer: an override switches to the other creation skill without re-investigating; an adjusted decomposition is carried into the plan.

## Output

`scope: single | epic`, the estimated task count, the work streams, the reason, and either `confirmed`, `overridden: <what>` or `auto-confirmed (non-interactive)`.
