#!/usr/bin/env bash
# Run runtime-asset marker tests in temporary Git fixtures; no tracked/state writes.
# Arguments are forwarded to Python unittest (for example -v or a test name).
# Exit 0 on success (including an explicitly reported integration skip), nonzero
# on assertion/tool/usage failure. Python and Git are required for all scope tests.
# Only the real-Go integration skips when Go is absent, CGO is disabled, the race
# target is unsupported or the configured C compiler is absent. No toolchain
# downloads or stand-ins; unexpected installed-tool/build errors fail.
# Temporary files are removed; local Python bytecode/Go build caches may populate.
set -euo pipefail
TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec python3 "$TESTS_DIR/test_gatelib_assets.py" "$@"
