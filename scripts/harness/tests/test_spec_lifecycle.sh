#!/usr/bin/env bash
# test_spec_lifecycle.sh: spec-lifecycle.sh resolves a spec to its single copy,
# refuses duplicates in the working tree or the index, moves only along the
# lifecycle table, and keeps the index in step with the working tree.

# shellcheck source=../../../.claude/hooks/tests/lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/../../../.claude/hooks/tests/lib.sh"
SL="$REPO_ROOT/scripts/harness/spec-lifecycle.sh"

# run <args...>: runs the script in $R; sets RC, OUT and ERR.
run() {
  local errf="$TEST_TMP/.err"
  OUT="$(cd "$R" && "$SL" "$@" 2>"$errf")"
  RC=$?
  ERR="$(cat "$errf")"
}

expect() {   # expect <rc> <description>
  CHECKS=$((CHECKS + 1))
  [[ "$RC" == "$1" ]] || fail "[$2] expected exit $1, got $RC. out: $OUT err: ${ERR:0:300}"
}

# repo <name>: a fresh repository with a committed spec in todo/ and an epic plan in unrefined/.
repo() {
  R="$TEST_TMP/$1"
  new_repo "$R"
  mkdir -p "$R/specs/todo/demo" "$R/specs/unrefined/big-epic"
  printf 'r\n' > "$R/specs/todo/demo/requirements.md"
  printf 'd\n' > "$R/specs/todo/demo/design.md"
  printf 't\n' > "$R/specs/todo/demo/tasks.md"
  printf 'p\n' > "$R/specs/unrefined/big-epic/plan.md"
  git -C "$R" add specs
  git -C "$R" commit -q -m specs
}

echo "== resolve =="
repo resolve
run resolve demo
expect 0 "a single committed copy resolves"
check "it prints the lifecycle path" [ "$OUT" == specs/todo/demo ]
mkdir -p "$R/sub/dir"
OUT="$(cd "$R/sub/dir" && "$SL" resolve demo 2>/dev/null)"
check "it resolves from a subdirectory" [ "$OUT" == specs/todo/demo ]
mkdir -p "$R/specs/refined/fresh" && printf 'r\n' > "$R/specs/refined/fresh/requirements.md"
run resolve fresh
expect 0 "an untracked spec resolves"
run resolve ghost
expect 1 "a missing spec is refused"
check "the refusal is a stanza" grep -q '^BLOCK: spec-lifecycle' <<< "$ERR"
run resolve Bad_Name
expect 2 "a non-kebab-case name is a usage error"

echo "== duplicates =="
repo dup-tree
mkdir -p "$R/specs/in-progress/demo" && cp "$R/specs/todo/demo/"* "$R/specs/in-progress/demo/"
run resolve demo
expect 1 "two working-tree copies are refused"
check "both copies are named" grep -q 'specs/in-progress/demo specs/todo/demo' <<< "$ERR"
repo dup-index
mkdir -p "$R/specs/in-progress"
mv "$R/specs/todo/demo" "$R/specs/in-progress/demo"
run resolve demo
expect 1 "a copy left in the index by a plain mv is refused"
check "the index copy is named" grep -q 'specs/todo/demo' <<< "$ERR"
repo index-only
rm -r "$R/specs/todo/demo"
run resolve demo
expect 1 "a copy only in the index is refused"
check "the index-only refusal says so" grep -q 'only in the git index' <<< "$ERR"

echo "== fail closed =="
repo corrupt-index
printf 'not an index' > "$R/.git/index"
run resolve demo
expect 1 "an unreadable index is refused, not treated as empty"
check "the refusal names the index" grep -q 'cannot read the index' <<< "$ERR"
run move demo in-progress
expect 1 "a move with an unreadable index is refused"
check "the refused move leaves the spec in place" [ -d "$R/specs/todo/demo" ]
repo inherited-env
OTHER="$TEST_TMP/other"
new_repo "$OTHER"
OUT="$(cd "$R" && GIT_DIR="$OTHER/.git" GIT_WORK_TREE="$OTHER" GIT_INDEX_FILE="$OTHER/.git/index" "$SL" resolve demo 2>/dev/null)"
check "inherited GIT_DIR, GIT_WORK_TREE and GIT_INDEX_FILE are ignored" [ "$OUT" == specs/todo/demo ]

echo "== move =="
repo move
run move demo in-progress
expect 0 "todo -> in-progress moves"
check "the new directory exists" [ -f "$R/specs/in-progress/demo/tasks.md" ]
check "the old directory is gone" [ ! -e "$R/specs/todo/demo" ]
check "the index follows the move" [ "$(git -C "$R" ls-files -- specs/todo/demo)" == "" ]
check "the index holds the new path" [ -n "$(git -C "$R" ls-files -- specs/in-progress/demo)" ]
check "nothing is committed" [ "$(git -C "$R" rev-list --count HEAD)" == 2 ]
run resolve demo
expect 0 "the moved spec resolves to its one copy"
printf 'note\n' > "$R/specs/in-progress/demo/scratchpad.md"
run move demo unfinalized
expect 0 "in-progress -> unfinalized moves an untracked file along"
check "the untracked file moved" [ -f "$R/specs/unfinalized/demo/scratchpad.md" ]
run move demo "done"
expect 0 "unfinalized -> done moves"
run move demo todo
expect 1 "done -> todo is not a transition"
check "the refusal names the table" grep -q 'specs/README.md' <<< "$ERR"
check "a refused move leaves the spec in place" [ -d "$R/specs/done/demo" ]
run move demo archived
expect 0 "done -> archived moves"
run move demo archived
expect 1 "archived -> archived is not a transition"

repo skip
run move demo "done"
expect 1 "todo -> done skips states and is refused"
run move demo sideways
expect 2 "an unknown state is a usage error"
mkdir -p "$R/specs/refined/new-idea" && printf 'r\n' > "$R/specs/refined/new-idea/requirements.md"
run move new-idea todo
expect 0 "an untracked refined spec moves to todo"
check "the untracked spec moved" [ -f "$R/specs/todo/new-idea/requirements.md" ]
mkdir -p "$R/specs/unrefined/clash" "$R/specs/todo/clash" && printf 'r\n' > "$R/specs/unrefined/clash/requirements.md"
run move clash todo
expect 1 "an existing destination is refused as a duplicate"

echo "== epic plans =="
repo epic
run move big-epic todo
expect 1 "an epic plan never enters todo/"
run move big-epic in-progress
expect 0 "an epic plan moves from unrefined/ to in-progress/"
run move big-epic unfinalized
expect 1 "an epic plan never enters unfinalized/"
run move big-epic "done"
expect 0 "an epic plan moves from in-progress/ to done/"

echo "== usage =="
repo usage
run
expect 2 "no arguments is a usage error"
run move demo
expect 2 "move without a state is a usage error"
OUT="$(cd "$TEST_TMP" && "$SL" resolve demo 2>&1)"; RC=$?
expect 2 "outside a repository is a usage error"

finish
