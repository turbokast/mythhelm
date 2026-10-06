## Version Numbering — Design

> Implements `specs/*/version-numbering/requirements.md` (MH-18). Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it. W-IDs refer to the waves in mythhelm-synthesis/MYTHHELM_Implementation_Plan.md.

### 1. Current state

- Zero releases, zero git tags (`git tag -l` returns nothing); no release pipeline (MH-7 triaged, unspecced; its Summary fixes only "tag-triggered" + GoReleaser terms, `product/backlog.md:67-76`); no release-process document exists in `docs/`.
- `CHANGELOG.md:6` already claims SemVer adherence (Keep-a-Changelog boilerplate), unexercised — the decision is ratify-or-overturn, not greenfield.
- `go.mod:1`: `module github.com/turbokast/mythhelm` — no `/vN` major suffix, while v2 dependencies in the same file carry theirs (`uax29/v2`, `go-osc52/v2`): the module path admits only v0/v1 tags under Go's major-version rules.
- `.github/release-drafter.yml`: `name-template`/`tag-template` are `v$RESOLVED_VERSION` with `semver-increment` major/minor rules — the in-tree release machinery already speaks v-prefixed SemVer. `docs/automation.md:68-72` keeps GoReleaser, attestations, cosign and licence notices at the "First release" tier (MH-7's territory).
- Dev strings today: `mythhelm version` on a bare checkout prints `mythhelm devel` / `commit: unknown` (`internal/buildinfo/buildinfo.go:43` default, `internal/cli/dispatch.go:154-177` wiring) — build-stamping states, not versions.
- The gap record: `specs/*/openssf-badge/design.md:29` — `version_semver` Unmet, "no releases yet, so no SemVer/CalVer format adopted", tracked in MH-18 / issue #113. G10 (v2 §18.3): the public-release gate. W16 governs release/tag/publish as explicit operator actions (`mythhelm-synthesis/MYTHHELM_Implementation_Plan.md:213-221`).
- ADRs live under `docs/decisions/` as `NNNN-short-title.md` per the README template (Status/Date/Context/Decision/Consequences); 0001–0008 exist, so this decision is 0009.

### 2. The decision (FR-1, G10, W16)

**Ratify Semantic Versioning 2.0.0** (Q1 option (a); the maintainer's spec checkpoint confirms). AC-1.1 rationale, weighed as required:

- **GoReleaser/tag ecosystem**: the module path admits only v0/v1 tags (no `/vN` suffix); CalVer years would need per-year major suffixes to stay Go-valid. Release-drafter already computes semver increments into `v`-prefixed names/tags. SemVer is the zero-churn fit; CalVer would fight both.
- **Pre-alpha 0.x semantics**: SemVer 0.x means "anything may change" — exactly the pre-alpha contract. No new vocabulary needed.
- **S1 preview vs later releases**: the S1 preview and subsequent pre-alpha releases are 0.x; 1.0.0 marks the first stability declaration (declaring it is a future decision, not this spec's — honesty register).

Scope (AC-1.3, Q3-a): release versions only. The `devel`/`unknown` dev strings stay as they are — they describe unstamped builds, and stamping is MH-7 build territory.

CHANGELOG (AC-1.2 ratify path): the `CHANGELOG.md:6` claim stands — no CHANGELOG edit; the ADR cites the claim (D5).

### 3. Records (FR-2, W16, G10)

Two files with distinct jobs, cross-linked (Q2-c):

- **`docs/decisions/0009-version-numbering-scheme.md`** — the decision with its rationale, following the decisions README template: Context (the OpenSSF gap, the unexercised CHANGELOG claim, zero tags), Decision (SemVer 2.0.0 ratified; releases-only scope; `v`-prefixed tags per the release-drafter convention, exact format owned by MH-19), Consequences (MH-7 implements tag-triggered GoReleaser releases under this scheme; MH-19's format carries it; overturning needs a new ADR).
- **`docs/release-process.md`** (new) — the operational record MH-7 implements and MH-19 cites: the adopted scheme (SemVer 2.0.0), example version strings (`v0.1.0` for an S1-style preview, `v1.2.3` for a later stable — shape illustrative, exact format per MH-19), scope (release versions only; dev strings out of scope), ownership (AC-2.2: MH-19 owns the tag format and mechanics carrying the scheme; MH-7 owns the pipeline implementing both), and an explicit scope line that pipeline sections land with MH-7 (nothing here assumes undesigned pipeline details, per N1/N2).

### 4. Prepared assessment justification (FR-3, §19.3 Revision 1.1, G10)

The spec record carries the paste-ready `version_semver` → Met text (D6). The operator pastes it after the first release demonstrates the scheme (N3); until then the live badge stays passing and this spec touches no assessment (AC-3.2):

```text
Met — releases follow Semantic Versioning 2.0.0 per ADR 0009
(docs/decisions/0009-version-numbering-scheme.md) and docs/release-process.md,
demonstrated by <FIRST-RELEASE-TAG>. Fill the tag at flip time; the flip
happens only after that release ships.
```

### 5. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | Ratify SemVer 2.0.0 (Q1-a) | Go module path admits only v0/v1 (CalVer years need absurd per-year suffixes); release-drafter already computes semver into `v`-prefixed names; CHANGELOG already claims it. Rejected: CalVer with a micro level (date-ordering does not outweigh the ecosystem fit; would force module-path and drafter reconfiguration). Maintainer checkpoint confirms. |
| D2 | ADR 0009 + `docs/release-process.md` (Q2-c) | AC-2.1 demands a release-process location MH-7 implements — an ADR alone is a decision record, not a process location. Rejected: ADR-only (no operational input for MH-7); process-doc-only (rationale belongs in an ADR per governance). |
| D3 | Releases-only scope (Q3-a) | `devel`/`unknown` are build-stamping states, not versions. Rejected: releases-plus-dev-rule (stamping is MH-7 build territory; deciding it here assumes undesigned pipeline details). |
| D4 | MH-19 owns the tag format and mechanics, constrained by this scheme (Q4-a) | Matches both card summaries and `specs/*/release-tagging/`, which is chartered to define the format string (AC-1.1). Rejected: this spec fixes the full format (duplicates MH-19's charter; two owners for one string). |
| D5 | No CHANGELOG edit on the ratify path | AC-1.2: the claim stands, the decision cites it. An edit would churn a correct claim. |
| D6 | Justification text lives in this design §4 | It is spec-record text by requirements (FR-3); `done/` keeps it permanently for the operator's flip. Rejected: a separate tree file (one more file for text that lives until the flip, then lives in the assessment). |

### 6. Honesty register

| Spec demand | Position |
|---|---|
| G10 public release under the scheme | Not yet met: no release exists. This spec records the scheme; MH-7 ships the first release under it; the assessment flips only after that release demonstrates it. |
| `version_semver` → Met | Prepared, not flipped: justification text in §4, operator action post-first-release (N3). |
| 1.0 stability line | Undeclared: 0.x until a future decision declares stability. This spec fixes the scheme, not the 1.0 date. |
| Dev/nightly version strings | Out of scope by decision (D3): `devel`/`unknown` continue untouched. |
| Tag format string | Owned by MH-19 (D4): the §3 examples are illustrative of the scheme, not the format. |

### 7. Cross-spec references

- Builds on: `specs/*/openssf-badge/` (MH-9, shipped) — the `version_semver` Unmet record (design §3) and the card-per-Unmet mechanism (FR-3).
- Feeds into: MH-7 (release pipeline, triaged, unspecced) — implements this scheme; this decision lands first.
- Feeds into: MH-19 (`specs/*/release-tagging/`, refined) — its tag format carries this scheme (delivery dependency recorded).
- Conflicts: none. `specs/todo/tui-tape-recording/` touches `docs/demos/`, embeds, `docs.yml` and `docs/automation.md` — disjoint from `docs/decisions/` and any release-process record; the refined siblings are disjoint by subject and files (`release-tagging` builds on this scheme, `strict-lint-set` touches lint config); `specs/in-progress/` and `specs/unfinalized/` are empty.
