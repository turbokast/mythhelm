#!/usr/bin/env bash
# test_learning_suite.sh: runs the Python suites for the learning loop
# (test_proposals.py, test_evals.py, test_health.py) for
# .claude/hooks/tests/run-tests.sh and prints their "checks=N failed=M" tally.
# Offline: git runs against throwaway repositories and a stand-in gh serves fixture
# JSON.

set -uo pipefail
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export PYTHONDONTWRITEBYTECODE=1
out="$(cd "$DIR" && python3 -m unittest -v test_proposals test_evals test_health 2>&1)"
rc=$?
printf '%s\n' "$out" | grep -E '(FAIL|ERROR|skipped)' || true
ran="$(printf '%s\n' "$out" | sed -n 's/^Ran \([0-9]*\) tests\{0,1\} in .*/\1/p')"
failed="$(printf '%s\n' "$out" | grep -cE '\.\.\. (FAIL|ERROR)$' || true)"
(( rc == 0 )) || printf '%s\n' "$out" | tail -60
echo "checks=${ran:-0} failed=${failed:-0}"
(( rc == 0 ))
