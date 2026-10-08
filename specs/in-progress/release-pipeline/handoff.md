# release-pipeline — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — GoReleaser configuration for the five-pair matrix

- **Produces**: `.goreleaser.yml` as designed (build/archive id `mythhelm`, `checksums.txt`, `sboms` with `artifacts: archive` writing `<archive>.sbom.json`, `release.mode: keep-existing`); `/third_party_licenses/` in `.gitignore`.
- **Pins for Tasks 2–4**: GoReleaser CLI `v2.18.2`; `goreleaser-action` `v7.2.3` at `f06c13b6b1a9625abc9e6e439d9c05a8f2190e94`; syft `v1.54.1`.
- **For dependents**: the `sboms` stanza runs the `syft` binary, so every workflow that runs `goreleaser release` (Tasks 2 and 3) must install syft at `v1.54.1` first; `goreleaser check` (Task 4) does not need it.
- **For dependents**: `go-licenses save` must run before GoReleaser, because the archives glob `third_party_licenses/` and fail without it.
- **For dependents**: `goreleaser check` only warns on a missing `version: 2` header (v2.18.2), so do not rely on it to refuse such a config; assert the header with a grep.
- **For dependents**: the 7-day cooldown on tool pins (`docs/automation.md:47`) was not checked for the pins above, which are days old; workflow tasks must re-check pin ages at their implementation time and adopt a release at least 7 days old, recording what they chose.

## Task 2 — Tag-triggered release workflow with protected publishing

<!-- pending -->

## Task 3 — Test-tag dry-run workflow

- **Produces**: `.github/workflows/release-dry-run.yml` as designed: trigger `push.tags: ['test/*']`, job `dry-run`, no environment or secret, `timeout-minutes: 60`; uploads `dist/` as the artifact named `dist` with 7-day retention (archives, SBOMs, `checksums.txt`, `checksums.txt.sigstore.json`).
- **For dependents (Tasks 2, 5, 6)**: tool pins chosen under the 7-day cooldown on 2026-10-08: syft `v1.52.0`, cosign `v3.1.3`, go-licenses `v2.0.1`, all by `go install`; syft `v1.54.1` was too new. Task 2 should use the same install shape and re-check ages.
- **For Task 6**: the cosign verify identity is `https://github.com/<repo>/.github/workflows/release-dry-run.yml@refs/tags/test/<name>`. The workflow first runs at Task 6; if snapshot version resolution fails on the `test/*` tag, check the `GORELEASER_CURRENT_TAG` hedge.

## Task 4 — CI validation of the release config

<!-- pending -->

## Task 5 — Pipeline operator and verify documentation

<!-- pending -->

## Task 6 — Operator dry run on a test tag

<!-- pending -->
