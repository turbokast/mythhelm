## Release Tagging — Requirements

> Unrefined. Run /refine-spec release-tagging before /spec.

> Establishes the tag-per-release discipline — tag format, who tags, and the GoReleaser trigger contract — alongside the MH-7 release pipeline and before the first release. Closes the OpenSSF `version_tags` (SUGGESTED) gap. Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it. Historical Revision 1.1 pins (§19.3, §19.4) keep their original meaning per docs/spec/README.md.

## Context

- **Backlog card**: MH-19
- **Issue**: [#114](https://github.com/turbokast/mythhelm/issues/114) — "Tag every release in git (MH-19)".
- **Problem**: The OpenSSF passing assessment answers `version_tags` (SUGGESTED) as Unmet: no releases exist and no git tags. The openssf-badge spec recorded the gap in design §3 with its justification referencing this card and issue. No release workflow exists (`release-drafter.yml` drafts notes only), and MH-7's pipeline is triaged but unspecced — so this discipline is designed ahead as MH-7's input, not extracted from automation.
- **Work**: Define the tag format carrying MH-18's numbering scheme, record operator-only tagging per W16, and define the tag→pipeline trigger contract MH-7 implements. This spec creates no tags (nothing to release yet) and flips no assessment; it records the discipline and prepares the justification.
- **Grounding verdicts** (2026-10-05, main @ 176fead):
  - Issue #114 describes this gap and names MH-19: HOLDS (read 2026-10-05; labels documentation, enhancement; no comments).
  - `version_tags` SUGGESTED, answered Unmet, tracked in MH-19/issue #114: HOLDS (`specs/done/openssf-badge/design.md:30`).
  - No releases and no git tags: HOLDS — `git tag -l` returns zero tags.
  - Current contract v2 §§17–18 via W16: HOLDS — W16 rules that "Release/tag/publish remains an explicit operator action" (`MYTHHELM_Implementation_Plan.md:220`), which governs the who-tags half directly; the per-release rule comes from the OpenSSF criterion.
  - Discipline lands alongside MH-7: HOLDS as a constraint — MH-7 is triaged with no spec and tag-triggered per its Summary; `.github/workflows/` holds 13 files, none a release pipeline.
  - No spec covers MH-19: HOLDS — `MH-19` appears in `specs/` only as the openssf-badge follow-up pointer and the `version-numbering` seam note; `spec-lifecycle.sh resolve release-tagging` finds nothing.
  - Gap is open and the discipline is writable today: HOLDS — criterion Unmet; no discipline record exists; scheme input comes from MH-18 (delivery dependency recorded).

### Objectives

- **O1**: The tag-per-release discipline — format, who tags, trigger contract — is recorded where MH-7 will implement it.
- **O2**: The tag format carries MH-18's numbering scheme without re-deciding it.
- **O3**: The `version_tags` → Met justification text is prepared and reviewed, ready for the operator to flip once the first tagged release demonstrates the discipline.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | The numbering scheme itself. | MH-18 decides it; this spec's format carries that scheme, and MH-19 builds on the MH-18 decision. |
| N2 | The release pipeline implementation. | MH-7 builds it; this discipline is its input, and nothing here assumes pipeline details MH-7 has not designed. |
| N3 | Flipping the assessment on bestpractices.dev. | Justification text + discipline record are prepared here; the flip is an operator action after the first tagged release. |
| N4 | Creating any tag now. | Zero releases exist, so zero tags are created; the discipline is recorded before the first tag it governs. |

## Functional Requirements

Acceptance criteria use EARS. Tags in brackets name the invariants, gates and sections each one serves.

### FR-1 — Tag format defined (G10, W16)

- **AC-1.1** [G10] The discipline shall define the tag format string — prefix, the MH-18 scheme it carries, and annotated/lightweight/signature expectations — with one valid and one invalid example.
- **AC-1.2** [W16] The format shall be implementable by a tag-triggered GoReleaser pipeline without re-deciding (MH-7 cites it).

### FR-2 — Tagging is an explicit operator action (W16, G10)

- **AC-2.1** [W16] The discipline shall state that only the operator creates release tags, and what repository permission that requires, leaving the settings change itself to the operator.
- **AC-2.2** [G10] The discipline shall define what counts as a release for tagging purposes (drafts and pre-releases included or excluded, explicitly).

### FR-3 — Tag→pipeline trigger contract (W16, G10)

- **AC-3.1** [W16] The discipline shall define the trigger contract MH-7 implements: pushing a release tag starts the release pipeline, and no release artifact ships without its tag.
- **AC-3.2** [G10] The contract shall state the failure rule: a pipeline run from a malformed or unauthorized tag fails closed with no partial release.

### FR-4 — Assessment evidence prepared (§19.3, G10)

- **AC-4.1** [§19.3] The spec record shall carry the new `version_tags` justification text citing the discipline record, ready for the operator to paste once the first tagged release demonstrates it.
- **AC-4.2** [G10] Until the operator flips the assessment, the live badge shall remain passing; this spec shall not edit the assessment itself.

## Non-Functional Requirements

- **NFR-1** [G10] The discipline shall be reviewable as prose in one pull request with no code changes required.

## Definition of Done

- [ ] AC-1.1 to AC-4.2 each have a named check; the checks are prose/record assertions, CI green.
- [ ] The discipline record exists where MH-7 will find it; it cites MH-18's scheme without re-deciding it.
- [ ] Zero tags created by this spec; the first future tag falls under the recorded discipline.
- [ ] Justification text for `version_tags` → Met is reviewed and ready for the operator.

## Open Questions

1. **Annotated, lightweight, or signed tags?** Blocks FR-1. Options: (a) annotated tags, unsigned; (b) signed tags (GPG/SSH); (c) lightweight. No default yet — /spec weighs GoReleaser/provenance practice and the MH-7 attestation plan.
2. **Tag prefix and exact shape?** Blocks FR-1. Options: (a) `v<semver>` Go convention; (b) scheme-dependent shape fixed once MH-18 decides. Default: (b) — the format follows the MH-18 scheme.
3. **What counts as a release?** Blocks AC-2.2. Options: (a) every published release including pre-releases gets a tag; (b) only full releases; drafts never tagged. Default: (a) with an explicit pre-release marker rule.
4. **Sequence with MH-7's design?** Blocks FR-3. Options: (a) design-ahead — this discipline is MH-7's fixed input; (b) co-designed with MH-7's spec. Default: (a), matching "alongside" as input-not-afterthought.

## Dependencies

- **Builds on: `openssf-badge` (MH-9, `specs/done/openssf-badge/`, shipped 2026-10-04)** — the `version_tags` Unmet record (design §3) and the card-per-Unmet mechanism (FR-3).
- **Builds on: MH-18 (`version-numbering`, unrefined)** — the tag format carries the MH-18 scheme (delivery dependency recorded; specifying runs ahead, building waits for the scheme decision).
- **Feeds into: MH-7 (release pipeline, triaged, unspecced)** — MH-7 implements this discipline; the discipline lands first.
- **Supersedes**: none. **Conflicts**: none known; `specs/todo/` and `specs/in-progress/` are empty.

## Impacted components

- Release-process record location (docs domain): per the joint home with MH-18's scheme record — /spec fixes whether one document or two.
- Spec record: format definition, trigger contract, prepared justification text.
- Repository tag settings: operator-configured per the recorded requirement — no file change, stated as an operator action.
