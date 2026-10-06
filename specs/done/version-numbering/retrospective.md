# version-numbering — Retrospective

## Review Summary

- **Range**: 1018b76..f2d5451 (PRs #161; fix PRs none)
- **Reviewer**: code-reviewer
- **Findings**: critical 0, important 0, suggestion 1 (confirmed); rejected 0
- **Open critical**: 0
- **Vendor review**: skipped (codex/muse/jev all unavailable — disabled)

| Severity | Finding | Anchor | Disposition |
|---|---|---|---|
| suggestion | ADR Context uses present tense ("no release process record exists yet") while the same commit creates the record | `docs/decisions/0009-version-numbering-scheme.md:8` | no action: decision-time context is conventional for ADRs; must not be "fixed" |

Foreign changes in range: none.

## Acceptance

| Criterion | Result | Evidence |
|---|---|---|
| AC-1.1 scheme + rationale | met | ADR 0009 Decision: SemVer 2.0.0, module-path fit, release-drafter machinery, 0.x/S1 framing (PR #161) |
| AC-1.2 ratify path | met | CHANGELOG untouched in range; ADR cites CHANGELOG.md:6 twice |
| AC-1.3 scope | met | Releases-only scope, ADR:22 + Scheme section; devel/unknown untouched |
| AC-2.1 scheme in MH-7 location | met | docs/release-process.md Scheme section with v0.1.0/v1.2.3 |
| AC-2.2 MH-19/MH-7 ownership | met | Both files name MH-19 (format) and MH-7 (pipeline) |
| AC-3.1 justification text | met | Design §4: ADR 0009, release-process.md, FIRST-RELEASE-TAG placeholder, flip condition |
| AC-3.2 badge untouched | met | Range holds 4 Markdown files only; no assessment edit |
| NFR-1 prose, one PR | met | PR #161: one commit, prose only, CI green |

## Deviations

- Task 1: None — recorded in the task entry; review confirmed D1–D6 landed as designed.

## CI history

- CI on PR #161 @07c1daf: infra, run cancelled — superseded by push aa2fe00 (same-minute re-push; the full workflow set completed successfully on aa2fe00).
- OSV-Scanner on PR #161 @07c1daf: infra, run cancelled — same superseding push; success on aa2fe00.
- No failure+success on the same headSha: no nondeterministic failure. No fail/retry rows in the run-events log. `finalize.py verify` emitted no `note=history:` lines.

## Effort

dispatched=1 returned=1 failed=0; attempts 1 over 1 tasks; first-pass 1/1; review rounds 0; wall-clock 2026-10-06T11:29:49Z → 2026-10-06T12:00:39Z

| Task | Agent | Attempts | Review rounds | PR | Merged | First pass |
|---|---|---|---|---|---|---|
| 1 | go-implementer | 1 | 0 | #161 | yes | yes |

## Lessons

- What worked: anchored failing-if-removed acceptance greps in a single-task docs spec — the task merged first-pass with 0 review rounds (PR #161).
- What worked: the MH-18/MH-19 delivery dependency (scheme first, format builds on it) held end to end — release-tagging cited ADR 0009 as fixed input with the Scheme section byte-intact.
- What to change: nothing evidenced by this spec — no defect passed a gate, no improvised step, no repeated failure class.

## Proposals

- None

Acceptance (spec-wide, adjudicated at head f2d5451): AC-1.1 met (ADR names SemVer 2.0.0 with module-path, release-drafter, 0.x/S1 rationale); AC-1.2 met (ratify path: CHANGELOG untouched in range, ADR cites CHANGELOG.md:6 twice); AC-1.3 met (releases-only scope, ADR:22 + Scheme); AC-2.1 met (Scheme section with v0.1.0/v1.2.3, declares itself MH-7's record); AC-2.2 met (MH-19 owns format, MH-7 pipeline, both files); AC-3.1 met (design §4 justification with ADR 0009, release-process.md, FIRST-RELEASE-TAG placeholder); AC-3.2 met (4 Markdown files only, no assessment edit); NFR-1 met (one PR, prose only). Seams: MH-18→MH-19 holds (release-tagging cites ADR 0009 as fixed input; Scheme byte-intact under the later Tag discipline section); MH-7 forward reference correct (still triaged); CHANGELOG agreement holds. Invariants touched: None — confirmed (prose only). Design vs shipped: no divergences (D1–D6 landed; Spec deviations: None accurate). Stale documentation: none.
