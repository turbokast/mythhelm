# Project automation

Every automated check, bot and housekeeping job in this repository, what it does, and when the deferred ones switch on. Keep this table current when a workflow or integration is added, changed or removed.

## Principles

- **Nothing requires a paid service.** Every automation here runs on GitHub's free features for public repositories, open-source tools inside GitHub Actions, or a SaaS free plan for open source. If an integration stops being free, it is removed, not paid for.
- **Third-party SaaS stays advisory.** AI reviewers and other hosted services may comment, but they are never a required status check and never approve or block a merge on their own. Required checks run in GitHub Actions from pinned, auditable code.
- **Fork pull requests get no secrets.** Workflows that run PR code use `pull_request` with a read-only token. `pull_request_target` is used only by jobs that never check out or execute PR code (labelling, greeting). Publishing credentials live only in tag-triggered, environment-protected release jobs.
- **Least privilege, pinned.** Top-level `permissions` is read-only or empty; each job asks for what it needs with a comment saying why. Every action is pinned to a full commit SHA with a `# vX.Y.Z` comment, and [zizmor](https://docs.zizmor.sh/) audits the workflows on every change.
- **Merge-queue ready.** Every workflow that produces a required check also triggers on `merge_group`. PR-only checks skip in the queue, and GitHub counts a skipped required check as passing.

## Tiers

- **GitHub-native**: GitHub features and first-party actions (`actions/*`, `github/*`).
- **OSS-in-Actions**: open-source tools run inside our own GitHub Actions workflows.
- **Free-plan SaaS (advisory)**: hosted services on a free open-source plan. Advisory only.

## Active

| Automation | What it does | Trigger | Configured in | Tier |
|---|---|---|---|---|
| CI (`CI OK`, required) | actionlint on all workflows; once `go.mod` exists, `gofmt`, `go vet`, `go test -race` and `go mod tidy -diff` on the runner matrix below, plus the `golangci-lint`, `govulncheck` and `go-licenses` jobs; the harness self-tests (shellcheck, hook and script tests, the agent-harness lint, public hygiene). `CI OK` aggregates the jobs. | push to `main`, PR, merge queue | `.github/workflows/ci.yml` | OSS-in-Actions |
| OS/arch runner matrix (`Go (<runner>)` jobs in CI) | Runs the Go job on `ubuntu-latest` (x64), `ubuntu-24.04-arm`, `macos-latest` (arm64), `windows-latest` (x64) and `windows-11-arm`, per the platform-specific testing in spec §19.4. Windows full suites bound package scheduling to one package at a time as runner capacity control: `windows-latest` uses `go test -race -p 1 ./...`, and `windows-11-arm` uses `go test -p 1 ./...` because the race detector does not support windows/arm64. This preserves within-package test parallelism, full test scope and existing timeouts. Other operating systems retain `go test -race ./...`. Both Windows jobs also run uncached, verbose launch- and stop-ownership acceptance tests, including native creation-time persistence in launch records and cleanup on creation-time read failure, and the `TestWindowsRunnerParentJobSpike` prerequisite probe, which reports parent-job membership and kill-on-close flags. Recovery tests also expose evidence for worker crashes, PID reuse, suspended-native recording, launch-boundary crashes and exited-process creation-time reads. Console-close survival and interrupt tests also print exercised outcomes or explicit blocked reasons when runner prerequisites are unavailable. These checks alone do not qualify the complete Windows lifecycle. | push to `main`, PR, merge queue | `.github/workflows/ci.yml` | GitHub-native |
| DCO (`DCO sign-off`, required) | Fails a PR with any commit lacking a matching `Signed-off-by`. | PR, merge queue (skipped) | `.github/workflows/dco.yml` | GitHub-native |
| Dependency review (`Dependency review`, required) | Blocks PRs adding dependencies with moderate+ advisories or licences outside the allow-list. | PR, merge queue (skipped) | `.github/workflows/dependency-review.yml` | GitHub-native |
| CodeQL (`Analyze (actions)`, required; `Analyze (go)`, required) | CodeQL `security-extended` analysis of the workflows and the Go code. | push to `main`, PR, merge queue, weekly | `.github/workflows/codeql.yml` | GitHub-native |
| Docs site (`Site build`, through required `CI OK`) | The `site-build` job calls the reusable Docs workflow once. It preserves mirror freshness, manifest revisions, transcript freshness and capability qualifiers, then builds Jekyll and asserts all mirrors rendered. The Pages artifact is uploaded before browser/npm/example steps. Its file-tree digest is checked after tools and after download; fixture markers and symlinks fail. Only a successful push-to-main build and examples job can deploy that same artifact to the protected Pages environment. No ruleset change is needed. | push to `main`, PR, merge queue through CI | `.github/workflows/ci.yml`, `.github/workflows/docs.yml` | GitHub-native |
| Docs content checks | Installs checksum-pinned ffprobe for video duration, then checks the built tree against source contracts: layout, tokens, origins, status claims, state labels, examples markers and search coverage. After artifact upload, the same pinned Pages action renders an isolated fixture using the real state include; valid labels and missing, empty or invalid values are checked, with fixture source/output outside the published tree. | Site build | `scripts/ci/check_docs_site.py`, `.github/workflows/docs.yml` | OSS-in-Actions |
| Docs browser checks | Installs the locked driver with `npm ci` and no dependency cache; uses the runner's preinstalled Chrome, reporting its version and failing if missing. Measures overflow, targets, contrast, motion, requests and landing weight; rejection fixtures use temporary directories outside the published tree. | Site build | `scripts/ci/browser/`, `.github/workflows/docs.yml` | OSS-in-Actions |
| Docs examples | Downloads the same run's Pages artifact, verifies its digest, then runs every listed command verbatim with its expected exit code. Output entries compare stdout only under `docs-output-v1`. After module prefetch, each command runs in a digest-pinned Go container with no network, credentials, interactive input or writable host mounts, and with `GOPROXY=off`. Harness fixtures reject wrong exits, missing blocks, output drift and credential/trigger references. Actual fork-run inspection remains a maintainer check. | Site build | `docs/_data/examples.yml`, `scripts/ci/check_docs_examples.py`, `.github/workflows/docs.yml` | OSS-in-Actions |
| Charm VHS recordings | Scripted, reproducible terminal recordings for docs and README: demo, TUI and hero GIFs, hero WebM/MP4 videos and PNG posters, regenerated with `docs/demos/record.sh`. | Manual re-record on behaviour change | `docs/demos/record.sh`, `docs/demos/manifest.json` | OSS-in-Actions |
| OpenSSF Scorecard | Supply-chain posture score, published to the Scorecard API and code scanning. | push to `main`, weekly, manual | `.github/workflows/scorecard.yml` | OSS-in-Actions |
| zizmor | Security audit of every workflow and `dependabot.yml`. The `zizmor` job fails on findings of low severity or higher; the `zizmor (code scanning)` job uploads SARIF to code scanning. | push to `main`, PR, merge queue (audit only) | `.github/workflows/zizmor.yml` | OSS-in-Actions |
| PR title lint | Enforces the Conventional Commits title convention in [CONTRIBUTING.md](../CONTRIBUTING.md#pull-request-titles). | PR opened, edited, synchronised, reopened | `.github/workflows/pr-title.yml` | OSS-in-Actions |
| Dependabot (`github-actions`) | Weekly grouped action updates, with a 7-day cooldown before adopting a new release. | weekly | `.github/dependabot.yml` | GitHub-native |
| Dependabot (`npm`) | Weekly grouped browser-driver updates, with the same 7-day cooldown. | weekly | `.github/dependabot.yml` | GitHub-native |
| Dependabot (`gomod`) | Weekly grouped Go module updates, with the same 7-day cooldown. | weekly | `.github/dependabot.yml` | GitHub-native |
| golangci-lint (`golangci-lint` job in CI) | Lint and security static analysis: the standard linters plus `bodyclose`, `errorlint`, `gosec`, `misspell` (UK), `nolintlint`, `revive`, `gocritic`, `perfsprint`, `dupl`, `modernize`, `copyloopvar`, `usetesting` and `godot`, with `gofmt` and `goimports` as formatters. The version is pinned in the workflow. | push to `main`, PR, merge queue | `.golangci.yml`, `.github/workflows/ci.yml` | OSS-in-Actions |
| govulncheck (`govulncheck` job in CI) | Reports known vulnerabilities in reachable Go code. Run with `go run` at a pinned version, because `golang/govulncheck-action` is not on the repository's action allow-list. | push to `main`, PR, merge queue | `.github/workflows/ci.yml` | OSS-in-Actions |
| go-licenses (`go-licenses` job in CI) | Fails when any package in the build or test import graph has a licence outside the allow-list. The allow-list is read at run time from `allow-licenses` in `dependency-review.yml`, so the two checks cannot drift. | push to `main`, PR, merge queue | `.github/workflows/ci.yml`, `.github/workflows/dependency-review.yml` | OSS-in-Actions |
| OSV-Scanner (`OSV-Scanner`) | Scans `go.mod`/`go.sum` against the OSV database and uploads SARIF to code scanning. Unlike govulncheck, it reports every known vulnerability in a dependency version, reachable or not. It fails on findings but is not a required check. | push to `main`, PR, weekly | `.github/workflows/osv-scanner.yml` | OSS-in-Actions |
| Labeler | Labels PRs by changed paths (`documentation`, `ci`, `harness`, `adapter`, `tui`, `protocol`, `dependencies`). | PR opened, synchronised, reopened (`pull_request_target`, no checkout) | `.github/workflows/labeler.yml`, `.github/labeler.yml` | GitHub-native |
| Stale | Marks issues stale after 60 days and closes 14 days later; PRs after 30 and 14. Exempt: `security`, `good first issue`, `help wanted`, `decision`, `needs-triage`. | weekly, manual | `.github/workflows/stale.yml` | GitHub-native |
| Lock threads | Locks closed issues and PRs after 90 days of inactivity. | weekly, manual | `.github/workflows/lock.yml` | OSS-in-Actions |
| Welcome | Greets first-time issue and PR authors, pointing to CONTRIBUTING.md and the DCO. | issue opened, PR opened (`pull_request_target`, no checkout) | `.github/workflows/welcome.yml` | GitHub-native |
| GoReleaser (OSS) | Builds the five-pair archive matrix (darwin/amd64 excluded) with `checksums.txt` and a per-archive SPDX SBOM. Pinned CLI and action; syft installed at a pin for the SBOM. The `test/*` run is a snapshot that publishes nothing. | tag `v*` (publishes, `release` environment); tag `test/*` (dry run) | `.goreleaser.yml`, `.github/workflows/release.yml` | OSS-in-Actions |
| `actions/attest-build-provenance` | SLSA build-provenance attestations over `checksums.txt` and the release archives, verified with `gh attestation verify`. | tag `v*` | `.github/workflows/release.yml` | GitHub-native |
| cosign keyless | Signs `checksums.txt` into a `.sigstore.json` bundle uploaded to the release, via GitHub OIDC; no stored keys. | tag `v*` | `.github/workflows/release.yml` | OSS-in-Actions |
| go-licenses notices | `go-licenses save ./cmd/mythhelm` generates the third-party licence notices shipped in every release archive. | tag `v*` | `.github/workflows/release.yml` | OSS-in-Actions |
| Release drafter | Keeps a draft release up to date, grouped by PR-title type and labels. Version bump: `feat` or `enhancement` = minor, a `!` title or `breaking-change` label = major, otherwise patch; `semver:major`/`semver:minor` labels override. `skip-changelog` excludes a PR. A maintainer publishes. | push to `main` only | `.github/workflows/release-drafter.yml`, `.github/release-drafter.yml` | OSS-in-Actions |
| CodeRabbit | AI review with summary, `assertive` profile, path-specific instructions (supply chain, spec invariants I01-I19, harness), plus golangci-lint, gitleaks, actionlint, shellcheck, markdownlint and yamllint. Never requests changes. Needs the maintainer to install the app. | PR | `.coderabbit.yaml` | Free-plan SaaS (advisory) |
| Sourcery | AI review. It has no repository config file for GitHub review; settings live in the Sourcery dashboard (see below). Needs the maintainer to install the app. | PR | Sourcery dashboard | Free-plan SaaS (advisory) |

Tools run with `go run <module>@<version>` (actionlint, govulncheck, go-licenses, OSV-Scanner) are pinned in the workflow files, and Dependabot does not update those pins. Bump them by hand, keeping the 7-day cooldown.

The checks marked required above (including those to be made required) are listed in `.claude/data/required-checks.json`. The harness compares the live ruleset with that file to notice a weakened gate, and its lint fails when the two lists differ, so change both together.

The repository ruleset requires review threads to be resolved before merge, and that includes threads opened by AI reviewers. Resolve a bot thread once you have acted on it or replied with a reason.

### Sourcery dashboard settings

Sourcery reads its [review settings](https://docs.sourcery.ai/reviews/configure/) and [review rules](https://docs.sourcery.ai/reviews/review-rules/) from its dashboard rather than a file in the repository. Suggested settings for this repository:

- Review profile: Balanced. CodeRabbit already runs `assertive`, so doubling the nitpicks adds noise.
- Path filters: exclude `docs/spec/master-spec.md`.
- Review rules: mirror the `path_instructions` in `.coderabbit.yaml` for `.github/workflows/**`, `internal/**`, `adapters/**`, `hosts/**`, `.claude/**` and `scripts/**`.
- Do not enable any Sourcery status check as required.

## Deferred

Each deferred item has a trigger that activates it. Add it in the PR that meets the trigger, or in the one straight after.

| Automation | What it will do | Activation trigger | Tier |
|---|---|---|---|
| ClusterFuzzLite | Continuous Go fuzzing of parsers on PRs and a schedule. | First parser or protocol code | OSS-in-Actions |
| Homebrew, Scoop, winget, AUR | Package-manager publishing. Tap and repository tokens live only in a tag-triggered job bound to a protected `release` environment; never reachable from PRs. | First release | OSS-in-Actions |
| macOS x64 runner | Adds an Intel macOS leg to the Go matrix; `macos-latest` covers arm64 today. | First release | GitHub-native |
| benchstat benchmarks | Benchmarks on PRs compared with `main` using benchstat. | First performance-sensitive code | OSS-in-Actions |
| harden-runner | Egress auditing, then blocking, for workflow runners. | Next PR, once `ci.yml` settles | OSS-in-Actions |
| Coveralls | Coverage reporting on PRs, advisory only. | Next PR, once `ci.yml` settles | Free-plan SaaS (advisory) |

## Maintainer set-up outside the repository

These need repository-admin action and are not configured by files here. Already done: the CodeRabbit and Sourcery apps are installed on this repository only, and `Analyze (go)` is a required check.

- Enable the merge queue in the `main` ruleset. All the required checks already run on `merge_group`.
- Optionally make `zizmor` a required check once it has been green on `main` for a while.
