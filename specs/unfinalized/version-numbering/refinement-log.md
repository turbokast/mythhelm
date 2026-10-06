# Refinement log: version-numbering (MH-18)

Refined 2026-10-06 in the deliver-backlog run 2026-10-05T14:45:12Z (Muse Code session, no Claude subagents; fresh-context assessors via native child agents with the refine-spec prompt).

dispatched=3 returned=3 failed=0

## Round 1 — Verdict: Needs work

Scores: D1 PASS, D2 PASS, D3 NEEDS_WORK, D4 PASS, D5 PASS, D6 PASS, D7 NEEDS_WORK, D8 PASS.

Auto-fixable (all applied, intent-preserving — stale create-spec-time claims re-grounded to 2026-10-06):
1. D3 — L20 "No spec covers MH-18" stale: the card is now specced by this spec (`product/backlog.md:89-94`), and `MH-18` in `specs/` now also includes the release-tagging cross-references. Re-grounded with the create-spec-time note preserved.
2. D7 — L80 called MH-19 "triaged, unspecced"; it is specced as `specs/*/release-tagging/` (`product/backlog.md:100-105`). Fixed, citing the spec path per the cite rule.
3. D7 — L81 conflicts line checked only `todo/`/`in-progress/`; added the unrefined-sibling check (release-tagging builds on this decision, consistent; strict-lint-set disjoint).

Human-required: none. Missing context: none.

## Round 2 — Verdict: Needs work

Scores: D3 NEEDS_WORK, all others PASS. Two citation nits round 1 missed (both applied):
1. D3 — L21 `mythhelm version` unstamped-`devel` claim uncited; appended `internal/buildinfo/buildinfo.go:43` (verified: default `Version = "devel"` when unstamped).
2. D3 — L18 cited `MYTHHELM_Implementation_Plan.md:213-221` without the directory prefix; fixed to `mythhelm-synthesis/MYTHHELM_Implementation_Plan.md:213-221` (verified: W16 operator-action rule).

Human-required: none. Missing context: none.

## Round 3 — Verdict: Ready

Scores: all eight dimensions PASS. Auto-fixable: none. Human-required: none. Missing context: none. The assessor independently re-opened citations (CHANGELOG.md:6, buildinfo.go:43 + dispatch.go:153-168 version wiring, openssf-badge design.md:29, backlog MH-7/MH-18 rows, W16, zero tags, no release workflow, empty todo//in-progress/, both unrefined siblings).

## Remaining notes for /spec

- Open Q1–Q4 each carry a default and a named decider (/spec weighs and recommends; the maintainer's spec checkpoint confirms): Q1 default (a) ratify SemVer; Q2 default (c) ADR + release-process record; Q3 default (a) releases only; Q4 default (a) MH-19 owns format/mechanics constrained by this scheme.
- MH-19 (`specs/*/release-tagging/`) builds on this spec's scheme decision; its delivery dependency is recorded.
