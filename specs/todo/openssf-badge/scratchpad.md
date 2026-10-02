# OpenSSF Best Practices Badge — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

Scope: single spec, ~3 tasks, one work stream. `scope auto-confirmed (non-interactive)`.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| 1 | Which host will the assessment URL use (`bestpractices.coreinfrastructure.org` or `www.bestpractices.dev`)? | The site-issued URL wins; either host with the right project id is accepted (design D5; redirect verified, see Research notes). | Maintainer / Task 1 |

## Research notes

- Badge program fetched 2026-10-02: `https://www.bestpractices.dev/en` (OpenSSF Best Practices Badge Program; voluntary self-certification; passing tier at `/en/criteria/0`; formerly CII Best Practices badge).
- Live probes 2026-10-02 with `curl -sSL` against project 1: `<project-URL>.json` → 200 JSON with `badge_percentage_0` (`100` ⟺ passing complete), `badge_level`, `<name>_status`/`<name>_justification` per-criterion keys, `repo_url`, `general_comments`; `/projects/<id>/badge` (non-locale form; the `/en/` form 404s) → `200 image/svg+xml` with `<title>openssf best practices: <level></title>`; `bestpractices.coreinfrastructure.org/projects/1` → 200 after redirect to `https://www.bestpractices.dev/en/projects/1/passing`.
- Backlog filing flow: `.claude/skills/backlog/SKILL.md` (`add` via `scripts/pm/pm.py` staging + pm-sync approval requests; GitHub issue creation needs explicit pre-approval in a non-interactive run, else report the `gh issue create` command).
- N/A answers count toward the passing percentage (grounds the AC-1.1/AC-3.1 interaction): sample project 1 JSON has 24 `<name>_status == 'N/A'` keys with `badge_percentage_0 == 100` (counted 2026-10-02).
- No unmet-criteria count is knowable before Task 1 walks the questionnaire; the count is task content, not a blocker (Tasks 1–2 acceptance covers both the zero and non-zero branches).

## Discoveries
