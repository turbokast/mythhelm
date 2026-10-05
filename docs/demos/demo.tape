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
# Binary-Version: v0.0.0-20261005073052-e0ecdbc9554c
# Binary-Commit: e0ecdbc9554c90da6ac61f6300b7cc836acae266
# VHS-Version: v0.12.1

Output docs/demos/demo.gif

Require git

Set Shell "bash"
Set FontSize 20
Set Width 1080
Set Height 620

Type "/tmp/mythhelm-record demo --check pass"
Enter

Wait+Screen@60s /removed afterwards/
Sleep 2s
