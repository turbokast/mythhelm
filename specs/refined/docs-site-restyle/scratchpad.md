# Docs Site That Shows the Product — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| Q1 | Can a Liquid-generated JSON search index be emitted under `actions/jekyll-build-pages` with `plugins: []` and no custom plugins? | Assume yes; Task 5 proves it in the CI build and stops and reports if it cannot, with the card's Starlight/mdBook revisit as the fallback (OQ-5, N1). | Task 5 / maintainer |
| Q2 | Which layout do front-matter-less pages receive today under Pages defaults? | Not relied on: Task 1 assigns the docs layout explicitly via `defaults:` in `docs/_config.yml` and Task 8 asserts the marker on every built page (AC-1.1). | Task 1 |
| Q3 | What is the exact set of published pages in `_site`? | Read from the built site at each check, never from the count 39 in requirements (AC-4.5, AC-7.1). | Tasks 5, 8, 9 |
| Q4 | Do the npm packages the browser driver needs all carry allow-listed licences? | Checked before the driver lands; any off-list licence stops Task 9 and reports, never widening the list (NFR-6). | Task 9 / maintainer |
| Q5 | Which Chrome does the driver use — the runner's preinstalled build or a checksum-verified download? | Runner's preinstalled Chrome with its version recorded, unless Task 9 finds it absent, in which case it records the choice and its checksum (NFR-6). | Task 9 |
| Q6 | Are the Paper/amber hexes and the four cell texts exactly as the design intended? | As decided in OQ-1 and OQ-11 from the maintainer's summary; the maintainer's review of the spec PR is the check, since the Claude Design file was not read. | Maintainer |

## Research notes

- Requirements grounding (2026-10-12, in `requirements.md`): live site measured at 390px (User guide 516px wide, 13 overflowing elements); front-matter-less Markdown is served as HTML (four sampled URLs returned 200), refuting the first draft's front-matter premise; Star light on Paper is 4.08:1 (WCAG, computed); no Ruby or Jekyll in the authoring worktree, so Q1 and Q2 are CI-proven.
- Investigation (this `/spec` run): `docs/_config.yml` is `theme: minima`, `plugins: []` with no `defaults:`; `.github/workflows/ci.yml` CI OK needs `[workflows, detect, go, lint, govulncheck, licenses, harness]` with no docs job; `.claude/data/required-checks.json` requires CI OK, DCO, Dependency review and the two Analyze jobs only — hence D6 (join CI OK's needs) instead of requiring Docs checks.
- The docs workflow builds with `actions/jekyll-build-pages` and uploads via `actions/upload-pages-artifact` after Assert mirrors rendered; the artifact hand-off point for D8 is immediately after that assert step.

## Discoveries

