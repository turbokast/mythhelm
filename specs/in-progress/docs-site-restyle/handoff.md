# docs-site-restyle — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Brand foundation and site contracts (docs)

- **Produces**: Brand table in `docs/assets/brand/README.md` (9 tokens: Night `#090f1a`, Frost `#ebeff5`, Ink `#101621`, Star (dark) `#4ddae9`, Star (light) `#00829d`, Mist `#989fab`, Paper `#f6f4ee`, Amber (dark) `#f0c27b`, Amber (light) `#8a5a00`); `docs/assets/site.css` holds them as `--color-<slug>` properties (token lowercased, spaces to hyphens, parentheses removed, e.g. `--color-star-dark`) — Task 8 binds parity to that mapping; literals live only in the `:root` block, everything else is `var()`, and the two amber properties are declared but referenced by no rule.
- **Produces**: `docs/_layouts/default.html` (name kept) emits `<!-- mythhelm-layout:v1 -->` in `<head>`, body class `surface-night` when `page.path == "index.md"` else `surface-paper`, crest `mark-on-dark.svg`/`mark-on-light.svg` in the header, stylesheet `assets/site.css` only, and the retained `Rendered from commit …` footer. `docs/_config.yml` removes `theme: minima` (recorded in a comment) and assigns `layout: "default"` to every page via `defaults:` (scope `path: ""`); a page built without that assignment, hence without the marker, is the AC-1.1 counterfactual Task 8 automates.
- **Produces**: `docs/_data/status.yml` items are exactly `{id, section, quote, column, tracker}`; `section` is a `##` heading of `docs/limitations.md` (region to the next `##`), `column` is `works`/`in-progress`/`not-yet`, `tracker` is null except `in-progress`, where the tracker string occurs inside `quote`. Seeds: `offline-demo` (works), `supervisor-routing` (in-progress, `#348`), `plugins-routing` (not-yet); a cell's label is its item's `column`. Task 3 completes the items without changing the shape.
- **Produces**: `docs/_data/examples.yml` entries are `{anchor, command, expect_exit}` (+ `output_anchor` on output entries); `anchor` is `<page>#<heading-slug>[<n>]` with the page path from the repository root (e.g. `docs/user-guide.md#quickstart[1]`, the first entry, command exactly `go run ./cmd/mythhelm demo`, verified exit 0 under `GOPROXY=off`). Drift normalisation is `docs-output-v1` (CRLF-to-LF, trailing-whitespace trim); only Task 10 executes the output diff.
- **Produces**: `docs/_data/site-scripts.yml` holds `{assets/search.js, search}`, `{assets/site.js, sidebar}`, `{assets/site.js, player}`; `purpose` is exactly `search`/`sidebar`/`player` and a dual-role file gets two entries. The named scripts do not exist yet — Task 4 ships `site.js`, Task 5 ships `search.js`; Task 8 validates built scripts against the listed `src` values.
- **Produces**: Search-index record shape pinned for Task 5 (no file yet): `{title, group, headings: [{text, anchor}], body, truncated}`, built to `_site/search.json` from `docs/search.json` (`layout: null`, which overrides the `defaults:` assignment) and fetched via `{{ '/search.json' | relative_url }}`.
- **Produces**: `docs/demos/manifest.json` hero `chapters: [{name, at_seconds}]` in the order title, run, kill, recover, review with placeholder offsets 0/5/10/15/20. Task 2 must replace the offsets from the rendered video's duration; the tape, transcript and `binary_commit` pin are untouched.
- **For dependents**: Fonts are same-origin only — `@font-face` families `"Archivo Expanded ExtraBold"` (800) and `"JetBrains Mono"` (400) from `docs/assets/fonts/*.woff2` with the OFL notice and provenance record beside them; add no third-party load. Paper small text/links stay Ink with a Star-light underline (Star light on Paper is 4.08:1); Mist is a dark-surface token only.
- **Deviations that change a later task's inputs**: None; every contract above is the design's shape as shipped.

## Task 2 — Hero video, chapters and recording posters (docs)

<!-- pending -->

## Task 3 — Night landing page, status panel and tmux cells (docs)

<!-- pending -->

## Task 4 — Paper docs layout: grouped sidebar and on-this-page (docs)

<!-- pending -->

## Task 5 — Client-side search index and control (docs)

<!-- pending -->

## Task 6 — State labels and declared-not-verified notes (docs)

<!-- pending -->

## Task 7 — Tested-in-CI marker on code examples (docs)

<!-- pending -->

## Task 8 — Built-site content checks (harness)

<!-- pending -->

## Task 9 — Browser-driver carve-out and responsive checks (harness)

<!-- pending -->

## Task 10 — Site-build job gating CI OK and docs workflow wiring (release)

<!-- pending -->

## Task 11 — Build evidence, automation record and final conformance (docs)

<!-- pending -->
