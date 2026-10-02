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

<!-- pending -->

## Task 4 — Built-in themes in mods/

- **Produces**: `theme.Tokens{Surface, SurfaceRaised, Text, TextMuted, Focus, Attention, OK, Warning, Err, Border}` (`internal/tui/theme/theme.go`); `theme.BuiltIn("dark"|"light")`, `theme.Load(path)`, `(Tokens) Validate()`, `theme.ErrUnknownTheme`, `theme.ErrInvalidTheme`; `themes.Files embed.FS` (`mods/themes/themes.go`); `mods/themes/dark.toml`, `mods/themes/light.toml`. As designed, no API differences.
- **For dependents**: consume `theme.Tokens` by value from `BuiltIn`/`Load` — both validate before returning, so no re-validation. TOML schema is flat snake_case (`surface_raised`, `text_muted`, …); colours strict `#rrggbb`; `border` is `rounded|ascii`.
- **For dependents**: `Validate` enforces WCAG 4.5:1 on Text/TextMuted × Surface/SurfaceRaised — task 9's adversarial-token test must expect low-contrast sets to fail `Validate` (render adversarial tokens directly, not via `Load`).
- **For dependents**: every `ErrInvalidTheme` names the offending key; branch with `errors.Is`, never string matching. `Load` reads are size-bounded (64 KiB).
- **Deviations that change a later task's inputs**: none.

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
