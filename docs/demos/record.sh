#!/usr/bin/env bash
# record.sh: the documented record command for the MYTHHELM terminal
# recordings (docs-site-demos, FR-2/FR-4). It builds the binary, regenerates
# the demo transcript, renders the demo GIF from the checked-in tape, and
# verifies all three. A bare `vhs docs/demos/demo.tape` only renders the GIF
# and is not the record command.
#
# Usage: docs/demos/record.sh [--repin]
#
# Without flags the checkout must match the manifest's binary_commit: writing
# fresh artifacts while a stale pin labels them is refused. With --repin (on
# a clean tree) the script regenerates from the current checkout and moves the
# manifest and tape-header pins to it.
#
# Needs only free tooling: go, git, python3, and a VHS install (vhs renders
# through ffmpeg and ttyd). Install VHS from https://github.com/charmbracelet/vhs
# (for example `go install github.com/charmbracelet/vhs@latest`) and ttyd from
# https://github.com/tsl0922/ttyd; ffmpeg comes from the OS packages.

set -euo pipefail

REPIN=0
if test "${1:-}" = "--repin"; then
	REPIN=1
	shift
fi
if test "$#" -gt 0; then
	echo "record.sh: usage: docs/demos/record.sh [--repin]" >&2
	exit 2
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

DEMOS="docs/demos"
BIN="/tmp/mythhelm-record"
TAPE="$DEMOS/demo.tape"
TRANSCRIPT="$DEMOS/demo.transcript.txt"
SED="$DEMOS/normalize.sed"
MANIFEST="$DEMOS/manifest.json"
GIF="$DEMOS/demo.gif"

for tool in go git vhs python3; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		echo "record.sh: need $tool on PATH" >&2
		exit 2
	fi
done

echo "record.sh: versions"
go version
git --version
vhs --version
python3 --version

pin="$(python3 -c 'import json;print(json.load(open("'"$MANIFEST"'"))["recordings"][0]["binary_commit"])')"
head="$(git rev-parse HEAD)"
if test "$head" != "$pin" && test "$REPIN" -eq 0; then
	echo "record.sh: checkout $head != manifest binary_commit $pin; refusing to write artifacts another revision labels" >&2
	echo "record.sh: re-run with --repin to regenerate and move the pins (or check out $pin)" >&2
	exit 1
fi
if test "$REPIN" -eq 1 && test -n "$(git status --porcelain)"; then
	echo "record.sh: --repin needs a clean tree (pins must name a reproducible commit)" >&2
	exit 1
fi

echo "record.sh: build the binary (go build, never go run)"
go build -o "$BIN" ./cmd/mythhelm

echo "record.sh: regenerate the transcript"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
"$BIN" demo --check pass | sed -E -f "$SED" >"$tmp"
# The binary's own exit status, never sed's: without this a failing binary
# still yields a zero pipeline status from sed and masks the failure.
test "${PIPESTATUS[0]}" -eq 0
if test -f "$TRANSCRIPT" && cmp -s "$tmp" "$TRANSCRIPT"; then
	echo "record.sh: transcript unchanged"
else
	cp "$tmp" "$TRANSCRIPT"
	echo "record.sh: transcript written to $TRANSCRIPT"
fi

echo "record.sh: render the GIF from the checked-in tape"
vhs "$TAPE"

echo "record.sh: verify"
# Determinism: the pipeline re-run from the same binary must byte-match the
# transcript just written.
"$BIN" demo --check pass | sed -E -f "$SED" >"$tmp"
test "${PIPESTATUS[0]}" -eq 0
diff "$tmp" "$TRANSCRIPT"
# The transcript keeps the binary's own labels verbatim (I09).
test "$(grep -c 'SCRIPTED DEMO' "$TRANSCRIPT")" -ge 4
# The manifest parses, and its declared revision is a commit in this repo (I07).
python3 - "$MANIFEST" <<'EOF'
import json, subprocess, sys
with open(sys.argv[1], encoding="utf-8") as fh:
    manifest = json.load(fh)
for rec in manifest["recordings"]:
    commit = rec["binary_commit"]
    assert len(commit) == 40 and all(c in "0123456789abcdef" for c in commit), commit
    subprocess.run(["git", "cat-file", "-e", commit], check=True)
    print("record.sh: manifest revision %s (%s) is present" % (commit, rec["name"]))
EOF
# The manifest's VHS pin matches the renderer that just ran.
want="$(python3 -c 'import json;print(json.load(open("'"$MANIFEST"'"))["recordings"][0]["vhs_version"])')"
have="$(vhs --version | sed -E 's/.*(v[0-9]+\.[0-9]+\.[0-9]+).*/\1/')"
test "$want" = "$have" || {
	echo "record.sh: manifest vhs_version $want != installed $have; re-pin the manifest and tape header" >&2
	exit 1
}
# The render produced a GIF.
test "$(file -b "$GIF" | cut -d, -f1)" = "GIF image data"

if test "$REPIN" -eq 1; then
	echo "record.sh: --repin: moving manifest and tape-header pins to $head"
	ver="$("$BIN" version | sed -n '1s/^mythhelm //p')"
	python3 - "$MANIFEST" "$head" "$ver" <<'EOF'
import json, sys
path, head, ver = sys.argv[1], sys.argv[2], sys.argv[3]
with open(path, encoding="utf-8") as fh:
    manifest = json.load(fh)
for rec in manifest["recordings"]:
    rec["binary_commit"] = head
    rec["binary_version"] = ver
with open(path, "w", encoding="utf-8") as fh:
    json.dump(manifest, fh, indent=2)
    fh.write("\n")
print("record.sh: manifest re-pinned to %s (%s)" % (head, ver))
EOF
	sed -E "s|^# Binary-Version: .*|# Binary-Version: $ver|; s|^# Binary-Commit: .*|# Binary-Commit: $head|" "$TAPE" >"$tmp"
	cat "$tmp" >"$TAPE"
fi

echo "record.sh: ok (binary, transcript, GIF verified)"
