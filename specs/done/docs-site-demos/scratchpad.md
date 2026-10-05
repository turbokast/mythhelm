# Documentation Site and Scripted Terminal Demos — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| 1 | Which site generator? | (a) GitHub-native Jekyll via `actions/jekyll-build-pages` (design D1) | Maintainer / Task 1 |
| 2 | Where do tapes and recordings live? | (a) side by side in `docs/demos/` (design D4) | Maintainer / Task 4 |
| 3 | GIF or video format? | (a) GIF in tree; revisit if `demo.gif` exceeds ~5 MB (design D5) | Maintainer / Task 5 |
| 4 | Which demo scenario does the first tape drive? | (a) `mythhelm demo --check pass` end to end (design D6) | Maintainer / Task 4 |
| 5 | Does enabling Pages need operator action? | Yes, assumed: Task 3's PR description requests Pages source "GitHub Actions" + ruleset update; deploy stays red until done | Operator / Task 3 rollout |
| 6 | Which VHS version pins the first recording? | Pinned at implementation from `vhs --version` actually used (design D9) | Task 4 implementer / Task 4 |
| 7 | What is the real Pages URL? | `https://turbokast.github.io/mythhelm/` assumed; Task 5 records the actual URL | Task 5 implementer / Task 5 |
| 8 | Which spec publishes shared §19.3/G10 sentences first — this one or `openssf-badge`? | RESOLVED 2026-10-05: `openssf-badge` shipped first; this spec reconciles (design §7) | Maintainer / Task 2 |
| 9 | What view-model/launch command does the FR-3 TUI tape drive? | Answerable now that `specs/done/tui-slice/` shipped; the FR-3 follow-up card (drafted by Task 7) pins it | tui-slice spec / FR-3 follow-up |
| 10 | GIF-to-tape binding scope (delivery Q-10)? | RESOLVED 2026-10-05: keep the disclosed residual (transcript + manifest + human review); no render-compare CI in this run | Maintainer / Tasks 5–6 |

## Research notes

- VHS renders GIF/MP4/WebM from `.tape` files (`Output` directive or `--output`); the
  tape `Set Shell`, `Type`/`Enter`, `Sleep` vocabulary drives an interactive shell running
  the local binary. No `vhs` binary on the authoring machine (`which vhs` empty), so no
  tape syntax was executed here — Task 4 runs it for real.
- `actions/jekyll-build-pages` + `actions/deploy-pages` is the Pages-recommended
  Actions deployment path; `source: ./docs` keeps the site out of the repo root. SHAs for
  `uses:` pins must be resolved from the releases at implementation time, never from
  memory (`.claude/rules/github-workflows.md`).
- `mythhelm demo` stdout embeds temp paths (`internal/cli/demo.go:84,111`), run/attempt
  ULIDs, snapshot/candidate SHAs, pids and evidence paths — hence the `normalize.sed`
  in design §3 (verified 2026-10-02 across `go run`, built-binary and custom-`TMPDIR`
  runs). Exit codes: pass 0, fail 5 — but only for a built binary; `go run` exits 1
  with an `exit status 5` trailer, so Task 4 builds first.
- Pids ≥ 10^6 print as `1.012413e+06` (float formatting via `p.text(…)` at
  `internal/cli/render.go:193`); `normalize.sed` covers both pid shapes. Existing
  renderer behaviour, not this spec's to change.
- Jekyll excludes non-special `_`-prefixed source directories from the build, so the
  root-guide mirrors live at `docs/mirror/`, not `docs/_mirror/`.
- Jekyll copies `.md` without front matter to `_site` unrendered: authored pages carry
  the design §2 block in the tree; mirrors get it from the build job's transient inject
  step (design §4), which runs after the drift check so it cannot mask staleness.
- `go test -run <missing>` exits 0 (`[no tests to run]`), so the AC-4.2 qualifier check
  asserts a `^--- PASS: <qualifier>` line under `pipefail` (design §4 step 4).
- NFR-2 macOS timing has no CI leg in this spec; the Task 4 implementer times the
  machine class they have and records it.

## Re-grounding (2026-10-05, pre-run; maintainer-confirmed)

`tui-slice` (MH-2) and `openssf-badge` (MH-9) both shipped while this spec waited, so every
"tui-slice in progress / lands later" premise was re-grounded: Task 2's limitations page
now describes the TUI as shipped (the old "planned, not shipped" check would have
published a falsehood); Task 7 keeps the scaffold but drafts the FR-3 follow-up recording
card (FR-3's "after the TUI slice has landed" precondition now holds); design §1/§2/§5/§7
and the requirements grounding say shipped. Delivery Q-10 resolved as keep-disclosed-
residual: no render-compare CI. No AC text changed — FR-3 still binds the follow-up.

## Discoveries
