# Evidence: spec-premise-grounding

Record for `.claude/rules/spec-premise-grounding.md`.

## Hazard

A description of the code written into an issue or a spec is treated as ground truth by everyone downstream. Nothing re-reads it against the source, so a wrong premise survives refinement, design, validation and implementation, and is found, if at all, when the shipped change does not fix the problem it was built for.

The common shapes:

- **The mechanism is misread.** The claim describes what a comment or a function name says, not what the code computes.
- **The citation drifted.** The cited lines moved or changed after the claim was written; the claim may still be true somewhere else, or no longer true at all.
- **The failure is unreachable.** The described bug sits behind dead code, a disabled path or an upstream defect, so no acceptance check can be made to fail today, and the spec's criteria pass whether or not the fix works.
- **An absence is asserted from a keyword search.** A `grep` with no hits proves that one spelling is missing from the lines it matched. The concept can be present under another name, or the hit can be dismissed as noise while it sits inside the very section being declared missing. `grep … || echo none` reports a false absence when the path is wrong.
- **A count is remembered, not derived.** "Two call sites" undercounts the third, and the design leaves it unhandled.

## Mechanism

Requiring a `file:line` or a command with its output for every codebase claim makes the claim re-checkable by the validator and the reviewer. Structural grounding of absences (enumerate the container, show the thing is not in it) closes the keyword-grep gap. The four verdicts (`HOLDS`, `PARTIAL`, `REFUTED`, `UNVERIFIABLE`) separate imprecision that is written down, which a spec can safely build on, from a false statement still standing, which it cannot.

This is prose-enforced: judging whether a claim holds needs reading and meaning, which no grep can do. `/create-spec` records the verdicts, `/refine-spec` and `/spec-validate` spot-check citations with fresh context, and `/evaluate-spec` re-grounds stale specs.

## Instances

None recorded in this repository yet.

## Loosening criteria

Narrow the rule (for example, to claims whose citations no longer resolve) only with three recorded specs where the full grounding found nothing the narrow form would have missed, each linked here.
