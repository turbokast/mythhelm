#!/usr/bin/env bash
# test_gatelib.sh: gate.sh records a pass only for a gate that exited 0 over an
# unchanged tree, clears the marker on failure, and gatelib.py's fingerprint,
# change-set gate selection and freshness report agree with the markers. Stand-in
# go, gofmt and shellcheck binaries on PATH play the tools.

# shellcheck source=../../../.claude/hooks/tests/lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/../../../.claude/hooks/tests/lib.sh"
GATE="$REPO_ROOT/scripts/harness/gate.sh"
LIB="$REPO_ROOT/scripts/harness/gatelib.py"

BIN="$TEST_TMP/bin"
mkdir -p "$BIN"
# The stand-ins: FAKE_RC sets the exit status, FAKE_GOFMT_OUT what gofmt -l prints,
# FAKE_TOUCH a file the tool appends to while it "runs".
for tool in go gofmt shellcheck; do
  cat > "$BIN/$tool" <<'EOF'
#!/usr/bin/env bash
[[ -z "${FAKE_TOUCH:-}" ]] || echo "// touched" >> "$FAKE_TOUCH"
[[ "$(basename "$0")" != gofmt || -z "${FAKE_GOFMT_OUT:-}" ]] || echo "$FAKE_GOFMT_OUT"
echo "$(basename "$0") $*" >&2
exit "${FAKE_RC:-0}"
EOF
  chmod +x "$BIN/$tool"
done
export PATH="$BIN:$PATH"

# repo <name>: a committed fixture tree with Go, a spec and the hygiene script.
repo() {
  R="$TEST_TMP/$1"
  new_repo "$R"
  mkdir -p "$R/internal/x" "$R/specs/in-progress/demo" "$R/scripts/ci" "$R/scripts/harness"
  printf '.claude/data/*\n' > "$R/.gitignore"
  printf 'package x\n' > "$R/internal/x/x.go"
  printf 'module example.com/x\n' > "$R/go.mod"
  printf '# Tasks\n' > "$R/specs/in-progress/demo/tasks.md"
  cp "$REPO_ROOT/scripts/ci/check-public-hygiene.sh" "$R/scripts/ci/"
  printf '#!/usr/bin/env bash\necho lint ok\n' > "$R/scripts/ci/lint-agent-harness.sh"
  chmod +x "$R/scripts/ci/lint-agent-harness.sh"
  git -C "$R" add -A
  git -C "$R" commit -q -m fixture
}

# gate <args...>: runs gate.sh inside $R; sets RC and OUT (stdout and stderr).
gate() {
  OUT="$(cd "$R" && "$GATE" "$@" 2>&1)"
  RC=$?
}

lib() {   # lib <args...>: gatelib.py inside $R; sets RC and OUT
  OUT="$(cd "$R" && python3 "$LIB" "$@" 2>&1)"
  RC=$?
}

expect() {   # expect <rc> <description>
  CHECKS=$((CHECKS + 1))
  [[ "$RC" == "$1" ]] || fail "[$2] expected exit $1, got $RC. output: ${OUT:0:400}"
}

has() {   # has <needle> <description>
  CHECKS=$((CHECKS + 1))
  [[ "$OUT" == *"$1"* ]] || fail "[$2] output lacks '$1': ${OUT:0:400}"
}

marker() { jq -r ".$2" "$R/.claude/data/gate-marker-$1.json" 2>/dev/null; }

echo "== fingerprint =="
repo fp
lib fingerprint --scope go; go1="$OUT"
lib fingerprint --scope all; all1="$OUT"
CHECKS=$((CHECKS + 1))
[[ "$go1" =~ ^[0-9a-f]{64}$ ]] || fail "[a fingerprint is 64 hex characters] $go1"
echo "notes" > "$R/specs/in-progress/demo/scratchpad.md"
lib fingerprint --scope go
check "a spec edit leaves the go scope unchanged" [ "$OUT" == "$go1" ]
lib fingerprint --scope all
check "an untracked file changes the all scope" [ "$OUT" != "$all1" ]
all2="$OUT"
mkdir -p "$R/.claude/data" && echo x > "$R/.claude/data/noise.json"
lib fingerprint --scope all
check "an ignored file never enters a fingerprint" [ "$OUT" == "$all2" ]
chmod +x "$R/internal/x/x.go"
lib fingerprint --scope go
check "the executable bit changes the fingerprint" [ "$OUT" != "$go1" ]
chmod -x "$R/internal/x/x.go"
rm "$R/internal/x/x.go"
lib fingerprint --scope go
check "deleting a tracked file changes the fingerprint" [ "$OUT" != "$go1" ]

echo "== required gates follow the change set =="
repo req
lib required; expect 0 "required on a clean tree"
check "a clean tree needs no gate" [ -z "$OUT" ]
echo "x" > "$R/specs/in-progress/demo/scratchpad.md"
lib required
check "a spec-only change needs hygiene only" [ "$OUT" == "hygiene" ]
echo "// y" >> "$R/internal/x/x.go"
lib required
check "a Go change needs the Go gates" [ "$OUT" == "go-fmt go-vet go-test go-mod-tidy hygiene" ]
echo "version: 2" > "$R/.golangci.yml"
echo "go 1.27" >> "$R/go.mod"
lib required
check "a lint config adds golangci-lint; go.mod adds govulncheck" [ "$OUT" == "go-fmt go-vet go-test go-mod-tidy golangci-lint govulncheck hygiene" ]
repo req-sh
printf '#!/usr/bin/env bash\n' > "$R/scripts/harness/new.sh"
lib required
check "a new harness script needs the harness gates and shellcheck" [ "$OUT" == "harness-lint harness-tests shellcheck hygiene" ]

echo "== a pass records a fresh marker =="
repo pass
echo "// y" >> "$R/internal/x/x.go"
gate go-vet
expect 0 "a passing gate exits 0"
has "gate go-vet: PASS (exit 0" "it says it passed and recorded"
lib fingerprint --scope go
check "the marker's fingerprint is the scope's" [ "$(marker go-vet fingerprint)" == "$OUT" ]
check "the marker names its tree" [ "$(marker go-vet tree)" == "$(cd "$R" && pwd -P)" ]
check "the marker records exit 0" [ "$(marker go-vet exit_code)" == 0 ]
gate go
expect 0 "the go group runs the Go gates the change set needs"
has "gate: 4 passed and recorded: go-fmt go-vet go-test go-mod-tidy" "the group summary"
gate hygiene
expect 0 "hygiene passes on a clean fixture"
lib status
expect 0 "every required gate is fresh"
has "go-test        fresh" "status names each gate"
echo "// z" >> "$R/internal/x/x.go"
lib status
expect 1 "an edit after the gates makes status fail"
has "go-test        stale" "the Go markers are stale"
has "hygiene        stale" "and so is hygiene"

echo "== failures and suspicious passes record nothing =="
repo failing
echo "// y" >> "$R/internal/x/x.go"
gate go-test; expect 0 "baseline pass"
check "baseline marker exists" [ -f "$R/.claude/data/gate-marker-go-test.json" ]
FAKE_RC=1 gate go-test
expect 1 "a failing gate exits 1"
has "gate go-test: FAIL (exit 1" "it says it failed"
check "a failing run deletes the marker" [ ! -e "$R/.claude/data/gate-marker-go-test.json" ]
FAKE_GOFMT_OUT="internal/x/x.go" gate go-fmt
expect 1 "gofmt listing a file is a failure"
check "no go-fmt marker" [ ! -e "$R/.claude/data/gate-marker-go-fmt.json" ]
FAKE_TOUCH="$R/internal/x/x.go" gate go-vet
expect 1 "a tree that changed during the gate is not recorded"
has "NOT recorded" "it says so"
check "no marker for the changed tree" [ ! -e "$R/.claude/data/gate-marker-go-vet.json" ]
FAKE_RC=1 gate go-fmt go-vet
expect 1 "a group stops at the first failure"
CHECKS=$((CHECKS + 1))
[[ "$OUT" != *"gate go-vet:"* ]] || fail "[gates after the failure did not run] $OUT"

echo "== tools, groups, directories =="
repo tools
echo "// y" >> "$R/internal/x/x.go"
# A PATH holding only what the wrapper itself needs, so no real go can be found.
MIN="$TEST_TMP/minbin"
mkdir -p "$MIN"
for t in bash git dirname env; do ln -s "$(command -v "$t")" "$MIN/$t"; done
ln -s "$(python3 -c 'import sys; print(sys.executable)')" "$MIN/python3"   # the interpreter, not a version-manager shim
OUT="$(cd "$R" && PATH="$MIN" "$GATE" go-vet 2>&1)"; RC=$?
expect 127 "a missing tool is exit 127"
has "not found on PATH" "and says which"
check "a missing tool records nothing" [ ! -e "$R/.claude/data/gate-marker-go-vet.json" ]
OUT="$(cd "$R/internal/x" && "$GATE" go-vet 2>&1)"; RC=$?
expect 0 "a gate run from a subdirectory"
check "its marker lands at the tree root" [ -f "$R/.claude/data/gate-marker-go-vet.json" ]
gate no-such-gate
expect 2 "an unknown gate is a usage error"
repo nogo
gate go
expect 0 "a group with nothing to do"
has "needs none" "says the change set needs none"

echo "== a marker attests only its own tree =="
repo tree-a
echo "// y" >> "$R/internal/x/x.go"
gate go-vet; expect 0 "tree a passes"
A="$R"
repo tree-b
echo "// y" >> "$R/internal/x/x.go"
mkdir -p "$R/.claude/data"
cp "$A/.claude/data/gate-marker-go-vet.json" "$R/.claude/data/"
lib status
has "go-vet         stale" "a marker copied from another tree with identical content is stale"

finish
