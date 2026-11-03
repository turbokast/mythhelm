#!/usr/bin/env bash
# Verify GitHub-dependent gate selection/freshness with earned markers and real
# offline consumers in temporary fixture repositories. Exit nonzero on failure;
# no real repository state, network, inference or gate tools are modified.
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
python3 - "$HERE" <<'PYTEST'
import sys
import unittest
suite = unittest.defaultTestLoader.discover(sys.argv[1], pattern="test_github_gate_scope.py")
count = suite.countTestCases()
result = unittest.TextTestRunner(verbosity=2).run(suite)
failed = len(result.failures) + len(result.errors)
print(f"checks={count} failed={failed}")
sys.exit(0 if count >= 7 and result.wasSuccessful() else 1)
PYTEST
