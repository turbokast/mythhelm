#!/usr/bin/env bash
# guard-autonomy.sh: PreToolUse hook (Bash; Edit|Write). Keeps the autonomy grant
# the maintainer's, keeps the delivery run's state files script-written, and holds a
# granted session to what the grant permits.
#
# Always blocked (Bash, command position only; quoted text and heredoc bodies pass):
#   (a) `autonomy.sh grant|renew` and `autonomy.py grant|renew`, run directly or
#       through python: only the maintainer grants or renews autonomy, from their own
#       terminal (`!` commands the maintainer types never reach this hook).
#       revoke, status, check, merge-check and pm-sync-check pass.
#   (b) any command other than a reader naming the grant or its audit log
#       (autonomy-grant.json, autonomy-audit.jsonl), or the run's INTENT.md,
#       RUN-LOG.md or QUESTIONS.md, except delivery.py, which writes those three.
#       Edit and Write of the same files are blocked too. Only the script stamps
#       RUN-LOG lines with the clock and keeps them append-only, and only the
#       maintainer answers a question.
#   (c) systemctl enable|start|link|reenable naming the heartbeat unit: installing the
#       unattended timer is the maintainer's.
#
# Blocked in a session the grant binds (a subagent carries its parent's session id,
# so the session's workers are bound too):
#   (d) the arming script (scripts/harness/arm-main-push.sh), so a granted session
#       never pushes main, publishes, tags or releases;
#   (e) `gh pr merge`, unless it names one pull request by number, uses --squash and
#       --match-head-commit <sha>, and `autonomy.py merge-check` recorded a ready
#       verdict for that pull request at that exact head under this grant in the last
#       15 minutes. --auto, --admin, --merge and --rebase are refused. When the grant
#       has expired, every merge is refused until the maintainer renews or revokes it.
#   A grant file that is unreadable or out of bounds binds no session it can name, so
#   (d) and (e) then block in every session until the maintainer revokes it.
#
# Residuals: a command inside a script file, an alias, a non-literal command word, a
# pseudo-terminal wrapped around the grant command, and a `gh api` merge call (which
# guard-publish.sh blocks as a gh api write). The grant is a file the same user can
# write; this hook stops the ordinary agent path, not an adversary.
#
# Exit 0 allows. Exit 2 blocks with the BLOCK stanza. Fails closed when it cannot parse
# a payload that names what it guards.

set -euo pipefail
export PATH="${PATH:-/usr/local/bin:/usr/bin:/bin}"

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hook-helpers.sh
. "$HOOK_DIR/hook-helpers.sh"

INPUT="$(cat)"
GUARDED_FILES=" autonomy-grant.json autonomy-audit.jsonl INTENT.md RUN-LOG.md QUESTIONS.md "
MERGE_TTL=900

file_block() {
  {
    echo "BLOCK: guard-autonomy"
    echo "File: $1"
    echo "Detail: $2"
    echo "Fix: $3"
  } >&2
  exit 2
}

guarded_name() {  # guarded_name <path>: 0 when the path is a file this hook guards
  local base parent
  base="$(hh_basename "$1")"
  parent="$(hh_basename "$(dirname "$1")")"
  case "$base" in
    autonomy-grant.json|autonomy-audit.jsonl) [[ "$parent" == data ]] ;;
    INTENT.md|RUN-LOG.md|QUESTIONS.md) [[ "$parent" == orchestration ]] ;;
    *) return 1 ;;
  esac
}

tool=""
if command -v "${HOOK_JQ_PROBE:-jq}" >/dev/null 2>&1; then
  tool="$(printf '%s' "$INPUT" | jq -r '.tool_name // empty' 2>/dev/null)" || tool=""
fi
case "$tool" in
  Edit|Write|MultiEdit|NotebookEdit)
    path="$(printf '%s' "$INPUT" | jq -r '.tool_input.file_path // .tool_input.notebook_path // empty' 2>/dev/null)" || path=""
    if [[ -n "$path" ]] && guarded_name "$path"; then
      file_block "$path" \
        "the autonomy grant and its audit log are the maintainer's, and the delivery run's INTENT, RUN-LOG and QUESTIONS files are written only by scripts/orchestration/delivery.py, which stamps each entry with the clock and keeps the log append-only." \
        "Use scripts/orchestration/delivery.py (item, run, log, question), or scripts/orchestration/autonomy.sh revoke|status. Granting, renewing and answering questions are the maintainer's (knowledge/autonomy.md)."
    fi
    exit 0 ;;
esac

hh_load_bash_payload guard-autonomy \
  'autonomy|intent\.md|run-log|questions\.md|arm-main-push|systemctl|(^|[^a-z0-9_-])gh([^a-z0-9_-]|$)' <<< "$INPUT" || exit 0

DATA_DIR="$(hh_data_dir "${HH_CWD:-}" "${CLAUDE_PROJECT_DIR:-}" "$PWD")"
hh_grant_state "$DATA_DIR" "$HH_SID" || true
READERS=" cat less more head tail jq grep egrep rg wc ls stat diff file "

# script_word <name>: the index of <name> run directly or as python's script, else -1.
script_word() {
  local name="$1" cmd k
  cmd="$(hh_basename "${HH_WORDS[HH_CI]}")"
  if [[ "$cmd" == "$name" ]]; then echo "$HH_CI"; return; fi
  if [[ "$cmd" =~ ^(python(3(\.[0-9]+)?)?|bash|sh)$ ]]; then
    for (( k = HH_CI + 1; k < ${#HH_WORDS[@]}; k++ )); do
      [[ "$(hh_basename "${HH_WORDS[k]}")" == "$name" ]] && { echo "$k"; return; }
    done
  fi
  echo -1
}

check_merge() {
  local k w pr="" sha="" squash=0
  for (( k = 1; k < ${#GH_ARGS[@]}; k++ )); do
    w="${GH_ARGS[k]}"
    case "$w" in
      --squash|-s) squash=1 ;;
      --match-head-commit) sha="${GH_ARGS[k+1]:-}"; k=$((k + 1)) ;;
      --match-head-commit=*) sha="${w#*=}" ;;
      --auto|--admin|--merge|-m|--rebase|-r)
        hh_block "a session under an autonomy grant merges only with --squash, after a recorded merge check; $w is not permitted under a grant." \
          "Run python3 scripts/orchestration/autonomy.py merge-check --pr <n> --spec <spec> --task <N>, then gh pr merge <n> --squash --match-head-commit <sha>." ;;
      -R|--repo|-t|--subject|-b|--body|-F|--body-file|-A|--author-email) k=$((k + 1)) ;;
      -*) ;;
      *) [[ -z "$pr" ]] && pr="$w" ;;
    esac
  done
  [[ "$pr" =~ /pull/([0-9]+)/?$ ]] && pr="${BASH_REMATCH[1]}"
  if [[ ! "$pr" =~ ^[0-9]+$ || $squash -eq 0 || ! "$sha" =~ ^[0-9a-f]{40}$ ]]; then
    hh_block "under an autonomy grant a merge names its pull request by number, squashes, and pins the exact head that was checked (--match-head-commit), so what merges is what the merge check saw." \
      "Run python3 scripts/orchestration/autonomy.py merge-check --pr <n> --spec <spec> --task <N> (or --lifecycle, --finalize, --fix), then the gh pr merge <n> --squash --match-head-commit <sha> it prints."
  fi
  local now found
  now="$(date +%s)"
  found="$(jq -rc --arg g "$HH_GRANT_ID" --argjson pr "$pr" --arg h "$sha" --argjson now "$now" --argjson ttl "$MERGE_TTL" \
    'select(.event == "merge-ready" and .grant_id == $g and .pr == $pr and .head == $h
            and ($now - (.at_epoch // 0)) >= 0 and ($now - (.at_epoch // 0)) <= $ttl) | .spec' \
    "$DATA_DIR/autonomy-audit.jsonl" 2>/dev/null | tail -n 1)" || found=""
  if [[ -z "$found" ]]; then
    hh_block "no ready merge check is recorded for pull request #$pr at head ${sha:0:12} under this grant in the last $((MERGE_TTL / 60)) minutes. Under an autonomy grant a pull request merges only when it is green, has no unresolved thread, passes the leak check, stays inside its task's scope and belongs to a granted spec." \
      "Run python3 scripts/orchestration/autonomy.py merge-check --pr $pr --spec <spec> --task <N> (or --lifecycle, --finalize, --fix); merge only if it prints verdict=ready."
  fi
  jq -cn --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --arg g "$HH_GRANT_ID" --arg s "$HH_SID" --argjson pr "$pr" \
    --arg h "$sha" --arg spec "$found" \
    '{ts:$ts,event:"merge-allowed",grant_id:$g,session_id:$s,pr:$pr,head:$h,spec:$spec}' \
    >> "$DATA_DIR/autonomy-audit.jsonl" 2>/dev/null || true
}

invalid_block() {
  hh_block "the autonomy grant file is unreadable or out of bounds, so it cannot say which session it binds; until the maintainer revokes it, no session merges or arms a push." \
    "Ask the maintainer to run scripts/orchestration/autonomy.sh revoke (and grant again if they want the run to continue)."
}

while IFS= read -r seg; do
  hh_split_words "$seg"
  (( ${#HH_WORDS[@]} > 0 )) || continue
  hh_cmd_index || continue
  cmd="$(hh_basename "${HH_WORDS[HH_CI]}")"

  # (a) granting and renewing are the maintainer's.
  for name in autonomy.sh autonomy.py; do
    at="$(script_word "$name")"
    if (( at >= 0 )) && [[ "${HH_WORDS[at+1]:-}" =~ ^(grant|renew)$ ]]; then
      hh_block "only the maintainer grants or renews autonomy: it lets a session run and merge unattended, so it is armed from the maintainer's own terminal, never by an agent." \
        "Ask the maintainer to run it themselves (inside Claude Code: ! scripts/orchestration/autonomy.sh ${HH_WORDS[at+1]} ...). You may run autonomy.sh status, or revoke."
    fi
  done

  # (b) the grant, its audit log and the run's state files. delivery.py counts as a
  # reader here: it is the writer, and only a redirection around it is refused.
  reader=0
  [[ "$READERS" == *" $cmd "* ]] && reader=1
  (( $(script_word delivery.py) >= 0 )) && reader=1
  for (( k = HH_CI + 1; k < ${#HH_WORDS[@]}; k++ )); do
    w="${HH_WORDS[k]}"
    target="${w##*>}"
    [[ -n "$target" ]] || continue
    [[ "$GUARDED_FILES" == *" $(hh_basename "$target") "* ]] || continue
    prev="${HH_WORDS[k-1]}"
    if [[ "$w" == *'>'* || "$prev" =~ ^[0-9]*('>'|'>>'|'>|'|'&>')$ || $reader -eq 0 ]]; then
      hh_block "$(hh_basename "$target") is written only by its script: the grant and its audit log by the maintainer's autonomy.sh, the run's INTENT, RUN-LOG and QUESTIONS by delivery.py, which stamps each entry with the clock and keeps the log append-only." \
        "Use python3 scripts/orchestration/delivery.py item|run|log|question, or autonomy.sh status|revoke. Reading these files is fine."
    fi
  done

  # (c) installing the heartbeat timer.
  if [[ "$cmd" == systemctl ]]; then
    verb="" unit=0
    for (( k = HH_CI + 1; k < ${#HH_WORDS[@]}; k++ )); do
      w="${HH_WORDS[k]}"
      [[ -z "$verb" && "$w" =~ ^(enable|start|restart|link|reenable)$ ]] && verb="$w"
      [[ "$w" == *mythhelm-heartbeat* ]] && unit=1
    done
    if [[ -n "$verb" && $unit -eq 1 ]]; then
      hh_block "the delivery heartbeat starts unattended sessions on a timer; installing or starting it is the maintainer's decision." \
        "Show the maintainer the install commands in scripts/orchestration/systemd/mythhelm-heartbeat@.timer and let them run them."
    fi
  fi

  [[ "$HH_GRANT_STATE" =~ ^(live|expired|invalid)$ ]] || continue

  # (d) a granted session never arms a push to main or a publish.
  if (( $(script_word arm-main-push.sh) >= 0 )); then
    [[ "$HH_GRANT_STATE" == invalid ]] && invalid_block
    hh_block "this session runs under an autonomy grant, which never pushes main, publishes, tags or releases." \
      "Leave the push or release for the maintainer: file it with python3 scripts/orchestration/delivery.py question, and continue with other work."
  fi

  # (e) merges.
  if [[ "$cmd" == gh ]]; then
    GH_ARGS=()
    for (( k = HH_CI + 1; k < ${#HH_WORDS[@]}; k++ )); do
      w="${HH_WORDS[k]}"
      if (( ${#GH_ARGS[@]} == 0 )); then
        case "$w" in
          -R|--repo) k=$((k + 1)); continue ;;
          -*) continue ;;
        esac
      fi
      GH_ARGS+=("$w")
    done
    if [[ "${GH_ARGS[0]:-}" == pr && "${GH_ARGS[1]:-}" == merge ]]; then
      GH_ARGS=("${GH_ARGS[@]:1}")
      [[ "$HH_GRANT_STATE" == invalid ]] && invalid_block
      if [[ "$HH_GRANT_STATE" == expired ]]; then
        hh_block "this session's autonomy grant has expired, so it may not merge." \
          "Stop and report; the maintainer renews the grant (autonomy.sh renew) or revokes it before merging resumes."
      fi
      check_merge
    fi
  fi
done < <(hh_command_segments "$HH_COMMAND")

exit 0
