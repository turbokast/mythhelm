# Refinement log: claude-strict-subscription

dispatched=2 returned=2 failed=0
Verdict: Ready after 2 rounds (round 2 all PASS; two nits fixed directly, no 3rd round needed)

## Round 1 — Needs work (D1, D3, D8 NEEDS_WORK)

Fixes applied to `requirements.md`:

- O1 "live-proven" → MH-10 vocabulary (`live-qualified` record, entitlement `proven`).
- Impacted components: dropped the false `internal/admission/qualify.go` / `ResolveQualification` citation; consume MH-10's `internal/qualify/` once it ships.
- AC-3.1 key match: full v2 §7.1 qualification key, parenthetical non-exhaustive.

Human-required (Q1 source gap, Q2 live-test grant) already captured as Open Questions with conservative defaults blocking implementation, not design.

## Round 2 — Ready (all PASS)

Fixes applied (post-round nits, no re-assessment):

- Line-20 citation `specs/refined/qualification-registry/` → `specs/*/qualification-registry/` per the authoring rule.
- AC-4.2 "every surface" enumerated: admission JSONL output, journal `runs.billing_posture` rows (`internal/journal/projections.go:39`), receipts (`internal/supervisor/receipt.go:173`), `runs` command output (`internal/cli/runs.go:121`).

## Remaining notes for /spec

- Re-ground the MH-10 registry citations at design time (MH-10 still in validation when this refined); design against its specified contract (Lookup/Record/InvalidateOnDrift/ResolveQualification).
- Q1–Q4 carry defaults that let design proceed; Q1/Q2 answers arrive as maintainer decisions before implementation.
- `specs/todo/` and `specs/in-progress/` were empty at refine time; re-check at /spec time.
