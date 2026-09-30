#!/usr/bin/env bash
# notify-proposals.sh: SessionStart hook. When .claude/proposals/pending.md holds
# proposals awaiting a maintainer's decision, prints one line (the count and the age of
# the oldest, from git blame) as a systemMessage for the user and as additionalContext
# for the model. Silent when nothing is pending.
#
# Advisory: it never blocks. Every failure (no python3, not a git checkout, an
# unreadable file, a timeout) exits 0 with no output. The parsing lives in
# scripts/harness/proposals.py (notify), the one parser of the proposal files.

payload="$(cat 2>/dev/null || true)"
script="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." 2>/dev/null && pwd)/scripts/harness/proposals.py"

root="${CLAUDE_PROJECT_DIR:-}"
if [[ -z "$root" ]] && command -v jq >/dev/null 2>&1; then
  root="$(printf '%s' "$payload" | jq -r '.cwd // empty' 2>/dev/null || true)"
fi
root="$(git -C "${root:-.}" rev-parse --show-toplevel 2>/dev/null || true)"
[[ -n "$root" && -f "$root/.claude/proposals/pending.md" ]] || exit 0
command -v python3 >/dev/null 2>&1 || exit 0

[[ -f "$script" ]] || exit 0
out="$(timeout 10 python3 "$script" notify --root "$root" 2>/dev/null || true)"
[[ -n "$out" ]] && printf '%s\n' "$out"
exit 0
