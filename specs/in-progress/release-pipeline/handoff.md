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

- **Produces**: `.github/workflows/release.yml` — trigger `push.tags: ['v*']`; single job `release` with `environment: release`, `timeout-minutes: 60`, `contents`/`id-token`/`attestations: write`; cosign bundle `dist/checksums.txt.sigstore.json` uploaded with `gh release upload`; attestations over `dist/checksums.txt`, `dist/*.tar.gz`, `dist/*.zip`. Also the `docs/limitations.md` footer in `.github/release-drafter.yml` and the four Active rows in `docs/automation.md`.
- **Pins for Tasks 3–4**: GoReleaser `v2.18.2`; goreleaser-action `v7.2.3` (`f06c13b6…`); cosign-installer `v4.1.2` (`6f9f17788090df1f26f669e9d70d6ae9567deba6`) with `cosign-release: v3.1.3`; syft `v1.52.0` (v1.54.x was inside the 7-day cooldown on 2026-10-08).
- **For dependents**: Task 3 installs syft the same way (`go install github.com/anchore/syft/cmd/syft@v1.52.0`, then add `$(go env GOPATH)/bin` to `$GITHUB_PATH`) and runs the go-licenses step before GoReleaser. Task 4 needs the notices step only.
- **For dependents**: the dry run needs the same cosign-installer pin; Task 3 should reuse it rather than re-resolve.
- **For Task 5**: the bundle is `checksums.txt.sigstore.json`; the re-run procedure re-uploads it with `gh release upload --clobber` and re-attests the same three subject globs.


## Task 3 — Test-tag dry-run workflow

<!-- pending -->

## Task 4 — CI validation of the release config

- **Produces**: three steps in the CI `workflows` job (`.github/workflows/ci.yml`): go-licenses `save`, a `version: 2` header grep, and `goreleaser check` via the Task 1 action and CLI pins. No new job; the `CI OK` needs list is unchanged.
- **For dependents**: the job's `setup-go` now reads `go-version-file: go.mod`. Task 5 can describe CI as validating the config on every PR; the header is enforced by the grep because `goreleaser check` only warns.

## Task 5 — Pipeline operator and verify documentation

<!-- pending -->

## Task 6 — Operator dry run on a test tag

<!-- pending -->
