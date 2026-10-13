## Docs Site That Shows the Product — Tasks

### Dependencies

- Prerequisite specs: none outside this spec. `specs/*/docs-site-demos/` and `specs/*/protected-acceptance/` are shipped and extended, not superseded (requirements Dependencies).
- Task 1 ships the contracts every later task consumes (brand token table, layout marker `<!-- mythhelm-layout:v1 -->`, `docs/_data/status.yml` schema, examples-list schema, search-index shape, `docs/demos/manifest.json` chapter schema; the state include contract is shipped by Task 6, before Task 8 consumes it). No later task edits a contract's shape; a needed change is a spec deviation recorded in that task's completion entry.
- Layout ownership and order: `docs/_layouts/default.html` is edited by Tasks 1, 3, 4, 5 and 7 in that dependency order (Task 3 → Task 4 → Task 5 → Task 6 → Task 7 via their `Depends on` chains; Task 6 edits no layout but sits in the chain for `docs/user-guide.md` ordering) and those tasks never run in parallel with each other. Task 2 (demos only) may run in parallel with any one layout editor. Task 8 may start fixtures against Task 1 contracts early but its content checks finalise after Tasks 3, 5, 6 and 7; it runs parallel to Task 9. Task 10 depends on Tasks 8 and 9. Task 11 depends on all.
- Shared files that forbid parallelism: `docs/_layouts/default.html` (Tasks 1, 3, 4, 5, 6, 7 — dependency-ordered, never parallel), `docs/_data/navigation.yml` (existing file extended by Tasks 3 and 4 — Task 3 adds governance, Task 4 adds the group data), `docs/demos/manifest.json` (Tasks 1 and 2 — sequential), `docs/user-guide.md` (Tasks 6 and 7 only — sequential; Task 4 does not edit it), `docs/index.md` (Tasks 3 and 7 — Task 7 depends on Task 3 through the layout chain), `docs/assets/site.css` (Tasks 1, 4 and 6 — dependency-ordered).
- **Gating (settled):** Tasks 9 and 10 implement OQ-5 (Node carve-out) and OQ-10 (site-build job joining `CI OK`). `product/decisions.md` D-104 recorded them as open; the superseding entry D-106 (request `docs-site-gating-decisions`) is approved and applied on this branch, so both tasks may start.
- **Gates for every task.** Format first, then the domain's gates from `.claude/skills/quality-gates/SKILL.md` run through `scripts/harness/gate.sh`: docs tasks run `scripts/harness/gate.sh harness` (specs lint) and `scripts/ci/check-public-hygiene.sh`; harness tasks also run `shellcheck` on changed `.sh` and `.claude/hooks/tests/run-tests.sh` when hooks are touched; release tasks run `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12` on changed workflows. Files existing or an agent reporting success is not completion: the gate marker must match the pushed tree.
- **Completion convention.** The implementing agent appends ` ✅ COMPLETED` to its task heading and adds `Status` (`✅ Completed — …; PR #<n>`), `Implementation` (with commit SHAs), `Spec deviations` (`None`, or each deviation with its reason) and `Files modified`, keeping every original field (`.claude/skills/task-completion/SKILL.md`; checked by `scripts/harness/runspec.py entry-check`). Every task also appends a Discoveries entry to `scratchpad.md`.
- **Commits**: signed off (`git commit -s`) with named paths only. The brand palette and gating decisions (OQ-10, OQ-11) are already recorded in `requirements.md` Decisions; no new ADR is created by these tasks.

---

## Implementation Tasks

### Task 1 — Brand foundation and site contracts (docs) ✅ COMPLETED

- **Domain/agent**: go-implementer (docs domain, `knowledge/domains.md`)
- **Budget**: complex (12 files: the brand table, config, layout, stylesheet, four font files and four data/manifest contracts are one inseparable contract set — splitting them would let later tasks build against half-pinned shapes; the completion entry records the file count against the 8-file guideline)
- **Depends on**: None
- **Change**: Lay the foundation every later task builds on: brand tokens with Paper and amber in the brand README, the Night and Paper layouts keeping the name `default`, the stylesheet holding all colours as token-named custom properties, self-hosted fonts, an explicit `defaults:` layout assignment, and the checked-in schemas for `status.yml`, the examples list, the search index and the manifest chapters, so harness tasks can write checks against fixed shapes.
- **Files**:
  - `docs/assets/brand/README.md` (add Paper `#f6f4ee`, Amber (dark) `#f0c27b`, Amber (light) `#8a5a00` with the amber rule; update the font non-redistribution sentence, AC-1.2, AC-1.4)
  - `docs/_config.yml` (add `defaults:` assigning the docs layout to every page; record the `theme: minima` choice, AC-1.1)
  - `docs/_layouts/default.html` (Night landing and Paper docs presentation with the layout marker; crest mark in the header, AC-1.1)
  - `docs/assets/site.css` (new: all colour literals as token-named custom properties, Star the only accent, no amber `var()` outside token declarations, AC-1.2, AC-1.3)
  - `docs/assets/fonts/archivo-expanded-extrabold.woff2` (new subset, AC-1.4)
  - `docs/assets/fonts/jetbrains-mono.woff2` (new subset, AC-1.4)
  - `docs/assets/fonts/OFL.txt` (new: OFL notice shipped beside the fonts, AC-1.4)
  - `docs/assets/fonts/README.md` (new: source release, SHA-256, subsetting command and Reserved Font Name record, AC-1.4)
  - `docs/_data/status.yml` (new: schema header comment — `id`, `section`, `quote`, `column`, `tracker` — with the first items quoted from `docs/limitations.md`, AC-2.3)
  - `docs/_data/examples.yml` (new: schema header comment — block anchor, command as written, expected exit code — first entry the quickstart, AC-6.1, AC-6.2)
  - `docs/_data/site-scripts.yml` (new allow-list: YAML list of `{src, purpose}`, `purpose` in `search`, `sidebar`, `player`, naming `assets/search.js` and `assets/site.js`; consumed by Task 8, AC-1.5)
  - `docs/demos/manifest.json` (add the `chapters` schema — `name`, `at_seconds` — to the hero recording with placeholder offsets Task 2 renders against, AC-2.2)
- **Produces**: brand colour table (token name → hex) in `docs/assets/brand/README.md`; layout marker `<!-- mythhelm-layout:v1 -->` in every built page's `<head>`; `docs/_data/status.yml` item shape `{id, section, quote, column, tracker}` with `column` exactly one of `works`, `in-progress`, `not-yet`; `docs/_data/site-scripts.yml` entries `{src, purpose}` with `purpose` exactly one of `search`, `sidebar`, `player` (a dual-role file such as `assets/site.js` gets two entries with the same `src`, one per purpose; Task 8 validates each entry's `purpose` against that set and each built script against the listed `src` values); `docs/_data/examples.yml` entry shape `{anchor, command, expect_exit}` where `anchor` is `<page>#<heading-slug>[<n>]` (docs-relative page, GitHub slug of the nearest preceding `##` heading, 1-based block ordinal under it), an output entry also carries `output_anchor` in the same format, and drift compares block lines exactly under the fixed normalisation `docs-output-v1` (CRLF-to-LF, trailing-whitespace trim); the output diff itself is executed only by Task 10's examples job; search-index record shape `{title, group, headings: [{text, anchor}], body, truncated}` (consumed by Task 5); `manifest.json` hero `chapters: [{name, at_seconds}]` (consumed by Task 2). Later tasks consume these shapes verbatim.
- **Acceptance**:
  - The Jekyll build in CI emits every published page with the layout marker and the Paper or Night surface class; a fixture page built without the `defaults:` assignment is named as the failing counterfactual the Task 8 check later automates (AC-1.1).
  - `grep -E "#[0-9a-fA-F]{3,8}|rgb\(|hsl\(" docs/assets/site.css` lists only values equal to a brand-table hex, plus `transparent` and `currentColor`; a stylesheet holding an unlisted hex is the failing fixture Task 8 automates (AC-1.2).
  - No rule outside the token declarations in `docs/assets/site.css` references an amber custom property (`grep -n "amber" docs/assets/site.css` hits only the declarations) (AC-1.3).
  - The built landing and docs pages load fonts only from the site origin (recorded from the CI build's request log or source audit); the font directory carries the OFL notice and the source, SHA-256 and subsetting record (AC-1.4, AC-1.5 source leg).
  - `docs/_data/status.yml` and `docs/_data/examples.yml` parse as YAML, every `status.yml` quote occurs under its named heading in `docs/limitations.md` (checked by the Task 8 script run by hand here), and the first examples entry's command is exactly `go run ./cmd/mythhelm demo` (AC-2.3, AC-6.1).
- **Test plan**: Build the site in the task's CI run (the existing Build docs site job) and audit the emitted HTML and CSS by hand; the automated fixtures for these assertions ship with Task 8, so this task records the manual commands and outputs in its completion entry as the red-to-green evidence.
- **Invariants touched**: I09 (v2 §2: token and label values are stated, never implied); I14 (v2 §2: the brand table is the versioned source the checks bind to); G10 (v2 §18.3: the footer and marker contracts the later G10 checks assert).
- **Status**: ✅ Completed — Brand tokens (Paper/amber), marker layout on Night/Paper surfaces, token stylesheet, self-hosted OFL font subsets and the status/examples/script/search/manifest contracts landed; PR #464.
- **Implementation**: One commit `9ceac13` carrying exactly the 12 files in `Files` (count recorded against the 8-file guideline per the task Budget: the contract set is inseparable). Manual red-to-green evidence: before, no marker/Paper/amber/`site.css`/fonts/data/`chapters`; after, the 45-check hand-run probe (YAML parse, anchored quotes, tracker-in-quote, anchor resolution, table↔CSS parity both ways, marker/surfaces, recorded font SHA-256) prints ALL PASS, the quickstart under `GOPROXY=off` exits 0, and the failing counterfactuals named for Task 8 are the no-`defaults:` fixture page without the marker and the unlisted-hex stylesheet. Built-site proof is the PR's Build docs site CI job (no Ruby/Jekyll in the worktree).
- **Spec deviations**: None.
- **Files modified**: `docs/assets/brand/README.md`, `docs/_config.yml`, `docs/_layouts/default.html`, `docs/assets/site.css`, `docs/assets/fonts/archivo-expanded-extrabold.woff2`, `docs/assets/fonts/jetbrains-mono.woff2`, `docs/assets/fonts/OFL.txt`, `docs/assets/fonts/README.md`, `docs/_data/status.yml`, `docs/_data/examples.yml`, `docs/_data/site-scripts.yml`, `docs/demos/manifest.json`, `specs/in-progress/docs-site-restyle/tasks.md`, `specs/in-progress/docs-site-restyle/handoff.md`, `specs/in-progress/docs-site-restyle/scratchpad.md`.

### Task 2 — Hero video, chapters and recording posters (docs) ✅ COMPLETED

- **Domain/agent**: go-implementer (docs domain)
- **Budget**: complex (8 files including two rendered videos and three posters; media rendering plus manifest offsets in one task)
- **Depends on**: Task 1 (manifest chapter schema)
- **Change**: Render the hero recording as web video from the same tape and give every recording a checked-in poster, so the landing page can play the real run with chapter controls instead of an autoplaying GIF.
- **Files**:
  - `docs/demos/record.sh` (render `hero.webm` and the MP4 fallback from `hero.tape`; generate a static poster for each of `hero.gif`, `demo.gif` and `tui.gif`, AC-2.2, AC-7.4)
  - `docs/demos/manifest.json` (fill the hero `chapters` offsets — title, run, kill, recover, review — from the rendered files, AC-2.2)
  - `docs/demos/hero.webm` (new rendered artifact)
  - `docs/demos/hero.mp4` (new rendered fallback)
  - `docs/demos/hero-poster.png` (new checked-in poster for `hero.gif`, AC-7.4)
  - `docs/demos/demo-poster.png` (new checked-in poster for `demo.gif`, AC-7.4)
  - `docs/demos/tui-poster.png` (new checked-in poster for `tui.gif`, AC-7.4)
  - `docs/demos/README.md` (document the video render, the posters and the chapter offsets)
- **Produces**: `hero.webm` and `hero.mp4` rendered from `docs/demos/hero.tape`; poster images beside each GIF (paths named in the completion entry); final `chapters` offsets in `docs/demos/manifest.json` (consumed by Task 3).
- **Acceptance**:
  - `hero.webm` and `hero.mp4` exist, are rendered from the checked-in `hero.tape` by `docs/demos/record.sh`, and the manifest chapter offsets are strictly increasing and no greater than the rendered video's duration read from the file (e.g. `ffprobe`), not from a manifest field; a fixture manifest with non-increasing or over-duration offsets is the failing case Task 8 automates (AC-2.2).
  - Every GIF in `docs/demos/` has a checked-in poster generated by `record.sh`, and site content is not generated in CI (the posters are committed, NFR-4) (AC-7.4).
  - The existing transcript-freshness assertions still pass unchanged: `docs/demos/hero.tape`, `hero.transcript.txt` and the manifest `binary_commit` pin are untouched by the render (AC-2.2, N5).
- **Test plan**: Run `docs/demos/record.sh` locally where VHS and ffmpeg exist, or record in the completion entry that the render ran in CI/manual maintainer step with its command; verify durations and offsets with `ffprobe` and a small Python check that Task 8 later automates.
- **Invariants touched**: I14 (v2 §2: the recording is the real pinned run, labelled scripted); G10 (v2 §18.3: no existing docs assertion weakened, N5).
- **Status**: ✅ Completed — Hero tape rendered as checked-in WebM/MP4 with one poster per GIF and the manifest chapters filled from the rendered durations; PR #465.
- **Implementation**: `record.sh` renders both videos in the hero VHS run (`-o` adds outputs), grabs each poster with `ffmpeg` (hero 0s title card; demo 5s/tui 6s first held content frames) and verifies codecs, dimensions, posters and ffprobe-fitted chapters each run. Commit f83856661dd1787ba7a86fd04c1404505a6cf115. Media rendered with the pinned binary (built at `031676ad` in a temporary worktree) through the same commands; the full updated `record.sh --repin` then passed in a throwaway worktree (`hero chapters [0, 2, 13, 19, 26] fit the rendered durations (webm 32.24s, mp4 32.24s)`, `ok (… verified)`, exit 0; its pin moves discarded with it).
- **Spec deviations**: None.
- **Files modified**: `docs/demos/record.sh`, `docs/demos/manifest.json`, `docs/demos/hero.webm`, `docs/demos/hero.mp4`, `docs/demos/hero-poster.png`, `docs/demos/demo-poster.png`, `docs/demos/tui-poster.png`, `docs/demos/README.md`, `specs/in-progress/docs-site-restyle/tasks.md`, `specs/in-progress/docs-site-restyle/handoff.md`, `specs/in-progress/docs-site-restyle/scratchpad.md`.

### Task 3 — Night landing page, status panel and tmux cells (docs) ✅ COMPLETED

- **Domain/agent**: go-implementer (docs domain)
- **Budget**: standard
- **Depends on**: Task 1 (status schema, layouts), Task 2 (hero media and chapters)
- **Change**: Build the landing page in the required order with the hero player, the four capability cells bound to status items, the generated works / in progress / not yet panel and the G10 footer, so the first screen shows the product without claiming more than ships.
- **Files**:
  - `docs/limitations.md` (add only "The attention queue for approvals is not supported yet." under Not supported yet so the attention status item has its required source phrase, AC-2.3, AC-2.4)
  - `docs/index.md` (nav, headline, quickstart, hero `<video>` with chapter buttons, four cells each naming a `status.yml` id, panel, footer order; exactly one panel link under the headline, AC-2.1, AC-2.2, AC-2.4)
  - `docs/_layouts/default.html` (landing presentation, hero player markup, footer with the six G10 links, AC-2.1, AC-2.5)
  - `docs/_data/status.yml` (complete the panel items: works from Supported today and limits caveats, in-progress only with a tracker inside the quoted region, the rest not-yet, AC-2.3, OQ-9)
  - `docs/_data/navigation.yml` (add the governance page, AC-2.5)
- **Produces**: landing page element order and the panel rendering from `docs/_data/status.yml` (consumed by Tasks 8 and 9 for phrase, label and position checks).
- **Acceptance**:
  - At 1280px and 390px the built landing page shows nav (crest, search placeholder, pre-alpha pill), headline, quickstart, hero recording, four cells, panel and footer in that order, with the quickstart's top edge inside the first 800px / 844px of height; the Task 9 browser pass later automates the measurement, and this task records the built HTML order as evidence (AC-2.1, O1).
  - The hero `<video>` does not autoplay, carries a poster, and its chapter buttons seek to the manifest offsets (AC-2.2).
  - Every panel item's displayed text equals its quoted phrase under its named `docs/limitations.md` heading, its column matches its source section, and each cell's label equals its `status.yml` item's label (attention labelled "not yet"); a removed phrase or a mismatched label is the failing fixture Task 8 automates (AC-2.3, AC-2.4).
  - Every published page's footer links licence, security policy, changelog, governance, limitations and contributing, each target existing in the built site, and the "Rendered from commit …" line remains (AC-2.5).
  - The landing quickstart block text equals the user guide's quickstart command text (AC-2.6; Task 6 keeps them equal when it marks up the guide).
- **Test plan**: Build the site and inspect the emitted `index.html` order, panel text and footer hrefs; run the Task 8 phrase check by hand against the built tree and record the output.
- **Invariants touched**: I09 (v2 §2: panel labels are the recorded state, never marketing copy); I14 (v2 §2: every cell binds to a limitations section or is labelled not-yet); G10 (v2 §18.3, AT-46: footer carries the public-release pages).

- **Status**: ✅ Completed — Night landing page with working manifest-backed hero chapters, limitations-bound capability cells and status panel, and six-link public footer; PR #466.
- **Implementation**: Commit 4e5a72b (Codex, docs implementer equivalent). The player fetches chapter offsets on request, promotes preload only for that request and seeks without starting playback. Status text and labels render from the existing data schema; verification limits nest under the working item.
- **Spec deviations**:
  - `docs/limitations.md` and `specs/in-progress/docs-site-restyle/requirements.md`: maintainer-approved missing attention-queue sentence and narrow N2 exception; the Task 3 Files list now includes the source sentence. This repairs the absent evidence required by AC-2.4 without claiming the capability ships.
  - `docs/assets/site.css` and `docs/assets/site.js`: bring landing styles and hero chapter wiring forward from Task 4, whose Files list already owns both paths, to satisfy Task 3 responsive and seeking acceptance now. Task 4 extends these files for sidebar and GIF-poster controls.
  - The search placeholder is a plain noninteractive label. Task 5 supplies the control/index under its acceptance: "A query matching a heading or (non-truncated) body text lists matching pages and sections with title/heading matches first, each linking to its anchor; a query matching nothing says so, and a body-text miss on a truncated page is not presented as "no match" (AC-4.2)." No search input is exposed before that wiring exists.
  - `specs/in-progress/docs-site-restyle/tasks.md`, `specs/in-progress/docs-site-restyle/handoff.md` and `specs/in-progress/docs-site-restyle/scratchpad.md`: required completion, dependency handoff and discovery records.
  - Review corrections, consistent with design §3.2: nest the verification caveat under its working item, request metadata on chapter interaction and expand the supervisor quote to retain its antecedent. Independent reviewer confirmed both findings resolved; parent desktop/mobile confirmation passed. External review round 1 adds `role="group"` to the labelled chapter container so its accessible name is exposed (consistent with AC-2.2 and design §3.2).
- **Acceptance evidence**: Offline Jekyll v1.0.13 build (2.771s); 9 anchored rendered quotes/columns/trackers; four matching cell labels; 40 built pages with six existing footer targets and revision lines; exact quickstart parity; built element order. Chrome first interaction Recover seeks 19s, then title/run/kill/recover/review seek 0/2/13/19/26s, paused throughout; initial media/manifest requests zero. Quickstart top is 413.375px at 1280×800 and 467.5px at 390×844, no overflowing elements, chapter targets at least 44px. Permanent rejecting fixtures belong to Task 8.
- **Files modified**: `docs/index.md`, `docs/_layouts/default.html`, `docs/_data/status.yml`, `docs/_data/navigation.yml`, `docs/limitations.md`, `docs/assets/site.css`, `docs/assets/site.js`, `specs/in-progress/docs-site-restyle/requirements.md`, `specs/in-progress/docs-site-restyle/tasks.md`, `specs/in-progress/docs-site-restyle/handoff.md`, `specs/in-progress/docs-site-restyle/scratchpad.md`.

### Task 4 — Paper docs layout: grouped sidebar and on-this-page (docs) ✅ COMPLETED

- **Domain/agent**: go-implementer (docs domain)
- **Budget**: standard
- **Depends on**: Task 1, Task 3
- **Change**: Give every docs page the Paper layout with a grouped sidebar, a build-time on-this-page list and a bounded prose column, on desktop and phone, with and without JavaScript.
- **Files**:
  - `docs/_data/navigation.yml` (add `group` — Start, Use, Understand, Reference — to every entry as data; user-guide deep links per group, AC-3.1, OQ-7)
  - `docs/_layouts/default.html` (sidebar rendering from the group data with the current page marked, on-this-page list for pages with ≥2 second-level headings, 80-character prose column with scrolling `pre`/`table` containers, collapsible sidebar control, AC-3.1 to AC-3.4)
  - `docs/assets/site.css` (sidebar, list and prose-column styles within the token palette, AC-3.3)
  - `docs/assets/site.js` (new: sidebar toggle, hero chapter wiring and GIF poster start-on-request, listed in `docs/_data/site-scripts.yml` with two entries of the same `src` — `sidebar` and `player` — AC-3.4, AC-2.2, AC-7.4)
- **Produces**: grouped navigation rendering and the on-this-page list markup (consumed by Tasks 8 and 9 for coverage and target-size checks).
- **Acceptance**:
  - The built sidebar shows at most four groups (Start, Use, Understand, Reference) with the current page marked, driven by `group` data in `docs/_data/navigation.yml` — changing a group in the data changes the built sidebar without a layout edit (AC-3.1).
  - A built page with two or more `##` headings shows an on-this-page list linking to their anchors and a page with fewer shows none; the four mirrors render with the same layout, sidebar and list (AC-3.2, AC-3.5).
  - No built page's prose exceeds 80 characters per line by layout, and a long `pre` or `table` scrolls inside its container (the Task 9 pass later measures page width; this task records the CSS and a built-page inspection) (AC-3.3).
  - At a narrow viewport the sidebar collapses behind a labelled control operable by keyboard and touch and, with JavaScript disabled, remains reachable as a plain link list (AC-3.4).
  - A recording served as a poster shows only the poster image and its labelled start control until the control is used; using it swaps in the GIF (hero: the AC-2.2 video), and with `prefers-reduced-motion` or JavaScript disabled no GIF loads or animates — before this change the guide's GIFs autoplay, which is the failing state (AC-7.4, AC-7.5).
- **Test plan**: Build the site and inspect representative built pages (user guide, a mirror, a page with <2 headings) at both widths with the browser's device emulation or the Task 9 driver run by hand.
- **Invariants touched**: None directly (presentation only; the honesty invariants bind Tasks 3, 6 and 8) — stated as `None (layout and navigation data; no state, billing or capability claim changes)`.

- **Status**: ✅ Completed — Paper docs navigation, generated heading lists and explicit recording controls; PR #467.
- **Implementation**: Commit b8f31e1. Navigation groups resolve source paths to generated URLs; Liquid builds heading lists and poster markup before resource loading. The sidebar progressively enhances a plain list; GIF controls respect reduced motion.
- **Spec deviations**: None.
- **Files modified**: `docs/_data/navigation.yml`, `docs/_layouts/default.html`, `docs/assets/site.css`, `docs/assets/site.js`, `specs/in-progress/docs-site-restyle/tasks.md`, `specs/in-progress/docs-site-restyle/handoff.md`.

### Task 5 — Client-side search index and control (docs) ✅ COMPLETED

- **Domain/agent**: go-implementer (docs domain)
- **Budget**: complex (new build artifact with a size bound and truncation rule, plus an interactive control with offline and assistive behaviour)
- **Depends on**: Task 1 (index record shape), Task 4 (navigation groups)
- **Change**: Emit a static Liquid-generated search index of every published page and a search control that ranks title and heading matches first, works offline and by keyboard, and disappears without JavaScript instead of sitting inert.
- **Files**:
  - `docs/search.json` (new Liquid template with YAML front matter `layout: null` — Liquid runs, no site layout or `defaults:` layout is applied — emitting `{title, group, headings, body, truncated}` per published page under the 300 KB ceiling, largest-body-first truncation, AC-4.1, NFR-5)
  - `docs/assets/search.js` (new: fetch-on-first-use, ranking, anchor links, no-match message, truncated marking, result-count announcement, offline behaviour, AC-4.2, AC-4.3)
  - `docs/_layouts/default.html` (search control in the nav on both surfaces, hidden when JavaScript is unavailable, documented shortcut, AC-4.3, AC-4.4)
- **Produces**: built search index at `_site/search.json` (source `docs/search.json`), fetched from the layout-emitted `{{ '/search.json' | relative_url }}` URL (`/mythhelm/search.json`), record shape `{title, group, headings: [{text, anchor}], body, truncated}` where `group` is the navigation group or the literal `none` for unlisted pages (consumed by Task 8's coverage, group-domain and size checks).
- **Acceptance**:
  - The CI Jekyll build emits the index with title, group and headings for every published page and body text subject to the 300 KB ceiling; when the ceiling would be exceeded, body text is truncated largest-first and the record is marked `truncated: true`, never dropping titles or headings (AC-4.1, NFR-5). If Liquid cannot emit this index under `actions/jekyll-build-pages`, the task stops and reports to the maintainer instead of changing generators (OQ-5, N1).
  - A query matching a heading or (non-truncated) body text lists matching pages and sections with title/heading matches first, each linking to its anchor; a query matching nothing says so, and a body-text miss on a truncated page is not presented as "no match" (AC-4.2).
  - The control is reachable by the documented shortcut and by Tab, announces result counts to assistive technology, works offline once page and index are loaded, and is hidden with JavaScript unavailable while every page stays readable and navigable (AC-4.3, AC-4.4).
  - The index is fetched only on first search use, so it does not count toward the landing page's transferred weight (AC-4.1, NFR-3; measured by Task 9).
- **Test plan**: Build the site in CI, query the built index for a heading-only term, a body term and a nonsense term against a served `_site`, and record the ranked results; exercise the control with keyboard only and with JavaScript disabled.
- **Invariants touched**: I13 (v2 §2: search is a static browser artifact, never a MYTHHELM service); I09 (v2 §2: a truncated page's miss is labelled, never presented as absence).


- **Status**: ✅ Completed — Static Liquid search index and keyboard-accessible search on both surfaces, with lazy loading, offline reuse and explicit partial-text warnings; PR #468.
- **Implementation**: Commit `7f73e75` keeps the five-field records in a URL-keyed object; UTF-8 byte accounting removes whole bodies largest-first. The pinned Pages build emits 40 pages / 297,258 bytes; 377 heading labels and anchors match the emitted HTML. Red probes found the index/control absent; green corpus, Unicode-size fixture and Chrome keyboard/offline/responsive probes pass (PR acceptance table).
- **Spec deviations**: Review corrections preserve the target scroll position when restoring focus after same-page navigation and make the first Escape close a nonempty native search input (consistent with AC-4.2/AC-4.3). No contract or file-scope deviation.
- **Files modified**: `docs/search.json`, `docs/assets/search.js`, `docs/_layouts/default.html`, `specs/in-progress/docs-site-restyle/tasks.md`, `specs/in-progress/docs-site-restyle/handoff.md`, `specs/in-progress/docs-site-restyle/scratchpad.md`.

### Task 6 — State labels and declared-not-verified notes (docs)

- **Domain/agent**: go-implementer (docs domain)
- **Budget**: standard
- **Depends on**: Task 1, Task 5
- **Change**: Introduce the single state-label markup and apply it to the guide prose that names MYTHHELM's data states, plus the neutral declared-not-verified note for unverified statements, without rewriting the prose.
- **Files**:
  - `docs/_includes/state.html` (new: render `reported`, `estimated` or `unknown` with the consistent style, `unknown` dotted-underlined and distinguishable without colour; empty or unrecognised input renders `unknown`, AC-5.1, AC-5.2)
  - `docs/user-guide.md` (apply the markup only where prose names a data label — the budget-section definitions and "unknown rather than zero" sentences — and the declared note to declared-billing and entitlement statements; no prose rewrite, AC-5.2, AC-5.4, N2)
  - `docs/limitations.md` (apply the markup only to state words naming a data label, never headings or Windows lines; prose otherwise untouched, AC-5.2)
  - `docs/assets/site.css` (state-label and note styles, distinguishable without colour, no amber, AC-5.1, AC-5.4, OQ-6)
  - `docs/contributing.md` (document the state-label include (`state.html`, argument `state`, the three values and the `unknown` fallback) and the declared-note style for authors, AC-5.1)
- **Produces**: `docs/_includes/state.html` with single argument `state` (`{% include state.html state="<reported|estimated|unknown>" %}`), rendering `unknown` for an empty or unrecognised value (consumed by Task 8's wrapping and source-markup checks).
- **Acceptance**:
  - The three states render with one consistent style on Night and Paper, `unknown` with a dotted underline, each distinguishable without colour (AC-5.1).
  - Only code spans holding a single state word that names a recorded data label are converted (`grep -n '`\(reported\|estimated\|unknown\)`' docs/user-guide.md` candidates reviewed one by one in the completion entry); ordinary English, output lines, field paths and other enums stay verbatim, and a fixture with an empty or misspelled state renders `unknown`, never blank or zero (AC-5.2, I09).
  - The declared-billing and entitlement statements in the user guide use the neutral "declared, not verified" note with no amber (AC-5.4).
  - The user guide's three GIFs (`docs/user-guide.md:39-46`) use the Task 4 poster markup (poster image plus labelled start control, GIF only on request); `docs/contributing.md` names the include, its argument and the fallback so an author can use the convention without reading the layout (AC-5.1, AC-7.4).
- **Test plan**: Build the site and inspect the rendered label spans in the budget and billing sections; render a fixture page with an empty and a misspelled state and record that both show `unknown`.
- **Invariants touched**: I09 (v2 §2: reported, estimated and unknown stay distinct and missing never renders as zero); I14 (v2 §2: unverified statements are labelled declared, not verified).

### Task 7 — Tested-in-CI marker on code examples (docs)

- **Domain/agent**: go-implementer (docs domain)
- **Budget**: standard
- **Depends on**: Task 1 (examples-list schema), Task 6 (user-guide markup, shared file — sequential)
- **Change**: Mark code blocks as tested in CI only from the checked-in examples list, starting with the quickstart, so the marker is a claim the examples job can keep true.
- **Files**:
  - `docs/_data/examples.yml` (complete the entries: anchor, command as written, expected exit code; first entry the quickstart `go run ./cmd/mythhelm demo`, AC-6.1, AC-6.2)
  - `docs/_layouts/default.html` (render the "tested in CI" marker only for blocks with a matching entry; unlisted blocks render without it, AC-6.1)
  - `docs/user-guide.md` (the quickstart block and any output blocks: output blocks marked only when Task 10's examples job diffs the command's `docs-output-v1`-normalised output against the entry's `output_anchor` block, otherwise unmarked; existing `<!-- Output of: … -->` comments stay and earn nothing, AC-6.1, AC-6.4, AC-2.6)
  - `docs/index.md` (the landing quickstart block carries the same command text and marker as the guide's, AC-2.6)
- **Produces**: completed examples list and marker rendering (consumed by Task 8's marker checks and Task 10's examples job).
- **Acceptance**:
  - A built page shows the marker only on blocks with a matching `docs/_data/examples.yml` entry; a hand-marked block with no entry is the failing fixture Task 8 automates, and the existing provenance comments alone earn no marker (AC-6.1).
  - The first entry is the quickstart, every entry records its expected exit code, and a block showing a refusal (e.g. a `subscription-only` run) is listed with its non-zero code or stays unmarked (AC-6.2).
  - An output block is marked only when its normalised output is diffed by the examples job; otherwise it renders unmarked (AC-6.4).
- **Test plan**: Build the site and audit marked blocks against the list by hand; run the listed quickstart command non-interactively with `GOPROXY=off` after `go mod download` and record its exit code.
- **Invariants touched**: I14 (v2 §2: the marker claims only what CI runs); G10 (v2 §18.3: tested examples as public-release evidence).

### Task 8 — Built-site content checks (harness)

- **Domain/agent**: agent-config-editor (harness domain, `knowledge/domains.md`)
- **Budget**: complex (eight check families, each with a rejecting fixture, across Python and Bash)
- **Depends on**: Task 1, Task 3, Task 5, Task 6, Task 7
- **Change**: Add the Bash/Python built-site checks that keep the site's claims true — token parity, origin and script allow-list, panel phrases, state words, examples marker and normalised-output diff, search coverage and index size — each with a fixture it must reject, gated by the harness job until Task 10 wires the site-build job.
- **Files**:
  - `scripts/ci/check_docs_site.py` (new: layout-marker coverage, hero chapter-offset ordering and duration, token parity and stylesheet literals, amber `var()` ban, off-origin references in HTML/CSS and the built-site script allow-list, panel phrase/column/tracker and cell-label agreement, footer targets, state wrapping and source-markup validity with all three states present, marker-without-entry/empty-list/first-entry, entry–block drift both ways and the marker-without-`output_anchor` guard, search coverage, group domain and index size, including parsing `_site/search.json` as JSON and rejecting unresolved Liquid (`{{`/`{%`) or layout markup in it (the AC-1.1 marker check applies to built HTML pages, not the JSON index); AC-1.1, AC-1.2, AC-1.3, AC-1.5, AC-2.2, AC-2.3, AC-2.4, AC-2.5, AC-4.1, AC-4.5, AC-5.3, AC-6.1, AC-6.3, AC-6.4, NFR-5)
  - `scripts/ci/tests/test_check_docs_site.sh` (new: run each check against the real built tree shape and its rejecting fixtures, AC teeth per `.claude/rules/teeth-discipline.md`)
  - `scripts/ci/tests/fixtures/docs-site/` (new: fixture stylesheet with an unlisted hex and a differing table row, amber `var()` rule, off-origin script and `@import`, removed panel phrase, mismatched cell label, missing footer target, wrapped non-state text, empty and misspelled state arguments, hand-marked block, drifted entry and block, index missing a page and oversized index — each named in the test)
  - `scripts/ci/lint-agent-harness.sh` (wire the new test into the harness checks without weakening existing ones)
- **Produces**: check invocation exactly `python3 scripts/ci/check_docs_site.py --site _site --docs docs` (exit 0 when every check passes, exit 1 with one `path:line: check: detail` finding per failure, exit 2 on usage or unparseable input; consumed by Task 10's workflow wiring); fixture directory layout under `scripts/ci/tests/fixtures/docs-site/`.
- **Acceptance**:
  - The search-index check also fails when a record's `group` is neither a navigation group nor `none`, or when a sidebar-listed page carries `none` (AC-4.1, AC-4.5). The landing check fails when the built landing page carries zero or two-or-more panel links under the headline (fixtures for each, AC-2.1).
  - Each check passes on a conforming built tree and fails on its named fixture: a published page without `<!-- mythhelm-layout:v1 -->` (AC-1.1), a manifest with non-increasing chapter offsets and one exceeding the rendered video's duration from `ffprobe` (AC-2.2), unlisted hex and differing table row (AC-1.2), amber `var()` (AC-1.3), off-origin script, stylesheet `@import` and a built-site script absent from the checked-in allow-list (AC-1.5), removed panel phrase, wrong column, tracker outside the quoted region and cell-label mismatch (AC-2.3, AC-2.4), missing footer target (AC-2.5), index missing a sidebar or G10 page and oversized index (AC-4.5, NFR-5), wrapped non-state text, empty state and `estimeted`/`observed` source arguments, with all three states present at least once so no check passes on zero matches (AC-5.3), hand-marked block, empty examples list, first entry not the quickstart (AC-6.1), entry/block drift in each direction (AC-6.3), and a marked output block whose entry lacks an `output_anchor` (AC-6.4 — the executing diff runs only in Task 10's examples job; this check owns the marker-without-`output_anchor` guard and its fixture, never the command run).
  - `scripts/ci/tests/test_check_docs_site.sh` reports every assertion and `scripts/harness/gate.sh harness` passes on the task's tree (specs lint, hygiene and shellcheck included).
  - Fixtures live under `scripts/ci/tests/fixtures/` or a separate check build, never under `docs/`, and are never deployed or indexed (NFR-4).
- **Test plan**: Table-driven shell test over fixture trees in temporary directories (never the real `.claude/data/`), plus a run against a real CI-built `_site` recorded in the completion entry.
- **Invariants touched**: I09 (v2 §2: state-word checks fail on empty or unknown states, never pass vacuously); I14 (v2 §2: panel, cell and marker claims bind to their sources); G10 (v2 §18.3: the checks are the published-limitations enforcement).

### Task 9 — Browser-driver carve-out and responsive checks (harness)

- **Domain/agent**: agent-config-editor (harness domain)
- **Budget**: complex (rule carve-out, new Node driver with lockfile and licence gate, headless-browser measurements)
- **Depends on**: Task 1, Task 2, Task 3, Task 4, Task 5, Task 6
- **Change**: Carve out exactly one browser-driver directory from the harness-scripts rule and add the pinned headless-Chrome driver that measures overflow, element boxes, tap targets, first-viewport position, landing weight and rendered contrast, so responsive and accessibility claims are measured, not eyeballed.
- **Files**:
  - `.claude/rules/harness-scripts.md` (explicit carve-out for exactly one named directory under `scripts/ci/` allowing Node there and nowhere else under `scripts/**`, with the evidence for it, AC-7.6)
  - `scripts/ci/browser/package.json` (new: pinned driver dependencies with a committed lockfile, NFR-6)
  - `scripts/ci/browser/package-lock.json` (new committed lockfile, installed with `npm ci`, NFR-6)
  - `scripts/ci/browser/check_layout.mjs` (new: per-page scroll width at 390px and 1280px, element-box overflow outside scroll containers, 44px targets, quickstart first-viewport position, landing transferred weight excluding the hero recording, contrast pairs computed from rendered pages, off-origin recorded requests; AC-1.5 browser leg, AC-2.1, AC-7.1, AC-7.2, AC-7.3, NFR-3)
  - `scripts/ci/browser/test_check_layout.sh` (new: fixture pages — a `pre` wider than the viewport outside its container, an `overflow-x: clip` page hiding overflow, a below-threshold contrast pair and an undeclared pair, a sub-44px target — each rejected; the driver never skips when Chrome is missing, AC-7.1, AC-7.3, AC-7.6)
- **Produces**: driver invocation exactly `node scripts/ci/browser/check_layout.mjs _site` (consumed by Task 10) and the recorded Chrome source and version in the completion entry.
- **Acceptance**:
  - Before any driver code lands, every package in the resolved tree's licence is checked against the allow-list in `.github/workflows/dependency-review.yml` (`grep -n "allow-licenses" .github/workflows/dependency-review.yml`) and the result recorded; if any licence is off the list the task stops and reports to the maintainer, never widening the list (NFR-6).
  - `scripts/ci/lint-agent-harness.sh` passes with the carve-out, and a structural enumeration in `scripts/ci/browser/test_check_layout.sh` lists every file under `scripts/**` outside `scripts/ci/browser/` and fails if any is named `package.json` or `package-lock.json`, ends in `.mjs`, `.cjs` or `.js`, or contains a `node` invocation as a command word; the fixture is a `package.json` planted outside the carved-out directory, which it must reject (AC-7.6).
  - The driver fails on each fixture (wide `pre` outside its container, clip-hidden overflow, below-threshold and undeclared contrast pairs, sub-44px target) and passes on a conforming built site; it never skips when Chrome is missing — it fails with the browser source it looked for (AC-7.1, AC-7.3, AC-7.6).
  - On the built site it asserts: no recording GIF is fetched or animating before its start control is used and the poster stands in under `prefers-reduced-motion` (AC-7.4, AC-7.5), no published page's scroll width exceeds the viewport at 390px or 1280px including `user-guide.html`, no element box passes the viewport outside a scroll container, tap targets ≥44px at 390px, the quickstart in the first viewport, landing weight ≤500 KB excluding the hero recording with `preload="none"`, and contrast 4.5:1 / 3:1 from rendered pairs with Star light restricted to large text and UI parts (AC-7.1, AC-7.2, AC-7.3, NFR-3).
- **Test plan**: Shell test over static fixture pages (no Jekyll needed) in temporary directories, plus a run against a real CI-built `_site`; record the Chrome binary source and version in the completion entry.
- **Invariants touched**: I09 (v2 §2: contrast and overflow are computed from rendered pages, never a hand-kept list); I13 (v2 §2: the driver is a CI check, never a site runtime dependency).

### Task 10 — Site-build job gating CI OK and docs workflow wiring (release)

- **Domain/agent**: release-engineer (release domain, `knowledge/domains.md`)
- **Budget**: complex (workflow restructuring across two files with artifact hand-off and gating semantics)
- **Depends on**: Task 8 (content checks), Task 9 (browser driver)
- **Change**: Wire the new checks into CI so they block merges through the already-required CI OK: build the site once, upload the artifact before any third-party step, run the content, browser and examples checks against that tree, and join the job to CI OK's needs.
- **Files**:
  - `.github/workflows/docs.yml` (build once; upload the artifact immediately after the build and Assert mirrors rendered and before any browser, npm or example step; fixtures build to a different `destination:`; no existing assertion removed or weakened; mirror injection never before a passing mirror-freshness check, AC-2.5, AC-6.2)
  - `.github/workflows/ci.yml` (add the site-build job — Jekyll build, `check_docs_site.py`, the browser driver after `npm ci`, and the examples job running every listed command as written with its expected exit code — to the `needs:` list of `CI OK`; `actions/setup-node` pinned by full SHA with no dependency cache, AC-6.2, NFR-6)
  - `docs/automation.md` (rows for the site-build job, the content checks, the browser driver and the examples job, per `.claude/rules/github-workflows.md`)
  - `.github/dependabot.yml` (add the `npm` ecosystem entry for the `scripts/ci/browser/` directory with the 7-day cooldown, NFR-6 — moved here from Task 9 to keep each task in one domain)
- **Produces**: site-build job with id `site-build` and name `Site build` in `.github/workflows/ci.yml` as a member of `CI OK`'s `needs:`; the deployed artifact is the checked tree.
- **Acceptance**:
  - `CI OK`'s `needs:` includes the site-build job and `.claude/data/required-checks.json` is unchanged because CI OK is already required; the job triggers on `pull_request` and `merge_group` (AC-6.2, OQ-10).
  - The docs workflow uploads the built artifact before any browser, npm or example step, the deploy job deploys that artifact (or the two builds' tree digests are compared), fixtures build to a different destination, and a check proves the deployed tree contains no fixture marker (AC-6.2).
  - The examples job runs every `docs/_data/examples.yml` command as written and fails when the actual exit code differs from the entry's expected code (fixture entry expecting 0 whose command exits non-zero), an entry names a block no published page contains (fixture naming a missing block), or — for an entry with `output_anchor` — the command's stdout under the fixed normalisation `docs-output-v1` (CRLF-to-LF, trailing-whitespace trim) differs line-by-line from that block (fixture output block differing from the normalised output, AC-6.4); this job is the only executor of the output diff. The quickstart runs non-interactively with `GOPROXY=off` after `go mod download`, needs no credentials, and a structural check fails when `.github/workflows/ci.yml` (which hosts the `site-build` job) or `.github/workflows/docs.yml` references `secrets.` for these jobs or uses `pull_request_target` (AC-6.2, AC-6.5).
  - No existing `.github/workflows/docs.yml` assertion (mirror freshness, manifest revisions, transcript freshness, qualifiers, Assert mirrors rendered) is removed or weakened — the diff shows additions only to those steps (AC-2.5).
  - `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12` passes on both workflows and every new `uses:` is pinned to a 40-character SHA (`.claude/rules/github-workflows.md`).
- **Test plan**: actionlint and a workflow diff review; a pull request run showing the site-build job executing the checks and CI OK waiting on it, recorded in the completion entry. The fork-PR examples run remains a recorded maintainer inspection (AC-6.5).
- **Invariants touched**: I14 (v2 §2: gating makes the tested-examples and limitations claims enforceable); G10 (v2 §18.3, AT-46: zero-credential, pinned, least-privilege automation per `docs/automation.md` principles).

### Task 11 — Build evidence, automation record and final conformance (docs)

- **Domain/agent**: go-implementer (docs domain)
- **Budget**: standard
- **Depends on**: Task 3, Task 4, Task 5, Task 6, Task 7, Task 8, Task 9, Task 10
- **Change**: Close the spec by verification only: record the build time and site size before and after and verify every Definition-of-Done item against the built site so the card's revisit triggers have numbers. (Automation-record fixes, if the verification finds any, belong to a release-domain follow-up, not this task.)
- **Files**:
  - `docs/index.md` (any conformance fix the final pass finds — order, panel or footer — or no change, recorded as such)
  - `docs/user-guide.md` (any conformance fix the final pass finds — labels, notes or quickstart text — or no change, recorded as such)
- **Produces**: before/after Jekyll build duration and built-site total size recorded in the task's pull request (NFR-2); the completed Definition-of-Done checklist evidence for `/finalize-spec`.
- **Acceptance**:
  - The pull request records the Jekyll build step's duration and the built site's total size before (on `main`) and after this spec, so NFR-2's "revisit if build speed hurts" has its number (NFR-2, N1).
  - Every Definition-of-Done item in `requirements.md` is checked against the built site and the CI run of the final tree: element order and first-viewport position, claim-to-limitations binding, token parity and no third-party request, sidebar/list/search behaviour including JavaScript-off, state labels and declared notes, marker-only-from-list with a green examples job, overflow/targets/contrast/reduced-motion, unweakened docs assertions with the `docs/automation.md` rows Task 10 landed (verified here, edited only by a release-domain follow-up if wrong) and G10 footer links (requirements Definition of Done).
  - Demo images fit the column with captions below, every recording's text alternative names what it shows and that it is scripted, reduced-motion visitors get posters and no unprompted animation, and the palettes ignore colour-scheme preference (AC-7.4, AC-7.5) — asserted by inspection of the built pages and the Task 9 driver's reduced-motion run where it supports it, with the method recorded.
  - `scripts/ci/check-public-hygiene.sh` passes and no file added by this spec contains a private path, secret or personal data (`.claude/rules/public-repo-hygiene.md` via the harness gate).
- **Test plan**: Full gate run (`scripts/harness/gate.sh all` as applicable to the changed paths) on the final tree plus the built-site checks of Tasks 8 and 9 re-run against the final `_site`; results recorded in the completion entry.
- **Invariants touched**: I09 (v2 §2: final claims are checked against observed build output); I14 (v2 §2) and G10 (v2 §18.3: the published site is the evidence).
