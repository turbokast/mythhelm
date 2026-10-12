# MYTHHELM scripted demo recording (docs-site-demos, FR-2).
# Drives `mythhelm demo --check pass` end to end against the fake adapter
# with local-scripted billing: no network, no paid credentials.
#
# Render only through the record command, which builds the binary this tape
# drives, regenerates the transcript, renders the GIF and verifies all three:
#
#   docs/demos/record.sh
#
# A bare `vhs docs/demos/demo.tape` renders the GIF but is not the record
# command: it skips the build, the transcript regen and the verify pass.
#
# Binary-Version: v0.0.0-20261006124821-179bba4ceae9
# Binary-Commit: 179bba4ceae958f9591e9fedb056f70da59ce40c
# VHS-Version: v0.12.1

Output docs/demos/demo.gif

Require git

Set Shell "bash"
Set FontSize 20
Set Width 1080
Set Height 620

# TERM=dumb forces the linear run: under VHS's tty the demo would otherwise
# launch the live TUI (selectLaunch) and never print the done banner. The
# forced output is byte-identical to the transcript (verified by diff).
Type "TERM=dumb /tmp/mythhelm-record demo --check pass"
Enter

# Fixed sleep, not Wait+Screen: VHS v0.12.1 screen-text matching only sees the
# first screenful, so a done-banner Wait never matches (verified: the demo
# completes and the banner renders, but the Wait times out). The demo runs in
# ~2s; 10s is a 5x margin. The transcript (not the GIF) is the proof, and
# Task 5 spot-checks the rendered GIF by eye.
Sleep 10s
