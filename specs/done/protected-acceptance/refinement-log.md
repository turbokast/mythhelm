# Refinement log: protected-acceptance

dispatched=3 returned=3 failed=0
Verdict: Good enough after 3 rounds (no FAIL; D5/D7/D8 NEEDS_WORK fixed or logged below)

## Round 1 — Needs work (D2, D6, D8 NEEDS_WORK)

Fixes applied to `requirements.md`:

- NFR-1 made measurable (per-step timeouts + recorded overhead); dropped the false 5-minute citation (that figure is MH-20's lint-CI budget per `orchestration/RUN-LOG.md`, not this area's).
- P1 citations §12 → §10 (definition at v2 §10, line 507).
- AC-3.1 scoped: declared-mode runs continue alongside P1, so branch 1 has live triggers; AC-4.3 governs P1 admission only.
- Added AC-4.4 (bounds recorded as data, Q4's default made testable); DoD range extended to AC-4.4.

## Round 2 — Needs work (D2, D8 NEEDS_WORK)

Fixes applied:

- AC-4.1 names the `determinism` receipt note as the disclosure artifact.
- NFR-1 scope clarified (per run, measured and recorded).

## Round 3 — Needs work (D5, D7, D8 NEEDS_WORK)

Fixes applied (post-round, unassessed):

- FR-2/AC-2.1/Context retagged I22 → I07 (I22 is learning-scoped); G03 dropped from FR-2 (nothing serves source protection).
- Prerequisites annotated with spec status (MH-10/MH-12 refined; MH-21/MH-13/MH-16-S1 card numbers only).

## Remaining notes for /spec

- /spec waits for MH-21 (Q1's adopt-verbatim default has no contract to adopt until MH-21 is specified); filed as Q-14 with default "design waits, no interim shape". Q2/Q3 defaults are actionable once MH-10/MH-12 ship.
- Re-check `todo/`/`in-progress/` for conflicts at /spec time (empty at refine time).
- Issues #72/#74/#75 must be re-read at implementation time (DoD item 2).
