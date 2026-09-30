#!/usr/bin/env bash
# shellcheck disable=SC2016  # fixture text is literal data
# test_notify_proposals.sh: notify-proposals.sh prints one SessionStart line with the
# count of pending proposals and the age of the oldest, stays silent when nothing is
# pending, and exits 0 with no output on every failure.

# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
HOOK="$HOOKS_DIR/notify-proposals.sh"
PROJ="$TEST_TMP/proj"
new_repo "$PROJ"
mkdir -p "$PROJ/.claude/proposals"
export CLAUDE_PROJECT_DIR="$PROJ"
PAYLOAD='{"hook_event_name":"SessionStart","source":"startup","session_id":"n-1","cwd":"."}'

proposal() {
  printf '\n## %s — A change\n\n- **Source spec**: `demo`\n- **Type**: knowledge\n- **Target**: `knowledge/x.md`\n- **Rationale**: r\n- **Evidence**: e\n\n**Proposed change:**\n\nText.\n' "$1"
}

echo "== nothing pending is silent =="
printf '# Pending proposals\n\nIntro.\n' > "$PROJ/.claude/proposals/pending.md"
git -C "$PROJ" add .claude && git -C "$PROJ" commit -qm seed
expect_rc 0 "no proposals" "$HOOK" "$PAYLOAD"
check "no proposals prints nothing" [ -z "$OUT" ]

echo "== pending proposals are counted with the oldest age =="
proposal P-demo-1 >> "$PROJ/.claude/proposals/pending.md"
git -C "$PROJ" add .claude
GIT_AUTHOR_DATE="$(date -u -d '10 days ago' '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u -v-10d '+%Y-%m-%dT%H:%M:%SZ')" \
  git -C "$PROJ" commit -qm "add P-demo-1"
proposal P-demo-2 >> "$PROJ/.claude/proposals/pending.md"
expect_rc 0 "two proposals" "$HOOK" "$PAYLOAD"
check "one line of JSON" [ "$(printf '%s\n' "$OUT" | wc -l)" -eq 1 ]
check "systemMessage counts both and names the oldest" \
  [ "$(printf '%s' "$OUT" | jq -r .systemMessage)" = "Harness proposals: 2 pending (oldest 10 days). Decide them with /apply-proposals." ]
check "additionalContext for SessionStart" \
  [ "$(printf '%s' "$OUT" | jq -r '.hookSpecificOutput.hookEventName + "|" + (.hookSpecificOutput.additionalContext | length > 0 | tostring)')" = "SessionStart|true" ]

echo "== a proposal inside a code fence is not counted =="
printf '\n```markdown\n## P-demo-9 — example\n```\n' >> "$PROJ/.claude/proposals/pending.md"
run_hook "$HOOK" "$PAYLOAD"
check "fenced heading ignored" [ "$(printf '%s' "$OUT" | jq -r .systemMessage | grep -c '2 pending')" -eq 1 ]

echo "== the payload's cwd locates the checkout without CLAUDE_PROJECT_DIR =="
unset CLAUDE_PROJECT_DIR
expect_rc 0 "cwd payload" "$HOOK" "$(jq -nc --arg w "$PROJ" '{hook_event_name:"SessionStart",cwd:$w}')"
check "cwd payload reports" [ -n "$OUT" ]
export CLAUDE_PROJECT_DIR="$PROJ"

echo "== failures are silent and never block =="
expect_rc 0 "malformed payload" "$HOOK" "not json"
check "malformed payload still reports from CLAUDE_PROJECT_DIR" [ -n "$OUT" ]
mkdir -p "$TEST_TMP/plain"
CLAUDE_PROJECT_DIR="$TEST_TMP/plain" run_hook "$HOOK" "$PAYLOAD"
check "outside a git checkout: exit 0" [ "$RC" -eq 0 ]
check "outside a git checkout: silent" [ -z "$OUT" ]
bin="$TEST_TMP/nopython"
mkdir -p "$bin"
for t in bash cat git jq dirname timeout; do ln -s "$(command -v "$t")" "$bin/$t"; done
PATH="$bin" run_hook "$HOOK" "$PAYLOAD"
check "without python3: exit 0" [ "$RC" -eq 0 ]
check "without python3: silent" [ -z "$OUT" ]
printf 'garbage \xff\xfe\n## P-bad\n' > "$PROJ/.claude/proposals/pending.md"
run_hook "$HOOK" "$PAYLOAD"
check "unparseable pending.md: exit 0" [ "$RC" -eq 0 ]

finish
