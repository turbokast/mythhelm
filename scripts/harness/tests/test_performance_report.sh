#!/usr/bin/env bash
# Run synthetic native-evidence reporting controls in temporary Git repositories.
# Exit 0 for a passing nonempty suite, 1 otherwise. No native agent or network.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
python3 - "$script_dir" <<'PY'
import sys
import unittest
suite = unittest.defaultTestLoader.discover(sys.argv[1], pattern="test_performance_report.py")
count = suite.countTestCases()
result = unittest.TextTestRunner(verbosity=2).run(suite)
print(f"checks={count} failed={len(result.failures) + len(result.errors)}")
sys.exit(0 if count >= 19 and result.wasSuccessful() else 1)
PY
