#!/usr/bin/env bash
# continue-run.sh: Stop hook. While the maintainer's autonomy grant is live for this
# session and the delivery run still has work an agent can do, it turns the end of a
# turn into "take the next action" instead of an idle stop.
#
# Why a hook: ending a turn is a harness event. A session told in prose to keep going
# unattended still stops after each unit of work and waits for someone to type
# "continue"; only the Stop event can change that.
#
# It continues (exit 2, a directive on stderr) only when ALL of these hold, checked in
# this order, and allows the stop (exit 0) otherwise:
#   1. the last assistant message does not end with a final line
#      `AWAITING MAINTAINER: <reason>`, the agent's named exit, honoured before
#      anything else. The line must be the last non-empty line: a message that only
#      mentions the sentinel is not an exit;
#   2. a valid autonomy grant binds this session and has not expired (hh_grant_state);
#   3. the delivery run is active and has an actionable item
#      (`scripts/orchestration/delivery.py actionable` exits 0): an item whose next
#      step an agent can take now, not one waiting for the maintainer;
#   4. the grant's continue budget (max_continues) is not spent;
#   5. on a stop that follows one of this hook's own continues (stop_hook_active), no
#      more than MAX_CHAIN continues in a row;
#   6. at least MIN_INTERVAL seconds since this hook's last continue, so a session
#      that ends its turn without doing anything cannot spin.
# Never under no grant, another session's grant, or an expired or invalid one.
#
# Every continue, and the stop that spends the budget, is appended to
# <main checkout>/.claude/data/autonomy-audit.jsonl. The counters live beside it in
# autonomy-continue.json, keyed by the grant id, so a new grant starts at zero.
#
# Fail-open: jq or python3 missing, an unreadable payload, grant or run state all allow
# the stop. A broken hook must never hold a session open.

set -euo pipefail

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hook-helpers.sh
. "$HOOK_DIR/hook-helpers.sh"

MAX_CHAIN=3
MIN_INTERVAL="${MYTHHELM_CONTINUE_MIN_INTERVAL:-60}"
[[ "$MIN_INTERVAL" =~ ^[0-9]{1,5}$ ]] || MIN_INTERVAL=60

INPUT="$(cat)"
command -v jq >/dev/null 2>&1 || exit 0
command -v python3 >/dev/null 2>&1 || exit 0
row="$(printf '%s' "$INPUT" | jq -r '[(.session_id // ""), (.cwd // ""), (.stop_hook_active // false | tostring),
                                      (.transcript_path // "")] | @tsv' 2>/dev/null)" || exit 0
IFS=$'\t' read -r sid cwd active transcript <<< "$row"
[[ -n "$sid" ]] || exit 0

# 1. The named exit. The message comes from the payload, else from the transcript's
# newest assistant text.
msg="$(printf '%s' "$INPUT" | jq -r '.last_assistant_message // empty' 2>/dev/null)" || msg=""
if [[ -z "$msg" && -n "$transcript" && -r "$transcript" ]]; then
  msg="$(tail -n 400 "$transcript" | jq -Rc 'fromjson? | select(.type? == "assistant")
          | [.message.content[]? | select(.type? == "text") | .text] | join("\n") | select(length > 0)' 2>/dev/null \
        | tail -n 1 | jq -r '.' 2>/dev/null)" || msg=""
fi
last="$(printf '%s\n' "$msg" | sed -e 's/[[:space:]]*$//' -e '/^$/d' | tail -n 1)"
[[ "$last" =~ ^[[:space:][:punct:]]*AWAITING\ MAINTAINER ]] && exit 0

# 2. A live grant for this session.
root="$(cd "${cwd:-${CLAUDE_PROJECT_DIR:-$PWD}}" 2>/dev/null && git rev-parse --show-toplevel 2>/dev/null)" || exit 0
data="$(hh_data_dir "$root")"
hh_grant_state "$data" "$sid" || exit 0

# 3. Actionable work in the delivery run.
work="$(cd "$root" && python3 "$HOOK_DIR/../../scripts/orchestration/delivery.py" actionable 2>/dev/null)" || exit 0
[[ -n "$work" ]] || exit 0

# 4-6. Budget, chain and interval, counted per grant.
state="$data/autonomy-continue.json"
count=0 chain=0 lastat=0
if [[ -r "$state" ]]; then
  srow="$(jq -r --arg g "$HH_GRANT_ID" 'if .grant_id == $g then "\(.count // 0)\t\(.chain // 0)\t\(.last_epoch // 0)" else "0\t0\t0" end' "$state" 2>/dev/null)" || srow=""
  IFS=$'\t' read -r count chain lastat <<< "$srow"
  [[ "$count" =~ ^[0-9]+$ && "$chain" =~ ^[0-9]+$ && "$lastat" =~ ^[0-9]+$ ]] || { count=0 chain=0 lastat=0; }
fi
now="$(date +%s)"
audit_row() {
  jq -cn --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --arg ev "$1" --arg g "$HH_GRANT_ID" --arg s "$sid" \
    --argjson n "$2" --argjson c "$3" '{ts:$ts,event:$ev,grant_id:$g,session_id:$s,continue_n:$n,chain:$c}' \
    >> "$data/autonomy-audit.jsonl" 2>/dev/null || true
}
if (( count >= HH_GRANT_MAXC )); then
  if (( count == HH_GRANT_MAXC )); then
    audit_row continue-budget-spent "$count" "$chain"
    jq -cn --arg g "$HH_GRANT_ID" --argjson n $((count + 1)) --argjson c "$chain" --argjson t "$lastat" \
      '{grant_id:$g,count:$n,chain:$c,last_epoch:$t}' > "$state" 2>/dev/null || true
  fi
  exit 0
fi
if [[ "$active" == true ]]; then
  (( chain < MAX_CHAIN )) || exit 0
  chain=$((chain + 1))
else
  chain=0
fi
(( now - lastat >= MIN_INTERVAL )) || exit 0

count=$((count + 1))
jq -cn --arg g "$HH_GRANT_ID" --argjson n "$count" --argjson c "$chain" --argjson t "$now" \
  '{grant_id:$g,count:$n,chain:$c,last_epoch:$t}' > "$state" 2>/dev/null || exit 0
audit_row continue "$count" "$chain"

items="$(printf '%s\n' "$work" | head -n 5 | tr '\n' ';' | sed 's/;$//; s/;/; /g')"
cat >&2 <<EOF
CONTINUE ($count/$HH_GRANT_MAXC): the maintainer's autonomy grant is live until $HH_GRANT_UNTIL and the delivery run has actionable work: $items.
Take the next action now; do not wait to be told.
- Lost the thread? Run scripts/orchestration/status.sh, then follow /deliver-backlog from its resume step.
- Waiting on CI or a review? Advance another actionable item meanwhile.
- A decision only the maintainer can make? File it with python3 scripts/orchestration/delivery.py question, park only the items it affects, and continue with the rest.
- Nothing left that an agent can do? End with a final line: AWAITING MAINTAINER: <reason>
EOF
exit 2
