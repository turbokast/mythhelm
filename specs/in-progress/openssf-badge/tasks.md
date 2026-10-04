## OpenSSF Best Practices Badge — Tasks

### Dependencies

- None outside this spec. Strictly sequential Task 1 → Task 2 → Task 3; no parallel group (design D4: the badge shows only when passing, and passing evidence completes in Task 2).
- Related specs touching `README.md` (`specs/*/tui-slice/`, `specs/*/docs-site-demos/`) edit disjoint regions and can land in any order; if the badges block moved, re-anchor with `grep -n 'badge\.svg\|scorecard\.dev\|shields\.io/badge' README.md`.
- **Gates for every task.** No Go files change, so `gofmt`/`go vet`/`go test` are skipped with that reason. Run `scripts/ci/check-public-hygiene.sh` (the any-change gate) and `scripts/ci/lint-agent-harness.sh --only specs` after editing spec files. A task is not complete because files exist or an agent reported success; cite the check output.
- **Completion convention.** Append ` ✅ COMPLETED` to the heading, keep every original field, and add `Status`, `Implementation` (assessment URL of record / card ids / commit SHA as applicable, with check outputs), `Spec deviations` (`None`, or each deviation with its reason) and `Files modified`.
- **Commits.** Every commit is signed off (`git commit -s`, DCO). No billing, persistence or process-ownership change, so no ADR.

---

## Implementation Tasks

### Task 1 — Register the assessment, answer the passing criteria, record the URL and N/A/Unmet table ✅ COMPLETED

- **Domain/agent**: maintainer
- **Budget**: standard
- **Depends on**: None
- **Change**: Register the project entry under the maintainer's personal bestpractices account (Q1), answer every passing-tier criterion with repo evidence (N/A only where the criterion allows it; SHOULD/SUGGESTED gaps answered Unmet with a justification naming the gap; applicable MUST/MUST NOT answered Met or allowed-N/A), and record the assessment URL of record, the site-generated badge snippet and the §3 rows. The `design.md` edit below is the AC-1.2-mandated exception per design D3.
- **Files**:
  - `specs/*/openssf-badge/design.md` (fill the §3 N/A/Unmet table: one row per N/A or Unmet answer; the spec's own directory in whichever lifecycle state holds it when this task runs)
- **Produces**: Assessment URL of record (stable `<host>/projects/<id>` URL); N/A and Unmet criterion ids with justifications (design §3 rows; ids are the app's criterion keys as in the `.json` `<name>_status` keys); site-generated badge snippet bytes (expected shape `[![OpenSSF Best Practices](<project-URL>/badge)](<project-URL>)` on the non-locale URL form; the site's bytes win).
- **Acceptance**:
  - `check-assessment-registered`: `curl -sSL <assessment-URL>.json` returns 200 JSON whose `repo_url` points at `github.com/turbokast/mythhelm`. Red before registration: non-200.
  - Questionnaire complete: `badge_percentage_0 == 100` in that JSON. Red on a fresh assessment: below 100.
  - `check-na-justified` (design §6) passes: every N/A sits on a criterion that allows it, the N/A-or-Unmet id set in the JSON equals the §3 rows, every justification non-empty and identical in both places.
  - The recorded snippet's link-target project id equals the URL of record's id.
- **Test plan**: Maintainer-run against the live app; paste the `curl`/JSON outputs into the completion entry. No repo tests: no code changes.
- **Invariants touched**: None (external questionnaire plus spec-table rows; no code, no release claim, no advertised capability changes in-repo).
- **Status**: ✅ Completed — project registered as bestpractices.dev/projects/15212, passing tier at 100 (passing); §3 rows recorded; PR #111.
- **Implementation**: Assessment URL of record `https://www.bestpractices.dev/projects/15212` (project id 15212). Checks: `curl -sSL <AU>.json` → http=200, `repo_url` = `https://github.com/turbokast/mythhelm`; `badge_percentage_0` = 100; `badge_level` = passing. N/A-or-Unmet passing-tier set = 3 Unmet (`version_semver`, `version_tags`, `warnings_strict`) + 6 N/A (`release_notes_vulns`, `vulnerability_report_response`, `crypto_keylength`, `crypto_pfs`, `crypto_password_storage`, `dynamic_analysis_unsafe`), all with non-empty justifications identical to design §3; every N/A sits on a criterion that allows it. `achieve_silver` (Unmet, empty justification) excluded as a derived next-level gate, not a passing-tier answer (noted in §3). Site-generated snippet (markdown form, link target id 15212 = URL of record id): `[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/15212/badge)](https://www.bestpractices.dev/projects/15212)`.
- **Spec deviations**: None.
- **Files modified**: `specs/in-progress/openssf-badge/design.md`, `specs/in-progress/openssf-badge/tasks.md`, `specs/in-progress/openssf-badge/handoff.md`.

### Task 2 — File backlog cards for unmet criteria and reference them from the assessment ✅ COMPLETED

- **Domain/agent**: maintainer
- **Budget**: standard
- **Depends on**: Task 1
- **Change**: File one backlog card per Unmet criterion from Task 1 through `/backlog add` and the normal approval flow (never by writing `product/` directly), then reference each card id from its Unmet justification or the assessment notes and fill the §3 card column. The `design.md` edit below is part of the AC-1.2-mandated record per design D3.
- **Files**:
  - `specs/*/openssf-badge/design.md` (fill the card-id column of the §3 Unmet rows Task 1 wrote)
- **Produces**: Card id per Unmet criterion (`MH-<n>`), recorded in design §3 and the completion entry.
- **Acceptance**:
  - `check-cards-complete` (design §6) passes: the `.json` Unmet count equals the count of approved backlog cards naming those criterion ids (`python3 scripts/pm/pm.py list` plus card inspection).
  - `check-cards-referenced` (design §6) passes: each Unmet justification or `general_comments` names its card id.
  - `check-assessment-passing` (design §6) passes with the card references in place: `badge_percentage_0 == 100`, `badge_level` in {passing, silver, gold}.
  - Zero-Unmet branch: if Task 1 recorded no Unmet rows, no cards are filed and the completion entry records the Unmet count of 0 with the JSON probe output alongside `badge_percentage_0 == 100` (the check asserts the zero count, not an empty "every card" universal).
- **Test plan**: Maintainer-run; paste the `pm.py`, `curl` and approval-request outputs into the completion entry. No repo tests: no code changes.
- **Invariants touched**: None (product approval flow plus external assessment notes; no code).
- **Status**: ✅ Completed — 3 cards filed and approved for 3 Unmet criteria; justifications reference them; §3 card column filled; PR #TBD.
- **Implementation**: Cards MH-18 (version_semver, issue #113), MH-19 (version_tags, issue #114), MH-20 (warnings_strict, issue #115) via product PRs #112 (cards + D-21/22/23) and #116 (issue links + D-24). Checks: `pm.py list` shows MH-18/19/20 triaged with criterion ids in Source (check-cards-complete: 3 Unmet = 3 cards); live `.json` shows each Unmet justification naming its card id + issue, verified byte-identical (check-cards-referenced); `badge_percentage_0` = 100, `badge_level` = passing with references in place (check-assessment-passing). Design §3 card column: MH-18/19/20 on Unmet rows, `—` on N/A rows.
- **Spec deviations**: None (D-24 decision entry was required by a review finding on PR #116, following the D-13 precedent — product-flow compliance, not a spec deviation).
- **Files modified**: `specs/in-progress/openssf-badge/design.md`, `specs/in-progress/openssf-badge/tasks.md`, `specs/in-progress/openssf-badge/handoff.md`.

### Task 3 — Display the badge in the README and verify it live

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 2
- **Change**: Append Task 1's site-generated snippet as a fourth line of the README badges block so the passing badge renders beside the existing badges, then verify the link live.
- **Files**:
  - `README.md` (badges block after line 10; re-anchor with `grep -n 'badge\.svg\|scorecard\.dev\|shields\.io/badge' README.md`)
- **Acceptance**:
  - `check-readme-badge` (design §6) passes: the badges block contains the Task 1 snippet verbatim with the assessment-URL-of-record link target, and the image URL fetches `200 image/svg+xml`.
  - `check-badge-link-current` (design §6) passes: the README link-target project id equals the URL-of-record id; red on the pre-change README, where extraction finds no badge id.
  - `scripts/ci/check-public-hygiene.sh` passes on the change.
- **Test plan**: No repo tests (one-line docs change; N3 forbids CI additions). Verification is the live checks above; paste their outputs into the completion entry.
- **Invariants touched**: I07 (§11.5: the badge claim is verified against the live assessment URL and the committed README bytes, not trusted from the paste action).
