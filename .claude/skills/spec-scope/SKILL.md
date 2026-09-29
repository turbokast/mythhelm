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

1. **Estimate** the task count with `/spec-decomposition`'s sizing rules, and list the work streams. A work stream is **independent** when different agents could build it in parallel with the others and it delivers something usable shipped alone; independent subsystems in different domains (`knowledge/domains.md`) are independent work streams.
2. **Decide** by the first rule that applies:
   1. Two or more independent work streams: **epic**.
   2. More than 14 estimated tasks: **epic**, unless no part can ship alone (every task is needed before anything is usable, as in a first end-to-end slice). Then **single spec**, and `design.md` states why it was not split.
   3. Otherwise: **single spec**. Above 12 tasks, `design.md` states why it was not split.
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
