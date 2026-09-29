# shellcheck shell=bash
# shellcheck disable=SC2034  # HH_* globals are set here for the sourcing hooks
# hook-helpers.sh: shared functions for the guard hooks.
#
# SOURCED, never executed. It defines functions and constants only: no `set`, no
# output, no side effects at source time, so it inherits the sourcing hook's shell
# options. Every function is safe under `set -euo pipefail`.
#
# Sections:
#   1. Payload and block helpers   hh_load_bash_payload, hh_block
#   2. Paths and repositories      hh_canon_path, hh_nearest_existing_ancestor,
#                                  hh_git_common_dir, hh_data_dir, hh_join_dir
#   3. Command tokenizer           hh_command_segments, hh_split_words, hh_cmd_index,
#                                  hh_inner_payload, hh_basename
#   4. git and cd                  hh_git_parse, hh_track_cd
#   5. Arming sentinels            hh_arm_parse, hh_sentinel_*, hh_audit
#
# The tokenizer is the load-bearing piece. Guards match COMMAND POSITION only,
# never substrings of the raw text: a quoted argument or heredoc body is data
# (`git commit -m "never git push --force"` is a commit), while wrappers,
# `bash -c`, `eval`, chains and substitutions are seen through.

# -----------------------------------------------------------------------------
# 1. Payload and block helpers
# -----------------------------------------------------------------------------

# hh_load_bash_payload <guard-name> <prefilter-ere>
#   Reads the PreToolUse payload from stdin and sets HH_GUARD, HH_INPUT,
#   HH_COMMAND, HH_CWD and HH_SID. Returns 1 when there is nothing to check (no
#   command, or the prefilter does not match), so the caller exits 0.
#   A guard that cannot parse fails CLOSED: when bash is older than 4, jq is
#   missing (HOOK_JQ_PROBE names the probe, a test seam), or the payload is not
#   JSON, it blocks if the raw payload matches the prefilter, and allows otherwise.
#   The prefilter only decides whether to do work; it never blocks on its own.
hh_load_bash_payload() {
  HH_GUARD="$1"
  local prefilter="$2"
  HH_INPUT="$(cat)"
  HH_COMMAND=""
  HH_CWD=""
  HH_SID=""
  local lower
  lower="$(hh_prefilter_text "$HH_INPUT")"

  if (( BASH_VERSINFO[0] < 4 )); then
    [[ "$lower" =~ $prefilter ]] || return 1
    hh_block "bash ${BASH_VERSION} is too old for ${HH_GUARD} (bash 4+ is required), so it cannot parse this command, and the payload names a command it guards. A blocking guard fails closed when it cannot parse." \
      "Install bash 4 or newer and put it first on PATH (on macOS: brew install bash), then retry."
  fi
  if ! command -v "${HOOK_JQ_PROBE:-jq}" >/dev/null 2>&1; then
    [[ "$lower" =~ $prefilter ]] || return 1
    hh_block "jq is not installed, so ${HH_GUARD} cannot parse this payload, and the payload names a command it guards. A blocking guard fails closed when it cannot parse." \
      "Install jq (apt install jq / brew install jq) from your own terminal, then retry."
  fi
  if ! HH_COMMAND="$(printf '%s' "$HH_INPUT" | jq -r '.tool_input.command // empty' 2>/dev/null)"; then
    HH_COMMAND=""
    [[ "$lower" =~ $prefilter ]] || return 1
    hh_block "the hook payload is not valid JSON and names a command ${HH_GUARD} guards, so it cannot be classified." \
      "Retry the command. If this persists, report the malformed payload."
  fi
  [[ -n "$HH_COMMAND" ]] || return 1
  lower="$(hh_prefilter_text "$HH_COMMAND")"
  [[ "$lower" =~ $prefilter ]] || return 1
  HH_CWD="$(printf '%s' "$HH_INPUT" | jq -r '.cwd // empty' 2>/dev/null)" || HH_CWD=""
  HH_SID="$(printf '%s' "$HH_INPUT" | jq -r '.session_id // empty' 2>/dev/null)" || HH_SID=""
  return 0
}

# hh_prefilter_text <text>: lowercased, JSON \n/\t escapes as spaces, quotes and
# backslashes dropped, so g"it" and g\it still match a prefilter.
hh_prefilter_text() {
  printf '%s' "$1" | sed 's/\\[nrt]/ /g' | tr -d '\\"'"'" | tr '[:upper:]' '[:lower:]'
}

# hh_block <detail> <fix>: prints the four-line block stanza on stderr, exits 2.
hh_block() {
  {
    echo "BLOCK: ${HH_GUARD:-guard}"
    echo "Command: ${HH_COMMAND:-<unparsed payload>}"
    echo "Detail: $1"
    echo "Fix: $2"
  } >&2
  exit 2
}

# -----------------------------------------------------------------------------
# 2. Paths and repositories
# -----------------------------------------------------------------------------

# hh_canon_path <path>: the physical path of an existing directory, else the input
# with trailing slashes removed. Two hooks naming one tree spell it identically.
hh_canon_path() {
  local p="${1:-}" resolved=""
  [[ -n "$p" ]] || return 0
  if [[ -d "$p" ]]; then
    resolved="$( (cd "$p" && pwd -P) 2>/dev/null )" || resolved=""
  fi
  if [[ -n "$resolved" ]]; then
    printf '%s' "$resolved"
    return 0
  fi
  while [[ "$p" == */ && "$p" != "/" ]]; do p="${p%/}"; done
  printf '%s' "$p"
}

# hh_nearest_existing_ancestor <path>: the closest existing directory at or above
# <path> ("/" at worst). A command can name a directory it creates later.
hh_nearest_existing_ancestor() {
  local p="${1:-}" next
  [[ -n "$p" ]] || return 0
  while [[ "$p" != "/" && ! -d "$p" ]]; do
    next="${p%/*}"
    if [[ -z "$next" || "$next" == "$p" ]]; then p="/"; break; fi
    p="$next"
  done
  printf '%s' "$p"
}

# hh_git_common_dir <dir>: the canonical `.git` common directory of the repository
# containing <dir>, shared by the main checkout and every linked worktree. Empty
# when <dir> is not inside a repository.
hh_git_common_dir() {
  local dir="${1:-}" common=""
  [[ -n "$dir" && -d "$dir" ]] || return 0
  common="$( (unset GIT_DIR GIT_WORK_TREE; cd "$dir" && git rev-parse --path-format=absolute --git-common-dir) 2>/dev/null )" || common=""
  [[ -n "$common" ]] || return 0
  hh_canon_path "$common"
}

# hh_data_dir <dir>...: the harness state directory, <main checkout>/.claude/data.
# The first <dir> inside a repository decides, so a linked worktree and its main
# checkout share one state directory. Falls back to <first dir>/.claude/data.
hh_data_dir() {
  local d common first=""
  for d in "$@"; do
    [[ -n "$d" ]] || continue
    [[ -n "$first" ]] || first="$d"
    common="$(hh_git_common_dir "$d")"
    if [[ -n "$common" ]]; then
      printf '%s/.claude/data' "$(dirname "$common")"
      return 0
    fi
  done
  printf '%s/.claude/data' "${first:-$PWD}"
}

# hh_join_dir <base> <target>: <target> resolved against <base>. A target the guard
# cannot read literally (a variable, a substitution, ~user) leaves <base>.
hh_join_dir() {
  local base="$1" t="${2:-}"
  # shellcheck disable=SC2088  # the literal, unexpanded tilde spelling
  case "$t" in
    '~') t="${HOME:-}" ;;
    '~/'*) t="${HOME:-}/${t#\~/}" ;;
  esac
  if [[ -z "$t" || "$t" == *'$'* || "$t" == *'`'* || "$t" == '~'* ]]; then
    printf '%s' "$base"
  elif [[ "$t" == /* ]]; then
    printf '%s' "$t"
  else
    printf '%s' "${base%/}/$t"
  fi
}

# -----------------------------------------------------------------------------
# 3. Command tokenizer
# -----------------------------------------------------------------------------
#
# hh_command_segments <command> [depth]
#   Emits one command segment per line, in execution order:
#   * Heredoc bodies are dropped as data, unless the heredoc feeds a shell
#     (`bash <<EOF`, `eval`, `$VAR <<EOF`), in which case the body is command text.
#   * The text is split at every unquoted separator: ; | & && || ( ) newline, a
#     brace group's { and }, and comments are dropped.
#   * A command substitution, $( ... ) or backticks, inside or outside double
#     quotes, is emitted as its own segments BEFORE the command that contains it,
#     and stands in that command as the non-literal word $__SUBST__. So
#     `git commit -m "$(cat <<EOF ... EOF)" -- file` keeps its pathspec.
#   * Quotes and backslashes are removed as the shell removes them, so each segment
#     holds the words the command receives. Whitespace inside a quoted span becomes
#     $HH_SENT, so a quoted argument stays ONE word and never reaches command
#     position. A newline inside quotes is part of the word.
#   * ${...} expansions stay whole inside their word.
#   * The payload of `bash -c '<text>'`, `sh -c`, `eval <words>`, and a multi-word
#     command word (`ssh host "git push"`), is emitted and then re-segmented,
#     recursively, up to depth 3.
#
# hh_split_words <segment>: sets HH_WORDS to the segment's words ($HH_SENT restored
#   to spaces inside each word).
# hh_cmd_index: sets HH_CI to the index of the command word in HH_WORDS, skipping
#   assignments, shell keywords and wrappers (env, command, sudo, timeout, nohup,
#   xargs, ...) with their options and operands. Returns 1 when there is none.
# hh_basename <word>: the last path component (/usr/bin/git -> git).

HH_SENT=$'\001'
HH_SUBST="\$__SUBST__"

hh_strip_heredocs() {
  printf '%s' "$1" | LC_ALL=C awk '
    # scan(line): sets TERM to the heredoc delimiter the line opens (if any) and
    # DETECT to the line text outside quotes and comments.
    function scan(line,   n, i, c, prev, qc, arith, rest) {
      TERM = ""; DETECT = ""
      n = length(line); i = 1; prev = ""; qc = ""; arith = 0
      while (i <= n) {
        c = substr(line, i, 1)
        if (qc != "") {
          # A substitution inside double quotes is command context again, so
          # `-m "$(cat <<EOF` opens a heredoc.
          if (qc == "\"" && c == "$" && substr(line, i + 1, 1) == "(") { qc = ""; prev = "("; i += 2; continue }
          if (qc == "\"" && c == "`") { qc = ""; prev = c; i++; continue }
          if (c == qc) qc = ""
          prev = c; i++; continue
        }
        if (c == "\\") { prev = ""; i += 2; continue }
        if (c == "\047" || c == "\"") { qc = c; prev = c; i++; continue }
        if (c == "#" && (prev == "" || prev == " " || prev == "\t" || prev == ";" || prev == "&" || prev == "|" || prev == "(")) return
        if (substr(line, i, 3) == "$((") { arith++; prev = "("; i += 3; continue }
        if (arith > 0 && substr(line, i, 2) == "))") { arith--; prev = ")"; i += 2; continue }
        if (arith == 0 && TERM == "" && substr(line, i, 2) == "<<" && substr(line, i + 2, 1) != "<" && prev != "<") {
          rest = substr(line, i + 2)
          sub(/^-/, "", rest)
          sub(/^[[:space:]]+/, "", rest)
          sub(/^["\047]/, "", rest)
          if (match(rest, /^[A-Za-z_][A-Za-z0-9_]*/)) TERM = substr(rest, 1, RLENGTH)
        }
        DETECT = DETECT c
        prev = c; i++
      }
    }
    # shellish(): the heredoc on this line feeds a shell, so its body is commands.
    function shellish(   cnt, arr, k) {
      # A shell name is a word bounded by space, a separator, a redirection or a
      # parenthesis: `bash<<EOF` and `tee >(bash) <<EOF` both run the body.
      if (DETECT ~ /(^|[[:space:]|;&()<>])([^[:space:]|;&()<>]*\/)?(bash|sh|zsh|dash|ksh|ash|mksh|busybox|fish|csh|tcsh)([[:space:]|;&()<>]|$)/) return 1
      if (DETECT ~ /(^|[[:space:]|;&()<>])(eval|source|\.)([[:space:]|;&()<>]|$)/) return 1
      cnt = split(DETECT, arr, /[[:space:]]+/)
      for (k = 1; k <= cnt; k++) {
        if (arr[k] == "" || arr[k] ~ /^[A-Za-z_][A-Za-z0-9_]*=/) continue
        return (arr[k] ~ /^\$\{?[A-Za-z_][A-Za-z0-9_]*\}?$/) ? 1 : 0
      }
      return 0
    }
    { L[NR] = $0 }
    END {
      i = 1
      while (i <= NR) {
        scan(L[i])
        term = TERM
        keep = (term != "" && shellish()) ? 1 : 0
        print L[i]
        i++
        if (term == "") continue
        for (j = i; j <= NR; j++) { t = L[j]; sub(/^[[:space:]]+/, "", t); if (t == term) break }
        if (j > NR) continue
        if (keep) for (k = i; k < j; k++) print L[k]
        i = j + 1
      }
    }
  '
}

hh_segment() {
  printf '%s' "$1" | LC_ALL=C awk -v SENT="$HH_SENT" -v SUBST="$HH_SUBST" '
    function flush() { if (buf != "") print buf; buf = "" }
    function add(ch) { buf = buf ((ch == " " || ch == "\t") ? SENT : ch) }
    function open_subst(kind) {
      if (kind == "$(") buf = substr(buf, 1, length(buf) - 1)
      depth++; K[depth] = kind; B[depth] = buf; Q[depth] = q
      buf = ""; q = ""; prev = ""
    }
    function close_subst() {
      flush()
      buf = B[depth] SUBST; q = Q[depth]; depth--; prev = ")"
    }
    BEGIN { q = ""; depth = 0; buf = ""; esc = 0 }
    {
      n = length($0); prev = ""
      for (i = 1; i <= n; i++) {
        c = substr($0, i, 1)
        if (esc) { add(c); esc = 0; prev = c; continue }
        if (c == "\\" && q != "\047") { esc = 1; continue }
        if (q == "\047") {
          if (c == q) { q = ""; prev = c; continue }
          add(c); prev = c; continue
        }
        if (c == "{" && prev == "$") {
          # ${...}: copy through the matching brace as part of the word.
          add(c); lvl = 1
          for (i++; i <= n && lvl > 0; i++) {
            c = substr($0, i, 1)
            if (c == "{") lvl++
            if (c == "}") lvl--
            add(c)
          }
          i--; prev = "}"; continue
        }
        if (q == "\"") {
          if (c == "`") { open_subst("`"); continue }
          if (c == "(" && prev == "$") { open_subst("$("); continue }
          if (c == q) { q = ""; prev = c; continue }
          add(c); prev = c; continue
        }
        atword = (prev == "" || prev == " " || prev == "\t")
        if (c == "#" && atword) break
        if (c == "\047" || c == "\"") { q = c; prev = c; continue }
        if (c == "(" && prev == "$") { open_subst("$("); continue }
        if (depth > 0 && c == ")" && K[depth] == "$(") { close_subst(); continue }
        if (depth > 0 && c == "`" && K[depth] == "`") { close_subst(); continue }
        if (c == "`") { open_subst("`"); continue }
        if (depth > 0 && c == "(") { depth++; K[depth] = "("; flush(); prev = ""; continue }
        if (depth > 0 && c == ")" && K[depth] == "(") { depth--; flush(); prev = ""; continue }
        nx = substr($0, i + 1, 1)
        if (c == "{" && atword && (nx == "" || nx == " " || nx == "\t")) { flush(); prev = ""; continue }
        if (c == "}" && atword) { flush(); prev = ""; continue }
        if (index(";|&()", c) > 0) { flush(); prev = ""; continue }
        buf = buf c; prev = c
      }
      if (esc) { esc = 0 }
      else if (q != "") { buf = buf SENT }
      else { flush() }
    }
    END { flush(); while (depth > 0) { if (B[depth] != "") print B[depth]; depth-- } }
  '
}

hh_basename() {
  local w="${1:-}"
  printf '%s' "${w##*/}"
}

hh_split_words() {
  local seg="${1:-}" w
  local -a raw=()
  HH_WORDS=()
  read -r -a raw <<< "$seg" || true
  for w in ${raw[@]+"${raw[@]}"}; do
    HH_WORDS+=("${w//$HH_SENT/ }")
  done
  return 0
}

HH_SKIP_WRAPPERS=" command builtin exec env time nohup sudo doas su runuser nice ionice taskset chrt stdbuf timeout setsid flock unbuffer script xargs watch strace parallel ssh if then elif else do while until ! "

# hh_takes_value <wrapper> <option>: the option consumes the next word.
hh_takes_value() {
  case "$1:$2" in
    sudo:-u|sudo:-g|sudo:-U|sudo:-C|sudo:-h|sudo:-p|sudo:-r|sudo:-t|sudo:-T) return 0 ;;
    doas:-u|doas:-C) return 0 ;;
    su:-c|su:-s|su:-g|runuser:-u|runuser:-c|runuser:-s|runuser:-g) return 0 ;;
    env:-u|env:-C|env:-S) return 0 ;;
    nice:-n|ionice:-c|ionice:-n|ionice:-p|ionice:-P|ionice:-u) return 0 ;;
    chrt:-p|taskset:-p|taskset:-c) return 0 ;;
    stdbuf:-i|stdbuf:-o|stdbuf:-e) return 0 ;;
    timeout:-k|timeout:-s|flock:-w|flock:-E|unbuffer:-p) return 0 ;;
    script:-c|script:-f|script:-l|script:-o|script:-t) return 0 ;;
    time:-o|time:-f|exec:-a) return 0 ;;
    xargs:-a|xargs:-E|xargs:-I|xargs:-i|xargs:-L|xargs:-n|xargs:-P|xargs:-s|xargs:-d) return 0 ;;
    watch:-n|watch:-d) return 0 ;;
    strace:-o|strace:-e|strace:-p|strace:-s|strace:-E|strace:-P|strace:-u) return 0 ;;
    parallel:-j|parallel:-N|parallel:-S|parallel:-a) return 0 ;;
    ssh:-o|ssh:-i|ssh:-p|ssh:-l|ssh:-F|ssh:-J|ssh:-c|ssh:-E|ssh:-L|ssh:-R|ssh:-D|ssh:-S|ssh:-W|ssh:-m|ssh:-b|ssh:-e) return 0 ;;
  esac
  return 1
}

# hh_takes_positional <wrapper>: the wrapper takes one operand before the command
# (timeout DURATION, flock FILE, ssh HOST, ...).
hh_takes_positional() {
  case "$1" in
    timeout|flock|taskset|chrt|su|script|ssh) return 0 ;;
  esac
  return 1
}

hh_cmd_index() {
  local n=${#HH_WORDS[@]} i=0 w wb need_pos
  HH_CI=-1
  while (( i < n )); do
    w="${HH_WORDS[i]}"
    if [[ -z "$w" || "$w" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]]; then
      i=$((i + 1)); continue
    fi
    wb="${w##*/}"
    if [[ "$HH_SKIP_WRAPPERS" == *" $wb "* ]]; then
      i=$((i + 1))
      need_pos=0
      hh_takes_positional "$wb" && need_pos=1
      while (( i < n )); do
        w="${HH_WORDS[i]}"
        if [[ "$w" == -* && "$w" != "-" ]]; then
          i=$((i + 1))
          if hh_takes_value "$wb" "$w" && (( i < n )); then i=$((i + 1)); fi
          continue
        fi
        if [[ "$w" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]]; then i=$((i + 1)); continue; fi
        if (( need_pos == 1 )); then need_pos=0; i=$((i + 1)); continue; fi
        break
      done
      continue
    fi
    HH_CI=$i
    return 0
  done
  return 1
}

# hh_inner_payload <segment>: sets HH_PAYLOAD to the command text a shell in this
# segment executes (`bash -c P`, `sh -lc P`, `eval W...`, or a multi-word command
# word), or to "". Runs in the caller's shell: no fork per segment.
hh_inner_payload() {
  local -a HH_WORDS=()
  local HH_CI=-1 cmd k w n
  HH_PAYLOAD=""
  hh_split_words "$1"
  hh_cmd_index || return 0
  n=${#HH_WORDS[@]}
  cmd="${HH_WORDS[HH_CI]}"
  if [[ "$cmd" == *" "* ]]; then
    HH_PAYLOAD="${HH_WORDS[*]:HH_CI}"
    return 0
  fi
  case "${cmd##*/}" in
    bash|sh|zsh|dash|ksh|ash|mksh|busybox)
      for (( k = HH_CI + 1; k < n; k++ )); do
        w="${HH_WORDS[k]}"
        if [[ "$w" =~ ^-[A-Za-z]*c[A-Za-z]*$ ]]; then
          (( k + 1 < n )) && HH_PAYLOAD="${HH_WORDS[k+1]}"
          return 0
        fi
        case "$w" in
          -o|+o|-O|+O|--rcfile|--init-file) k=$((k + 1)) ;;
          -*|+*) ;;
          *) return 0 ;;
        esac
      done ;;
    eval)
      (( HH_CI + 1 < n )) && HH_PAYLOAD="${HH_WORDS[*]:HH_CI+1}" ;;
  esac
  return 0
}

hh_command_segments() {
  local cmd="${1:-}" depth="${2:-0}" raw segs seg
  raw="$(hh_strip_heredocs "$cmd")"
  segs="$(hh_segment "$raw")"
  while IFS= read -r seg; do
    [[ -n "${seg//[[:space:]]/}" ]] || continue
    printf '%s\n' "$seg"
    if (( depth < 3 )); then
      hh_inner_payload "$seg"
      [[ -n "$HH_PAYLOAD" ]] && hh_command_segments "$HH_PAYLOAD" $((depth + 1))
    fi
  done <<< "$segs"
  return 0
}

# -----------------------------------------------------------------------------
# 4. git and cd
# -----------------------------------------------------------------------------

# hh_git_parse <dir>: for a segment whose command word is git (HH_WORDS/HH_CI),
# skips git's global options and sets:
#   HH_GIT_SUB   the subcommand ("" when there is none)
#   HH_GIT_ARGS  the words after the subcommand
#   HH_GIT_DIR   <dir> moved by each literal `-C <path>`
# Returns 1 when there is no subcommand.
hh_git_parse() {
  local n=${#HH_WORDS[@]} k w
  HH_GIT_SUB=""
  HH_GIT_ARGS=()
  HH_GIT_DIR="${1:-}"
  for (( k = HH_CI + 1; k < n; k++ )); do
    w="${HH_WORDS[k]}"
    case "$w" in
      -C) HH_GIT_DIR="$(hh_join_dir "$HH_GIT_DIR" "${HH_WORDS[k+1]:-}")"; k=$((k + 1)); continue ;;
      -c|--git-dir|--work-tree|--namespace|--super-prefix|--config-env|--attr-source) k=$((k + 1)); continue ;;
      -*) continue ;;
    esac
    HH_GIT_SUB="$w"
    (( k + 1 < n )) && HH_GIT_ARGS=("${HH_WORDS[@]:k+1}")
    return 0
  done
  return 1
}

# hh_track_cd: for a `cd`/`pushd` segment (HH_WORDS/HH_CI), moves HH_EFF_DIR to its
# literal target (a bare `cd` goes to $HOME). `cd -` and non-literal targets leave
# it. Segments are flat, so a cd inside ( ... ) still moves it for later segments.
hh_track_cd() {
  local k w
  for (( k = HH_CI + 1; k < ${#HH_WORDS[@]}; k++ )); do
    w="${HH_WORDS[k]}"
    case "$w" in
      -) return 0 ;;
      -*) continue ;;
    esac
    HH_EFF_DIR="$(hh_join_dir "$HH_EFF_DIR" "$w")"
    return 0
  done
  [[ -n "${HOME:-}" ]] && HH_EFF_DIR="$HOME"
  return 0
}

# -----------------------------------------------------------------------------
# 5. Arming sentinels
# -----------------------------------------------------------------------------
#
# A guarded action runs only inside an ARMED WINDOW. Arming is hook-witnessed: the
# guard sees `scripts/harness/arm-main-push.sh --reason "<why>"` in command
# position on its own stdin and writes the sentinel itself, stamped with the
# session_id of that payload. The script only validates and reports; it cannot
# attribute a session, so it never writes a sentinel.
#
#   <data>/<kind>-arm-<session>.json   one per armed session and kind
#   <data>/guard-audit.jsonl           every arm, disarm, allow and block
#
# <kind> is `main-push` or `publish`. A sentinel is valid for HH_ARM_TTL seconds
# from its armed_at_epoch (fixed, not configurable); a future-dated one is invalid.
# A window allows several actions until it expires or is disarmed.

HH_ARM_TTL=1800

# hh_arm_parse: for a segment in HH_WORDS/HH_CI, returns 0 when it runs the arming
# script and sets HH_ARM_KIND (main-push|publish), HH_ARM_ACTION (arm|disarm|invalid),
# HH_ARM_REASON and HH_ARM_OPERATOR (true|false). The script runs as its own command
# word or as the first operand of bash/sh. Arguments are validated as the script
# validates them, so an invocation the script rejects never arms.
hh_arm_parse() {
  local n=${#HH_WORDS[@]} k="$HH_CI" w disarm=0 bad=0 reason_set=0
  local word="${HH_WORDS[k]}"
  case "${word##*/}" in
    bash|sh)
      k=$((k + 1))
      while (( k < n )); do
        case "${HH_WORDS[k]}" in
          -o|+o|-O|+O|--rcfile|--init-file) k=$((k + 2)) ;;
          -*|+*) k=$((k + 1)) ;;
          *) break ;;
        esac
      done
      (( k < n )) || return 1 ;;
  esac
  word="${HH_WORDS[k]}"
  [[ "${word##*/}" == arm-main-push.sh ]] || return 1
  HH_ARM_KIND=main-push
  HH_ARM_ACTION=invalid
  HH_ARM_REASON=""
  HH_ARM_OPERATOR=false
  for (( k = k + 1; k < n; k++ )); do
    w="${HH_WORDS[k]}"
    case "$w" in
      --publish) HH_ARM_KIND=publish ;;
      --disarm) disarm=1 ;;
      --reason|--operator)
        [[ "$w" == --operator ]] && HH_ARM_OPERATOR=true
        reason_set=1
        HH_ARM_REASON="${HH_WORDS[k+1]:-}"
        k=$((k + 1)) ;;
      *) bad=1 ;;
    esac
  done
  (( bad == 0 )) || return 0
  if (( disarm == 1 )); then
    (( reason_set == 0 )) && HH_ARM_ACTION=disarm
    return 0
  fi
  if [[ -n "${HH_ARM_REASON//[[:space:]]/}" && "$HH_ARM_REASON" != -* ]]; then
    HH_ARM_ACTION=arm
  fi
  return 0
}

# hh_sentinel_file <data-dir> <kind> <session>
hh_sentinel_file() {
  local sid="${3//[^A-Za-z0-9._-]/_}"
  printf '%s/%s-arm-%s.json' "$1" "$2" "$sid"
}

# hh_audit <data-dir> <kind> <event> <reason> <operator>: appends one audit row.
# Best effort: an audit failure never changes a guard's verdict.
hh_audit() {
  local dir="$1" kind="$2" event="$3" reason="$4" operator="${5:-false}"
  [[ "$operator" == true ]] || operator=false
  mkdir -p "$dir" 2>/dev/null || return 0
  jq -cn --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --arg sid "${HH_SID:-}" \
    --arg guard "${HH_GUARD:-}" --arg kind "$kind" --arg ev "$event" \
    --arg cmd "${HH_COMMAND:0:200}" --arg reason "$reason" --argjson op "$operator" \
    '{schema_version:1,ts:$ts,session_id:$sid,guard:$guard,kind:$kind,event:$ev,command:$cmd,reason:$reason,operator:$op}' \
    >> "$dir/guard-audit.jsonl" 2>/dev/null || true
  return 0
}

# hh_sentinel_arm <data-dir> <kind> <reason> <operator>: writes this session's
# sentinel. Without a session id nothing is armed.
hh_sentinel_arm() {
  local dir="$1" kind="$2" reason="$3" operator="$4" file
  [[ -n "${HH_SID:-}" ]] || return 0
  mkdir -p "$dir" 2>/dev/null || return 0
  file="$(hh_sentinel_file "$dir" "$kind" "$HH_SID")"
  jq -n --arg sid "$HH_SID" --arg kind "$kind" --arg at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --argjson ep "$(date +%s)" --argjson ttl "$HH_ARM_TTL" --arg reason "$reason" \
    --argjson op "$operator" \
    '{schema_version:1,session_id:$sid,kind:$kind,armed_at:$at,armed_at_epoch:$ep,ttl_seconds:$ttl,reason:$reason,operator:$op}' \
    > "$file" 2>/dev/null || return 0
  hh_audit "$dir" "$kind" arm "$reason" "$operator"
}

# hh_sentinel_disarm <data-dir> <kind>: removes this session's sentinel.
hh_sentinel_disarm() {
  local dir="$1" kind="$2"
  [[ -n "${HH_SID:-}" ]] && rm -f "$(hh_sentinel_file "$dir" "$kind" "$HH_SID")" 2>/dev/null
  hh_audit "$dir" "$kind" disarm "" false
}

# hh_sentinel_status <data-dir> <kind>: sets HH_ARM_STATUS to valid, expired,
# absent or unattributable (no session id); returns 0 only when valid. An expired
# or future-dated sentinel is removed. Other sessions' stale sentinels are swept.
hh_sentinel_status() {
  local dir="$1" kind="$2" file f epoch now
  now="$(date +%s)"
  shopt -s nullglob
  for f in "$dir/$kind"-arm-*.json; do
    epoch="$(jq -r '.armed_at_epoch // 0' "$f" 2>/dev/null)" || epoch=0
    [[ "$epoch" =~ ^[0-9]+$ ]] || epoch=0
    (( now - epoch > HH_ARM_TTL || now - epoch < 0 )) || continue
    [[ -n "${HH_SID:-}" && "$f" == "$(hh_sentinel_file "$dir" "$kind" "$HH_SID")" ]] && continue
    rm -f "$f" 2>/dev/null || true
  done
  shopt -u nullglob
  if [[ -z "${HH_SID:-}" ]]; then HH_ARM_STATUS=unattributable; return 1; fi
  file="$(hh_sentinel_file "$dir" "$kind" "$HH_SID")"
  if [[ ! -f "$file" ]]; then HH_ARM_STATUS=absent; return 1; fi
  epoch="$(jq -r '.armed_at_epoch // 0' "$file" 2>/dev/null)" || epoch=0
  [[ "$epoch" =~ ^[0-9]+$ ]] || epoch=0
  if (( now - epoch >= 0 && now - epoch <= HH_ARM_TTL )); then
    HH_ARM_STATUS=valid
    return 0
  fi
  HH_ARM_STATUS=expired
  rm -f "$file" 2>/dev/null || true
  return 1
}
