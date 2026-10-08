# Release Pipeline — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| 1 | Exact GoReleaser CLI + action pins at implementation time (newest seen v2.18.2 / action v7.2.3, Oct 2026; 7-day cooldown applies) | Task 1 re-checks the releases pages, records the exact pins in its completion entry; Tasks 2–4 pin to them | Task 1 / blocks 2–4 |
| 2 | Exact `actions/attest-build-provenance` (v4.x), `cosign-installer` (v4.1.x) and cosign CLI (v3.0.x) pins + SHAs at implementation time | Task 2 resolves from the named releases, never from memory (workflows rule); records SHAs in its entry | Task 2 / blocks nothing further |
| 3 | `subject-path` multi-subject separator in the pinned attest action | List checksums + archives per the action README at the pinned version; acceptance verifies an archive and checksums.txt both attest | Task 2 / blocks nothing further |
| 4 | Upstream note: "new implementations should use `actions/attest`" instead of `attest-build-provenance` | Stay with `actions/attest-build-provenance` (requirements fix it by name; design D4); revisit only via a new ADR | Maintainer / blocks nothing |

## Research notes

- GoReleaser v2: `version: 2` header required; `--rm-dist` → `--clean`; `archives.format` → `formats:` with `format_overrides` per-OS (appackio/apppack #190, 2026-09-17); `release.mode: keep-existing|append|replace` (goreleaser.com/customization/publish/scm/, fetched Oct 2026); `snapshot.version_template` exists; `go install github.com/goreleaser/goreleaser/v2@<v>` is a documented install path.
- cosign v3 (e.g. v3.0.6 via installer v4.1.2): `sign-blob` defaults to self-contained `.sigstore.json` bundles; `--output-signature`/`--output-certificate` are silently ignored (splattner/goucrt, 2026-09). Verify: `cosign verify-blob --bundle … --certificate-identity-regexp … --certificate-oidc-issuer https://token.actions.githubusercontent.com`.
- attest-build-provenance v3 = Node 24 line; v4 (Feb 2026) wraps `actions/attest`. Job needs `attestations: write` + `id-token: write`.
- go-licenses: `save ./cmd/<bin> --save_path=<dir>` collects per-module LICENSE texts; CI already pins `v2.0.1` (`.github/workflows/ci.yml:120`).
- No CGO in tree (`grep -rn 'import "C"'` empty); ADR-0003 pure-Go SQLite + `TestBuildWithoutCgo` ground single-runner cross-compilation.
- Q1/Q2/Q3 answered 2026-10-07 (refinement-log.md): matrix linux/amd64+arm64, darwin/arm64, windows/amd64+arm64; `release` env exists (required reviewer + `v*` tag policy); `test/*` operator-owned.

## Discoveries

- Task 1 (attempt 2): GoReleaser v2.18.2 `goreleaser check` exits 0 on a config without the `version: 2` header (it prints "only version: 2 configuration files are supported, yours is version: 0" as a warning and still reports "1 configuration file(s) validated"), so Task 1's negative acceptance bullet cannot hold as written. All other Task 1 acceptance bullets pass with the committed `.goreleaser.yml`, with syft v1.54.1 installed. Resolved by the orchestrator: the bullet now asserts the warning. `design.md` still claims a v1-style config is refused; stale for v2.18.2, left for the orchestrator.
