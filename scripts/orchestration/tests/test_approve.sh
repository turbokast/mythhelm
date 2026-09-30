#!/usr/bin/env bash
# test_approve.sh: approve.sh and approvals.py outside the hook. The wrapper takes
# only the maintainer's subcommands; requests are validated before anything is
# written; the maintainer verbs refuse without a terminal; init-key creates a 0600
# key once; and audit verifies signatures and detects a removed or altered decision.
# The hook's use of approvals.py is covered by test_guard_product_write.sh.

# shellcheck source=../../../.claude/hooks/tests/lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/../../../.claude/hooks/tests/lib.sh"
APPROVE="$REPO_ROOT/scripts/orchestration/approve.sh"
APPROVALS="$REPO_ROOT/scripts/orchestration/approvals.py"
R="$TEST_TMP/repo"
new_repo "$R"
mkdir -p "$R/product"
printf 'v1\n' > "$R/product/backlog.md"
export MYTHHELM_APPROVALS_KEY="$TEST_TMP/cfg/approvals.key"
LEDGER="$R/orchestration/approvals.jsonl"

# run <cmd...>: runs in $R without a terminal; sets RC and OUT (stdout and stderr).
run() { OUT="$(cd "$R" && "$@" 2>&1 </dev/null)"; RC=$?; }
expect() {   # expect <rc> <description>
  CHECKS=$((CHECKS + 1))
  [[ "$RC" == "$1" ]] || fail "[$2] expected exit $1, got $RC: ${OUT:0:300}"
}
has() {      # has <needle> <description>
  CHECKS=$((CHECKS + 1))
  [[ "$OUT" == *"$1"* ]] || fail "[$2] output lacks '$1': ${OUT:0:300}"
}
# tty <cmd>: runs a shell command in $R under a pseudo-terminal, as a person would.
tty() {
  if script --version >/dev/null 2>&1; then
    script -qec "cd $(printf '%q' "$R") && $1" /dev/null </dev/null >/dev/null 2>&1
  else
    script -q /dev/null bash -c "cd $(printf '%q' "$R") && $1" </dev/null >/dev/null 2>&1
  fi
}

echo "== the wrapper =="
run "$APPROVE"
expect 2 "no subcommand is a usage error"
run "$APPROVE" request x --path product/backlog.md --proposed /dev/null
expect 2 "request is not a maintainer subcommand"
run "$APPROVE" list
expect 0 "list with an empty queue"
has "no requests" "an empty queue says so"

echo "== requests are validated =="
printf 'v2\n' > "$TEST_TMP/v2"
run python3 "$APPROVALS" request Bad_Id --path product/backlog.md --proposed "$TEST_TMP/v2"
expect 1 "an id with capitals and underscores"
run python3 "$APPROVALS" request a --path docs/x.md --proposed "$TEST_TMP/v2"
expect 1 "a path outside product/"
run python3 "$APPROVALS" request a --path product/ --proposed "$TEST_TMP/v2"
expect 1 "the whole directory"
run python3 "$APPROVALS" request a --path ../product/backlog.md --proposed "$TEST_TMP/v2"
expect 1 "a traversing path"
run python3 "$APPROVALS" request a --path product/backlog.md --proposed "$R/product/backlog.md"
expect 1 "a proposal inside product/"
printf 'v1\n' > "$TEST_TMP/same"
run python3 "$APPROVALS" request a --path product/backlog.md --proposed "$TEST_TMP/same"
expect 1 "a proposal identical to the file"
printf 'v1\n\377\n' > "$TEST_TMP/binary"
run python3 "$APPROVALS" request a --path product/backlog.md --proposed "$TEST_TMP/binary"
expect 1 "a proposal that is not UTF-8, whose diff could not be shown faithfully"
has "not valid UTF-8" "the refusal says why"
run python3 "$APPROVALS" request a1 --path ./product//backlog.md --proposed "$TEST_TMP/v2" --summary "bump"
expect 0 "a valid request"
check "the path is stored canonically" grep -q '"path": "product/backlog.md"' "$R/orchestration/requests/a1/request.json"
check "the diff is stored" grep -q '^+v2' "$R/orchestration/requests/a1/diff"
run python3 "$APPROVALS" request a1 --path product/backlog.md --proposed "$TEST_TMP/v2"
expect 1 "a duplicate id"
run "$APPROVE" show a1
expect 0 "show"
has "+v2" "show prints the diff"
cp "$R/orchestration/requests/a1/diff" "$TEST_TMP/diff.bak"
printf -- '--- a/x\n+++ b/x\n+harmless\n' > "$R/orchestration/requests/a1/diff"
run "$APPROVE" show a1
CHECKS=$((CHECKS + 1))
[[ "$OUT" == *"+v2"* && "$OUT" != *harmless* ]] || fail "[show recomputes the diff instead of trusting the stored copy] ${OUT:0:300}"
cp "$TEST_TMP/diff.bak" "$R/orchestration/requests/a1/diff"

echo "== the maintainer verbs need a terminal and a key =="
run "$APPROVE" init-key
expect 1 "init-key without a terminal"
check "no key was created" [ ! -e "$MYTHHELM_APPROVALS_KEY" ]
tty "$(printf '%q' "$APPROVE") init-key"
check "init-key under a terminal creates the key" [ -s "$MYTHHELM_APPROVALS_KEY" ]
check "the key is private" [ "$(stat -c %a "$MYTHHELM_APPROVALS_KEY" 2>/dev/null || stat -f %Lp "$MYTHHELM_APPROVALS_KEY")" == 600 ]
KEY_BEFORE="$(cat "$MYTHHELM_APPROVALS_KEY")"
tty "$(printf '%q' "$APPROVE") init-key"
check "a second init-key keeps the key" [ "$(cat "$MYTHHELM_APPROVALS_KEY")" == "$KEY_BEFORE" ]
run "$APPROVE" approve a1 --yes
expect 1 "approve without a terminal"
has "interactive terminal" "the refusal says why"
check "nothing was recorded" [ ! -s "$LEDGER" ]

echo "== audit =="
tty "$(printf '%q' "$APPROVE") approve a1 --yes"
printf 'v3\n' > "$TEST_TMP/v3"
run python3 "$APPROVALS" request a2 --path product/backlog.md --proposed "$TEST_TMP/v3"
tty "$(printf '%q' "$APPROVE") reject a2 --yes"
run "$APPROVE" audit
expect 0 "a clean chain audits clean"
has "audited 2 decision(s): 0 problem(s)" "both decisions verify"
run "$APPROVE" list
has "approved" "list shows the decision"
cp "$LEDGER" "$TEST_TMP/ledger.bak"
sed '1d' "$TEST_TMP/ledger.bak" > "$LEDGER"
run "$APPROVE" audit
expect 1 "a removed decision breaks the chain"
has "CHAIN-BREAK a2" "the break is named"
sed 's/"state": "rejected"/"state": "approved"/' "$TEST_TMP/ledger.bak" > "$LEDGER"
run "$APPROVE" audit
expect 1 "an altered decision"
has "FORGED a2" "the forgery is named"
cp "$TEST_TMP/ledger.bak" "$LEDGER"
tty "$(printf '%q' "$APPROVE") approve a1 --yes"
run "$APPROVE" audit
has "audited 2 decision(s)" "a decided request cannot be decided again"

echo "== approve checks the worktree a request names =="
OTHER="$TEST_TMP/other"
new_repo "$OTHER"
mkdir -p "$OTHER/product"
printf 'v1\n' > "$OTHER/product/backlog.md"
printf 'w\n' > "$TEST_TMP/w"
run python3 "$APPROVALS" request wt1 --path product/backlog.md --proposed "$TEST_TMP/w"
jq --arg o "$OTHER" '.worktree = $o' "$R/orchestration/requests/wt1/request.json" > "$TEST_TMP/req.json"
cp "$TEST_TMP/req.json" "$R/orchestration/requests/wt1/request.json"
tty "$(printf '%q' "$APPROVE") approve wt1 --yes"
check "a request retargeted to another repository is not approved" bash -c "! grep -q '\"id\": \"wt1\"' '$LEDGER'"
run "$APPROVE" audit
has "audited 2 decision(s)" "no decision was recorded for it"
run "$APPROVE" show ../../x
expect 1 "show refuses a path-like id"

finish
