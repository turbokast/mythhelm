#!/usr/bin/env bash
# guard-blocking-ask.sh: PreToolUse hook (AskUserQuestion). Blocks a synchronous
# question when nobody is there to answer it:
#
#   * the session runs under a live autonomy grant (scripts/orchestration/autonomy.py):
#     the maintainer has said the run proceeds without them; or
#   * the session was started unattended: MYTHHELM_NONINTERACTIVE=1 in its
#     environment, which scripts/orchestration/heartbeat.sh sets for the sessions it
#     starts.
#
# A blocked question stalls the whole session until someone happens to look, and most
# such questions have an answer the agent already recommends. The block names the two
# sanctioned routes instead: take the recommended default when the decision is inside
# the run's authority and record it, or file the question for the maintainer with
# `delivery.py question`, park only the items it affects, and carry on with
# independent work.
#
# Every other session is untouched: the maintainer asked to be asked there.
#
# Fail-open: with no jq, an unreadable payload or an unreadable grant it allows, since
# a guard that cannot read its inputs must never swallow a question.

set -euo pipefail

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hook-helpers.sh
. "$HOOK_DIR/hook-helpers.sh"

INPUT="$(cat)"
command -v jq >/dev/null 2>&1 || exit 0
row="$(printf '%s' "$INPUT" | jq -r '[(.tool_name // ""), (.session_id // ""), (.cwd // "")] | @tsv' 2>/dev/null)" || exit 0
IFS=$'\t' read -r tool sid cwd <<< "$row"
[[ "$tool" == AskUserQuestion ]] || exit 0

why=""
if [[ "${MYTHHELM_NONINTERACTIVE:-}" == 1 ]]; then
  why="this session was started unattended (MYTHHELM_NONINTERACTIVE=1), so nobody is watching to answer."
elif [[ -n "$sid" ]]; then
  data="$(hh_data_dir "$cwd" "${CLAUDE_PROJECT_DIR:-}" "$PWD")"
  if hh_grant_state "$data" "$sid"; then
    why="this session runs under the maintainer's autonomy grant (until $HH_GRANT_UNTIL): the run proceeds without them, and a synchronous question would stall it until someone looks."
  fi
fi
[[ -n "$why" ]] || exit 0

headers="$(printf '%s' "$INPUT" | jq -r '[.tool_input.questions[]?.header // empty] | join(", ")' 2>/dev/null)" || headers=""
{
  echo "BLOCK: guard-blocking-ask"
  echo "Command: AskUserQuestion${headers:+ ($headers)}"
  echo "Detail: $why"
  echo "Fix: If the decision is within the run's authority, take your recommended default and record it with python3 scripts/orchestration/delivery.py log. Otherwise file it: python3 scripts/orchestration/delivery.py question --title ... --context ... --default ... --items <ids>, park only those items (delivery.py item <id> --stage parked --note Q-<n>), and continue with independent work. If nothing independent remains, end your turn with a final line 'AWAITING MAINTAINER: <reason>'."
} >&2
exit 2
