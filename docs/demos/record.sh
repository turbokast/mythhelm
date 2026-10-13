#!/usr/bin/env bash
# record.sh: the documented record command for the MYTHHELM terminal
# recordings (docs-site-demos, FR-2/FR-4; tui-tape-recording, FR-1/FR-2). It
# builds the binary, regenerates every manifest recording's transcript,
# renders every GIF from its checked-in tape, renders the hero tape also
# as WebM/MP4 video, generates one poster per GIF, and verifies all of
# them (including the hero chapter offsets against the rendered videos).
# A bare `vhs docs/demos/demo.tape` only renders one GIF and is not the
# record command.
#
# Usage: docs/demos/record.sh [--repin]
#
# Without flags the checkout must match every manifest binary_commit: writing
# fresh artifacts while a stale pin labels them is refused. With --repin (on
# a clean tree) the script regenerates from the current checkout and moves the
# manifest and tape-header pins to it.
#
# Needs only free tooling: go, git, python3, GNU timeout (the hero tape kills
# its run with it; macOS: brew install coreutils and put its timeout on PATH),
# ffmpeg/ffprobe (the OS packages; the posters and the video verification
# need them), and a VHS install (vhs renders
# through ffmpeg and ttyd). Install VHS from https://github.com/charmbracelet/vhs
# (for example `go install github.com/charmbracelet/vhs@latest`) and ttyd from
# https://github.com/tsl0922/ttyd.

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
SED="$DEMOS/normalize.sed"
MANIFEST="$DEMOS/manifest.json"

for tool in go git vhs ffmpeg ffprobe python3 timeout; do
	if ! command -v "$tool" >/dev/null 2>&1; then
		echo "record.sh: need $tool on PATH" >&2
		exit 2
	fi
done

echo "record.sh: versions"
go version
git --version
vhs --version
ffprobe -version | sed -n '1p'
python3 --version

# Recordings come from the manifest: names drive the per-recording loop, and
# each recording carries its own tape/artifact/transcript paths and pins.
names="$(python3 -c 'import json;print(" ".join(r["name"] for r in json.load(open("'"$MANIFEST"'"))["recordings"]))')"
field() { # field <name> <key>: one manifest value for one recording
	python3 -c 'import json,sys;m=json.load(open("'"$MANIFEST"'"));print([r for r in m["recordings"] if r["name"]==sys.argv[1]][0][sys.argv[2]])' "$1" "$2"
}

head="$(git rev-parse HEAD)"
if test "$REPIN" -eq 0; then
	for n in $names; do
		pin="$(field "$n" binary_commit)"
		if test "$head" != "$pin"; then
			echo "record.sh: checkout $head != $n binary_commit $pin; refusing to write artifacts another revision labels" >&2
			echo "record.sh: re-run with --repin to regenerate and move the pins (or check out $pin)" >&2
			exit 1
		fi
	done
fi
if test "$REPIN" -eq 1 && test -n "$(git status --porcelain)"; then
	echo "record.sh: --repin needs a clean tree (pins must name a reproducible commit)" >&2
	exit 1
fi
if test "$REPIN" -eq 0; then
	# Recording inputs: Go sources, the module files, the normalization
	# script, every recording's tape, and every go:embed asset baked into
	# the binary (extend this list when adding an embed directive).
	tapes=()
	for n in $names; do
		tapes+=("$(field "$n" tape)")
	done
	recording_dirt="$(git status --porcelain -- \
		'*.go' 'go.mod' 'go.sum' "$SED" "${tapes[@]}" \
		'adapters/fake/scenarios/*.json' \
		"$DEMOS/hero-*.sh" \
		'internal/journal/migrations/*.sql' \
		'mods/themes/*.toml')"
	if test -n "$recording_dirt"; then
		echo "record.sh: uncommitted build or recording inputs could make the output disagree with the pins" >&2
		echo "record.sh: commit them or re-run with --repin to regenerate and move the pins" >&2
		exit 1
	fi
fi

echo "record.sh: build the binary (go build, never go run)"
go build -o "$BIN" ./cmd/mythhelm

tmpdir="$(mktemp -d)"
# The hero scratch directory has a short fixed path so the recording's paths
# read /tmp/demo/...; hero-env.sh replaces a leftover one that carries its
# marker file, and the exit trap removes it the same way.
HERO_TMP="/tmp/demo"
cleanup() {
	rm -rf "$tmpdir"
	if test -f "$HERO_TMP/.mythhelm-hero"; then
		rm -rf "$HERO_TMP"
	fi
}
trap cleanup EXIT

# regen_transcript <name> <out>: run one recording's transcript pipeline.
# Per-name functions, never manifest commands: the manifest is data the
# checks parse, not executable content (tui-tape-recording D9).
regen_transcript() {
	case "$1" in
	demo)
		"$BIN" demo --check pass | sed -E -f "$SED" >"$2"
		# The binary's own exit status, never sed's: without this a failing
		# binary still yields a zero pipeline status from sed and masks the
		# failure.
		test "${PIPESTATUS[0]}" -eq 0
		;;
	tui)
		# Linear-mode outputs driven alongside the interactive takes: the
		# interactive bytes are escape- and timing-dependent and can never
		# byte-match a re-run. Each take is its own pipeline with its own
		# PIPESTATUS assertion — one brace-group pipeline would expose only
		# the last take's status and mask the first take's failure.
		TERM=dumb "$BIN" demo --check pass 2>&1 | sed -E -f "$SED" >"$tmpdir/tui.t1"
		test "${PIPESTATUS[0]}" -eq 0
		"$BIN" demo --check pass --accessible 2>&1 | sed -E -f "$SED" >"$tmpdir/tui.t2"
		test "${PIPESTATUS[0]}" -eq 0
		{
			echo "=== take: linear (TERM=dumb) ==="
			cat "$tmpdir/tui.t1"
			echo "=== take: accessible (--accessible) ==="
			cat "$tmpdir/tui.t2"
		} >"$2"
		;;
	hero)
		# The taped take drives the TUI; this linear companion runs the same
		# commands (run, SIGKILL, runs list, recover, review) with stdout and
		# stderr merged, in a scratch directory of its own.
		(HERO_BIN="$BIN" HERO_TMP="$HERO_TMP" "$DEMOS/hero-transcript.sh" 2>&1) | sed -E -f "$SED" >"$2"
		test "${PIPESTATUS[0]}" -eq 0
		;;
	*)
		echo "record.sh: recording '$1' has no transcript pipeline; add one here (never in the manifest)" >&2
		exit 2
		;;
	esac
}

echo "record.sh: regenerate the transcripts"
for n in $names; do
	transcript="$(field "$n" transcript)"
	regen_transcript "$n" "$tmpdir/$n.transcript"
	if test -f "$transcript" && cmp -s "$tmpdir/$n.transcript" "$transcript"; then
		echo "record.sh: $n transcript unchanged"
	else
		cp "$tmpdir/$n.transcript" "$transcript"
		echo "record.sh: $n transcript written to $transcript"
	fi
done

# Every manifest VHS pin matches the installed renderer: checked before any
# render, so a mismatch never leaves GIFs written by the wrong renderer.
have="$(vhs --version | sed -E 's/.*(v[0-9]+\.[0-9]+\.[0-9]+).*/\1/')"
for n in $names; do
	want="$(field "$n" vhs_version)"
	test "$want" = "$have" || {
		echo "record.sh: $n vhs_version $want != installed $have; re-pin the manifest and tape header" >&2
		exit 1
	}
done

echo "record.sh: render the GIFs from the checked-in tapes"
for n in $names; do
	if test "$n" = "hero"; then
		# One hero run renders the GIF plus the WebM/MP4 videos the
		# landing player serves (AC-2.2): -o adds outputs, it does not
		# replace the tape's own GIF output.
		HERO_BIN="$BIN" HERO_TMP="$HERO_TMP" vhs -o "$DEMOS/hero.webm" -o "$DEMOS/hero.mp4" "$(field "$n" tape)"
	else
		HERO_BIN="$BIN" HERO_TMP="$HERO_TMP" vhs "$(field "$n" tape)"
	fi
done

# poster_seek <name>: the GIF timestamp (seconds) of the still each poster
# shows. The hero poster is its title card (the first frame); the demo and
# tui first frames are the blank prompt before typing starts, so their
# posters use the first held content frame instead.
poster_seek() { # poster_seek <name>
	case "$1" in
	demo) echo 5 ;;
	tui) echo 6 ;;
	hero) echo 0 ;;
	*)
		echo "record.sh: recording '$1' has no poster timestamp; add one here" >&2
		exit 2
		;;
	esac
}

echo "record.sh: generate the recording posters from the rendered GIFs"
for n in $names; do
	artifact="$(field "$n" artifact)"
	ffmpeg -y -v error -i "$artifact" -ss "$(poster_seek "$n")" -frames:v 1 "${artifact%.gif}-poster.png"
done

echo "record.sh: verify"
for n in $names; do
	transcript="$(field "$n" transcript)"
	# Determinism: the pipeline re-run from the same binary must byte-match
	# the transcript just written.
	regen_transcript "$n" "$tmpdir/$n.verify"
	diff "$tmpdir/$n.verify" "$transcript"
done
# The transcripts keep the binary's own labels verbatim (I09). The demo keeps
# its floor; the tui counts are exact — with two takes emitting more than any
# single-take floor, a floor cannot discriminate a deleted label line.
assert_labels() { # assert_labels <name> <transcript>
	case "$1" in
	demo)
		test "$(grep -c 'SCRIPTED DEMO' "$2")" -ge 4
		;;
	tui)
		test "$(grep -c 'SCRIPTED DEMO' "$2")" -eq 8
		test "$(grep -c '^goal: ' "$2")" -eq 1
		test "$(grep -c '^run state: ' "$2")" -eq 1
		test "$(grep -c '^attempt' "$2")" -eq 8
		test "$(grep -c '^candidate: ' "$2")" -eq 1
		test "$(grep -c '^next action: ' "$2")" -eq 1
		test "$(grep -c '^actions: ' "$2")" -eq 1
		;;
	hero)
		# The run and the recovery each replay the admission decision, which
		# carries the SCRIPTED label; recovery reattaches to the original
		# worker (one notice) and ends the run once.
		test "$(grep -c '^  SCRIPTED: fake adapter' "$2")" -eq 2
		test "$(grep -c '^reattached to worker pid ' "$2")" -eq 1
		test "$(grep -c '^exit status 137$' "$2")" -eq 1
		test "$(grep -c '^NNN$' "$2")" -eq 1
		test "$(grep -c '^result: ready_for_review; exit 0' "$2")" -eq 1
		test "$(grep -c '^State: ready_for_review$' "$2")" -eq 1
		test "$(grep -c '^attempt: worker pid ' "$2")" -eq 2
		;;
	*)
		echo "record.sh: recording '$1' has no label assertions; add them here" >&2
		exit 2
		;;
	esac
}
for n in $names; do
	assert_labels "$n" "$(field "$n" transcript)"
done
# The manifest parses, and its declared revisions are commits in this repo (I07).
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
# Every tape validates and names exactly one Output (multiple Outputs render
# only the last, silently — the assertion keeps a stray second Output from
# shipping), and every render produced a GIF.
for n in $names; do
	tape="$(field "$n" tape)"
	artifact="$(field "$n" artifact)"
	vhs validate "$tape"
	test "$(grep -c '^Output ' "$tape")" -eq 1
	test "$(file -b "$artifact" | cut -d, -f1)" = "GIF image data"
done
# The hero videos are real renders of the hero tape (VP9 in WebM, H.264 in
# MP4) at the hero GIF's dimensions, and every poster is a PNG still at its
# GIF's dimensions.
probe_dims() { # probe_dims <file>: "<width>,<height>" of the first video stream
	ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0 "$1"
}
probe_codec() { # probe_codec <file>: codec name of the first video stream
	ffprobe -v error -select_streams v:0 -show_entries stream=codec_name -of default=noprint_wrappers=1:nokey=1 "$1"
}
hero_dims="$(probe_dims "$(field hero artifact)")"
test "$(probe_codec "$DEMOS/hero.webm")" = "vp9"
test "$(probe_dims "$DEMOS/hero.webm")" = "$hero_dims"
test "$(probe_codec "$DEMOS/hero.mp4")" = "h264"
test "$(probe_dims "$DEMOS/hero.mp4")" = "$hero_dims"
for n in $names; do
	artifact="$(field "$n" artifact)"
	poster="${artifact%.gif}-poster.png"
	test "$(file -b "$poster" | cut -d, -f1)" = "PNG image data"
	test "$(probe_dims "$poster")" = "$(probe_dims "$artifact")"
done
# The hero chapter offsets name the tape's beats (title, run, kill,
# recover, review), strictly increase, and fit inside the rendered videos.
# Durations are read from the video files with ffprobe, never from a
# manifest field (AC-2.2).
webm_duration="$(ffprobe -v error -show_entries format=duration -of default=noprint_wrappers=1:nokey=1 "$DEMOS/hero.webm")"
mp4_duration="$(ffprobe -v error -show_entries format=duration -of default=noprint_wrappers=1:nokey=1 "$DEMOS/hero.mp4")"
python3 - "$MANIFEST" "$webm_duration" "$mp4_duration" <<'EOF'
import json, sys
path, webm_s, mp4_s = sys.argv[1], float(sys.argv[2]), float(sys.argv[3])
with open(path, encoding="utf-8") as fh:
    manifest = json.load(fh)
hero = [r for r in manifest["recordings"] if r["name"] == "hero"][0]
chapters = hero["chapters"]
assert [c["name"] for c in chapters] == ["title", "run", "kill", "recover", "review"], chapters
offsets = [c["at_seconds"] for c in chapters]
assert all(b > a for a, b in zip(offsets, offsets[1:])), offsets
limit = min(webm_s, mp4_s)
assert offsets[0] >= 0 and offsets[-1] <= limit, (offsets, limit)
print("record.sh: hero chapters %s fit the rendered durations (webm %.2fs, mp4 %.2fs)" % (offsets, webm_s, mp4_s))
EOF

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
	for n in $names; do
		tape="$(field "$n" tape)"
		sed -E "s|^# Binary-Version: .*|# Binary-Version: $ver|; s|^# Binary-Commit: .*|# Binary-Commit: $head|" "$tape" >"$tmpdir/repin"
		cat "$tmpdir/repin" >"$tape"
	done
fi

echo "record.sh: ok (binary, transcripts, GIFs, hero videos, posters, chapters verified)"
