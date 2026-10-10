# Refinement log — contained-execution-profiles

dispatched=3 returned=3 failed=0 (fresh-context assessors, 2026-10-08).

## Round 1 — Needs human input

D2/D3/D8 NEEDS_WORK plus 3 human-required questions. Fixes applied:
- Dropped the unreproducible ", 54 matches" count (evidence carries the verdict).
- Q2 investigation default: shared mechanism.
- AC-1.2: "naming each missing coverage" (aggregate).
- Maintainer decisions 2026-10-08: Q1 per-platform native (refuse where unavailable); Q2 shared mechanism; Q3 evaluator isolation here only (revision binding + receipts stay MH-22).

## Round 2 — Needs work

D2/D8 NEEDS_WORK, 4 auto-fixable, none human-required. Fixes applied (citations verified):
- AC-1.3: new configuration = no recorded profile choice (existing keep theirs per I04); trusted-host via `--execution-profile` flag or the admission.go:436-450 consent flow.
- AC-3.1: disclosure surfaces pinned (admission/CLI disclosure + receipt.go:98-99 label).
- AC-4.2: enumerated channels with one failing fixture each; residual named, not silently covered.
- Q4 closed as decided: aggregate all gaps.

## Round 3 — Needs work

D4/D7/D8 NEEDS_WORK, 3 auto-fixable, none human-required. Fixes applied post-assessment (three-round cap):
- N1 still-binds cell recast to this spec's invariant/gate (I07, G07/AT-47).
- Dependencies: MH-16 sequenced-alongside line (not a prerequisite); budget-ledger-s1:109 softened to match.
- AC-2.1: refuse all checks under `inspect` until MH-22 ships check scoping (no per-check scope in the admitted list today).

No FAIL in any round: Good enough. `/spec` should confirm the round-3 post-assessment wording.
