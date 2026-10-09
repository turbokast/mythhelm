# release-pipeline — Retrospective

## Review Summary

- **Range**: cb5f2c3..2c16303 (PRs #216, #217, #221, #218, #222, #269; fix PRs none)
- **Reviewer**: code-reviewer, performed inline (single-domain change: workflows + GoReleaser config + docs)
- **Findings**: critical 0, important 0, suggestion 3 (confirmed); rejected 2
- **Open critical**: 0
- **Vendor review**: skipped (codex, muse, jev all disabled)

| Severity | Finding | Anchor | Disposition |
|---|---|---|---|
| suggestion | `design.md` still says a v1-style config "is refused"; v2.18.2 only warns (Task 1 recorded this staleness, left for orchestrator) | `specs/done/release-pipeline/design.md:43` | fixed in finalize PR |
| suggestion | Tag regex accepts `v1.2.3-rc.01` (leading-zero numeric prerelease), which strict semver — hence ADR-0010's `v<semver>` shape — rejects. Behavior is disclosed in `docs/release-process.md`, impact is an odd-but-gated version string | `.github/workflows/release.yml:32` | kept in list; tighten regex or record ADR acceptance as follow-up |
| suggestion | Tool-install shapes diverge between the two workflows: cosign via `cosign-installer` (release.yml:63) vs `go install` (dry-run:29-36), syft via `GOPATH` append vs `GOBIN=$RUNNER_TEMP/bin`. Same versions (cosign v3.1.3, syft v1.52.0), both proven, but bumps must be applied in two shapes | `.github/workflows/release.yml:50-53`, `.github/workflows/release-dry-run.yml:29-36` | kept in list; unify in a follow-up |
| rejected | Missing `artifact-metadata: write` permission | `.github/workflows/release.yml` permissions | registry-only; default provenance needs only `id-token`/`attestations` — verified in upstream README + pinned action.yml |
| rejected | `keep-existing` re-run collision semantics | `.github/release-drafter.yml` | procedure works under either behavior; H6 covers confirmation at first real release |

Foreign changes in range: none.

## Acceptance

| Criterion | Result | Evidence |
|---|---|---|
| AC-1.1 matrix + checksums + SBOM | met | `.goreleaser.yml` builds/checksum/sboms; Task 6 dry run green (PR #269) |
| AC-1.2 version-tag trigger only | met | `release.yml` `tags: ['v*']` only; zero `release.yml` runs exist; only `v*`/`test/*` triggers in `.github/workflows/` (PR #217) |
| AC-1.3 test-tag dry run | met | `release-dry-run.yml`; run 37914280148 success on `test/2026-10-09-mh7` (PR #269) |
| AC-2.1 attest + keyless sig, no stored keys | met as configured | cosign `--bundle` + attest steps; glob `subject-path` confirmed supported in pinned action's own action.yml; behavioral proof deferred to first real release per N2/H2 (accepted) (PR #217) |
| AC-2.2 notices + changelog/checksums/SBOM | met as configured | `third_party_licenses/` in archives (dry-run proven); changelog via `keep-existing` (H6, accepted unproven) (PR #217) |
| AC-2.3 documented verify + tamper leg | met | Task 6 executed checksum + cosign + tamper legs; attestation leg excluded per H2 (accepted) (PR #269) |
| AC-3.1 publish only in `release` job | met | `secrets.GITHUB_TOKEN` only inside `release` job; dry-run has no `secrets.`/`environment:`; CI workflows job is `contents: read` (PR #217) |
| AC-3.2 missing env stops before publish | met | `environment: release`; env exists per Q2 (PR #217) |
| AC-3.3 limitations link in notes | met as configured | footer live — current `v0.1.0` draft body ends with the `docs/limitations.md` link; preservation via `keep-existing` is H6 (accepted) (PR #217) |
| NFR-1 timeouts | met | `timeout-minutes: 60` in release.yml and dry-run (PR #217, #218) |
| NFR-2 free/OSS only | met | OSS distribution, syft/cosign/go-licenses, no new secrets (PR #216, #217, #218) |

## Deviations

- Task 1: acceptance bullet 1 reworded to assert the measured v2.18.2 warning behavior instead of refusal — recorded in the task entry; design.md sentence left stale for the orchestrator (fixed in this finalize PR).
- Task 2: syft install step added (spec predates the tool the `sboms` stanza shells out to); syft pinned v1.52.0 not v1.54.1 (7-day cooldown); full pin-age re-check recorded — recorded in the task entry; no impact beyond the pins.
- Task 3: syft v1.52.0 (same cooldown, re-checked independently); cosign via `go install` not `cosign-installer` (installer SHA unresolvable from the session); `GORELEASER_CURRENT_TAG=v0.0.0` guard added — recorded in the task entry; install-shape divergence is review suggestion 3.
- Task 4: red-check bullet amended to a header grep (same v2.18.2 exit-0 measurement as Task 1); extra header-assertion CI step; `setup-go` moved to `go-version-file: go.mod` — recorded in the task entry; no impact.
- Task 5: stale-bundle delete before re-run (`--clobber` gap); untested until the first real release — recorded in the task entry; H6 covers it.
- Task 6: binary reports `0.0.0-SNAPSHOT-f1cf02a` (tag-derived, not tag-named) — recorded in the task entry; accepted as the required `test/*` evidence.

## CI history

- Task PR branches (`ci/release-pipeline-t1` … `docs/release-pipeline-t6`): every workflow run green on every head SHA; `cancelled` runs are superseded pushes only. No failure-and-success pair on the same head SHA: no nondeterministic failures.
- Docs/Deploy docs site on main at 94d2138 (#218's merge): infra — the `deploy-pages` step failed with GitHub's HTTP 500 "Failed to create deployment … Server error" (run 37801208368, job 113394239478). Single occurrence; the same step is green on the next and all later main Docs runs (37802893771 et seq.). Failing value: deploy step 500 at 94d2138; passing value: deploy step green at the following main push. GitHub-side cause, not a repo defect.

## Effort

dispatched=7 returned=6 failed=1; attempts 7 over 6 tasks; first-pass 4/6; review rounds 6; wall-clock 2026-10-08T11:25:07Z → 2026-10-09T10:23:16Z

| Task | Agent | Attempts | Review rounds | PR | Merged | First pass |
|---|---|---|---|---|---|---|
| 1 | release-engineer | 3 | 1 | #216 | yes | no |
| 2 | release-engineer | 1 | 0 | #217 | yes | yes |
| 3 | release-engineer | 1 | 2 | #218 | yes | yes |
| 4 | release-engineer | 1 | 0 | #221 | yes | yes |
| 5 | go-implementer | 1 | 3 | #222 | yes | yes |
| 6 | - | 0 | 0 | #269 | yes | no |

## Lessons

- What worked: the cloud trial ran Task 1 alone first, so the missing-syft spec gap failed cheaply on attempt 1 instead of inside the parallel wave (run-events `fail` 2026-10-08T13:02:38Z); binding orchestrator decisions (reword to measured behavior) unblocked Task 1 within 40 minutes; wave-2 Tasks 2–4 implemented and merged the same afternoon; the Task 6 dry run proved archives at the merged head before finalize.
- What worked: review rounds caught doc precision issues (T5: hand-pushed tag claim, download dir, recovery procedure, tag grammar, attestation subject, cleanup loop) before merge across PR #222's 3 rounds.
- What to change: the spec predated `syft`, the tool its own config shells out to — Task 1 needed 3 attempts for what the spec should have provisioned (PR #216).
- What to change: the 7-day pin-age cooldown forced the same syft downgrade (v1.54.1 → v1.52.0) independently in Tasks 2 and 3; a shared pin record would have decided once (PR #217, #218).
- What to change: Task 5's docs needed 3 review rounds for 6 claim fixes — a claim-verification checklist at implement time would have caught them in one pass (proposal P-release-pipeline-1).
- What to change: the design §4 stale sentence survived from Task 1 to the finalize review; fixed in this PR.

## Proposals

- P-release-pipeline-1 — docs-task claim verification checklist
