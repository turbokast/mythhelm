#!/usr/bin/env bash
# shellcheck disable=SC2016  # commands under test are literal data
# test_guard_product_write.sh: guard-product-write.sh and approvals.py. Agent writes
# to product/ pass only with a signed, unconsumed approval of that exact change;
# approvals are single-use, stale when the file moved on, and bound to the result;
# the ledger, the key and the maintainer verbs are out of an agent's reach; and
# read-only commands that merely mention product/ pass.

# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
HOOK="$HOOKS_DIR/guard-product-write.sh"
APPROVALS="$REPO_ROOT/scripts/orchestration/approvals.py"
PROJ="$TEST_TMP/proj"
new_repo "$PROJ"
mkdir -p "$PROJ/product" "$PROJ/docs"
printf '# Backlog\n\nline one\n' > "$PROJ/product/backlog.md"
printf '# Decisions\n' > "$PROJ/product/decisions.md"
git -C "$PROJ" add product
git -C "$PROJ" commit -q -m product
export CLAUDE_PROJECT_DIR="$PROJ"
export MYTHHELM_APPROVALS_KEY="$TEST_TMP/keys/approvals.key"
mkdir -p "$TEST_TMP/keys"
printf '%s\n' "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" > "$MYTHHELM_APPROVALS_KEY"
chmod 600 "$MYTHHELM_APPROVALS_KEY"
LEDGER="$PROJ/orchestration/approvals.jsonl"
SID="p-$$"

block() { expect_rc 2 "$1" "$HOOK" "$(bash_payload "$2" "${3:-$PROJ}" "$SID")"; }
allow() { expect_rc 0 "$1" "$HOOK" "$(bash_payload "$2" "${3:-$PROJ}" "$SID")"; }

# write_payload <tool> <path> <content> [cwd]
write_payload() {
  jq -nc --arg t "$1" --arg p "$2" --arg c "$3" --arg w "${4:-$PROJ}" \
    '{hook_event_name:"PreToolUse",tool_name:$t,session_id:"s",cwd:$w,tool_input:{file_path:$p,content:$c}}'
}
# edit_payload <path> <old> <new>
edit_payload() {
  jq -nc --arg p "$1" --arg o "$2" --arg n "$3" --arg w "$PROJ" \
    '{hook_event_name:"PreToolUse",tool_name:"Edit",session_id:"s",cwd:$w,tool_input:{file_path:$p,old_string:$o,new_string:$n}}'
}

# request <dir> <id> <path> <content>: an agent files a request from <dir>.
request() {
  printf '%s' "$4" > "$TEST_TMP/proposed-$2"
  (cd "$1" && python3 "$APPROVALS" request "$2" --path "$3" --proposed "$TEST_TMP/proposed-$2" --summary "test $2") >/dev/null
}

# maintainer <dir> <args...>: runs approvals.py under a pseudo-terminal, as a person would.
maintainer() {
  local dir="$1"
  shift
  local cmd
  cmd="cd $(printf '%q' "$dir") && python3 $(printf '%q' "$APPROVALS") $(printf '%q ' "$@")"
  if script --version >/dev/null 2>&1; then
    script -qec "$cmd" /dev/null </dev/null >/dev/null 2>&1
  else
    script -q /dev/null bash -c "$cmd" </dev/null >/dev/null 2>&1
  fi
}

echo "== writes need a signed approval of the exact change =="
NEW=$'# Backlog\n\nline two\n'
expect_rc 2 "unapproved Write" "$HOOK" "$(write_payload Write "$PROJ/product/backlog.md" "$NEW")"
check "the block is a four-line File stanza" [ "$(printf '%s\n' "$ERR" | sed -n 1p)" == "BLOCK: guard-product-write" ]
expect_err "File: $PROJ/product/backlog.md" "the stanza names the file"
expect_err "approvals.py request" "the fix names the request command"

request "$PROJ" a1 product/backlog.md "$NEW"
expect_rc 2 "an open request is not an approval" "$HOOK" "$(write_payload Write "$PROJ/product/backlog.md" "$NEW")"
maintainer "$PROJ" approve a1 --yes
check "approve under a terminal records a signed row" grep -q '"state": "approved"' "$LEDGER"
expect_rc 2 "approved, but different content" "$HOOK" "$(write_payload Write "$PROJ/product/backlog.md" $'# Backlog\n\nsomething else\n')"
expect_err "approves different content" "result mismatch is named"
expect_rc 0 "approved exact content" "$HOOK" "$(write_payload Write "$PROJ/product/backlog.md" "$NEW")"
check "the release is recorded as consumed" grep -q '"state": "consumed"' "$LEDGER"
expect_rc 2 "an approval releases one write" "$HOOK" "$(write_payload Write "$PROJ/product/backlog.md" "$NEW")"
expect_err "already used" "reuse is named"
expect_rc 2 "a relative path is resolved against cwd" "$HOOK" "$(write_payload Write "product/backlog.md" "$NEW")"

echo "== an approval binds the base =="
request "$PROJ" a2 product/decisions.md $'# Decisions\n\nD-1\n'
maintainer "$PROJ" approve a2 --yes
printf '# Decisions\n\nsomeone else\n' > "$PROJ/product/decisions.md"
expect_rc 2 "the file changed after approval" "$HOOK" "$(write_payload Write "$PROJ/product/decisions.md" $'# Decisions\n\nD-1\n')"
expect_err "stale" "a stale approval is named"
printf '# Decisions\n' > "$PROJ/product/decisions.md"
expect_rc 0 "the same base again releases it" "$HOOK" "$(write_payload Write "$PROJ/product/decisions.md" $'# Decisions\n\nD-1\n')"

echo "== Edit is matched by the content it leaves =="
printf '# Backlog\n\nline one\n' > "$PROJ/product/backlog.md"
request "$PROJ" e1 product/backlog.md $'# Backlog\n\nline ONE\n'
maintainer "$PROJ" approve e1 --yes
expect_rc 2 "an Edit leaving other content" "$HOOK" "$(edit_payload "$PROJ/product/backlog.md" "one" "uno")"
expect_rc 0 "an Edit leaving the approved content" "$HOOK" "$(edit_payload "$PROJ/product/backlog.md" "one" "ONE")"
expect_rc 2 "an Edit whose old_string is absent" "$HOOK" "$(edit_payload "$PROJ/product/backlog.md" "absent" "x")"
expect_err "cannot be computed" "an uncomputable result is named"

echo "== forged, rejected and tampered approvals do not release =="
request "$PROJ" f1 product/backlog.md "forged content"
jq -nc '{schema_version:1,id:"f1",state:"approved",path:"product/backlog.md",base_sha256:"x",result_sha256:"y",mac:"00"}' >> "$LEDGER"
expect_rc 2 "a hand-written approved row" "$HOOK" "$(write_payload Write "$PROJ/product/backlog.md" "forged content")"
expect_err "valid maintainer signature" "the forgery is named"
request "$PROJ" r1 product/backlog.md "rejected content"
maintainer "$PROJ" reject r1 --yes
expect_rc 2 "a rejected request" "$HOOK" "$(write_payload Write "$PROJ/product/backlog.md" "rejected content")"
request "$PROJ" t1 product/backlog.md "honest content"
printf 'tampered content' > "$PROJ/orchestration/requests/t1/proposed"
maintainer "$PROJ" approve t1 --yes
check "approve refuses a tampered request" bash -c "! grep -q '\"id\": \"t1\".*approved' '$LEDGER'"
request "$PROJ" s1 product/backlog.md "stale content"
printf 'moved on\n' > "$PROJ/product/backlog.md"
maintainer "$PROJ" approve s1 --yes
check "approve refuses a request whose file changed" bash -c "! grep -q '\"id\": \"s1\".*approved' '$LEDGER'"
rm -f "$MYTHHELM_APPROVALS_KEY.bak"
mv "$MYTHHELM_APPROVALS_KEY" "$MYTHHELM_APPROVALS_KEY.bak"
request "$PROJ" k1 product/backlog.md "keyless"
expect_rc 2 "no key, nothing verifies" "$HOOK" "$(write_payload Write "$PROJ/product/backlog.md" "keyless")"
mv "$MYTHHELM_APPROVALS_KEY.bak" "$MYTHHELM_APPROVALS_KEY"

echo "== the maintainer verbs need a terminal =="
OUT="$(cd "$PROJ" && python3 "$APPROVALS" approve k1 --yes 2>&1 </dev/null)"; RC=$?
check "approve without a terminal is refused" [ "$RC" == 1 ]
check "the refusal says why" bash -c "[[ \"\$1\" == *'interactive terminal'* ]]" _ "$OUT"
OUT="$(cd "$PROJ" && python3 "$APPROVALS" request bad --path docs/x.md --proposed /dev/null 2>&1)"; RC=$?
check "a request outside product/ is refused" [ "$RC" == 1 ]
OUT="$(cd "$PROJ" && python3 "$APPROVALS" request bad2 --path product/backlog.md --proposed product/decisions.md 2>&1)"; RC=$?
check "a proposal drafted inside product/ is refused" [ "$RC" == 1 ]
OUT="$(cd "$PROJ" && python3 "$APPROVALS" audit 2>&1)"; RC=$?
check "audit reports the forged row" [ "$RC" == 1 ]
check "audit names it" bash -c "[[ \"\$1\" == *'FORGED f1'* || \"\$1\" == *'CHAIN-BREAK f1'* ]]" _ "$OUT"

echo "== approve --apply writes the file and spends the approval =="
printf '# Backlog\n' > "$PROJ/product/backlog.md"
request "$PROJ" ap1 product/backlog.md $'# Backlog\n\napplied\n'
maintainer "$PROJ" approve ap1 --apply --yes
check "the maintainer's apply wrote the file" grep -q applied "$PROJ/product/backlog.md"
expect_rc 2 "the applied approval is spent" "$HOOK" "$(write_payload Write "$PROJ/product/backlog.md" $'# Backlog\n\napplied\n')"

echo "== linked worktrees share the main checkout's queue =="
git -C "$PROJ" add -A product >/dev/null 2>&1; git -C "$PROJ" commit -q -m sync
WT="$TEST_TMP/wt"
git -C "$PROJ" worktree add -q "$WT" -b wt-branch
request "$WT" w1 product/backlog.md "from the worktree"
check "the request lands in the main checkout" [ -f "$PROJ/orchestration/requests/w1/request.json" ]
maintainer "$PROJ" approve w1 --yes
expect_rc 0 "an approved worktree write" "$HOOK" "$(write_payload Write "$WT/product/backlog.md" "from the worktree" "$WT")"
request "$PROJ" x1 product/backlog.md "cross"
maintainer "$PROJ" approve x1 --yes
expect_rc 2 "an approval for the main checkout does not release a worktree write" "$HOOK" "$(write_payload Write "$WT/product/backlog.md" "cross" "$WT")"
expect_err "signed for the worktree" "the worktree mismatch is named"

echo "== what is never released =="
nb="$(jq -nc --arg p "$PROJ/product/x.ipynb" '{tool_name:"NotebookEdit",cwd:"/",tool_input:{notebook_path:$p,new_source:"x"}}')"
expect_rc 2 "NotebookEdit under product/" "$HOOK" "$nb"
expect_rc 2 "Write to the ledger" "$HOOK" "$(write_payload Write "$LEDGER" '{}')"
expect_rc 2 "Write to the key" "$HOOK" "$(write_payload Write "$MYTHHELM_APPROVALS_KEY" 'k')"
ln -s ../product/backlog.md "$PROJ/docs/pb.md"
expect_rc 2 "a symlink into product/" "$HOOK" "$(write_payload Write "$PROJ/docs/pb.md" "via link")"

echo "== writes elsewhere pass =="
expect_rc 0 "docs file" "$HOOK" "$(write_payload Write "$PROJ/docs/guide.md" "x")"
expect_rc 0 "a look-alike directory" "$HOOK" "$(write_payload Write "$PROJ/productive/notes.md" "x")"
expect_rc 0 "a look-alike file name" "$HOOK" "$(write_payload Write "$PROJ/knowledge/product-management.md" "x")"
expect_rc 0 "a product directory that is not at a repository root" "$HOOK" "$(write_payload Write "$PROJ/docs/product/x.md" "x")"
expect_rc 0 "a request file" "$HOOK" "$(write_payload Write "$PROJ/orchestration/requests/w1/notes" "x")"
expect_rc 0 "Read is not guarded" "$HOOK" "$(jq -nc --arg p "$PROJ/product/backlog.md" '{tool_name:"Read",tool_input:{file_path:$p}}')"

echo "== read-only commands that mention product/ pass =="
allow "wc" 'wc -l product/backlog.md'
allow "grep" 'grep -n "MH-" product/backlog.md'
allow "cat" 'cat product/backlog.md product/decisions.md'
allow "ls" 'ls -la product/'
allow "ls the directory" 'ls product'
allow "head" 'head -20 product/decisions.md'
allow "rg" 'rg -n "Status" product'
allow "a pipeline of readers" 'cat product/backlog.md | grep MH- | wc -l'
allow "sed -n" "sed -n '1,5p' product/backlog.md"
allow "awk print" "awk '/^### MH/ {print \$2}' product/backlog.md"
allow "find -name" "find product -name '*.md'"
allow "sort" 'sort product/backlog.md'
allow "diff" 'diff product/backlog.md /tmp/draft.md'
allow "git diff" 'git diff -- product/'
allow "git log" 'git log --oneline -- product/backlog.md'
allow "git add" 'git add product/backlog.md'
allow "git commit naming the file" 'git commit -s -m "product: seed the backlog" -- product/backlog.md'
allow "pm.py validate" 'python3 scripts/pm/pm.py validate'
allow "pm.py draft" 'python3 scripts/pm/pm.py set-status MH-1 shipped --out /tmp/backlog.md'
allow "approvals request" 'python3 scripts/orchestration/approvals.py request x --path product/backlog.md --proposed /tmp/x'
allow "approvals list" 'python3 scripts/orchestration/approvals.py list'
allow "reading the ledger" 'jq -c . orchestration/approvals.jsonl'
allow "prose in a redirect elsewhere" 'echo "see product/backlog.md" > /tmp/note.txt'
allow "prose in a gh body" 'gh issue create --title "Card" --body "Tracked in product/backlog.md"'
allow "a heredoc body is data" $'cat > /tmp/body.md <<EOF\nsed -i s/a/b/ product/backlog.md\nEOF'
allow "cd into product/ to read" 'cd product && cat backlog.md && wc -l decisions.md'
allow "a look-alike directory" 'rm -rf productive/tmp'
allow "no mention at all" 'go test ./...'

echo "== writes through Bash block =="
block "sed -i" 'sed -i s/a/b/ product/backlog.md'
block "redirect" 'echo x > product/backlog.md'
block "append redirect" 'echo x >> product/decisions.md'
block "attached redirect" 'printf x >product/x.md'
block "cp into" 'cp /tmp/x product/backlog.md'
block "mv" 'mv product/backlog.md /tmp/'
block "rm" 'rm product/backlog.md'
block "rm the directory" 'rm -rf product'
block "tee" 'echo x | tee -a product/backlog.md'
block "dd of=" 'dd if=/tmp/x of=product/backlog.md'
block "ln over a file" 'ln -sf /tmp/x product/backlog.md'
block "python" "python3 -c \"open('product/backlog.md','w').write('x')\""
block "bash -c" "bash -c 'sed -i s/a/b/ product/backlog.md'"
block "git checkout" 'git checkout -- product/backlog.md'
block "git restore" 'git restore product/'
block "git show --output" 'git show --output=product/x.md HEAD:product/backlog.md'
block "sort -o" 'sort -o product/backlog.md product/backlog.md'
block "sed w command" "sed -n '1w product/x.md' product/backlog.md"
block "sed s///w" "sed -n 's/a/b/w /tmp/y' product/backlog.md"
block "awk redirect" "awk '{print > \"/tmp/y\"}' product/backlog.md"
block "awk -i inplace" "awk -i inplace '{print}' product/backlog.md"
block "find -delete" 'find product -name "*.md" -delete'
block "uniq with an output file" 'uniq product/backlog.md product/decisions.md'
block "cd then write" 'cd product && sed -i s/a/b/ backlog.md'
block "cd then redirect" 'cd product; echo x > backlog.md'
block "cd - leaves the directory unknown" 'cd product; cd /tmp; cd -; rm backlog.md'
block "a payload cwd inside product/" 'sed -i s/a/b/ backlog.md' "$PROJ/product"
block "an editor" 'vim product/backlog.md'

echo "== the approval machinery is the maintainer's =="
block "approve.sh approve" 'scripts/orchestration/approve.sh approve a1 --yes'
block "approve.sh list" 'bash scripts/orchestration/approve.sh list'
block "approve.sh show" './scripts/orchestration/approve.sh show a1'
block "approvals.py approve" 'python3 scripts/orchestration/approvals.py approve a1 --yes'
block "approvals.py reject" 'python3 scripts/orchestration/approvals.py reject a1'
block "approvals.py init-key" 'python3 scripts/orchestration/approvals.py init-key'
block "approvals.py check-write" 'echo "{}" | python3 scripts/orchestration/approvals.py check-write'
block "appending to the ledger" "echo '{\"state\":\"approved\"}' >> orchestration/approvals.jsonl"
block "editing the ledger" 'sed -i d orchestration/approvals.jsonl'
block "reading the key" 'cat ~/.config/mythhelm/approvals.key'
block "reading the key by variable" 'cat "$HOME/.config/mythhelm/approvals.key"'
run_hook "$HOOK" "$(bash_payload 'scripts/orchestration/approve.sh approve a1' "$PROJ" "$SID")"
expect_stanza guard-product-write "the Bash block teaches"

echo "== fails closed without its tools =="
PAYLOAD="$(bash_payload 'sed -i s/a/b/ product/backlog.md' "$PROJ" "$SID")"
OUT="$(printf '%s' "$PAYLOAD" | HOOK_JQ_PROBE=no-such-jq "$HOOK" 2>&1)"; RC=$?
check "no jq, a payload naming product/ blocks" [ "$RC" == 2 ]
OUT="$(printf '%s' "$(bash_payload 'go test ./...' "$PROJ" "$SID")" | HOOK_JQ_PROBE=no-such-jq "$HOOK" 2>&1)"; RC=$?
check "no jq, an unrelated payload passes" [ "$RC" == 0 ]
OUT="$(printf '%s' "$(write_payload Write "$PROJ/product/backlog.md" x)" | HOOK_PYTHON_PROBE=no-such-python "$HOOK" 2>&1)"; RC=$?
check "no python3, a Write naming product/ blocks" [ "$RC" == 2 ]

finish
