#!/usr/bin/env bash
# shellcheck disable=SC2016  # file contents under test are literal data
# test_validate_agent_config.sh: validate-agent-config.sh on agents, skills, rules,
# hook scripts and settings, for Write and Edit payloads, with PyYAML and with the
# built-in fallback parser.

# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
HOOK="$HOOKS_DIR/validate-agent-config.sh"

R="$TEST_TMP/repo"
mkdir -p "$R/.claude/agents" "$R/.claude/skills/run-spec" "$R/.claude/rules" "$R/.claude/hooks"
printf '#!/usr/bin/env bash\nexit 0\n' > "$R/.claude/hooks/ok.sh"
chmod +x "$R/.claude/hooks/ok.sh"
printf '#!/usr/bin/env bash\n' > "$R/.claude/hooks/not-exec.sh"

write_payload() {   # write_payload <path> <content>
  jq -nc --arg p "$1" --arg c "$2" '{hook_event_name:"PreToolUse",tool_name:"Write",tool_input:{file_path:$p,content:$c}}'
}
edit_payload() {    # edit_payload <path> <old> <new>
  jq -nc --arg p "$1" --arg o "$2" --arg n "$3" '{hook_event_name:"PreToolUse",tool_name:"Edit",tool_input:{file_path:$p,old_string:$o,new_string:$n}}'
}
ok()  { expect_rc 0 "$1" "$HOOK" "$(write_payload "$2" "$3")"; }
bad() { expect_rc 2 "$1" "$HOOK" "$(write_payload "$2" "$3")"; [[ -z "${4:-}" ]] || expect_err "$4" "$1: detail"; }

AGENT="$R/.claude/agents/go-implementer.md"
SKILL="$R/.claude/skills/run-spec/SKILL.md"
RULE="$R/.claude/rules/go-style.md"
GOOD_AGENT=$'---\nname: go-implementer\ndescription: Implements Go changes.\nmodel: sonnet\ntools: [Read, Edit, Bash]\n---\n\nBody.\n'
GOOD_SKILL=$'---\nname: run-spec\ndescription: >\n  Implement every task in a spec.\n---\n\nBody.\n'

run_suite() {
  echo "== agents =="
  ok  "valid agent" "$AGENT" "$GOOD_AGENT"
  ok  "block-list tools" "$AGENT" $'---\nname: go-implementer\ndescription: d\nmodel: opus\ntools:\n  - Read\n  - Bash\n---\n'
  bad "no frontmatter" "$AGENT" $'# Go implementer\n' "missing YAML frontmatter"
  bad "unclosed frontmatter" "$AGENT" $'---\nname: go-implementer\n' "never closed"
  bad "missing description" "$AGENT" $'---\nname: go-implementer\nmodel: sonnet\n---\n' "description"
  bad "missing model" "$AGENT" $'---\nname: go-implementer\ndescription: d\n---\n' "model"
  bad "dated model id" "$AGENT" $'---\nname: go-implementer\ndescription: d\nmodel: claude-sonnet-4-5\n---\n' "not one of opus, sonnet, haiku"
  bad "name differs from file" "$AGENT" $'---\nname: go-impl\ndescription: d\nmodel: haiku\n---\n' "does not match"
  bad "name not kebab-case" "$R/.claude/agents/Go_Impl.md" $'---\nname: Go_Impl\ndescription: d\nmodel: haiku\n---\n' "lowercase"
  bad "tool allowed and denied" "$AGENT" $'---\nname: go-implementer\ndescription: d\nmodel: sonnet\ntools: [Read, Bash]\ndisallowedTools: [Bash]\n---\n' "both tools and disallowedTools"
  bad "unterminated quote" "$AGENT" $'---\nname: "go-implementer\ndescription: d\nmodel: sonnet\n---\n' "does not parse"

  echo "== skills =="
  ok  "valid skill" "$SKILL" "$GOOD_SKILL"
  ok  "skill with a model" "$SKILL" $'---\nname: run-spec\ndescription: d\nmodel: haiku\n---\n'
  bad "skill without name" "$SKILL" $'---\ndescription: d\n---\n' "name"
  bad "skill name differs from directory" "$SKILL" $'---\nname: runspec\ndescription: d\n---\n' "skill directory"
  bad "skill bad model" "$SKILL" $'---\nname: run-spec\ndescription: d\nmodel: gpt\n---\n' "not one of"
  ok  "skill helper doc is not validated" "$R/.claude/skills/run-spec/reference.md" $'no frontmatter\n'

  echo "== rules =="
  ok  "rule without frontmatter" "$RULE" $'# Go style\n'
  ok  "rule with paths" "$RULE" $'---\ndescription: Go style\npaths:\n  - "internal/**/*.go"\n  - "cmd/*.go"\n  - "**/*.{go,mod}"\n---\n'
  ok  "rule with a flow list" "$RULE" $'---\npaths: ["internal/**", "go.mod"]\n---\n'
  bad "absolute path glob" "$RULE" $'---\npaths:\n  - /internal/**\n---\n' "relative"
  bad "parent traversal" "$RULE" $'---\npaths:\n  - ../other/**\n---\n' "climb"
  bad "unterminated class" "$RULE" $'---\npaths:\n  - "internal/[ab.go"\n---\n' "unterminated"
  bad "unbalanced brace" "$RULE" $'---\npaths:\n  - "**/*.{go,mod"\n---\n' "unbalanced"
  bad "empty paths" "$RULE" $'---\npaths: []\n---\n' "non-empty"

  echo "== hooks and settings =="
  ok  "valid hook script" "$R/.claude/hooks/new.sh" $'#!/usr/bin/env bash\nif true; then echo ok; fi\n'
  bad "hook syntax error" "$R/.claude/hooks/new.sh" $'#!/usr/bin/env bash\nif true; then echo ok\n' "bash -n"
  ok  "valid settings" "$R/.claude/settings.json" '{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"$CLAUDE_PROJECT_DIR/.claude/hooks/ok.sh","timeout":5}]}]}}'
  ok  "settings without hooks" "$R/.claude/settings.json" '{"permissions":{"deny":[]}}'
  bad "settings JSON syntax" "$R/.claude/settings.json" '{"hooks": {' "not valid JSON"
  bad "unknown event" "$R/.claude/settings.json" '{"hooks":{"preToolUse":[]}}' "unknown hook event"
  bad "missing hook script" "$R/.claude/settings.json" '{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"$CLAUDE_PROJECT_DIR/.claude/hooks/missing.sh"}]}]}}' "not an executable file"
  bad "non-executable hook script" "$R/.claude/settings.local.json" '{"hooks":{"Stop":[{"hooks":[{"type":"command","command":".claude/hooks/not-exec.sh"}]}]}}' "not an executable file"
  ok  "command without a path is not checked" "$R/.claude/settings.json" '{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"echo done"}]}]}}'
  bad "other .claude JSON" "$R/.claude/config.json" '{"a":' "not valid JSON"
}

run_suite
if python3 -c 'import yaml' 2>/dev/null; then
  echo "== again with the fallback parser =="
  export VALIDATE_AGENT_CONFIG_NO_YAML=1
  run_suite
  unset VALIDATE_AGENT_CONFIG_NO_YAML
fi

echo "== Edit payloads =="
printf '%s' "$GOOD_AGENT" > "$AGENT"
expect_rc 0 "edit keeps it valid" "$HOOK" "$(edit_payload "$AGENT" "Implements Go changes." "Implements Go.")"
expect_rc 2 "edit breaks the model" "$HOOK" "$(edit_payload "$AGENT" "model: sonnet" "model: gpt-5")"
expect_rc 0 "old_string not found is left to the Edit tool" "$HOOK" "$(edit_payload "$AGENT" "absent text" "x")"

echo "== skipped =="
expect_rc 0 "file outside .claude" "$HOOK" "$(write_payload "$R/docs/x.md" $'---\nbroken: "\n---\n')"
expect_rc 0 "no file path" "$HOOK" '{"tool_name":"Write","tool_input":{}}'
expect_rc 0 "not JSON" "$HOOK" 'not json'

echo "== the block teaches =="
run_hook "$HOOK" "$(write_payload "$AGENT" $'---\nname: go-implementer\n---\n')"
CHECKS=$((CHECKS + 1))
stanza_re="^BLOCK: validate-agent-config"$'\n'"File: .+"$'\n'"Detail: .+"$'\n'"Fix: .+"
[[ "$ERR" =~ $stanza_re ]] || fail "[stanza] $ERR"

finish
