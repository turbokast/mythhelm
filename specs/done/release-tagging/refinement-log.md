# Refinement log: release-tagging (MH-19)

Refined 2026-10-06 in the deliver-backlog run 2026-10-05T14:45:12Z (Muse Code session, no Claude subagents; fresh-context assessors via native child agents with the refine-spec prompt).

dispatched=3 returned=3 failed=0

## Round 1 — Verdict: Needs work

Scores: D1 PASS, D2 PASS, D3 NEEDS_WORK, D4 PASS, D5 PASS, D6 PASS, D7 NEEDS_WORK, D8 PASS.

Auto-fixable (all six applied, intent-preserving):
1. D3 — L18 `.github/workflows/` count wrong (13 vs 14; wrong when written). Fixed to 14 with the `ls` enumeration.
2. D3 — L17 W16 quote cited `:220` (blank line) without the directory prefix. Fixed to `mythhelm-synthesis/MYTHHELM_Implementation_Plan.md:219` (verified).
3. D7 — L82 called MH-18 "unrefined"; version-numbering is now refined. Fixed to `specs/*/version-numbering/` (refined).
4. D7 — L81 cited `specs/done/openssf-badge/` by lifecycle path. Fixed to `specs/*/openssf-badge/` per the cite rule.
5. D7 — L84 conflicts line omitted siblings. Extended with the checked-sibling sweep (tui-tape-recording and strict-lint-set disjoint, no MH-19 mention; version-numbering is the builds-on parent).
6. D5 — L5 header declared §/I/G/AT → v2 but not the W-ID source. Added "W-IDs refer to the waves in mythhelm-synthesis/MYTHHELM_Implementation_Plan.md".

Plus one self-spotted carry-over (same staleness shape as version-numbering round 1): L19 "No spec covers MH-19" was stale — the card is specced by this spec (`product/backlog.md:100-105`). Re-grounded with the create-spec-time note preserved.

Human-required: none. Missing context: none.

## Round 2 — Verdict: Needs work

Scores: D2 NEEDS_WORK, D8 NEEDS_WORK, all others PASS. One issue (applied):
1. D2/D8 — AC-1.2 (L44) "implementable ... without re-deciding (MH-7 cites it)" had no failing counterfactual within this spec: MH-7 is triaged but unspecced, so nothing this spec writes can pass or fail it. Reworded as a now-evaluable prose/record assertion (the record states the trigger using only the terms already fixed in MH-7's card Summary — tag-triggered, GoReleaser — per N2), and moved "MH-7 cites it" to a forward-trace note on the L83 feeds-into-MH-7 dependency as MH-7's acceptance, not this spec's check.

Human-required: none. Missing context: none.

## Round 3 — Verdict: Ready

Scores: all eight dimensions PASS. Auto-fixable: none. Human-required: none. Missing context: none. The assessor re-opened five citations (openssf-badge design.md:30, backlog MH-19/MH-7 rows, Implementation Plan :219, zero tags / 14 workflows / empty todo+in-progress by command) and re-verified sibling disjointness.

## Remaining notes for /spec

- Open Q1 (annotated vs signed vs lightweight; no default — /spec weighs GoReleaser/provenance practice and the MH-7 attestation plan), Q2 (default b: format follows the MH-18 scheme), Q3 (default a: every published release tagged, explicit pre-release marker rule), Q4 (default a: design-ahead as MH-7's fixed input).
- Record home (one document shared with MH-18's scheme record, or two) is explicitly deferred to /spec in Impacted components.
- Builds on MH-18 (`specs/*/version-numbering/`): specifying runs ahead, building waits for the scheme decision (delivery dependency recorded).
