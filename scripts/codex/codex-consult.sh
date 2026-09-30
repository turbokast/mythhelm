#!/usr/bin/env bash
# codex-consult.sh: the sanctioned entry point for a read-only Codex consult.
#
#   scripts/codex/codex-consult.sh --stage <stage> --target <clean-linked-worktree> \
#     --context <file> [--context <file>]... [--diff-base <ref>] [--out <answer.json>] [--spec <name>]
#
# A thin door into scripts/vendors/consult.py, which holds the envelope (policy,
# opt-in, clean-worktree target, redaction, version pin, sign-in via the vendor's
# own status command, daily caps, timeout, schema check, post-run clean check).
# Prints one JSON response line; exit 0 on every run path, 64 on a usage error.
# Advisory only: the caller adjudicates every finding (.claude/rules/vendor-usage.md).

set -euo pipefail
exec python3 "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/../vendors/consult.py" --vendor codex "$@"
