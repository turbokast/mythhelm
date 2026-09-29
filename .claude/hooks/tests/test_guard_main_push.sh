#!/usr/bin/env bash
# shellcheck disable=SC2016  # commands under test are literal data
# test_guard_main_push.sh: guard-main-push.sh classifies pushes that can update
# main, blocks them outside an armed window, witnesses arming and disarming in
# command order, keys windows by session, expires them, and audits every decision.

# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
HOOK="$HOOKS_DIR/guard-main-push.sh"

PROJ="$TEST_TMP/proj"
new_repo "$PROJ"
export CLAUDE_PROJECT_DIR="$PROJ"
DATA="$(cd "$PROJ" && pwd -P)/.claude/data"
ARM='scripts/harness/arm-main-push.sh'

n=0
fresh_sid() { n=$((n + 1)); SID="s$n-$$"; }
fresh_sid

block() { expect_rc 2 "$1" "$HOOK" "$(bash_payload "$2" "$PROJ" "$SID")"; }
allow() { expect_rc 0 "$1" "$HOOK" "$(bash_payload "$2" "$PROJ" "$SID")"; }

echo "== unarmed: pushes that can update main block =="
block "origin main" 'git push origin main'
block "refs/heads/main" 'git push origin refs/heads/main'
block "src:main" 'git push origin topic:main'
block "src:refs/heads/main" 'git push origin HEAD:refs/heads/main'
block "HEAD" 'git push origin HEAD'
block "@" 'git push origin @'
block "bare push" 'git push'
block "bare push with -u" 'git push -u'
block "remote only" 'git push origin'
block "--follow-tags with no refspec" 'git push --follow-tags'
block "--all" 'git push --all origin'
block "--branches" 'git push --branches origin'
block "glob refspec" 'git push origin "refs/heads/*:refs/heads/*"'
block "variable refspec" 'git push origin "$BRANCH"'
block "substitution refspec" 'git push origin $(git branch --show-current)'
block "--repo with main" 'git push --repo=origin main'
block "one of several refspecs" 'git push origin topic main'
block "-C and options" 'git -C . -c push.default=current push --no-verify origin main'

echo "== unarmed: bypass spellings =="
block "bash -c" "bash -c 'git push origin main'"
block "eval" 'eval "git push origin main"'
block "env" 'env GIT_SSH_COMMAND=ssh git push origin main'
block "command" 'command git push origin main'
block "chain" 'git fetch && git push origin main'
block "semicolon" 'true; git push origin main'
block "pipe" 'yes | git push origin main'
block "subshell" '(cd sub && git push origin main)'
block "quoted ref" "git push origin 'main'"
block "partially quoted ref" 'git push origin ma"i"n'
block "quoted command word" '"git" push origin main'
block "backslash ref" 'git push origin ma\in'
block "substitution" 'echo $(git push origin main)'

echo "== other branches and tags pass =="
allow "feature branch" 'git push -u origin harness/p1-substrate'
allow "HEAD to a branch" 'git push origin HEAD:refs/heads/topic'
allow "delete a branch" 'git push origin --delete topic'
allow "--tags only" 'git push --tags origin'
allow "tag refspec" 'git push origin v1.2.3'
allow "main as a source only" 'git push origin main:backup'
allow "force-with-lease on a branch" 'git push --force-with-lease origin topic'
allow "commit message prose" 'git commit -m "then git push origin main" -- a.go'
allow "heredoc prose" $'cat <<EOF\ngit push origin main\nEOF'
allow "echo" 'echo git push origin main'
allow "grep" 'grep -n "git push origin main" README.md'
allow "fetch main" 'git fetch origin main'
allow "pull main" 'git pull origin main'

echo "== never, armed or not =="
block "delete remote main" 'git push origin :main'
block "--delete main" 'git push origin --delete main'
block "-d main" 'git push -d origin main'
block "force-with-lease main" 'git push --force-with-lease origin main'
block "+main" 'git push origin +main'
block "force bare" 'git push -f'

echo "== arming =="
fresh_sid
allow "arm" "$ARM --reason 'hotfix requested by the operator'"
check "sentinel written for this session" [ -f "$DATA/main-push-arm-$SID.json" ]
check "sentinel names the reason" grep -q 'hotfix requested by the operator' "$DATA/main-push-arm-$SID.json"
allow "armed push" 'git push origin main'
allow "armed bare push" 'git push'
block "armed, still never force" 'git push --force-with-lease origin main'
block "armed, still never delete" 'git push origin :main'
OWN="$SID"
fresh_sid
block "another session is not armed" 'git push origin main'
SID="$OWN"
allow "disarm" "$ARM --disarm"
check "sentinel removed" [ ! -f "$DATA/main-push-arm-$SID.json" ]
block "disarmed" 'git push origin main'

echo "== arming and pushing in one command, in order =="
fresh_sid
allow "arm && push" "$ARM --reason 'release branch' && git push origin main"
fresh_sid
block "push before arm" "git push origin main; $ARM --reason late"
fresh_sid
allow "operator arm via bash" "bash $ARM --operator 'operator said so' && git push origin main"
check "operator flag recorded" grep -q '"operator": true' "$DATA/main-push-arm-$SID.json"
fresh_sid
allow "arm, push, disarm" "$ARM --reason x && git push origin main && $ARM --disarm"
block "window closed after the disarm" 'git push origin main'

echo "== invalid arming never arms =="
for bad in "$ARM" "$ARM --reason" "$ARM --reason ''" "$ARM --reason --disarm" "$ARM --reason x --bogus" \
           "echo $ARM --reason x" "$ARM --publish --reason x" "cat $ARM"; do
  fresh_sid
  expect_rc 0 "not an arm: $bad" "$HOOK" "$(bash_payload "$bad" "$PROJ" "$SID")"
  check "no sentinel after: $bad" [ ! -f "$DATA/main-push-arm-$SID.json" ]
done
fresh_sid
allow "arm text in a heredoc" $'cat <<EOF\n'"$ARM"$' --reason x\nEOF'
check "heredoc arm wrote nothing" [ ! -f "$DATA/main-push-arm-$SID.json" ]

echo "== expiry =="
fresh_sid
mkdir -p "$DATA"
jq -n --arg s "$SID" --argjson ep "$(( $(date +%s) - 1801 ))" \
  '{schema_version:1,session_id:$s,kind:"main-push",armed_at_epoch:$ep,ttl_seconds:1800,reason:"old",operator:false}' \
  > "$DATA/main-push-arm-$SID.json"
block "expired window" 'git push origin main'
expect_err "expired" "expiry is named"
check "expired sentinel removed" [ ! -f "$DATA/main-push-arm-$SID.json" ]
fresh_sid
jq -n --arg s "$SID" --argjson ep "$(( $(date +%s) + 600 ))" \
  '{schema_version:1,session_id:$s,kind:"main-push",armed_at_epoch:$ep,ttl_seconds:1800,reason:"future",operator:false}' \
  > "$DATA/main-push-arm-$SID.json"
block "future-dated window" 'git push origin main'
STALE="$DATA/main-push-arm-stale-peer.json"
jq -n --argjson ep 1 '{armed_at_epoch:$ep}' > "$STALE"
fresh_sid
block "sweeps stale peers" 'git push origin main'
check "stale peer sentinel removed" [ ! -f "$STALE" ]

echo "== no session id =="
expect_rc 2 "unattributable push" "$HOOK" "$(bash_payload 'git push origin main' "$PROJ")"
expect_err "no session id" "unattributable is named"
expect_rc 0 "unattributable arm" "$HOOK" "$(bash_payload "$ARM --reason x" "$PROJ")"
check "no sentinel for an empty session id" [ ! -f "$DATA/main-push-arm-.json" ]

echo "== audit =="
AUDIT="$DATA/guard-audit.jsonl"
check "audit exists" [ -f "$AUDIT" ]
check "audit rows are JSON" jq -e . "$AUDIT" >/dev/null
check "arm rows" grep -q '"event":"arm"' "$AUDIT"
check "allow rows" grep -q '"event":"allow"' "$AUDIT"
check "block rows" grep -q '"event":"block"' "$AUDIT"
check "disarm rows" grep -q '"event":"disarm"' "$AUDIT"

echo "== the block teaches =="
fresh_sid
run_hook "$HOOK" "$(bash_payload 'git push origin main' "$PROJ" "$SID")"
expect_stanza guard-main-push "stanza shape"
expect_err "arm-main-push.sh --reason" "fix names the arming command"
run_hook "$HOOK" "$(bash_payload 'git push' "$PROJ" "$SID")"
expect_err "git push origin <branch>" "bare push fix names the branch form"

echo "== fail closed without jq =="
CHECKS=$((CHECKS + 1))
RC=$(bash_payload 'git push origin main' "$PROJ" "$SID" | HOOK_JQ_PROBE=jq-not-installed "$HOOK" 2>/dev/null; echo $?)
[[ "$RC" == 2 ]] || fail "[jq missing blocks a push] got $RC"
CHECKS=$((CHECKS + 1))
RC=$(bash_payload 'git status' "$PROJ" "$SID" | HOOK_JQ_PROBE=jq-not-installed "$HOOK" 2>/dev/null; echo $?)
[[ "$RC" == 0 ]] || fail "[jq missing allows git status] got $RC"

finish
