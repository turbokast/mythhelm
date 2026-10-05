## Version Numbering — Requirements

> Unrefined. Run /refine-spec version-numbering before /spec.

> Decides the project's release version-numbering scheme — Semantic Versioning or Calendar Versioning with a micro level — and records it where the release process will honor it, before MH-7 ships the first release. Closes the OpenSSF `version_semver` (SUGGESTED) gap. Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it. Historical Revision 1.1 pins (§19.3, §19.4) keep their original meaning per docs/spec/README.md.

## Context

- **Backlog card**: MH-18
- **Issue**: [#113](https://github.com/turbokast/mythhelm/issues/113) — "Adopt SemVer or CalVer for releases (MH-18)".
- **Problem**: The OpenSSF passing assessment answers `version_semver` (SUGGESTED) as Unmet: no releases exist and no demonstrated version-numbering scheme backs a release process. The openssf-badge spec recorded the gap in design §3 with its justification referencing this card and issue. The decision is not greenfield: `CHANGELOG.md` already claims Semantic Versioning adherence, but no release has ever carried a version and no release process exists to honor the claim (MH-7 is triaged, unspecced).
- **Work**: Ratify SemVer or overturn it for CalVer with a recorded rationale, reconcile the CHANGELOG claim with the decision, and record the scheme where MH-7's release pipeline will implement it. The assessment flip on bestpractices.dev stays an operator action; this spec prepares the justification text.
- **Grounding verdicts** (2026-10-05, main @ 176fead):
  - Issue #113 describes this gap and names MH-18: HOLDS (read 2026-10-05; labels documentation, enhancement; no comments).
  - `version_semver` SUGGESTED, answered Unmet, tracked in MH-18/issue #113: HOLDS (`specs/done/openssf-badge/design.md:29`).
  - No releases exist: HOLDS — `git tag -l` returns zero tags.
  - No version-numbering format adopted: PARTIAL — `CHANGELOG.md:6` already claims "this project adheres to Semantic Versioning" (Keep-a-Changelog boilerplate), but the claim is unexercised (zero releases) and recorded in no release process (none exists). The decision is ratify-or-overturn, not greenfield.
  - Current contract v2 §§17–18 via W16: PARTIAL — v2 names no versioning scheme; W16 governs release/tag/publish as explicit operator actions (`MYTHHELM_Implementation_Plan.md:213-221`). The scheme demand comes from the OpenSSF criterion, executed through the W16 release language.
  - Decision must land before MH-7's first release: HOLDS as a sequencing constraint — MH-7 is triaged with no spec; its Summary is a tag-triggered GoReleaser pipeline and its Notes reserve tags to operator action.
  - No spec covers MH-18: HOLDS — `MH-18` appears in `specs/` only as the openssf-badge follow-up pointer (`specs/done/openssf-badge/tasks.md:53`, `handoff.md` Task 2, retrospective acceptance); `spec-lifecycle.sh resolve version-numbering` finds nothing.
  - Gap is open and closable today: HOLDS — criterion Unmet; no release-process record exists to contradict; `mythhelm version` prints an unstamped `devel` on a bare checkout.

### Objectives

- **O1**: The project has a recorded, reasoned version-numbering decision: SemVer ratified, or CalVer with a micro level adopted.
- **O2**: The scheme is recorded where MH-7's release pipeline will implement it, and the CHANGELOG claim agrees with it.
- **O3**: The `version_semver` → Met justification text is prepared and reviewed, ready for the operator to flip once the first release demonstrates the scheme.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | The first release itself and the release pipeline. | MH-7: this decision lands first and MH-7 implements it; nothing here assumes pipeline details MH-7 has not designed. |
| N2 | Tag mechanics (tag format string, who tags, GoReleaser trigger). | MH-19 owns those; its format must carry this spec's scheme, so MH-19 builds on this decision. |
| N3 | Flipping the assessment on bestpractices.dev. | Justification text + scheme record are prepared here; the flip itself is an operator action after the first release demonstrates the scheme. |
| N4 | Renumbering anything already shipped. | Nothing versioned has shipped (zero tags); dev pseudo-versions stay as they are unless the decision explicitly covers them. |

## Functional Requirements

Acceptance criteria use EARS. Tags in brackets name the invariants, gates and sections each one serves.

### FR-1 — Scheme decided with rationale (G10, W16)

- **AC-1.1** [G10] The spec record shall name the adopted scheme — SemVer, or CalVer with its micro-level rule — with a rationale weighing the GoReleaser/tag ecosystem, pre-alpha 0.x semantics, and the S1 preview versus later releases.
- **AC-1.2** [G10] If the decision overturns SemVer for CalVer, then `CHANGELOG.md`'s SemVer claim shall be corrected in the same change; if it ratifies SemVer, the claim shall stand and the decision shall cite it.
- **AC-1.3** [W16] The decision shall state its scope: release versions only, or release plus dev/nightly version strings.

### FR-2 — Scheme recorded for the release process (W16, G10)

- **AC-2.1** [W16] The adopted scheme, with an example version string, shall be recorded in the release-process location MH-7 will implement, so MH-7's spec can cite it instead of re-deciding.
- **AC-2.2** [G10] The record shall name MH-19 as the owner of the tag format and mechanics carrying the scheme, and MH-7 as the pipeline implementing both.

### FR-3 — Assessment evidence prepared (§19.3, G10)

- **AC-3.1** [§19.3] The spec record shall carry the new `version_semver` justification text citing the scheme record, ready for the operator to paste once the first release demonstrates it.
- **AC-3.2** [G10] Until the operator flips the assessment, the live badge shall remain passing; this spec shall not edit the assessment itself.

## Non-Functional Requirements

- **NFR-1** [G10] The decision record shall be reviewable as prose in one pull request with no code changes required.

## Definition of Done

- [ ] AC-1.1 to AC-3.2 each have a named check; the checks are prose/record assertions, CI green.
- [ ] The scheme record exists where MH-7 will find it; CHANGELOG agrees with it.
- [ ] MH-19's dependency on the scheme decision is recorded (this spec's decision, not its release).
- [ ] Justification text for `version_semver` → Met is reviewed and ready for the operator.

## Open Questions

1. **SemVer or CalVer?** Blocks FR-1. Options: (a) ratify SemVer — CHANGELOG already claims it, GoReleaser-native, overturn needs a reason; (b) adopt CalVer with a micro level — date-ordered, suits a fast pre-alpha cadence. Default: (a). /spec weighs and recommends; the maintainer's spec checkpoint confirms.
2. **Where is the scheme recorded?** Blocks FR-2. Options: (a) an ADR under `docs/decisions/` (a public-contract decision) plus a pointer from the release docs; (b) a new release-process document the MH-7 spec extends; (c) both — ADR decides, release doc carries. Default: (c). /spec fixes the files.
3. **Do dev/nightly versions fall in scope?** Blocks AC-1.3. Options: (a) releases only — manifest-style pseudo-versions continue untouched; (b) releases plus a dev-version rule. Default: (a).
4. **Who owns the tag-format string at the MH-18/MH-19 seam?** Blocks AC-2.2. Options: (a) MH-19 owns format and mechanics, constrained by this scheme; (b) this spec fixes the full format and MH-19 implements it. Default: (a), matching the card summaries.

## Dependencies

- **Builds on: `openssf-badge` (MH-9, `specs/done/openssf-badge/`, shipped 2026-10-04)** — the `version_semver` Unmet record (design §3) and the card-per-Unmet mechanism (FR-3).
- **Feeds into: MH-7 (release pipeline, triaged, unspecced)** — MH-7 implements this scheme; this decision lands first.
- **Feeds into: MH-19 (tag discipline, triaged, unspecced)** — MH-19's tag format carries this scheme; MH-19 builds on this decision (delivery dependency recorded).
- **Supersedes**: nothing (the CHANGELOG line is reconciled, not superseded). **Conflicts**: none known; `specs/todo/` and `specs/in-progress/` are empty.

## Impacted components

- `docs/decisions/` (docs domain): likely new ADR recording the decision — /spec confirms file and number.
- Release-process record location (docs domain): per Open Q2 — the file MH-7 will cite.
- `CHANGELOG.md` (docs domain): header claim ratified or corrected, only if the decision requires it.
- Spec record: rationale, prepared justification text.
