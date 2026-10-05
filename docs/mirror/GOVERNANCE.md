# Governance

MYTHHELM is an open-source project hosted by the [TurboKast](https://github.com/turbokast) GitHub organization.

## Roles

- **Contributors**: anyone who opens an issue, discussion or pull request.
- **Maintainers**: listed in [MAINTAINERS.md](MAINTAINERS.md). They review and merge changes, triage issues, and cut releases.
- **Lead maintainer**: breaks ties and holds release authority until the project has enough maintainers to share it.

New maintainers are nominated by an existing maintainer after sustained, high-quality contribution. Nominations are accepted by lazy consensus among maintainers.

## Decisions

- **Routine changes**: a maintainer approves and merges.
- **Significant changes**: changes to security boundaries, billing and admission, the public plugin/control protocol, user-facing configuration, or persistence formats. These need a short decision record in [`docs/decisions/`](docs/decisions/), merged via pull request, plus tests that pin the decided behaviour.
- **Disagreements**: first seek consensus in the pull request or discussion. If none emerges, the lead maintainer decides and records the reasoning.

Small fixes never need a design document.

## The free commitment

The official project, meaning the complete core, built-in adapters, safety controls, TUI, headless mode, official themes, plugin SDK/specification and local routing, is and will remain free, with no license fee, paid tier, feature gate or required account. This is a project commitment. The Apache-2.0 license still permits others to build commercial offerings.

## Releases and compatibility

The public protocol and user-facing configuration follow [Semantic Versioning](https://semver.org). Deprecations are announced in release notes, and a stated compatibility window applies before removal.

## Changes to this document

Changes to this document follow the "significant changes" process.
