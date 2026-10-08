## Release Pipeline — Tasks

### Dependencies

- Prerequisite specs: `specs/*/version-numbering/` (done; ADR-0009 scheme) and `specs/*/release-tagging/` (done; ADR-0010 discipline + the trigger contract in design §2/§4 that Task 2 implements). No other spec touches `.github/workflows/release*.yml`, `.goreleaser.yml`, `docs/automation.md` or `docs/release-process.md` (`specs/*/qualification-registry/` Tasks 1–9 Files lists are `internal/*`, `tests/e2e/`, `adapters/claudecode/*`, `docs/user-guide.md`, `docs/decisions/0011-*`).
- Order: Task 1 first (the config contract). Tasks 2, 3 and 4 after Task 1 with disjoint Files — they may run in parallel. Task 5 after Tasks 2–4 (it documents the finished mechanism). Task 6 (operator dry run) after Task 5 (it also validates the documented verify steps).
- **Gates for every task.** No Go code changes, so `go vet`/`go test`/`golangci-lint` are skipped with that reason. Every task: `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12` over the touched workflows (same pin as `.github/workflows/ci.yml:28`), the PR's `zizmor` job green, and `scripts/ci/check-public-hygiene.sh`. Files existing or an agent reporting success is not completion; cite the runner output.
- **Completion convention.** Append ` ✅ COMPLETED` to the heading, keep every original field, and add `Status` (`✅ Completed — …; PR #<n>`), `Implementation` (at most 3 lines, plus the commit SHA; Task 1 additionally records the exact GoReleaser CLI + action versions chosen, which Tasks 2–4 pin against), `Spec deviations` (`None`, or each deviation with its reason) and `Files modified`.
- **Commits.** Every commit is signed off (`git commit -s`, DCO). Version pins respect the 7-day cooldown (a release newer than 7 days is skipped for the newest older one; the entry says so). No new ADR: no billing, persistence or process-ownership change (design D8).

---

## Implementation Tasks

### Task 1 — GoReleaser configuration for the five-pair matrix ✅ COMPLETED

- **Domain/agent**: release-engineer
- **Budget**: complex (cross-platform behaviour: five OS/arch pairs from one runner)
- **Change**: Add `.goreleaser.yml` (design §2/§4) building the confirmed matrix with checksums, per-archive SPDX SBOMs and licence-notice payloads, so Tasks 2–4 have a validated config to run.
- **Files**:
  - `.goreleaser.yml` (version 2; builds, archives, checksum, sboms, release)
  - `.gitignore` (ignore the generated notices directory)
- **Produces**: `.goreleaser.yml` contract — build `id: mythhelm`, `main: ./cmd/mythhelm`; archives `{{ .Name }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}` (tar.gz, zip on windows) containing the binary, `LICENSE`, `NOTICE`, `third_party_licenses/`; `checksums.txt`; `<archive>.sbom.json` (SPDX); ldflags `-X …/buildinfo.Version={{.Version}} -X …/buildinfo.Commit={{.FullCommit}}`; `release.mode: keep-existing`. Plus the exact GoReleaser CLI + `goreleaser-action` versions recorded in the completion entry (newest seen v2.18.2 / v7.2.3, re-checked at implementation; OSS `goreleaser` distribution, never Pro).
- **Acceptance**:
  - `go install github.com/goreleaser/goreleaser/v2@<pin>` (documented install path) then `goreleaser check` exits 0; a config with the `version: 2` header removed still exits 0 but prints the `only version: 2 configuration files are supported, yours is version: 0` warning (measured on v2.18.2 — the header is warned on, not refused).
  - After `go run github.com/google/go-licenses/v2@v2.0.1 save ./cmd/mythhelm --save_path=third_party_licenses` (Task 1 uses the CI pin from `.github/workflows/ci.yml:120`), `goreleaser release --snapshot --clean --skip=publish` exits 0 and `dist/` contains the five required archives for linux/amd64, linux/arm64, darwin/arm64, windows/amd64 and windows/arm64, plus `checksums.txt` and five `.sbom.json` files; GoReleaser metadata such as `artifacts.json` is allowed; `ls dist/ | grep -c darwin_amd64` prints 0 (N4 exclusion) while deleting the `ignore` entry would produce that pair.
  - Unpacking the linux/amd64 archive yields the `mythhelm` binary, `LICENSE`, `NOTICE` and a non-empty `third_party_licenses/`; `./mythhelm version` prints a version containing `SNAPSHOT` (an unstamped build would print `devel`).
  - Every `dist/*.sbom.json` contains `spdxVersion` (SPDX per design D9); `sha256sum -c dist/checksums.txt` passes from `dist/`.
  - `.gitignore` contains `/third_party_licenses/` (beside `/dist/`), and after the snapshot run `git status --porcelain -- third_party_licenses/ dist/` prints nothing.
  - The config names the matrix explicitly with the darwin/amd64 `ignore` citing N4, `CGO_ENABLED=0`, the `buildinfo` ldflags paths, `sboms: [{artifacts: archive}]` and `release.mode: keep-existing` — each anchored by grep; removing any fails its check.
- **Test plan**: Local runs only (no workflow dispatch — dispatch is a publishing action). Commands above, run in order; paste the `dist/` listing and the `version` output in the entry.
- **Invariants touched**: I14 (v2 §17.1: the matrix names exactly the five supported pairs, darwin/amd64 excluded as untested); I13 (v2 §1.2 UR-01: OSS distribution, no paid tool).
- **Status**: ✅ Completed — `.goreleaser.yml` builds the five-pair matrix with checksums, per-archive SPDX SBOMs and licence payloads, and the local snapshot run passes every acceptance bullet; PR #216.
- **Implementation**: Pins: GoReleaser CLI `v2.18.2` (OSS, `go install github.com/goreleaser/goreleaser/v2@v2.18.2`), `goreleaser-action` `v7.2.3` (`f06c13b6b1a9625abc9e6e439d9c05a8f2190e94`), syft `v1.54.1` (`go install github.com/anchore/syft/cmd/syft@v1.54.1`; the `sboms` stanza shells out to `syft`, so Tasks 2 and 3 workflows must install it at this pin). The 7-day cooldown on these releases could not be checked from this session (release dates unreachable), so the pins are the newest tags seen. Snapshot run, `dist/`: five archives (`mythhelm_0.0.0-SNAPSHOT-59eed44_{linux_amd64,linux_arm64,darwin_arm64}.tar.gz`, `…_{windows_amd64,windows_arm64}.zip`), five `.sbom.json`, `checksums.txt`, `artifacts.json`, `metadata.json`, `config.yaml`; `grep -c darwin_amd64` 0; `sha256sum -c` all OK; `./mythhelm version` printed `mythhelm 0.0.0-SNAPSHOT-59eed44` / `go: go1.27.1` / `platform: linux/amd64`. Commits 25a0f28, 59eed44 and the completion commit on this branch.
- **Spec deviations**: Acceptance bullet 1 reworded per the orchestrator's binding decision: GoReleaser v2.18.2 exits 0 on a config without the `version: 2` header and only warns (`only version: 2 configuration files are supported, yours is version: 0`), so the bullet now asserts that measured behaviour instead of failure. `design.md` still says a v1-style config is refused; that sentence is stale for v2.18.2 and is outside this task's Files, left for the orchestrator. `scratchpad.md` also gained a Discoveries note about the same finding (added by an earlier attempt).
- **Files modified**: `.goreleaser.yml`, `.gitignore`, `specs/in-progress/release-pipeline/tasks.md`, `specs/in-progress/release-pipeline/handoff.md`, `specs/in-progress/release-pipeline/scratchpad.md`.

### Task 2 — Tag-triggered release workflow with protected publishing ✅ COMPLETED

- **Domain/agent**: release-engineer
- **Budget**: complex (security primitives: OIDC keyless signing, publishing credentials, protected environment)
- **Depends on**: Task 1
- **Change**: Add `.github/workflows/release.yml` (design §3/§5/§7) that validates the tag, builds and publishes from the `release` environment only, and wire the limitations-link footer plus the automation-table activation, so version tags ship verifiable releases and nothing else can publish.
- **Files**:
  - `.github/workflows/release.yml` (tag trigger, validation, notices, GoReleaser, cosign, attest)
  - `.github/release-drafter.yml` (limitations-link footer in `template:`)
  - `docs/automation.md` (activate the four release rows; principle to present tense)
- **Produces**: The publish contract Task 6 exercises — trigger `push.tags: ['v*']`, single job `release` with `environment: release`, `timeout-minutes: 60`, permissions `contents: write` + `id-token: write` + `attestations: write`; cosign bundle `<checksums>.sigstore.json` uploaded to the release; SLSA attestations over checksums + archives.
- **Acceptance**:
  - `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12` passes on the new workflow, and the PR's `zizmor` job is green.
  - The `on:` block triggers on `push.tags: ['v*']` only: `sed -n '/^on:/,/^permissions:/p' .github/workflows/release.yml | grep -E 'branches|pull_request|merge_group|test/'` prints nothing, while it prints the `tags:` line (AC-1.2; adding a `branches:` line fails the check).
  - Every `uses:` line matches `@[0-9a-f]{40} # v` (40-char SHA + version comment); the `goreleaser-action` pin and its `version:` input equal the versions in Task 1's completion entry (a tag ref or a drifted CLI version fails the check).
  - The job sets `environment: release`, `timeout-minutes: 60`, `contents: write`, `id-token: write`, `attestations: write` (each with a why-comment); top-level `permissions:` is `{}`; checkout sets `fetch-depth: 0` and `persist-credentials: false` (each anchored; deleting any fails its grep).
  - The validation step carries the design §3 regex and fails closed: `printf '%s\n' v0.1.0 v1.2.3-rc.1 v1.2.3-beta.2 | grep -E "^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)+)?$"` matches all three, while `printf '%s\n' 0.1.0 v1.2 v1.2.3-rc1 test/x malicious v01.2.3 v1.2.3-rc..1 v1.2.3-rc.1. | grep -E …` matches none; the step sits before every build/publish step — the validation step's line number is less than the GoReleaser, cosign and attest steps' line numbers (compare `grep -n` outputs, cited in the entry; moving the validation step below the build fails the check).
  - No secret is reachable outside the publish job: the file's only `secrets.` references are `secrets.GITHUB_TOKEN` inside the `release` job, and no `pull_request*` trigger exists (AC-3.1; `grep -n 'secrets\.'` output is cited in full in the entry).
  - The cosign steps use `--bundle` only (no `--output-signature`/`--output-certificate`, silently ignored under cosign v3): grep for the legacy flags prints nothing.
  - The notices step runs `go-licenses save ./cmd/mythhelm --save_path=third_party_licenses` at the CI pin (v2.0.1) before the GoReleaser step (design §6; each anchored by grep — deleting the step or changing the binary path fails its check).
  - The cosign bundle is uploaded to the release with `gh release upload <tag> <bundle>` (design §5; the `Produces` upload leg — deleting the upload step fails its grep).
  - The attest step's `subject-path` names both `dist/checksums.txt` and the release archives (design §5; the `Produces` attestation leg per the pinned action's multi-subject shape in `scratchpad.md` Q3 — a subject-path covering only one fails its grep).
  - `.github/release-drafter.yml`'s `template:` contains a `docs/limitations.md` link (AC-3.3; deleting it fails the grep); GoReleaser's `keep-existing` mode is untouched by this task.
  - `docs/automation.md`: the GoReleaser, attest-build-provenance, cosign keyless and go-licenses-notices rows appear under Active with the tag trigger and the new workflow/config named; the Deferred table no longer lists them; the Homebrew and macOS-x64 rows stay Deferred; the fork-PR principle reads in the present tense. Each leg anchored; the pre-change wording fails the new greps.
- **Test plan**: Static checks above + `actionlint` + the PR's CI (`zizmor`, `Docs checks` for the release-drafter edit). Behavioural proof is Task 6; the first real release (operator, N2) proves upload + attest-to-release.
- **Invariants touched**: I03 (v2 §8.3: tag text cannot grant publishing — the format gate + environment approval stand between a tag and publish); I13 (no paid service, no new secret — NFR-2).
- **Status**: ✅ Completed — `release.yml` validates the tag, then builds, signs, uploads and attests only from the `release` environment; the limitations footer and the four automation rows are wired; PR #217.
- **Implementation**: Single job on `v*` tags; tag-format step first, then notices, syft, GoReleaser, cosign `--bundle` sign, `gh release upload`, attest over checksums + archives. Pins: cosign-installer v4.1.2 (`6f9f1778`), cosign CLI v3.1.3, attest-build-provenance v4.2.2 (`4d101475`), syft v1.52.0. Static checks all pass; `grep -n 'secrets\.'` shows only `secrets.GITHUB_TOKEN` at the GoReleaser step (`GITHUB_TOKEN`) and the upload step (`GH_TOKEN`), both in the `release` job. Commit 15529cc.
- **Spec deviations**:
  - syft install step added before GoReleaser: the Task 1 hand-off found the `sboms` stanza shells out to `syft`, which the spec predates.
  - syft pinned at v1.52.0 (2026-09-17), not the hand-off's v1.54.1: v1.54.1 (2026-10-06) and v1.54.0 (2026-10-01) are inside the 7-day cooldown.
  - Pin-age re-check done as the hand-off requires: GoReleaser v2.18.2 (2026-09-17), goreleaser-action v7.2.3 (2026-06-27), cosign-installer v4.1.2 (2026-05-06), cosign v3.1.3 (2026-08-05), attest v4.2.2 (2026-08-06) are all older than 7 days. Dates come from the Go module proxy and tag commit dates.
- **Files modified**: `.github/workflows/release.yml`, `.github/release-drafter.yml`, `docs/automation.md`, `specs/in-progress/release-pipeline/tasks.md`, `specs/in-progress/release-pipeline/handoff.md`.

### Task 3 — Test-tag dry-run workflow ✅ COMPLETED

- **Domain/agent**: release-engineer
- **Budget**: standard
- **Depends on**: Task 1
- **Change**: Add `.github/workflows/release-dry-run.yml` (design §8) so an operator-owned `test/*` tag proves the archives still build — including the cosign roundtrip — without any publish path.
- **Files**:
  - `.github/workflows/release-dry-run.yml` (test-tag trigger, snapshot build, cosign roundtrip, `dist/` artifact upload)
- **Produces**: The dry-run contract Task 6 exercises — trigger `push.tags: ['test/*']`, single job with no `environment`, top-level `permissions: {}` plus job-level `contents: read` + `id-token: write`, `timeout-minutes: 60`; snapshot build of the five-pair matrix, cosign sign + `verify-blob` roundtrip over `dist/checksums.txt`, short-lived `dist/` artifact upload; no publish path.
- **Acceptance**:
  - `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12` passes on the new workflow, and the PR's `zizmor` job is green.
  - The `on:` block triggers on `push.tags: ['test/*']` only: the `sed`/`grep` pair from Task 2 adapted to this file prints the `test/*` line and nothing matching `branches|pull_request|merge_group|'v\*'|"v\*"`.
  - The file contains no `environment:` key, no `secrets.` reference at all, and no `release upload` / `attest` / `publish` step: `grep -nE 'environment:|secrets\.|gh release upload|attest|skip *= *publish' .github/workflows/release-dry-run.yml` prints nothing except the `--skip=publish` snapshot flag (adding an upload step fails the check).
  - Top-level `permissions:` is `{}`; the job declares `contents: read` and `id-token: write` (each with a why-comment); no `contents: write`, `attestations: write` or `packages: write` appears anywhere (each anchored; adding any fails its grep).
  - Every `uses:` line matches `@[0-9a-f]{40} # v`; the `goreleaser-action` pin and `version:` input equal Task 1's recorded versions; the job sets `timeout-minutes: 60`.
  - The build steps run `goreleaser release --snapshot --clean`, assert five archives + `checksums.txt` + five `.sbom.json` in `dist/`, unpack one archive and check `version` output plus the notices payload, then run `cosign sign-blob --bundle` and `cosign verify-blob --bundle` over `dist/checksums.txt` (each step anchored by grep; deleting the verify step fails its check).
  - The last step uploads `dist/` via pinned `actions/upload-artifact` with `retention-days: 7` (anchored; deleting the step fails its grep — the short-lived artifact Task 6 downloads).
- **Test plan**: Static checks + `actionlint` + the PR's CI. The workflow itself runs in Task 6 (operator pushes the first `test/*` tag); this task only lands the file.
- **Invariants touched**: I03 (v2 §8.3: the dry run holds no publishing authority by construction — no environment, no secret).
- **Status**: ✅ Completed — `.github/workflows/release-dry-run.yml` runs a snapshot build, matrix and payload assertions and a cosign sign/verify roundtrip on `test/*` tags, with no publish path; PR #218.
- **Implementation**: Single job, top-level `permissions: {}`, job `contents: read` + `id-token: write`; syft, go-licenses and cosign are installed with `go install` at pinned versions, then goreleaser-action `v7.2.3` (`f06c13b6…`) at CLI `v2.18.2`. Static checks only: `actionlint@v1.7.12` exit 0, anchored greps clean, hygiene gate pass. Commit a81bd62.
- **Spec deviations**:
  - syft is `v1.52.0`, not Task 1's `v1.54.1`: the hand-off requires a syft install step and a pin-age re-check, and v1.54.1 (2026-10-06) is under the 7-day cooldown; v1.52.0 (2026-09-17) is the newest release at least 7 days old (v1.53.0 and v1.54.0 were 6d23h and 6d19h old on 2026-10-08).
  - cosign is installed via `go install github.com/sigstore/cosign/v3/cmd/cosign@v3.1.3` (2026-08-05), not `cosign-installer` at v3.0.6: the installer SHA cannot be resolved from this session (repo outside its scope) and pins are never taken from memory. Task 2 needs the same decision.
  - The goreleaser-action `v7.2.3` age was not checkable here (repo outside session scope); the pin is kept as Task 1 recorded it. GoReleaser `v2.18.2` is 21 days old.
  - Added `GORELEASER_CURRENT_TAG=v0.0.0` so a non-semver `test/*` tag cannot break snapshot version resolution; unverified until Task 6.
- **Files modified**: `.github/workflows/release-dry-run.yml`, `specs/in-progress/release-pipeline/tasks.md`, `specs/in-progress/release-pipeline/handoff.md`.

### Task 4 — CI validation of the release config

- **Domain/agent**: release-engineer
- **Budget**: standard
- **Depends on**: Task 1
- **Change**: Extend the CI `workflows` job with pinned notices-generation + `goreleaser check` steps (design §9), so a broken release config fails on the PR before any dry run.
- **Files**:
  - `.github/workflows/ci.yml` (`workflows` job only; no new job, no ruleset change)
- **Acceptance**:
  - The `workflows` job contains a `go-licenses save ./cmd/mythhelm` step at the CI pin (v2.0.1, same line shape as the `licenses` job) followed by a `goreleaser check` step via the Task 1 action pin + CLI version (each anchored; deleting either fails its grep).
  - Every added `uses:` line matches `@[0-9a-f]{40} # v`; no new job is added (`grep -c '^  [a-z-]*:$' .github/workflows/ci.yml` is unchanged from `main` — the `CI OK` needs list is untouched, so no maintainer ruleset update is needed).
  - `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12` passes; the PR's full CI (`CI OK`, `zizmor`) is green, with the new steps visible in the `Lint workflows` job log (paste the job URL in the entry).
  - Red check: temporarily breaking `.goreleaser.yml` (e.g. deleting `version: 2`) in a scratch checkout makes the new steps fail — verified by running the same two commands locally, cited in the entry (a check that cannot fail proves nothing).
- **Test plan**: Static greps + local red/green run of the two commands + the PR's CI.
- **Invariants touched**: None (CI-only: no shipped behaviour changes; the release jobs keep I03/I13 per Task 2).

### Task 5 — Pipeline operator and verify documentation

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 2, Task 3, Task 4
- **Change**: Add the `## Pipeline` section to `docs/release-process.md` (design §9) with the operator publish steps and the user install + verify commands, so the shipped pipeline is described where operators and users look.
- **Files**:
  - `docs/release-process.md` (new `## Pipeline` section + intro flip)
- **Acceptance**:
  - A `## Pipeline` section exists stating the `v*` trigger, the five-pair matrix with the darwin/amd64 exclusion, and the operator publish flow (`gh release edit … --draft=false`, never `git tag` by hand) — each anchored to the section via `sed -n '/^## Pipeline/,/^## /p'`; deleting any fails its grep.
  - The section documents the three verify commands against a release's assets — `sha256sum -c` over `checksums.txt`, `gh attestation verify`, and `cosign verify-blob --bundle` with `--certificate-oidc-issuer https://token.actions.githubusercontent.com` — plus the tamper leg (a modified archive fails verification, AC-2.3); each command anchored to the section.
  - The section documents the `test/*` dry-run procedure (operator pushes the tag at the PR head; the run builds without publishing) and the `release` environment's required settings (required reviewer + `v*` tag policy, recorded as observed Q2 state, unchanged by this task).
  - The section documents the re-run procedure after a late-step failure (design honesty register H7): re-upload the cosign bundle with `gh release upload --clobber` and re-attest the release subjects — each leg anchored to the section via `sed -n '/^## Pipeline/,/^## /p'`; deleting either fails its grep.
  - The intro sentence claiming only the scheme is recorded is reworded to include the pipeline; the `## Scheme` and `## Tag discipline` sections are byte-untouched (`git diff` shows added lines only outside them).
  - `scripts/ci/check-public-hygiene.sh` passes; the PR's `Docs checks` job is green.
- **Test plan**: Anchored greps + hygiene script + the PR's docs job. No Go test asserts prose; each item cites the command and its pre-change failure (absent section).
- **Invariants touched**: None (prose only; I09/I14 compliance lives in the mechanism tasks — the matrix and `unknown` renderings this section describes).

### Task 6 — Operator dry run on a test tag

- **Domain/agent**: maintainer
- **Budget**: standard
- **Depends on**: Task 5
- **Change**: The operator pushes a `test/*` tag at the merged head and observes the dry-run workflow green, proving the pipeline end to end without publishing a real release — except attest-to-release and `keep-existing` behaviour, which only the first real release exercises (design H2/H6; N2) (G10, AC-1.3, DoD).
- **Files**: None (operator-run; no tree changes — run URLs and observed outputs go in the completion entry)
- **Acceptance**:
  - With Tasks 1–5 merged, the operator pushes `test/<date>-mh7` at `origin/main` and the `release-dry-run.yml` run for that tag is green (paste the run URL; a red run fails the task and files the fix, not a waiver).
  - The run log shows five archives + `checksums.txt` + five `.sbom.json` built, the unpacked `mythhelm version` reporting a `test/*`-derived version (not `devel`), the notices payload present, and the cosign sign + `verify-blob` roundtrip passing (cite the log lines).
  - The operator downloads the run's short-lived `dist` artifact (Task 3 upload; never from a release — no release exists) and follows the Task 5 verify steps scoped to the checksum + cosign-bundle checks plus the tamper leg on a locally modified copy; attestation verification is explicitly excluded (attest does not run in the dry run, design honesty register H2). Each included check behaves as documented.
  - No release, draft or tag mutation results: `gh release list` shows no new entry and the `release.yml` workflow has no run for the `test/*` tag (AC-1.2 separation observed, not just configured).
- **Test plan**: Operator-executed; the entry records the tag name, run URL, and the observed log lines for each leg. On failure the operator deletes the test tag and the fix goes through a normal PR + a fresh test tag.
- **Invariants touched**: None (observation only; the pipeline under test keeps I03/I13/I14 per Tasks 1–3).
