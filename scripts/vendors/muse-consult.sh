#!/usr/bin/env bash
# muse-consult.sh: the sanctioned entry point for a read-only Muse consult. Muse
# takes few, large prompts, so its stages (dossier, spec-validate, and the review
# stages beside Codex) carry large context caps in .claude/data/vendor-policy.json.
#
#   scripts/vendors/muse-consult.sh --stage <stage> --target <clean-linked-worktree> \
#     --context <file> [--context <file>]... [--diff-base <ref>] [--out <answer.json>] [--spec <name>]
#
# A thin door into consult.py (the shared envelope). Muse runs with writes, shell
# and web tools disabled and restricted network. Prints one JSON response line;
# exit 0 on every run path, 64 on a usage error. Advisory only.

set -euo pipefail
exec python3 "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/consult.py" --vendor muse "$@"
