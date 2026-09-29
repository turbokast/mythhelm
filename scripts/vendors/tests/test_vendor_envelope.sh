#!/usr/bin/env bash
# test_vendor_envelope.sh: runs the Python suites of the optional vendor layer
# (test_envelope.py, test_lane.py) for .claude/hooks/tests/run-tests.sh and prints
# its "checks=N failed=M" tally. Offline: the vendor CLIs are the stubs in
# fixtures/bin, and every run happens in a throwaway fixture repository. Sandboxed
# lane runs are skipped, with the reason, where bwrap cannot run.

set -uo pipefail
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export PYTHONDONTWRITEBYTECODE=1
out="$(cd "$DIR" && python3 -m unittest -v test_envelope test_lane 2>&1)"
rc=$?
printf '%s\n' "$out" | grep -E '(FAIL|ERROR|skipped)' || true
ran="$(printf '%s\n' "$out" | sed -n 's/^Ran \([0-9]*\) tests\{0,1\} in .*/\1/p')"
failed="$(printf '%s\n' "$out" | grep -cE '\.\.\. (FAIL|ERROR)$' || true)"
(( rc == 0 )) || printf '%s\n' "$out" | tail -40
echo "checks=${ran:-0} failed=${failed:-0}"
(( rc == 0 ))
