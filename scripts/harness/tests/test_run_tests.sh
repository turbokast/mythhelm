#!/usr/bin/env bash
# Verify the harness runner with synthetic isolated suites, including concurrency,
# complete failure reporting and cancellation. No real repository state changes.
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
python3 - "$HERE" <<'PYTEST'
import sys
import unittest
suite = unittest.defaultTestLoader.discover(sys.argv[1], pattern="test_run_tests.py")
count = suite.countTestCases()
result = unittest.TextTestRunner(verbosity=2).run(suite)
failed = len(result.failures) + len(result.errors)
print(f"checks={count} failed={failed}")
sys.exit(0 if count >= 12 and result.wasSuccessful() else 1)
PYTEST
