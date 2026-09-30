#!/usr/bin/env bash
# block-destructive.sh: PreToolUse hook (Bash). Blocks commands that destroy work
# irrecoverably or rewrite shared history:
#
#   * git stash, except `stash list` and `stash show`. The stash is shared by every
#     session and worktree of a repository, so a stash in a shared checkout can
#     sweep up, and a pop or drop can destroy, a concurrent session's uncommitted
#     work. Commit on a branch or use a worktree instead.
#   * git reset --hard; git clean -f; git branch -D; git update-ref -d of a branch;
#     git worktree remove --force; checkout/restore/switch forms that discard the
#     working tree (-f, --discard-changes, a whole-tree pathspec). Merged local
#     branches and their clean worktrees are removed through
#     scripts/harness/prune-merged.sh, which verifies the merge first.
#   * force pushes: --force, -f in any short-flag cluster, a +refspec, --mirror.
#     --force-with-lease stays allowed for non-main branches.
#   * recursive rm, and find -delete, aimed at /, a top-level directory, $HOME, the
#     repository root, one of their ancestors, or a critical directory (.git,
#     .claude, .github). Each target is judged on its own.
#   * shared-index sweeps, in the MAIN checkout of this repository only: sessions
#     working in the main checkout share one index, so `git add -A|-u|.` and a
#     `git commit` that does not name its paths (-a, -i, a whole-tree pathspec, or
#     no pathspec) commit whatever other live sessions have staged. Linked
#     worktrees and other repositories are exempt; `git commit ... -- <paths>`,
#     --pathspec-from-file, --dry-run and a private GIT_INDEX_FILE pass.
#
# Detection is command-position only, via hook-helpers.sh: quoted strings and
# heredoc bodies are data, while wrappers, `bash -c`, `eval`, chains, `git -C` and
# global options are seen through. The directory a command runs in is the
# payload's cwd, moved by literal `cd <dir>` segments and `git -C <dir>`.
#
# Exit 0 allows. Exit 2 blocks, with the BLOCK/Command/Detail/Fix stanza on stderr.

set -euo pipefail
export PATH="${PATH:-/usr/local/bin:/usr/bin:/bin}"

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hook-helpers.sh
. "$HOOK_DIR/hook-helpers.sh"

hh_load_bash_payload block-destructive '(^|[^a-z])(rm|git|find)([^a-z]|$)' || exit 0

CRITICAL_DIRS=" .git .claude .github "
BROAD_PATHSPEC=" . ./ :/ :/* * ./* :(top) :(top)* :(glob)* :(glob)** .. ../ "

BASE_DIR="${HH_CWD:-${CLAUDE_PROJECT_DIR:-$PWD}}"
[[ -d "$BASE_DIR" ]] || BASE_DIR="$PWD"
PROJECT_ROOT=""
if [[ -n "${CLAUDE_PROJECT_DIR:-}" && -d "${CLAUDE_PROJECT_DIR}" ]]; then
  PROJECT_ROOT="$(hh_canon_path "$CLAUDE_PROJECT_DIR")"
fi
HOME_CANON="$(hh_canon_path "${HOME:-/nonexistent-home}")"

# -----------------------------------------------------------------------------
# rm / find targets
# -----------------------------------------------------------------------------

# canon <path>: absolute, `.` and `..` collapsed, symlinks resolved where possible.
canon() {
  local p="$1" part out=""
  local -a parts=() kept=()
  [[ "$p" == /* ]] || p="${HH_EFF_DIR%/}/$p"
  if realpath -m / >/dev/null 2>&1; then
    realpath -m "$p"
    return 0
  fi
  IFS=/ read -r -a parts <<< "$p"
  for part in ${parts[@]+"${parts[@]}"}; do
    case "$part" in
      ''|.) ;;
      ..) (( ${#kept[@]} )) && unset 'kept[${#kept[@]}-1]' ;;
      *) kept+=("$part") ;;
    esac
  done
  for part in ${kept[@]+"${kept[@]}"}; do out+="/$part"; done
  printf '%s' "${out:-/}"
}

is_ancestor_or_self() {   # is_ancestor_or_self <a> <b>: b is a, or lies under a
  local a="${1%/}" b="${2%/}"
  [[ -n "$1" && -n "$2" ]] || return 1
  [[ -z "$a" || "$b" == "$a" || "$b" == "$a"/* ]]
}

rm_block() {   # rm_block <verb> <target> <why>
  hh_block "$1 on '$2' is not allowed: $3." \
    "Delete specific files or a disposable directory (bin/, dist/, a /tmp path) instead. Never delete /, \$HOME, the repository root or one of its ancestors, or a critical directory (.git, .claude, .github)."
}

# brace_expand <word>: one level of {a,b} expansion, one result per line.
brace_expand() {
  local w="$1" pre body post alt
  if [[ "$w" =~ ^([^{]*)\{([^{}]*,[^{}]*)\}(.*)$ ]]; then
    pre="${BASH_REMATCH[1]}" body="${BASH_REMATCH[2]}" post="${BASH_REMATCH[3]}"
    local -a alts=()
    IFS=, read -r -a alts <<< "$body,"
    for alt in "${alts[@]}"; do printf '%s\n' "$pre$alt$post"; done
  else
    printf '%s\n' "$w"
  fi
}

check_target() {   # check_target <verb> <target>
  local verb="$1" t
  while IFS= read -r t; do
    check_one "$verb" "$t"
  done < <(brace_expand "$2")
}

check_one() {   # check_one <verb> <target>
  local verb="$1" t="$2" dir pat="" r root name
  [[ -n "$t" ]] || return 0
  # shellcheck disable=SC2088,SC2016  # literal, unexpanded spellings
  case "$t" in
    '~'|'~/'|'~/*'|'$HOME'|'$HOME/'|'$HOME/*'|'${HOME}'|'${HOME}/'|'${HOME}/*') rm_block "$verb" "$t" "it targets \$HOME" ;;
    '/'|'/*'|'/.'|'//') rm_block "$verb" "$t" "it targets the filesystem root" ;;
  esac
  # shellcheck disable=SC2088,SC2016
  [[ "$t" == '~/'* ]] && t="${HOME:-/nonexistent-home}/${t#\~/}"
  # shellcheck disable=SC2016
  [[ "$t" == '$HOME/'* ]] && t="${HOME:-/nonexistent-home}/${t#\$HOME/}"
  # shellcheck disable=SC2016
  [[ "$t" == '${HOME}/'* ]] && t="${HOME:-/nonexistent-home}/${t#\$\{HOME\}/}"
  # A non-literal target cannot be resolved here.
  [[ "$t" == *'$'* ]] && return 0
  local lex="$t"
  while [[ "$lex" == ./* ]]; do lex="${lex#./}"; done
  case "$lex" in
    ''|.|..|../|'../*'|'..*') rm_block "$verb" "$t" "it targets the working directory or its parent" ;;
  esac

  # A glob is judged by the directory it expands in and the pattern it applies. A
  # pattern of only wildcards and dots (*, .*, *.*) matches everything there.
  local matchall=0
  dir="$t"
  if [[ "$t" == *[\*\?\[]* ]]; then
    pat="${t##*/}"
    if [[ "$t" == */* ]]; then dir="${t%/*}"; [[ -n "$dir" ]] || dir=/; else dir=.; fi
    if [[ "$dir" == *[\*\?\[]* ]]; then
      rm_block "$verb" "$t" "a wildcard in a directory component cannot be judged safely"
    fi
    [[ -z "${pat//[\*\?.]/}" ]] && matchall=1
  fi
  r="$(canon "$dir")"

  [[ "$r" == / ]] && rm_block "$verb" "$t" "it resolves to the filesystem root"
  if [[ "$r" =~ ^/[^/]+$ && "$r" != /tmp ]]; then
    rm_block "$verb" "$t" "it resolves to the top-level directory $r"
  fi
  if is_ancestor_or_self "$r" "$HOME_CANON" && [[ -z "$pat" || "$r" != "$HOME_CANON" || $matchall -eq 1 ]]; then
    rm_block "$verb" "$t" "it resolves to \$HOME or one of its ancestors ($r)"
  fi
  for root in "$PROJECT_ROOT" "$EFF_TOP"; do
    [[ -n "$root" ]] || continue
    if is_ancestor_or_self "$r" "$root"; then
      if [[ -z "$pat" || "$r" != "$root" || $matchall -eq 1 ]]; then
        rm_block "$verb" "$t" "it resolves to the repository root or one of its ancestors ($r)"
      fi
      for name in $CRITICAL_DIRS; do
        # shellcheck disable=SC2053  # $pat is a glob pattern on purpose
        [[ "$name" == $pat ]] && rm_block "$verb" "$t" "the pattern '$pat' in the repository root matches the critical directory $name"
      done
      continue
    fi
    if [[ "$r" == "$root"/* ]]; then
      name="${r#"$root"/}"; name="${name%%/*}"
      [[ "$CRITICAL_DIRS" == *" $name "* ]] && rm_block "$verb" "$t" "it lies inside the critical directory $name ($r)"
    fi
  done
  [[ "$r" == /tmp/* ]] && return 0
  if [[ "$r" =~ /(\.git|\.claude|\.github)(/|$) ]]; then
    rm_block "$verb" "$t" "it lies inside a critical directory (${BASH_REMATCH[1]}) of a repository ($r)"
  fi
  if [[ -n "$pat" ]]; then
    for name in $CRITICAL_DIRS; do
      # shellcheck disable=SC2053
      [[ -e "$r/$name" && "$name" == $pat ]] && rm_block "$verb" "$t" "the pattern '$pat' matches $r/$name"
    done
  fi
  return 0
}

check_rm() {
  local n=${#HH_WORDS[@]} k w recursive=0 ddash=0
  local -a targets=()
  for (( k = HH_CI + 1; k < n; k++ )); do
    w="${HH_WORDS[k]}"
    if (( ddash == 0 )); then
      case "$w" in
        --) ddash=1; continue ;;
        --recursive) recursive=1; continue ;;
        --*) continue ;;
        -?*) [[ "$w" == *[rR]* ]] && recursive=1; continue ;;
      esac
    fi
    targets+=("$w")
  done
  (( recursive == 1 )) || return 0
  for w in ${targets[@]+"${targets[@]}"}; do check_target "rm -r" "$w"; done
}

check_find() {
  local n=${#HH_WORDS[@]} k w deletes=0 narrowed=0 in_paths=1 nx
  local -a starts=()
  for (( k = HH_CI + 1; k < n; k++ )); do
    w="${HH_WORDS[k]}"
    if (( in_paths == 1 )); then
      case "$w" in
        -H|-L|-P) continue ;;
        -D) k=$((k + 1)); continue ;;
        -O*) continue ;;
        -*|'('|'!'|')') in_paths=0 ;;
        *) starts+=("$w"); continue ;;
      esac
    fi
    case "$w" in
      -delete) deletes=1 ;;
      -exec|-execdir|-ok|-okdir)
        nx="$(hh_basename "${HH_WORDS[k+1]:-}")"
        case "$nx" in rm|rmdir|unlink|shred) deletes=1 ;; esac ;;
      -name|-iname|-path|-ipath|-wholename|-iwholename|-regex|-iregex)
        case "${HH_WORDS[k+1]:-}" in '*'|'.*'|'**') ;; *) narrowed=1 ;; esac ;;
      -newer*|-mtime|-mmin|-atime|-amin|-ctime|-cmin|-size|-inum|-samefile|-links|-empty)
        narrowed=1 ;;
    esac
  done
  (( deletes == 1 && narrowed == 0 )) || return 0
  (( ${#starts[@]} > 0 )) || starts=(.)
  for w in "${starts[@]}"; do check_target "find -delete" "$w"; done
}

# -----------------------------------------------------------------------------
# git
# -----------------------------------------------------------------------------

check_git() {
  local w
  hh_git_parse "$HH_EFF_DIR" || return 0
  local -a A=(${HH_GIT_ARGS[@]+"${HH_GIT_ARGS[@]}"})
  case "$HH_GIT_SUB" in
    add|commit) check_shared_index "$HH_GIT_SUB" "$HH_GIT_DIR" ${A[@]+"${A[@]}"} ;;
    push) check_push ${A[@]+"${A[@]}"} ;;
    reset)
      for w in ${A[@]+"${A[@]}"}; do
        [[ "$w" == --hard ]] && hh_block "git reset --hard discards uncommitted work irrecoverably." \
          "Commit the work on a branch first, or use git reset --soft / --mixed. Never use git stash in an agent session."
      done ;;
    branch)
      local del=0 force=0
      for w in ${A[@]+"${A[@]}"}; do
        case "$w" in
          --delete) del=1 ;;
          --force) force=1 ;;
          --*) ;;
          -*) [[ "$w" == *D* ]] && { del=1; force=1; }
              [[ "$w" == *d* ]] && del=1
              [[ "$w" == *f* ]] && force=1 ;;
        esac
      done
      (( del == 1 && force == 1 )) && hh_block "git branch -D force-deletes a branch that may hold unmerged work." \
        "Use git branch -d, which refuses to delete an unmerged branch."
      ;;
    update-ref)
      local del=0 ref="" skip=0
      for w in ${A[@]+"${A[@]}"}; do
        if (( skip == 1 )); then skip=0; continue; fi
        case "$w" in
          -m) skip=1 ;;
          -d|--delete) del=1 ;;
          -*) ;;
          *) [[ -z "$ref" ]] && ref="$w" ;;
        esac
      done
      if (( del == 1 )) && [[ "$ref" == refs/heads/* || "$ref" != refs/* ]]; then
        hh_block "git update-ref -d deletes a branch without checking that its work is merged anywhere." \
          "Use scripts/harness/prune-merged.sh, which deletes a local branch only after GitHub reports its pull request merged and the branch tip is part of what merged, or git branch -d."
      fi ;;
    worktree)
      if [[ "${A[0]:-}" == remove ]]; then
        for w in ${A[@]+"${A[@]:1}"}; do
          [[ "$w" == --force || "$w" =~ ^-[a-z]*f ]] && hh_block "git worktree remove --force discards the worktree's uncommitted and untracked work." \
            "Commit or push the work first; git worktree remove without --force refuses a dirty worktree. scripts/harness/prune-merged.sh removes merged, clean worktrees."
        done
      fi ;;
    checkout|restore|switch) check_discard "$HH_GIT_SUB" ${A[@]+"${A[@]}"} ;;
    clean)
      for w in ${A[@]+"${A[@]}"}; do
        case "$w" in
          --force) ;;
          --*) continue ;;
          -*) [[ "$w" == *f* ]] || continue ;;
          *) continue ;;
        esac
        hh_block "git clean -f permanently deletes untracked files." \
          "Preview with git clean -n, then delete only the files you mean to by path."
      done ;;
    stash)
      local verb=""
      for w in ${A[@]+"${A[@]}"}; do
        [[ "$w" == -* ]] && continue
        verb="$w"; break
      done
      case "$verb" in
        list|show) ;;
        *) hh_block "git stash${verb:+ $verb} moves or drops work through the stash, which every session and worktree of this repository shares. A stash in a shared checkout can sweep up a concurrent session's uncommitted work, and a pop or drop can destroy it. Only 'git stash list' and 'git stash show' are allowed." \
             "Commit the work on a branch (git switch -c <branch> && git commit), or do the isolated work in a linked worktree (git worktree add)." ;;
      esac ;;
  esac
  return 0
}

check_push() {
  local w skip=0
  for w in "$@"; do
    if (( skip == 1 )); then skip=0; continue; fi
    case "$w" in
      --force-with-lease|--force-with-lease=*|--force-if-includes) ;;
      --force) hh_block "git push --force overwrites remote history." \
        "Push normally. On your own non-main branch, use --force-with-lease if you must rewrite it." ;;
      --mirror) hh_block "git push --mirror force-updates and deletes remote refs to match local ones." \
        "Push the branch you mean: git push origin <branch>." ;;
      -o|--push-option|--repo|--receive-pack|--exec) skip=1 ;;
      --*) ;;
      -*) [[ "$w" == *f* ]] && hh_block "git push $w is a force push (-f in a short-flag cluster) and overwrites remote history." \
        "Push normally. On your own non-main branch, use --force-with-lease if you must rewrite it." ;;
      +*) hh_block "git push $w is a force push (+refspec) and overwrites remote history." \
        "Drop the leading '+'. On your own non-main branch, use --force-with-lease if you must rewrite it." ;;
    esac
  done
  return 0
}

check_discard() {   # check_discard <checkout|restore|switch> <args...>
  local sub="$1" w staged=0 worktree=0 broad=""
  shift
  for w in "$@"; do
    case "$w" in
      --force|--discard-changes)
        hh_block "git $sub $w discards uncommitted changes in the working tree." \
          "Commit the work on a branch first." ;;
      --staged) staged=1 ;;
      --worktree) worktree=1 ;;
      --*) ;;
      -*)
        [[ "$sub" != restore && "$w" == *f* ]] && hh_block "git $sub $w force-discards uncommitted changes in the working tree." \
          "Commit the work on a branch first."
        [[ "$sub" == restore && "$w" == *S* ]] && staged=1
        [[ "$sub" == restore && "$w" == *W* ]] && worktree=1 ;;
      *) [[ "$BROAD_PATHSPEC" == *" $w "* ]] && broad="$w" ;;
    esac
  done
  [[ -n "$broad" && "$sub" != switch ]] || return 0
  [[ "$sub" == restore && $staged -eq 1 && $worktree -eq 0 ]] && return 0
  hh_block "git $sub with the pathspec '$broad' discards every uncommitted change under it irrecoverably." \
    "Restore specific files by path, or commit the work on a branch first."
}

# -----------------------------------------------------------------------------
# shared-index sweeps
# -----------------------------------------------------------------------------

# is_shared_main_checkout <dir>: <dir> is in the MAIN working tree (its git dir is
# the common dir, so not a linked worktree) of this project's repository (the same
# common dir as CLAUDE_PROJECT_DIR, when that is set).
is_shared_main_checkout() {
  local d out gd common inside proj
  d="$(hh_nearest_existing_ancestor "$1")"
  [[ -n "$d" ]] || return 1
  out="$( (unset GIT_DIR GIT_WORK_TREE; git -C "$d" rev-parse --path-format=absolute \
      --git-dir --git-common-dir --is-inside-work-tree) 2>/dev/null )" || return 1
  { read -r gd; read -r common; read -r inside; } <<< "$out"
  [[ -n "$gd" && "$gd" == "$common" && "$inside" == true ]] || return 1
  if [[ -n "$PROJECT_ROOT" ]]; then
    proj="$(hh_git_common_dir "$PROJECT_ROOT")"
    [[ -n "$proj" && "$proj" == "$(hh_canon_path "$common")" ]] || return 1
  fi
  return 0
}

# A redirection word is never a pathspec. `>/dev/null` and `2>&1` carry their
# target; a bare `>` or `<<` takes the next word.
RE_REDIR='^[0-9]*[<>]'
RE_REDIR_BARE='^[0-9]*(<<<|<<-?|>>|>\||<>|<|>)$'

add_sweep_reason() {   # add_sweep_reason <args after `add`>
  local -a args=("$@") paths=()
  local k w all=0 fromfile=0 ddash=0
  for (( k = 0; k < ${#args[@]}; k++ )); do
    w="${args[k]}"
    if [[ $w =~ $RE_REDIR ]]; then [[ $w =~ $RE_REDIR_BARE ]] && k=$((k + 1)); continue; fi
    if (( ddash == 1 )); then paths+=("$w"); continue; fi
    case "$w" in
      --) ddash=1 ;;
      --all|--update|--no-ignore-removal) all=1 ;;
      --pathspec-from-file) fromfile=1; k=$((k + 1)) ;;
      --pathspec-from-file=*) fromfile=1 ;;
      --*) ;;
      -*) [[ "$w" == *[Au]* ]] && all=1 ;;
      *) paths+=("$w") ;;
    esac
  done
  for w in ${paths[@]+"${paths[@]}"}; do
    if [[ "$BROAD_PATHSPEC" == *" $w "* ]]; then
      printf "git add '%s' stages every change in the tree" "$w"; return 0
    fi
  done
  if (( all == 1 && ${#paths[@]} == 0 && fromfile == 0 )); then
    printf 'git add -A/--all/-u with no pathspec stages every change in the tree'
  fi
  return 0
}

commit_sweep_reason() {   # commit_sweep_reason <args after `commit`>
  local -a args=("$@") paths=()
  local k j c w all=0 include=0 dry=0 fromfile=0 ddash=0
  for (( k = 0; k < ${#args[@]}; k++ )); do
    w="${args[k]}"
    if [[ $w =~ $RE_REDIR ]]; then [[ $w =~ $RE_REDIR_BARE ]] && k=$((k + 1)); continue; fi
    if (( ddash == 1 )); then paths+=("$w"); continue; fi
    case "$w" in
      --) ddash=1 ;;
      --all) all=1 ;;
      --include) include=1 ;;
      --dry-run) dry=1 ;;
      --pathspec-from-file) fromfile=1; k=$((k + 1)) ;;
      --pathspec-from-file=*) fromfile=1 ;;
      --message|--file|--reuse-message|--reedit-message|--fixup|--squash|--author|--date|--template|--cleanup|--trailer)
        k=$((k + 1)) ;;
      --*) ;;
      -?*)
        # A short cluster. m F C c t take a value (the rest of the cluster, else the
        # next word), so `-am msg` is -a plus -m, while `-mall` is a message.
        for (( j = 1; j < ${#w}; j++ )); do
          c="${w:j:1}"
          case "$c" in
            a) all=1 ;;
            i) include=1 ;;
            m|F|C|c|t) (( j + 1 < ${#w} )) || k=$((k + 1)); break ;;
            S|u) break ;;
          esac
        done ;;
      *) paths+=("$w") ;;
    esac
  done
  (( dry == 1 )) && return 0
  if (( all == 1 )); then
    printf 'git commit -a/--all stages and commits every tracked change in the tree'; return 0
  fi
  if (( include == 1 )); then
    printf 'git commit -i/--include commits the whole index plus the named paths'; return 0
  fi
  for w in ${paths[@]+"${paths[@]}"}; do
    if [[ "$BROAD_PATHSPEC" == *" $w "* ]]; then
      printf "git commit with the pathspec '%s' commits every change in the tree" "$w"; return 0
    fi
  done
  if (( ${#paths[@]} == 0 && fromfile == 0 )); then
    printf 'git commit with no pathspec commits the whole index'
  fi
  return 0
}

check_shared_index() {   # check_shared_index <add|commit> <dir> <args...>
  local sub="$1" dir="$2" why="" k
  shift 2
  case "$sub" in
    add) why="$(add_sweep_reason "$@")" ;;
    commit) why="$(commit_sweep_reason "$@")" ;;
  esac
  [[ -n "$why" ]] || return 0
  for (( k = 0; k < HH_CI; k++ )); do
    [[ "${HH_WORDS[k]}" == GIT_INDEX_FILE=* ]] && return 0
  done
  is_shared_main_checkout "$dir" || return 0
  hh_block "$why. In the shared main checkout ($dir) every session uses one index, so this commits whatever other live sessions have staged or edited." \
    "Name your files: git add <file>... && git commit -m \"<msg>\" -- <file>..., or do the work in a linked worktree (git worktree add)."
}

# -----------------------------------------------------------------------------
# dispatcher
# -----------------------------------------------------------------------------

HH_EFF_DIR="$BASE_DIR"
EFF_TOP=""
set_eff_top() {
  EFF_TOP="$( (unset GIT_DIR GIT_WORK_TREE; git -C "$(hh_nearest_existing_ancestor "$HH_EFF_DIR")" rev-parse --show-toplevel) 2>/dev/null || true)"
  [[ -z "$EFF_TOP" ]] || EFF_TOP="$(hh_canon_path "$EFF_TOP")"
}
set_eff_top

mapfile -t SEGMENTS < <(hh_command_segments "$HH_COMMAND")
for seg in ${SEGMENTS[@]+"${SEGMENTS[@]}"}; do
  hh_split_words "$seg"
  hh_cmd_index || continue
  cmd_word="${HH_WORDS[HH_CI]}"
  case "${cmd_word##*/}" in
    cd|pushd) hh_track_cd; set_eff_top ;;
    git) check_git ;;
    rm) check_rm ;;
    find) check_find ;;
  esac
done

exit 0
