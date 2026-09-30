#!/usr/bin/env bash
# test_prune_merged.sh: prune-merged.sh deletes a local branch (and its clean linked
# worktree) only when a stand-in gh reports its pull request merged and the local tip
# is part of what merged; it is a dry run by default and skips main, the current
# branch, unmerged, unknown, diverged and dirty cases.

# shellcheck source=../../../.claude/hooks/tests/lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/../../../.claude/hooks/tests/lib.sh"
PM="$REPO_ROOT/scripts/harness/prune-merged.sh"

# A stand-in gh: `gh pr view <branch> ...` prints $GH_FIXTURES/<branch>.json, or
# fails like gh does when no pull request exists.
mkdir -p "$TEST_TMP/bin" "$TEST_TMP/fixtures"
cat > "$TEST_TMP/bin/gh" <<'EOF'
#!/usr/bin/env bash
[[ "$1 $2" == "pr view" ]] || exit 1
f="$GH_FIXTURES/${3//\//_}.json"
[[ -f "$f" ]] || { echo "no pull requests found for branch \"$3\"" >&2; exit 1; }
cat "$f"
EOF
chmod +x "$TEST_TMP/bin/gh"
export PATH="$TEST_TMP/bin:$PATH" GH_FIXTURES="$TEST_TMP/fixtures"

R="$TEST_TMP/repo"
new_repo "$R"
commit_on() {  # commit_on <branch> <file>: a new commit on <branch>; prints its sha
  git -C "$R" switch -q "$1" 2>/dev/null || git -C "$R" switch -q -c "$1"
  printf '%s\n' "$2" > "$R/$2"
  git -C "$R" add -- "$2"
  git -C "$R" commit -q -m "$2" -- "$2"
  git -C "$R" rev-parse HEAD
  git -C "$R" switch -q main
}
pr() {  # pr <branch> <number> <state> <head>
  local merged=""
  [[ "$3" == MERGED ]] && merged="2026-01-02T03:04:05Z"
  jq -nc --argjson n "$2" --arg s "$3" --arg m "$merged" --arg h "$4" \
    '{number:$n,state:$s,mergedAt:(if $m == "" then null else $m end),headRefOid:$h}' > "$GH_FIXTURES/$1.json"
}

exact="$(commit_on exact a.txt)";            pr exact 1 MERGED "$exact"
extra="$(commit_on behind-extra c.txt)"       # the PR head is one commit past the local tip
git -C "$R" switch -q behind-extra
git -C "$R" commit -q --allow-empty -m "pushed after the local copy"
behind_head="$(git -C "$R" rev-parse HEAD)"
git -C "$R" switch -q main
git -C "$R" branch -q -f behind-extra "$extra"
pr behind-extra 2 MERGED "$behind_head"
diverged_head="$(commit_on diverged d.txt)"; pr diverged 4 MERGED "$diverged_head"
commit_on diverged e.txt >/dev/null          # a local commit the PR never had
open_head="$(commit_on open f.txt)";         pr open 5 OPEN "$open_head"
commit_on nopr g.txt >/dev/null
wt_head="$(commit_on wtclean h.txt)";        pr wtclean 6 MERGED "$wt_head"
git -C "$R" worktree add -q "$TEST_TMP/wt-clean" wtclean
dirty_head="$(commit_on wtdirty i.txt)";     pr wtdirty 7 MERGED "$dirty_head"
git -C "$R" worktree add -q "$TEST_TMP/wt-dirty" wtdirty
printf 'scratch\n' > "$TEST_TMP/wt-dirty/untracked.txt"
pr main 8 MERGED "$(git -C "$R" rev-parse main)"

run() { OUT="$(cd "$R" && "$PM" "$@" 2>&1)"; RC=$?; }
has() { CHECKS=$((CHECKS + 1)); [[ "$OUT" == *"$1"* ]] || fail "[$2] output lacks '$1': $OUT"; }
ref() { git -C "$R" rev-parse --verify --quiet "refs/heads/$1" >/dev/null; }

echo "== dry run changes nothing =="
run
check "dry run exits 0" test "$RC" = 0
has "would prune exact" "exact merged at the tip"
has "would prune behind-extra" "tip is an ancestor of the merged head"
has "would prune wtclean" "clean worktree"
has "dry run, pass --apply" "says it is a dry run"
check "dry run kept exact" ref exact
check "dry run kept the clean worktree" test -d "$TEST_TMP/wt-clean"

echo "== apply prunes only the verified =="
run --apply
check "apply exits 0" test "$RC" = 0
check "exact deleted" bash -c "! git -C '$R' rev-parse --verify --quiet refs/heads/exact"
check "behind-extra deleted" bash -c "! git -C '$R' rev-parse --verify --quiet refs/heads/behind-extra"
check "wtclean deleted" bash -c "! git -C '$R' rev-parse --verify --quiet refs/heads/wtclean"
check "its clean worktree removed" test ! -d "$TEST_TMP/wt-clean"
check "diverged kept" ref diverged
has "skip diverged: local tip" "a local commit the PR never merged keeps the branch"
check "open kept" ref open
has "pull request #5 is OPEN" "an open PR keeps the branch"
check "nopr kept" ref nopr
has "skip nopr: no pull request found" "no PR keeps the branch"
check "wtdirty kept" ref wtdirty
check "dirty worktree kept" test -f "$TEST_TMP/wt-dirty/untracked.txt"
has "skip wtdirty: its worktree" "a dirty worktree keeps the branch"
check "main kept" ref main
has "skip main" "main is never pruned"

echo "== --branch limits the run; bad usage exits 2 =="
run --apply --branch diverged
has "pruned 0, skipped 1" "one named branch, skipped"
run --bogus
check "unknown flag exits 2" test "$RC" = 2

finish
