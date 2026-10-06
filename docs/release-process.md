# Release process

How MYTHHELM versions, tags and ships releases. The pipeline sections land with MH-7; this revision records only the version-numbering scheme the pipeline will implement.

## Scheme

Releases follow [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html), ratified in [ADR 0009](decisions/0009-version-numbering-scheme.md).

Example version strings: `v0.1.0` for an S1-style preview release, `v1.2.3` for a later stable release. The shapes are illustrative of the scheme; the exact tag format is owned by MH-19, whose tag format and mechanics carry this scheme.

Scope is release versions only; dev strings such as unstamped-build markers are out of scope and continue untouched. MH-7 implements the tag-triggered GoReleaser pipeline under this scheme. MH-7's spec extends this document with the pipeline sections; nothing here assumes pipeline details MH-7 has not designed.
