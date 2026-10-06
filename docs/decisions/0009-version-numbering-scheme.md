# 0009. Version numbering scheme

- Status: accepted
- Date: 2026-10-06

## Context

The OpenSSF passing assessment answers `version_semver` (SUGGESTED) as Unmet: no releases exist and no demonstrated version-numbering scheme backs a release process (tracked in MH-18 / issue #113). Zero git tags exist and no release process record exists yet.

Meanwhile `CHANGELOG.md:6` already claims the project adheres to Semantic Versioning — Keep-a-Changelog boilerplate that no release has ever exercised. The decision is therefore ratify-or-overturn, not greenfield: either the claim stands with a scheme behind it, or the claim is corrected.

## Decision

Ratify **Semantic Versioning 2.0.0** as the version-numbering scheme for releases, with this rationale:

- **Go module-path fit.** `go.mod` declares the module path with no `/vN` major suffix, so under Go's major-version rules the path admits only v0/v1 tags; a CalVer year-major scheme would need absurd per-year suffixes to stay Go-valid. SemVer is the zero-churn fit.
- **In-tree v-prefixed SemVer machinery.** `.github/release-drafter.yml` already computes semver increments into v-prefixed names and tags (`v$RESOLVED_VERSION` with `semver-increment` major/minor rules). The release machinery speaks SemVer already; CalVer would force its reconfiguration.
- **Pre-alpha 0.x semantics.** SemVer 0.x means "anything may change" — exactly the pre-alpha contract, with no new vocabulary needed. The S1 preview and subsequent pre-alpha releases are 0.x; 1.0.0 marks the first stability declaration, and declaring it is a future decision, not this one.

The standing `CHANGELOG.md:6` claim stands: no CHANGELOG edit accompanies this decision.

Scope is release versions only. The `devel`/`unknown` dev strings continue untouched — they describe unstamped builds, not versions, and build stamping is MH-7 territory. Tags are assumed v-prefixed per the release-drafter convention; the exact tag format is owned by MH-19.

## Consequences

MH-7 implements tag-triggered GoReleaser releases under this scheme, and MH-19's tag format carries it; both cite this record instead of re-deciding. Overturning the scheme needs a new ADR. The `version_semver` assessment stays Unmet until the operator flips it after the first release demonstrates the scheme.
