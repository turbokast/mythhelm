#!/usr/bin/env bash
# run-tests.sh: runs every harness test: .claude/hooks/tests/test_*.sh and
# scripts/**/tests/test_*.sh. Each test is a self-contained bash script that exits
# 0 on success and prints "checks=N failed=M". Exits 1 when any test fails.
#
#   .claude/hooks/tests/run-tests.sh [<name-substring>]

set -uo pipefail

TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$TESTS_DIR/../../.." && pwd)"
filter="${1:-}"

mapfile -t tests < <(
  { find "$TESTS_DIR" -maxdepth 1 -name 'test_*.sh' -type f
    find "$REPO_ROOT/scripts" -path '*/tests/test_*.sh' -type f 2>/dev/null
  } | LC_ALL=C sort
)

passed=0 failed=0 checks=0
failed_names=()
for t in "${tests[@]}"; do
  name="${t#"$REPO_ROOT"/}"
  [[ -z "$filter" || "$name" == *"$filter"* ]] || continue
  out="$(bash "$t" 2>&1)"
  rc=$?
  n="$(printf '%s\n' "$out" | sed -n 's/^checks=\([0-9]*\) failed=.*/\1/p' | tail -1)"
  checks=$((checks + ${n:-0}))
  if (( rc == 0 )); then
    echo "PASS  $name (${n:-?} checks)"
    passed=$((passed + 1))
  else
    echo "FAIL  $name"
    printf '%s\n' "$out" | sed 's/^/      /'
    failed=$((failed + 1))
    failed_names+=("$name")
  fi
done

echo
echo "Test summary: $passed passed, $failed failed, $checks checks"
if (( passed + failed == 0 )); then
  echo "No tests matched." >&2
  exit 1
fi
if (( failed > 0 )); then
  printf '  failed: %s\n' "${failed_names[@]}" >&2
  exit 1
fi
