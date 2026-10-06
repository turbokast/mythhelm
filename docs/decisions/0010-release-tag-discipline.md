# 0010. Release tag discipline

- Status: accepted
- Date: 2026-10-06

## Context

The OpenSSF passing assessment answers `version_tags` (SUGGESTED) as Unmet: no releases exist and no git tags (tracked in MH-19 / issue #114).

The harness's own release flow already fixes the tagging mechanics (`.claude/skills/finalize-spec-tag/SKILL.md`): publishing runs `gh release edit vX.Y.Z --draft=false --target <merge>`, "publishing the draft creates the tag `vX.Y.Z` at `<merge>`", and "never create or push a `v*` tag by hand". Tags created by release publication are lightweight GitHub tags.

Provenance is planned at the artifact level via SLSA build-provenance attestations and cosign signatures (the MH-7 plan), not in tag objects; no tag-signing practice exists anywhere in the tree and no key infrastructure exists.

## Decision

Use lightweight tags, created only by release publication, never by hand.

The mechanism is the finalize-spec-tag publish flow: the operator publishes the release (`gh release edit vX.Y.Z --draft=false --target <merge>`), and publishing the draft creates the tag at the merge. Never `git tag` by hand; a hand-pushed `v*` tag is a discipline violation.

Tag shape is `v<semver>` with dotted pre-release markers, carrying the MH-18 scheme: `v0.1.0` for an S1-style preview, `v1.2.3-rc.1` for a dotted pre-release. Only the operator publishes, holding release authority.

Tags are unsigned: authenticity is covered by artifact-level SLSA and cosign per the MH-7 plan, and no key infrastructure exists for tag signing.

## Consequences

GoReleaser consumes the pushed tag name on tag-push; provenance rides the artifacts via SLSA and cosign, not tag objects, and tagger identity lives in the GitHub push record.

Tags stay unsigned because artifact-level SLSA and cosign per the MH-7 plan already cover authenticity and no key infrastructure exists. Introducing tag signing would need new key infrastructure with no additional coverage.

A hand-pushed `v*` tag is a discipline violation. Overturning this discipline needs a new ADR. The `version_tags` assessment stays Unmet until the operator flips it after the first tagged release demonstrates the discipline.
