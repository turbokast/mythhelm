#!/usr/bin/env bash
# verify-task-completion.sh: Stop and SubagentStop hook. A session that marked a
# spec task complete cannot stop until the completion is backed by evidence.
#
# A task counts as claimed by this stop when all of these hold:
#   - the working tree's specs/<state>/<name>/tasks.md marks it complete (heading
#     "✅ COMPLETED" or a Status field starting with "✅") and the same spec's
#     tasks.md at the base (merge-base of HEAD and origin/main) does not;
#   - this session edited that tasks.md: its transcript (the subagent's own
#     transcript on SubagentStop) holds an Edit/Write/MultiEdit/NotebookEdit of the
#     file, or a Bash command naming it. A transcript that cannot be read counts as
#     an edit, so a missing transcript never waves a claim through.
# With no claim the hook allows the stop: another session's work in a shared tree,
# a read-only subagent or a stop with no completion claim is never blocked.
#
# For a claim, it blocks (exit 2, BLOCK/File/Detail/Fix stanza) unless:
#   - each claimed task's entry is well-formed (task-completion skill), and
#   - every gate the tree's change set needs (Go files: the Go gates; harness
#     files: the harness gates; any change: hygiene) has a fresh marker from
#     scripts/harness/gate.sh in this tree.
# All state is keyed by session id and tree, under <main checkout>/.claude/data/:
#   stop-gate-<session>.json           consecutive blocks per tree and fingerprint
#   stop-gate-override-<session>.json  the escape hatch, written only by
#                                      `gatelib.py override --reason`; honoured while
#                                      the tree is byte-identical to when it was written
#   stop-gate-audit.jsonl              every block, release and override
# After three consecutive blocks on an unchanged tree, a stop with
# stop_hook_active=true is released and audited, so a wrong block cannot trap a
# session forever; the release is shown to the user.
#
# The logic lives in scripts/harness/gatelib.py (stop-hook), next to the gate
# runner, so the marker format and the fingerprint have one implementation.
#
# When the checker cannot decide (python3 missing, a payload that is not JSON, an
# internal error), this wrapper fails closed on what it guards and allows the
# rest: it blocks when a spec's tasks.md differs from the base in the payload's
# tree, so a completion claim may exist, and allows every other stop with a
# notice. A stop with stop_hook_active=true after such a block is allowed with the
# notice, so an environment fault costs one extra turn and never traps a session.

# Residual: a completion claim made without editing tasks.md through a tool the
# transcript records (for example by a script the session ran) is attributed only
# when the command names the file.

set -euo pipefail

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHECKER="$HOOK_DIR/../../scripts/harness/gatelib.py"
payload="$(cat)"

notice() {   # a systemMessage for the user; printf-escaped by hand, python3 may be missing
  local m="verify-task-completion: $1"
  m="${m//\\/\\\\}"
  m="${m//\"/\\\"}"
  printf '{"systemMessage":"%s"}\n' "$m"
}

# fallback <why>: the checker could not decide; fail closed only on a possible claim.
fallback() {
  local why="$1" cwd="" base="" changed=""
  if [[ "$payload" =~ \"cwd\"[[:space:]]*:[[:space:]]*\"([^\"]*)\" ]]; then
    cwd="${BASH_REMATCH[1]}"
  fi
  cwd="${cwd:-${CLAUDE_PROJECT_DIR:-$PWD}}"
  # Root-relative queries: pathspecs resolve from -C's directory, so a nested cwd
  # would otherwise miss specs/*/*/tasks.md.
  if cwd="$(git -C "$cwd" rev-parse --show-toplevel 2>/dev/null)"; then
    base="$(git -C "$cwd" merge-base HEAD origin/main 2>/dev/null || echo HEAD)"
    changed="$( { git -C "$cwd" diff --name-only "$base" -- 'specs/*/*/tasks.md' 2>/dev/null
                  git -C "$cwd" ls-files --others --exclude-standard -- 'specs/*/*/tasks.md' 2>/dev/null; } | head -5)"
  fi
  if [[ -z "$changed" ]]; then
    notice "$why; no tasks.md changed, so no completion claim was possible."
    exit 0
  fi
  if [[ "$payload" =~ \"stop_hook_active\"[[:space:]]*:[[:space:]]*true ]]; then
    notice "$why; a completion may be claimed in ${changed//$'\n'/, } but was not verified."
    exit 0
  fi
  printf 'BLOCK: verify-task-completion\nFile: %s\nDetail: %s, and tasks.md differs from the base, so a completion claim cannot be ruled out. A guard fails closed on what it cannot check.\nFix: repair the checker (install python3 3.10+, or run python3 scripts/harness/gatelib.py stop-hook with the payload to see the error), then stop again; a second stop is allowed with a notice.\n' \
    "${changed//$'\n'/, }" "$why" >&2
  exit 2
}

if ! command -v "${HOOK_PYTHON_PROBE:-python3}" >/dev/null 2>&1; then
  fallback "python3 is not installed, so the task-completion gate did not run"
fi

rc=0
out="$(printf '%s' "$payload" | python3 "$CHECKER" stop-hook)" || rc=$?
case "$rc" in
  0|2)
    [[ -z "$out" ]] || printf '%s\n' "$out"
    exit "$rc"
    ;;
  *)
    out="${out//$'\n'/ }"
    fallback "the checker could not decide (exit $rc${out:+: ${out:0:200}})"
    ;;
esac
