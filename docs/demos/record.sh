#!/usr/bin/env bash
# record.sh: the documented record command for the MYTHHELM terminal
# recordings (docs-site-demos, FR-2/FR-4; tui-tape-recording, FR-1/FR-2). It
# builds the binary, regenerates every selected manifest recording's
# transcript, renders every selected GIF from its checked-in tape, renders the
# hero tape also as WebM/MP4 video when hero is selected, generates one poster
# per selected GIF, and verifies all of them (including the hero chapter
# offsets against the rendered videos). Without --recording every manifest
# recording is selected. A bare `vhs docs/demos/demo.tape` only renders one
# GIF and is not the record command.
#
# Usage: docs/demos/record.sh [--repin] [--recording NAME]
#
# --recording NAME selects exactly one manifest recording (demo, tui or hero).
# Unselected recordings stay historical: their pins must name commits in this
# repository, but their files and manifest entries are never rewritten.
#
# Without --repin the checkout must match every selected binary_commit:
# writing fresh artifacts while a stale pin labels them is refused. With
# --repin (on a clean tree) the script regenerates the selected recordings
# from the current checkout and moves only their manifest and tape-header pins
# to it.
#
# Recording uses the fixed paths /tmp/mythhelm-record and /tmp/demo (the
# tapes type them): run one invocation at a time.
#
# Needs only free tooling: go, git, python3, GNU timeout (the hero tape kills
# its run with it; macOS: brew install coreutils and put its timeout on PATH),
# ffmpeg/ffprobe (the OS packages; the posters and the video verification
# need them), file (the GIF and poster format checks need it), ttyd, and VHS
# (vhs renders through ffmpeg and ttyd). Install VHS from https://github.com/charmbracelet/vhs
# (for example `go install github.com/charmbracelet/vhs@latest`) and ttyd from
# https://github.com/tsl0922/ttyd.

# record_main ROOT BIN HERO_TMP [--repin] [--recording NAME]: the whole record
# command. The public entry at the end of this file passes the fixed
# production paths; the three leading arguments are a private seam for
# docs/demos/record_test.sh fixtures, not user options. The body runs in a
# subshell so its directory, shell options and EXIT trap stay scoped.
record_main() (
	set -euo pipefail

	ROOT="$1"
	BIN="$2"
	HERO_TMP="$3"
	shift 3

	usage() {
		echo "record.sh: usage: docs/demos/record.sh [--repin] [--recording NAME]" >&2
		exit 2
	}
	REPIN=0
	seen_repin=0
	selected=""
	seen_recording=0
	while test "$#" -gt 0; do
		case "$1" in
		--repin)
			test "$seen_repin" -eq 0 || usage
			seen_repin=1
			REPIN=1
			shift
			;;
		--recording)
			test "$seen_recording" -eq 0 || usage
			test "$#" -ge 2 || usage
			seen_recording=1
			selected="$2"
			shift 2
			;;
		*)
			usage
			;;
		esac
	done
	if test "$seen_recording" -eq 1; then
		case "$selected" in
		demo | tui | hero) ;;
		*)
			echo "record.sh: unknown recording '$selected' (want demo, tui or hero)" >&2
			exit 2
			;;
		esac
	fi

	cd "$ROOT"

	DEMOS="docs/demos"
	SED="$DEMOS/normalize.sed"
	MANIFEST="$DEMOS/manifest.json"

	for tool in go git vhs ttyd ffmpeg ffprobe python3 timeout file; do
		if ! command -v "$tool" >/dev/null 2>&1; then
			echo "record.sh: need $tool on PATH" >&2
			exit 2
		fi
	done

	# Recordings come from the manifest: names drive the per-recording loop,
	# and each recording carries its own tape/artifact/transcript paths and
	# pins. A selected name must be exactly one manifest entry.
	all="$(python3 -c 'import json,sys;print(" ".join(r["name"] for r in json.load(open(sys.argv[1]))["recordings"]))' "$MANIFEST")"
	if test "$seen_recording" -eq 1; then
		count=0
		for n in $all; do
			if test "$n" = "$selected"; then
				count=$((count + 1))
			fi
		done
		if test "$count" -ne 1; then
			echo "record.sh: recording '$selected' must be exactly one manifest entry (found $count)" >&2
			exit 2
		fi
		names="$selected"
	else
		names="$all"
	fi
	has_hero=0
	for n in $names; do
		if test "$n" = "hero"; then
			has_hero=1
		fi
	done
	field() { # field <name> <key>: one manifest value for one recording
		python3 -c 'import json,sys;m=json.load(open(sys.argv[1]));print([r for r in m["recordings"] if r["name"]==sys.argv[2]][0][sys.argv[3]])' "$MANIFEST" "$1" "$2"
	}

	echo "record.sh: versions"
	go version
	git --version
	vhs --version
	ffprobe -version | sed -n '1p'
	python3 --version
	echo "record.sh: recordings: $names"

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
		# script, every selected recording's tape, and every go:embed asset
		# baked into the binary (extend this list when adding an embed
		# directive). The hero scripts count when hero is selected.
		inputs=('*.go' 'go.mod' 'go.sum' "$SED"
			'adapters/fake/scenarios/*.json'
			'adapters/claudecode/testdata/qualification/routes.json'
			'adapters/codex/testdata/qualification/routes.json'
			'internal/processsupport/SUPPORT.md'
			'internal/journal/migrations/*.sql'
			'mods/themes/*.toml')
		for n in $names; do
			inputs+=("$(field "$n" tape)")
		done
		if test "$has_hero" -eq 1; then
			inputs+=("$DEMOS/hero-*.sh")
		fi
		recording_dirt="$(git status --porcelain -- "${inputs[@]}")"
		if test -n "$recording_dirt"; then
			echo "record.sh: uncommitted build or recording inputs could make the output disagree with the pins" >&2
			echo "record.sh: commit them or re-run with --repin to regenerate and move the pins" >&2
			exit 1
		fi
	fi

	# Validate all output routing before building or writing any recording.
	# Pin fields are outputs: a commit cannot contain its own object ID.
	# All other manifest configuration must match the committed configuration
	# for no-repin runs; a dirty transcript/artifact route is never accepted.
	# shellcheck disable=SC2086 # $names is a word list of validated recording names
	python3 - "$MANIFEST" "$REPIN" $names <<'EOF'
import json,re,subprocess,sys
from pathlib import Path
path,repin,*selected=sys.argv[1:]
manifest=json.loads(Path(path).read_text(encoding="utf-8"))
def refuse(message):
    sys.exit("record.sh: " + message)
if repin == "0":
    committed=json.loads(subprocess.check_output(["git","show","HEAD:"+path]))
    def configuration(value):
        value=json.loads(json.dumps(value))
        for rec in value["recordings"]:
            rec.pop("binary_commit",None)
            rec.pop("binary_version",None)
        return value
    if configuration(manifest) != configuration(committed):
        refuse("uncommitted manifest recording configuration; commit it before recording")
seen=set()
for rec in manifest["recordings"]:
    name=rec["name"]
    if name not in ("demo","tui","hero") or name in seen:
        refuse("unknown or duplicate manifest recording name")
    seen.add(name)
    # These are the existing public literal paths, not configurable output
    # destinations. Enforcing them also prevents cross-record path aliases,
    # traversal and aliases of derived poster/video paths.
    for key,suffix in (("tape",".tape"),("artifact",".gif"),("transcript",".transcript.txt")):
        if rec[key] != "docs/demos/"+name+suffix:
            refuse("%s %s must name its own fixed recording path" % (name,key))
    if name not in selected:
        continue
    tape=Path(rec["tape"]).read_text(encoding="utf-8")
    # VHS skips whitespace between tokens, so a second Output can be
    # indented, follow another command, or put its path on the next line.
    # Ignore quoted Type text, comments and regex literals while counting
    # exact Output keywords; require the sole output's canonical literal
    # line too. This is a restricted tape contract, not a VHS parser.
    tokens=re.finditer(r"\"[^\"\r\n]*\"|'[^'\r\n]*'|`[^`\r\n]*`|#[^\r\n]*|/(?:\\.|[^/\\\r\n])*/|[A-Za-z.][A-Za-z0-9._/%-]*",tape)
    output_count=sum(token.group() == "Output" for token in tokens)
    outputs=re.findall(r"^Output[ \t]+(.*)$",tape,re.M)
    if output_count != 1 or outputs != [rec["artifact"]]:
        refuse("%s tape needs one literal Output matching its artifact" % name)
    for key,pattern in (("Binary-Commit",r"[0-9a-f]{40}"),("Binary-Version",r"\S+")):
        headers=re.findall(r"^# "+key+r":(.*)$",tape,re.M)
        if len(headers) != 1 or re.fullmatch(" "+pattern,headers[0]) is None:
            refuse("%s tape needs exactly one valid %s header" % (name,key))
EOF

	echo "record.sh: build the binary (go build, never go run)"
	go build -o "$BIN" ./cmd/mythhelm

	tmpdir="$(mktemp -d)"
	# The hero scratch directory has a short fixed path so the recording's
	# paths read /tmp/demo/...; hero-env.sh replaces a leftover one that
	# carries its marker file, and the exit trap removes it the same way.
	# shellcheck disable=SC2317,SC2329 # invoked indirectly by the EXIT trap (0.9/newer diagnostics)
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
			# The binary's own exit status, never sed's: without this a
			# failing binary still yields a zero pipeline status from sed and
			# masks the failure.
			test "${PIPESTATUS[0]}" -eq 0
			;;
		tui)
			# Linear-mode outputs driven alongside the interactive takes: the
			# interactive bytes are escape- and timing-dependent and can never
			# byte-match a re-run. Each take is its own pipeline with its own
			# PIPESTATUS assertion — one brace-group pipeline would expose
			# only the last take's status and mask the first take's failure.
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
			# The taped take drives the TUI; this linear companion runs the
			# same commands (run, SIGKILL, runs list, recover, review) with
			# stdout and stderr merged, in a scratch directory of its own.
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

	# Every selected VHS pin matches the installed renderer: checked before
	# any render, so a mismatch never leaves GIFs written by the wrong
	# renderer.
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
		# Determinism: the pipeline re-run from the same binary must
		# byte-match the transcript just written.
		regen_transcript "$n" "$tmpdir/$n.verify"
		diff "$tmpdir/$n.verify" "$transcript"
	done
	# The transcripts keep the binary's own labels verbatim (I09). The demo
	# keeps its floor; the tui counts are exact — with two takes emitting more
	# than any single-take floor, a floor cannot discriminate a deleted label
	# line.
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
			# The run and the recovery each replay the admission decision,
			# which carries the SCRIPTED label; recovery reattaches to the
			# original worker (one notice) and ends the run once.
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
	# The manifest parses, and every declared revision — selected or
	# historical — is a commit in this repo (I07).
	python3 - "$MANIFEST" <<'EOF'
import json, subprocess, sys
with open(sys.argv[1], encoding="utf-8") as fh:
    manifest = json.load(fh)
for rec in manifest["recordings"]:
    commit = rec["binary_commit"]
    assert len(commit) == 40 and all(c in "0123456789abcdef" for c in commit), commit
    subprocess.run(["git", "cat-file", "-e", commit + "^{commit}"], check=True)
    print("record.sh: manifest revision %s (%s) is present" % (commit, rec["name"]))
EOF
	# Every selected tape validates and names exactly one Output (multiple
	# Outputs render only the last, silently — the assertion keeps a stray
	# second Output from shipping), and every render produced a GIF.
	for n in $names; do
		tape="$(field "$n" tape)"
		artifact="$(field "$n" artifact)"
		vhs validate "$tape"
		test "$(grep -c '^Output ' "$tape")" -eq 1
		test "$(file -b "$artifact" | cut -d, -f1)" = "GIF image data"
	done
	probe_dims() { # probe_dims <file>: "<width>,<height>" of the first video stream
		ffprobe -v error -select_streams v:0 -show_entries stream=width,height -of csv=p=0 "$1"
	}
	probe_codec() { # probe_codec <file>: codec name of the first video stream
		ffprobe -v error -select_streams v:0 -show_entries stream=codec_name -of default=noprint_wrappers=1:nokey=1 "$1"
	}
	# Every selected poster is a PNG still at its GIF's dimensions.
	for n in $names; do
		artifact="$(field "$n" artifact)"
		poster="${artifact%.gif}-poster.png"
		test "$(file -b "$poster" | cut -d, -f1)" = "PNG image data"
		test "$(probe_dims "$poster")" = "$(probe_dims "$artifact")"
	done
	if test "$has_hero" -eq 1; then
		# The hero videos are real renders of the hero tape (VP9 in WebM,
		# H.264 in MP4) at the hero GIF's dimensions.
		hero_dims="$(probe_dims "$(field hero artifact)")"
		test "$(probe_codec "$DEMOS/hero.webm")" = "vp9"
		test "$(probe_dims "$DEMOS/hero.webm")" = "$hero_dims"
		test "$(probe_codec "$DEMOS/hero.mp4")" = "h264"
		test "$(probe_dims "$DEMOS/hero.mp4")" = "$hero_dims"
		# The hero chapter offsets name the tape's beats (title, run, kill,
		# recover, review), strictly increase, and fit inside the rendered
		# videos. Durations are read from the video files with ffprobe, never
		# from a manifest field (AC-2.2).
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
	fi

	if test "$REPIN" -eq 1; then
		echo "record.sh: --repin: moving the pins of $names to $head"
		repin_incomplete() {
			echo "record.sh: --repin incomplete: some pins of $names may already name $head and others not; nothing was rolled back, inspect git diff -- $MANIFEST" >&2
			exit 1
		}
		ver="$("$BIN" version | sed -n '1s/^mythhelm //p')"
		test -n "$ver" || repin_incomplete
		# Rewrite only the selected entries' binary_commit/binary_version
		# string values in place, so every other byte of the shared manifest
		# (unselected entries, keys, capabilities, chapters, formatting) stays
		# as it was.
		# shellcheck disable=SC2086 # $names is a word list of manifest names
		python3 - "$MANIFEST" "$head" "$ver" $names <<'EOF' || repin_incomplete
import json, os, re, stat, sys, tempfile
path, head, ver, names = sys.argv[1], sys.argv[2], sys.argv[3], set(sys.argv[4:])
with open(path, encoding="utf-8") as fh:
    text = fh.read()
before = json.loads(text)
dec = json.JSONDecoder()
start = re.search(r'"recordings"\s*:\s*\[', text).end()
spans, i = [], start
while True:
    i = re.compile(r'[\s,]*').match(text, i).end()
    if text[i] == "]":
        break
    rec, end = dec.raw_decode(text, i)
    spans.append((rec["name"], i, end))
    i = end
assert [s[0] for s in spans] == [r["name"] for r in before["recordings"]], spans
out, last = [], 0
for name, s, e in spans:
    if name not in names:
        continue
    chunk = text[s:e]
    for key, val in (("binary_commit", head), ("binary_version", ver)):
        chunk, n = re.subn(r'("%s"\s*:\s*)"[^"\\]*"' % key, lambda m: m.group(1) + json.dumps(val), chunk)
        assert n == 1, (name, key, n)
    out.append(text[last:s])
    out.append(chunk)
    last = e
out.append(text[last:])
new = "".join(out)
expect = before
for rec in expect["recordings"]:
    if rec["name"] in names:
        rec["binary_commit"], rec["binary_version"] = head, ver
assert json.loads(new) == expect
# Replace the complete manifest atomically: a failed/interrupted write never
# truncates the previous manifest. Tape headers remain separate writes below.
fd, staged = tempfile.mkstemp(prefix=".manifest-", dir=os.path.dirname(path))
try:
    with os.fdopen(fd, "w", encoding="utf-8") as fh:
        fh.write(new)
        fh.flush()
        os.fsync(fh.fileno())
    os.chmod(staged, stat.S_IMODE(os.stat(path).st_mode))
    os.replace(staged, path)
finally:
    if os.path.exists(staged):
        os.unlink(staged)
print("record.sh: manifest re-pinned %s to %s (%s)" % (" ".join(sorted(names)), head, ver))
EOF
		for n in $names; do
			tape="$(field "$n" tape)" || repin_incomplete
			sed -E "s|^# Binary-Version: .*|# Binary-Version: $ver|; s|^# Binary-Commit: .*|# Binary-Commit: $head|" "$tape" >"$tmpdir/repin" || repin_incomplete
			cat "$tmpdir/repin" >"$tape" || repin_incomplete
			# Verify the actual rewritten header, not just sed's exit status.
			test "$(grep -cFx "# Binary-Commit: $head" "$tape")" -eq 1 || repin_incomplete
			test "$(grep -cFx "# Binary-Version: $ver" "$tape")" -eq 1 || repin_incomplete
		done
	fi

	if test "$has_hero" -eq 1; then
		echo "record.sh: ok ($names: binary, transcripts, GIFs, hero videos, posters, chapters verified)"
	else
		echo "record.sh: ok ($names: binary, transcripts, GIFs, posters verified)"
	fi
)

if test "${BASH_SOURCE[0]}" = "$0"; then
	record_main "$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)" /tmp/mythhelm-record /tmp/demo "$@"
fi
