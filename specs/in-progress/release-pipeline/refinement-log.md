# Refinement log: release-pipeline

dispatched=3 returned=3 failed=0
Verdict: Good enough after 3 rounds + maintainer answers (no FAIL; round-3 NEEDS_WORK fixed post-round, Q-12/Q-13 answered — unblocks the move)

## Round 1 — Needs work (D2, D5, D6, D8 NEEDS_WORK)

Fixes applied to `requirements.md` (ADR-0010 verified: tags created only by
release publication, never pushed by hand; GoReleaser consumes the tag on
tag-push):

- O1/AC-1.1: operator publishes a release, which creates the tag; workflow triggers on the tag.
- NFR-1 measurable (`timeout-minutes`, default 60); NFR-2 "tag push" → "release publication".
- Tags: NFR-2 `[G01, I13]`, AC-3.3 `[G10, I14]`.

## Round 2 — Needs work (D2, D8 NEEDS_WORK)

Fixes applied:

- AC-3.3: names `docs/limitations.md` and the release-notes link location.
- AC-1.3: folded the Q3 `test/*` default (later restated per round 3).
- Filed delivery question Q-12 (Q1 release matrix, Q2 release environment).

## Round 3 — Needs human input (D2, D8 NEEDS_WORK)

Fixes applied (post-round, unassessed):

- AC-1.1: explicit default pair list (CI runner matrix, darwin/amd64 excluded per N4), marked pending Q1 confirmation.
- AC-1.3: inlined the `test/*` pattern, marked pending Q3 authorization.
- Q4 reclassified as a design decision (pinned GoReleaser, SPDX); /spec records the pin.

Human-required, filed as Q-12 (Q1/Q2) and Q-13 (Q3); all answered 2026-10-07:

- Q1: CI runner matrix (linux/amd64+arm64, darwin/arm64, windows/amd64+arm64; no darwin/amd64) — confirmed.
- Q2: environment did not exist; operator created `release` with required reviewer (self) + `v*` tag policy, verified via API.
- Q3: operator-owned `test/*`, excluded from real-release matching — confirmed.

## Remaining notes for /spec

- Unblocks when the maintainer answers Q-12/Q-13 or confirms the defaults.
- Re-check `todo/`/`in-progress/` for conflicts at /spec time (empty at refine time).
