#!/usr/bin/env bash
# spec-lifecycle.sh: finds a spec by name across the lifecycle directories under
# specs/, and moves it from one lifecycle state to the next.
#
#   scripts/harness/spec-lifecycle.sh resolve <name>
#   scripts/harness/spec-lifecycle.sh move <name> <to-state>
#
# resolve prints specs/<state>/<name>. It first asserts that exactly one copy of
# the spec exists across the working tree AND the git index: a copy left in the
# index by a plain mv comes back on the next checkout or restore, and a later
# lookup then finds the wrong one. Two copies are never resolved by guessing.
#
# move checks the transition against the lifecycle table in specs/README.md,
# refuses when the destination exists, and renames the directory with git mv
# (plain mv for a spec git has never seen), so the index follows the working
# tree. It stages nothing else and commits nothing: the change that finishes the
# lifecycle step commits the move.
#
#   single spec: unrefined -> refined | todo;  refined -> todo;  todo -> in-progress;
#                in-progress -> unfinalized;  unfinalized -> done
#   epic plan:   unrefined -> refined | in-progress;  refined -> in-progress;
#                in-progress -> done
#   either:      any state but archived -> archived
#
# A directory holding plan.md and no requirements.md is an epic plan.
#
# Runs in the repository that contains the current directory. Exit 0 on success;
# 1 when the spec is missing, has more than one copy, or the move is refused (a
# BLOCK/File/Detail/Fix stanza on stderr); 2 on a usage error.
#
# Residual: a copy that exists only in another worktree or on another branch is
# invisible here.

set -euo pipefail

STATES=(in-progress unfinalized todo refined unrefined "done" archived)

usage() {
  echo "usage: spec-lifecycle.sh resolve <name> | move <name> <to-state>" >&2
  exit 2
}

refuse() {   # refuse <file> <detail> <fix>
  printf 'BLOCK: spec-lifecycle\nFile: %s\nDetail: %s\nFix: %s\n' "$1" "$2" "$3" >&2
  exit 1
}

# copies <name>: every specs/<state>/<name> present in the working tree or the index.
copies() {
  local s
  {
    for s in "${STATES[@]}"; do
      [[ -d "specs/$s/$1" ]] && echo "specs/$s/$1"
    done
    git ls-files -- ":(glob)specs/*/$1/**" | cut -d/ -f1-3
  } | LC_ALL=C sort -u
}

# resolve <name>: sets DIR to the one copy, or refuses.
resolve() {
  local -a found=()
  mapfile -t found < <(copies "$1")
  if (( ${#found[@]} == 0 )); then
    refuse "specs/*/$1" "no spec named $1 exists in any lifecycle state (${STATES[*]})." \
      "Check the name with ls specs/*/; create a spec with /create-spec or /spec."
  fi
  if (( ${#found[@]} > 1 )); then
    refuse "${found[*]}" "spec $1 has ${#found[@]} copies across the working tree and the index; exactly one may exist." \
      "Decide which copy is live, then remove the others from both the working tree and the index (git rm -r --cached, then rm -r), and retry. Never pick one by guessing."
  fi
  DIR="${found[0]}"
  [[ -d "$DIR" ]] || refuse "$DIR" "spec $1 exists only in the git index, not in the working tree." \
    "Restore it with git checkout -- $DIR, or remove it with git rm -r --cached $DIR, then retry."
}

allowed() {   # allowed <kind> <from> <to>
  [[ "$3" == archived && "$2" != archived ]] && return 0
  case "$1:$2:$3" in
    single:unrefined:refined|single:unrefined:todo|single:refined:todo) return 0 ;;
    single:todo:in-progress|single:in-progress:unfinalized|single:unfinalized:done) return 0 ;;
    epic:unrefined:refined|epic:unrefined:in-progress|epic:refined:in-progress) return 0 ;;
    epic:in-progress:done) return 0 ;;
  esac
  return 1
}

(( $# >= 2 )) || usage
cmd="$1"
name="$2"
[[ "$name" =~ ^[a-z0-9][a-z0-9-]*$ ]] || { echo "spec-lifecycle: spec names are kebab-case: $name" >&2; exit 2; }
root="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "spec-lifecycle: not inside a git repository" >&2; exit 2; }
cd "$root"

case "$cmd" in
  resolve)
    (( $# == 2 )) || usage
    resolve "$name"
    echo "$DIR"
    ;;
  move)
    (( $# == 3 )) || usage
    to="$3"
    [[ " ${STATES[*]} " == *" $to "* ]] || { echo "spec-lifecycle: unknown state: $to (${STATES[*]})" >&2; exit 2; }
    resolve "$name"
    from="$(cut -d/ -f2 <<< "$DIR")"
    kind=single
    [[ -f "$DIR/plan.md" && ! -f "$DIR/requirements.md" ]] && kind=epic
    allowed "$kind" "$from" "$to" \
      || refuse "$DIR" "moving a $kind spec from $from/ to $to/ is not a lifecycle transition (see specs/README.md)." \
        "Run the skill that owns the step (/refine-spec, /spec, /run-spec, /finalize-spec, /evaluate-spec), or ask the maintainer."
    dest="specs/$to/$name"
    [[ ! -e "$dest" ]] || refuse "$dest" "the destination already exists." "Resolve the duplicate first; never overwrite a spec."
    mkdir -p "specs/$to"
    if [[ -n "$(git ls-files -- "$DIR")" ]]; then
      git mv -- "$DIR" "$dest"
    else
      mv -- "$DIR" "$dest"
    fi
    echo "$dest"
    ;;
  *) usage ;;
esac
