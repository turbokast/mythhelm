#!/usr/bin/env bash
# shellcheck disable=SC2016,SC2088  # commands under test are literal data
# test_block_destructive.sh: block-destructive.sh on git history/worktree
# operations, recursive deletes, and the bypass spellings the tokenizer must see
# through. The shared-index rule has its own test file.

# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
HOOK="$HOOKS_DIR/block-destructive.sh"

PROJ="$TEST_TMP/proj"
new_repo "$PROJ"
mkdir -p "$PROJ/internal/tui" "$PROJ/bin"
export CLAUDE_PROJECT_DIR="$PROJ"
# A linked worktree is used as the cwd for the non-index checks, so the
# shared-index rule never decides a verdict here.
WT="$TEST_TMP/wt"
git -C "$PROJ" worktree add -q "$WT" -b wt 2>/dev/null

block() { expect_rc 2 "$1" "$HOOK" "$(bash_payload "$2" "${3:-$WT}")"; }
allow() { expect_rc 0 "$1" "$HOOK" "$(bash_payload "$2" "${3:-$WT}")"; }

echo "== git stash =="
block "stash" 'git stash'
block "stash push" 'git stash push -m wip'
block "stash pop" 'git stash pop'
block "stash apply" 'git stash apply stash@{0}'
block "stash drop" 'git stash drop'
block "stash clear" 'git stash clear'
block "stash with -u" 'git stash -u'
allow "stash list" 'git stash list'
allow "stash show" 'git stash show -p'

echo "== history and worktree =="
block "reset --hard" 'git reset --hard origin/main'
allow "reset --soft" 'git reset --soft HEAD~1'
allow "reset (mixed)" 'git reset HEAD a.go'
block "clean -f" 'git clean -f'
block "clean -fdx" 'git clean -fdx'
block "clean --force" 'git clean --force -d'
allow "clean -n" 'git clean -n'
block "branch -D" 'git branch -D feature'
block "branch --delete --force" 'git branch --delete --force feature'
allow "branch -d" 'git branch -d feature'
block "checkout -- ." 'git checkout -- .'
block "checkout ." 'git checkout .'
block "checkout -f" 'git checkout -f main'
block "restore ." 'git restore .'
block "restore --worktree :/" 'git restore --worktree :/'
allow "restore --staged ." 'git restore --staged .'
allow "restore a file" 'git restore a.go'
block "switch --discard-changes" 'git switch --discard-changes main'
allow "switch -c" 'git switch -c topic'

echo "== force push =="
block "--force" 'git push --force origin topic'
block "-f" 'git push -f origin topic'
block "-uf cluster" 'git push -uf origin topic'
block "+refspec" 'git push origin +topic'
block "--mirror" 'git push --mirror origin'
allow "--force-with-lease" 'git push --force-with-lease origin topic'
allow "plain push" 'git push -u origin topic'
allow "-o value is not a flag cluster" 'git push -o ci.skip origin topic'

echo "== recursive rm =="
block "rm -rf /" 'rm -rf /'
block "rm -rf /*" 'rm -rf /*'
block "rm -rf ~" 'rm -rf ~'
block "rm -rf \$HOME" 'rm -rf $HOME'
block "rm -rf \${HOME}/" 'rm -rf ${HOME}/'
block "rm -rf a top-level dir" 'rm -rf /usr'
block "rm -rf home by path" "rm -rf $HOME"
block "rm -rf home glob" "rm -rf $HOME/*"
block "rm -rf the repo by path" "rm -rf $PROJ"
block "rm -rf the worktree root" 'rm -rf .' "$WT"
block "rm -rf .." 'rm -rf ..' "$WT/internal"
block "rm -rf * at the root" 'rm -rf *' "$PROJ"
block "rm -rf .git" 'rm -rf .git' "$PROJ"
block "rm -r -f split flags" 'rm -r -f .git' "$PROJ"
block "rm --recursive" 'rm --recursive --force .claude' "$PROJ"
block "rm -rf .github" 'rm -rf ./.github' "$PROJ"
block "rm -rf inside .claude" 'rm -rf .claude/data' "$PROJ"
block "each target judged" 'rm -rf bin .git' "$PROJ"
block "brace expansion" 'rm -rf {bin,.git}' "$PROJ"
block "cd .. then the repo" "cd .. && rm -rf proj" "$PROJ"
block "absolute .git of another repo" "rm -rf /srv/other/.git"
block "glob matching .git" 'rm -rf .g*' "$PROJ"
allow "rm -rf bin" 'rm -rf bin' "$PROJ"
allow "rm -rf internal/tui" 'rm -rf internal/tui' "$PROJ"
allow "rm -rf a glob of files" 'rm -rf *.tmp' "$PROJ"
allow "rm -rf /tmp path" 'rm -rf /tmp/scratch-x'
allow "non-recursive rm" 'rm .git/index.lock' "$PROJ"
allow "rm -rf a variable" 'rm -rf "$tmpdir"'

echo "== find -delete =="
block "find . -delete at the root" 'find . -delete' "$PROJ"
block "find / -exec rm" 'find / -exec rm {} +'
allow "find -name -delete" 'find . -name "*.orig" -delete' "$PROJ"
allow "find without delete" 'find . -type f' "$PROJ"

echo "== bypass spellings =="
block "bash -c" "bash -c 'git stash'"
block "sh -c chained" "sh -c 'true && git stash'"
block "eval" 'eval git stash'
block "eval quoted" 'eval "git reset --hard"'
block "env wrapper" 'env GIT_TRACE=1 git stash'
block "command wrapper" 'command git stash'
block "sudo wrapper" 'sudo -u root git stash'
block "timeout wrapper" 'timeout 5 git stash'
block "absolute git" '/usr/bin/git stash'
block "git -C" 'git -C /tmp stash'
block "global -c option" 'git -c core.pager=cat stash'
block "after ;" 'git status; git stash'
block "after &&" 'git status && git stash'
block "after ||" 'false || git stash'
block "after |" 'git status | git stash'
block "background &" 'sleep 1 & git stash'
block "subshell" '(git stash)'
block "brace group" '{ git stash; }'
block "command substitution" 'echo $(git stash)'
block "backticks" 'echo `git stash`'
block "inside quoted substitution" 'echo "$(git stash)"'
block "if condition" 'if git stash; then :; fi'
block "heredoc into bash" $'bash <<EOF\ngit stash\nEOF'
block "heredoc into bash, no space" $'bash<<EOF\ngit stash\nEOF'
block "heredoc into a process substitution" $'tee >(bash) <<EOF\ngit reset --hard\nEOF'
block "bash -o pipefail -c" "bash -o pipefail -c 'git reset --hard'"
block "quoted command word" '"git" stash'
block "partially quoted" 'g"it" st"ash"'
block "backslash inside word" 'g\it stash'
block "multi-line command" $'git status\ngit stash'
allow "commit message prose" 'git commit -m "never git stash or git reset --hard" -- a.go' "$WT"
allow "heredoc prose" $'cat <<EOF\ngit stash\nrm -rf /\nEOF'
allow "grep argument" 'grep -rn "git push --force" .'
allow "echo argument" 'echo git stash'
allow "comment" 'git status # git stash'
allow "single-quoted substitution text" "echo '\$(git stash)'"

echo "== the block teaches =="
run_hook "$HOOK" "$(bash_payload 'git stash' "$WT")"
expect_stanza block-destructive "stash stanza"
expect_err "worktree" "stash fix names a worktree"

echo "== fail closed without jq, fail open for unrelated commands =="
CHECKS=$((CHECKS + 1))
RC=$(printf '%s' "$(bash_payload 'git stash')" | HOOK_JQ_PROBE=jq-not-installed "$HOOK" 2>/dev/null; echo $?)
[[ "$RC" == 2 ]] || fail "[jq missing blocks git] got $RC"
CHECKS=$((CHECKS + 1))
RC=$(printf '%s' "$(bash_payload 'ls -la')" | HOOK_JQ_PROBE=jq-not-installed "$HOOK" 2>/dev/null; echo $?)
[[ "$RC" == 0 ]] || fail "[jq missing allows ls] got $RC"
expect_rc 2 "malformed JSON naming git" "$HOOK" '{"tool_input":{"command":"git stash"'
expect_rc 0 "malformed JSON, nothing guarded" "$HOOK" '{"tool_input":'
expect_rc 0 "no command" "$HOOK" '{"tool_input":{}}'

finish
