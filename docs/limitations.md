---
layout: default
title: Limitations
---

# Limitations

MYTHHELM is **pre-alpha**: there is no usable release yet. This register
lists what is supported today and what is not, so expectations stay honest.
Anything below marked experimental may change without notice.

## Supported today

- Headless `mythhelm run` with admission, supervision, verification, review,
  apply and recovery, on Linux, macOS and Windows.
- `mythhelm demo`: a fully offline scripted run in a disposable repository;
  every screen is labelled `SCRIPTED DEMO`.
- `mythhelm doctor`: a read-only prerequisite report that writes nothing.
- The TUI is shipped: the focused mission view on a TTY, with responsive
  layouts, palette/command navigation and event-driven motion.
- Stable linear output without a TTY, under `TERM=dumb`, with `--plain`, and
  machine output (`--format jsonl`).

TUI behaviour is qualified by the G09 evidence record in the repository
(`docs/tui-slice-g09-evidence.md`); unrecorded combinations are experimental.

## TUI limits

- Only the recorded interactive combination is claimed: GNOME Terminal 3.52
  (VTE 0.76) + zsh 5.9 on Ubuntu 24.04, 190x45. Interactive use in any other
  terminal/shell is experimental until recorded.
- Screen-reader support is experimental: no human session recorded yet.
- The `q` exit-options dialog renders its rows but key selection is unwired;
  Esc closes it, Ctrl-C quits.
- Mid-run Ctrl-C cancels the run (exit 130) instead of detaching.

## Not supported yet

- Plugins, routing and releases — all still planned.
- Narrow adapter coverage: the scripted fake adapter plus a Claude Code
  adapter. Native Claude execution is refused on Windows by design (exit 7).
- Allowance exhaustion is recognised only from a synthetic fixture shape; no
  recorded native run produces it, so on a real route the bucket is not
  recorded and the retry schedule never starts.
- A run's repair, replan and transport-retry ceilings are checked at launch
  only, and no command records an envelope extension.
- `subscription-only` billing always blocks: no native surface qualifies an
  included-only boundary yet.
- The OAuth-token exception is unwired and refused at both gates.
- macOS MDM preferences and remote cached managed policy as certified trust
  sources; the settings inventory covers files only.
