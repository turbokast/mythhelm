#!/usr/bin/env bash
# Exercise the content checker on temporary synthetic sites and rejecting
# fixtures. No network; temporary files removed on exit. Exit 0 pass, 1 failure.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
python3 "$SCRIPT_DIR/fixtures/docs-site/test_cases.py"
