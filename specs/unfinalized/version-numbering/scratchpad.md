# Version Numbering — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| 1 | When is 1.0.0 declared? | Undeclared — 0.x until a future stability decision. Not this spec's to set. | Maintainer, future / blocks nothing |
| 2 | What is the first release's exact tag? | MH-19 fixes the format string; the operator creates the tag per W16. The §4 justification text carries a fill-at-flip placeholder. | MH-19 + operator / blocks nothing here |

Requirements Q1–Q4 were all resolved during design (see design §5 D1–D4): ratify SemVer; ADR 0009 + release-process record; releases-only scope; MH-19 owns the format.

## Research notes

- Go major-version fit, grounded in-tree: `go.mod:1` has no `/vN` suffix while v2 dependencies in the same file carry theirs (`uax29/v2`, `go-osc52/v2`) — the module path admits only v0/v1 tags, which rules out CalVer years.
- Release-drafter already computes semver: `name-template`/`tag-template: "v$RESOLVED_VERSION"` with `semver-increment` major/minor rules (`.github/release-drafter.yml`).
- Dev strings observed 2026-10-06: `mythhelm version` → `mythhelm devel` / `commit: unknown` / `go: go1.27.1` / `platform: linux/amd64`.
- ADR template (`docs/decisions/README.md`): `# NNNN. Title`, Status/Date lines, Context/Decision/Consequences; 0001–0008 exist.
- G10 (v2 §18.3): "Public release | AT-40, AT-46 and published limitations for all advertised capabilities."

## Discoveries
