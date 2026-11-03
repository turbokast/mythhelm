#!/usr/bin/env bash
# Run runtime-asset marker tests, including actual local Go, in temporary Git
# fixtures. No stand-in tools; exit nonzero on failure; no repository state writes.
set -euo pipefail
TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec python3 "$TESTS_DIR/test_gatelib_assets.py" "$@"
