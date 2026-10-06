# TUI Tape Recording — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| 1 | Does the TUI follow pty resize under VHS (rung-per-launch needs no live resize, but Bubble Tea must size correctly at each fresh launch width)? | Yes — fresh launches read winsize at startup; the demo precedent launches the TUI under VHS successfully. If a rung renders at the wrong width, the implementer records a deviation and falls back to calibrating `Set Width` per rung take. | Task 1 / blocks nothing unless observed |
| 2 | Exact `Sleep` margins per take? | Derived at implementation: start from the demo's 5x margin over the observed ~2s scenario, extend until three consecutive renders show every take's final state; fixed sleeps, never Wait+Screen. | Task 1 |
| 3 | Exact `normalize.sed` additions for the linear/accessible takes? | Derived at implementation: run the §3 pipeline repeatedly, diff, add one rule per varying token class (ULIDs/pids/SHAs already covered). | Task 1 |
| 4 | GIF size vs the ~5 MB budget? | Measured at implementation (`stat`, recorded in the entry). Levers in order: shorter holds, lower `Set Framerate`, then the NFR-2 path (report back; smaller format becomes a follow-up). Motion stays default throughout. | Task 2 |

Requirements Q1–Q5 were all resolved during design (see design §8 D1–D5): tui.gif; keystrokes with no code change; one declared revision with demo re-recorded together; linear-takes transcript; Quickstart + README embeds.

## Research notes

- VHS v0.12.1 probes (2026-10-06, `/tmp/vhs-probe*.tape`, frames viewed): no `Run` directive; multiple `Output` renders only the last, silently; mid-tape `Set` ignored with a warning; `stty cols` honored (85→60→150 on rendered frames); `vhs validate` gates syntax (exit 0/1); 1000px/16px font = 85 columns.
- The TUI owns all input while running (`internal/tui/nav.go:21-65`): rung changes cannot be typed mid-session — hence rung-per-launch (design D1). Only `ctrl+c` quits; `q` opens the excluded exit dialog; `esc` never quits.
- `demo --check pass --accessible` is non-interactive on any output (accessible wins over TTY per `selectLaunch`) and exits 0 — the linear endpoint take.
- Qualifier tests (§6 table) verified passing at HEAD 2026-10-06; CI re-verifies at the pin.
- Task 7's "no-recording-image check" was verified absent from `docs.yml` and `record.sh` — dropped (design D8), not silently ignored.

## Discoveries
