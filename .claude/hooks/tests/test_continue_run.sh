#!/usr/bin/env bash
# test_continue_run.sh: continue-run.sh continues a stop only for the session a live
# grant binds, while the delivery run has actionable work; it honours the
# AWAITING MAINTAINER final line (from the payload or the transcript), and stops
# continuing when the grant's budget is spent, after three chained continues, and
# inside the minimum interval. Every continue is audited.

# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
HOOK="$HOOKS_DIR/continue-run.sh"
DV="$REPO_ROOT/scripts/orchestration/delivery.py"
PROJ="$TEST_TMP/proj"
new_repo "$PROJ"
export CLAUDE_PROJECT_DIR="$PROJ"
DATA="$PROJ/.claude/data"
mkdir -p "$DATA"
export MYTHHELM_CONTINUE_MIN_INTERVAL=0
TRANSCRIPT="$TEST_TMP/t.jsonl"
: > "$TRANSCRIPT"

stop() {  # stop <session> <stop_hook_active> [last message]
  jq -nc --arg s "$1" --arg w "$PROJ" --argjson a "$2" --arg t "$TRANSCRIPT" --arg m "${3-Done with the task.}" \
    '{hook_event_name:"Stop",session_id:$s,cwd:$w,stop_hook_active:$a,transcript_path:$t}
     + (if $m == "-" then {} else {last_assistant_message:$m} end)'
}
grant() {  # grant <id> <session> <until offset> [max continues]
  local now
  now="$(date +%s)"
  jq -n --arg g "$1" --arg s "$2" --argjson i $((now - 60)) --argjson u $((now + $3)) --argjson m "${4:-5}" \
    '{schema_version:1,id:$g,session_id:$s,issued_epoch:$i,until_epoch:$u,until:"2099-01-01T00:00:00Z",max_continues:$m}' \
    > "$DATA/autonomy-grant.json"
}
dv() { (cd "$PROJ" && python3 "$DV" "$@" >/dev/null 2>&1); }
continues() { jq -s '[.[] | select(.event == "continue")] | length' "$DATA/autonomy-audit.jsonl" 2>/dev/null || echo 0; }

dv init --scope MH-1,MH-2
dv item MH-1 --stage run-spec
dv item MH-2 --stage approval

echo "== no grant, no continue =="
expect_rc 0 "no grant" "$HOOK" "$(stop s1 false)"
grant g1 s1 3600 5
expect_rc 0 "another session's grant" "$HOOK" "$(stop s2 false)"

echo "== a live grant with actionable work continues =="
expect_rc 2 "continue" "$HOOK" "$(stop s1 false)"
expect_err "CONTINUE (1/5)" "the directive counts"
expect_err "MH-1 run-spec" "the directive names the work"
check "the continue is audited" test "$(continues)" = 1

echo "== the named exit =="
expect_rc 0 "final line AWAITING MAINTAINER" "$HOOK" "$(stop s1 false $'Parked MH-1.\n\n**AWAITING MAINTAINER: spec approval for MH-2**\n')"
expect_rc 2 "a mention is not an exit" "$HOOK" "$(stop s1 false $'If blocked I will write AWAITING MAINTAINER: x.\nNext: run-spec.')"
jq -nc '{type:"assistant",message:{content:[{type:"text",text:"Nothing left.\nAWAITING MAINTAINER: approvals"}]}}' > "$TRANSCRIPT"
expect_rc 0 "the sentinel from the transcript" "$HOOK" "$(stop s1 false -)"
jq -nc '{type:"assistant",message:{content:[{type:"text",text:"AWAITING MAINTAINER: old"}]}}' > "$TRANSCRIPT"
jq -nc '{type:"assistant",message:{content:[{type:"text",text:"Merged #4; continuing."}]}}' >> "$TRANSCRIPT"
expect_rc 2 "only the newest message counts" "$HOOK" "$(stop s1 false -)"

echo "== nothing actionable, or a paused run =="
dv item MH-1 --stage approval
expect_rc 0 "everything waits on the maintainer" "$HOOK" "$(stop s1 false)"
dv item MH-1 --stage run-spec
dv run --status paused
expect_rc 0 "a paused run" "$HOOK" "$(stop s1 false)"
dv run --status active

echo "== chained continues stop at three =="
rm -f "$DATA/autonomy-continue.json" "$DATA/autonomy-audit.jsonl"
grant g2 s1 3600 50
expect_rc 2 "unchained" "$HOOK" "$(stop s1 false)"
expect_rc 2 "chain 1" "$HOOK" "$(stop s1 true)"
expect_rc 2 "chain 2" "$HOOK" "$(stop s1 true)"
expect_rc 2 "chain 3" "$HOOK" "$(stop s1 true)"
expect_rc 0 "chain 4 stops" "$HOOK" "$(stop s1 true)"
expect_rc 2 "a fresh turn resets the chain" "$HOOK" "$(stop s1 false)"

echo "== the budget is per grant =="
grant g3 s1 3600 2
expect_rc 2 "a new grant starts at zero (1)" "$HOOK" "$(stop s1 false)"
expect_rc 2 "2" "$HOOK" "$(stop s1 false)"
expect_rc 0 "the budget is spent" "$HOOK" "$(stop s1 false)"
check "the spent budget is audited" grep -q '"event":"continue-budget-spent"' "$DATA/autonomy-audit.jsonl"
expect_rc 0 "and stays spent" "$HOOK" "$(stop s1 false)"

echo "== the minimum interval =="
grant g4 s1 3600 50
MYTHHELM_CONTINUE_MIN_INTERVAL=60 expect_rc 2 "first" "$HOOK" "$(stop s1 false)"
MYTHHELM_CONTINUE_MIN_INTERVAL=60 expect_rc 0 "an immediate second stop is allowed" "$HOOK" "$(stop s1 false)"

echo "== expired grants and broken inputs let the session stop =="
grant g5 s1 -30 50
expect_rc 0 "expired" "$HOOK" "$(stop s1 false)"
grant g6 s1 3600 50
expect_rc 0 "garbage payload" "$HOOK" "not json"
printf 'broken' > "$DATA/autonomy-grant.json"
expect_rc 0 "unreadable grant" "$HOOK" "$(stop s1 false)"

finish
