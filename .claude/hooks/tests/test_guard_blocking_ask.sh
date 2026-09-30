#!/usr/bin/env bash
# test_guard_blocking_ask.sh: guard-blocking-ask.sh blocks AskUserQuestion in a
# session a live autonomy grant binds, and in a session started unattended; it allows
# the question everywhere else (no grant, another session's grant, an expired or
# forged grant) and whenever it cannot read its inputs.

# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
HOOK="$HOOKS_DIR/guard-blocking-ask.sh"
PROJ="$TEST_TMP/proj"
new_repo "$PROJ"
export CLAUDE_PROJECT_DIR="$PROJ"
DATA="$PROJ/.claude/data"
mkdir -p "$DATA"
unset MYTHHELM_NONINTERACTIVE

ask() {  # ask <session>
  jq -nc --arg s "$1" --arg w "$PROJ" '{hook_event_name:"PreToolUse",tool_name:"AskUserQuestion",session_id:$s,cwd:$w,
    tool_input:{questions:[{header:"Format",question:"Which format?",options:[{label:"A"},{label:"B"}]}]}}'
}
grant() {  # grant <session> <issued offset> <until offset>
  local now
  now="$(date +%s)"
  jq -n --arg s "$1" --argjson i $((now + $2)) --argjson u $((now + $3)) \
    '{schema_version:1,id:"g1",session_id:$s,issued_epoch:$i,until_epoch:$u,until:"2099-01-01T00:00:00Z",max_continues:5}' \
    > "$DATA/autonomy-grant.json"
}

echo "== no grant: questions pass =="
expect_rc 0 "no grant" "$HOOK" "$(ask s1)"

echo "== a live grant for this session blocks =="
grant s1 -60 3600
expect_rc 2 "granted session" "$HOOK" "$(ask s1)"
expect_err "delivery.py question" "the fix files the question"
expect_err "AWAITING MAINTAINER" "the fix names the exit"
expect_err "Format" "the block names the question"
expect_rc 0 "another session" "$HOOK" "$(ask s2)"

echo "== expired or forged grants bind nothing =="
grant s1 -7200 -60
expect_rc 0 "expired" "$HOOK" "$(ask s1)"
grant s1 -60 $((48 * 3600))
expect_rc 0 "over-long" "$HOOK" "$(ask s1)"
printf 'not json' > "$DATA/autonomy-grant.json"
expect_rc 0 "unreadable grant" "$HOOK" "$(ask s1)"

echo "== an unattended session blocks, grant or not =="
rm -f "$DATA/autonomy-grant.json"
MYTHHELM_NONINTERACTIVE=1 expect_rc 2 "unattended" "$HOOK" "$(ask s9)"
expect_err "unattended" "says why"

echo "== other tools and bad payloads pass =="
grant s1 -60 3600
expect_rc 0 "Bash payload" "$HOOK" "$(bash_payload 'ls' "$PROJ" s1)"
expect_rc 0 "garbage" "$HOOK" "not json"

finish
