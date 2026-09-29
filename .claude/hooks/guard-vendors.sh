#!/usr/bin/env bash
# guard-vendors.sh: PreToolUse hook (Bash; Edit|Write). Every call to an optional
# paid vendor goes through its sanctioned wrapper, and only the contributor opts a
# checkout in to paying for one.
#
# Blocked (Bash, command position only; quoted text, heredoc bodies and search
# patterns that merely mention a vendor pass):
#   (a) the Codex or Muse CLI itself, in any spelling the tokenizer can see: a bare
#       or absolute-path `codex`/`muse` (or a `codex-*`/`muse-bin*` binary), and the
#       package runners `npx`/`bunx`/`pnpx`/`npm exec`/`pnpm dlx`/`yarn dlx` naming
#       a codex package. A direct call skips the envelope in scripts/vendors/:
#       opt-in, the clean-worktree target, redaction, the read-only posture, the
#       daily caps and the call record. `--version` and `--help` are not exempt;
#       `python3 scripts/vendors/vendors.py status` reports availability instead.
#       Lookups (`command -v`, `type`, `which`) pass.
#   (b) `vendors.py enable` (also as `python3 -m vendors enable`): opting in spends the contributor's subscription, so the
#       contributor runs it from their own terminal (hooks never bind it).
#   (c) writing .claude/data/vendor-policy.local.json (the opt-in file): an Edit or
#       Write of it, a shell redirection into it, or any command naming it other
#       than a reader (cat, less, head, tail, jq, grep, rg, wc, ls, stat, diff, and
#       read-only git subcommands such as status, log, show and diff).
# The sanctioned wrappers pass: scripts/codex/codex-consult.sh,
# scripts/codex/codex-review.sh, scripts/vendors/muse-consult.sh,
# scripts/vendors/codex-implement.sh, scripts/vendors/muse-implement.sh and the
# Python envelope they call.
#
# One guard covers both CLI vendors: they share one envelope, one policy and one
# tokenizer pass, so two guards would duplicate the walk and the inventory row
# without adding a rule. The classifier vendor is an HTTP API, not a CLI; its only
# client is the envelope, and no hook ever calls it.
#
# Residuals: a vendor call inside a script file, an alias or function defined
# outside the session, a non-literal command word ($CLI), a runtime that loads the
# vendor's JavaScript by a path the guard cannot read, and direct HTTP to a vendor
# API. The envelope's opt-in still gates everything that goes through it.
#
# Exit 0 allows. Exit 2 blocks with the BLOCK/Command (or File)/Detail/Fix stanza.
# Fails closed when it cannot parse a payload that names what it guards.

set -euo pipefail
export PATH="${PATH:-/usr/local/bin:/usr/bin:/bin}"

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hook-helpers.sh
. "$HOOK_DIR/hook-helpers.sh"

INPUT="$(cat)"
LOCAL_POLICY=vendor-policy.local.json
WRAPPERS="scripts/codex/codex-consult.sh, scripts/codex/codex-review.sh, scripts/vendors/muse-consult.sh, scripts/vendors/codex-implement.sh or scripts/vendors/muse-implement.sh"

file_block() {
  {
    echo "BLOCK: guard-vendors"
    echo "File: $1"
    echo "Detail: $2"
    echo "Fix: $3"
  } >&2
  exit 2
}

# A payload jq cannot read reaches hh_load_bash_payload, which blocks it when it
# names anything this guard covers (the opt-in file included) and allows the rest.
tool=""
if command -v "${HOOK_JQ_PROBE:-jq}" >/dev/null 2>&1; then
  tool="$(printf '%s' "$INPUT" | jq -r '.tool_name // empty' 2>/dev/null)" || tool=""
fi
case "$tool" in
  Edit|Write|MultiEdit|NotebookEdit)
    path="$(printf '%s' "$INPUT" | jq -r '.tool_input.file_path // .tool_input.notebook_path // empty' 2>/dev/null)" || path=""
    if [[ "$(hh_basename "$path")" == "$LOCAL_POLICY" ]]; then
      file_block "$path" \
        "the local vendor policy opts this checkout in to paid vendors and sets their caps; only the contributor changes it." \
        "Ask the contributor to run 'python3 scripts/vendors/vendors.py enable|disable <vendor>' from their own terminal (knowledge/vendors.md)."
    fi
    exit 0 ;;
esac

hh_load_bash_payload guard-vendors 'codex|muse|vendors\.py|vendor-policy\.local' <<< "$INPUT" || exit 0

READERS=" cat less more head tail jq grep egrep rg wc ls stat diff file "
GIT_READERS=" status log show diff ls-files check-ignore blame grep "
CODEX_PKG_RE='^(@openai/codex|codex)(@.*)?$'

block_cli() {
  hh_block "the $1 CLI runs only through its sanctioned wrapper ($WRAPPERS). A direct call skips the vendor envelope: opt-in, the clean-worktree target, redaction, the read-only posture, the daily caps and the call record." \
    "Run the wrapper for the stage you need (knowledge/vendors.md), or 'python3 scripts/vendors/vendors.py status' to see whether the vendor is available."
}

check_package_words() {
  local k w
  for (( k = $1; k < ${#HH_WORDS[@]}; k++ )); do
    w="${HH_WORDS[k]}"
    [[ "$w" == -* ]] && continue
    [[ "$w" =~ $CODEX_PKG_RE ]] && block_cli codex
    return 0
  done
}

while IFS= read -r seg; do
  hh_split_words "$seg"
  (( ${#HH_WORDS[@]} > 0 )) || continue
  [[ "${HH_WORDS[0]}" == command && "${HH_WORDS[1]:-}" =~ ^-[vV]$ ]] && continue
  hh_cmd_index || continue
  cmd="$(hh_basename "${HH_WORDS[HH_CI]}")"

  # (c) the opt-in file: redirections into it, or any non-reader naming it.
  for (( k = HH_CI + 1; k < ${#HH_WORDS[@]}; k++ )); do
    w="${HH_WORDS[k]}"
    [[ "$w" == *"$LOCAL_POLICY"* ]] || continue
    prev="${HH_WORDS[k-1]}"
    reader=0
    [[ "$READERS" == *" $cmd "* ]] && reader=1
    if [[ "$cmd" == git ]] && hh_git_parse "$HH_CWD" && [[ "$GIT_READERS" == *" $HH_GIT_SUB "* ]]; then
      reader=1
    fi
    if [[ "$w" == *'>'* || "$prev" =~ ^[0-9]*('>'|'>>'|'>|'|'&>')$ || $reader -eq 0 ]]; then
      hh_block "this command can write .claude/data/$LOCAL_POLICY, which opts the checkout in to paid vendors and sets their caps; only the contributor changes it." \
        "Ask the contributor to run 'python3 scripts/vendors/vendors.py enable|disable <vendor>' from their own terminal (knowledge/vendors.md)."
    fi
  done

  case "$cmd" in
    *.sh|*.py) ;;
    codex|codex-*|codex.js) block_cli codex ;;
    muse|muse-bin*) block_cli muse ;;
    npx|bunx|pnpx) check_package_words $((HH_CI + 1)) ;;
    npm|pnpm|yarn)
      for (( k = HH_CI + 1; k < ${#HH_WORDS[@]}; k++ )); do
        case "${HH_WORDS[k]}" in
          -*) continue ;;
          exec|x|dlx) check_package_words $((k + 1)) ;;
        esac
        break
      done ;;
  esac

  # (b) opting in: vendors.py enable, run directly or through python.
  script_at=-1
  if [[ "$cmd" == vendors.py ]]; then
    script_at=$HH_CI
  elif [[ "$cmd" =~ ^python(3(\.[0-9]+)?)?$ ]]; then
    # Every word is scanned: an interpreter option can take a value (-X dev,
    # -W ignore), and `-m vendors` runs the module without naming the file.
    for (( k = HH_CI + 1; k < ${#HH_WORDS[@]}; k++ )); do
      w="${HH_WORDS[k]}"
      if [[ "$(hh_basename "$w")" == vendors.py || "$w" == -mvendors \
            || ( "$w" == vendors && "${HH_WORDS[k-1]}" == -m ) ]]; then
        script_at=$k
        break
      fi
    done
  fi
  if (( script_at >= 0 )) && [[ "${HH_WORDS[script_at+1]:-}" == enable ]]; then
    hh_block "opting a checkout in to a paid vendor spends the contributor's subscription, so only the contributor does it." \
      "Ask the contributor to run 'python3 scripts/vendors/vendors.py enable <vendor>' from their own terminal (knowledge/vendors.md). Agents may run 'status' and 'disable'."
  fi
done < <(hh_command_segments "$HH_COMMAND")

exit 0
