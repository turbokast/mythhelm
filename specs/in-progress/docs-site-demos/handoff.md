# docs-site-demos — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Site scaffold: config, nav, layout, home

<!-- pending -->

## Task 2 — Guide pages and root-guide mirrors

<!-- pending -->

## Task 3 — Pages build-and-deploy workflow

<!-- pending -->

## Task 4 — Demo tape, transcript and manifest

- **Produces**: tape contract as shipped — `docs/demos/demo.tape` (header
  `Binary-Version: v0.0.0-20261005073052-e0ecdbc9554c` /
  `Binary-Commit: e0ecdbc9554c90da6ac61f6300b7cc836acae266` /
  `VHS-Version: v0.12.1`; single `Type` line driving `/tmp/mythhelm-record demo
  --check pass`), `docs/demos/demo.transcript.txt` (55 lines, 4 `SCRIPTED DEMO`
  labels), `docs/demos/normalize.sed` (byte-identical to design §3),
  `docs/demos/manifest.json` (schema per design §3; demo entry with qualifier
  `TestDemoOfflineNoCredentials`), `docs/demos/record.sh` (build + regen +
  `vhs` render + verify), `docs/demos/README.md` (record command + table).
- **For Task 5**: render with `docs/demos/record.sh` (never bare `vhs`); the
  GIF is ~155 KB at 1080x620 and its caption must name `docs/demos/demo.tape`
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
- **Deviations affecting later tasks**: none.

## Task 5 — Rendered demo GIF and embeds

<!-- pending -->

## Task 6 — Recording honesty checks in CI

<!-- pending -->

## Task 7 — TUI tape scaffold (FR-3 follow-up filed; tui-slice shipped)

<!-- pending -->
