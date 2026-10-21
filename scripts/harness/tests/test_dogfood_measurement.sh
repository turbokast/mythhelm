#!/usr/bin/env bash
# Run synthetic ledger negative controls. No live agent, credentials or network.
# Exit 0 on a passing nonempty suite, 1 otherwise; only temporary fixture writes.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
python3 - "$script_dir" <<'PY'
import sys
import unittest
suite = unittest.defaultTestLoader.discover(sys.argv[1], pattern="test_dogfood_measurement.py")
count = suite.countTestCases()
result = unittest.TextTestRunner(verbosity=2).run(suite)
failed = len(result.failures) + len(result.errors)
print(f"checks={count} failed={failed}")
sys.exit(0 if count >= 20 and result.wasSuccessful() else 1)
PY
