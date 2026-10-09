## Release Pipeline — Requirements

> A tag-triggered release pipeline that builds cross-platform archives with GoReleaser, checksums, SBOM, build-provenance attestations, keyless signatures and licence notices, publishing from a protected environment only — turning the Stage 1 binary into something users can install and verify. A slice of v2 §§1, 17–18. Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it.

## Context

- **Backlog card**: MH-7
- **Issue**: [#28](https://github.com/turbokast/mythhelm/issues/28) (card discussion; card holds status, stage, score)

No release pipeline exists: the repository has no `.goreleaser*` config, no release workflow (`.github/workflows/` holds CI, lint, docs and `release-drafter.yml` for notes only), no `Makefile` and no `packaging/`. Releases to date (v0.0.1) are manual operator actions, not pipeline products. The deferred automation rows that this pipeline activates are listed in `docs/automation.md` (GoReleaser archives/checksums/SBOM, `actions/attest-build-provenance`, cosign keyless via GitHub OIDC, `go-licenses` notices), each with trigger "First release"; the package-manager and macOS x64 rows stay Deferred (N1, N4). The scheme the pipeline implements is decided and accepted (ADR-0009 `docs/decisions/0009-version-numbering-scheme.md`), and tag format/mechanics belong to MH-19 (ADR-0010 `docs/decisions/0010-release-tag-discipline.md`, accepted). Actual tags, signing settings and releases stay operator actions per the card Notes.

Grounding verdicts (2026-10-07):

- `no release pipeline exists` HOLDS (no `.goreleaser*`, no release workflow, no `Makefile`/`packaging/`; v0.0.1 published manually).
- `docs/automation.md Deferred table holds the release rows` HOLDS (GoReleaser, attestations, cosign keyless, package managers, go-licenses, macOS x64 — all trigger "First release").
- `v2 demands installable verifiable releases` HOLDS (v2 §17.3; gates G01/G10 at v2 §18.3).
- `issue #28 section references (§16.5, §19.4, §19.5, §20.3, gates at §18.7)` PARTIAL: stale numbering; the card's v2 §§1, 17–18 govern.
- `tags, signing settings and releases stay operator actions` HOLDS (card Notes; ADR-0010 accepted).
- `version scheme recorded for MH-7 to implement` HOLDS (ADR-0009 accepted; MH-18 AC-2.1).

### Objectives

- **O1**: Publishing a release (operator action), which creates the version tag per ADR-0010, runs a release job that builds, signs and publishes verifiable archives for every supported platform.
- **O2**: A user can install the binary from a release archive and verify its checksum, provenance attestation and signature with documented commands.
- **O3**: Nothing release-capable is reachable from pull requests: publishing lives only in the tag-triggered job bound to the protected `release` environment.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Package-manager publishing (Homebrew, Scoop, winget, AUR). | v2 §17.3: archives are the baseline; store/tap tokens stay out of every job this spec adds. |
| N2 | The first real release itself (operator cuts it when ready). | G10: the pipeline must be proven on a test tag before any real release uses it. |
| N3 | Tag format and mechanics (MH-19, ADR-0010). | ADR-0010: the pipeline consumes the tag format, never redefines it. |
| N4 | macOS x64 CI leg beyond the release matrix (deferred row). | v2 §17.1: only tested subsets are advertised; the release matrix names exactly what it builds. |

## Functional Requirements

Acceptance criteria use EARS. Tags in brackets name the invariants, gates and sections each one serves.

### FR-1 — Tag-triggered GoReleaser build (§17.3, G01, G10, AT-46)

- **AC-1.1** [§17.3] When the operator publishes a release, which creates a version tag matching the ADR-0010 format, the release workflow shall trigger on the tag (GoReleaser consumes the tag on tag-push per ADR-0010) and build archives for the release matrix — linux/amd64, linux/arm64, darwin/arm64, windows/amd64, windows/arm64 per the CI runner matrix (`docs/automation.md:24`), darwin/amd64 excluded per N4, per the confirmed Q1 answer (`refinement-log.md`) — with checksums and an SBOM per archive.
- **AC-1.2** [G10] The release workflow shall trigger only on version tags; a push to any branch or a pull request shall not run any release job.
- **AC-1.3** [AT-46] When the build matrix or GoReleaser config changes, a pipeline dry-run on an operator-owned `test/*` tag (excluded from real-release matching, per the confirmed Q3 answer in `refinement-log.md`) shall prove the archives still build before the change merges. Initial delivery is proven once by the Task 6 dry run at the merged head; every later change uses the design §8 procedure at the PR head.

### FR-2 — Provenance, signatures and notices (§17.3, G10, AT-40)

- **AC-2.1** [§17.3] Each release's archives and checksums shall carry SLSA build-provenance attestations and Sigstore keyless signatures via GitHub OIDC, with no stored signing keys. The signature model is signed-checksum binding: cosign signs `dist/checksums.txt` and the archives verify through it (design §5); attestations cover checksums and archives directly.
- **AC-2.2** [§17.3] Each release archive shall include the third-party licence notices generated by `go-licenses`, and the release shall publish the changelog, checksums and SBOM beside the archives.
- **AC-2.3** [AT-40] A user following the documented verify steps shall be able to check an archive's checksum, attestation and signature; a tampered archive shall fail verification.

### FR-3 — Protected publishing (v2 §17.3, §8, G10)

- **AC-3.1** [§17.3] Publishing steps shall run only in jobs bound to the protected `release` environment; no secret or token capable of publishing shall be reachable from any other job.
- **AC-3.2** [§8] If the `release` environment or its required approvals are missing, then the pipeline shall stop before publishing rather than publishing unprotected.
- **AC-3.3** [G10, I14] Every release shall link `docs/limitations.md` (the published limitations register) from the release notes, so advertised capabilities stay bounded by what is tested.

## Non-Functional Requirements

- **NFR-1** [§17.2] Each release job shall set an explicit `timeout-minutes` (default 60); a hung build step fails the job on timeout rather than hanging the release. No custom runners.
- **NFR-2** [G01, I13] Everything the pipeline needs besides the operator's release publication shall be free and public: no paid service, private runner or stored secret the project does not already hold.

## Definition of Done

- [ ] AC-1.1 to AC-3.3 each have a named test or check that fails before the change and passes after it.
- [ ] A test-tag dry run proves the pipeline end to end without publishing a real release, except attest-to-release and `keep-existing` behaviour, which only the first real release exercises (design H2/H6; N2).
- [ ] `scripts/harness/gate.sh go` and `gate.sh harness` pass; workflow lint (`actionlint`, `zizmor` in CI) green.
- [ ] MH-7 synced to `specced` (this step), then implemented, finalized and PM-synced to `shipped`.

## Open Questions

- **Q1** (answered 2026-10-07): Confirmed as the CI runner matrix — linux/amd64, linux/arm64, darwin/arm64, windows/amd64, windows/arm64, no darwin/amd64 (`refinement-log.md`). Had blocked FR-1.
- **Q2** (answered 2026-10-07): The environment did not exist; the operator created `release` with required reviewer (self) + `v*` tag policy, verified via API (`refinement-log.md`). Had blocked FR-3.
- **Q3** (answered 2026-10-07): Confirmed as operator-owned `test/*` tags, excluded from real-release matching (`refinement-log.md`). Had blocked AC-1.3.
- **Q4**: GoReleaser config version pinning and SBOM format (SPDX vs CycloneDX)? Design decision, not a blocker: no AC names a version or format. Default: pinned GoReleaser action version, SPDX SBOM; /spec records the exact pin in `design.md`.

## Dependencies

- Prerequisite decisions: ADR-0009 (version scheme, accepted) and ADR-0010 (tag discipline, accepted) — consumed, not re-decided.
- Prerequisite specs: `specs/*/version-numbering/` and `specs/*/release-tagging/` (both done).
- Conflicting: none; `todo/` empty and `in-progress/qualification-registry/` disjoint per design §12; release domain is otherwise untouched.
- Independent of MH-10/MH-12/MH-22; runs in the parallel lane.

## Impacted components

- New: `.github/workflows/release.yml` (tag-triggered pipeline), `.goreleaser.yml` (archives, checksums, SBOM, licence notices).
- Touched: `docs/automation.md` (activate the Deferred release rows), `docs/release-process.md` (operator verify/publish steps), `.github/` workflow lint surface.
- Tests: workflow lint (`actionlint` pinned, `zizmor` in CI), config validation (`goreleaser check`), test-tag dry run (operator-triggered, observed in CI).
