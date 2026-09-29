#!/usr/bin/env bash
# shellcheck disable=SC2016,SC2088  # commands under test are literal data
# test_hook_helpers.sh: the shared tokenizer, git/cd tracking and arming parser in
# hook-helpers.sh, tested directly on command strings.

# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
# shellcheck source=../hook-helpers.sh
. "$HOOKS_DIR/hook-helpers.sh"

# segs <command>: the segments, one per line, with $HH_SENT shown as a space.
segs() { hh_command_segments "$1" | tr '\001' ' '; }

# expect_segs <description> <command> <expected segments, newline-separated>
expect_segs() {
  local got
  got="$(segs "$2" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
  CHECKS=$((CHECKS + 1))
  [[ "$got" == "$3" ]] || fail "[$1] segments differ. got:"$'\n'"$got"$'\n'"want:"$'\n'"$3"
}

# cmd_of <segment>: the command word hh_cmd_index finds, or <none>.
cmd_of() {
  hh_split_words "$1"
  if hh_cmd_index; then printf '%s' "${HH_WORDS[HH_CI]}"; else printf '<none>'; fi
}

echo "== segmentation =="
expect_segs "quoted argument is one word" 'git commit -m "never git push --force" -- a.go' \
  'git commit -m never git push --force -- a.go'
expect_segs "chains split" 'a && b || c; d | e & f' $'a\nb\nc\nd\ne\nf'
expect_segs "subshell and brace group split" '(cd x; git status) && { git log; }' $'cd x\ngit status\ngit log'
expect_segs "comment dropped" 'git status # git push --force' 'git status'
expect_segs "hash inside a word is not a comment" 'echo a#b' 'echo a#b'
expect_segs "heredoc body is data" $'cat <<EOF\ngit push --force\nEOF\ngit status' $'cat <<EOF\ngit status'
expect_segs "quoted heredoc delimiter" $'cat <<\'EOF\' > f\nrm -rf /\nEOF' 'cat <<EOF > f'
expect_segs "heredoc into bash is commands" $'bash <<EOF\ngit push --force\nEOF' $'bash <<EOF\ngit push --force'
expect_segs "heredoc into bash, no space" $'bash<<EOF\ngit stash\nEOF' $'bash<<EOF\ngit stash'
expect_segs "heredoc into source, no space" $'source<<EOF\ngit stash\nEOF' $'source<<EOF\ngit stash'
expect_segs "heredoc into a process substitution shell" $'tee >(bash) <<EOF\ngit stash\nEOF' $'tee >\nbash\n<<EOF\ngit stash'
expect_segs "piped heredoc into sh is commands" $'cat <<EOF | sh\ngit stash\nEOF' $'cat <<EOF\nsh\ngit stash'
expect_segs "here-string is not a heredoc" 'grep x <<< "$v"; git status' $'grep x <<< $v\ngit status'
expect_segs "substitution runs first and stays a word" 'git push origin $(git branch --show-current)' \
  $'git branch --show-current\ngit push origin $__SUBST__'
expect_segs "backticks" 'echo `git stash` done' $'git stash\necho $__SUBST__ done'
expect_segs "substitution inside double quotes" 'git commit -m "x $(date) y" -- f' $'date\ngit commit -m x $__SUBST__ y -- f'
expect_segs "heredoc inside a quoted substitution keeps the pathspec" \
  $'git commit -m "$(cat <<\'EOF\'\nfix (x\ngit push --force\nEOF\n)" -- a.go' $'cat <<EOF\ngit commit -m $__SUBST__ -- a.go'
expect_segs "newline inside quotes stays in the word" $'git commit -m "one\n\ntwo" -- f' 'git commit -m one  two -- f'
expect_segs "line continuation joins" $'git push \\\n  origin main' 'git push   origin main'
expect_segs "escaped space makes one command word, re-segmented" 'git\ push' $'git push\ngit push'
expect_segs "brace expansion stays a word" 'rm -rf {a,.git}' 'rm -rf {a,.git}'
expect_segs "parameter expansion stays a word" 'x=${A:-a b} git status' 'x=${A:-a b} git status'
expect_segs "bash -c payload is re-segmented" "bash -c 'git stash && git status'" \
  $'bash -c git stash && git status\ngit stash\ngit status'
expect_segs "bash -o pipefail -c payload" "bash -o pipefail -c 'git stash'" $'bash -o pipefail -c git stash\ngit stash'
expect_segs "bash +O and --rcfile before -c" "bash +O extglob --rcfile rc -e -c 'git stash'" $'bash +O extglob --rcfile rc -e -c git stash\ngit stash'
expect_segs "sh -lc payload" 'sh -lc "git stash"' $'sh -lc git stash\ngit stash'
expect_segs "eval payload" 'eval git stash' $'eval git stash\ngit stash'
expect_segs "nested bash -c eval" "bash -c 'eval \"git stash\"'" $'bash -c eval "git stash"\neval git stash\ngit stash'
expect_segs "ssh remote command" 'ssh host "git stash"' $'ssh host git stash\ngit stash'
expect_segs "single quotes keep dollar-paren literal" "echo '\$(git stash)'" 'echo $(git stash)'
expect_segs "arithmetic shift is not a heredoc" $'x=$((1 << 2))\ngit status' $'1 << 2\nx=$__SUBST__\ngit status'

echo "== command position =="
check "plain" [ "$(cmd_of 'git push')" = git ]
check "assignment prefix" [ "$(cmd_of 'A=1 B=2 git push')" = git ]
check "env with options" [ "$(cmd_of 'env -u X A=1 git push')" = git ]
check "command" [ "$(cmd_of 'command git push')" = git ]
check "sudo -u user" [ "$(cmd_of 'sudo -u root git push')" = git ]
check "timeout duration" [ "$(cmd_of 'timeout -k 5 30 git push')" = git ]
check "nohup nice" [ "$(cmd_of 'nohup nice -n 5 git push')" = git ]
check "xargs" [ "$(cmd_of 'xargs -n 1 git push')" = git ]
check "keywords" [ "$(cmd_of 'if ! git push')" = git ]
check "absolute path" [ "$(cmd_of '/usr/bin/git push')" = /usr/bin/git ]
check "basename" [ "$(hh_basename /usr/bin/git)" = git ]
check "no command" [ "$(cmd_of 'A=1')" = '<none>' ]

echo "== git options and -C =="
hh_split_words "git -C /work -c user.name=x --no-pager --git-dir=/g push origin main"
hh_cmd_index
hh_git_parse /base
check "subcommand after global options" [ "$HH_GIT_SUB" = push ]
check "-C moves the directory" [ "$HH_GIT_DIR" = /work ]
check "arguments after the subcommand" [ "${HH_GIT_ARGS[*]}" = "origin main" ]
hh_split_words "git -C sub -C deeper status"; hh_cmd_index; hh_git_parse /base
check "relative -C accumulates" [ "$HH_GIT_DIR" = /base/sub/deeper ]
hh_split_words 'git -C $X status'; hh_cmd_index; hh_git_parse /base
check "non-literal -C keeps the directory" [ "$HH_GIT_DIR" = /base ]
hh_split_words "git --version"; hh_cmd_index
hh_git_parse /base || true
check "no subcommand" [ -z "$HH_GIT_SUB" ]

echo "== cd tracking =="
HH_EFF_DIR=/base
for c in "cd /abs" "cd rel" 'cd "$X"' "cd -" "pushd -n other"; do
  hh_split_words "$c"; hh_cmd_index; hh_track_cd
done
check "cd sequence" [ "$HH_EFF_DIR" = /abs/rel/other ]
hh_split_words "cd"; hh_cmd_index; hh_track_cd
check "bare cd goes home" [ "$HH_EFF_DIR" = "$HOME" ]
check "join tilde" [ "$(hh_join_dir /b '~/x')" = "$HOME/x" ]

echo "== arming parser =="
# arm_of <command>: "<action> <kind> <operator> <reason>" or "not-arm".
arm_of() {
  hh_split_words "$1"
  hh_cmd_index || { printf 'not-arm'; return; }
  if hh_arm_parse; then printf '%s %s %s %s' "$HH_ARM_ACTION" "$HH_ARM_KIND" "$HH_ARM_OPERATOR" "$HH_ARM_REASON"
  else printf 'not-arm'; fi
}
check "arm" [ "$(arm_of 'scripts/harness/arm-main-push.sh --reason fix')" = "arm main-push false fix" ]
check "arm, quoted multi-word reason" [ "$(arm_of "$(hh_command_segments 'scripts/harness/arm-main-push.sh --reason "hot fix"')")" = "arm main-push false hot fix" ]
check "operator arm" [ "$(arm_of './scripts/harness/arm-main-push.sh --operator why')" = "arm main-push true why" ]
check "publish arm" [ "$(arm_of 'scripts/harness/arm-main-push.sh --publish --reason v1')" = "arm publish false v1" ]
check "via bash" [ "$(arm_of 'bash scripts/harness/arm-main-push.sh --reason x')" = "arm main-push false x" ]
check "disarm" [ "$(arm_of 'scripts/harness/arm-main-push.sh --disarm')" = "disarm main-push false " ]
check "missing reason" [ "$(arm_of 'scripts/harness/arm-main-push.sh --reason')" = "invalid main-push false " ]
check "flag as reason" [ "$(arm_of 'scripts/harness/arm-main-push.sh --reason --publish')" = "invalid main-push false --publish" ]
check "unknown flag" [ "$(arm_of 'scripts/harness/arm-main-push.sh --reason x --force')" = "invalid main-push false x" ]
check "disarm with reason" [ "$(arm_of 'scripts/harness/arm-main-push.sh --disarm --reason x')" = "invalid main-push false x" ]
check "echo is not an arm" [ "$(arm_of 'echo scripts/harness/arm-main-push.sh --reason x')" = "not-arm" ]

echo "== data dir =="
new_repo "$TEST_TMP/repo"
git -C "$TEST_TMP/repo" worktree add -q "$TEST_TMP/wt" -b wt 2>/dev/null
check "main checkout" [ "$(hh_data_dir "$TEST_TMP/repo")" = "$(hh_canon_path "$TEST_TMP/repo")/.claude/data" ]
check "linked worktree shares it" [ "$(hh_data_dir "$TEST_TMP/wt")" = "$(hh_canon_path "$TEST_TMP/repo")/.claude/data" ]
check "first repository wins" [ "$(hh_data_dir "" "$TEST_TMP/nowhere" "$TEST_TMP/wt")" = "$(hh_canon_path "$TEST_TMP/repo")/.claude/data" ]
check "outside a repository" [ "$(hh_data_dir "$TEST_TMP")" = "$TEST_TMP/.claude/data" ]

finish
