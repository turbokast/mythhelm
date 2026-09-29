#!/usr/bin/env bash
# test_settings_registration.sh: .claude/settings.json, the hook scripts and
# INVENTORY.md agree. Every registered command is an executable file, every hook
# script is registered, listed in the inventory and exercised by a test, and the
# settings file passes validate-agent-config.sh.

# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
SETTINGS="$REPO_ROOT/.claude/settings.json"
INVENTORY="$HOOKS_DIR/INVENTORY.md"

check "settings.json is JSON" jq -e . "$SETTINGS" >/dev/null

mapfile -t COMMANDS < <(jq -r '.hooks[][]?.hooks[]?.command' "$SETTINGS")
check "hooks are registered" [ "${#COMMANDS[@]}" -gt 0 ]
for c in "${COMMANDS[@]}"; do
  check "uses \$CLAUDE_PROJECT_DIR: $c" [ "${c#\$CLAUDE_PROJECT_DIR/}" != "$c" ]
  path="$REPO_ROOT/${c#\$CLAUDE_PROJECT_DIR/}"
  check "executable: $c" [ -x "$path" ]
  name="$(basename "$path")"
  check "in INVENTORY.md: $name" grep -q "^| \`$name\` |" "$INVENTORY"
  check "tested: $name" grep -lq "$name" "$HOOKS_DIR"/tests/test_*.sh
done

for f in "$HOOKS_DIR"/*.sh; do
  name="$(basename "$f")"
  [[ "$name" == hook-helpers.sh ]] && continue
  check "registered: $name" grep -q "/.claude/hooks/$name\"" "$SETTINGS"
done

payload="$(jq -nc --arg p "$SETTINGS" --rawfile c "$SETTINGS" '{tool_name:"Write",tool_input:{file_path:$p,content:$c}}')"
expect_rc 0 "settings.json passes validate-agent-config" "$HOOKS_DIR/validate-agent-config.sh" "$payload"

finish
