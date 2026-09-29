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
# Fails open: missing jq, an unreadable payload or transcript, or no project
# directory allows the dispatch. The guard enforces a routing convention, not a
# safety property, and must never wedge dispatch; the routing lint and the
# run summary's agent column are the backstop.

set -euo pipefail

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hook-helpers.sh
. "$HOOK_DIR/hook-helpers.sh"

command -v "${HOOK_JQ_PROBE:-jq}" >/dev/null 2>&1 || exit 0
input="$(cat)"
fields="$(printf '%s' "$input" | jq -r '[
    (.tool_name // ""),
    (if (.tool_input // {}) | has("subagent_type") then (.tool_input.subagent_type // "") else "<omitted>" end),
    (.transcript_path // ""),
    (.cwd // "")
  ] | map(gsub("[\u001f\n]"; " ")) | join("\u001f")' 2>/dev/null)" || exit 0
# A non-whitespace separator, so an empty subagent_type stays an empty field.
IFS=$'\x1f' read -r tool agent transcript cwd <<<"$fields"
[[ "$tool" == Agent || "$tool" == Task ]] || exit 0
[[ -n "$transcript" && -r "$transcript" ]] || exit 0

grep -Eq '"skill" *: *"(run-spec|implement)"|command-name>/?(run-spec|implement)<' "$transcript" 2>/dev/null || exit 0

project="${CLAUDE_PROJECT_DIR:-}"
if [[ -z "$project" && -n "$cwd" ]]; then
  project="$( (cd "$cwd" && git rev-parse --show-toplevel) 2>/dev/null || true)"
fi
[[ -n "$project" && -d "$project/.claude/agents" ]] || exit 0

case "$agent" in
  Explore|Plan) exit 0 ;;
  "<omitted>"|""|general-purpose|claude|fork) ;;
  *)
    if [[ "$agent" =~ ^[a-z0-9][a-z0-9-]*$ && -f "$project/.claude/agents/$agent.md" ]]; then
      exit 0
    fi
    ;;
esac

HH_GUARD=guard-dispatch-pin
HH_COMMAND="$tool(subagent_type=$agent)"
hh_block "this session is running /run-spec or /implement, and a dispatch without a project agent runs on the session's own model: the agent's model and effort pins never engage (knowledge/agent-routing.md)." \
  "Name the agent the task's Domain/agent line gives, e.g. subagent_type=\"go-implementer\" (agents: .claude/agents/), and pass no model. Read-only research may use Explore or Plan."
