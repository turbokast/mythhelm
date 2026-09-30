#!/usr/bin/env bash
# test_guard_dispatch_pin.sh: in a session running /run-spec, /implement or /finalize-spec, an
# Agent dispatch must name a project agent (or Explore/Plan); every other session
# and an unreadable input fails closed on the dispatches it guards.

# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
HOOK="$HOOKS_DIR/guard-dispatch-pin.sh"

PROJ="$TEST_TMP/proj"
new_repo "$PROJ"
mkdir -p "$PROJ/.claude/agents"
printf -- '---\nname: go-implementer\n---\n' > "$PROJ/.claude/agents/go-implementer.md"
export CLAUDE_PROJECT_DIR="$PROJ"

RUN="$TEST_TMP/run.jsonl"
jq -nc '{type:"assistant",message:{content:[{type:"tool_use",name:"Skill",input:{skill:"run-spec",args:"demo"}}]}}' > "$RUN"
SLASH="$TEST_TMP/slash.jsonl"
jq -nc '{type:"user",message:{content:"<command-name>/implement</command-name> demo"}}' > "$SLASH"
PLAIN="$TEST_TMP/plain.jsonl"
jq -nc '{type:"user",message:{content:"explain the run-spec skill, then implement nothing"}}' > "$PLAIN"

# agent <transcript> [subagent_type|-] [tool]: an Agent payload; "-" omits the type.
agent() {
  jq -nc --arg t "$1" --arg a "${2--}" --arg tool "${3:-Agent}" --arg cwd "$PROJ" \
    '{hook_event_name:"PreToolUse",tool_name:$tool,session_id:"s1",cwd:$cwd,transcript_path:$t,
      tool_input:({description:"d",prompt:"p"} + (if $a == "-" then {} else {subagent_type:$a} end))}'
}

echo "== in an implementation run =="
expect_rc 2 "an omitted subagent_type blocks" "$HOOK" "$(agent "$RUN")"
expect_stanza guard-dispatch-pin "omitted"
expect_err "go-implementer" "the fix names an agent to use"
for a in general-purpose claude fork "" "no-such-agent" "../agents/go-implementer"; do
  expect_rc 2 "subagent_type '$a' blocks" "$HOOK" "$(agent "$RUN" "$a")"
done
expect_rc 0 "a project agent passes" "$HOOK" "$(agent "$RUN" go-implementer)"
expect_rc 0 "Explore passes" "$HOOK" "$(agent "$RUN" Explore)"
expect_rc 0 "Plan passes" "$HOOK" "$(agent "$RUN" Plan)"
expect_rc 2 "the older tool name Task is guarded too" "$HOOK" "$(agent "$RUN" general-purpose Task)"
expect_rc 2 "a slash-command /implement puts the session in scope" "$HOOK" "$(agent "$SLASH" general-purpose)"
for s in finalize-spec finalize-spec-review finalize-spec-verify-ci; do
  jq -nc --arg s "$s" '{type:"assistant",message:{content:[{type:"tool_use",name:"Skill",input:{skill:$s,args:"demo"}}]}}' > "$TEST_TMP/fin.jsonl"
  expect_rc 2 "/$s puts the session in scope" "$HOOK" "$(agent "$TEST_TMP/fin.jsonl" general-purpose)"
  expect_rc 0 "/$s allows a project agent" "$HOOK" "$(agent "$TEST_TMP/fin.jsonl" go-implementer)"
done
jq -nc '{type:"user",message:{content:"<command-name>/finalize-spec</command-name> demo"}}' > "$TEST_TMP/fin-slash.jsonl"
expect_rc 2 "a slash-command /finalize-spec puts the session in scope" "$HOOK" "$(agent "$TEST_TMP/fin-slash.jsonl" general-purpose)"
jq -nc '{type:"assistant",message:{content:[{type:"tool_use",name:"Skill",input:{skill:"finalize-spec-pm",args:"demo"}}]}}' > "$TEST_TMP/fin-pm.jsonl"
expect_rc 0 "a finalize step that dispatches nothing is out of scope" "$HOOK" "$(agent "$TEST_TMP/fin-pm.jsonl" general-purpose)"

echo "== outside a run, and on unreadable input =="
expect_rc 0 "an ordinary session may dispatch general-purpose" "$HOOK" "$(agent "$PLAIN" general-purpose)"
expect_rc 0 "an ordinary session may omit the type" "$HOOK" "$(agent "$PLAIN")"
expect_rc 0 "another tool is ignored" "$HOOK" "$(agent "$RUN" general-purpose Bash)"
expect_rc 2 "an unreadable transcript counts as in scope" "$HOOK" "$(agent "$TEST_TMP/missing.jsonl" general-purpose)"
expect_rc 0 "an unreadable transcript still allows a project agent" "$HOOK" "$(agent "$TEST_TMP/missing.jsonl" go-implementer)"
expect_rc 0 "a payload that is not JSON and names no Agent tool is allowed" "$HOOK" "not json"
expect_rc 2 "a payload that is not JSON but names the Agent tool blocks" "$HOOK" '{"tool_name":"Agent","tool_input":{'
expect_stanza guard-dispatch-pin "malformed Agent payload"
OUT="$(agent "$RUN" go-implementer | HOOK_JQ_PROBE=no-such-jq "$HOOK" 2>&1)"; RC=$?
check "missing jq blocks an Agent dispatch" [ "$RC" == 2 ]
check "and says to install jq" grep -q "jq is not installed" <<<"$OUT"
jq -nc '{tool_name:"Bash",tool_input:{command:"ls"}}' | HOOK_JQ_PROBE=no-such-jq "$HOOK" >/dev/null 2>&1; RC=$?
check "missing jq allows a payload for another tool" [ "$RC" == 0 ]
agent "$RUN" general-purpose | CLAUDE_PROJECT_DIR="$TEST_TMP/nowhere" "$HOOK" >/dev/null 2>&1; RC=$?
check "without .claude/agents the unpinned forms still block" [ "$RC" == 2 ]
agent "$RUN" some-agent | CLAUDE_PROJECT_DIR="$TEST_TMP/nowhere" "$HOOK" >/dev/null 2>&1; RC=$?
check "without .claude/agents a named agent cannot be checked and passes" [ "$RC" == 0 ]

finish
