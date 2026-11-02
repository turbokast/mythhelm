#!/usr/bin/env bash
# record_test.sh: fixture tests for docs/demos/record.sh (tui-observatory-view
# Task 21). Each case copies the actual record.sh into a fresh temporary Git
# repository with synthetic tapes, transcripts and manifest, puts fake
# go/vhs/ttyd/ffmpeg/ffprobe/file/timeout on PATH, and runs the shared
# record_main body with fixture-owned ROOT, BIN, HERO_TMP and TMPDIR. The
# public entry is invoked only for refusals that must stop before any build.
#
# The fake tools write sentinel media: these tests qualify the script's
# selection, provenance and failure routing, not real rendering, real
# recordings or accessibility. Nothing here runs real VHS or the real binary.
#
# Usage: docs/demos/record_test.sh

# SC2016: single-quoted bash -c bodies and sed programs expand later on
# purpose. SC2030/SC2031: PATH and FAKE_LOG are scoped to subshells on purpose.
# shellcheck disable=SC2016,SC2030,SC2031

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$HERE/record.sh"
WORK="$(mktemp -d)"
trap 'chmod -R u+w "$WORK"; rm -rf "$WORK"' EXIT

pass=0
fail=0
ok() {
	pass=$((pass + 1))
	echo "ok   $1"
}
bad() {
	fail=$((fail + 1))
	echo "FAIL $1" >&2
}

# fake_tools <dir>: synthetic stand-ins for every external recording tool.
# Their behaviour is steered by FAKE_FAIL (one failure point) and FAKE_LOG.
fake_tools() {
	local d="$1"
	mkdir -p "$d"
	cat >"$d/go" <<'EOF'
#!/usr/bin/env bash
echo "go $*" >>"$FAKE_LOG"
case "$1" in
version) echo "go version go0.0-fixture" ;;
build)
	test -n "${FAKE_GO_REFUSE:-}" && { echo "fake go: refusing build" >&2; exit 1; }
	test "$2" = "-o"
	cat >"$3" <<'BIN'
#!/usr/bin/env bash
echo "bin $*" >>"$FAKE_LOG"
case "$1" in
version) echo "mythhelm v9.9.9-fixture"; exit 0 ;;
esac
acc=0
for a in "$@"; do test "$a" = "--accessible" && acc=1; done
if test "$acc" -eq 1; then
	test "${FAKE_FAIL:-}" = take2 && { echo "SCRIPTED DEMO partial"; exit 3; }
	for i in 1 2 3 4; do echo "SCRIPTED DEMO $i"; echo "attempt $i"; done
	printf '%s\n' "goal: Demo task" "run state: done" "candidate: c1" "next action: review" "actions: review"
else
	test "${FAKE_FAIL:-}" = take1 && { echo "SCRIPTED DEMO partial"; exit 3; }
	for i in 1 2 3 4; do echo "SCRIPTED DEMO $i"; echo "attempt $i"; done
fi
if test "${FAKE_FAIL:-}" = drift; then
	c="$FAKE_STATE/drift"; n=$(( $(cat "$c" 2>/dev/null || echo 0) + 1 )); echo "$n" >"$c"; echo "run $n"
fi
exit 0
BIN
	chmod +x "$3"
	;;
*) exit 1 ;;
esac
EOF
	cat >"$d/vhs" <<'EOF'
#!/usr/bin/env bash
echo "vhs $*" >>"$FAKE_LOG"
if test "$1" = "--version"; then echo "vhs version ${FAKE_VHS_VERSION:-v0.12.1} (fixture)"; exit 0; fi
if test "$1" = "validate"; then test "${FAKE_FAIL:-}" != validate; exit; fi
extra=()
while test "$1" = "-o"; do extra+=("$2"); shift 2; done
out="$(sed -n 's/^Output //p' "$1" | tail -n 1)"
printf 'GIF89a fixture %s\n' "$1" >"$out"
test "${FAKE_FAIL:-}" = render && exit 4
for o in "${extra[@]}"; do printf 'VIDEO fixture %s\n' "$o" >"$o"; done
exit 0
EOF
	cat >"$d/ffmpeg" <<'EOF'
#!/usr/bin/env bash
echo "ffmpeg $*" >>"$FAKE_LOG"
test "${FAKE_FAIL:-}" = poster && exit 5
for a in "$@"; do out="$a"; done
printf 'PNGSENT fixture\n' >"$out"
EOF
	cat >"$d/ffprobe" <<'EOF'
#!/usr/bin/env bash
echo "ffprobe $*" >>"$FAKE_LOG"
case "$*" in
-version) echo "ffprobe version fixture" ;;
*codec_name*) case "${!#}" in *.webm) echo vp9 ;; *.mp4) echo h264 ;; *) echo gif ;; esac ;;
*width,height*) echo "1280,650" ;;
*duration*) echo "${FAKE_DURATION:-32.24}" ;;
*) exit 1 ;;
esac
EOF
	cat >"$d/file" <<'EOF'
#!/usr/bin/env bash
f="${!#}"
case "$(head -c 7 "$f")" in
GIF89a*) echo "GIF image data, version 89a, 1280 x 650" ;;
PNGSENT) echo "PNG image data, 1280 x 650, 8-bit/color RGBA" ;;
*) echo "data" ;;
esac
EOF
	printf '#!/usr/bin/env bash\nexit 0\n' >"$d/ttyd"
	printf '#!/usr/bin/env bash\nexit 0\n' >"$d/timeout"
	chmod +x "$d"/*
}

# tape <name> <commit>: a synthetic tape with the production header shape.
tape() {
	printf '# synthetic fixture tape %s\n# Binary-Version: v0.0.1-old\n# Binary-Commit: %s\n\nOutput docs/demos/%s.gif\nType "echo %s"\n' "$1" "$2" "$1" "$1"
}

# manifest <demo-pin> <tui-pin> <hero-pin>: sentinel formatting (tabs, compact
# objects, odd spacing, a non-ASCII value) that any global re-serialisation
# would change. expected_manifest relies on its line ranges per entry.
manifest() {
	cat <<EOF
{
	"recordings": [
		{"name": "demo", "tape": "docs/demos/demo.tape", "artifact": "docs/demos/demo.gif", "transcript": "docs/demos/demo.transcript.txt",
		 "binary_version": "v0.0.1-old",
		 "binary_commit": "$1",
		 "vhs_version": "v0.12.1", "capabilities": [{"name": "offline é run", "qualifier": "TestA"}]},
		{
		  "name"   : "tui",
		  "tape": "docs/demos/tui.tape", "artifact": "docs/demos/tui.gif", "transcript": "docs/demos/tui.transcript.txt",
		  "binary_version":"v0.0.1-old",
		  "binary_commit":"$2",
		  "vhs_version": "v0.12.1",
		  "capabilities": [ {"name": "mission view", "qualifier": "TestB"} ]
		},
		{"name": "hero", "tape": "docs/demos/hero.tape", "artifact": "docs/demos/hero.gif", "transcript": "docs/demos/hero.transcript.txt",
		 "binary_version": "v0.0.1-old", "binary_commit": "$3", "vhs_version": "v0.12.1",
		 "capabilities": [],
		 "chapters": [ { "name": "title", "at_seconds": 0 }, {"name":"run","at_seconds":2}, { "name": "kill", "at_seconds": 13 }, { "name": "recover", "at_seconds": 19 }, { "name": "review", "at_seconds": 26 } ]}
	]
}
EOF
}

g() { # g <repo> <git args...>: git isolated from the caller's identity, signing and hooks
	git -C "$1" -c user.name=fixture -c user.email=fixture@example.com \
		-c commit.gpgsign=false -c core.hooksPath=/dev/null "${@:2}"
}

commit() { # commit <repo> [message]: commit everything; prints the new HEAD
	g "$1" add -A
	g "$1" commit -qm "${2:-next}"
	g "$1" rev-parse HEAD
}

# fixture <dir> [script]: a Git repository whose first commit C1 holds every
# recording input with historical artifacts and whose HEAD (C2) pins every
# recording to C1; prints C1.
fixture() {
	local r="$1" script="${2:-$SCRIPT}" c1
	mkdir -p "$r/docs/demos" "$r/cmd/mythhelm"
	cp "$script" "$r/docs/demos/record.sh"
	printf 's#^[0-9]+$#NNN#\ns#[0-9a-f]{40}#<SHA40>#g\n' >"$r/docs/demos/normalize.sed"
	printf 'package main\n\nfunc main() {}\n' >"$r/cmd/mythhelm/main.go"
	printf 'module example.com/fixture\n' >"$r/go.mod"
	cat >"$r/docs/demos/hero-transcript.sh" <<'EOF'
#!/usr/bin/env bash
mkdir -p "$HERO_TMP"
touch "$HERO_TMP/.mythhelm-hero"
test "${FAKE_FAIL:-}" = hero && exit 6
printf '%s\n' "  SCRIPTED: fake adapter" "exit status 137" "4242" "  SCRIPTED: fake adapter" \
	"reattached to worker pid 4242" "attempt: worker pid 4242" "attempt: worker pid 4242" \
	"result: ready_for_review; exit 0" "State: ready_for_review"
EOF
	chmod +x "$r/docs/demos/hero-transcript.sh" "$r/docs/demos/record.sh"
	for n in demo tui hero; do
		tape "$n" 0000000000000000000000000000000000000000 >"$r/docs/demos/$n.tape"
		printf 'historical %s gif\n' "$n" >"$r/docs/demos/$n.gif"
		printf 'historical %s poster\n' "$n" >"$r/docs/demos/$n-poster.png"
		printf 'historical %s transcript\n' "$n" >"$r/docs/demos/$n.transcript.txt"
	done
	printf 'historical webm\n' >"$r/docs/demos/hero.webm"
	printf 'historical mp4\n' >"$r/docs/demos/hero.mp4"
	manifest 0000000000000000000000000000000000000000 0000000000000000000000000000000000000000 0000000000000000000000000000000000000000 >"$r/docs/demos/manifest.json"
	git init -q "$r"
	c1="$(commit "$r" c1)"
	manifest "$c1" "$c1" "$c1" >"$r/docs/demos/manifest.json"
	commit "$r" pins >/dev/null
	echo "$c1"
}

# snapshot <dir> <out>: sha256 of every file outside .git.
snapshot() {
	(cd "$1" && find . -path ./.git -prune -o -type f -print | LC_ALL=C sort | xargs sha256sum) >"$2"
}

# changed <before> <after>: the paths whose bytes differ or appeared, sorted.
changed() {
	# diff exits 1 on any difference; under pipefail that status would make
	# every negated caller pass, so only diff's trouble status (2) fails.
	{ diff "$1" "$2" || test "$?" -eq 1; } | sed -n 's/^> [0-9a-f]* *\.\///p' | LC_ALL=C sort -u
}
export -f changed

# run_body <case-dir> <args...>: source the fixture's own copy of record.sh
# and run record_main with fixture-owned paths and fake tools. Sets rc.
# Never call it from an if/&&/|| context: bash ignores set -e there, even
# inside record_main's own subshell.
run_body() {
	local c="$1"
	shift
	mkdir -p "$c/tmp" "$c/state"
	: >"$c/log"
	set +e
	(
		export PATH="$c/fake:$PATH" TMPDIR="$c/tmp" FAKE_LOG="$c/log" FAKE_STATE="$c/state"
		# shellcheck disable=SC1091
		source "$c/repo/docs/demos/record.sh"
		record_main "$c/repo" "$c/bin/mythhelm-record" "$c/hero" "$@"
	) >"$c/out" 2>&1
	rc=$?
	set -e
}

# run_with <VAR=value...> -- <record_main args...>: run_body with extra
# environment. Sets rc.
run_with() {
	local envs=()
	while test "$1" != "--"; do
		envs+=("$1")
		shift
	done
	shift
	set +e
	(
		export "${envs[@]}"
		run_body "$C" "$@"
		exit "$rc"
	)
	rc=$?
	set -e
}

# new_case <name> [script]: fresh case directory with repo and fake tools;
# sets C (case dir), C1 (historical pin) and C2 (HEAD).
cases=0
new_case() {
	cases=$((cases + 1))
	C="$WORK/$cases-$1"
	mkdir -p "$C/bin" "$C/tmp"
	fake_tools "$C/fake"
	C1="$(fixture "$C/repo" "${2:-$SCRIPT}")"
	C2="$(g "$C/repo" rev-parse HEAD)"
}

set_pins() { # set_pins <demo> <tui> <hero>: rewrite the fixture manifest's pins
	manifest "$1" "$2" "$3" >"$C/repo/docs/demos/manifest.json"
}

pin_of() { # pin_of <name> <key>
	python3 -c 'import json,sys;print([r for r in json.load(open(sys.argv[1]))["recordings"] if r["name"]==sys.argv[2]][0][sys.argv[3]])' "$C/repo/docs/demos/manifest.json" "$1" "$2"
}

cleaned() { # the invocation tempdir and the marker-owned hero scratch are gone
	test -z "$(ls -A "$C/tmp")" && test ! -e "$C/hero"
}

expect_rc() { # expect_rc <label> <want>
	if test "$rc" -eq "$2"; then ok "$1 exits $2"; else
		bad "$1 exits $rc, want $2"
		sed 's/^/     | /' "$C/out" >&2
	fi
}

check() { # check <label> <command...>
	local label="$1"
	shift
	if "$@"; then ok "$label"; else bad "$label"; fi
}

# expected_manifest <selected...>: the fixture manifest (all pins C1) with
# exactly the selected entries' version/commit values moved to HEAD.
expected_manifest() {
	local d="$C1" t="$C1" h="$C1" dv=v0.0.1-old tv=v0.0.1-old hv=v0.0.1-old n head
	head="$(g "$C/repo" rev-parse HEAD)"
	for n in "$@"; do
		case "$n" in
		demo) d="$head" dv=v9.9.9-fixture ;;
		tui) t="$head" tv=v9.9.9-fixture ;;
		hero) h="$head" hv=v9.9.9-fixture ;;
		esac
	done
	manifest "$d" "$t" "$h" | sed "3,6s/v0.0.1-old/$dv/; 7,14s/v0.0.1-old/$tv/; 15,18s/v0.0.1-old/$hv/"
}

manifest_is() { # manifest_is <selected...>
	cmp -s "$C/repo/docs/demos/manifest.json" <(expected_manifest "$@")
}

only_changed() { # only_changed <path...>: the snapshot diff is exactly these paths
	diff <(changed "$C/before" "$C/after") <(printf '%s\n' "$@" | LC_ALL=C sort -u) >/dev/null
}

unchanged_except() { # unchanged_except <ERE>: every changed path matches the ERE
	! changed "$C/before" "$C/after" | grep -Ev "$1" >/dev/null
}

# --- shared-body success -------------------------------------------------

# Default-all, no repin: every pin names HEAD (uncommitted manifest edit).
new_case default-all
set_pins "$C2" "$C2" "$C2"
snapshot "$C/repo" "$C/before"
run_body "$C"
snapshot "$C/repo" "$C/after"
expect_rc "default-all" 0
check "default-all writes every recording's media, posters and transcripts, nothing else" only_changed \
	docs/demos/{demo,tui,hero}.gif docs/demos/{demo,tui,hero}-poster.png docs/demos/{demo,tui,hero}.transcript.txt \
	docs/demos/hero.webm docs/demos/hero.mp4
check "default-all tui transcript keeps both takes' labels" test "$(grep -c 'SCRIPTED DEMO' "$C/repo/docs/demos/tui.transcript.txt")" -eq 8
check "default-all verifies hero chapters" grep -q 'hero chapters' "$C/out"
check "default-all cleans its tempdir and hero scratch" cleaned

# Default-all --repin: a clean tree moves every pin and tape header.
new_case default-repin
run_body "$C" --repin
expect_rc "default-all --repin" 0
check "default-all --repin moves every manifest pin, keeping every other byte" manifest_is demo tui hero
check "default-all --repin moves every tape header" test "$(grep -l "^# Binary-Commit: $C2$" "$C/repo"/docs/demos/*.tape | wc -l)" -eq 3

# tui selected --repin with historical demo/hero pins (mixed-pin sentinel).
tui_repin_case() { # tui_repin_case <name> [script] [VAR=value...]
	new_case "$1" "${2:-$SCRIPT}"
	snapshot "$C/repo" "$C/before"
	run_with FAKE_FAIL="${3:-none}" -- --recording tui --repin
	snapshot "$C/repo" "$C/after"
}
selected_tui_only() { # only tui files and the manifest's tui values changed
	only_changed docs/demos/tui.gif docs/demos/tui-poster.png docs/demos/tui.transcript.txt \
		docs/demos/tui.tape docs/demos/manifest.json && manifest_is tui
}
tui_repin_case tui-repin
expect_rc "--recording tui --repin" 0
check "tui --repin changes only tui files and tui's two manifest values (I07)" selected_tui_only
check "tui --repin pins tui to HEAD" test "$(pin_of tui binary_commit)" = "$C2"
check "tui --repin leaves demo and hero historical" test "$(pin_of demo binary_commit) $(pin_of hero binary_commit)" = "$C1 $C1"
check "tui --repin moves the tui tape header" grep -q "^# Binary-Commit: $C2$" "$C/repo/docs/demos/tui.tape"
check "tui --repin never renders or probes unselected media" bash -c '! grep -Eq "demo\.(tape|gif)|hero\.|hero-" "$1"' _ "$C/log"
check "tui --repin cleans its tempdir" cleaned

new_case tui-order
run_body "$C" --repin --recording tui
expect_rc "--repin --recording tui" 0
check "--repin --recording tui moves only tui" manifest_is tui

# tui selected without repin: only tui's pin must equal HEAD.
new_case tui-norepin
set_pins "$C1" "$C2" "$C1"
snapshot "$C/repo" "$C/before"
run_body "$C" --recording tui
snapshot "$C/repo" "$C/after"
expect_rc "--recording tui with historical unselected pins" 0
check "--recording tui writes only tui media" only_changed docs/demos/tui.gif docs/demos/tui-poster.png docs/demos/tui.transcript.txt

new_case hero-repin
snapshot "$C/repo" "$C/before"
run_body "$C" --recording hero --repin
snapshot "$C/repo" "$C/after"
expect_rc "--recording hero --repin" 0
check "hero --repin changes only hero files" only_changed docs/demos/hero.gif docs/demos/hero-poster.png \
	docs/demos/hero.transcript.txt docs/demos/hero.webm docs/demos/hero.mp4 docs/demos/hero.tape docs/demos/manifest.json
check "hero --repin moves only hero's manifest values" manifest_is hero
check "hero --repin verifies chapters" grep -q 'hero chapters' "$C/out"
check "hero --repin removes the marker-owned hero scratch" cleaned

# --- failures: nonzero, unselected bytes kept, selected pin not advanced ---

fail_case() { # fail_case <label> <recording> <VAR=value...>
	local label="$1" sel="$2"
	shift 2
	new_case "fail-$sel"
	snapshot "$C/repo" "$C/before"
	run_with "$@" -- --recording "$sel" --repin
	snapshot "$C/repo" "$C/after"
	if test "$rc" -ne 0; then ok "$label fails"; else bad "$label exits 0"; fi
	check "$label never advances the manifest or tape pins" unchanged_except "^docs/demos/${sel}[.-](gif|webm|mp4|transcript\.txt|poster\.png)$"
	check "$label writes no unselected file" unchanged_except "^docs/demos/${sel}[.-]"
	check "$label leaves $sel pinned historically" test "$(pin_of "$sel" binary_commit)" = "$C1"
	check "$label cleans its tempdir and hero scratch" cleaned
}
fail_case "first tui take failure" tui FAKE_FAIL=take1
check "first tui take failure leaves the tui transcript unaccepted (unchanged)" unchanged_except '^$'
fail_case "second tui take failure" tui FAKE_FAIL=take2
check "second tui take failure leaves the tui transcript unchanged" unchanged_except '^$'
fail_case "render failure" tui FAKE_FAIL=render
check "render failure reports partial selected media nonzero" grep -q 'fixture' "$C/repo/docs/demos/tui.gif"
fail_case "poster failure" tui FAKE_FAIL=poster
fail_case "tape validation failure" tui FAKE_FAIL=validate
fail_case "nondeterministic transcript" tui FAKE_FAIL=drift
fail_case "VHS version mismatch" tui FAKE_VHS_VERSION=v0.99.0
fail_case "hero transcript failure" hero FAKE_FAIL=hero
fail_case "hero chapters beyond the rendered duration" hero FAKE_DURATION=10

# Wrong commit and dirty inputs refuse before building.
new_case wrong-commit
run_body "$C" --recording tui
expect_rc "selected pin != HEAD without --repin" 1
check "wrong commit never builds" bash -c '! grep -q "go build" "$1"' _ "$C/log"

# An unselected historical pin naming an existing non-commit object (a real
# Git blob) fails the selected --repin run and leaves every pin in place.
new_case noncommit-history
blob="$(g "$C/repo" rev-parse HEAD:go.mod)"
set_pins "$blob" "$C1" "$C1"
commit "$C/repo" noncommit-manifest >/dev/null
cp "$C/repo/docs/demos/manifest.json" "$C/manifest.before"
run_body "$C" --recording tui --repin
expect_rc "historical blob pin with tui --repin" 1
check "historical blob pin keeps the manifest bytes" cmp -s "$C/manifest.before" "$C/repo/docs/demos/manifest.json"

dirty_case() { # dirty_case <label> <path>: tui pinned to HEAD, <path> edited
	new_case dirty
	set_pins "$C1" "$C2" "$C1"
	echo "dirt" >>"$C/repo/$2"
	run_body "$C" --recording tui
	expect_rc "$1" 1
	check "$1 never builds" bash -c '! grep -q "go build" "$1"' _ "$C/log"
}
dirty_case "dirty Go source with tui selected" cmd/mythhelm/main.go
dirty_case "dirty shared normalization with tui selected" docs/demos/normalize.sed
dirty_case "dirty selected tape" docs/demos/tui.tape

new_case dirty-repin
echo dirt >"$C/repo/untracked.txt"
run_body "$C" --recording tui --repin
expect_rc "--repin on a dirty tree" 1

# Pin-write failure: reported as incomplete repinning, unselected untouched.
if test "$(id -u)" -eq 0; then
	echo "skip tape pin-write failure (root ignores file modes)"
else
	new_case pin-write
	snapshot "$C/repo" "$C/before"
	chmod a-w "$C/repo/docs/demos/tui.tape"
	run_body "$C" --recording tui --repin
	chmod u+w "$C/repo/docs/demos/tui.tape"
	snapshot "$C/repo" "$C/after"
	expect_rc "tape pin-write failure" 1
	check "pin-write failure says incomplete repinning" grep -q 'repin incomplete' "$C/out"
	check "pin-write failure keeps unselected bytes" unchanged_except '^docs/demos/(tui[.-]|manifest\.json$)'
fi

# --- argument refusals: exit 2 before any tool or build -----------------

for args in "--recording" "--recording tui --recording tui" "--repin --repin" "--bogus" \
	"--recording nope" "--recording=tui" "tui"; do
	new_case args
	snapshot "$C/repo" "$C/before"
	# shellcheck disable=SC2086 # word-split the argument vector on purpose
	run_body "$C" $args
	snapshot "$C/repo" "$C/after"
	expect_rc "record_main $args" 2
	check "record_main $args runs no tool and writes nothing" bash -c 'test ! -s "$1" && cmp -s "$2" "$3"' _ "$C/log" "$C/before" "$C/after"
done

new_case dup-manifest
python3 - "$C/repo/docs/demos/manifest.json" <<'EOF'
import json, sys
m = json.load(open(sys.argv[1]))
m["recordings"].append(dict(m["recordings"][1]))
json.dump(m, open(sys.argv[1], "w"), indent=2)
EOF
commit "$C/repo" >/dev/null
run_body "$C" --recording tui --repin
expect_rc "--recording naming a duplicated manifest entry" 2
check "duplicated entry never builds" bash -c '! grep -q "go build" "$1"' _ "$C/log"

new_case source-only
: >"$C/log"
set +e
(
	export PATH="$C/fake:$PATH" FAKE_LOG="$C/log"
	# shellcheck disable=SC1091
	source "$C/repo/docs/demos/record.sh"
	declare -F record_main >/dev/null
)
rc=$?
set -e
expect_rc "sourcing record.sh defines record_main" 0
check "sourcing record.sh runs no tool" test ! -s "$C/log"

# Literal public entry: refusals stop before the fixed binary or scratch.
# FAKE_GO_REFUSE makes the fake go refuse to write even if a build were
# (wrongly) reached, so the fixed path /tmp/mythhelm-record is never written.
public_entry() { # public_entry <label> <PATH> <args...>
	local label="$1" path="$2"
	shift 2
	: >"$C/log"
	set +e
	env PATH="$path" FAKE_LOG="$C/log" FAKE_GO_REFUSE=1 TMPDIR="$C/tmp" bash "$C/repo/docs/demos/record.sh" "$@" >"$C/out" 2>&1
	rc=$?
	set -e
	expect_rc "public entry: $label" 2
	check "public entry: $label never builds" bash -c '! grep -q "go build" "$1"' _ "$C/log"
}
new_case public
public_entry "unknown option" "$C/fake:$PATH" --bogus
public_entry "missing recording name" "$C/fake:$PATH" --recording
public_entry "duplicate --repin" "$C/fake:$PATH" --repin --repin
public_entry "unknown recording" "$C/fake:$PATH" --recording nope
# The fakes minus vhs, plus only the system tools the script reaches.
mkdir -p "$C/novhs"
for t in go ttyd ffmpeg ffprobe timeout file; do ln -s "$C/fake/$t" "$C/novhs/$t"; done
for t in bash env git python3 sed grep cut cat cp cmp diff mktemp rm dirname head tail ls wc mkdir; do
	ln -s "$(command -v "$t")" "$C/novhs/$t"
done
public_entry "missing vhs" "$C/novhs" --recording tui
check "public entry: missing vhs is the reported reason" grep -q 'need vhs on PATH' "$C/out"

# --- mutants: each must fail an effect assertion ------------------------

mutant() { # mutant <name> <sed-ERE>: a mutated copy of record.sh at $WORK/<name>.sh
	sed -E "$2" "$SCRIPT" >"$WORK/$1.sh"
	if cmp -s "$SCRIPT" "$WORK/$1.sh"; then bad "mutant $1 did not change record.sh"; fi
}

mutant global-repin 's/(python3 - "\$MANIFEST" "\$head" "\$ver") \$names/\1 $all/'
tui_repin_case mut-global-repin "$WORK/global-repin.sh"
if test "$rc" -eq 0 && ! manifest_is tui; then ok "global-repin mutant fails the manifest byte check"; else bad "global-repin mutant not caught (rc=$rc)"; fi

mutant unselected-write 's/^(\t\tnames=)"\$selected"$/\1"$all"/'
tui_repin_case mut-unselected-write "$WORK/unselected-write.sh"
if ! selected_tui_only; then ok "unselected-write mutant fails the unselected-bytes check"; else bad "unselected-write mutant not caught"; fi

# `|| true` swallows the first take's status: the PIPESTATUS check that
# follows then reads true's status, and pipefail no longer applies.
mutant swallow-take1 's/(>"\$tmpdir\/tui\.t1")$/\1 || true/'
tui_repin_case mut-swallow "$WORK/swallow-take1.sh" take1
if ! unchanged_except '^$'; then ok "swallowed first-take mutant writes the failed take's transcript"; else
	bad "swallowed first-take mutant not caught (rc=$rc)"
	sed 's/^/     | /' "$C/out" >&2
fi

mutant no-cleanup '/^\ttrap cleanup EXIT$/d'
tui_repin_case mut-no-cleanup "$WORK/no-cleanup.sh"
if ! cleaned; then ok "missing-cleanup mutant fails the cleanup check"; else bad "missing-cleanup mutant not caught"; fi

echo "record_test.sh: $pass passed, $fail failed"
test "$fail" -eq 0
