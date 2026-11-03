#!/usr/bin/env bash
# Run all portable harness tests, serially by default; --jobs 1..64 opts into
# bounded concurrency. A positional substring selects tests. Every selected test
# runs and output retains discovery order. Exit 0 on pass, 1 on failure/no match,
# 2 on usage errors, or 128+signal after stopping active workers on interruption.
# Temporary output files are removed on exit; each test isolates its own fixtures.
#
#   .claude/hooks/tests/run-tests.sh [<name-substring>] [--jobs N]
set -euo pipefail
TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec python3 "$TESTS_DIR/../../../scripts/harness/run_tests.py" "$@"
