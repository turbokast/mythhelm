#!/usr/bin/env bash
# arm-main-push.sh: the arming command for guard-main-push.sh and guard-publish.sh.
#
#   scripts/harness/arm-main-push.sh --reason "<why>"              arm a direct push to main
#   scripts/harness/arm-main-push.sh --operator "<why>"            the same, audited as an operator override
#   scripts/harness/arm-main-push.sh --publish --reason "<why>"    arm publishing (releases, v* tags, settings)
#   scripts/harness/arm-main-push.sh --publish --operator "<why>"
#   scripts/harness/arm-main-push.sh --disarm                      end every window of this session
#   scripts/harness/arm-main-push.sh --publish --disarm            end the publish window only
#
# The guards WITNESS this command in the Bash tool call and write the session's
# sentinel themselves, stamped with the session id of that call. This script
# cannot know the session id, so it never writes a sentinel. It validates the
# arguments exactly as the guards do, and reports the windows it finds. A window
# lasts 30 minutes.
#
# Run outside Claude Code there is no hook, so nothing is armed; nothing needs to
# be, because the guards bind only agent tool calls.

set -euo pipefail

usage() {
  sed -n '3,9p' "$0" | sed 's/^# \{0,1\}//' >&2
  exit 1
}

KIND=main-push
DISARM=0
OPERATOR=false
REASON=""
REASON_SET=0

while (( $# > 0 )); do
  case "$1" in
    --publish) KIND=publish ;;
    --disarm) DISARM=1 ;;
    --reason|--operator)
      [[ "$1" == --operator ]] && OPERATOR=true
      REASON_SET=1
      REASON="${2-}"
      (( $# > 1 )) && shift ;;
    -h|--help) usage ;;
    *) echo "arm-main-push: unknown argument: $1" >&2; usage ;;
  esac
  shift
done

if (( DISARM == 1 )); then
  if (( REASON_SET == 1 )); then
    echo "arm-main-push: --disarm takes no --reason or --operator." >&2
    usage
  fi
elif [[ -z "${REASON//[[:space:]]/}" || "$REASON" == -* ]]; then
  echo "arm-main-push: a reason is required: --reason \"<why>\" or --operator \"<why>\"." >&2
  usage
fi

data_dir() {
  local base="${CLAUDE_PROJECT_DIR:-$PWD}" common
  common="$(git -C "$base" rev-parse --path-format=absolute --git-common-dir 2>/dev/null)" || common=""
  if [[ -n "$common" ]]; then
    printf '%s/.claude/data' "$(dirname "$common")"
  else
    printf '%s/.claude/data' "$base"
  fi
}

# list_windows <kind>: prints each unexpired window; sets FRESH=1 when one was
# armed in the last 10 seconds (by the guard witnessing this very call).
FRESH=0
list_windows() {
  local kind="$1" f epoch age now reason operator
  now="$(date +%s)"
  command -v jq >/dev/null 2>&1 || return 0
  shopt -s nullglob
  for f in "$DATA_DIR/$kind"-arm-*.json; do
    epoch="$(jq -r '.armed_at_epoch // 0' "$f" 2>/dev/null)" || epoch=0
    [[ "$epoch" =~ ^[0-9]+$ ]] || epoch=0
    age=$(( now - epoch ))
    (( age >= 0 && age <= 1800 )) || continue
    reason="$(jq -r '.reason // ""' "$f" 2>/dev/null)" || reason=""
    operator="$(jq -r '.operator // false' "$f" 2>/dev/null)" || operator=false
    echo "    $kind: armed ${age}s ago, operator=${operator}, reason: ${reason}"
    (( age < 10 )) && FRESH=1
  done
  shopt -u nullglob
}

DATA_DIR="$(data_dir)"

if (( DISARM == 1 )); then
  if [[ "$KIND" == publish ]]; then
    echo "arm-main-push: disarm requested for this session's publish window."
  else
    echo "arm-main-push: disarm requested for every window of this session."
  fi
  echo "  windows still armed in this repository (any session):"
  list_windows main-push
  list_windows publish
  exit 0
fi

label="direct push to main"
[[ "$KIND" == publish ]] && label="publishing"
note=""
[[ "$OPERATOR" == true ]] && note=" (operator override)"
echo "arm-main-push: ${label} window requested for this session (30 minutes)."
echo "  reason: ${REASON}${note}"
list_windows "$KIND"
if (( FRESH == 0 )); then
  echo "  note: no guard witnessed this call, so nothing is armed. Outside Claude Code"
  echo "        the guards do not apply and nothing needs arming."
fi
exit 0
