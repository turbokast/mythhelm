#!/usr/bin/env bash
# guard-dispatch-pin.sh: PreToolUse hook on the Agent tool (named Task in older
# Claude Code versions). In a session running an implementation skill, every
# dispatch must name an agent whose model pin engages.
#
# Claude Code applies a subagent's model and effort from its definition's
# frontmatter (.claude/agents/<name>.md, knowledge/agent-routing.md). A dispatch
# that omits subagent_type, or names general-purpose, claude or fork, runs on the
# dispatching session's own model instead: an implementation task silently moves
# to the most expensive tier, and the reviewer-differs-from-implementer split in
# the routing table no longer holds.
#
# Scope: a session whose transcript shows /run-spec or /implement (a Skill tool
# call or a slash command). Other sessions are untouched; a person dispatching a
# general-purpose researcher in an ordinary session chooses that on purpose.
#
# In scope, a dispatch passes when subagent_type names a file in .claude/agents/
# or one of the read-only built-ins Explore and Plan. Anything else blocks
# (exit 2, BLOCK/Command/Detail/Fix stanza).
#
# Fails closed on what it guards (.claude/rules/strict-by-default.md): when jq is
# missing or the payload is not JSON, it blocks a payload that names the Agent or
# Task tool and allows the rest; when the transcript cannot be read, the session
# is treated as in scope, so the pins are enforced rather than skipped. Without a
# project .claude/agents directory it cannot check a name, so it blocks only the
# unpinned forms (omitted, general-purpose, claude, fork).

set -euo pipefail

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hook-helpers.sh
. "$HOOK_DIR/hook-helpers.sh"

HH_GUARD=guard-dispatch-pin
input="$(cat)"
names_agent_tool() { [[ "$input" =~ \"tool_name\"[[:space:]]*:[[:space:]]*\"(Agent|Task)\" ]]; }
if ! command -v "${HOOK_JQ_PROBE:-jq}" >/dev/null 2>&1; then
  names_agent_tool || exit 0
  HH_COMMAND="<unparsed Agent payload>"
  hh_block "jq is not installed, so ${HH_GUARD} cannot read this Agent dispatch. A guard fails closed on what it cannot parse." \
    "Install jq (apt install jq / brew install jq) from your own terminal, then retry."
fi
if ! fields="$(printf '%s' "$input" | jq -r '[
    (.tool_name // ""),
    (if (.tool_input // {}) | has("subagent_type") then (.tool_input.subagent_type // "") else "<omitted>" end),
    (.transcript_path // ""),
    (.cwd // "")
  ] | map(gsub("[\u001f\n]"; " ")) | join("\u001f")' 2>/dev/null)"; then
  names_agent_tool || exit 0
  HH_COMMAND="<unparsed Agent payload>"
  hh_block "the hook payload is not valid JSON and names the Agent tool, so the dispatch cannot be checked." \
    "Retry the dispatch. If this persists, report the malformed payload."
fi
# A non-whitespace separator, so an empty subagent_type stays an empty field.
IFS=$'\x1f' read -r tool agent transcript cwd <<<"$fields"
[[ "$tool" == Agent || "$tool" == Task ]] || exit 0

# In scope when the transcript shows an implementation skill, or cannot be read.
if [[ -n "$transcript" && -r "$transcript" ]]; then
  grep -Eq '"skill" *: *"(run-spec|implement)"|command-name>/?(run-spec|implement)<' "$transcript" 2>/dev/null || exit 0
fi

project="${CLAUDE_PROJECT_DIR:-}"
if [[ -z "$project" && -n "$cwd" ]]; then
  project="$( (cd "$cwd" && git rev-parse --show-toplevel) 2>/dev/null || true)"
fi
agents_dir=""
[[ -n "$project" && -d "$project/.claude/agents" ]] && agents_dir="$project/.claude/agents"

case "$agent" in
  Explore|Plan) exit 0 ;;
  "<omitted>"|""|general-purpose|claude|fork) ;;
  *)
    [[ -n "$agents_dir" ]] || exit 0
    if [[ "$agent" =~ ^[a-z0-9][a-z0-9-]*$ && -f "$agents_dir/$agent.md" ]]; then
      exit 0
    fi
    ;;
esac

HH_COMMAND="$tool(subagent_type=$agent)"
hh_block "this session is running /run-spec or /implement, and a dispatch without a project agent runs on the session's own model: the agent's model and effort pins never engage (knowledge/agent-routing.md)." \
  "Name the agent the task's Domain/agent line gives, e.g. subagent_type=\"go-implementer\" (agents: .claude/agents/), and pass no model. Read-only research may use Explore or Plan."
