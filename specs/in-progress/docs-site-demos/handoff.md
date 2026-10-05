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

- **Produces**: tape contract as shipped — `docs/demos/demo.tape` (header
  `Binary-Version: v0.0.0-20261005073052-e0ecdbc9554c` /
  `Binary-Commit: e0ecdbc9554c90da6ac61f6300b7cc836acae266` /
  `VHS-Version: v0.12.1`; single `Type` line driving `TERM=dumb
  /tmp/mythhelm-record demo --check pass`, then `Sleep 10s`),
  `docs/demos/demo.transcript.txt` (55 lines, 4 `SCRIPTED DEMO`
  labels), `docs/demos/normalize.sed` (byte-identical to design §3),
  `docs/demos/manifest.json` (schema per design §3; demo entry with qualifier
  `TestDemoOfflineNoCredentials`), `docs/demos/record.sh` (build + regen +
  `vhs` render + verify; refuses a checkout that mismatches the manifest pin
  unless `--repin`), `docs/demos/README.md` (record command + table).
- **For Task 5**: render with `docs/demos/record.sh` (never bare `vhs`); the
  GIF is ~160 KB at 1080x620 and its caption must name `docs/demos/demo.tape`
  and `docs/demos/record.sh`. `demo.gif` is yours to commit — this task
  rendered it as proof only.
- **For Task 6**: `binary_commit e0ecdbc9…` rebuilds and the §3 pipeline
  re-runs to a byte-identical transcript (verified from a detached worktree);
  qualifier check asserts the `^--- PASS: TestDemoOfflineNoCredentials` line,
  never a bare `-run` exit.
- **For Task 7**: this task added no TUI row to `docs/demos/README.md` and no
  TUI manifest entry — both are yours.
- **Traps**: the record binary path `/tmp/mythhelm-record` is shared by the
  tape, the transcript pipeline and `record.sh` — keep all three in sync.
  `vhs --version` prints `vhs version v0.12.1`; the manifest stores the bare
  `v0.12.1`. VHS v0.12.1 has no `Run` command, so tapes use `Type`+`Enter`.
  `TERM=dumb` is load-bearing (without it the demo launches the live TUI under
  VHS's tty); `Wait+Screen` cannot see past the first screenful in VHS v0.12.1,
  so completion waits are fixed `Sleep`s with margin.
- **Deviations affecting later tasks**: review-round fixes — tape forces
  linear output and sleeps for completion; `record.sh` gains `--repin`.

## Task 5 — Rendered demo GIF and embeds

<!-- pending -->

## Task 6 — Recording honesty checks in CI

<!-- pending -->

## Task 7 — TUI tape scaffold (FR-3 follow-up filed; tui-slice shipped)

- **Produces**: `docs/demos/tui.tape` (comment-only scaffold: states the
  recording is planned, names the interface as the `specs/done/tui-slice/`
  acceptance, no `Run` lines); `docs/demos/README.md` TUI row marked planned
  with the tui-slice dependency.
- **For Task 6**: no TUI manifest entry and no TUI recording image exist, so
  the qualifier check exempts the scaffold by construction; the follow-up adds
  both (manifest entry + artifact + transcript) and falls under the checks then.
- **For the orchestrator**: file the drafted FR-3 follow-up card below via the
  backlog flow (`pm.py next-id` assigns the id; open the GitHub issue; the
  tape itself stays a scaffold with no `Run` lines):
  ```markdown
  ### MH-N: TUI terminal recording (FR-3 follow-up)
  - **Status**: idea
  - **Stage**: 1
  - **Gates**: G10
  - **Score**: 8.0 = (value 2 + urgency 5 + risk 1) / effort 1
  - **Spec**: (none)
  - **Issue**: (none)
  - **Source**: master spec §14.9, §22.3; docs-site-demos FR-3/AC-3.2/AC-3.3 and design §5; specs/done/tui-slice/ (shipped 2026-10-04)
  - **Summary**: Record the shipped TUI on tape: drive the mission view
    (internal/tui/mission.go), responsive layouts down to linear mode
    (internal/tui/layout.go) and keyboard flows (internal/tui/nav.go) with the
    TUI's own verification status on screen, limited to exactly the covered
    tui-slice behaviour; ship the full tape with header pins, a manifest entry
    naming qualifying tests from internal/tui/*_test.go, and a normalized
    transcript per the docs-site-demos tape contract, never pane text as proof.
  ```
- **Traps**: never name a rendered TUI artifact filename in `docs/` or
  `README.md` until the follow-up renders one — the no-recording-image check
  greps those trees for exactly such names. Score uses urgency 5 (Stage 1 is
  current per product/objectives.md); rescore if the stage advances before filing.
