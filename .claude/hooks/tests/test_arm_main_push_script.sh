#!/usr/bin/env bash
# test_arm_main_push_script.sh: scripts/harness/arm-main-push.sh validates its
# arguments as the guards do, never writes a sentinel itself, and reports the
# windows a guard armed.

# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
SCRIPT="$REPO_ROOT/scripts/harness/arm-main-push.sh"

PROJ="$TEST_TMP/proj"
new_repo "$PROJ"
export CLAUDE_PROJECT_DIR="$PROJ"
DATA="$(cd "$PROJ" && pwd -P)/.claude/data"

# run_script <args...>: sets RC and OUT (stdout and stderr together).
run_script() { OUT="$("$SCRIPT" "$@" 2>&1)"; RC=$?; }
expect_script() {   # expect_script <want-rc> <description> <args...>
  local want="$1" desc="$2"
  shift 2
  CHECKS=$((CHECKS + 1))
  run_script "$@"
  [[ "$RC" == "$want" ]] || fail "[$desc] expected exit $want, got $RC: $OUT"
}

echo "== usage errors =="
expect_script 1 "no arguments"
expect_script 1 "reason without a value" --reason
expect_script 1 "blank reason" --reason "  "
expect_script 1 "flag as the reason" --reason --publish
expect_script 1 "unknown argument" --reason x --force
expect_script 1 "disarm with a reason" --disarm --reason x
expect_script 1 "help" --help

echo "== valid forms =="
expect_script 0 "arm" --reason "hot fix"
check "names the note when nothing was witnessed" grep -q "no guard witnessed" <<< "$OUT"
check "wrote no sentinel" [ ! -d "$DATA" ]
expect_script 0 "operator arm" --operator "operator request"
check "operator noted" grep -q "operator override" <<< "$OUT"
expect_script 0 "publish arm" --publish --reason "v1.0.0"
check "publish label" grep -q "publishing window" <<< "$OUT"
expect_script 0 "disarm" --disarm
expect_script 0 "publish disarm" --publish --disarm

echo "== reports a window a guard armed =="
SID="arm-script-$$"
bash_payload "scripts/harness/arm-main-push.sh --reason 'witnessed'" "$PROJ" "$SID" \
  | "$HOOKS_DIR/guard-main-push.sh" 2>/dev/null
check "the guard armed" [ -f "$DATA/main-push-arm-$SID.json" ]
expect_script 0 "arm after the witness" --reason witnessed
check "lists the window" grep -q "reason: witnessed" <<< "$OUT"
CHECKS=$((CHECKS + 1))
[[ "$OUT" != *"no guard witnessed"* ]] || fail "[no outside-harness note] $OUT"

finish
