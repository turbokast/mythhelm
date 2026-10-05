# docs-site-demos — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Site scaffold: config, nav, layout, home

- **Produces**: `docs/` is the Jekyll source root; `_data/navigation.yml` is the nav (schema: `- name:`/`  path:` with `path` relative to `docs/`); `_layouts/default.html` overrides minima with a header nav loop and a `Rendered from commit {{ site.github.build_revision }}` footer; `index.md` home stub.
- **Produces**: `_config.yml` sets `theme: minima`, `plugins: []`, `url: https://turbokast.github.io`, `baseurl: /mythhelm`; no `exclude:` (tapes ship as plain text per design §2).
- **For Task 2**: the nav already names all seven routes — create the six missing files (`user-guide.md`, `contributing.md`, `limitations.md`, `mirror/SECURITY.md`, `mirror/CHANGELOG.md`, `mirror/LICENSE.md`) so the route loop prints nothing.
- **For Task 2**: authored pages start with the §2 front matter block (`---` / `layout: default` / `title: …`); mirrors carry no front matter in the tree and live under `docs/mirror/` (never `_mirror/`).
- **For Tasks 2–3**: the layout links nav entries as `{{ site.baseurl }}/{{ path | replace: '.md', '.html' }}`; keep nav `path` values as `.md` sources.
- **For dependents**: keep every scaffold URL on `github.com/turbokast/mythhelm` or `*.github.io` (I13); the layout loads no external assets.

## Task 2 — Guide pages and root-guide mirrors

<!-- pending -->

## Task 3 — Pages build-and-deploy workflow

<!-- pending -->

## Task 4 — Demo tape, transcript and manifest

<!-- pending -->

## Task 5 — Rendered demo GIF and embeds

<!-- pending -->

## Task 6 — Recording honesty checks in CI

<!-- pending -->

## Task 7 — TUI tape scaffold (FR-3 follow-up filed; tui-slice shipped)

<!-- pending -->
