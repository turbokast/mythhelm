#!/usr/bin/env bash
# test_verify_task_completion.sh: the Stop/SubagentStop completion gate blocks a
# session that marked a task complete until the entry is well-formed and the
# change set's gate markers are fresh, and leaves every other stop alone: no
# claim, another session's claim, a claim already on the base. Also its bounded
# release, the audited override, worktrees and the fail-open paths.

# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
HOOK="$HOOKS_DIR/verify-task-completion.sh"
GATE="$REPO_ROOT/scripts/harness/gate.sh"
LIB="$REPO_ROOT/scripts/harness/gatelib.py"

BIN="$TEST_TMP/bin"
mkdir -p "$BIN"
for tool in go gofmt; do
  printf '#!/usr/bin/env bash\nexit 0\n' > "$BIN/$tool"
  chmod +x "$BIN/$tool"
done
export PATH="$BIN:$PATH"

# shellcheck disable=SC2016  # backticks are Markdown, not command substitution
TASK1='### Task 1 — Journal

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Change**: Journal.
- **Files**:
  - `internal/x/x.go`
- **Acceptance**:
  - tests pass.
- **Invariants touched**: I09.
'
# shellcheck disable=SC2016  # backticks are Markdown
DONE_ENTRY='- **Status**: ✅ Completed — journal; PR #5.
- **Implementation**: Append-only journal. Commit abc1234.
- **Spec deviations**: None.
- **Files modified**: `internal/x/x.go`, `specs/in-progress/demo/tasks.md`.
'

# repo <name>: a clone of a bare origin with a spec whose Task 1 is open.
repo() {
  R="$TEST_TMP/$1"
  local origin="$TEST_TMP/$1-origin.git"
  git init -q --bare -b main "$origin"
  new_repo "$R"
  git -C "$R" remote add origin "$origin"
  mkdir -p "$R/internal/x" "$R/specs/in-progress/demo" "$R/scripts/ci"
  printf '.claude/data/*\n' > "$R/.gitignore"
  printf 'package x\n' > "$R/internal/x/x.go"
  printf '%s' "$TASK1" > "$R/specs/in-progress/demo/tasks.md"
  cp "$REPO_ROOT/scripts/ci/check-public-hygiene.sh" "$R/scripts/ci/"
  git -C "$R" add -A
  git -C "$R" commit -q -m fixture
  git -C "$R" push -q origin main
  TR="$TEST_TMP/$1-transcript.jsonl"
  : > "$TR"
}

claim() {   # claim [entry]: marks Task 1 complete in the working tree
  printf '%s' "${TASK1/'### Task 1 — Journal'/'### Task 1 — Journal ✅ COMPLETED'}${1-$DONE_ENTRY}" \
    > "$R/specs/in-progress/demo/tasks.md"
}

edited_by_session() {   # the transcript records an Edit of the tasks.md
  jq -nc --arg p "$R/specs/in-progress/demo/tasks.md" \
    '{type:"assistant",message:{content:[{type:"tool_use",name:"Edit",input:{file_path:$p,old_string:"a",new_string:"b"}}]}}' >> "$TR"
}

payload() {   # payload [event] [stop_hook_active] [transcript-field]: sets P
  local ev="${1:-Stop}" active="${2:-false}" field="${3:-transcript_path}"
  P="$(jq -nc --arg ev "$ev" --arg cwd "$R" --arg tr "$TR" --arg f "$field" --argjson a "$active" \
    '{hook_event_name:$ev,session_id:"s1",cwd:$cwd,stop_hook_active:$a} + {($f):$tr}')"
}

gates() { (cd "$R" && "$GATE" "$@" >/dev/null 2>&1); }

AUDIT() { cat "$R/.claude/data/stop-gate-audit.jsonl" 2>/dev/null; }

echo "== no claim, no block =="
repo none
echo "// edit" >> "$R/internal/x/x.go"
payload; expect_rc 0 "an unclaimed Go edit with no markers stops freely" "$HOOK" "$P"
check "nothing was audited" [ -z "$(AUDIT)" ]
payload SubagentStop false agent_transcript_path
expect_rc 0 "the same on SubagentStop" "$HOOK" "$P"

echo "== a claim needs fresh markers =="
repo claim
echo "// edit" >> "$R/internal/x/x.go"
claim; edited_by_session
payload; expect_rc 2 "a claim with no markers blocks" "$HOOK" "$P"
expect_err "BLOCK: verify-task-completion" "the stanza names the hook"
expect_err "File: specs/in-progress/demo/tasks.md" "and the tasks.md"
expect_err "demo#1" "and the claimed task"
expect_err "gate go-test: marker missing" "and each missing gate"
expect_err "scripts/harness/gate.sh go-test" "the fix names the wrapper command"
expect_err "gatelib.py override --session s1" "and the escape hatch for this session"
check "the block is audited in the main checkout" grep -q '"event":"block"' "$R/.claude/data/stop-gate-audit.jsonl"
gates go
gates hygiene
payload; expect_rc 0 "fresh Go and hygiene markers let it stop" "$HOOK" "$P"
echo "// later" >> "$R/internal/x/x.go"
payload; expect_rc 2 "an edit after the gates makes the markers stale" "$HOOK" "$P"
expect_err "gate go-vet: marker stale" "stale, not missing"

echo "== the entry must be well-formed =="
repo entry
claim '- **Status**: ✅ Completed — journal.
'
edited_by_session
gates hygiene
payload; expect_rc 2 "a claim with a thin entry blocks even with fresh markers" "$HOOK" "$P"
expect_err "no '- **Implementation**:' field" "names the missing field"
expect_err "Status names no 'PR #<n>'" "and the missing PR"
expect_err "task-completion/SKILL.md" "and points at the format"
claim; gates hygiene
payload; expect_rc 0 "the full entry with fresh hygiene stops" "$HOOK" "$P"

echo "== attribution: only this session's claim =="
repo other
claim
payload; expect_rc 0 "a claim this session never edited (another session's) does not block" "$HOOK" "$P"
jq -nc --arg c "python3 fix.py specs/in-progress/demo/tasks.md" \
  '{type:"assistant",message:{content:[{type:"tool_use",name:"Bash",input:{command:$c}}]}}' >> "$TR"
payload; expect_rc 2 "a Bash command naming the file attributes it" "$HOOK" "$P"
payload SubagentStop false transcript_path
expect_rc 2 "a SubagentStop with no agent transcript cannot rule the claim out" "$HOOK" "$P"
rm -f "$TR"
payload; expect_rc 2 "an unreadable transcript counts as this session's edit" "$HOOK" "$P"

echo "== a claim already on the base is not a new claim =="
repo base
claim
git -C "$R" commit -q -am "task 1 done"
git -C "$R" push -q origin main
edited_by_session
echo "// unrelated" >> "$R/internal/x/x.go"
payload; expect_rc 0 "the completion is on origin/main already" "$HOOK" "$P"
repo base-local
claim
git -C "$R" commit -q -am "task 1 done, not pushed"
edited_by_session
payload; expect_rc 2 "a committed but unmerged claim is still this branch's claim" "$HOOK" "$P"

echo "== bounded release =="
repo loop
claim; edited_by_session
payload Stop false; expect_rc 2 "block 1" "$HOOK" "$P"
payload Stop true; expect_rc 2 "block 2 (stop_hook_active)" "$HOOK" "$P"
payload Stop true; expect_rc 2 "block 3" "$HOOK" "$P"
payload Stop true; expect_rc 0 "released after three blocks on an unchanged tree" "$HOOK" "$P"
check "the release reaches the user" grep -q 'systemMessage' <<<"$OUT"
check "the release is audited" grep -q '"event":"released_after_repeats"' "$R/.claude/data/stop-gate-audit.jsonl"
echo "// new work" >> "$R/internal/x/x.go"
payload Stop true; expect_rc 2 "a changed tree resets the count" "$HOOK" "$P"

echo "== override: recorded, bound to session, tree and bytes =="
repo override
claim; edited_by_session
OUT="$(cd "$R" && python3 "$LIB" override --session s1 --reason "short" 2>&1)"; RC=$?
check "a reason under ten characters is refused" [ "$RC" == 2 ]
(cd "$R" && python3 "$LIB" override --session s1 --reason "go toolchain unavailable on this machine" >/dev/null)
payload; expect_rc 0 "the override lets this session stop" "$HOOK" "$P"
check "the override was audited as recorded and honoured" \
  bash -c "grep -q override_recorded '$R/.claude/data/stop-gate-audit.jsonl' && grep -q override_honored '$R/.claude/data/stop-gate-audit.jsonl'"
P2="$(jq -c '.session_id = "s2"' <<<"$P")"
expect_rc 2 "another session's stop is not covered" "$HOOK" "$P2"
echo "// more" >> "$R/internal/x/x.go"
payload; expect_rc 2 "any change to the tree lapses the override" "$HOOK" "$P"

echo "== linked worktrees =="
repo wt
WT="$TEST_TMP/wt-linked"
git -C "$R" worktree add -q -b feat/demo-t1 "$WT" origin/main
MAIN="$R"
R="$WT"
claim; edited_by_session
echo "// wt" >> "$R/internal/x/x.go"
payload SubagentStop false agent_transcript_path
expect_rc 2 "a worker's claim in its worktree blocks its SubagentStop" "$HOOK" "$P"
check "the audit lands in the main checkout, which outlives the worktree" \
  grep -q '"event":"block"' "$MAIN/.claude/data/stop-gate-audit.jsonl"
gates go; gates hygiene
check "markers land in the worktree" [ -f "$WT/.claude/data/gate-marker-go-test.json" ]
payload SubagentStop false agent_transcript_path
expect_rc 0 "the worktree's own markers clear it" "$HOOK" "$P"
R="$MAIN"
payload; expect_rc 0 "the orchestrator's main checkout has no claim" "$HOOK" "$P"

echo "== the checker cannot decide: fail closed on a possible claim, never trap =="
repo failclosed
claim; edited_by_session
payload
OUT="$(printf '%s' "$P" | HOOK_PYTHON_PROBE=no-such-python "$HOOK" 2>"$TEST_TMP/err")"; RC=$?
check "python3 missing with a changed tasks.md blocks" [ "$RC" == 2 ]
check "the block is the stanza naming the tasks.md" grep -q '^File: specs/in-progress/demo/tasks.md' "$TEST_TMP/err"
P_NESTED="$(jq -c --arg d "$R/internal/x" '.cwd = $d' <<<"$P")"
printf '%s' "$P_NESTED" | HOOK_PYTHON_PROBE=no-such-python "$HOOK" >/dev/null 2>&1; RC=$?
check "a cwd nested inside the tree still finds the changed tasks.md" [ "$RC" == 2 ]
printf '%s' "$P" | HOOK_PYTHON_PROBE=no-such-python HOOK_GIT_PROBE=no-such-git "$HOOK" >/dev/null 2>&1; RC=$?
check "python3 and git both missing blocks: a claim cannot be ruled out" [ "$RC" == 2 ]
P_GONE="$(jq -c --arg d "$TEST_TMP/no-such-dir" '.cwd = $d' <<<"$P")"
printf '%s' "$P_GONE" | HOOK_PYTHON_PROBE=no-such-python "$HOOK" >/dev/null 2>&1; RC=$?
check "a cwd git cannot resolve blocks" [ "$RC" == 2 ]
UNBORN="$TEST_TMP/unborn"
git init -q -b main "$UNBORN"
printf '%s' "$(jq -c --arg d "$UNBORN" '.cwd = $d' <<<"$P")" | HOOK_PYTHON_PROBE=no-such-python "$HOOK" >/dev/null 2>&1; RC=$?
check "a listing git cannot produce (no commit to diff against) blocks" [ "$RC" == 2 ]
payload Stop true
OUT="$(printf '%s' "$P" | HOOK_PYTHON_PROBE=no-such-python "$HOOK" 2>/dev/null)"; RC=$?
check "the stop after it is allowed" [ "$RC" == 0 ]
check "with a notice to the user" grep -q 'systemMessage' <<<"$OUT"
payload
mkdir -p "$R/.claude" && : > "$R/.claude/data"
expect_rc 2 "an internal error (state directory unwritable) with a possible claim blocks" "$HOOK" "$P"
expect_err "the checker could not decide (exit 3: internal error" "the block says why"
rm -f "$R/.claude/data"
repo noclaim-fault
echo "// edit" >> "$R/internal/x/x.go"
payload
OUT="$(printf '%s' "$P" | HOOK_PYTHON_PROBE=no-such-python "$HOOK" 2>/dev/null)"; RC=$?
check "python3 missing with no tasks.md change allows" [ "$RC" == 0 ]
check "and tells the user the gate did not run" grep -q 'did not run' <<<"$OUT"
expect_rc 0 "a payload that is not JSON, from a directory outside any repository, is allowed" "$HOOK" "not json"
OUT="$(printf '%s' "$(jq -c --arg d "$TEST_TMP" '.cwd = $d' <<<"$P")" | HOOK_PYTHON_PROBE=no-such-python "$HOOK" 2>/dev/null)"; RC=$?
check "python3 missing in a directory outside any repository allows" [ "$RC" == 0 ]
P3="$(jq -c --arg d "$TEST_TMP" '.cwd = $d' <<<"$P")"
expect_rc 0 "a cwd outside any repository is allowed" "$HOOK" "$P3"

finish
