#!/usr/bin/env bash
# guard-product-write.sh: PreToolUse hook (Bash; Edit|Write). Agents propose product
# changes; maintainers approve them. No agent write reaches product/ without a
# maintainer's signed approval of that exact change.
#
# Edit, Write, MultiEdit and NotebookEdit (decided by approvals.py check-write):
#   * A write to <repository>/product/... is released only by a signed, unconsumed
#     approval in <main checkout>/orchestration/approvals.jsonl whose path, base
#     (the file's current SHA-256) and result (the SHA-256 of the content this call
#     would leave, computed from the payload) all match. The release is recorded as
#     a consumed row, so an approval releases one write. The path is resolved
#     through symlinks first. NotebookEdit is never released. Without a matching
#     approval, a write the maintainer's live autonomy grant pre-approves as a
#     lifecycle sync (--allow-pm-sync: the granted session moving a granted card
#     specced -> implementing -> shipped, its lifecycle-sync decision entries, the
#     regenerated roadmap; scripts/orchestration/autonomy.py) is released and
#     recorded as a preapproved row.
#   * Writes to the approval ledger and to the maintainer's key are blocked.
#
# Bash (command position only; quoted text and heredoc bodies are data):
#   (a) Any call of scripts/orchestration/approve.sh: it is the maintainer's command.
#   (b) approvals.py with any verb other than request, list, show and audit.
#   (c) Any command naming the approval key, including readers.
#   (d) A command that names a path at or under <repository>/product/ (resolved
#       against the payload cwd and literal cd/pushd targets), or runs in such a
#       directory, or names the ledger, unless it only reads: cat, head, tail, less,
#       more, wc, ls, stat, du, grep, egrep, fgrep, rg, ag, diff, cmp, comm, the
#       checksum tools, od, strings, hexdump, echo, printf, pwd, basename, dirname,
#       realpath, readlink, test, [, true, false, jq, column, nl, tac, rev, fold, cut,
#       tr, date, sleep; read-only git subcommands (status, log, show, diff, grep,
#       blame, ls-files, ls-tree, cat-file, rev-parse, describe, shortlog) plus add
#       and commit, which never touch the working tree, without --output or -O; the
#       read-only forms of find, sort, uniq, sed and awk (a closed grammar per tool,
#       below); scripts/pm/pm.py, which drafts to scratch files and refuses to write
#       into product/; and approvals.py with an agent verb. For an interpreter
#       (python, perl, ruby, node, ...) any word holding a standalone "product"
#       token counts as naming product/, since its paths live inside string
#       literals. A redirection into
#       product/ or the ledger blocks whatever the command. There is no approval
#       path through Bash: approved content is written with Write or Edit.
#
# Read-only filter grammar. find: none of -delete, -exec, -execdir, -ok, -okdir,
# -fprint, -fprint0, -fprintf, -fls. sort: none of -o, --output, -T,
# --temporary-directory, --compress-program, nor a short-option cluster holding o
# or T. uniq: at most one operand. sed: only -n, -E, -r, -s, -u, -z, --posix,
# --debug, --sandbox and -e SCRIPT, where every ;-separated command is an optional
# address or range and one of p P = l q Q d D n N g G h H x z, s/../../ with flags
# from gpIiMm0-9, or y/../../ (so w, W, r, R, e, s///w and s///e block). awk: only
# -F and -v, and a program with no >, |, system or @.
#
# Residuals: a write made by a script file, an interpreter or an alias the command
# text does not name; a path built at run time ($VAR, $(...)) or passed through a
# pipe to xargs; a symlink whose name does not lead into product/ (the Edit and Write
# arm resolves symlinks, this arm does not); a directory entered in an earlier Bash
# call when the payload cwd is stale. `cd -` and popd make the directory unknown,
# which blocks later non-reading segments of a command that mentions product. The
# key is an ordinary file of the same user. Pull request review and the product
# check of scripts/ci/lint-agent-harness.sh are the backstop.
#
# Exit 0 allows. Exit 2 blocks with the BLOCK/Command (or File)/Detail/Fix stanza.
# Fails closed when jq, python3 or bash 4 is missing and the payload names product,
# approvals or approve.sh.

set -euo pipefail
export PATH="${PATH:-/usr/local/bin:/usr/bin:/bin}"

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hook-helpers.sh
. "$HOOK_DIR/hook-helpers.sh"
APPROVALS="$HOOK_DIR/../../scripts/orchestration/approvals.py"
GUARDED_RE='product|approvals|approve\.sh'
FIX_PROPOSE="Draft the change with python3 scripts/pm/pm.py <verb> --out <scratch file>, file it with python3 scripts/orchestration/approvals.py request <id> --path product/<file> --proposed <scratch file> --summary '<why>', and continue with other work. A maintainer approves it from their own terminal (scripts/orchestration/approve.sh); then write the approved content with Write or Edit, or the maintainer applies it with --apply."

INPUT="$(cat)"
HH_GUARD=guard-product-write

# fail_closed <why>: blocks when the raw payload names what this guard covers.
fail_closed() {
  if [[ "$(hh_prefilter_text "$INPUT")" =~ $GUARDED_RE ]]; then
    HH_COMMAND="<unparsed payload>"
    hh_block "$1, so this guard cannot decide whether the call writes product/ or the approval ledger, and the payload names them. A blocking guard fails closed." \
      "Install the missing tool (jq, python3 3.10+, bash 4+) from your own terminal, then retry."
  fi
  exit 0
}

(( BASH_VERSINFO[0] >= 4 )) || fail_closed "bash ${BASH_VERSION} is older than 4"
command -v "${HOOK_JQ_PROBE:-jq}" >/dev/null 2>&1 || fail_closed "jq is not installed"
tool="$(printf '%s' "$INPUT" | jq -r '.tool_name // empty' 2>/dev/null)" || fail_closed "the payload is not valid JSON"

case "$tool" in
  Edit|Write|MultiEdit|NotebookEdit)
    command -v "${HOOK_PYTHON_PROBE:-python3}" >/dev/null 2>&1 || fail_closed "python3 is not installed"
    rc=0
    printf '%s' "$INPUT" | python3 "$APPROVALS" check-write || rc=$?
    case "$rc" in
      0|2) exit "$rc" ;;
      *) fail_closed "approvals.py check-write failed (exit $rc)" ;;
    esac ;;
  Bash) ;;
  *) exit 0 ;;
esac

hh_load_bash_payload guard-product-write '.' <<< "$INPUT" || exit 0
CMD_TEXT="$(hh_prefilter_text "$HH_COMMAND")"
START_DIR="${HH_CWD:-${CLAUDE_PROJECT_DIR:-$PWD}}"

# norm_path <absolute path>: the path with . and .. resolved lexically.
norm_path() {
  local part
  local -a parts=() out=()
  IFS=/ read -r -a parts <<< "$1" || true
  for part in ${parts[@]+"${parts[@]}"}; do
    case "$part" in
      ''|.) ;;
      ..) (( ${#out[@]} > 0 )) && unset 'out[${#out[@]}-1]' ;;
      *) out+=("$part") ;;
    esac
  done
  if (( ${#out[@]} == 0 )); then printf '/'; else printf '/%s' "${out[@]}"; fi
}

# in_product <absolute path>: the path is a repository's product directory or lies
# under it (a "product" component whose parent holds .git).
in_product() {
  local part prefix=""
  local -a parts=()
  IFS=/ read -r -a parts <<< "$(norm_path "$1")" || true
  for part in ${parts[@]+"${parts[@]}"}; do
    [[ -n "$part" ]] || continue
    if [[ "$part" == product && -e "$prefix/.git" ]]; then
      return 0
    fi
    prefix="$prefix/$part"
  done
  return 1
}

# classify <word>: sets KIND to product, ledger, key or "" for one path-like word.
classify() {
  local w="$1" abs
  KIND=""
  [[ "$w" == *product* || "$w" == *approvals* ]] || return 0
  if [[ "$(hh_basename "$w")" == approvals.key ]]; then KIND=key; return 0; fi
  abs="$(norm_path "$(hh_join_dir "$EFF_DIR" "$w")")"
  if [[ "$abs" == */orchestration/approvals.jsonl ]]; then KIND=ledger; return 0; fi
  if in_product "$abs"; then KIND=product; fi
  return 0
}

READERS=" cat head tail less more wc ls stat du grep egrep fgrep rg ag diff cmp comm md5sum sha1sum sha256sum sha512sum shasum cksum od strings hexdump echo printf pwd basename dirname realpath readlink test [ true false jq column nl tac rev fold cut tr date sleep "
GIT_READERS=" status log show diff grep blame ls-files ls-tree cat-file rev-parse describe shortlog add commit "
AGENT_VERBS=" request list show audit "
INTERPRETERS=" python python3 perl ruby node deno bun php lua tclsh Rscript "
PRODUCT_TOKEN_RE='(^|[^A-Za-z0-9_.-])product($|/|[^A-Za-z0-9_-])'

SED_ADDR='([0-9]+(~[0-9]+)?|\$|/([^/\\]|\\.)*/[IM]*)'
SED_ADDR2='([0-9]+|\$|\+[0-9]+|~[0-9]+|/([^/\\]|\\.)*/[IM]*)'
SED_CMD='(p|P|=|l|q[0-9]*|Q[0-9]*|d|D|n|N|g|G|h|H|x|z|s/([^/\\]|\\.)*/([^/\\]|\\.)*/[gpIiMm0-9]*|y/([^/\\]|\\.)*/([^/\\]|\\.)*/)'
SED_SAFE_RE="^(${SED_ADDR}(,${SED_ADDR2})?)?[[:space:]]*!?[[:space:]]*${SED_CMD}\$"

sed_script_safe() {
  local piece
  [[ -n "${1//[[:space:]]/}" ]] || return 1
  while IFS= read -r piece; do
    piece="${piece#"${piece%%[![:space:]]*}"}"
    piece="${piece%"${piece##*[![:space:]]}"}"
    [[ -z "$piece" ]] && continue
    [[ "$piece" =~ $SED_SAFE_RE ]] || return 1
  done <<< "$(printf '%s' "$1" | tr ';' '\n')"
  return 0
}

# readonly_filter <tool> <args...>: the closed read-only grammar for one tool.
readonly_filter() {
  local t="$1" w k have_script=0 ops=0
  shift
  local -a a=("$@")
  case "$t" in
    find)
      for w in "$@"; do
        case "$w" in -delete|-exec|-execdir|-ok|-okdir|-fprint|-fprint0|-fprintf|-fls) return 1 ;; esac
      done ;;
    sort)
      for w in "$@"; do
        case "$w" in
          -o|--output|--output=*|-T|--temporary-directory*|--compress-program*) return 1 ;;
          --*) ;;
          -*) [[ "$w" == *o* || "$w" == *T* ]] && return 1 ;;
        esac
      done ;;
    uniq)
      for (( k = 0; k < ${#a[@]}; k++ )); do
        case "${a[k]}" in
          -f|-s|-w) k=$((k + 1)) ;;
          -) ops=$((ops + 1)) ;;
          -*) ;;
          *) ops=$((ops + 1)) ;;
        esac
      done
      (( ops <= 1 )) || return 1 ;;
    sed)
      for (( k = 0; k < ${#a[@]}; k++ )); do
        w="${a[k]}"
        case "$w" in
          -n|--quiet|--silent|-E|-r|--regexp-extended|-s|--separate|-u|--unbuffered|-z|--null-data|--posix|--debug|--sandbox) continue ;;
          -e|--expression) sed_script_safe "${a[k+1]:-}" || return 1; have_script=1; k=$((k + 1)); continue ;;
          --expression=*) sed_script_safe "${w#--expression=}" || return 1; have_script=1; continue ;;
        esac
        [[ "$w" =~ ^-[nErsuz]+$ ]] && continue
        [[ "$w" == -* ]] && return 1
        if (( have_script == 0 )); then
          sed_script_safe "$w" || return 1
          have_script=1
        fi
      done
      (( have_script == 1 )) || return 1 ;;
    awk|gawk|mawk|nawk)
      for (( k = 0; k < ${#a[@]}; k++ )); do
        w="${a[k]}"
        case "$w" in
          -F|-v|--field-separator|--assign) k=$((k + 1)); continue ;;
          -F?*|-v?*|--field-separator=*|--assign=*|--) continue ;;
          -*) return 1 ;;
        esac
        case "$w" in *'>'*|*'|'*|*system*|*'@'*) return 1 ;; esac
        return 0
      done
      return 1 ;;
    *) return 1 ;;
  esac
  return 0
}

# script_word: the index of the script a segment runs (the command word itself, or
# the first operand of an interpreter), or -1.
script_word() {
  local k="$HH_CI" w
  SW=-1
  case "$(hh_basename "${HH_WORDS[k]}")" in
    bash|sh|zsh|dash|python|python3|python3.*)
      for (( k = HH_CI + 1; k < ${#HH_WORDS[@]}; k++ )); do
        w="${HH_WORDS[k]}"
        case "$w" in
          -X|-W|-o|+o|-O|+O) k=$((k + 1)); continue ;;
          -*|+*) continue ;;
        esac
        SW=$k
        return 0
      done ;;
    *) SW=$HH_CI ;;
  esac
  return 0
}

block_bash() { hh_block "$1" "$2"; }

EFF_DIR="$START_DIR"
EFF_UNKNOWN=0
MENTIONS_PRODUCT=0
[[ "$CMD_TEXT" == *product* ]] && MENTIONS_PRODUCT=1
if ! [[ "$CMD_TEXT" =~ $GUARDED_RE ]] && ! in_product "$START_DIR"; then
  exit 0
fi

while IFS= read -r seg; do
  hh_split_words "$seg"
  (( ${#HH_WORDS[@]} > 0 )) || continue
  HH_CI=0
  hh_cmd_index || HH_CI=0
  cmd="$(hh_basename "${HH_WORDS[HH_CI]}")"

  case "$cmd" in
    cd|pushd)
      if [[ "${HH_WORDS[HH_CI+1]:-}" == - ]]; then
        EFF_UNKNOWN=1
      else
        HH_EFF_DIR="$EFF_DIR"
        hh_track_cd
        EFF_DIR="$HH_EFF_DIR"
      fi
      continue ;;
    popd) EFF_UNKNOWN=1; continue ;;
  esac

  # (a), (b): the approval commands.
  script_word
  script=""
  (( SW >= 0 )) && script="$(hh_basename "${HH_WORDS[SW]}")"
  if [[ "$script" == approve.sh ]]; then
    block_bash "scripts/orchestration/approve.sh records the maintainer's decisions; an agent never runs it, not even to list or show (use approvals.py list or show)." \
      "Leave the decision to the maintainer: tell them the request id and that they approve it from their own terminal with scripts/orchestration/approve.sh approve <id>. Continue with other work meanwhile."
  fi
  if [[ "$script" == approvals.py ]]; then
    verb="${HH_WORDS[SW+1]:-}"
    if [[ "$AGENT_VERBS" != *" $verb "* ]]; then
      block_bash "approvals.py $verb is not an agent verb: agents may only request, list, show and audit. Approving, rejecting, creating the key and the hook's own check are the maintainer's or the hook's." \
        "File the request (approvals.py request ...) and ask the maintainer to decide it with scripts/orchestration/approve.sh."
    fi
    continue
  fi

  # (c), (d): what the segment names.
  names_product=0 names_ledger=0
  redirect_target=""
  n=${#HH_WORDS[@]}
  for (( k = 0; k < n; k++ )); do
    w="${HH_WORDS[k]}"
    target=""
    if [[ "$w" =~ ^[0-9]*(\>\>?|\>\|)$ ]]; then
      target="${HH_WORDS[k+1]:-}"
    elif [[ "$w" =~ ^[0-9]*(\>\>?|\>\|)(.+)$ ]]; then
      target="${BASH_REMATCH[2]}"
    fi
    if [[ -n "$target" ]]; then
      classify "$target"
      if [[ "$KIND" == product || "$KIND" == ledger || "$KIND" == key ]] \
        || { in_product "$EFF_DIR" && [[ "$target" != /* ]]; }; then
        redirect_target="$target"
      fi
    fi
    # An interpreter's code names paths inside string literals, which are not
    # path-shaped words, so for interpreters a standalone "product" token counts.
    if { [[ "$INTERPRETERS" == *" $cmd "* ]] || [[ "$cmd" =~ ^python[0-9.]+$ ]]; } \
      && [[ "$w" =~ $PRODUCT_TOKEN_RE ]]; then
      names_product=1
    fi
    value="$w"
    [[ "$value" == *=* ]] && value="${value##*=}"
    for v in "$w" "$value"; do
      classify "$v"
      case "$KIND" in
        key)
          block_bash "this command names the maintainer's approval key; an agent never reads or writes it, since holding it would let an agent sign its own approvals." \
            "Leave the key to the maintainer. To see the queue, run python3 scripts/orchestration/approvals.py list." ;;
        ledger) names_ledger=1 ;;
        product) names_product=1 ;;
      esac
    done
  done
  if [[ -n "$redirect_target" ]]; then
    block_bash "this command redirects output into '$redirect_target', which is under product/ or is the approval ledger. Product files change only through an approved request, and the ledger only through approvals.py." \
      "$FIX_PROPOSE"
  fi

  here_product=0
  if in_product "$EFF_DIR" || { (( EFF_UNKNOWN == 1 && MENTIONS_PRODUCT == 1 )); }; then
    here_product=1
  fi
  (( names_product == 1 || names_ledger == 1 || here_product == 1 )) || continue

  args=()
  (( HH_CI + 1 < n )) && args=("${HH_WORDS[@]:HH_CI+1}")
  if [[ "$READERS" == *" $cmd "* ]]; then continue; fi
  if [[ "$script" == pm.py ]]; then continue; fi
  if readonly_filter "$cmd" ${args[@]+"${args[@]}"}; then continue; fi
  if [[ "$cmd" == git ]] && hh_git_parse "$EFF_DIR" && [[ "$GIT_READERS" == *" $HH_GIT_SUB "* ]]; then
    reader=1
    for g in ${HH_GIT_ARGS[@]+"${HH_GIT_ARGS[@]}"}; do
      case "$g" in --output|--output=*|-O*|--open-files-in-pager*) reader=0 ;; esac
    done
    (( reader == 1 )) && continue
  fi

  if (( names_ledger == 1 )); then
    block_bash "this command can write the approval ledger, which records maintainer decisions; only approvals.py writes it." \
      "Read it with cat, jq or python3 scripts/orchestration/approvals.py list; file requests with approvals.py request."
  fi
  block_bash "'$cmd' is not a read-only command, and this segment names a path under product/ or runs inside it. Agents propose product changes; a maintainer approves each one, and approved content is written with Write or Edit, never through Bash." \
    "$FIX_PROPOSE"
done < <(hh_command_segments "$HH_COMMAND")

exit 0
