#!/usr/bin/env bash
# shellcheck disable=SC2016  # commands under test are literal data
# test_guard_vendors.sh: guard-vendors.sh blocks direct Codex and Muse CLI calls,
# package-runner spellings, `vendors.py enable` and writes to the local vendor
# policy; passes the sanctioned wrappers, lookups, readers and mere mentions; fails
# closed without jq; and no hook ever calls the classifier client.

# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
HOOK="$HOOKS_DIR/guard-vendors.sh"
PROJ="$TEST_TMP/proj"
new_repo "$PROJ"
export CLAUDE_PROJECT_DIR="$PROJ"
SID="v-$$"

block() { expect_rc 2 "$1" "$HOOK" "$(bash_payload "$2" "$PROJ" "$SID")"; }
allow() { expect_rc 0 "$1" "$HOOK" "$(bash_payload "$2" "$PROJ" "$SID")"; }
edit_payload() {
  jq -nc --arg t "$1" --arg p "$2" '{hook_event_name:"PreToolUse",tool_name:$t,tool_input:{file_path:$p,content:"{}"}}'
}

echo "== direct CLI calls block =="
block "codex exec" 'codex exec "review this"'
block "codex with the read-only sandbox" 'codex --sandbox read-only exec -'
block "codex --version has no carve-out" 'codex --version'
block "codex login status" 'codex login status'
block "muse exec" 'muse exec --json --prompt-file p.md'
block "muse --help" 'muse --help'
block "absolute path" '/opt/tools/bin/codex exec x'
block "versioned binary" 'muse-bin-1.4.0 exec x'
block "platform binary" 'codex-x86_64-unknown-linux-musl exec x'
block "bash -c" "bash -c 'codex exec x'"
block "eval" 'eval codex exec x'
block "env wrapper" 'env OPENAI_API_KEY=x codex exec x'
block "timeout wrapper" 'timeout 60 muse exec x'
block "chained" 'git status && codex exec x'
block "piped" 'cat p.md | codex exec -'
block "substitution" 'echo "$(codex exec x)"'
block "quote tricks" 'c"od"ex exec x'
block "npx" 'npx @openai/codex exec x'
block "npx with version" 'npx -y @openai/codex@latest exec x'
block "npx bare name" 'npx codex exec x'
block "npm exec" 'npm exec @openai/codex -- exec x'
block "pnpm dlx" 'pnpm dlx @openai/codex exec x'
block "bunx" 'bunx @openai/codex'
block "exec builtin" 'exec codex exec x'
run_hook "$HOOK" "$(bash_payload 'codex exec x' "$PROJ" "$SID")"
expect_stanza guard-vendors "the block teaches"
expect_err "scripts/codex/codex-consult.sh" "fix names a wrapper"

echo "== opting in is the contributor's =="
block "vendors.py enable" 'python3 scripts/vendors/vendors.py enable codex'
block "enable with lanes" 'python3 scripts/vendors/vendors.py enable codex --lanes'
block "direct execution" 'scripts/vendors/vendors.py enable muse'
block "python with -u" 'python3 -u scripts/vendors/vendors.py enable jev'
block "redirect into the opt-in file" "echo '{\"enabled\":[\"codex\"]}' > .claude/data/vendor-policy.local.json"
block "append redirect" 'printf x >> .claude/data/vendor-policy.local.json'
block "a reader with a redirect" 'cat other.json > .claude/data/vendor-policy.local.json'
block "tee" 'echo x | tee .claude/data/vendor-policy.local.json'
block "cp" 'cp /tmp/p.json .claude/data/vendor-policy.local.json'
block "sed in place" "sed -i s/a/b/ .claude/data/vendor-policy.local.json"
expect_rc 2 "Write of the opt-in file" "$HOOK" "$(edit_payload Write "$PROJ/.claude/data/vendor-policy.local.json")"
expect_err "File: $PROJ/.claude/data/vendor-policy.local.json" "file stanza names the file"
expect_rc 2 "Edit of the opt-in file" "$HOOK" "$(edit_payload Edit "$PROJ/.claude/data/vendor-policy.local.json")"

echo "== sanctioned and harmless forms pass =="
allow "codex consult wrapper" 'bash scripts/codex/codex-consult.sh --stage task-review --target /tmp/wt --context c.md'
allow "codex review wrapper" 'scripts/codex/codex-review.sh --target /tmp/wt --base origin/main'
allow "muse consult wrapper" 'bash scripts/vendors/muse-consult.sh --stage dossier --target /tmp/wt --context c.md'
allow "implement wrappers" 'bash scripts/vendors/codex-implement.sh --spec-dir specs/todo/x --task 1 && bash scripts/vendors/muse-implement.sh --spec-dir specs/todo/x --task 2'
allow "envelope directly" 'python3 scripts/vendors/consult.py --vendor codex --stage task-review --target /tmp/wt --context c.md'
allow "status" 'python3 scripts/vendors/vendors.py status'
allow "disable" 'python3 scripts/vendors/vendors.py disable all'
allow "command -v" 'command -v codex'
allow "which" 'which muse'
allow "type" 'type codex'
allow "grep for the word" 'grep -rn "codex exec" scripts/'
allow "commit message" 'git commit -m "document the codex and muse wrappers" -- knowledge/vendors.md'
allow "echo" 'echo "run codex exec through the wrapper"'
allow "heredoc body" $'cat > notes.md <<EOF\ncodex exec x\nEOF'
allow "path argument" 'ls scripts/codex/ && cat scripts/vendors/muse-consult.sh'
allow "reading the opt-in file" 'cat .claude/data/vendor-policy.local.json && jq .enabled .claude/data/vendor-policy.local.json'
allow "npx of another package" 'npx prettier --check .'
allow "unrelated" 'go test ./...'
expect_rc 0 "Write of the tracked policy" "$HOOK" "$(edit_payload Write "$PROJ/.claude/data/vendor-policy.json")"
expect_rc 0 "Write elsewhere" "$HOOK" "$(edit_payload Write "$PROJ/README.md")"

echo "== fail closed without jq =="
CHECKS=$((CHECKS + 1))
RC=$(bash_payload 'codex exec x' "$PROJ" "$SID" | HOOK_JQ_PROBE=jq-not-installed "$HOOK" 2>/dev/null; echo $?)
[[ "$RC" == 2 ]] || fail "[jq missing blocks codex] got $RC"
CHECKS=$((CHECKS + 1))
RC=$(edit_payload Write "$PROJ/.claude/data/vendor-policy.local.json" | HOOK_JQ_PROBE=jq-not-installed "$HOOK" 2>/dev/null; echo $?)
[[ "$RC" == 2 ]] || fail "[jq missing blocks a Write of the opt-in file] got $RC"
CHECKS=$((CHECKS + 1))
RC=$(bash_payload 'go test ./...' "$PROJ" "$SID" | HOOK_JQ_PROBE=jq-not-installed "$HOOK" 2>/dev/null; echo $?)
[[ "$RC" == 0 ]] || fail "[jq missing allows go test] got $RC"
expect_rc 2 "malformed payload naming codex" "$HOOK" '{"tool_name":"Bash","tool_input":{"command":"codex exec x"'

echo "== no hook calls the classifier =="
# A blocking hook must never wait on the network or allow on a model's say-so.
hits="$(grep -l 'scripts/jev\|jev\.py\|jev_client' "$HOOKS_DIR"/*.sh || true)"
check "no hook script references the classifier client" [ -z "$hits" ]
bad="$TEST_TMP/bad-hook.sh"
printf '#!/usr/bin/env bash\npython3 scripts/jev/jev.py private-material x\n' > "$bad"
check "the check bites on a hook that does" grep -q 'scripts/jev' "$bad"

finish
