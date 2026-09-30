#!/usr/bin/env bash
# shellcheck disable=SC2016  # commands under test are literal data
# test_guard_autonomy.sh: guard-autonomy.sh blocks agents from granting or renewing
# autonomy, from writing the grant, its audit log or the run's state files other
# than through their scripts, and from installing the heartbeat; in a granted
# session it blocks the arming script and every merge without a fresh recorded merge
# check at the exact head; other sessions merge as before.

# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
HOOK="$HOOKS_DIR/guard-autonomy.sh"
PROJ="$TEST_TMP/proj"
new_repo "$PROJ"
export CLAUDE_PROJECT_DIR="$PROJ"
DATA="$PROJ/.claude/data"
mkdir -p "$DATA"
SID="granted-1"
HEAD="0123456789abcdef0123456789abcdef01234567"

block() { expect_rc 2 "$1" "$HOOK" "$(bash_payload "$2" "$PROJ" "${3:-$SID}")"; }
allow() { expect_rc 0 "$1" "$HOOK" "$(bash_payload "$2" "$PROJ" "${3:-$SID}")"; }
edit_payload() {
  jq -nc --arg t "$1" --arg p "$2" '{hook_event_name:"PreToolUse",tool_name:$t,tool_input:{file_path:$p,content:"x"}}'
}
grant() {  # grant <session> <issued offset> <until offset>
  local now
  now="$(date +%s)"
  jq -n --arg s "$1" --argjson i $((now + $2)) --argjson u $((now + $3)) \
    '{schema_version:1,id:"g1",session_id:$s,issued_epoch:$i,until_epoch:$u,until:"2099-01-01T00:00:00Z",max_continues:5}' \
    > "$DATA/autonomy-grant.json"
}
record() {  # record <pr> <head> <age seconds> [grant id]
  jq -cn --argjson pr "$1" --arg h "$2" --argjson at $(( $(date +%s) - $3 )) --arg g "${4:-g1}" \
    '{event:"merge-ready",grant_id:$g,pr:$pr,head:$h,spec:"demo",at_epoch:$at}' >> "$DATA/autonomy-audit.jsonl"
}

echo "== granting and renewing are the maintainer's =="
block "autonomy.sh grant" 'scripts/orchestration/autonomy.sh grant --hours 8 --scope MH-1 --reason x'
block "autonomy.sh renew" 'scripts/orchestration/autonomy.sh renew --hours 8'
block "python autonomy.py grant" 'python3 scripts/orchestration/autonomy.py grant --hours 1 --scope MH-1 --reason x'
block "bash autonomy.sh" 'bash scripts/orchestration/autonomy.sh renew --hours 2'
block "through bash -c" "bash -c 'scripts/orchestration/autonomy.sh grant --hours 1 --scope MH-1 --reason x'"
block "in a chain" 'true && ./autonomy.sh grant --hours 1 --scope MH-1 --reason x'
block "quote tricks" 'scripts/orchestration/auto"nomy".sh gr"ant" --hours 1'
run_hook "$HOOK" "$(bash_payload 'scripts/orchestration/autonomy.sh grant --hours 1' "$PROJ" "$SID")"
expect_stanza guard-autonomy "the block teaches"
expect_err "! scripts/orchestration/autonomy.sh" "the fix names the maintainer's route"
allow "status" 'scripts/orchestration/autonomy.sh status'
allow "revoke" 'scripts/orchestration/autonomy.sh revoke --reason done'
allow "merge-check" 'python3 scripts/orchestration/autonomy.py merge-check --pr 3 --spec demo --task 1'
allow "a mention" 'echo "ask the maintainer to run autonomy.sh grant"'
allow "a commit message" 'git commit -m "document autonomy.sh grant" -- knowledge/autonomy.md'

echo "== the grant, its audit log and the run files are written by their scripts =="
block "redirect into the grant" 'echo {} > .claude/data/autonomy-grant.json'
block "glued redirect" 'echo {}>.claude/data/autonomy-grant.json'
block "cp over the grant" 'cp /tmp/g.json .claude/data/autonomy-grant.json'
block "rm the audit log" 'rm .claude/data/autonomy-audit.jsonl'
block "append to RUN-LOG" 'echo "- 2026-01-01T00:00:00Z — guessed" >> orchestration/RUN-LOG.md'
block "sed on QUESTIONS" 'sed -e s/open/answered/ orchestration/QUESTIONS.md > /tmp/q && mv /tmp/q orchestration/QUESTIONS.md'
block "tee INTENT" 'printf x | tee orchestration/INTENT.md'
block "python writing RUN-LOG" 'python3 -c "open(1)" orchestration/RUN-LOG.md'
block "a redirect around delivery.py" 'python3 scripts/orchestration/delivery.py show > orchestration/INTENT.md'
allow "cat the grant" 'cat .claude/data/autonomy-grant.json'
allow "jq the audit log" 'jq -c . .claude/data/autonomy-audit.jsonl'
allow "tail RUN-LOG" 'tail -n 20 orchestration/RUN-LOG.md'
allow "delivery.py log" 'python3 scripts/orchestration/delivery.py log "merged #12"'
allow "delivery.py question" 'python3 scripts/orchestration/delivery.py question --title t --context c --default d'
for tool in Write Edit MultiEdit; do
  expect_rc 2 "$tool the grant" "$HOOK" "$(edit_payload "$tool" "$PROJ/.claude/data/autonomy-grant.json")"
  expect_rc 2 "$tool RUN-LOG" "$HOOK" "$(edit_payload "$tool" "$PROJ/orchestration/RUN-LOG.md")"
done
expect_rc 2 "Write QUESTIONS" "$HOOK" "$(edit_payload Write "$PROJ/orchestration/QUESTIONS.md")"
expect_rc 2 "Edit INTENT" "$HOOK" "$(edit_payload Edit "$PROJ/orchestration/INTENT.md")"
expect_rc 2 "Write the audit log" "$HOOK" "$(edit_payload Write "$PROJ/.claude/data/autonomy-audit.jsonl")"
expect_rc 0 "Write orchestration/README.md" "$HOOK" "$(edit_payload Write "$PROJ/orchestration/README.md")"
expect_rc 0 "Write a spec's INTENT.md elsewhere" "$HOOK" "$(edit_payload Write "$PROJ/docs/INTENT.md")"

echo "== the heartbeat is the maintainer's to install =="
block "enable the timer" 'systemctl --user enable --now mythhelm-heartbeat@tmp-my\x2drepo.timer'
block "start the service" 'systemctl --user start mythhelm-heartbeat@tmp-my\x2drepo.service'
allow "list timers" 'systemctl --user list-timers mythhelm-heartbeat@tmp-my\x2drepo.timer'
allow "status" 'systemctl --user status mythhelm-heartbeat@tmp-my\x2drepo.timer'

echo "== no grant: merges and arming are untouched =="
rm -f "$DATA/autonomy-grant.json"
allow "merge without a grant" 'gh pr merge 12 --squash'
allow "arming without a grant" 'scripts/harness/arm-main-push.sh --reason "operator asked"'

echo "== a grant for another session changes nothing here =="
grant other-session -60 3600
allow "merge in another session" 'gh pr merge 12 --squash'
allow "arming in another session" 'scripts/harness/arm-main-push.sh --reason x'

echo "== in the granted session =="
grant "$SID" -60 3600
block "arming a main push" 'scripts/harness/arm-main-push.sh --reason "ship it"'
block "arming a publish" 'bash scripts/harness/arm-main-push.sh --publish --reason "release"'
block "merge with no record" "gh pr merge 12 --squash --match-head-commit $HEAD"
record 12 "$HEAD" 30
allow "merge with a fresh record at that head" "gh pr merge 12 --squash --match-head-commit $HEAD --delete-branch"
allow "the repo flag before the subcommand" "gh -R acme/demo pr merge 12 --squash --match-head-commit=$HEAD"
allow "a pull request URL" "gh pr merge https://github.com/acme/demo/pull/12 --squash --match-head-commit $HEAD"
check "an allowed merge is audited" grep -q '"event":"merge-allowed"' "$DATA/autonomy-audit.jsonl"
block "another head" "gh pr merge 12 --squash --match-head-commit ${HEAD//0/f}"
block "another pull request" "gh pr merge 13 --squash --match-head-commit $HEAD"
block "no head pin" 'gh pr merge 12 --squash'
block "no squash" "gh pr merge 12 --match-head-commit $HEAD"
block "--merge" "gh pr merge 12 --merge --match-head-commit $HEAD"
block "--auto" "gh pr merge 12 --squash --auto --match-head-commit $HEAD"
block "--rebase" "gh pr merge 12 --rebase --match-head-commit $HEAD"
block "no number" "gh pr merge --squash --match-head-commit $HEAD"
block "through bash -c" "bash -c 'gh pr merge 13 --squash --match-head-commit $HEAD'"
record 14 "$HEAD" 1000
block "a stale record" "gh pr merge 14 --squash --match-head-commit $HEAD"
record 15 "$HEAD" 10 other-grant
block "a record under another grant" "gh pr merge 15 --squash --match-head-commit $HEAD"
allow "gh pr view" 'gh pr view 12 --json state'
allow "gh pr checks" 'gh pr checks 12'
run_hook "$HOOK" "$(bash_payload 'gh pr merge 13 --squash' "$PROJ" "$SID")"
expect_err "autonomy.py merge-check" "the fix names the merge check"

echo "== an expired grant stops merges until renewed or revoked =="
grant "$SID" -7200 -60
block "merge after expiry" "gh pr merge 12 --squash --match-head-commit $HEAD"
block "arming after expiry" 'scripts/harness/arm-main-push.sh --reason x'

echo "== an invalid grant file blocks merges and arming everywhere until revoked =="
grant "$SID" -60 $((30 * 3600))
block "an over-long grant blocks a merge" 'gh pr merge 12 --squash'
block "in another session too" 'gh pr merge 12 --squash' other-session
block "arming" 'scripts/harness/arm-main-push.sh --reason x' other-session
printf 'not json' > "$DATA/autonomy-grant.json"
block "an unreadable grant blocks a merge" "gh pr merge 12 --squash --match-head-commit $HEAD"
run_hook "$HOOK" "$(bash_payload 'gh pr merge 12 --squash' "$PROJ" other-session)"
expect_err "autonomy.sh revoke" "the fix is the maintainer's revoke"
allow "revoking is still allowed" 'scripts/orchestration/autonomy.sh revoke'
allow "other gh commands pass" 'gh pr view 12'
rm -f "$DATA/autonomy-grant.json"
allow "after the revoke, merges are ordinary again" 'gh pr merge 12 --squash' other-session

echo "== fail closed without jq =="
HOOK_JQ_PROBE=jq-missing-probe expect_rc 2 "no jq, guarded payload" "$HOOK" \
  "$(bash_payload 'scripts/orchestration/autonomy.sh grant --hours 1' "$PROJ" "$SID")"
HOOK_JQ_PROBE=jq-missing-probe expect_rc 0 "no jq, unrelated payload" "$HOOK" "$(bash_payload 'ls -la' "$PROJ" "$SID")"

finish
