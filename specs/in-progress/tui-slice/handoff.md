# tui-slice — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — TUI dependencies

<!-- pending -->

## Task 2 — View-model read seam

<!-- pending -->

## Task 3 — Terminal capability parsing

- **Produces**: `internal/tui/caps`: `Parse(colour, motion, icons string) (Prefs, error)` with `ErrInvalidMode` (check with `errors.Is`; message names dimension and value, e.g. `invalid --colour mode "rainbow"`); `Resolve(p Prefs, getenv func(string) string) Caps`; `Caps{Colour ColourLevel, Motion MotionLevel, Icons IconSet}`; levels `ColourNever/Basic/Full`, `MotionOff/Reduced/Full`, `IconsASCII/Unicode`. As designed.
- **For dependents** (tasks 5, 6, 12): register `--colour`/`--motion`/`--icons` flag defaults as the literal `"auto"` — `Parse` rejects `""` and wrong case. Pass `os.Getenv` as `getenv` in production. `--color` alias is flag-registration only (task 12), not in `caps`.
- **Precedence** (suppression wins): explicit `never`/`off`/`ascii` always holds; `NO_COLOR` set-and-non-empty forces `ColourNever` even over `always`; `TERM=dumb` (exact match) forces all-minimal even over `always`/`full`/`unicode`. Zero `Caps` is all-suppressing (fail-closed).
- **Traps**: `Resolve` never returns `ColourBasic` in this slice (no terminfo grading; `auto` means full). `COLORFGBG` is never read; no OS motion preference is queried (D5).
- **Deviations affecting later tasks**: none.

## Task 4 — Built-in themes in mods/

<!-- pending -->

## Task 5 — Diff viewer component

<!-- pending -->

## Task 6 — App shell, layouts and mission view

<!-- pending -->

## Task 7 — Navigation, palette and help

<!-- pending -->

## Task 8 — Truthful motion

<!-- pending -->

## Task 9 — Palette actions and confirmations

<!-- pending -->

## Task 10 — Live render loop and history

<!-- pending -->

## Task 11 — Accessible linear renderer

<!-- pending -->

## Task 12 — CLI launch wiring

<!-- pending -->

## Task 13 — End-to-end launch and fallback

<!-- pending -->

## Task 14 — G09 evidence, UX session and README status

<!-- pending -->
