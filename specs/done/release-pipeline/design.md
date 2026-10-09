## Release Pipeline — Design

> Implements `specs/*/release-pipeline/requirements.md` (MH-7, card MH-7 / issue #28). Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it. W-IDs refer to mythhelm-synthesis/MYTHHELM_Implementation_Plan.md. Tag format/mechanics are fixed input from ADR-0010 (`docs/decisions/0010-release-tag-discipline.md`); the version scheme from ADR-0009. Maintainer answers (2026-10-07, recorded in `refinement-log.md`): Q1 release matrix confirmed, Q2 `release` environment created with required reviewer + `v*` tag policy, Q3 `test/*` dry-run tags confirmed.

### 1. Current state

- No release pipeline exists. No `.goreleaser*` config, no release workflow, no `Makefile`, no `packaging/` (`ls .goreleaser* packaging/ Makefile` → all absent, verified 2026-10-08). `.github/workflows/` holds CI, lint, docs, CodeQL, dependency-review, zizmor and `release-drafter.yml` (notes only; triggers on push to `main`, `.github/workflows/release-drafter.yml:5-7`).
- The deferred automation rows this pipeline activates are in `docs/automation.md` (Deferred table): GoReleaser archives/checksums/SBOM, `actions/attest-build-provenance`, cosign keyless via GitHub OIDC, `go-licenses` notices — each with trigger "First release". The package-manager and macOS x64 rows stay Deferred (N1, N4; §11 H5).
- Release authority and mechanics are decided: operator publishes the release-drafter draft, which creates the tag (`docs/release-process.md:17`; ADR-0010). Tags are lightweight, `v<semver>` with dotted pre-release markers (`v0.1.0`, `v1.2.3-rc.1`), never pushed by hand. `specs/*/release-tagging/design.md:21` fixes the trigger contract MH-7 implements: the `v*` filter selects candidates, but a format check on the pushed name is the gate, because a bare glob admits malformed tags.
- Cross-compilation has no CGO blocker: `grep -rn 'import "C"' --include='*.go' .` returns nothing, and ADR-0003 records the pure-Go SQLite driver with `CGO_ENABLED=0` builds on all three OSes (`docs/decisions/0003-local-state-sqlite.md:45`; `TestBuildWithoutCgo`).
- The binary already accepts version stamping: `internal/buildinfo/buildinfo.go:9-12` documents `-X .../buildinfo.Version=v0.1.0 -X .../buildinfo.Commit=<sha>` ldflags; empty values fall back to `devel`/`unknown` (`buildinfo.go:22-50`). `mythhelm version` prints them (`internal/cli/dispatch.go:36`; grep `runVersion` in `internal/cli/dispatch.go`).
- Workflow conventions that bind every new file: SHA-pinned `uses:` with `# vX.Y.Z` comments, top-level read-only `permissions:`, `persist-credentials: false`, no secrets on PR workflows, `actionlint` + `zizmor` green (`.claude/rules/github-workflows.md`). Required checks are `CI OK`, `DCO sign-off`, `Dependency review`, `Analyze (actions)`, `Analyze (go)` (`.claude/data/required-checks.json`); this spec adds no required check. Dependabot covers `github-actions` weekly with a 7-day cooldown (`.github/dependabot.yml`); `go run` tool pins are bumped by hand (`docs/automation.md:47`).
- CI's Go matrix runs `ubuntu-latest`, `ubuntu-24.04-arm`, `macos-latest` (arm64), `windows-latest` (x64), `windows-11-arm` (`.github/workflows/ci.yml:49`); `windows-11-arm` runs `go test` without `-race` (`.github/workflows/ci.yml:65-66`). The pinned toolchain actions are `actions/checkout` v7.0.1 and `actions/setup-go` v7.0.0 (`.github/workflows/ci.yml:21-24`); `actionlint` v1.7.12 (`.github/workflows/ci.yml:28`); `go-licenses` v2.0.1 against the dependency-review allow-list (`.github/workflows/ci.yml:112-120`). `LICENSE` and `NOTICE` exist at the repo root.

### 2. Release matrix (FR-1, v2 §17.1, I14)

Five pairs, per the confirmed Q1 answer: linux/amd64, linux/arm64, darwin/arm64, windows/amd64, windows/arm64. darwin/amd64 is excluded per N4 (no macOS x64 runner exists; `docs/automation.md` keeps that Deferred row).

GoReleaser cross-compiles all five from one `ubuntu-latest` runner with `CGO_ENABLED=0` (no CGO in tree, §1). The matrix is named explicitly in `.goreleaser.yml` (`goos`/`goarch` lists plus an `ignore` entry for darwin/amd64 with a comment citing N4), so v2 §17.1's "advertise only tested subsets" holds by construction: the config is the advertisement, and the dry run (§8) builds exactly it.

### 3. Tag trigger and format gate (FR-1, ADR-0010, release-tagging §2/§4)

`.github/workflows/release.yml` triggers only on version tags:

```yaml
on:
  push:
    tags: ['v*']
```

No `branches`, no `pull_request`, no `merge_group` (AC-1.2). `test/*` tags never match the `v*` glob (AC-1.3 separation is structural, not conditional).

The first job step validates the pushed tag name against the ADR-0010 shape before anything builds (release-tagging §2: the format check is the gate, not the glob). Conservative regex, passed through `env:` and quoted per the workflows rule:

```text
^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)+)?$
```

Accepts `v0.1.0`, `v1.2.3-rc.1`, `v1.2.3-beta.2`; rejects `0.1.0`, `v1.2`, `v1.2.3-rc1` (undotted marker, invalid per project convention), `v01.2.3` (leading zero), `v1.2.3-rc..1` (empty identifier) and `v1.2.3-rc.1.` (trailing dot). A non-matching tag fails the job before any build or publish step runs (fail-closed per release-tagging §4's failure rule).

### 4. GoReleaser configuration (FR-1, FR-2)

New `.goreleaser.yml`, `version: 2` (GoReleaser v2 requires the header; for this headerless configuration, v2.18.2 warns and exits 0 with `only version: 2 configuration files are supported, yours is version: 0`; CI rejects a missing header with a grep, not `goreleaser check`). Pinned GoReleaser `~> v2` exact version re-checked at implementation time; newest seen v2.18.2 (goreleaser.com, Oct 2026). Action `goreleaser/goreleaser-action`, newest seen v7.2.3 (multiple 2026-09 pins), distribution `goreleaser` (OSS, never `goreleaser-pro`), `args: release --clean` (`--rm-dist` was renamed `--clean` in v2).

- **builds**: one build, `id: mythhelm`, `main: ./cmd/mythhelm`, `goos: [linux, darwin, windows]`, `goarch: [amd64, arm64]`, `ignore: [{goos: darwin, goarch: amd64}]` (N4, §2), `env: [CGO_ENABLED=0]`. ldflags stamp the §1 surface: `-X github.com/turbokast/mythhelm/internal/buildinfo.Version={{.Version}} -X github.com/turbokast/mythhelm/internal/buildinfo.Commit={{.FullCommit}}`.
- **archives**: default name template (`{{ .Name }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}`); `formats: [tar.gz]` with a `format_overrides` entry giving windows `formats: [zip]` (v2 `formats:` shape per appackio/apppack #190, 2026-09-17). Each archive carries `LICENSE`, `NOTICE` and the generated `third_party_licenses/` directory (§6).
- **checksum**: default `checksums.txt` (sha256).
- **sboms**: `artifacts: archive` — one SBOM document per archive (`{{ .ArtifactName }}.sbom.json`, SPDX per the Q4 default), uploaded beside the archives (AC-1.1, AC-2.2).
- **release**: `mode: keep-existing` — the operator-published release already carries the release-drafter notes with the limitations link (§7); GoReleaser uploads artifacts without touching the body (`keep-existing`/`append`/`replace` documented at goreleaser.com/customization/publish/scm/, fetched Oct 2026). `goreleaser check` plus the Task 1 snapshot run verify the keys against the pinned version; any rename is a recorded deviation, not a silent edit.
- **changelog**: skipped — the release-drafter notes are the changelog (AC-2.2); GoReleaser must not append a second one over them.

### 5. Signatures and attestations (FR-2, v2 §17.3)

Both ride GitHub OIDC; no stored keys (AC-2.1).

- **cosign keyless** runs as workflow steps after GoReleaser, not in GoReleaser's `signs` section (D3): install via `sigstore/cosign-installer` (newest seen v4.1.2), cosign release pinned (newest seen v3.0.6), then `cosign sign-blob --yes --bundle <checksums>.sigstore.json dist/checksums.txt`, then `gh release upload <tag> <bundle>` (preinstalled `gh`, no new action). v3 defaults to the self-contained `.sigstore.json` bundle; the legacy `--output-signature`/`--output-certificate` flags are silently ignored under v3 (splattner/goucrt, 2026-09), so the step uses `--bundle` only. Users verify with `cosign verify-blob --bundle … --certificate-identity-regexp … --certificate-oidc-issuer https://token.actions.githubusercontent.com` (documented in `docs/release-process.md`, §9).
- **SLSA provenance** via `actions/attest-build-provenance` (newest major v4; D4), `subject-path` naming `dist/checksums.txt` and the archives, in the same job so the subjects are the run's own artifacts. Users verify with `gh attestation verify`. Job permissions add `attestations: write` and `id-token: write` with why-comments.
- **Checksum verification** is `sha256sum -c` against `checksums.txt` (documented verify steps; AC-2.3's tamper leg: a modified archive fails the checksum, hence the attestation and signature checks bound to it).

### 6. Licence notices (FR-2, v2 §17.3, AT-46)

A workflow step before GoReleaser runs the CI-pinned tool in lockstep (`github.com/google/go-licenses/v2@v2.0.1`, same pin as `.github/workflows/ci.yml:120`):

```sh
go-licenses save ./cmd/mythhelm --save_path=third_party_licenses
```

`./cmd/mythhelm` scopes notices to the shipped binary's graph (D5); the CI `check ./... --include_tests` allow-list stays the superset gate. GoReleaser `archives.files` ships `LICENSE`, `NOTICE`, `third_party_licenses/` in every archive (AC-2.2). The generated directory is gitignored (`/third_party_licenses/` in `.gitignore`, beside the existing `/dist/` entry).

### 7. Protected publishing (FR-3, v2 §§8.2, 17.3)

- Single job (`release`) on `ubuntu-latest`: checkout (`fetch-depth: 0` for tags, `persist-credentials: false`), setup-go, tag validation (§3), notices (§6), GoReleaser, cosign sign + upload, attest. One job keeps `dist/` local — no artifact passing between jobs.
- `environment: release` on the job (the operator-created environment with required reviewer + `v*` tag policy, Q2 answer). `GITHUB_TOKEN` (contents: write) is the only publishing credential and exists only in this job; no other job in either new workflow touches a secret (AC-3.1). If the environment or its approvals are missing, GitHub stops the job before any step runs — publishing is unreachable without the gate (AC-3.2; v2 §8.2 standing-grant semantics: missing authority blocks the dependent work).
- Top-level `permissions: {}`; the job declares `contents: write` (upload artifacts), `id-token: write` (OIDC for cosign + attest), `attestations: write`, each with a why-comment.
- `timeout-minutes: 60` on the job (NFR-1 default). No custom runners, no paid service, no new secret (NFR-2; I13).
- **Limitations link** (AC-3.3): a footer in `.github/release-drafter.yml`'s `template:` linking `docs/limitations.md`, so every generated draft — hence every published release — carries it. GoReleaser's `keep-existing` mode (§4) preserves it. Release-drafter regenerates the open draft on the next push to `main` after the template merges.

### 8. Dry-run workflow (AC-1.3, G10)

Separate `.github/workflows/release-dry-run.yml` triggered only by operator-owned test tags:

```yaml
on:
  push:
    tags: ['test/*']
```

Separate file because AC-1.2 requires the release workflow to trigger only on version tags (D2). Permissions mirror §7's least-privilege shape: top-level `permissions: {}`, and the single job declares `contents: read` (checkout only) and `id-token: write` (mints the OIDC token for the cosign keyless roundtrip; grants no repository write), each with a why-comment. No `environment:`, no secret, and no `contents: write`, `attestations: write` or `packages: write` anywhere — `GITHUB_TOKEN` is read-only by construction. The job builds with `goreleaser release --snapshot --clean` (snapshot skips publishing), asserts the five archives plus checksums plus per-archive SBOMs exist in `dist/`, unpacks one archive and checks `mythhelm version` reports the snapshot version and the notices are inside. It additionally runs the cosign sign + `verify-blob` roundtrip over `dist/checksums.txt` (keyless OIDC works on any tag; proves the §5 signing path without publishing). The attest step does not run here (attestations would pollute the repo's attestation store for throwaway artifacts; honesty register H2). The last step uploads `dist/` (archives, checksums, SBOMs and the `.sigstore.json` bundle) via pinned `actions/upload-artifact` with `retention-days: 7`, so the Task 6 operator downloads exactly what the run built; the upload is short-lived verification evidence, never a release channel.

Operator procedure: push a `test/*` tag at the PR head commit; the workflow runs from the tagged commit, proving the changed config before merge. `test/*` tags never match `v*`, so the dry run can never publish (AC-1.3, Q3 answer).

### 9. CI validation and docs

- **Config validation**: the CI `workflows` job gains pinned steps that generate notices (§6 command) and run `goreleaser check` via the same pinned action + CLI version as release.yml (D6). A broken config fails on the PR, before any dry run. `actionlint` already lints every workflow file including the two new ones (`.github/workflows/ci.yml:28`); `zizmor` audits them in CI.
- **`docs/automation.md`**: move the GoReleaser, attest-build-provenance, cosign keyless and go-licenses-notices rows from Deferred to Active with trigger "tag `v*` (+ `test/*` dry run)" and configured-in `.github/workflows/release.yml` / `.goreleaser.yml`; reword the "Publishing credentials will live only…" principle to present tense. Package-manager and macOS-x64 rows stay Deferred (N1, N4).
- **`docs/release-process.md`**: new `## Pipeline` section (MH-7's half of the joint home; the intro already promises "The pipeline sections land with MH-7"): trigger + matrix, operator publish steps, user install + verify commands (checksum, `gh attestation verify`, `cosign verify-blob`), dry-run procedure, environment settings record. Also flips the intro sentence that says only the scheme is recorded.

### 10. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | GoReleaser OSS v2 (`version: 2` config), exact CLI pin + pinned action, re-checked at implementation (newest seen v2.18.2 / action v7.2.3) | v2 is the current line; v1 configs are refused. Exact pins follow the repo's pinned-everything rule; Dependabot covers the action, the CLI pin is bumped by hand like the `go run` tools. Rejected: `~> v2` float (unreproducible) and Pro distribution (paid; NFR-2). |
| D2 | Dry run is a separate workflow on `test/*`, not a conditional job in release.yml | AC-1.2 says the release workflow triggers only on version tags — a shared file would violate it literally. Structural separation also makes "dry run can never publish" reviewable at the trigger level. Rejected: shared workflow with `if:` gates (one miswritten condition away from publishing). |
| D3 | cosign as workflow steps (`sign-blob --bundle` + `gh release upload`), not GoReleaser `signs` | cosign v3 ignores GoReleaser's `--output-signature`/`--output-certificate` template flow (bundle-only); the workflow step uses the same `--bundle` CLI users run to verify, and `gh` needs no new action. Rejected: `signs` section (cleaner uploads, but v3 interplay unverified). |
| D4 | `actions/attest-build-provenance` (latest v4.x at implementation), not `actions/attest` | Requirements Context and the Deferred row name this action; it is maintained as a wrapper. Rejected: `actions/attest` (upstream's "new implementations" note acknowledged in scratchpad; switching would re-decide fixed input). |
| D5 | Notices from `go-licenses save ./cmd/mythhelm` (binary graph), no CSV report | Ships exactly what the binary links; CI's `./... --include_tests` check stays the superset gate. A CSV `report` adds an unread machine file; the SBOM already inventories modules. Rejected: `./...` (test-only dep noise) and report-with-template (template to maintain for no reader). |
| D6 | `goreleaser check` runs in the CI `workflows` job via the same pins as release.yml | Catches config breakage on the PR instead of at dry-run time. Same-pin lockstep asserted by Task 4's acceptance. Rejected: check only in dry-run (later signal) and a new action (reuse the release action). |
| D7 | Single-job release workflow on `ubuntu-latest`; GoReleaser cross-compiles | No matrix needed (pure Go, §1); one job keeps `dist/` local for sign + attest with no artifact passing. Rejected: per-OS build jobs (slower, needs artifact fan-in, gains nothing for a static binary). |
| D8 | No new ADR for the pipeline mechanics | ADR-0009/0010 already decide scheme and discipline; action choices and pins are implementation detail recorded here and bumped routinely — an ADR per pin would churn. Rejected: pipeline ADR (would also collide with `specs/*/qualification-registry/` Task 8's 0011 numbering). |
| D9 | SPDX SBOM via GoReleaser `sboms: [{artifacts: archive}]` | The Q4 default; per-archive documents satisfy AC-1.1/AC-2.2 directly. Rejected: CycloneDX (no consumer requires it). |

### 11. Honesty register

| ID | Spec demand | Position |
|---|---|---|
| H1 | G10 proven pipeline (test-tag dry run, DoD) | Proven by Task 6 (operator dry run) before this spec finalizes; until then the workflows are statically checked only. |
| H2 | v2 §17.3 signed/attested artifacts on a real release | The full publish path (upload + attest to a published release) is exercised only by the first real release — an operator action explicitly out of scope (N2). The dry run proves build + sign roundtrip; attest-to-release is statically configured and documented. |
| H3 | AT-46 zero-credential contribution evidence | This spec changes no contributor path; contributor checks already run credential-free (CI). The release itself adds no contributor requirement (NFR-2). |
| H4 | v2 §17.3 reproducible-build instructions | Deferred: GoReleaser builds are version-pinned and re-runnable, but byte-reproducibility is not claimed or tested here. |
| H5 | Package managers (N1), macOS x64 (N4) | Out of scope; Deferred rows stay. Archives are the baseline per §17.3. |
| H6 | `release.mode: keep-existing` preserving draft notes | Configured and `goreleaser check`-valid; behaviorally confirmed only at the first real release (N2). If it misbehaves, the fix is a config-only follow-up. |
| H7 | §7 ordering: GoReleaser publishes before cosign sign + upload and attest run | Known gap: a late-step failure strands a published release without bundle/attestation (violating AC-2.1), and `gh release upload` without `--clobber` fails the re-run after partial success. Release-tagging §4's "aborted run publishes nothing" holds only for pre-publish aborts. Recovery is the Task 5 re-run procedure (`--clobber` re-upload, re-attest). |

### 12. Cross-spec references

- Builds on: `specs/*/version-numbering/` (MH-18, done) — ADR-0009 scheme; `specs/*/release-tagging/` (MH-19, done) — ADR-0010 discipline and the trigger contract (§3 cites its design §2/§4).
- Feeds into: the first real release (operator action, N2); package-manager publishing (future card, N1).
- Conflicts: none. `specs/in-progress/qualification-registry/` touches `internal/*`, `tests/e2e/`, `adapters/claudecode/*`, `docs/user-guide.md`, `docs/decisions/0011-*` (Tasks 1–9 Files lists) — disjoint from `.github/`, `.goreleaser.yml`, `.gitignore`, `docs/automation.md`, `docs/release-process.md`. `specs/todo/`, `specs/unfinalized/`, `specs/unrefined/` are empty (verified 2026-10-08 via `ls`). No new ADR, so no decisions-numbering collision (D8).
