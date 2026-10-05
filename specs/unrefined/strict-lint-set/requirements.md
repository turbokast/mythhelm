## Strict Lint Set — Requirements

> Unrefined. Run /refine-spec strict-lint-set before /spec.

> Evaluates further golangci-lint linters past the current standard-plus-five set, enables the strictest practical ones, and fixes the resulting fallout at source. Closes the OpenSSF `warnings_strict` (SUGGESTED) gap. Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it. Historical Revision 1.1 pins (§19.3, §19.4) keep their original meaning per docs/spec/README.md.

## Context

- **Backlog card**: MH-20
- **Issue**: [#115](https://github.com/turbokast/mythhelm/issues/115) — "Strictest practical lint set (MH-20)".
- **Problem**: The OpenSSF passing assessment answers `warnings_strict` (SUGGESTED) as Unmet: the lint set — golangci-lint standard plus bodyclose, errorlint, gosec, misspell and nolintlint — is solid but not maximal. The openssf-badge spec recorded the gap in design §3 with its justification referencing this card and issue, keeping the badge at passing while the gap stays open.
- **Work**: "Strictest practical" is a measured claim, not a maximal one: each candidate linter is trial-run against the current tree, its findings weighed against false-positive and CI-time cost, the winners enabled in `.golangci.yml`, and every resulting finding fixed at source or individually justified. The assessment flip on bestpractices.dev is an external service change and stays an operator action; this spec prepares the justification text and the evidence behind it.
- **Grounding verdicts** (2026-10-05, main @ 176fead):
  - Issue #115 describes this gap and names MH-20: HOLDS (read 2026-10-05; labels enhancement, ci; no comments).
  - `warnings_strict` SUGGESTED, answered Unmet, tracked in MH-20/issue #115: HOLDS (`specs/done/openssf-badge/design.md:31`, recorded 2026-10-04 at `badge_level == passing` per `specs/done/openssf-badge/design.md:39`).
  - Lint set is standard plus bodyclose, errorlint, gosec, misspell, nolintlint: HOLDS (`.golangci.yml`, exact enable list; one test-scoped G204 exclusion).
  - Current contract v2 §§17–18 via W16: PARTIAL — v2 §17 covers platforms/distribution and §18 delivery/acceptance with no explicit lint demand; W16 requires running "applicable Go formatting/tidy/shared repository gates for code changes" (`MYTHHELM_Implementation_Plan.md:219`). The strictness demand comes from the OpenSSF criterion, executed through the W16 gates language.
  - No spec covers MH-20: HOLDS — `MH-20` appears in `specs/` only as the openssf-badge follow-up pointer (card filing recorded in `specs/done/openssf-badge/tasks.md:52-53`); `spec-lifecycle.sh resolve strict-lint-set` finds nothing.
  - Gap is open and trials are runnable today: HOLDS — criterion Unmet today; `go` 1.27.1 installed locally, golangci-lint installable free; CI enforces the set via `golangci/golangci-lint-action` v9.3.0 (`.github/workflows/ci.yml:70-81`).
  - Assessment lives outside the repo; flipping it is an operator action: HOLDS — bestpractices.dev project 15212, verified live during openssf-badge (`specs/done/openssf-badge/tasks.md:53`).

### Objectives

- **O1**: The repository lints under the strictest practical golangci-lint set, chosen from measured trials, with CI green.
- **O2**: Every finding from the newly enabled linters is fixed at source or carries an individual justification — no blanket exclusions, no silent `//nolint`.
- **O3**: The `warnings_strict` → Met justification text and its evidence are prepared and reviewed, ready for the operator to flip the assessment.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Flipping the assessment on bestpractices.dev. | FR-3 of openssf-badge: justification text + evidence are prepared and reviewed here; the live assessment stays passing throughout. |
| N2 | Linters needing paid tooling, accounts or network access. | I13 analog: contributor gates stay free and offline-runnable. |
| N3 | Silver-tier or other beyond-passing criteria. | G10: no passing-tier answer regresses; the badge stays passing. |
| N4 | Rewrites or refactors beyond what the findings require. | Fallout fixes are minimal and behaviour-preserving; anything larger becomes a follow-up. |

## Functional Requirements

Acceptance criteria use EARS. Tags in brackets name the invariants, gates and sections each one serves.

### FR-1 — Candidates evaluated by trial (W16, G10)

- **AC-1.1** [W16] For every linter evaluated, the spec record shall name the linter, the exact golangci-lint version trial-run, the command, the finding count on the current tree, and the verdict (enable or reject with reason).
- **AC-1.2** [W16] A linter shall be enabled only when its trial shows true findings worth fixing at an acceptable false-positive and CI-time cost; a rejected candidate shall record which cost disqualified it.

### FR-2 — Strictest practical set enabled (W16, G10)

- **AC-2.1** [W16] `.golangci.yml` shall enable the evaluated winners, and `golangci-lint run ./...` at the CI-pinned version shall report zero findings on the tree.
- **AC-2.2** [W16] When a newly enabled linter cannot apply to a path, the config shall scope the exclusion to that path with a dated rationale comment; no new whole-tree exclusion shall exist.
- **AC-2.3** [G10] The CI `golangci-lint` job shall be green with the new set on Linux, macOS and Windows.

### FR-3 — Fallout fixed at source (W16)

- **AC-3.1** [W16] Every finding from the newly enabled linters shall be fixed in the flagged code; each remaining `//nolint` for those linters shall name its reason in the machine-checked form nolintlint enforces.
- **AC-3.2** [W16] Fallout fixes shall keep every existing test passing with no behaviour change beyond what the finding requires; any fix that cannot stay behaviour-preserving shall become a follow-up issue instead of a deviation.

### FR-4 — Assessment evidence prepared (§19.3, G10)

- **AC-4.1** [§19.3] The spec record shall carry the new `warnings_strict` justification text citing the enabled set (config reference) and the green CI run, ready for the operator to paste into the assessment.
- **AC-4.2** [G10] Until the operator flips the assessment, the live badge shall remain passing; this spec shall not edit the assessment itself.

## Non-Functional Requirements

- **NFR-1** [W16] The CI lint job with the new set shall complete within the workflow's existing job budget on all three runners; the implementing task records the measured times.
- **NFR-2** [I13 analog] Every enabled linter shall run offline with free tooling; a contributor reproduces the CI verdict with a documented local command.

## Definition of Done

- [ ] AC-1.1 to AC-4.2 each have a named check; CI green on Linux, macOS and Windows.
- [ ] Trial matrix in the spec record covers every evaluated linter with version, command, count and verdict.
- [ ] Zero findings at the CI-pinned version; zero new whole-tree exclusions.
- [ ] Justification text for `warnings_strict` → Met is reviewed and ready for the operator.

## Open Questions

1. **Which linters are candidates, and what makes one "practical"?** Blocks FR-1. Options: (a) enable-all-then-trim trial over the full golangci-lint catalogue; (b) a curated shortlist (e.g. revive, gocritic, perfsprint, testpackage, dupl) trial-run one by one. No default yet — /spec runs the trials and lets the findings decide.
2. **How are trials run reproducibly?** Blocks FR-1. Options: (a) locally installed golangci-lint v2.13.2, the binary version CI pins (`.github/workflows/ci.yml` lint job; v9.3.0 is the action version); (b) trial CI runs on the spec branch. Default: (a), with (b) as the referee.
3. **Where does the prepared justification text live?** Blocks FR-4. Options: (a) the spec record (scratchpad/retrospective) plus the task PR description; (b) a docs note beside the badge link. Default: (a) — the assessment is external, so no tree file can hold its live state.
4. **Fallout that spans domains?** Blocks task scoping. Options: (a) one config task plus per-domain fallout tasks joined by dependencies; (b) a single task when fallout is small and single-domain. Default: (a) per the one-domain-per-task rule; /spec decides from the trial spread.

## Dependencies

- **Builds on: `openssf-badge` (MH-9, `specs/done/openssf-badge/`, shipped 2026-10-04)** — the `warnings_strict` Unmet record (design §3), the card-per-Unmet mechanism (FR-3), and the live-assessment verification precedent.
- **Supersedes**: none. **Conflicts**: none known; `specs/todo/` and `specs/in-progress/` are empty. Fallout fixes coordinate by package at /spec time.

## Impacted components

- `.golangci.yml` (release domain): the enabled set plus any path-scoped exclusions with rationale.
- `.github/workflows/ci.yml` (release domain): only if the job needs changes (timeout, version pin) — read-only unless trials prove otherwise.
- Go sources per fallout (core/tui/adapters/protocol domains): /spec enumerates from the trial spread; each domain's fixes stay in that domain's task.
- Spec record: trial matrix, verdicts, prepared justification text.
