## OpenSSF Best Practices Badge — Design

Scope: single spec, ~3 tasks, one work stream (external assessment + README badge). `scope auto-confirmed (non-interactive)`: the work is a strictly sequential chain (assessment URL → backlog cards → badge) and no part ships usefully alone. Implements `specs/*/openssf-badge/requirements.md` (ground truth; Q1–Q3 resolved there).

### 1. Current state

- Badges block `README.md:8-10` (find with `grep -n 'badge\.svg\|scorecard\.dev\|shields\.io/badge' README.md`): CI, OpenSSF Scorecard and Apache-2.0 badges. No Best Practices badge: `grep -n -i 'best practices' README.md` has no hits (observed 2026-10-02).
- G10 document artifacts exist at the root: `LICENSE`, `NOTICE`, `SECURITY.md`, `CONTRIBUTING.md`, `GOVERNANCE.md`, `CODE_OF_CONDUCT.md`, `CHANGELOG.md` (observed via `ls -la`, 2026-10-02).
- Master spec: §18.7 gate table `docs/spec/master-spec.md:1934`, G10 row `:1947` ("Licence, security policy, contribution path, provenance, changelog and limitations are published"); §19.3 `:2059`; §19.4 `:2067` (find with `grep -n '^### 18.7\|G10 — Public release\|^### 19.3\|^### 19.4' docs/spec/master-spec.md`).
- `docs/decisions/0001-license-and-contribution-policy.md` (accepted 2026-09-29): Apache-2.0 + DCO 1.1, no CLA — the evidence source for §19.3 questionnaire answers.
- The master spec never names "OpenSSF Best Practices" (requirements C5); the badge is MH-9's concrete interpretation of §19.3/§19.4/G10, not an explicit master-spec demand.
- External app verified live 2026-10-02: `https://www.bestpractices.dev/en` (OpenSSF Best Practices Badge Program; self-certification; passing tier at `/en/criteria/0`). The requirements' URL form resolves to the same app: `bestpractices.coreinfrastructure.org/projects/1` returns 200 after redirect to `https://www.bestpractices.dev/en/projects/1/passing` (same project ids on both hosts).
- Machine-readable status verified with `curl -sSL` against project 1 (2026-10-02): `<project-URL>.json` returns 200 JSON carrying `badge_percentage_0` (`100` ⟺ passing tier complete), `badge_level` (tier string), per-criterion `<name>_status`/`<name>_justification` keys, `repo_url` and `general_comments`. `/projects/<id>/badge` (non-locale path form; the `/en/`-prefixed form 404s) returns `200 image/svg+xml` with `<title>openssf best practices: <level></title>`.
- No in-repo badge-check infrastructure and no Go code in reach: `bestpractices` matches only this spec's own directory and `orchestration/QUESTIONS.md` (observed 2026-10-02).

### 2. Assessment registration and answers (FR-1; §19.3, §19.4, G10)

- The maintainer registers the project entry under their personal bestpractices account (Q1 settled) and records the stable `<host>/projects/<id>` URL as the **assessment URL of record** in the Task 1 completion entry and `handoff.md`.
- The maintainer walks every passing-tier criterion. Each answer cites repo evidence (file + line/section, e.g. the §1 artifacts) or N/A with a justification naming the gap (FR-3 feeds these).
- The maintainer copies the site-generated embed snippet verbatim from the project page and records it in the completion entry. Expected shape (pattern verified live, §1; `<project-URL>` is the non-locale `<host>/projects/<id>` form): `[![OpenSSF Best Practices](<project-URL>/badge)](<project-URL>)`; the site's own bytes win over this pattern.
- Host form: the site-issued URL wins. Either host with the right project id satisfies AC-1.1/AC-2.2 (verified same-project redirect, §1; D5).

### 3. N/A justification record (AC-1.2)

Task 1 adds one row per N/A answer; Task 2 fills the card column. Criterion ids are the app's criterion keys as they appear in the `.json` `<name>_status` keys (§1). Justification text is identical in the assessment and here.

| Criterion id | Why N/A | Backlog card |
|---|---|---|

Zero rows until Task 1 walks the questionnaire; an empty table with `badge_percentage_0 == 100` means nothing was N/A.

### 4. Backlog cards for unmet criteria (FR-3; §19.3, §19.4, G10)

- One card per unmet criterion via `/backlog add` (product-skills flow): the title names the criterion id and the gap; the summary grounds the gap at source; scored and filed for maintainer approval through the pm-sync flow, with a GitHub issue per that skill's second write gate. Pre-alpha-unmeetable criteria proceed as N/A-with-backlog-card now (Q2 settled); the badge is not deferred to the first release.
- Nobody writes `product/` directly (`knowledge/domains.md`); the task is maintainer-run and the cards land only through approval.
- After approval the maintainer references each card id from its N/A justification (or the assessment notes, AC-3.2) and fills the §3 card column.
- Count rule (AC-3.1): the number of cards equals the number of N/A answers; `check-cards-complete` compares the `.json` N/A count against the filed cards.

### 5. README badge (FR-2, NFR-1; §19.3, G10)

- Placement: append the snippet as a fourth line of the badges block after `README.md:10`, in the same image-link form as the sibling badges.
- Content: the Task 1 site-generated snippet verbatim; its link target must equal the assessment URL of record (host and id).
- Only when the assessment shows passing (AC-2.1): enforced by task order (Task 3 runs after Task 2's passing check).
- No vendored image (NFR-1): the `.../badge` URL serves live SVG (§1).

### 6. Verification checks (one named check per AC)

`<AU>` is the assessment URL of record. Every check states its red state.

- `check-assessment-passing` (AC-1.1): `curl -sSL <AU>.json` returns 200 with `badge_percentage_0 == 100` and `badge_level` in {passing, silver, gold}. Red: URL unregistered (non-200) or percentage below 100.
- `check-na-justified` (AC-1.2): every `<name>_status == 'N/A'` in the JSON has a non-empty `<name>_justification`, and the N/A id set equals the §3 table rows with identical text. Red: an unjustified N/A or a table mismatch.
- `check-readme-badge` (AC-2.1): the badges block (anchored region around the `grep -n 'badge\.svg\|scorecard\.dev\|shields\.io/badge' README.md` hits) contains the snippet verbatim with the `<AU>` link target, and `curl -sSL -o /dev/null -w '%{http_code} %{content_type}' <image-URL>` prints `200 image/svg+xml`. Red today: no badge line.
- `check-badge-link-current` (AC-2.2): the project id extracted from the README link target equals the id in `<AU>`. Red: a re-registration changes the id without a README update; also red on the pre-change README, where extraction finds no badge id.
- `check-cards-complete` (AC-3.1): the `.json` N/A count equals the count of backlog cards naming those criterion ids (`python3 scripts/pm/pm.py list` plus card inspection). Red: a gap without a card.
- `check-cards-referenced` (AC-3.2): each N/A justification or `general_comments` names its card id. Red: a justification without a card reference.

### 7. Task contracts

- Task 1 → Task 2: assessment URL of record; N/A criterion ids with justifications (§3 rows); site-generated snippet.
- Task 2 → Task 3: card id per N/A criterion (§3 card column); verified passing state.
- Record locations: each task's completion entry in `tasks.md` and its `handoff.md` section (seeded by run-spec). No Go signatures: the contracts are the URL, the snippet bytes and the §3 rows.

### 8. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | Paste the site-generated embed snippet verbatim (Task 1 records it; Task 3 applies it). | Alternative: hand-construct `<url>/badge`. The site's bytes are canonical per NFR-1 and survive a host rename; the pattern is verified live (§1) but the site wins. |
| D2 | One-time named curl/grep checks (§6); no CI job. | Alternative: a CI link-check workflow. N3 forbids CI changes beyond what a passing criterion requires, and none requires one. |
| D3 | Task 1 (maintainer) fills the §3 table in this file. | Alternative: scratchpad-only record. AC-1.2 mandates `design.md`; the edit is the authorized exception, declared in Task 1's Files. Completion entries still follow the tasks.md/scratchpad.md convention. |
| D4 | Strictly sequential tasks 1 → 2 → 3; no parallel group. | Alternative: {2, 3} parallel after 1. AC-2.1 shows the badge only when passing, and passing evidence completes in Task 2 (AC-3.2 references). |
| D5 | The site-issued URL is the URL of record; either host accepted. | Alternative: force the requirements' literal host. Verified redirect (§1): both hosts address the same project id, and AC-2.2 compares ids. |

### 9. Honesty register

| Spec demand | Position |
|---|---|
| G10 "provenance … published" | Not met in this spec: no releases exist pre-alpha, so release/provenance criteria go N/A-with-backlog-card per FR-3 (N2 still binds). |
| G10 "limitations … published" | Partially met: `README.md` carries a pre-alpha status section; any passing criterion demanding more goes N/A-with-backlog-card per FR-3. |
| §19.4 reproducible builds / SBOM / attestations | Deferred via FR-3 cards where the questionnaire demands them; not built here (N2). |
| §19.3/§19.4 demands surfaced as unmet criteria | N/A-with-backlog-card per FR-3; this spec satisfies none of them directly (N2). |
| Badge tiers above passing | Out of scope (N1); the checks accept passing-or-higher but no task pursues silver or gold. |

### 10. Cross-spec references

- `specs/*/tui-slice/` (README status section, `requirements.md:105`) and `specs/*/docs-site-demos/` (README embeds/links): disjoint README regions from this spec's badges block; all three can land in any order.
- `specs/*/dogfood-slice/`: spec-format reference only; unrelated in subject.
