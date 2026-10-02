# TUI Slice — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| 1 | Which screen-reader/terminal combination is advertised first? | The one the task-14 maintainer session actually records; all others experimental/unsupported (D16, I14) | Maintainer / task 14 |
| 2 | Which terminals/shells beyond CI's OS legs get recorded for G09? | The maintainer's own terminal plus CI plain/non-TTY coverage; the §16.4 remainder stays experimental | Maintainer / task 14 |
| 3 | Do the pinned Charm versions still resolve vulnerability-free at implementation time? | Task 1 re-verifies (`govulncheck`, dependency-review); a CVE finding upgrades the pin as a recorded deviation | Task 1 implementer |
| 4 | Does Bubble Tea v1.3.x behave on Windows ConPTY for the §16.4 fields? | Unknown until recorded; Windows Terminal starts experimental and is promoted only by evidence rows | Maintainer / task 14 |

## Research notes

- Requirements Q1 (view-model API) decided in design §3: option (b), `internal/tui/viewmodel` over read-only journal projections plus receipt. The read functions it needs all exist: `journal.OpenReadOnly` (`internal/journal/projections.go:346`), `Run`/`LatestAttempt`/`LatestVerification`/`Candidate` (`projections.go`), `Events` (`internal/journal/journal.go:387`), `supervisor.ReadReceipt` (`internal/supervisor/receipt.go:302`).
- Requirements Q2 (motion subset) decided in design §11: option (a) — moments 1, 2, 3, 4, 6, 8 (partial), 9, 10; moments 5 and 7 have no Stage 1 trigger and are honesty-register rows, not stubs.
- Requirements Q3 (advertised screen-reader combinations) decided in design §12: option (a) — exactly the tested combinations, default one.
- Requirements Q4 (G09 matrix) decided in design §15: dogfood CI OS legs plus maintainer-recorded terminals; remainder experimental.
- Requirements Q5 ("very short height") decided in design §6: fewer than 10 rows (D14); tests use ≤5 rows.
- Charm versions queried live on 2026-10-02: `go list -m -versions github.com/charmbracelet/bubbletea@latest` → newest `v1.3.10`; `lipgloss@latest` → `v1.1.0`; `bubbles@latest` → `v1.0.0`. All are MIT-licensed (Charm); task 1 re-verifies licences via the repo's `licenses` CI job.
- `mattn/go-isatty v0.0.24` is already an indirect requirement in `go.mod`; task 1 promotes it to direct use (D12).
- No `internal/tui/` or `mods/` directories exist; tasks 2 and 4 create them (noted in their Files lists).
- Sibling specs checked 2026-10-02: `specs/in-progress/`, `specs/unfinalized/` are empty; `specs/todo/` holds only `specs/*/docs-site-demos/`, which explicitly claims no `internal/tui/` or `mods/` changes and sequences its FR-3 behind this spec. No other live sibling spec exists. No file conflicts. The two specs' planned `README.md` edits are disjoint regions (status section vs demo embed).

## Discoveries

- 2026-10-02 spec fix (Task 1): `go mod tidy` drops unimported requires, so Task 1's original Files (`go.mod`/`go.sum` only) left `go.mod` byte-identical to base and its acceptance was unachievable. Fix: Task 1 now also writes `internal/tui/tools.go` (package `tui`, blank imports of the four pinned modules) so the pins survive `tidy -diff`; go-implementer owns that pin file as incidental to the `go.mod` change.
