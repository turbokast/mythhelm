---
paths:
  - ".github/**"
---

# GitHub Workflows

Workflows run with the repository's token on every pull request, including from forks. Principles and the list of active automation are in `docs/automation.md`; keep its tables current in the same change.

## Supply chain

- Pin every `uses:` to a full 40-character commit SHA with a `# vX.Y.Z` comment. Never a tag or branch. Resolve the SHA from the release you name, and never from memory.
- Run tools at pinned versions (`go run tool@vX.Y.Z`, a pinned action, or a checksum-verified download). Never `curl | sh`.
- New actions come from `actions/*`, `github/*` or a well-maintained OSS project; say why in the pull request.

## Permissions and secrets

- Top-level `permissions:` is `contents: read` or `{}`. A job that needs more asks for it itself, with a comment saying why.
- `actions/checkout` sets `persist-credentials: false`.
- Never combine `pull_request_target` or `workflow_run` with a checkout or execution of pull-request code. Those triggers are for jobs that only label, comment or read metadata.
- No workflow on `pull_request` needs a secret or a paid service: fork pull requests must pass without them. Publishing credentials live only in tag-triggered jobs bound to a protected environment.
- Never interpolate `${{ }}` expressions from event data into `run:` scripts. Pass them through `env:` and quote them.

## Required checks

- `CI OK` in `ci.yml` is the single aggregate required check. A new required job joins its `needs:` list; the aggregate fails on `failure` or `cancelled`.
- Every workflow that produces a required check also triggers on `merge_group`.
- Adding a separately named required check needs the maintainer to update the ruleset: say so in the pull request description.

## Gates

`actionlint` and `zizmor` pass on every workflow change. Run `go run github.com/rhysd/actionlint/cmd/actionlint@<pinned version>` locally; the `zizmor` workflow runs in CI. A workflow run, rerun or dispatch is a publishing action (`guard-publish.sh`): only on the operator's request.
