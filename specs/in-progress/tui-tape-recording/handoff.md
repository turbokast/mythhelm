# tui-tape-recording — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — TUI tape, transcript, manifest + record.sh generalization

- **Produces**: Record contract — `record.sh [--repin]` drives every manifest recording (demo + tui): paths/pins per recording from the manifest; per-name transcript pipelines (`demo` verbatim, `tui` = design §3 two-take pipeline with per-take `PIPESTATUS` assertions); per-name label assertions (demo floor 4; tui exact: SCRIPTED DEMO 8, goal/run-state/candidate/next-action/actions 1, attempt 7); per-recording VHS-pin check, `--repin` for every tape header, `vhs validate` + single-`Output` assertion per tape. TUI tape contract: header pins, take order (rung-1 flows → rungs 2–4 → accessible take), rung widths 140/120/90/70 each with in-frame `tput cols` proof. Pins: both recordings at `09fb91081cf71d96e3f75b9144241872fc4d70e0` (`v0.0.0-20261006120359-09fb91081cf7`), VHS `v0.12.1`; `len(pins) == 1` holds.
- **For dependents**: `tui.gif` is intentionally NOT in this PR — Task 2 renders it with `record.sh` (trial render 3.0 MB / 81s at Framerate 30, under the ~5 MB threshold). `normalize.sed` lines 1–9 are the frozen demo rules; the 5 appended TUI rules normalize live-stream sampling artifacts (see scratchpad Discoveries) — Task 3's CI transcript step must use the checked-in sed file as-is. `record.sh` runs `vhs` from the repo root (VHS `Output` paths are relative). Full record takes ~3.5 min. Demo GIF restored byte-identical (encoder-nondeterministic, excluded).
- **Deviations that change a later task's inputs**: none.

## Task 2 — Rendered TUI GIF and embeds

<!-- pending -->

## Task 3 — Recording-honesty-check generalization in CI

<!-- pending -->
