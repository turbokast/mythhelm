#!/usr/bin/env bash
# gate.sh: runs quality gates and records each verified pass as a gate marker.
#
#   scripts/harness/gate.sh <gate|group>...
#
# Gates: go-fmt go-vet go-test go-mod-tidy golangci-lint govulncheck (scope go),
# harness-lint harness-tests shellcheck (scope harness), hygiene (whole tree).
# Groups: go, harness, all — the gates of that scope the tree's change set needs.
#
# Each gate runs its fixed command unpiped from the root of the working tree that
# contains the current directory. Its marker, .claude/data/gate-marker-<gate>.json
# in that tree, is written only when the command exits 0 and the tree did not
# change while it ran; a failing run deletes the marker. The wrapper writes the
# marker itself, so a pass counts however the command is launched: from any
# directory of the tree, chained, piped or in the background.
#
# Exit 0 when every gate passed and was recorded; 1 when one failed or was not
# recorded (it stops there); 127 when a gate's tool is missing; 2 on a usage error.
# The mechanism and marker format are in scripts/harness/gatelib.py.

set -euo pipefail
exec python3 "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/gatelib.py" run "$@"
