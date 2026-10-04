# openssf-badge — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Register the assessment, answer the passing criteria, record the URL and N/A/Unmet table

- **Produces**: assessment URL of record `https://www.bestpractices.dev/projects/15212` (`badge_percentage_0` = 100, `badge_level` = passing, verified live 2026-10-04); site-generated markdown snippet `[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/15212/badge)](https://www.bestpractices.dev/projects/15212)`; design §3 rows for 3 Unmet + 6 N/A passing-tier answers with verbatim justifications.
- **For dependents** (task 2): the 3 Unmet criteria needing cards are `version_semver`, `version_tags`, `warnings_strict` (all SUGGESTED gaps, all pre-release). The 6 N/A rows take `—` in the card column. `achieve_silver` (Unmet, empty justification) is a derived next-level gate, not an answer — excluded from §3 and from the card count by design.
- **For dependents** (task 3): use the snippet bytes above verbatim; link-target id 15212 already equals the URL of record id.

## Task 2 — File backlog cards for unmet criteria and reference them from the assessment

- **Produces**: cards MH-18/19/20 (issues #113/114/115) for the 3 Unmet criteria; assessment justifications reference them; design §3 card column filled; `badge_percentage_0` = 100 unchanged.
- **For dependents** (task 3): no open items — the snippet bytes in task 1's handoff are final; the badge shows passing.

## Task 3 — Display the badge in the README and verify it live

- **Produces**: README line 11 carries the Task 1 snippet verbatim, linking to `https://www.bestpractices.dev/projects/15212`; badge image live at 200 `image/svg+xml`, showing passing.
- **For dependents**: terminal task — no dependents. Future README badge edits must keep the snippet bytes verbatim (link-target id 15212 = URL of record id).
