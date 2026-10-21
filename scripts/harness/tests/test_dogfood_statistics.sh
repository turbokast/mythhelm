#!/usr/bin/env bash
# Run synthetic preregistration/statistics controls; only temporary file writes.
# Exit 0 for a passing nonempty suite, 1 otherwise. No native agent or network.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
python3 - "$script_dir" <<'PY'
import sys
import unittest
suite = unittest.defaultTestLoader.discover(sys.argv[1], pattern="test_dogfood_statistics.py")
count = suite.countTestCases()
result = unittest.TextTestRunner(verbosity=2).run(suite)
print(f"checks={count} failed={len(result.failures) + len(result.errors)}")
sys.exit(0 if count >= 9 and result.wasSuccessful() else 1)
PY
