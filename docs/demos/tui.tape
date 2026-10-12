# MYTHHELM TUI recording (tui-tape-recording, FR-1).
# Drives the shipped TUI slice on `mythhelm demo --check pass` end to end
# against the fake adapter with local-scripted billing: no network, no paid
# credentials. Rung-per-launch: the TUI owns all input while it runs, so a
# rung change cannot be typed mid-session; each rung is a fresh launch at its
# `stty cols` width, the way a user opens the TUI at that size. The live
# resize transition itself is not shown — rungs are fresh launches.
#
# Scope is exactly the met tui-slice acceptance (AC-1.2/I14): the mission
# view, the responsive layout ladder down to the linear accessible mode, and
# the keyboard flows (arrows, `/` filter, `:` palette, `?` help,
# `Enter`/`Escape`). Excluded partials, never shown as working: the inert
# exit-dialog rows (issue #106) — `q` is never pressed, only `ctrl+c` quits —
# and the unwired history search, never opened.
#
# Render only through the record command, which builds the binary this tape
# drives, regenerates the transcript, renders the GIF and verifies all three:
#
#   docs/demos/record.sh
#
# A bare `vhs docs/demos/tui.tape` renders the GIF but is not the record
# command: it skips the build, the transcript regen and the verify pass.
#
# Binary-Version: v0.0.0-20261006124821-179bba4ceae9
# Binary-Commit: 179bba4ceae958f9591e9fedb056f70da59ce40c
# VHS-Version: v0.12.1

Output docs/demos/tui.gif

Require git

Set Shell "bash"
Set FontSize 16
Set Width 1650
Set Height 620
Set Framerate 30

# Rung 1: wide (140 columns, AC-2.1) — the mission view plus every keyboard
# flow. Each rung's `tput cols` runs visible in-frame right after its `stty`:
# rung-width proof on screen.
Type "stty cols 140 && tput cols"
Enter
Sleep 1s
Type "/tmp/mythhelm-record demo --check pass"
Enter
# The scenario runs ~2s; the hold keeps the mission view (goal, next action,
# activity, candidate diff, verification status) readable before the flows.
Sleep 8s
# Arrows move selection.
Down
Sleep 500ms
Up
Sleep 500ms
# `/` filter: type, apply, clear.
Type "/"
Sleep 500ms
Type "demo"
Sleep 500ms
Enter
Sleep 1s
Escape
Sleep 500ms
# `:` palette: open, move, close.
Type ":"
Sleep 1s
Down
Sleep 500ms
Escape
Sleep 500ms
# `?` help: open, close.
Type "?"
Sleep 1s
Escape
Sleep 500ms
# `Enter` details: open, back out.
Enter
Sleep 1s
Escape
Sleep 500ms
# Only ctrl+c quits; `q` would open the excluded exit dialog (issue #106).
Ctrl+C
Sleep 3s

# Rung 2: two-pane (120 columns, AC-2.2).
Type "stty cols 120 && tput cols"
Enter
Sleep 1s
Type "/tmp/mythhelm-record demo --check pass"
Enter
Sleep 8s
Ctrl+C
Sleep 3s

# Rung 3: single (90 columns, AC-2.3).
Type "stty cols 90 && tput cols"
Enter
Sleep 1s
Type "/tmp/mythhelm-record demo --check pass"
Enter
Sleep 8s
Ctrl+C
Sleep 3s

# Rung 4: compact (70 columns, AC-2.4) — the `--accessible for linear` offer
# stays visible in the first five rows.
Type "stty cols 70 && tput cols"
Enter
Sleep 1s
Type "/tmp/mythhelm-record demo --check pass"
Enter
Sleep 8s
Ctrl+C
Sleep 3s

# Take 5: the linear endpoint the compact rung offers — `--accessible` runs
# non-interactively to exit 0, streaming the state labels.
Type "/tmp/mythhelm-record demo --check pass --accessible"
Enter
Sleep 8s
