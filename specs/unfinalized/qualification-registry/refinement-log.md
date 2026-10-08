# Refinement log: qualification-registry

dispatched=3 returned=3 failed=0
Verdict: Good enough after 3 rounds (no FAIL in any round; round-3 NEEDS_WORK fixed directly, without a 4th assessment round)

## Round 1 — Needs work (D2, D3, D6, D8 NEEDS_WORK)

Fixes applied to `requirements.md`:

- N1/MH-12 boundary made explicit: MH-12 owns the live strict admission-pass; FR-3 is the authorised-test basis MH-12 builds on.
- AC-3.1 restated in terms of authorised-test evidence with bootstrap order evidence → record → admission, plus fixtures-only vs live-permission branches.
- AC-4.1 "relevant configuration" enumerated (startup hooks, MCP, plugins, managed config, pinned digests).
- AC-1.1 aligned to the full v2 §7.1 key.
- Context wording: "no live observation" → "no live qualification evidence".

Human-required carried forward as Q1 (first route) and Q2 (live-test permission); filed as delivery question Q-11 with defaults (Claude Code; fixtures only).

## Round 2 — Needs work (D3 NEEDS_WORK)

Fix applied: absence claim narrowed — enumerated the near-miss grep hits (`adapter.go:351,357` descriptor strings, `billing.go:56` block code, `Qualified` default-false flags, `palette.go:35` TUI palette) with the grounding command recorded in the verdict line. Verified each hit in the tree before editing.

## Round 3 — Needs work (D8 NEEDS_WORK)

Fixes applied (post-round, no 4th round per the three-round cap):

- AC-1.2 now values each column `proven | not-proven | unknown`; AC-3.1/AC-3.3 use those terms instead of undefined "proven"/"admittable".
- AC-3.1 live ceiling stated: up to but not including a live strict admission pass (N1, MH-12 owns it).
- AC-5.3 names the `doctor`-section default subject to Q4.

## Assessment notes

- Issue #31 body holds no demands beyond the MH-10 card's v2 §§4, 7–8, 14, 17–18 (read in full at create-spec time); the line-23 PARTIAL verdict on its stale section numbers stands.
- All three assessors confirmed Q3 (SQLite per ADR-0003) and Q4 (`doctor` section) need no human input.
- Round-3 D3: assessor independently re-ran the absence-claim grep and confirmed the same categories.

## Remaining notes for /spec

- Q-11 (Q1/Q2) awaits the maintainer; design proceeds on the defaults (Claude Code first route; fixtures only, admission still blocks per AC-3.2).
- The round-3 fixes above were not re-assessed; /spec-validate covers them with the rest of the spec.
- `specs/todo/` and `specs/in-progress/` were empty at refine time; re-check for conflicts at /spec time.
