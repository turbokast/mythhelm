## OpenSSF Best Practices Badge — Requirements

> Complete the OpenSSF Best Practices self-assessment to the passing level, display the badge in the README, and file a backlog card for every passing-level criterion the project does not yet meet. A slice of §19.3, §19.4 and the G10 gate of §18.7. Normative source: docs/spec/master-spec.md; § numbers, I-IDs and G-IDs refer to it.

## Context

- **Backlog card**: MH-9
- **Issue**: [turbokast/mythhelm#30](https://github.com/turbokast/mythhelm/issues/30) (read-only; work is discussed there, status lives on the card)
- **Source**: master spec §19.3, §19.4 and the G10 public-release gate of §18.7

MYTHHELM is a public open-source repository aiming at a public preview whose release gate (G10) requires licence, security policy, contribution path, provenance, changelog and limitations to be published. The OpenSSF Best Practices badge is an independent, externally verifiable checklist that the project's governance, contribution and CI/supply-chain posture meets a recognised passing bar. Earning it exercises the same artifacts G10 demands and gives contributors and downstream users a single visible signal.

The repository already publishes most of the G10 document set (LICENSE, NOTICE, SECURITY.md, CONTRIBUTING.md, GOVERNANCE.md, CODE_OF_CONDUCT.md, CHANGELOG.md) and carries CI, licence and OpenSSF Scorecard badges in the README, but no OpenSSF Best Practices badge: the acceptance state fails today. Because the project is pre-alpha with no releases yet, some passing-level criteria (notably around releases, provenance and published limitations) may not be satisfiable now; the card explicitly scopes those as new backlog cards rather than silent N/A answers.

Grounding verdicts (2026-10-02; no vendor second reader per dispatch):

- C1 HOLDS — master-spec §19.3 (governance/contribution), §19.4 (CI/supply-chain) and the §18.7 gate table exist: `docs/spec/master-spec.md:2059`, `:2067`, `:1934`.
- C2 HOLDS — G10 pass condition is "Licence, security policy, contribution path, provenance, changelog and limitations are published": `docs/spec/master-spec.md:1947`.
- C3 HOLDS — G10 document artifacts exist at the root: `LICENSE`, `NOTICE`, `SECURITY.md`, `CONTRIBUTING.md`, `GOVERNANCE.md`, `CODE_OF_CONDUCT.md`, `CHANGELOG.md` (observed via `ls -la`, 2026-10-02).
- C4 HOLDS — `README.md` exists and carries CI, OpenSSF Scorecard and licence badges only; no Best Practices badge (`grep -n -i "openssf|best practices|badge" README.md` shows lines 8–10, none a Best Practices badge). The badge criterion fails today.
- C5 PARTIAL — the cited sections exist and cover the badge's subject matter, but the master spec never names "OpenSSF Best Practices" (`grep -n -i 'openssf\|best.practice' docs/spec/master-spec.md` has no hits); the badge is the card's concrete interpretation of §19.3/§19.4/G10, not an explicit master-spec demand.
- C6 HOLDS (external) — the OpenSSF Best Practices program offers a self-assessed passing tier at bestpractices.coreinfrastructure.org, above which sit silver and gold (multiple public guides corroborate; questionnaire answers are maintained by the project owner, not committable repo state).
- C7 PROCEDURE — "record any unmet criterion as its own card" is a work instruction, not a codebase claim; carried into FR-3.

Failure reachable today: yes — the badge is absent and no assessment is linked, so every AC below fails on the current tree.

### Objectives

- **O1**: The project holds a passing-level OpenSSF Best Practices badge whose assessment URL is public and linked from the README.
- **O2**: Every passing-level criterion the project cannot yet meet is tracked as its own backlog card instead of a silent gap.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Silver or gold badge tiers. | G10: the passing badge and its unmet-criteria cards are published. |
| N2 | Satisfying currently-unmeetable criteria (releases, provenance) inside this spec. | FR-3: each becomes a card; no criterion is answered N/A without a card or a recorded justification. |
| N3 | Changes to CI workflows beyond what a passing criterion requires. | §19.4: existing checks keep passing. |

## Functional Requirements

Acceptance criteria use EARS. Tags in brackets name the invariants, gates and sections each one serves.

### FR-1 — Passing-level assessment completed (§19.3, §19.4, G10)

- **AC-1.1** [G10] The project shall hold a published OpenSSF Best Practices assessment at the passing level (100% of passing criteria answered passing or justified N/A), reachable at a stable `bestpractices.coreinfrastructure.org/projects/<id>` URL. The maintainer registers the entry under their personal bestpractices.coreinfrastructure.org account and owns future re-attestations.
- **AC-1.2** [§19.3, §19.4] If a passing criterion is answered N/A, then the assessment's justification text shall state why it does not apply, and the reason shall be recorded in this spec's `design.md`. No in-repo snapshot of the assessment state is kept — the external assessment URL is the only record.

### FR-2 — Badge displayed in the README (§19.3, G10)

- **AC-2.1** [G10] When the assessment reaches passing, `README.md` shall display the OpenSSF Best Practices badge image linking to the assessment URL, alongside the existing badges near `README.md:8-10`.
- **AC-2.2** [G10] If the assessment URL changes (re-registration), then the README link shall point at the current URL; the check fetches the README badge link target and compares its project id against the assessment URL of record (red when a re-registration changes the id without a README update).

### FR-3 — Unmet criteria become backlog cards (§19.3, §19.4, G10)

- **AC-3.1** [G10] For every passing-level criterion the project does not meet (e.g. no release exists yet, no build provenance, no published limitations page), the assessment shall answer N/A with a justification naming the gap, and the system of record shall contain a backlog card naming the criterion id and the gap; the count of such cards equals the count of unmet criteria. Pre-alpha-unmeetable criteria proceed as N/A-with-backlog-card now; the badge is not deferred until the first release.
- **AC-3.2** [§19.3] When all such cards are filed, the assessment shall reference them (in N/A justifications or the assessment's notes) so an auditor can trace each gap.

## Non-Functional Requirements

- **NFR-1** [G10] The badge image and link shall use the assessment site's canonical badge URL form, not a vendored copy that can go stale.

## Definition of Done

- [ ] AC-1.1 to AC-3.2 each have a named check; badge link verified live.
- [ ] Unmet-criteria cards filed through the normal backlog flow and linked from the assessment.
- [ ] `README.md` renders the badge beside the existing badges.

## Open Questions

- **Q1 — RESOLVED (2026-10-02)**: The bestpractices.coreinfrastructure.org entry is owned by the maintainer's personal account. The maintainer registers the entry and owns future re-attestations (see AC-1.1).
- **Q2 — RESOLVED (2026-10-02)**: The maintainer accepts N/A-with-backlog-card for pre-alpha-unmeetable criteria; proceed now, do not defer until the first release (see AC-3.1).
- **Q3 — RESOLVED (2026-10-02)**: External URL only — no in-repo snapshot of the assessment state; N/A reasons live in this spec's `design.md` per AC-1.2 (see AC-1.2).

## Dependencies

- Prerequisite: none in-repo — all G10 document artifacts exist and the README badge slot is free. External prerequisite: the maintainer registers and attests the assessment under their personal account and owns future re-attestations (Q1 resolved).
- Conflicting: none — `specs/in-progress/` and `specs/unfinalized/` are empty, and the one `specs/todo/` entry (`specs/*/docs-site-demos/`, covered by the Related note below) touches a disjoint README region (observed 2026-10-02). `specs/*/tui-slice/` (§15 TUI) touches `README.md` only in a status section (its `requirements.md:105`) disjoint from this spec's badges block; the two specs share no criteria and can land in either order.
- Related: `specs/*/docs-site-demos/` (§19.3, G10) also edits `README.md` (recording embeds, site URL link); the two specs touch disjoint README regions (badges block vs embeds/links) and can land in either order. Its coordination note (which spec publishes the contribution-path/security-policy content the site assembles) is answered by C3: the content already exists at the root; neither spec publishes new governance prose.
- Superseded: none. Reference: `specs/*/dogfood-slice/` is the spec-format reference only; unrelated in subject.
- Related decision record: `docs/decisions/0001-license-and-contribution-policy.md` (licence/contribution posture underlying §19.3 evidence).

## Impacted components

- `README.md` (root badges block, lines 8–10): add the Best Practices badge link — docs domain.
- External: the bestpractices.coreinfrastructure.org assessment entry (not repo state; FR-1/FR-3 evidence).
- `product/` (new cards for unmet criteria, via the normal backlog filing flow — not written by this spec's implementer directly).
