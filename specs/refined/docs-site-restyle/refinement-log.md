# Refinement log — docs-site-restyle

dispatched=3 returned=3 failed=0
Verdict: Good enough after 3 rounds (no FAIL remains; round 3 left NEEDS_WORK items, all but the last applied afterwards without a fourth assessment). Run non-interactively on 2026-10-12; no human-required finding was raised in any round, and the open questions that need the maintainer carry a default (OQ-1, OQ-9, OQ-11).

## Round 1 — Needs human input

D1 PASS; D2 to D8 NEEDS_WORK. Twenty auto-fixes applied, among them: the amber premise corrected (`#8a5a00` is already the TUI light warning colour; Paper is in no file); the domain list corrected to docs, release and harness with a required architect review; NFR-4 no longer forbids the spec's own checks; "unchanged" CI checks restated as "none weakened"; AC-2.3's check bound to a quoted phrase under a named section with a rejecting fixture; AC-5.2, AC-6.4 and NFR-3 made checkable; O1 given viewport sizes; the Definition of Done completed; windows-process-ownership and claude-strict-subscription named as content conflicts.

Four human-required questions were raised (the panel's "in progress" source, the four cell texts, the palette values and amber rule, whether post-build checks gate merges). They were not answered; each was recorded as an open question with a conservative default (OQ-9, OQ-1, OQ-11, OQ-10).

## Round 2 — Needs work

D1 PASS; D3 FAIL; others NEEDS_WORK; no human-required finding. The FAIL was a refuted premise: the spec said only pages with front matter are rendered, while the live site serves front-matter-less Markdown as HTML too (HTTP 200 on four sampled pages). "Published page" is now defined from the built site, the REFUTED verdict is recorded, and OQ-8 was rewritten. Other fixes: rejecting fixtures for four checks, the amber check made well defined, a closed-palette default, reduced-motion posters for the GIFs, the AC-2.6 exception to N2, one meaning of "in progress", OQ options annotated with the ACs they would amend, OQ-12 routed by domain, the other-spec dependency facts corrected.

## Round 3 — Needs work

D1, D4 and D5 PASS; D2, D3, D6, D7 and D8 NEEDS_WORK; no FAIL. Eleven auto-fixes were written into `requirements.md` after the round, not re-assessed: AC-5.2's code-span rule, OQ-1's default aligned with AC-2.1's order, an explicit `defaults:` layout entry with a built-site marker check, three more rejecting fixtures, NFR-5 (a 300 KB search-index ceiling, a default for the maintainer), poster generation in `docs/demos/record.sh`, `package.json` placed under `scripts/ci/`, the hosting job named for the new checks, `docs/limitations.md` marked not edited beyond front matter, windows-process-ownership Tasks 10 and 11 named, OQ-9 restated against the file's four sections.

## Remaining notes for /spec

- The layout that front-matter-less pages actually receive, whether a Liquid-built index works under `actions/jekyll-build-pages`, and the real set of published pages in `_site` were never observed with a local build; the first task proves them in CI.
- The 516px and 13-element figures and the HTTP 200 results were measured against the live site by the author, not by an assessor.
- The NFR-5 ceiling (300 KB) and the landing weight (500 KB) are defaults for the maintainer.
- The Paper and amber hexes and the four cell texts rest on the maintainer's design summary only (OQ-1, OQ-11); the Claude Design file was not read.
- The AC count (about 35) is large for one spec; `/spec`'s scope step may split the checks from the layout work if the task count exceeds the effort budget.

## Maintainer decisions and architect review (2026-10-12)

The coordinator relayed the maintainer's acceptance of all twelve defaults. They were folded into the acceptance criteria and `requirements.md` now has a Decisions section in place of Open Questions. An `architect` review (the spec spans docs, release and harness) then returned 2 blockers, 10 should-fix findings and 11 notes.

Resolved in the spec text: S1 (build once, upload the artifact before any third-party step, fixtures to a separate destination), S2, first part (NFR-6 supply-chain terms), S3 (column, text and tracker binding for the panel), S4 (cells bound to `status.yml` ids; the headline carved out as a maintainer statement), S5 (a marker with no entry fails), S6 (source-markup check for state words), S7 (origin check over CSS, media and recorded requests), S9 (fixture homes), S10 (overflow clip, computed contrast, video duration), and notes N1 to N4 and N6 to N11 (N3: state-word markup also applies in `docs/limitations.md`, never its headings or the Windows lines).

Needs a maintainer call, recorded in the spec and not resolved:
- B1 (OQ-10 reopened): `Docs checks` is not a required check, so the new checks would be advisory. Options: require it in the ruleset (and in `docs/automation.md` and `.claude/data/required-checks.json`), or add a site-build job to `ci.yml`'s `needs:`.
- B2 (OQ-5 reopened): a Node and Chrome driver under `scripts/ci/` contradicts `.claude/rules/harness-scripts.md`. Options: a carve-out for one browser-driver directory, a release-owned path, or no browser measurement.
- S2, second part (a separate sub-decision from the NFR-6 terms above): whether the driver's npm licences fit the dependency-review allow-list.
- S4: whether the headline's panel link is "exactly one" rather than "at most one".
- S8: whether the amber rule is stated for the docs site only, since the shipped TUI light theme uses `#8a5a00` as its warning colour.
- N5: per-item confirmation of the twelve decisions, which arrived as one relayed acceptance.
