# Refinement log — budget-ledger-s1

dispatched=2 returned=2 failed=0 (fresh-context assessors, 2026-10-08).

## Round 1 — Needs work

D2/D3/D6/D7/D8 NEEDS_WORK, 15 auto-fixable, none human-required. Fixes applied:
- AC-2.1: concrete normalization rule (scope/unit/source match, newest cumulative baseline, (producer, sequence) identity, identity-less deltas estimated).
- AC-2.3: semantics established = per-route qualified evidence; unmapped components `unknown`.
- AC-4.1: ceiling resolution chain (flags > profile config > 30m/3/2/5 built-ins).
- AC-5.1 + OQ-3: reserve = one verification pass + FR-4 repair ceiling; no separate configured envelope; OQ-3 resolved.
- AC-6.2: ≤3 reset retries; unknown reset backs off 1m/5m/15m then stops.
- AC-6.4: S1 preauthorises no alternative route (future-admission constraint only); re-admission per I04.
- NFR-1: 1 s run-path bound, measured by the latency test.
- Grounding: 14 packages, real grep paths, refreshed hit summary; `specs/*/` recitations; MH-13 downstream line.
- Shown surfaces: receipts + CLI run output (AC-1.4/5.2/7.1); TUI unchanged; `internal/cli/` impacted.
- OQ-1: recommended (a), maintainer checkpoint decides. OQ-2: default (a) with fixtures (MH-12 T7 held per Q-15). OQ-4: answered from ADR-0005 (run-owner sole writer).

## Round 2 — Ready

All dimensions PASS; citations re-verified against the tree. No changes.

No human-required issues in any round. Ready for `/spec`; `/spec` consumes the OQ-1 recommendation at its maintainer checkpoint.
