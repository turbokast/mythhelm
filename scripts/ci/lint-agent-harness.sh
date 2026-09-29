#!/usr/bin/env bash
# lint-agent-harness.sh: CI gate for the agent harness configuration. Runs every
# check in scripts/ci/harness_lint.py against the tree, each in its own process, so
# one failing or crashing check never hides another, then prints a summary table.
#
#   scripts/ci/lint-agent-harness.sh [--root DIR] [--only CHECK[,CHECK...]]
#
#   --root DIR    the tree to lint (default: this repository). Tests point it at
#                 fixture trees.
#   --only LIST   run only the named checks (see harness_lint.py --list).
#
# The checks, in order: frontmatter, routing-pins, haiku-effort, rule-budget,
# paths-globs, hook-inventory, skill-contexts, dollar-zero, abs-paths, references,
# specs, product.
# What each one enforces is documented at the top of harness_lint.py.
#
# Read-only: no network, no writes. Exit 0 when every check passes, 1 when any
# check fails, 2 on a usage error or a missing prerequisite (bash 4+, git,
# python3 3.10+).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LINT="$SCRIPT_DIR/harness_lint.py"
ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
ONLY=""

while (( $# > 0 )); do
  case "$1" in
    --root)
      [[ $# -ge 2 && -d "$2" ]] || { echo "lint-agent-harness: --root needs a directory" >&2; exit 2; }
      ROOT="$(cd "$2" && pwd)"; shift 2 ;;
    --only)
      [[ $# -ge 2 && -n "$2" ]] || { echo "lint-agent-harness: --only needs a check list" >&2; exit 2; }
      ONLY="$2"; shift 2 ;;
    *)
      echo "lint-agent-harness: unknown argument: $1" >&2; exit 2 ;;
  esac
done

if (( BASH_VERSINFO[0] < 4 )); then
  echo "lint-agent-harness: bash 4+ is required (found $BASH_VERSION)" >&2
  exit 2
fi
for tool in git python3; do
  command -v "$tool" >/dev/null 2>&1 || { echo "lint-agent-harness: $tool is required" >&2; exit 2; }
done
python3 -c 'import sys; sys.exit(sys.version_info < (3, 10))' \
  || { echo "lint-agent-harness: python3 3.10+ is required" >&2; exit 2; }

list_out="$(python3 "$LINT" --list)" \
  || { echo "lint-agent-harness: harness_lint.py --list failed" >&2; exit 2; }
mapfile -t ALL <<< "$list_out"
if [[ -z "${ALL[0]:-}" ]]; then
  echo "lint-agent-harness: harness_lint.py --list returned no checks" >&2
  exit 2
fi
if [[ -n "$ONLY" ]]; then
  IFS=',' read -r -a CHECKS <<< "$ONLY"
  for c in "${CHECKS[@]}"; do
    printf '%s\n' "${ALL[@]}" | grep -qxF -- "$c" \
      || { echo "lint-agent-harness: unknown check: $c (known: ${ALL[*]})" >&2; exit 2; }
  done
else
  CHECKS=("${ALL[@]}")
fi

declare -A RESULT=()
overall=0
for c in "${CHECKS[@]}"; do
  echo "=== $c ==="
  rc=0
  python3 "$LINT" "$c" --root "$ROOT" || rc=$?
  case "$rc" in
    0) RESULT[$c]=PASS ;;
    1) RESULT[$c]=FAIL; overall=1 ;;
    *) RESULT[$c]="ERROR($rc)"; overall=1 ;;
  esac
done

echo
echo "=== Summary ==="
for c in "${CHECKS[@]}"; do
  printf '%-16s %s\n' "$c" "${RESULT[$c]}"
done
if (( overall != 0 )); then
  echo "lint-agent-harness: FAILED (fix the findings above; each is path:line: check: detail)"
  exit 1
fi
echo "lint-agent-harness: all ${#CHECKS[@]} checks passed"
