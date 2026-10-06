# Release process

How MYTHHELM versions, tags and ships releases. The pipeline sections land with MH-7; this revision records only the version-numbering scheme the pipeline will implement.

## Scheme

Releases follow [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html), ratified in [ADR 0009](decisions/0009-version-numbering-scheme.md).

Example version strings: `v0.1.0` for an S1-style preview release, `v1.2.3` for a later stable release. The shapes are illustrative of the scheme; the exact tag format is owned by MH-19, whose tag format and mechanics carry this scheme.

Scope is release versions only; dev strings such as unstamped-build markers are out of scope and continue untouched. MH-7 implements the tag-triggered GoReleaser pipeline under this scheme. MH-7's spec extends this document with the pipeline sections; nothing here assumes pipeline details MH-7 has not designed.

## Tag discipline

Tag format is `v<semver>` carrying the scheme above, with dotted pre-release markers (MH-19 discipline, [ADR 0010](decisions/0010-release-tag-discipline.md)). Valid examples are `v0.1.0` and `v1.2.3-rc.1`. Invalid examples are `0.1.0` (missing `v`), `v1.2` (incomplete SemVer) and `v1.2.3-rc1` (undotted marker, invalid per project convention, though bare SemVer would allow it; the convention is stricter than the specification and never mistaken for it).

The operator-publishes mechanism applies: only the operator creates release tags, and only the operator publishes. The maintainer holding release authority publishes the release, which creates the tag, never by hand and never via `git tag` by hand. Required permission is release write (release-write); tag creation rides the release write. Publishing runs `gh release edit vX.Y.Z --draft=false --target <merge>`, the finalize-spec-tag flow, where publishing the draft creates the tag at the merge.

Observed settings are read-only and unchanged by this task. Command run on 2026-10-06 (read-only, settings unchanged):

```text
gh api repos/turbokast/mythhelm/rulesets
[{"id":24186707,"name":"Protect main","target":"branch","source_type":"Repository","source":"turbokast/mythhelm","enforcement":"active","node_id":"RRS_lACqUmVwb3NpdG9yec5TLnMAzgFxD1M","_links":{"self":{"href":"https://api.github.com/repos/turbokast/mythhelm/rulesets/24186707"},"html":{"href":"https://github.com/turbokast/mythhelm/rules/24186707"}},"created_at":"2026-09-29T15:20:24.522+01:00","updated_at":"2026-09-29T19:53:47.868+01:00"}]
```

The listed `Protect main` ruleset is the observed baseline. Any ruleset or tag-protection change stays an operator action.

Trigger contract for MH-7 to implement: Pushing a release tag starts the release pipeline, with publication creating `vX.Y.Z` at the target merge and the tag push matching the `v*` filter triggering the pipeline. No release artifact ships without its tag, since the pipeline requires tag context. Failure rule: a pipeline run from a malformed or unauthorized tag fails closed with no partial release, where non-matching tags never match the trigger filter and an aborted run publishes nothing.

Release scope for tagging: every published release including pre-releases gets a tag, with the dotted marker rule above; drafts are never tagged, since a draft is not a release.
