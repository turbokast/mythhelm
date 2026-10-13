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
- The docs workflow builds with `actions/jekyll-build-pages` and uploads via `actions/upload-pages-artifact` after Assert mirrors rendered; the upload step currently precedes that assert (`.github/workflows/docs.yml:177-178`); D8 and Task 10 prescribe moving the hand-off to immediately after the assert step.

## Discoveries

- Task 1 (PR #464): the worktree has no Ruby/Jekyll, so the Jekyll build is proven by the PR's Build docs site CI job and local evidence is a source-level hand-run probe (45 checks: YAML, anchored `status.yml` quotes, examples-anchor resolution, brand-table↔CSS parity, font SHA-256). Font subsets were instanced/subsetted locally with fontTools 4.63.0 from the Google Fonts OFL copies (Archivo Version 2.001, JetBrains Mono Version 2.211; neither declares a Reserved Font Name); the shipped `--color-<slug>` mapping, `surface-night`/`surface-paper` classes and repo-root example anchors (`docs/user-guide.md#quickstart[1]`) are pinned in `handoff.md` for Tasks 2–10.
- Task 2 (PR #465): VHS `-o` adds outputs to the tape's own GIF rather than replacing it, so one hero run writes GIF+WebM+MP4; both videos probe at 32.24s (VP9/H.264, 1280x650) against the GIF's 32.16s. The demo/tui first GIF frames are the blank prompt — posters must seek to the first held content frame (demo 5s, tui 6s) while the hero poster is frame 0 (title card). This branch is never at the manifest pin, so shipped media was rendered with the pin-built binary via `record.sh`'s own commands and the updated `record.sh --repin` was proven in a throwaway worktree whose pin moves were discarded; the tape/transcripts/pins in the PR are untouched for Task 8's fixtures to build on.

- Task 3 (PR #466): the attention cell had no matching limitation phrase; the maintainer approved adding the exact not-yet sentence and the narrow N2 exception. Landing CSS and chapter JS moved forward from Task 4 to deliver working controls. Jekyll v1.0.13 builds offline with the existing container image and workflow mirror injection; 40 HTML pages were checked. Browser chapter evidence needs HTTP Range support: a plain Python static server exposed seekable `[0,0]` and reset seeks, while the Range-capable server proved all five manifest offsets. Chapter interaction changes preload to metadata only on request; the untouched page requests neither media nor manifest.

- Task 5 (PR #468): the pinned Pages build supports the Liquid index without custom plugins. Records are keyed by emitted URL to preserve the pinned five-field shape. UTF-8 size uses URL-encoded byte accounting, and whole bodies are omitted largest-first (current index 297,258 bytes, master-spec only); a Unicode/ASCII oversized fixture proves byte ordering. Built corpus has 40 pages and 377 indexed heading labels/anchors. Same-page search results need focus restoration with `preventScroll` or the asynchronous dialog close event scrolls back to the header; native search inputs need explicit Escape handling to close on the first keypress as documented.
