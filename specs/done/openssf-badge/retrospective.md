# openssf-badge — Retrospective

## Review Summary

- **Range**: f938e10..139d387 (PRs #111, #112, #116, #117, #118; fix PRs none)
- **Reviewer**: orchestrator, direct (docs-only change, 7 files / 93 insertions; no Claude code-reviewer subagent in this client — reviewed inline against requirements, design and invariants, recorded per AGENTS.md)
- **Findings**: critical 0, important 0, suggestion 1 (confirmed); rejected 0
- **Open critical**: 0
- **Vendor review**: skipped (all vendors disabled)

| Severity | Finding | Anchor | Disposition |
|---|---|---|---|
| suggestion | Definition-of-Done checkboxes still unchecked though all three boxes hold | `specs/done/openssf-badge/requirements.md:65-67` | fixed in finalize PR |

Foreign changes in range: none.

Spec-wide acceptance: AC-1.1 met (assessment 15212 at `badge_percentage_0 == 100`, `badge_level == passing`, verified live; `www.bestpractices.dev` host accepted per design D5 with the verified same-project redirect); AC-1.2 met (9 §3 rows verbatim; `achieve_silver` exclusion reasoned); AC-2.1/AC-2.2 met (snippet verbatim at README:11, id 15212 = URL of record, badge live `200 image/svg+xml` titled passing); AC-3.1 met (3 Unmet = 3 cards MH-18/19/20, all SUGGESTED, no MUST unmet); AC-3.2 met (card+issue refs byte-verified in the live JSON); NFR-1 met (canonical site snippet). Seams: task 1 → 2 → 3 contracts (URL of record, §3 rows, snippet bytes) consumed verbatim; no divergence between design.md and shipped state. Invariants: I07 honored throughout (every badge/assessment claim verified against the live app and committed bytes, never trusted from paste). No stale documentation: the README pre-alpha status section still holds; product cards MH-18/19/20 are open follow-ups, not stale.

## Acceptance

| Criterion | Result | Evidence |
|---|---|---|
| AC-1.1 passing assessment at stable URL | met | `check-assessment-passing`: live `.json`, 100/passing, PR #111 |
| AC-1.2 N/A/Unmet reasons in design.md | met | `check-na-justified`: 9 rows byte-identical, PR #111 |
| AC-2.1 badge in README | met | `check-readme-badge`: README:11 live passing, PR #118 |
| AC-2.2 badge link follows re-registration | met | `check-badge-link-current`: id 15212 = 15212, PR #118 |
| AC-3.1 one card per Unmet, MUSTs met | met | `check-cards-complete`: 3 = 3, PR #117 |
| AC-3.2 assessment references cards | met | `check-cards-referenced`: byte-verified, PR #117 |
| NFR-1 canonical badge URL, no vendored copy | met | site-generated snippet verbatim, PR #118 |

## Deviations

- Task 1: None — recorded in the task entry; no impact.
- Task 2: None — recorded in the task entry (D-24 was product-flow compliance per the D-13 precedent, not a deviation); no impact.
- Task 3: None — recorded in the task entry; no impact.
- Review found no design divergences.

## CI history

- No failed jobs on any spec pull request (#111, #112, #116, #117, #118): every workflow conclusion is `success` or `cancelled`. The `cancelled` runs are superseded-push cancellations (infra class — the runs never judged the code): #117 d2a67ed (CI, OSV-Scanner, zizmor) superseded by 2a05a1c; #118 b67742d (CI, OSV-Scanner, one PR-title) superseded by 9d60074.
- No same-sha fail+success pair anywhere: zero nondeterministic failures, zero unexplained nondeterminism.
- Main-tip CI for the #119 lifecycle merge (d77f2e8): 6/6 push runs green. One tooling false-negative on the way: `finalize.py ci --sha d77f2e8` reported `verdict=pending` ("no push run on main yet") while six completed runs existed, because `gh run list --commit` does not match a short SHA; the full SHA returned `verdict=green`. See P-openssf-badge-1.
- Run-events log: no `fail` or `retry` rows for this spec.

## Effort

dispatched=1 returned=0 failed=0; attempts 1 over 3 tasks (tasks 1–2 were maintainer-run, no agent dispatch); first-pass 1/1 agent tasks; review rounds 0 recorded (per-PR review threads: #116 needed one fix push for D-24, #118 one for a wrong URL); wall-clock 2026-10-04T11:39 run_start → 2026-10-04T23:09 last merge (~11.5h, mostly maintainer questionnaire sessions)

| Task | Agent | Attempts | Review rounds | PR | Merged | First pass |
|---|---|---|---|---|---|---|
| 1 | - | 0 | 0 | #111 | yes | no |
| 2 | - | 0 | 0 | #117 | yes | no |
| 3 | go-implementer | 1 | 0 | #118 | yes | yes |

## Lessons

- What worked: maintainer-domain tasks with orchestrator-drafted skeletons and paste-to-record walkthroughs (tasks 1–2) kept the external questionnaire honest — every claim re-verified live via `.json` rather than trusted from the browser paste (I07).
- What worked: the site's automation-proposal URLs (short `/passing/edit` chunks) turned a 60+-criterion questionnaire into reviewable diffs the maintainer could approve field by field.
- What worked: byte-verification of justifications in both directions (assessment → design §3, assessment → card refs) caught no drift because the checks ran at merge time, not after.
- What to change: `gh run list --commit` silently misses short SHAs, and `finalize.py ci` turns that into a false `verdict=pending` — see P-openssf-badge-1.
- What to change: DoD checkboxes in `requirements.md` shipped unchecked through three task reviews; only the spec-wide review caught it (F-1). One-off slip, no mechanism — stays a lesson.
- What to change: bestpractices.dev proposal URLs are finicky (`/en/`-prefixed badge paths 404, `/choose/edit` drops proposals where `/passing/edit` keeps them, long URLs fail silently) — recorded here for the next re-attestation; external quirk, no harness change.

## Proposals

- P-openssf-badge-1 — verify-ci passes the full commit SHA (short SHAs miss)
