#!/usr/bin/env bash
# shellcheck disable=SC2016  # fixture text is literal data
# test_lint_agent_harness.sh: lint-agent-harness.sh passes a clean fixture harness
# and fails each check on the broken input that check exists to catch. Every check
# has at least one kept broken fixture (the teeth) and, where a look-alike could be
# mistaken for a violation, a control that must stay clean. Leak fixtures are
# assembled at run time, so this file itself stays clean for the real checks.

# shellcheck source=../../../.claude/hooks/tests/lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/../../../.claude/hooks/tests/lib.sh"
LINT="$REPO_ROOT/scripts/ci/lint-agent-harness.sh"

# fixture <dir>: a minimal harness that passes every check.
fixture() {
  local d="$1"
  new_repo "$d"
  mkdir -p "$d/.claude/agents" "$d/.claude/rules" "$d/.claude/skills/demo" \
    "$d/.claude/hooks" "$d/knowledge" "$d/src"
  cp "$REPO_ROOT/.claude/hooks/validate-agent-config.sh" "$d/.claude/hooks/"
  printf 'package main\n' > "$d/src/main.go"
  printf '# Fixture\n\nSee [routing](knowledge/agent-routing.md).\n' > "$d/CLAUDE.md"
  cat > "$d/knowledge/agent-routing.md" <<'EOF'
# Agent routing

## Canonical agent → model mapping

| Agent | Model | Tier | Rationale |
|---|---|---|---|
| `worker` | `sonnet` | Implementer | writes code |
| `clerk` | `haiku` | Mechanical | counts things |

## Tier effort defaults

| Tier | Default effort | Rationale |
|---|---|---|
| `opus` (planner) | `xhigh` | recall |
| `sonnet` (implementer) | `high` | enough |
| `haiku` (mechanical) | *(none — never pass `effort`)* | rejects it |

## Rationale

Text.
EOF
  cat > "$d/.claude/agents/worker.md" <<'EOF'
---
name: worker
description: Writes code.
model: sonnet
effort: high
---

# Worker
EOF
  cat > "$d/.claude/agents/clerk.md" <<'EOF'
---
name: clerk
description: Counts things.
model: haiku
tools: [Read, Grep, Bash]
---

# Clerk
EOF
  printf '# Always\n\nDo the thing.\n' > "$d/.claude/rules/always.md"
  cat > "$d/.claude/rules/scoped.md" <<'EOF'
---
paths:
  - "**/*.go"
  # future: the later/ tree lands with a later task
  - "later/**"
---

# Scoped

Only for Go.
EOF
  cat > "$d/.claude/skills/demo/SKILL.md" <<'EOF'
---
name: demo
description: A demonstration skill.
---

# Demo

Reads `knowledge/agent-routing.md` and [the routing page](../../../knowledge/agent-routing.md).
Placeholders such as `knowledge/<topic>.md` and globs such as `specs/*/tasks.md` are not paths.
Costs are written \$0 when escaped.

## Invocation contexts

- **Slash command**: runs.
- **Model-invoked**: runs.
- **Non-interactive**: runs.

## Steps

```text
Agent(subagent_type: "clerk", prompt: "Run the count; never pass effort.")
```
EOF
  cat > "$d/.claude/settings.json" <<'EOF'
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          { "type": "command", "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/validate-agent-config.sh", "timeout": 10 }
        ]
      }
    ]
  }
}
EOF
  cat > "$d/.claude/hooks/INVENTORY.md" <<'EOF'
# Hook inventory

| Hook | Event / matcher | Posture | What it does | Tests |
|---|---|---|---|---|
| `validate-agent-config.sh` | PreToolUse / `Edit\|Write` | Blocks | Validates config. | none |

## Sourced helpers and scripts

| File | Role |
|---|---|
EOF
}

# lint <dir> [args...]: sets RC and OUT.
lint() {
  local d="$1"
  shift
  OUT="$("$LINT" --root "$d" "$@" 2>&1)"
  RC=$?
}

# expect_fail <check> <needle> <description>: the last lint failed that check with the needle.
expect_fail() {
  CHECKS=$((CHECKS + 1))
  [[ "$RC" == 1 && "$OUT" == *"$1"*"FAIL"* && "$OUT" == *"$2"* ]] \
    || fail "[$3] expected $1 to fail with '$2' (rc $RC): ${OUT:0:1500}"
}

# expect_pass <description>: the last lint passed.
expect_pass() {
  CHECKS=$((CHECKS + 1))
  [[ "$RC" == 0 ]] || fail "[$1] expected a clean lint (rc $RC): ${OUT:0:1500}"
}

# fresh <name>: a new clean fixture at $TEST_TMP/<name>; sets F.
fresh() {
  F="$TEST_TMP/$1"
  fixture "$F"
}

# set_line <file> <old> <new>: replaces a whole line (fixture edits, no sed -i).
set_line() {
  python3 - "$1" "$2" "$3" <<'EOF'
import sys
path, old, new = sys.argv[1:]
lines = open(path).read().split("\n")
assert old in lines, (old, path)
open(path, "w").write("\n".join(new if l == old else l for l in lines))
EOF
}

echo "== clean fixture =="
fresh clean
lint "$F"
expect_pass "the clean fixture passes every check"
check "summary names all ten checks" [ "$(grep -cE ' (PASS|FAIL)$' <<< "$OUT")" == 10 ]

echo "== usage =="
lint "$F" --only no-such-check
check "unknown check is a usage error" [ "$RC" == 2 ]
lint "$F" --only routing-pins,abs-paths
check "--only runs just the named checks" [ "$RC" == 0 ] && check "--only prints two rows" [ "$(grep -cE ' PASS$' <<< "$OUT")" == 2 ]

echo "== fail closed =="
BIN="$TEST_TMP/bin"
mkdir -p "$BIN"
cp "$LINT" "$BIN/lint-agent-harness.sh"
printf 'import sys\nsys.exit(1)\n' > "$BIN/harness_lint.py"
OUT="$("$BIN/lint-agent-harness.sh" --root "$TEST_TMP/clean" 2>&1)"; RC=$?
check "a failing check list is an error, not a pass" [ "$RC" == 2 ]
printf 'pass\n' > "$BIN/harness_lint.py"
OUT="$("$BIN/lint-agent-harness.sh" --root "$TEST_TMP/clean" 2>&1)"; RC=$?
check "an empty check list is an error, not a pass" [ "$RC" == 2 ]
check "an empty check list is named" grep -q "returned no checks" <<< "$OUT"
NOGIT="$TEST_TMP/nogit"
mkdir -p "$NOGIT/.claude"
lint "$NOGIT" --only abs-paths
check "a tree git cannot list is an error, not a pass" [ "$RC" == 1 ] && check "the tree error is reported" grep -q "cannot list files" <<< "$OUT"
check "the check shows as an error" grep -qE "abs-paths +ERROR\(2\)" <<< "$OUT"
fresh symlink
ln -s /etc/hostname "$F/knowledge/outside.md"
lint "$F" --only abs-paths
expect_pass "a symlink out of the tree is skipped, not read"
check "the skipped symlink is reported" grep -q "symlink resolves outside the tree" <<< "$OUT"

echo "== frontmatter =="
fresh fm-name
set_line "$F/.claude/agents/worker.md" "name: worker" "name: wrong"
lint "$F" --only frontmatter
expect_fail frontmatter "does not match the file name" "agent name differs from its file"
fresh fm-rule
printf -- '---\npaths: ["src/[a"]\n---\n# Bad\n' > "$F/.claude/rules/bad.md"
lint "$F" --only frontmatter
expect_fail frontmatter "unterminated [ character class" "rule glob does not compile"

echo "== routing-pins =="
fresh rp-mismatch
set_line "$F/.claude/agents/worker.md" "model: sonnet" "model: opus"
lint "$F" --only routing-pins
expect_fail routing-pins "PIN-MISMATCH" "model differs from the table"
fresh rp-row
set_line "$F/knowledge/agent-routing.md" '| `worker` | `sonnet` | Implementer | writes code |' ""
lint "$F" --only routing-pins
expect_fail routing-pins "MISSING-ROW" "agent without a table row"
fresh rp-file
rm "$F/.claude/agents/clerk.md"
lint "$F" --only routing-pins
expect_fail routing-pins "MISSING-FILE" "table row without an agent file"
fresh rp-effort
set_line "$F/.claude/agents/worker.md" "effort: high" ""
lint "$F" --only routing-pins
expect_fail routing-pins "MISSING-EFFORT" "sonnet agent without effort"
fresh rp-effort-mismatch
set_line "$F/.claude/agents/worker.md" "effort: high" "effort: xhigh"
lint "$F" --only routing-pins
expect_fail routing-pins "EFFORT-MISMATCH" "sonnet agent with the opus effort"
fresh rp-haiku-effort
set_line "$F/.claude/agents/clerk.md" "model: haiku" $'model: haiku\neffort: low'
lint "$F" --only routing-pins
expect_fail routing-pins "EFFORT-FORBIDDEN" "haiku agent pinning effort"
fresh rp-haiku-agent
set_line "$F/.claude/agents/clerk.md" "tools: [Read, Grep, Bash]" "tools: [Read, Grep, Bash, Agent]"
lint "$F" --only routing-pins
expect_fail routing-pins "HAIKU-TOOLS" "haiku agent with the Agent tool"
fresh rp-haiku-notools
set_line "$F/.claude/agents/clerk.md" "tools: [Read, Grep, Bash]" ""
lint "$F" --only routing-pins
expect_fail routing-pins "HAIKU-TOOLS" "haiku agent with no tools allowlist"
fresh rp-unparsed
set_line "$F/knowledge/agent-routing.md" "## Canonical agent → model mapping" "## Agents"
lint "$F" --only routing-pins
expect_fail routing-pins "no rows parsed" "renamed table section fails closed"

echo "== haiku-effort =="
fresh he-subagent
cat >> "$F/.claude/skills/demo/SKILL.md" <<'EOF'

```text
Agent(subagent_type: "clerk", effort: "low", prompt: "count")
```
EOF
lint "$F" --only haiku-effort
expect_fail haiku-effort "belongs to the haiku dispatch" "named haiku dispatch with effort"
fresh he-inline
printf '\nDispatch with `model: haiku` and\n`effort: medium` for the count.\n' >> "$F/.claude/agents/worker.md"
lint "$F" --only haiku-effort
expect_fail haiku-effort "belongs to the haiku dispatch" "inline haiku model with effort"
fresh he-sonnet
printf '\nAgent(subagent_type: "worker", effort: "high")\n' >> "$F/.claude/skills/demo/SKILL.md"
lint "$F" --only haiku-effort
expect_pass "effort on a sonnet dispatch is allowed"

echo "== rule-budget =="
fresh rb-over
python3 -c 'print("# Big\n\n" + "x" * 20000)' > "$F/.claude/rules/big.md"
lint "$F" --only rule-budget
expect_fail rule-budget "exceed the budget" "oversized always-on rule"
printf -- '---\npaths:\n  - "src/**"\n---\n' | cat - "$F/.claude/rules/big.md" > "$F/big.tmp" && mv "$F/big.tmp" "$F/.claude/rules/big.md"
lint "$F" --only rule-budget
expect_pass "the same rule made path-conditional does not count"

echo "== paths-globs =="
fresh pg-dead
set_line "$F/.claude/rules/scoped.md" '  - "**/*.go"' '  - "nowhere/**/*.go"'
lint "$F" --only paths-globs
expect_fail paths-globs "matches no file" "unmarked glob matching nothing"
fresh pg-future-live
mkdir -p "$F/later" && printf 'x\n' > "$F/later/file.txt"
lint "$F" --only paths-globs
expect_pass "a future glob that now matches only warns"
check "stale future marker is reported" grep -q "delete its '# future' marker" <<< "$OUT"
fresh pg-comment
set_line "$F/.claude/rules/scoped.md" "  # future: the later/ tree lands with a later task" "  # unrelated comment"
lint "$F" --only paths-globs
expect_fail paths-globs "'later/**' matches no file" "a comment that is not a future marker does not exempt"

echo "== hook-inventory =="
fresh hi-row
set_line "$F/.claude/hooks/INVENTORY.md" '| `validate-agent-config.sh` | PreToolUse / `Edit\|Write` | Blocks | Validates config. | none |' ""
lint "$F" --only hook-inventory
expect_fail hook-inventory "has no inventory row" "registered hook missing from the inventory"
fresh hi-matcher
set_line "$F/.claude/hooks/INVENTORY.md" '| `validate-agent-config.sh` | PreToolUse / `Edit\|Write` | Blocks | Validates config. | none |' '| `validate-agent-config.sh` | PreToolUse / `Bash` | Blocks | Validates config. | none |'
lint "$F" --only hook-inventory
expect_fail hook-inventory "inventory says" "inventory matcher differs from settings"
fresh hi-stray
printf '#!/usr/bin/env bash\nexit 0\n' > "$F/.claude/hooks/stray.sh"
lint "$F" --only hook-inventory
expect_fail hook-inventory "neither registered nor listed" "unregistered hook script"
printf '| `stray.sh` | Sourced by other hooks. |\n' >> "$F/.claude/hooks/INVENTORY.md"
lint "$F" --only hook-inventory
expect_pass "a script listed as a sourced helper is allowed"

echo "== skill-contexts =="
fresh sc-none
set_line "$F/.claude/skills/demo/SKILL.md" "## Invocation contexts" "## Contexts"
lint "$F" --only skill-contexts
expect_fail skill-contexts "no '## Invocation contexts' section" "skill without the section"
fresh sc-label
set_line "$F/.claude/skills/demo/SKILL.md" "- **Non-interactive**: runs." "- Headless: runs."
lint "$F" --only skill-contexts
expect_fail skill-contexts "does not name **Non-interactive**" "section missing one context"

echo "== dollar-zero =="
fresh dz
printf '\nIt costs $%s.02 per call.\n' 0 >> "$F/.claude/skills/demo/SKILL.md"
lint "$F" --only dollar-zero
expect_fail dollar-zero "unescaped" "bare dollar-zero in a skill"

echo "== abs-paths =="
fresh ap
H="ho""me"
printf '\nLogs live in /%s/alice/logs.\n' "$H" >> "$F/knowledge/agent-routing.md"
lint "$F" --only abs-paths
expect_fail abs-paths "absolute home path" "home path in a harness file"
fresh ap-placeholder
printf '\nNever write /%s/<user> paths; use $HOME/x.\n' "$H" >> "$F/CLAUDE.md"
lint "$F" --only abs-paths
expect_pass "placeholder and HOME-relative paths are allowed"

echo "== references =="
fresh rf-link
printf '\nSee [gone](knowledge/gone.md).\n' >> "$F/CLAUDE.md"
lint "$F" --only references
expect_fail references "link target 'knowledge/gone.md' does not exist" "broken markdown link"
fresh rf-path
printf '\nRead `knowledge/missing.md` first.\n' >> "$F/.claude/agents/worker.md"
lint "$F" --only references
expect_fail references "path 'knowledge/missing.md' does not exist" "missing backticked path"
fresh rf-fence
printf '\n```text\nSee [gone](knowledge/gone.md) and `knowledge/missing.md`.\n```\n' >> "$F/CLAUDE.md"
lint "$F" --only references
expect_pass "references inside a code fence are examples"

finish
