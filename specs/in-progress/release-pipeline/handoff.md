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

## Task 2 — Tag-triggered release workflow with protected publishing

<!-- pending -->

## Task 3 — Test-tag dry-run workflow

<!-- pending -->

## Task 4 — CI validation of the release config

<!-- pending -->

## Task 5 — Pipeline operator and verify documentation

<!-- pending -->

## Task 6 — Operator dry run on a test tag

<!-- pending -->
