#!/usr/bin/env bash
# approve.sh: the maintainer's side of the product approval queue. Run it from your
# own terminal; agents cannot (guard-product-write.sh blocks every agent call of it).
#
#   scripts/orchestration/approve.sh list                  requests and their state
#   scripts/orchestration/approve.sh show <id>             a request's summary and diff
#   scripts/orchestration/approve.sh approve <id> [--apply] [--yes]
#                                                          sign it; --apply also writes the file
#   scripts/orchestration/approve.sh reject <id> [--yes]   decline it
#   scripts/orchestration/approve.sh audit                 verify every signed decision
#   scripts/orchestration/approve.sh init-key              create the signing key (once)
#
# An approval binds one product file's current content and the exact content the
# agent proposed, and releases one write. approve refuses a request whose file has
# changed since it was filed, or whose proposed content was altered.
#
# The work is done by approvals.py beside this script (its header documents the
# ledger, the key and the residuals). approve, reject and init-key refuse to run
# without an interactive terminal.
#
# Exit codes: 0 success, 1 refused, 2 usage error.

set -euo pipefail

usage() {
  sed -n '5,10p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//' >&2
  exit 2
}

case "${1:-}" in
  list|show|approve|reject|audit|init-key) ;;
  *) usage ;;
esac

command -v python3 >/dev/null 2>&1 || { echo "approve.sh: python3 3.10+ is required" >&2; exit 2; }
exec python3 "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/approvals.py" "$@"
