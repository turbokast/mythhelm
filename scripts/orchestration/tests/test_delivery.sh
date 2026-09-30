#!/usr/bin/env bash
# test_delivery.sh: delivery.py keeps one run per main checkout, stamps every RUN-LOG
# line with the clock and only appends, numbers questions in order, leaves answers to
# a terminal, and reports as actionable only work an agent can take now (building
# waits for dependencies, specifying does not).

# shellcheck source=../../../.claude/hooks/tests/lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/../../../.claude/hooks/tests/lib.sh"
DV="$REPO_ROOT/scripts/orchestration/delivery.py"

R="$TEST_TMP/repo"
new_repo "$R"
git -C "$R" worktree add -q "$TEST_TMP/wt" -b wt
O="$R/orchestration"

run() {  # run <dir> <args...>
  local d="$1"
  shift
  OUT="$(cd "$d" && python3 "$DV" "$@" 2>&1 </dev/null)"
  RC=$?
}
expect() { CHECKS=$((CHECKS + 1)); [[ "$RC" == "$1" ]] || fail "[$2] expected exit $1, got $RC: $OUT"; }
has() { CHECKS=$((CHECKS + 1)); [[ "$OUT" == *"$1"* ]] || fail "[$2] output lacks '$1': $OUT"; }

echo "== one run per main checkout =="
run "$TEST_TMP/wt" init --scope MH-1,spec:beta,MH-3
expect 0 "init from a linked worktree"
check "state lives in the main checkout" test -f "$O/INTENT.md"
check "not in the worktree" test ! -e "$TEST_TMP/wt/orchestration/INTENT.md"
run "$R" init --scope MH-9
expect 1 "a second init while a run is active"
has "already active" "says why"
run "$R" init --scope 'MH-1;x'
expect 1 "bad scope"

echo "== RUN-LOG: clock-stamped, append-only =="
run "$R" item MH-1 --spec alpha --stage run-spec --wave 1
run "$R" item MH-3 --stage run-spec --spec gamma --depends MH-1
run "$R" log "note with
a newline"
check "every entry is one line stamped with the clock" bash -c \
  "grep '^- ' '$O/RUN-LOG.md' | grep -vcE '^- [0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z — ' | grep -qx 0"
now="$(date -u +%Y-%m-%dT%H:%M)"
check "stamps are the current time" grep -q "^- $now" "$O/RUN-LOG.md"
check "the newline was flattened" grep -q "note with a newline" "$O/RUN-LOG.md"
before="$(cat "$O/RUN-LOG.md")"
run "$R" item MH-1 --note "started"
after="$(cat "$O/RUN-LOG.md")"
check "the log only grows" test "${after:0:${#before}}" = "$before"
run "$R" item MH-1 --note "started"
has "unchanged" "an unchanged item logs nothing"
run "$R" item MH-1 --stage shipping
expect 1 "an unknown stage"

echo "== actionable: dependency-safe =="
run "$R" actionable
expect 0 "there is work"
has "MH-1 run-spec" "a build with no dependency is actionable"
has "spec:beta refine-spec" "specifying is actionable"
check "MH-3 waits for MH-1" bash -c "! grep -q 'MH-3' <<< '$OUT'"
run "$R" item MH-1 --stage "done"
run "$R" actionable
has "MH-3 run-spec" "MH-3 is actionable once MH-1 is done"
run "$R" item MH-3 --stage approval
run "$R" item spec:beta --stage parked --note Q-1
run "$R" actionable
expect 1 "approval, parked and done are not actionable"
run "$R" item spec:beta --stage spec
run "$R" run --status paused
run "$R" actionable
expect 1 "a paused run has no actionable work"
run "$R" run --status active

echo "== questions =="
run "$R" question --title "Pick a format" --context "two readings" --default "the first" --items spec:beta
expect 0 "file a question"
check "prints its id" test "$OUT" = "Q-1"
run "$R" question --title "Second" --context "c" --default "d"
check "ids increase" test "$OUT" = "Q-2"
check "opened is stamped" grep -qE '^- \*\*Opened\*\*: [0-9]{4}-.*Z$' "$O/QUESTIONS.md"
run "$R" answer Q-1 --text "the first"
expect 1 "an agent (no terminal) cannot answer"
has "the maintainer's" "says whose it is"
OUT="$(cd "$R" && python3 -c 'import pty, sys; sys.exit(pty.spawn(sys.argv[1:]) >> 8)' python3 "$DV" answer Q-1 --text "the first" 2>&1 </dev/null)"
RC=$?
expect 0 "the maintainer answers from a terminal"
check "status answered" grep -q -- "- \*\*Status\*\*: answered" "$O/QUESTIONS.md"
check "answer recorded" grep -q -- "- \*\*Answer\*\*: the first" "$O/QUESTIONS.md"
OUT="$(cd "$R" && python3 -c 'import pty, sys; sys.exit(pty.spawn(sys.argv[1:]) >> 8)' python3 "$DV" answer Q-1 --text "the second" 2>&1 </dev/null)"
RC=$?
expect 1 "an answered question is not answered again"
has "already answered" "says why"
check "one answer only" test "$(grep -c -- '- \*\*Answer\*\*:' "$O/QUESTIONS.md")" = 1
run "$R" show --json
check "Q-2 still open" bash -c "jq -e '.open_questions == [\"Q-2\"]' <<< '$OUT' >/dev/null"

echo "== a finished run makes way for the next =="
run "$R" run --status complete
run "$R" init --scope MH-7
expect 0 "init after complete"
printf '| broken row |\n' >> "$O/INTENT.md"
run "$R" show
expect 1 "a malformed INTENT row is refused, not guessed"

finish
