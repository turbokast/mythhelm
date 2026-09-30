#!/usr/bin/env bash
# autonomy.sh: the maintainer's front end to the autonomy grant, the time-boxed,
# scope-limited permission for one Claude Code session to run the delivery loop
# unattended (autonomy.py beside this script does the work; its header documents the
# grant file, the audit log and what a grant permits).
#
#   scripts/orchestration/autonomy.sh grant --hours N --scope MH-3,spec:<name> --reason TEXT
#                                     [--session ID] [--allow-pm-sync] [--no-spec-checkpoint]
#                                     [--max-continues N]
#   scripts/orchestration/autonomy.sh renew --hours N [--reason TEXT]
#   scripts/orchestration/autonomy.sh revoke [--reason TEXT]
#   scripts/orchestration/autonomy.sh status [--json]
#
# grant and renew are the maintainer's: they refuse without an interactive terminal,
# and guard-autonomy.sh blocks every agent call of them. Inside Claude Code, run them
# with the `!` prefix in the session to be granted, which binds that session. Hours
# run from 1 to 24; renew starts a new window of at most 24 hours from now and keeps
# the scope and the session. Anyone may revoke.
#
# Exit codes: those of autonomy.py (0 success, 1 not covered or not ready, 2 refused
# or a usage error).

set -euo pipefail

case "${1:-}" in
  grant|renew|revoke|status|check|merge-check|pm-sync-check) ;;
  *) sed -n '7,12p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//' >&2; exit 2 ;;
esac

command -v python3 >/dev/null 2>&1 || { echo "autonomy.sh: python3 3.10+ is required" >&2; exit 2; }
exec python3 "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/autonomy.py" "$@"
