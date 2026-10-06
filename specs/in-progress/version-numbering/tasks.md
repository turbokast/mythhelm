## Version Numbering — Tasks

### Dependencies

- Prerequisite specs: `specs/*/openssf-badge/` (shipped; the `version_semver` Unmet record this spec prepares to close). Feeds into MH-7 (release pipeline) and MH-19 (`specs/*/release-tagging/`): this decision lands first; both cite it.
- Order: single task; nothing to parallelize.
- **Gates for every task.** No Go code changes, so the Go gates are skipped with that reason. Docs task: `scripts/ci/check-public-hygiene.sh`. Files existing or an agent reporting success is not completion; cite the runner output.
- **Completion convention.** Append ` ✅ COMPLETED` to the heading, keep every original field, and add `Implementation` (at most 3 lines, plus the commit SHA), `Spec deviations` ("None" or a justification) and `Files modified`.
- **Commits.** Every commit is signed off (`git commit -s`, DCO). No decision record needed beyond the task's own ADR file: no billing, persistence or process-ownership change (the versioning scheme is a public-contract decision, recorded as ADR 0009 in this task).

---

## Implementation Tasks

### Task 1 — Version-numbering records (ADR + release-process scheme)

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Change**: Record the ratified SemVer scheme with its rationale as ADR 0009 and as the operational scheme section of the new release-process record (design §2–§3); no CHANGELOG edit on the ratify path.
- **Files**:
  - `docs/decisions/0009-version-numbering-scheme.md` (the decision per design §3)
  - `docs/release-process.md` (new; scheme section per design §3)
- **Produces**: Scheme contract — SemVer 2.0.0 ratified; releases-only scope; `v`-prefixed tags assumed, exact format owned by MH-19; MH-7 implements the pipeline under this scheme. MH-19 and MH-7 cite these records instead of re-deciding (forward trace; their acceptance, not this spec's check).
- **Acceptance**:
  - The ADR follows the `docs/decisions/README.md` template: title `# 0009. Version numbering scheme`, a `Status: accepted` line and a `Date: YYYY-MM-DD` line (deleting any of the three fails the check), plus `Context`, `Decision` and `Consequences` sections.
  - The ADR names `Semantic Versioning 2.0.0` as the adopted scheme with the three AC-1.1 rationale elements — the Go module-path fit (suffix-less path admits only v0/v1), the in-tree v-prefixed semver machinery (release-drafter), and pre-alpha 0.x semantics with S1-preview-vs-later framing — cites the standing `CHANGELOG.md:6` claim, and states the releases-only scope with the `devel`/`unknown` dev strings continuing untouched (each element anchored to its section; removing any fails its grep).
  - `docs/release-process.md` carries the scheme section: the strings `v0.1.0` and `v1.2.3` as the example version strings, an MH-19-owns-format sentence naming MH-19, and an MH-7-implements-pipeline sentence naming MH-7 (each anchored to the scheme section; deleting any fails its grep). The file states pipeline sections land with MH-7 and assumes no undesigned pipeline detail (no GoReleaser workflow, job or secret names beyond the card-fixed "tag-triggered" and "GoReleaser" terms — `grep -nE 'workflow|job:|secrets\.' prints nothing for the file).
  - No CHANGELOG edit: `git status --porcelain CHANGELOG.md` prints nothing (ratify path — the claim stands; any edit fails this leg).
  - The prepared `version_semver` justification text sits in design §4 naming ADR 0009, `docs/release-process.md` and the first-release demonstration condition with its fill-at-flip tag placeholder (deleting any of the three fails the grep); the maintainer's spec checkpoint reviews it before this task merges.
  - `scripts/ci/check-public-hygiene.sh` passes (a hygiene violation in either new file fails the script).
- **Test plan**: anchored `grep`/template checks run locally; the scheme's consumers (MH-19, MH-7) cite these records in their own specs.
- **Invariants touched**: None (prose records only: no code, no advertised capability — the first release demonstrates the scheme later, and the assessment stays Unmet until the operator flips it).
