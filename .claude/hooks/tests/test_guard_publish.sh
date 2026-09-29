#!/usr/bin/env bash
# shellcheck disable=SC2016  # commands under test are literal data
# test_guard_publish.sh: guard-publish.sh blocks publishing and repository-settings
# actions (gh release/workflow/run/secret/variable/ruleset/repo/api, pr merge
# --admin, v* tag pushes) outside an armed publish window, passes read-only gh,
# and never allows gh repo delete.

# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
HOOK="$HOOKS_DIR/guard-publish.sh"

PROJ="$TEST_TMP/proj"
new_repo "$PROJ"
export CLAUDE_PROJECT_DIR="$PROJ"
DATA="$(cd "$PROJ" && pwd -P)/.claude/data"
ARM='scripts/harness/arm-main-push.sh'

n=0
fresh_sid() { n=$((n + 1)); SID="p$n-$$"; }
fresh_sid

block() { expect_rc 2 "$1" "$HOOK" "$(bash_payload "$2" "$PROJ" "$SID")"; }
allow() { expect_rc 0 "$1" "$HOOK" "$(bash_payload "$2" "$PROJ" "$SID")"; }

echo "== gh: guarded forms block =="
block "release create" 'gh release create v1.0.0 --notes x'
block "release edit" 'gh release edit v1.0.0 --draft=false'
block "release delete" 'gh release delete v1.0.0 -y'
block "release upload" 'gh release upload v1.0.0 dist/a.tar.gz'
block "release delete-asset" 'gh release delete-asset v1.0.0 a.tar.gz'
block "release -R before the subcommand" 'gh release -R turbokast/mythhelm create v1'
block "workflow run" 'gh workflow run release.yml -f tag=v1'
block "workflow enable" 'gh workflow enable ci.yml'
block "workflow disable" 'gh workflow disable ci.yml'
block "run rerun" 'gh run rerun 123 --failed'
block "run cancel" 'gh run cancel 123'
block "run delete" 'gh run delete 123'
block "secret set" 'gh secret set TOKEN < t'
block "secret delete" 'gh secret delete TOKEN'
block "secret remove" 'gh secret remove TOKEN'
block "variable set" 'gh variable set X --body 1'
block "variable delete" 'gh variable delete X'
block "ruleset mutation (unknown sub)" 'gh ruleset create'
block "repo edit" 'gh repo edit --visibility public'
block "repo rename" 'gh repo rename newname'
block "repo archive" 'gh repo archive -y'
block "repo transfer" 'gh repo transfer other-org'
block "repo create" 'gh repo create x --public'
block "repo deploy-key add" 'gh repo deploy-key add key.pub'
block "repo sync --force" 'gh repo sync --force'
block "pr merge --admin" 'gh pr merge 12 --squash --admin'
block "pr merge --admin=true" 'gh pr merge 12 --admin=true'
block "alias set" "gh alias set rc 'release create'"
block "api DELETE" 'gh api -X DELETE repos/turbokast/mythhelm/git/refs/tags/v1'
block "api -XPATCH" 'gh api -XPATCH repos/turbokast/mythhelm -f private=false'
block "api --method=PUT" 'gh api --method=PUT repos/{owner}/{repo}/topics'
block "api --method put (lowercase)" 'gh api --method put orgs/turbokast/x'
block "api implied POST to releases" 'gh api repos/turbokast/mythhelm/releases -f tag_name=v1'
block "api POST to dispatches" 'gh api -X POST repos/o/r/actions/workflows/release.yml/dispatches'
block "api POST to git refs" 'gh api repos/o/r/git/refs -f ref=refs/tags/v1 -f sha=abc'
block "api POST to orgs" 'gh api -X POST orgs/turbokast/repos -f name=x'
block "api full URL" 'gh api -X DELETE https://api.github.com/repos/o/r'
block "api leading slash" 'gh api -X PATCH /repos/o/r'
block "api non-literal endpoint" 'gh api -X DELETE "$EP"'
block "api graphql mutation" "gh api graphql -f query='mutation { deleteRef(input: {refId: \"x\"}) { clientMutationId } }'"
block "api graphql --input" 'gh api graphql --input q.json'

echo "== gh: bypass spellings =="
block "bash -c" "bash -c 'gh release create v1'"
block "eval" 'eval gh release create v1'
block "env" 'env GH_TOKEN=x gh release create v1'
block "command" 'command gh release create v1'
block "chain" 'go test ./... && gh release create v1'
block "absolute path" '/usr/bin/gh release create v1'
block "quoted words" '"gh" "release" "create" v1'
block "substitution" 'echo $(gh secret set X -b y)'

echo "== gh: read-only and ordinary forms pass =="
allow "release list" 'gh release list'
allow "release view" 'gh release view v1.0.0'
allow "release download" 'gh release download v1.0.0'
allow "workflow list" 'gh workflow list'
allow "workflow view" 'gh workflow view ci.yml'
allow "run list" 'gh run list --limit 5'
allow "run watch" 'gh run watch 123'
allow "run view --log-failed" 'gh run view 123 --log-failed'
allow "secret list" 'gh secret list'
allow "variable get" 'gh variable get X'
allow "ruleset view" 'gh ruleset view 1'
allow "ruleset check" 'gh ruleset check main'
allow "repo view" 'gh repo view'
allow "repo clone" 'gh repo clone turbokast/mythhelm'
allow "pr create" 'gh pr create --base main --head topic --title t --body b'
allow "pr checks" 'gh pr checks 12 --watch --interval 20'
allow "pr merge without --admin" 'gh pr merge 12 --squash'
allow "pr comment" 'gh pr comment 12 --body "do not gh release create yet"'
allow "issue create" 'gh issue create --title "gh repo delete is scary" --body x'
allow "api GET" 'gh api repos/o/r/releases'
allow "api GET with -q" 'gh api repos/o/r/pulls -q ".[].number"'
allow "api -X GET with fields" 'gh api -X GET search/issues -f q=repo:o/r'
allow "api POST to an issue comment" 'gh api repos/o/r/issues/1/comments -f body=hi'
allow "api graphql query" "gh api graphql -f query='query { viewer { login } }'"
allow "api DELETE outside repos/orgs" 'gh api -X DELETE user/starred/o/r'
allow "echo" 'echo gh release create v1'
allow "heredoc" $'cat <<EOF\ngh release create v1\nEOF'
allow "auth status" 'gh auth status'

echo "== git: release tags block =="
block "push a v tag" 'git push origin v1.2.3'
block "push refs/tags/v" 'git push origin refs/tags/v1.2.3'
block "push HEAD to a v tag" 'git push origin HEAD:refs/tags/v2.0.0'
block "delete a v tag" 'git push origin :refs/tags/v1.2.3'
block "--delete a v tag" 'git push --delete origin v1.2.3'
block "--tags" 'git push --tags'
block "--tags with remote" 'git push origin --tags'
block "--follow-tags" 'git push --follow-tags origin topic'
block "--mirror" 'git push --mirror origin'
block "tag glob" 'git push origin "refs/tags/*:refs/tags/*"'
block "non-literal refspec" 'git push origin "$REF"'
block "+tag" 'git push origin +v1.2.3'
block "bash -c" "bash -c 'git push origin v1.2.3'"
block "git -C" 'git -C /tmp push origin v1.2.3'
allow "branch push" 'git push -u origin harness/p1-substrate'
allow "v-prefixed branch spelled in full" 'git push origin refs/heads/v2-work'
allow "non-release tag" 'git push origin refs/tags/nightly-build'
allow "fetch tags" 'git fetch --tags'
allow "local tag" 'git tag -a v1.2.3 -m v1.2.3'

echo "== never, armed or not =="
fresh_sid
allow "publish arm" "$ARM --publish --reason 'release v1.0.0 requested by the operator'"
block "repo delete" 'gh repo delete turbokast/mythhelm --yes'
expect_err "operator" "repo delete names the operator"
block "api DELETE of the repository, armed" 'gh api -X DELETE repos/turbokast/mythhelm'
block "api DELETE of the repository, placeholders" 'gh api --method DELETE repos/{owner}/{repo}/'
block "api DELETE of the repository, full URL" 'gh api -X DELETE https://api.github.com/repos/o/r'
block "api DELETE of a variable repository" 'gh api -X DELETE "repos/$REPO"'
allow "armed api DELETE of a tag ref" 'gh api -X DELETE repos/o/r/git/refs/tags/v0'

echo "== arming =="
fresh_sid
block "unarmed release" 'gh release create v1.0.0'
allow "arm" "$ARM --publish --reason 'operator asked for v1.0.0'"
check "publish sentinel written" [ -f "$DATA/publish-arm-$SID.json" ]
allow "armed release" 'gh release create v1.0.0 --notes x'
allow "armed tag push" 'git push origin v1.0.0'
allow "armed api" 'gh api -X PATCH repos/o/r -f description=x'
OWN="$SID"
fresh_sid
block "another session is not armed" 'gh release create v1.0.0'
SID="$OWN"
allow "publish disarm" "$ARM --publish --disarm"
check "publish sentinel removed" [ ! -f "$DATA/publish-arm-$SID.json" ]
block "disarmed" 'gh release create v1.0.0'
fresh_sid
allow "arm && release in one command" "$ARM --publish --reason v1 && gh release create v1"
allow "plain --disarm disarms publish too" "$ARM --disarm"
check "sentinel removed by plain disarm" [ ! -f "$DATA/publish-arm-$SID.json" ]
fresh_sid
allow "main-push arm" "$ARM --reason 'direct push'"
block "a main-push window does not publish" 'gh release create v1'
check "no publish sentinel from a main-push arm" [ ! -f "$DATA/publish-arm-$SID.json" ]
fresh_sid
allow "arm with no reason" "$ARM --publish"
block "no window after an invalid arm" 'gh release create v1'
fresh_sid
jq -n --arg s "$SID" --argjson ep "$(( $(date +%s) - 1801 ))" '{session_id:$s,armed_at_epoch:$ep}' > "$DATA/publish-arm-$SID.json"
block "expired window" 'gh release create v1'
expect_err "expired" "expiry is named"

echo "== the block teaches =="
fresh_sid
run_hook "$HOOK" "$(bash_payload 'gh release create v1' "$PROJ" "$SID")"
expect_stanza guard-publish "stanza shape"
expect_err "--publish --reason" "fix names the publish arming command"
run_hook "$HOOK" "$(bash_payload 'git push origin v2' "$PROJ" "$SID")"
expect_err "refs/heads/" "tag fix names the full branch spelling"

echo "== fail closed without jq =="
CHECKS=$((CHECKS + 1))
RC=$(bash_payload 'gh release create v1' "$PROJ" "$SID" | HOOK_JQ_PROBE=jq-not-installed "$HOOK" 2>/dev/null; echo $?)
[[ "$RC" == 2 ]] || fail "[jq missing blocks gh] got $RC"
CHECKS=$((CHECKS + 1))
RC=$(bash_payload 'ls' "$PROJ" "$SID" | HOOK_JQ_PROBE=jq-not-installed "$HOOK" 2>/dev/null; echo $?)
[[ "$RC" == 0 ]] || fail "[jq missing allows ls] got $RC"

finish
