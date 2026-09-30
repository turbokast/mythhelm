#!/usr/bin/env bash
# lane.sh: advisory locks over single-copy resources shared by concurrent sessions of
# one repository (the main checkout's index, a spec directory, the product files, the
# delivery run). lanes.py beside this script does the work; its header documents the
# ownership token, the dead-holder staleness rule and the files.
#
#   scripts/orchestration/lane.sh token
#   scripts/orchestration/lane.sh acquire <resource> --owner TOKEN [--pid PID] [--note TEXT]
#   scripts/orchestration/lane.sh release <resource> --owner TOKEN
#   scripts/orchestration/lane.sh status <resource>
#   scripts/orchestration/lane.sh list [--json]
#   scripts/orchestration/lane.sh reap
#
# Exit codes: 0 done or held, 1 refused or free, 2 usage error.

set -euo pipefail
command -v python3 >/dev/null 2>&1 || { echo "lane.sh: python3 3.10+ is required" >&2; exit 2; }
exec python3 "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lanes.py" "$@"
