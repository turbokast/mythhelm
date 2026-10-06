## Release Tagging — Design

> Implements `specs/*/release-tagging/requirements.md` (MH-19). Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it. W-IDs refer to the waves in mythhelm-synthesis/MYTHHELM_Implementation_Plan.md.

### 1. Current state

- Zero releases, zero git tags (`git tag -l` returns nothing); no release pipeline (MH-7 triaged, unspecced — tag-triggered GoReleaser with checksums, SBOM, SLSA build-provenance attestations, keyless signatures and licence notices, publishing from a protected environment only, per `product/backlog.md:67-76` and issue #28).
- The harness's own release flow already fixes the tagging mechanics (`.claude/skills/finalize-spec-tag/SKILL.md`): the version comes from release-drafter's resolved draft, publishing runs `gh release edit vX.Y.Z --draft=false --target <merge>`, "publishing the draft creates the tag `vX.Y.Z` at `<merge>`", and "never create or push a `v*` tag by hand". `.claude/hooks/guard-publish.sh` blocks publishing commands outside an armed window. Tags created by release publication are lightweight GitHub tags.
- Release-drafter resolves versions into `v`-prefixed names/tags (`tag-template: "v$RESOLVED_VERSION"`, `semver-increment` major/minor rules, `.github/release-drafter.yml`); drafts are never tagged — a draft is not a release.
- Release authority: the lead maintainer holds it (`GOVERNANCE.md:9`); maintainers "cut releases" (`GOVERNANCE.md:8`). W16 keeps release/tag/publish an explicit operator action (`mythhelm-synthesis/MYTHHELM_Implementation_Plan.md:213-221`).
- The scheme input: `specs/todo/version-numbering/` (MH-18, moved 2026-10-06) recommends SemVer 2.0.0 ratified (design D1), recorded as ADR 0009 + `docs/release-process.md` scheme section; this spec's format carries that scheme (delivery dependency recorded; both checkpoints run together).
- Repository settings are readable without changing them: `gh api repos/turbokast/mythhelm/rulesets` lists the `Protect main` ruleset (verified 2026-10-06); classic tag protection has no readable record (endpoint 404s).
- No tag-signing practice exists anywhere in the tree (structural sweep 2026-10-06 over the signing-relevant surfaces — `grep -rniE "gpgsign|gpg.*(tag|sign)|tag.*(sign|gpg)|ssh-sign|ssh\.sign" .github/ scripts/ docs/ *.md *.yml` piped through `grep -vi "signed-off|sign-off|DCO"` — returns three `commit.gpgsign=false` disables in harness scripts plus one WORKFLOW.md prose false positive: commit mechanisms, no tag signing; provenance is planned at the artifact level via SLSA/cosign, MH-7). The gap record: `specs/*/openssf-badge/design.md:30` — `version_tags` Unmet, "no releases yet, no git tags", tracked in MH-19 / issue #114.

### 2. Tag format (FR-1, G10, W16)

`v<semver>` (Q2-b, following MH-18's scheme): `v0.1.0` for an S1-style preview, `v1.2.3` for a later stable. Pre-release markers use dotted numeric suffixes: `v1.2.3-rc.1`, `v1.2.3-beta.2`. Valid examples: `v0.1.0`, `v1.2.3-rc.1`. Invalid examples: `0.1.0` (missing `v`), `v1.2` (incomplete SemVer), `v1.2.3-rc1` (undotted marker — invalid per project convention, though bare SemVer would allow it; the distinction is stated so the convention is never mistaken for the spec).

Tag object type (Q1-c): lightweight, as created by release publication. The established publish flow yields lightweight tags; provenance lives on the artifacts (SLSA/cosign per the MH-7 plan) and tagger identity in the GitHub push record — not in tag objects. No GPG/SSH tag signing: no key infrastructure exists, and artifact-level signatures already cover authenticity.

The format is implementable by a tag-triggered GoReleaser pipeline without re-deciding (AC-1.2): GoReleaser consumes the pushed tag name; the `v*` trigger filter selects tag-push candidates and the pipeline validates the pushed name against this shape before publishing (a bare `v*` glob alone admits malformed tags, so the format check is the gate, not the glob).

### 3. Tagging is an explicit operator action (FR-2, W16, G10)

Only the operator creates release tags (AC-2.1): the maintainer holding release authority publishes the release (`gh release edit vX.Y.Z --draft=false`, the finalize-spec-tag flow), which creates the tag — never `git tag` by hand. Required permission: release write (tag creation rides the release write); the implementing task records the observed ruleset/tag-protection state via read-only `gh api` (rulesets verified readable; the `Protect main` baseline recorded) and any settings change stays an operator action.

A release for tagging purposes (AC-2.2, Q3-a): every published release including pre-releases gets a tag, with the §2 marker rule; drafts are never tagged (a draft is not a release — matches the release-drafter flow, where an empty Unreleased is a stop, not a tag).

### 4. Tag→pipeline trigger contract (FR-3, W16, G10)

Required behaviors MH-7 implements (Q4-a: design-ahead fixed input — no workflow YAML here, per N2):

- Pushing a release tag starts the release pipeline: publication creates `vX.Y.Z` at the target merge, and the tag push matching the `v*` filter triggers the pipeline. No release artifact ships without its tag — the pipeline requires tag context.
- Failure rule (AC-3.2): a pipeline run from a malformed or unauthorized tag fails closed with no partial release — non-matching tags never match the trigger filter, and an aborted run publishes nothing.

### 5. Records (FR-2)

Two files, mirroring MH-18's shape (joint home per requirements Impacted components — one document, D6):

- **`docs/decisions/0010-release-tag-discipline.md`** — the mechanics decision per the decisions README template: Context (the OpenSSF gap, the harness publish flow, the artifact-level provenance plan), Decision (lightweight tags created only by release publication, never by hand; `v<semver>` shape with dotted pre-release markers; operator publishes), Consequences (GoReleaser consumes tag-push; provenance via SLSA/cosign, not tag objects; a hand-pushed `v*` tag is a discipline violation).
- **`docs/release-process.md`** (created by MH-18's task; this task adds the tag-discipline section under the `## Tag discipline` heading) — the operational record: format string with valid/invalid examples, the publish-creates-tag mechanism with the never-by-hand rule, the permission + observed-settings record, the §4 trigger contract, and the pre-release rule. Delivery order guarantees MH-18 merged first; if the file is absent the task stops and reports rather than authoring MH-18's section.

### 6. Prepared assessment justification (FR-4, §19.3 Revision 1.1, G10)

The spec record carries the paste-ready `version_tags` → Met text. The operator pastes it after the first tagged release demonstrates the discipline (N3); until then the live badge stays passing and this spec touches no assessment (AC-4.2):

```text
Met — every release is tagged in git per ADR 0010
(docs/decisions/0010-release-tag-discipline.md) and the tag-discipline
section of docs/release-process.md, demonstrated by <FIRST-RELEASE-TAG>.
Fill the tag at flip time; the flip happens only after that tagged
release ships.
```

### 7. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | Lightweight tags, created only by release publication, never by hand (Q1-c) | The harness's own release skill mandates publish-creates-tag (which yields lightweight tags) and forbids hand-pushed `v*` tags; contradicting it would split one repo into two disciplines. Rejected: annotated (contradicts the established flow); signed tags (no key infrastructure; authenticity already covered by artifact-level SLSA/cosign in the MH-7 plan). |
| D2 | `v<semver>` with dotted pre-release markers (Q2-b) | Follows MH-18's scheme; matches the release-drafter `v`-prefixed templates already in-tree. Rejected: a scheme-independent shape (the format must carry the decided scheme — N1 binds). |
| D3 | Every published release incl. pre-releases is tagged; drafts never (Q3-a) | Pre-releases are published artifacts users install — leaving them untagged breaks tag-per-release. Drafts are explicitly not releases (release-drafter flow). Rejected: full-releases-only (holes in the discipline). |
| D4 | Design-ahead fixed input for MH-7 (Q4-a) | MH-7 is unspecced with no date; the discipline must exist before the pipeline it constrains. Rejected: co-design (couples this spec's landing to MH-7's unknown schedule). |
| D5 | ADR 0010 + tag-discipline section in `docs/release-process.md` | Tag mechanics are a public-contract decision with supply-chain implications — ADR-worthy; the operational detail belongs with MH-18's scheme section as MH-7's single input. Rejected: process-doc-only (buries a real decision); separate tag-discipline doc (two homes for one release process). |
| D6 | One document (`docs/release-process.md`) for scheme + discipline + (later) pipeline | Single operational input for MH-7, no cross-file drift. The file states each section's owner (MH-18 scheme, MH-19 discipline, MH-7 pipeline). |

### 8. Honesty register

| Spec demand | Position |
|---|---|
| G10 tag-per-release | Not yet met: zero tags exist. This spec records the discipline; the first release demonstrates it under MH-7. |
| `version_tags` → Met | Prepared, not flipped: justification text in §6, operator action post-first-tagged-release (N3). |
| Format conditional on MH-18's scheme | Resolved at the spec checkpoint (both approved together): the checkpoint confirmed SemVer with no overturn, so the format is `v<semver>` unconditionally and no CalVer follow exists. The pre-checkpoint conditional survives in git history. |
| Trigger/pipeline behaviors | Required here, implemented by MH-7: stated as behaviors, not workflow YAML (N2 binds — nothing assumes undesigned pipeline details). |
| Tag-protection settings | Observed, not changed: the implementer records current state read-only; any ruleset/tag-protection change is an operator action. |

### 9. Cross-spec references

- Builds on: `specs/*/openssf-badge/` (MH-9, shipped) — the `version_tags` Unmet record (design §3) and the card-per-Unmet mechanism (FR-3).
- Builds on: MH-18 (`specs/todo/version-numbering/`, moved 2026-10-06) — the tag format carries the MH-18 scheme (delivery dependency recorded; specifying runs ahead, building waits for the scheme decision).
- Feeds into: MH-7 (release pipeline, triaged, unspecced) — implements this discipline as fixed input; the discipline lands first.
- Conflicts: none. `specs/todo/tui-tape-recording/` touches `docs/demos/`, embeds, `docs.yml` and `docs/automation.md` — disjoint from `docs/decisions/` and `docs/release-process.md`; `specs/todo/version-numbering/` is the builds-on parent (its files are this spec's inputs, not overlaps); `specs/*/strict-lint-set/` touches lint config only; `specs/in-progress/` and `specs/unfinalized/` are empty.
