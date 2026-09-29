#!/usr/bin/env bash
# shellcheck disable=SC2016  # commands under test are literal data
# test_block_destructive_shared_index.sh: in the MAIN checkout of the project's
# repository, git add/commit forms that sweep the shared index are blocked; the
# same forms pass in a linked worktree and in any other repository.
#
# Fixture: a project repository (CLAUDE_PROJECT_DIR) with a linked worktree, plus
# an unrelated repository. Every payload carries an explicit cwd.

# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
HOOK="$HOOKS_DIR/block-destructive.sh"

MAIN="$TEST_TMP/main"
WT="$TEST_TMP/main/.claude/worktrees/wt"
OTHER="$TEST_TMP/other"
new_repo "$MAIN"
mkdir -p "$MAIN/internal"
git -C "$MAIN" worktree add -q "$WT" -b wt 2>/dev/null
new_repo "$OTHER"

# expect <want> <description> <cwd> <command> [project dir]
expect() {
  local proj="${5-$MAIN}"
  CHECKS=$((CHECKS + 1))
  RC=$(bash_payload "$4" "$3" | CLAUDE_PROJECT_DIR="$proj" "$HOOK" 2>/dev/null; echo $?)
  [[ "$RC" == "$1" ]] || fail "[$2] expected exit $1, got $RC (cwd $3, command: $4)"
}
B=2
A=0

echo "== main checkout: sweeping forms block =="
expect $B "add -A"                        "$MAIN" 'git add -A'
expect $B "add --all"                     "$MAIN" 'git add --all'
expect $B "add ."                         "$MAIN" 'git add .'
expect $B "add -u"                        "$MAIN" 'git add -u'
expect $B "add :/"                        "$MAIN" 'git add :/'
expect $B "add -A from a subdirectory"    "$MAIN/internal" 'git add -A'
expect $B "commit -a"                     "$MAIN" 'git commit -a -m wip'
expect $B "commit -am"                    "$MAIN" 'git commit -am wip'
expect $B "commit --all"                  "$MAIN" 'git commit --all -m wip'
expect $B "bare commit"                   "$MAIN" 'git commit -m "wip"'
expect $B "bare commit -s"                "$MAIN" 'git commit -s -m wip'
expect $B "bare commit --amend"           "$MAIN" 'git commit --amend --no-edit'
expect $B "commit -F - from a heredoc"    "$MAIN" $'git commit -F - <<EOF\nmsg\nEOF'
expect $B "commit -i path"                "$MAIN" 'git commit -i -m wip a.go'
expect $B "commit ."                      "$MAIN" 'git commit -m wip .'
expect $B "commit -- ."                   "$MAIN" 'git commit -m wip -- .'
expect $B "add path then bare commit"     "$MAIN" 'git add a.go && git commit -m mine'
expect $B "git -C main from a worktree"   "$WT"   "git -C $MAIN commit -m wip"
expect $B "cd main from a worktree"       "$WT"   "cd $MAIN && git add -A"
expect $B "bash -c"                       "$MAIN" "bash -c 'git commit -am wip'"
expect $B "env wrapper"                   "$MAIN" 'env FOO=1 git commit -m wip'
expect $B "-c option before subcommand"   "$MAIN" 'git -c user.name=x commit -m wip'
expect $B "redirect is not a pathspec"    "$MAIN" 'git commit -m wip > /dev/null 2>&1'
expect $B "no pathspec after a heredoc message" "$MAIN" $'git commit -m "$(cat <<\'EOF\'\nsubject\nEOF\n)"'
expect $B "unbalanced paren in message"   "$MAIN" $'git commit -m "$(cat <<EOF\nmsg )\nEOF\n)"'
expect $B "sweep inside a substitution"   "$MAIN" 'x=$(git add -A)'

echo "== main checkout: scoped forms pass =="
expect $A "commit -- path"                "$MAIN" 'git commit -m wip -- a.go'
expect $A "commit path"                   "$MAIN" 'git commit -m wip a.go'
expect $A "commit -s -- path"             "$MAIN" 'git commit -s -m "wip" -- a.go b.go'
expect $A "pathspec after a heredoc message" "$MAIN" $'git commit -m "$(cat <<\'EOF\'\nfix (x\n\nbody\nEOF\n)" -- a.go'
expect $A "pathspec after a multi-line message" "$MAIN" $'git commit -m "subject\n\nbody" -- a.go'
expect $A "add path && commit path"       "$MAIN" 'git add a.go && git commit -m m a.go'
expect $A "pathspec then redirects"       "$MAIN" 'git commit -m wip -- a.go >/dev/null 2>&1'
expect $A "message names git add -A"      "$MAIN" 'git commit -m "never git add -A or git commit -a" -- a.go'
expect $A "-mall is a message"            "$MAIN" 'git commit -mall -- a.go'
expect $A "heredoc prose"                 "$MAIN" $'git commit -F - -- a.go <<EOF\ngit add -A\nEOF'
expect $A "echo mentions git add -A"      "$MAIN" 'echo "git add -A && git commit -am x"'
expect $A "--pathspec-from-file"          "$MAIN" 'git commit --pathspec-from-file=list.txt -m wip'
expect $A "--dry-run"                     "$MAIN" 'git commit --dry-run'
expect $A "private GIT_INDEX_FILE"        "$MAIN" 'GIT_INDEX_FILE=/tmp/idx git commit -m wip'
expect $A "add a file"                    "$MAIN" 'git add a.go'
expect $A "add -A with a scoped pathspec" "$MAIN" 'git add -A -- internal/'
expect $A "add -u -- file"                "$MAIN" 'git add -u -- a.go'
expect $A "status"                        "$MAIN" 'git status --porcelain'

echo "== linked worktree and other repositories pass =="
expect $A "worktree: add -A && commit"    "$WT"    'git add -A && git commit -m "task 1"'
expect $A "worktree: commit -am"          "$WT"    'git commit -am wip'
expect $A "main cwd, git -C worktree"     "$MAIN"  "git -C $WT commit -am wip"
expect $A "main cwd, cd worktree first"   "$MAIN"  "cd $WT && git add -A && git commit -m wip"
expect $A "another repository"            "$OTHER" 'git add -A && git commit -m init'
expect $A "not a repository"              "$TEST_TMP" 'git commit -am wip'
expect $A "non-literal cd keeps the cwd"  "$WT"    'cd "$X" && git commit -am wip'

echo "== project unknown: any main checkout is guarded =="
expect $B "no project dir, main checkout" "$OTHER" 'git commit -m init' ""
expect $A "no project dir, worktree"      "$WT"    'git commit -m init' ""

echo "== the block teaches =="
run_hook "$HOOK" "$(bash_payload 'git commit -m wip' "$MAIN")"
expect_stanza block-destructive "stanza shape"
expect_err "-- <file>" "fix names an explicit pathspec"

finish
