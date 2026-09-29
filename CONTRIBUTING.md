# Contributing to MYTHHELM

Thanks for helping. This guide covers how changes get in.

## Before you start

- **Small fixes** (typos, docs, obvious bugs): open a pull request directly.
- **Features and behaviour changes**: open an issue or a [Discussion](https://github.com/turbokast/mythhelm/discussions) first, so we can agree on scope before you invest time.
- **Architectural changes** to security, billing, the public protocol or persistence: these need a short [decision record](docs/decisions/) and tests. See [GOVERNANCE.md](GOVERNANCE.md).
- **Security issues**: follow [SECURITY.md](SECURITY.md). Never use a public issue.

Look for [`good first issue`](https://github.com/turbokast/mythhelm/labels/good%20first%20issue) and [`help wanted`](https://github.com/turbokast/mythhelm/labels/help%20wanted) if you want a starting point.

## You don't need paid subscriptions

The full contributor test suite and the offline demo run without any agent credentials, because a scripted fake adapter stands in for real agents. If a change needs a live vendor account to verify, say so in the pull request. A maintainer will run the opt-in live canary.

## Developer Certificate of Origin (DCO)

We use the [Developer Certificate of Origin](DCO) instead of a CLA, and there is no copyright assignment. Every commit must carry a `Signed-off-by` line whose name and email match the commit author:

```
Signed-off-by: Your Name <you@example.com>
```

`git commit -s` adds it for you. To fix the commits on a branch, run `git rebase --signoff main` and force-push the branch. A CI check blocks pull requests with unsigned commits.

## AI-assisted contributions

AI-assisted contributions are welcome and are judged like any other: on evidence, provenance and quality. By signing off, you, a human, attest that:

- you reviewed and understand every line you are submitting;
- you have the right to submit it under the project license; and
- the tests and evidence in the pull request are real, not generated claims.

Please mention significant AI assistance in the pull request description. Unreviewed bulk output will be closed.

## Pull requests

- Keep each PR focused on one change. Large PRs get split.
- Include tests that fail without your change and pass with it.
- Update documentation when behaviour changes.
- Every required CI check must pass. Branches merge by squash or rebase, so history stays linear.
- Be patient and kind. This is a volunteer project, so reviews take time.

### Pull request titles

PR titles follow [Conventional Commits](https://www.conventionalcommits.org/): `type(optional-scope): summary`, for example `fix(tui): keep focus after resize` or `feat!: drop the v0 protocol`. A `!` marks a breaking change. A CI check enforces this, and the release notes are drafted from these titles.

Allowed types: `feat`, `fix`, `docs`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert` and `harness` (the development harness: `.claude/`, `knowledge/`, `scripts/`). Scopes are optional and free-form. Edit the title and the check re-runs.

AI review bots may comment on pull requests. Their comments are advisory: act on them or reply saying why not. They are never a required check, but every review thread, a bot's included, must be resolved before merge. See [docs/automation.md](docs/automation.md).

## Development setup

The Go toolchain version is pinned in `go.mod` once the codebase lands. Build, test and lint commands will be documented here alongside the first code.

## Licensing of contributions

Contributions are licensed under the [Apache License 2.0](LICENSE), in line with its section 5. Don't copy code from other projects unless its license is compatible, and record it in the PR.
