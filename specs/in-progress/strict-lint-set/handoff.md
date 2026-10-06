# strict-lint-set — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Adapters fallout (revive, modernize)

- **Produces**: `./adapters/...` clean under all five fallout trial linters (revive, modernize, gocritic, perfsprint, dupl — each exits 0 with the uncapped trial command). Shipped: doc comments on `AdapterID`/`New`, `caps`/`canonical` renames, drain-loop bind in `waitStarted`, `SplitSeq`/`new(n)`/`for range N`/`strings.Cut`/`errors.AsType`/`typ.Fields()` simplifications across the 7 files.
- **For dependents**: Task 8's full-tree run covers these paths with zero findings expected from them. No `//nolint` was added, so no machine-checked-form debt. New comments carry trailing periods (godot stays clean).
- **Deviations that change a later task's inputs**: none — no API, behavior, or file-list change; Task 8 needs nothing from this task beyond its merged green state.

## Task 2 — Core fallout A (adapter, admission, integration)

<!-- pending -->

## Task 3 — Core fallout B (cli)

<!-- pending -->

## Task 4 — Core fallout C (supervisor, workers)

<!-- pending -->

## Task 5 — Core fallout D (workspace, journal)

<!-- pending -->

## Task 6 — TUI fallout A (mission, model, views)

<!-- pending -->

## Task 7 — TUI fallout B (nav, tools, tests)

<!-- pending -->

## Task 8 — Enable the strict set + enforce the time budget

<!-- pending -->
