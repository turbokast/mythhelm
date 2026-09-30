#!/usr/bin/env bash
# prune-merged.sh: deletes LOCAL branches, and the clean linked worktrees that hold
# them, once their pull requests have merged. It never deletes anything on the remote.
#
#   scripts/harness/prune-merged.sh [--apply] [--repo OWNER/REPO] [--branch NAME]...
#
# Dry run by default: it prints what it would prune and why each other branch stays.
# With --apply it prunes. --branch limits the run to the named branches.
#
# A branch is pruned only when every check passes:
#   1. it is not main, not the repository's default branch, and not checked out in
#      the main checkout or in the worktree this runs from;
#   2. `gh pr view <branch> --json state,mergedAt,headRefOid,number` reports the pull
#      request MERGED with a merge time;
#   3. the local tip equals the pull request's head, or is an ancestor of it, so every
#      local commit is part of what merged. A squash merge leaves no merged ancestry
#      for `git branch -d` to see, which is why this compares against the pull
#      request's head instead. When the head commit is not local it is fetched from
#      the pull request's ref (read-only);
#   4. a linked worktree holding the branch has no uncommitted or untracked change,
#      and is removed with `git worktree remove` (never --force, which refuses a dirty
#      tree anyway).
# The branch is then deleted with `git update-ref -d refs/heads/<b> <tip>`: the old
# value makes it a compare-and-swap, so a branch that moved after the check survives.
# block-destructive.sh refuses a bare `git update-ref -d` of a branch, and
# `git branch -D`, from an agent; this script is the sanctioned path because it runs
# the checks above first (.claude/hooks/README.md, "Pruning merged branches").
#
# Exit codes: 0 done (including branches skipped), 2 usage error or a missing tool.

set -euo pipefail

apply=0 repo="" only=()
while (( $# )); do
  case "$1" in
    --apply) apply=1; shift ;;
    --repo) repo="${2:-}"; shift 2 || { echo "prune-merged.sh: --repo needs a value" >&2; exit 2; } ;;
    --branch) only+=("${2:-}"); shift 2 || { echo "prune-merged.sh: --branch needs a value" >&2; exit 2; } ;;
    *) sed -n '5,8p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//' >&2; exit 2 ;;
  esac
done
for tool in git gh jq; do
  command -v "$tool" >/dev/null 2>&1 || { echo "prune-merged.sh: $tool is required" >&2; exit 2; }
done
top="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "prune-merged.sh: not inside a git repository" >&2; exit 2; }
repo_args=()
[[ -n "$repo" ]] && repo_args=(--repo "$repo")

# Branches checked out anywhere, and where. The first worktree listed is the main one.
declare -A wt_of=()
main_wt="" path=""
while IFS= read -r line; do
  case "$line" in
    "worktree "*) path="${line#worktree }"; [[ -n "$main_wt" ]] || main_wt="$path" ;;
    "branch refs/heads/"*) wt_of["${line#branch refs/heads/}"]="$path" ;;
  esac
done < <(git -C "$top" worktree list --porcelain)

default="$(git -C "$top" symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null || true)"
default="${default#origin/}"
current="$(git -C "$top" symbolic-ref --quiet --short HEAD 2>/dev/null || true)"

if (( ${#only[@]} )); then
  branches=("${only[@]}")
else
  mapfile -t branches < <(git -C "$top" for-each-ref --format='%(refname:short)' refs/heads/)
fi

mode="would prune"
(( apply )) && mode="pruned"
pruned=0 skipped=0
skip() { echo "skip $1: $2"; skipped=$((skipped + 1)); }

for b in "${branches[@]}"; do
  tip="$(git -C "$top" rev-parse --verify --quiet "refs/heads/$b^{commit}" || true)"
  if [[ -z "$tip" ]]; then skip "$b" "no such local branch"; continue; fi
  if [[ "$b" == main || "$b" == master || "$b" == "$default" || "$b" == "$current" ]]; then
    skip "$b" "main, default or current branch"; continue
  fi
  wt="${wt_of[$b]:-}"
  if [[ -n "$wt" && "$wt" == "$main_wt" ]]; then skip "$b" "checked out in the main checkout"; continue; fi

  view="$(gh pr view "$b" ${repo_args[@]+"${repo_args[@]}"} --json number,state,mergedAt,headRefOid 2>/dev/null || true)"
  row="$(printf '%s' "$view" | jq -r '[(.number // "" | tostring), (.state // ""), (.mergedAt // ""), (.headRefOid // "")] | join("\u001f")' 2>/dev/null || true)"
  IFS=$'\x1f' read -r num state merged head <<< "$row"
  if [[ -z "$num" ]]; then skip "$b" "no pull request found"; continue; fi
  if [[ "$state" != MERGED || -z "$merged" ]]; then skip "$b" "pull request #$num is $state, not merged"; continue; fi
  if [[ ! "$head" =~ ^[0-9a-f]{40}$ ]]; then skip "$b" "pull request #$num reports no head commit"; continue; fi
  if ! git -C "$top" cat-file -e "$head^{commit}" 2>/dev/null; then
    git -C "$top" fetch -q origin "refs/pull/$num/head" 2>/dev/null || true
  fi
  if [[ "$tip" != "$head" ]] && ! git -C "$top" merge-base --is-ancestor "$tip" "$head" 2>/dev/null; then
    skip "$b" "local tip ${tip:0:12} holds commits pull request #$num did not merge (head ${head:0:12})"; continue
  fi
  if [[ -n "$wt" ]]; then
    if [[ -n "$(git -C "$wt" status --porcelain 2>/dev/null || echo unreadable)" ]]; then
      skip "$b" "its worktree $wt has uncommitted or untracked changes"; continue
    fi
  fi
  if (( apply )); then
    if [[ -n "$wt" ]]; then
      git -C "$top" worktree remove "$wt" || { skip "$b" "git worktree remove $wt failed"; continue; }
    fi
    git -C "$top" update-ref -d "refs/heads/$b" "$tip" || { skip "$b" "the branch moved after the check"; continue; }
  fi
  echo "$mode $b (pull request #$num merged $merged; tip ${tip:0:12} is in head ${head:0:12})${wt:+; worktree $wt}"
  pruned=$((pruned + 1))
done

echo "prune-merged: $mode $pruned, skipped $skipped$( (( apply )) || echo '; dry run, pass --apply to prune')"
