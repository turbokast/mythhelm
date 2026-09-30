#!/usr/bin/env bash
# test_lane.sh: lane.sh keys lanes by (main checkout, resource), lets only the token
# holder release or refresh a lane, judges staleness by the holder process (pid and
# start time) and never by age, reclaims a dead holder's lane, and records the
# nearest `claude` ancestor as the holder by default.

# shellcheck source=../../../.claude/hooks/tests/lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/../../../.claude/hooks/tests/lib.sh"
LANE="$REPO_ROOT/scripts/orchestration/lane.sh"

R="$TEST_TMP/repo"
new_repo "$R"
git -C "$R" worktree add -q "$TEST_TMP/wt" -b wt

run() {  # run <dir> <args...>: sets RC and OUT (stdout and stderr)
  local d="$1"
  shift
  OUT="$(cd "$d" && "$LANE" "$@" 2>&1)"
  RC=$?
}
expect() { CHECKS=$((CHECKS + 1)); [[ "$RC" == "$1" ]] || fail "[$2] expected exit $1, got $RC: $OUT"; }
has() { CHECKS=$((CHECKS + 1)); [[ "$OUT" == *"$1"* ]] || fail "[$2] output lacks '$1': $OUT"; }

sleep 600 & HOLDER=$!
sleep 600 & OTHER=$!
trap 'kill $HOLDER $OTHER 2>/dev/null; cleanup_test_tmp' EXIT

run "$R" token
expect 0 "token"
A="$OUT"
check "token shape" bash -c "[[ '$A' =~ ^lane-[0-9a-f]{16}$ ]]"
B="lane-bbbbbbbbbbbbbbbb"

echo "== acquire, hold, refuse =="
run "$R" acquire spec:demo --owner "$A" --pid "$HOLDER" --note "run-spec demo"
expect 0 "acquire a free lane"
has "LANE-ACQUIRED spec:demo" "acquired"
run "$R" acquire spec:demo --owner "$B" --pid "$OTHER"
expect 1 "another owner is refused"
has "LANE-HELD spec:demo by owner $A" "names the holder"
run "$TEST_TMP/wt" status spec:demo
expect 0 "a linked worktree sees the main checkout's lane"
run "$R" acquire spec:demo --owner "$A" --pid "$OTHER"
expect 0 "the owner refreshes its own lane"
run "$R" acquire spec:other --owner "$B" --pid "$OTHER"
expect 0 "a different resource is independent"

echo "== only the owner releases =="
run "$R" release spec:demo --owner "$B"
expect 1 "a non-owner release is refused"
has "LANE-NOT-OWNER" "says why"
run "$R" status spec:demo
expect 0 "the lane survives a non-owner release"
run "$R" release spec:demo --owner "$A"
expect 0 "the owner releases"
run "$R" status spec:demo
expect 1 "released lane is free"
run "$R" release spec:demo --owner "$A"
expect 0 "releasing a free lane is a no-op"

echo "== staleness is a dead holder, never age =="
run "$R" acquire index --owner "$A" --pid "$HOLDER"
f="$(ls "$R"/.claude/data/lanes/index-*.json)"
jq '.acquired_at = "1990-01-01T00:00:00Z"' "$f" > "$f.tmp" && mv "$f.tmp" "$f"
run "$R" acquire index --owner "$B" --pid "$OTHER"
expect 1 "an old lane with a live holder is still held"
jq '.pid_start = "Mon Jan  1 00:00:00 1990"' "$f" > "$f.tmp" && mv "$f.tmp" "$f"
run "$R" status index
expect 1 "a reused pid (start time differs) is stale"
has "stale lock" "reported stale"
run "$R" acquire index --owner "$B" --pid "$OTHER"
expect 0 "a stale lane is taken over"
has "LANE-STALE index" "takeover is announced"
kill "$OTHER" 2>/dev/null; wait "$OTHER" 2>/dev/null
run "$R" list
has "stale index" "a dead holder lists as stale"
run "$R" acquire index --owner "$A" --pid "$HOLDER"
expect 0 "a dead holder's lane is reclaimed at once"
run "$R" acquire spec:other --owner "$A" --pid "$HOLDER"
expect 0 "spec:other (dead holder) reclaimed"
run "$R" release spec:other --owner "$A"
run "$R" acquire gone --owner "$B" --pid "$HOLDER"
jq '.pid_start = "x"' "$(ls "$R"/.claude/data/lanes/gone-*.json)" > "$TEST_TMP/g" && mv "$TEST_TMP/g" "$(ls "$R"/.claude/data/lanes/gone-*.json)"
run "$R" reap
has "LANE-REAPED gone" "reap removes dead-holder lanes"
run "$R" status index
expect 0 "reap leaves live lanes"

echo "== the default holder is the nearest claude ancestor =="
cp "$(command -v bash)" "$TEST_TMP/claude"
# shellcheck disable=SC2016  # the inner script expands in the stand-in claude shell
OUT="$(cd "$R" && "$TEST_TMP/claude" -c 'echo "pid=$$"; "$0" acquire run:delivery --owner "$1"; true' "$LANE" "$A" 2>&1)"
want="$(printf '%s\n' "$OUT" | sed -n 's/^pid=//p')"
got="$(jq -r '.pid' "$(ls "$R"/.claude/data/lanes/run_delivery-*.json)")"
check "records the claude ancestor ($want), got $got" test "$want" = "$got"
run "$R" status run:delivery
expect 1 "the ancestor exited, so the lane is stale"

echo "== bad input =="
run "$R" acquire 'Bad Name' --owner "$A"
expect 2 "resource name"
run "$R" acquire index --owner short
expect 2 "owner token"
run "$R" acquire index --owner "$A" --pid 999999999
expect 2 "a holder pid that is not running"

finish
