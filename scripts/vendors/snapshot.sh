#!/usr/bin/env bash
# snapshot.sh: builds a CLEAN linked worktree from a tree that is not, so the
# consult wrappers (which refuse anything but a clean linked worktree) can read a
# task's uncommitted work.
#
#   snapshot.sh create --from <dir> [--include-uncommitted] [--only <path>]...
#   snapshot.sh remove --dir <snapshot-dir>
#
# create adds a detached worktree at <dir>'s HEAD under $TMPDIR, copies the
# uncommitted changes when asked (tracked modifications and deletions, and untracked
# files that are NOT ignored: the `git add -A` set, so ignored files such as .env
# never leave), commits them there, and prints:
#   snapshot_dir=<worktree>  snapshot_base=<source HEAD>  snapshot_head=<commit>
#   snapshot_changed_files=<N>  snapshot_skipped_symlinks=<N>
# `--only <path>` (repeatable, a path prefix) limits the copy to a task's own files.
# .claude/data/ is never copied. Symlinks are never copied: dereferenced they could
# carry an ignored file's contents, and as links they point back at the source.
# The review diff of a snapshot is snapshot_base..snapshot_head (HEAD~1..HEAD when
# anything was copied).
#
# remove deletes only a directory of the shape create makes
# (<tmp>/vendor-snapshot.XXXXXX/wt) and prunes its registration.
#
# Exit 0 on success, 64 on a usage error, 65 when the source is not a work tree or
# the snapshot could not be built (nothing is left behind).

set -euo pipefail

usage() {
  sed -n '4,5p' "${BASH_SOURCE[0]}" | sed 's/^# *//' >&2
  exit 64
}

g() {
  local dir="$1"
  shift
  env -u GIT_DIR -u GIT_WORK_TREE -u GIT_INDEX_FILE -u GIT_COMMON_DIR git -C "$dir" "$@"
}

(( $# >= 1 )) || usage
MODE="$1"
shift
FROM="" DIR="" UNCOMMITTED=0
ONLY=()
while (( $# > 0 )); do
  case "$1" in
    --from) [[ $# -ge 2 ]] || usage; FROM="$2"; shift 2 ;;
    --dir) [[ $# -ge 2 ]] || usage; DIR="$2"; shift 2 ;;
    --only) [[ $# -ge 2 ]] || usage; ONLY+=("${2%/}"); shift 2 ;;
    --include-uncommitted) UNCOMMITTED=1; shift ;;
    *) echo "snapshot.sh: unknown argument: $1" >&2; usage ;;
  esac
done

selected() {
  local f="$1" p
  [[ "$f" == .claude/data/* ]] && return 1
  (( ${#ONLY[@]} == 0 )) && return 0
  for p in "${ONLY[@]}"; do
    [[ "$f" == "$p" || "$f" == "$p"/* ]] && return 0
  done
  return 1
}

case "$MODE" in
  create)
    [[ -n "$FROM" ]] || usage
    FROM="$(g "$FROM" rev-parse --show-toplevel 2>/dev/null)" || { echo "snapshot.sh: $FROM is not inside a git work tree" >&2; exit 65; }
    BASE="$(g "$FROM" rev-parse HEAD 2>/dev/null)" || { echo "snapshot.sh: $FROM has no HEAD" >&2; exit 65; }
    ROOT="$(mktemp -d "${TMPDIR:-/tmp}/vendor-snapshot.XXXXXX")"
    WT="$ROOT/wt"
    DONE=0
    cleanup() {
      (( DONE == 1 )) && return 0
      g "$FROM" worktree remove --force "$WT" >/dev/null 2>&1 || true
      g "$FROM" worktree prune >/dev/null 2>&1 || true
      rm -rf "$ROOT"
    }
    trap cleanup EXIT
    g "$FROM" worktree add -q --detach "$WT" "$BASE" >/dev/null 2>&1 || { echo "snapshot.sh: git worktree add failed" >&2; exit 65; }
    COPIED=0 LINKS=0
    if (( UNCOMMITTED == 1 )); then
      while IFS= read -r -d '' f; do
        selected "$f" || continue
        if [[ -L "$FROM/$f" ]]; then
          LINKS=$((LINKS + 1))
          continue
        fi
        if [[ -e "$FROM/$f" ]]; then
          mkdir -p "$WT/$(dirname "$f")"
          cp -p "$FROM/$f" "$WT/$f"
        else
          rm -f "$WT/$f"
        fi
        COPIED=$((COPIED + 1))
      done < <({ g "$FROM" diff HEAD -z --name-only --no-renames; g "$FROM" ls-files -z -o --exclude-standard; } | sort -zu)
      g "$WT" add -A >/dev/null
      if ! g "$WT" diff --cached --quiet; then
        g "$WT" -c user.name=vendor-snapshot -c user.email=vendor-snapshot@example.com \
          -c commit.gpgsign=false -c core.hooksPath=/dev/null \
          commit -q --no-verify -m "snapshot: uncommitted changes" >/dev/null
      fi
    fi
    if [[ -n "$(g "$WT" status --porcelain --ignored=matching)" ]]; then
      echo "snapshot.sh: the snapshot is not clean after its commit" >&2
      exit 65
    fi
    DONE=1
    printf 'snapshot_dir=%s\nsnapshot_base=%s\nsnapshot_head=%s\nsnapshot_changed_files=%s\nsnapshot_skipped_symlinks=%s\n' \
      "$WT" "$BASE" "$(g "$WT" rev-parse HEAD)" "$COPIED" "$LINKS"
    ;;
  remove)
    [[ -n "$DIR" ]] || usage
    DIR="${DIR%/}"
    PARENT="$(dirname "$DIR")"
    if [[ "$(basename "$DIR")" != wt || ! "$(basename "$PARENT")" =~ ^vendor-snapshot\.[A-Za-z0-9]{6}$ ]]; then
      echo "snapshot.sh: refusing to remove $DIR: not a directory snapshot.sh created" >&2
      exit 65
    fi
    if [[ -d "$DIR" ]]; then
      MAIN="$(g "$DIR" rev-parse --path-format=absolute --git-common-dir 2>/dev/null)" || MAIN=""
      if [[ -n "$MAIN" ]]; then
        g "$(dirname "$MAIN")" worktree remove --force "$DIR" >/dev/null 2>&1 || true
        g "$(dirname "$MAIN")" worktree prune >/dev/null 2>&1 || true
      fi
    fi
    rm -rf "$PARENT"
    echo "removed=$DIR"
    ;;
  *) usage ;;
esac
