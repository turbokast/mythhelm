#!/usr/bin/env bash
# heartbeat.sh: OPTIONAL. Resumes the granted delivery session when it has stopped and
# the run still has work an agent can do. Meant for a systemd user timer the
# maintainer installs (scripts/orchestration/systemd/); nothing in the repository
# installs or enables it.
#
#   scripts/orchestration/heartbeat.sh                 one tick: resume if eligible
#   scripts/orchestration/heartbeat.sh --dry-run       decide and print, never launch
#   scripts/orchestration/heartbeat.sh --print-install the maintainer's install commands
#
# A tick launches only when every check passes, in this order; each refusal is the
# normal outcome and exits 0:
#   1. a valid autonomy grant is live                      (else: no-grant / expired)
#   2. the delivery run is active with actionable work     (else: nothing-actionable)
#   3. no live session holds the run:delivery lane, which
#      /deliver-backlog takes for its whole run            (else: session-live)
#   4. the Claude Code CLI is on PATH                      (else: no-claude)
# It then runs, from the main checkout,
#   MYTHHELM_NONINTERACTIVE=1 claude -p --resume <grant's session> "<resume prompt>"
# resuming the granted session, since a new session would fall outside the grant.
# One tick buys one print-mode turn plus the continues continue-run.sh grants; it is
# periodic resumption, not a daemon. The argv is built as an array and never carries
# --dangerously-skip-permissions: an unattended session keeps every permission gate.
# MYTHHELM_NONINTERACTIVE makes guard-blocking-ask.sh refuse synchronous questions.
#
# Every decision is appended to .claude/data/autonomy-audit.jsonl (event heartbeat).
# Environment: MYTHHELM_CLAUDE names the CLI binary (default: claude).
#
# Exit codes: 0 launched, refused or dry run; the CLI's own code when it launched;
# 2 usage error.

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
mode=tick
case "${1:-}" in
  "") ;;
  --dry-run) mode=dry ;;
  --print-install) mode=install ;;
  *) sed -n '7,9p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//' >&2; exit 2 ;;
esac

top="$(git -C "$HERE" rev-parse --path-format=absolute --git-common-dir 2>/dev/null)" \
  || { echo "heartbeat.sh: not inside a git repository" >&2; exit 2; }
top="$(dirname "$top")"

if [[ "$mode" == install ]]; then
  inst="$(systemd-escape --path "$top" 2>/dev/null || echo '<systemd-escape --path of this checkout>')"
  cat <<EOF
# Run these yourself, from your own terminal. Stop the timer with:
#   systemctl --user disable --now 'mythhelm-heartbeat@$inst.timer'
mkdir -p ~/.config/systemd/user
cp '$top/scripts/orchestration/systemd/mythhelm-heartbeat@.service' \\
   '$top/scripts/orchestration/systemd/mythhelm-heartbeat@.timer' ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now 'mythhelm-heartbeat@$inst.timer'
EOF
  exit 0
fi

cd "$top"
data="$top/.claude/data"
mkdir -p "$data"
log() {  # log <decision> <reason>
  jq -cn --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" --arg d "$1" --arg r "$2" --argjson dry "$([[ $mode == dry ]] && echo true || echo false)" \
    '{ts:$ts,event:"heartbeat",decision:$d,reason:$r,dry_run:$dry}' >> "$data/autonomy-audit.jsonl" 2>/dev/null || true
}
refuse() { log refused "$1"; echo "heartbeat: refused: $1"; exit 0; }

command -v jq >/dev/null 2>&1 || { echo "heartbeat: jq is required" >&2; exit 0; }
state="$(python3 "$HERE/autonomy.py" status --json 2>/dev/null || echo '{}')"
grant_state="$(printf '%s' "$state" | jq -r '.state // "none"')"
sid="$(printf '%s' "$state" | jq -r '.grant.session_id // ""')"
case "$grant_state" in
  live) ;;
  none) refuse no-grant ;;
  *) refuse "grant-$grant_state" ;;
esac
[[ "$sid" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$ ]] || refuse grant-session-malformed
python3 "$HERE/delivery.py" actionable >/dev/null 2>&1 || refuse nothing-actionable
if python3 "$HERE/lanes.py" status run:delivery >/dev/null 2>&1; then refuse session-live; fi
claude_bin="${MYTHHELM_CLAUDE:-claude}"
command -v "$claude_bin" >/dev/null 2>&1 || refuse no-claude

prompt='Resume the delivery run: run scripts/orchestration/status.sh, then continue /deliver-backlog from its resume step. If nothing is left that an agent can do, end with a final line: AWAITING MAINTAINER: <reason>'
argv=("$claude_bin" -p --resume "$sid" "$prompt")
if [[ "$mode" == dry ]]; then
  log would-launch "session $sid"
  printf '%q ' MYTHHELM_NONINTERACTIVE=1 "${argv[@]}"
  echo
  exit 0
fi
log launched "session $sid"
MYTHHELM_NONINTERACTIVE=1 exec "${argv[@]}"
