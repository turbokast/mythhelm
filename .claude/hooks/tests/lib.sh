# shellcheck shell=bash
# shellcheck disable=SC2034  # globals are read by the sourcing tests
# lib.sh: helpers shared by the harness tests. Sourced by every test_*.sh; the
# runner never executes it on its own.
#
# Every test runs against throwaway fixtures: the working directory, any
# CLAUDE_PROJECT_DIR and the git configuration point into a temporary directory,
# so no test reads or writes the real repository's .claude/data or the caller's
# git identity. Commands under test
# live in the test files as data and reach the hooks only as JSON on stdin.

set -uo pipefail

LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$LIB_DIR/../../.." && pwd)"
HOOKS_DIR="$REPO_ROOT/.claude/hooks"

TEST_TMP="$(mktemp -d)"
cleanup_test_tmp() { rm -rf "$TEST_TMP"; }
trap cleanup_test_tmp EXIT

export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.com
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.com
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE CLAUDE_PROJECT_DIR
cd "$TEST_TMP" || exit 1

CHECKS=0
FAILS=0

# new_repo <dir>: an initialised repository on main with one commit.
new_repo() {
  git init -q -b main "$1"
  git -C "$1" commit -q --allow-empty -m init
}

# bash_payload <command> [cwd] [session_id]: a PreToolUse Bash payload.
bash_payload() {
  jq -nc --arg c "$1" --arg w "${2:-}" --arg s "${3:-}" \
    '{hook_event_name:"PreToolUse",tool_name:"Bash",tool_input:{command:$c}}
     + (if $w == "" then {} else {cwd:$w} end)
     + (if $s == "" then {} else {session_id:$s} end)'
}

# run_hook <hook-path> <payload>: sets RC, OUT and ERR.
run_hook() {
  local errf="$TEST_TMP/.stderr"
  OUT="$(printf '%s' "$2" | "$1" 2>"$errf")"
  RC=$?
  ERR="$(cat "$errf")"
}

fail() {
  FAILS=$((FAILS + 1))
  echo "FAIL: $*" >&2
}

# expect_rc <want> <description> <hook-path> <payload>
expect_rc() {
  local want="$1" desc="$2"
  CHECKS=$((CHECKS + 1))
  run_hook "$3" "$4"
  [[ "$RC" == "$want" ]] || fail "[$desc] expected exit $want, got $RC. stderr: ${ERR:0:300}"
}

# expect_err <needle> <description>: the last run's stderr contains <needle>.
expect_err() {
  CHECKS=$((CHECKS + 1))
  [[ "$ERR" == *"$1"* ]] || fail "[$2] stderr lacks '$1'. stderr: ${ERR:0:300}"
}

# expect_stanza <guard> <description>: the last run's stderr is a block stanza.
expect_stanza() {
  CHECKS=$((CHECKS + 1))
  local re="^BLOCK: $1"$'\n'"Command: .+"$'\n'"Detail: .+"$'\n'"Fix: .+"
  [[ "$ERR" =~ $re ]] || fail "[$2] stderr is not the BLOCK/Command/Detail/Fix stanza: ${ERR:0:300}"
}

# check <description> <condition...>: a free-form assertion.
check() {
  local desc="$1"
  shift
  CHECKS=$((CHECKS + 1))
  "$@" || fail "[$desc] condition failed: $*"
}

# finish: prints the tally the runner aggregates, exits 1 on any failure.
finish() {
  echo "checks=$CHECKS failed=$FAILS"
  (( FAILS == 0 ))
}
