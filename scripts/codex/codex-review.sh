#!/usr/bin/env bash
# codex-review.sh: one read-only Codex review of a worktree's diff against a base.
#
#   scripts/codex/codex-review.sh --target <clean-linked-worktree> --base <ref> \
#     [--context <file>]... [--out <answer.json>] [--spec <name>]
#
# Runs the change-review stage with the diff <base>...HEAD (commits, diffstat and
# the diff itself, redacted) as its context. Everything else is codex-consult.sh:
# one JSON response line, exit 0 on every run path, 64 on a usage error, advisory
# only. The /codex-review skill renders the findings.

set -euo pipefail
args=()
while (( $# > 0 )); do
  case "$1" in
    --base) [[ $# -ge 2 ]] || { echo "codex-review.sh: --base needs a ref" >&2; exit 64; }
            args+=(--diff-base "$2"); shift 2 ;;
    --stage) echo "codex-review.sh: the stage is fixed (change-review)" >&2; exit 64 ;;
    *) args+=("$1"); shift ;;
  esac
done
exec python3 "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../vendors/consult.py" \
  --vendor codex --stage change-review ${args[@]+"${args[@]}"}
