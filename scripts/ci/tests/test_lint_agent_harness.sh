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
  spec_fixture "$d"
}

# spec_fixture <dir>: a lifecycle tree with one refined idea, one full spec in todo/
# and an epic plan naming it, all clean.
spec_fixture() {
  local d="$1" s
  for s in unrefined refined todo in-progress unfinalized "done" archived; do
    mkdir -p "$d/specs/$s"
  done
  touch "$d/specs/in-progress/.gitkeep" "$d/specs/unfinalized/.gitkeep" \
    "$d/specs/done/.gitkeep" "$d/specs/archived/.gitkeep"
  printf '# Specs\n' > "$d/specs/README.md"
  mkdir -p "$d/specs/refined/idea" "$d/specs/unrefined/epic-x" "$d/specs/todo/demo"
  printf '## Idea — Requirements\n' > "$d/specs/refined/idea/requirements.md"
  cat > "$d/specs/unrefined/epic-x/plan.md" <<'EOF'
## Epic X — Master Plan

### Work Streams

| # | Spec | Scope | Dependencies |
|---|---|---|---|
| 1 | `demo` | the demo | None |
| 2 | `idea` | the idea | Spec 1 |

### Open Questions

- None.
EOF
  printf '## Demo — Requirements\n' > "$d/specs/todo/demo/requirements.md"
  printf '## Demo — Design\n' > "$d/specs/todo/demo/design.md"
  cat > "$d/specs/todo/demo/tasks.md" <<'EOF'
## Demo — Tasks

### Dependencies

- None outside this spec.

## Implementation Tasks

### Task 1 — Types ✅ COMPLETED

- **Domain/agent**: worker
- **Budget**: standard
- **Change**: Add the types.
- **Files**:
  - `src/types.go`
- **Acceptance**:
  - `TestTypes` passes.
- **Invariants touched**: I02 (unknown blocks).
- **Status**: ✅ Completed.

### Task 2 — Store

- **Domain/agent**: `worker`
- **Budget**: complex (the hard one)
- **Depends on**: Task 1 (types first)
- **Change**: Add the store.
- **Files**: `src/store.go`
- **Acceptance criteria** (each test uses a temp dir): `TestStore` passes.
- **Invariants touched**: None (no persisted user data).

```markdown
### Task 9 — An example inside a fence is not a task
```

### Task 3 — Command

- **Domain/agent**: maintainer (human), assisted by worker
- **Budget**: standard (wiring only)
- **Depends on**: Task 1, Task 2
- **Change**: Wire the command.
- **Files**:
  - `src/cmd.go`
- **Acceptance**:
  - `TestCommand` passes.
- **Invariants touched**: I08.

## Open Questions

- None.
EOF
}

# set_task_line <old> <new>: replaces a whole line of the fixture's tasks.md.
set_task_line() { set_line "$F/specs/todo/demo/tasks.md" "$1" "$2"; }

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
check "summary names all eleven checks" [ "$(grep -cE ' (PASS|FAIL)$' <<< "$OUT")" == 11 ]

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

echo "== specs =="
fresh sp-required
rm "$F/specs/todo/demo/design.md"
lint "$F" --only specs
expect_fail specs "a spec in todo/ needs design.md" "todo spec without design.md"
fresh sp-refined-required
rm "$F/specs/refined/idea/requirements.md" && printf 'x\n' > "$F/specs/refined/idea/notes.md"
lint "$F" --only specs
expect_fail specs "a spec in refined/ needs requirements.md" "refined spec without requirements.md"
fresh sp-state
mkdir -p "$F/specs/backlog/x" && printf 'x\n' > "$F/specs/backlog/x/requirements.md"
lint "$F" --only specs
expect_fail specs "'backlog' is not a lifecycle state" "unknown lifecycle directory"
fresh sp-stray
printf 'x\n' > "$F/specs/todo/notes.md"
lint "$F" --only specs
expect_fail specs "is not in a spec directory" "file directly under a state directory"
fresh sp-flat
printf 'x\n' > "$F/specs/notes.md"
lint "$F" --only specs
expect_fail specs "only README.md sits directly under specs/" "file directly under specs/"
fresh sp-duplicate
mkdir -p "$F/specs/in-progress/demo" && cp "$F/specs/todo/demo/"*.md "$F/specs/in-progress/demo/"
lint "$F" --only specs
expect_fail specs "exists in 2 lifecycle states (in-progress, todo)" "one spec in two states"
fresh sp-files
set_task_line '- **Files**: `src/store.go`' ''
lint "$F" --only specs
expect_fail specs "Task 2 has no '- **Files**:' field" "task without Files"
fresh sp-invariants
set_task_line '- **Invariants touched**: I08.' ''
lint "$F" --only specs
expect_fail specs "Task 3 has no '- **Invariants touched**:' field" "task without Invariants touched"
fresh sp-empty
set_task_line '- **Acceptance criteria** (each test uses a temp dir): `TestStore` passes.' '- **Acceptance**:'
lint "$F" --only specs
expect_fail specs "Task 2: 'Acceptance' is empty" "task with an empty Acceptance"
fresh sp-depends-missing
set_task_line '- **Depends on**: Task 1 (types first)' ''
lint "$F" --only specs
expect_fail specs "Task 2 has no '- **Depends on**:' field" "a later task without Depends on"
fresh sp-first-depends
set_task_line '- **Budget**: standard' $'- **Budget**: standard\n- **Depends on**: None'
lint "$F" --only specs
expect_pass "the first task may state Depends on: None"
fresh sp-dangling
set_task_line '- **Depends on**: Task 1, Task 2' '- **Depends on**: Task 1, Task 7'
lint "$F" --only specs
expect_fail specs "Task 3 depends on Task 7, which does not exist" "dependency on a missing task"
fresh sp-self
set_task_line '- **Depends on**: Task 1 (types first)' '- **Depends on**: Task 2'
lint "$F" --only specs
expect_fail specs "Task 2 depends on itself" "self dependency"
fresh sp-cycle
set_task_line '- **Depends on**: Task 1 (types first)' '- **Depends on**: Task 3'
lint "$F" --only specs
expect_fail specs "dependency cycle: Task 2 -> Task 3 -> Task 2" "two-task cycle"
fresh sp-cycle-first
set_task_line '- **Budget**: standard' $'- **Budget**: standard\n- **Depends on**: Task 3'
lint "$F" --only specs
expect_fail specs "dependency cycle" "cycle through the first task"
fresh sp-prose
set_task_line '- **Depends on**: Task 1 (types first)' '- **Depends on**: after the types land'
lint "$F" --only specs
expect_fail specs "is not 'None' or a list of 'Task N'" "prose dependency"
fresh sp-after-note
set_task_line '- **Depends on**: Task 1, Task 2' '- **Depends on**: Task 1 (types first), Task 7'
lint "$F" --only specs
expect_fail specs "Task 3 depends on Task 7, which does not exist" "a reference after a note is still checked"
fresh sp-nested-note
set_task_line '- **Depends on**: Task 1, Task 2' '- **Depends on**: Task 1 (types (first)), Task 2'
lint "$F" --only specs
expect_fail specs "is not 'None' or a list of 'Task N'" "nested parentheses are unreadable"
fresh sp-agent
set_task_line '- **Domain/agent**: `worker`' '- **Domain/agent**: backend-implementer'
lint "$F" --only specs
expect_fail specs "Domain/agent 'backend-implementer' is not an agent" "unknown agent"
fresh sp-budget
set_task_line '- **Budget**: complex (the hard one)' '- **Budget**: trivial'
lint "$F" --only specs
expect_fail specs "Budget 'trivial' is not one of standard/complex" "unknown budget tier"
fresh sp-twice
set_task_line '### Task 3 — Command' '### Task 2 — Command'
lint "$F" --only specs
expect_fail specs "Task 2 is defined twice" "duplicate task number"
fresh sp-no-tasks
printf '## Demo — Tasks\n\nTBD.\n' > "$F/specs/todo/demo/tasks.md"
lint "$F" --only specs
expect_fail specs "no '### Task N' blocks" "tasks.md without tasks"
fresh sp-epic-missing
set_line "$F/specs/unrefined/epic-x/plan.md" '| 2 | `idea` | the idea | Spec 1 |' '| 2 | `ghost` | the ghost | Spec 1 |'
lint "$F" --only specs
expect_fail specs "work stream spec 'ghost' has no directory" "epic naming a missing spec"
fresh sp-epic-row
set_line "$F/specs/unrefined/epic-x/plan.md" '| 2 | `idea` | the idea | Spec 1 |' $'| 2 | `idea` | the idea | Spec 1 |\n| 3 | ghost | no backticks | Spec 2 |'
lint "$F" --only specs
expect_fail specs "plan.md:9: specs: Work Streams row has no backticked spec name" "an unreadable Work Streams row"
fresh sp-epic-state
mv "$F/specs/unrefined/epic-x" "$F/specs/todo/epic-x"
lint "$F" --only specs
expect_fail specs "an epic plan never enters todo/" "an epic plan in todo/"
fresh sp-epic-in-progress
mv "$F/specs/unrefined/epic-x" "$F/specs/in-progress/epic-x"
lint "$F" --only specs
expect_pass "an epic plan in in-progress/ is allowed"
fresh sp-index-copy
git -C "$F" add specs/todo/demo
mv "$F/specs/todo/demo" "$F/specs/in-progress/demo"
lint "$F" --only specs
expect_fail specs "exists in 2 lifecycle states (in-progress, todo)" "a copy left in the index by a plain mv"
OTHER="$TEST_TMP/other-repo"
new_repo "$OTHER"
OUT="$(GIT_DIR="$OTHER/.git" GIT_WORK_TREE="$OTHER" GIT_INDEX_FILE="$OTHER/.git/index" "$LINT" --root "$F" --only specs 2>&1)"; RC=$?
expect_fail specs "exists in 2 lifecycle states" "inherited GIT_DIR, GIT_WORK_TREE and GIT_INDEX_FILE do not hide the index copy"
git -C "$F" add -A specs
lint "$F" --only specs
expect_pass "the same move staged in the index is one copy"
fresh sp-epic-section
set_line "$F/specs/unrefined/epic-x/plan.md" '### Work Streams' '### Streams'
lint "$F" --only specs
expect_fail specs "epic plan has no '## Work Streams' section" "epic without a Work Streams section"

finish
