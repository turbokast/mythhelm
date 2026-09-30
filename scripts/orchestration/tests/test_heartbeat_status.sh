#!/usr/bin/env bash
# test_heartbeat_status.sh: heartbeat.sh resumes the granted session only with a live
# grant, actionable work and no live session on the run lane, never passes
# --dangerously-skip-permissions and marks the session unattended; status.sh prints
# every section, counts unresolved threads, and degrades when GitHub is unavailable.

# shellcheck source=../../../.claude/hooks/tests/lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/../../../.claude/hooks/tests/lib.sh"

# The scripts resolve the repository from their own location, so each run uses a
# copy of scripts/orchestration inside the fixture repository.
R="$TEST_TMP/repo"
new_repo "$R"
mkdir -p "$R/scripts" "$TEST_TMP/bin"
cp -R "$REPO_ROOT/scripts/orchestration" "$R/scripts/"
HB="$R/scripts/orchestration/heartbeat.sh"
ST="$R/scripts/orchestration/status.sh"
DATA="$R/.claude/data"
mkdir -p "$DATA"

cat > "$TEST_TMP/bin/claude" <<'EOF'
#!/usr/bin/env bash
{ printf 'argv:'; printf ' [%s]' "$@"; echo; echo "noninteractive=${MYTHHELM_NONINTERACTIVE:-}"; } > "$CLAUDE_RECORD"
EOF
cat > "$TEST_TMP/bin/gh" <<'EOF'
#!/usr/bin/env bash
[[ -n "${GH_OFFLINE:-}" ]] && exit 1
case "$1 $2" in
  "repo view") echo "acme/demo" ;;
  "api graphql") echo '{"data":{"repository":{"pullRequests":{"nodes":[{"number":12,"title":"feat: demo task 1","headRefName":"feat/demo-t1","isDraft":false,"reviewThreads":{"nodes":[{"isResolved":false},{"isResolved":true},{"isResolved":false}]}}]}}}}' ;;
  "run list") echo '[{"workflowName":"CI","status":"completed","conclusion":"success","headSha":"0123456789abcdef","createdAt":"2026-01-01T00:00:00Z"}]' ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$TEST_TMP/bin/claude" "$TEST_TMP/bin/gh"
export PATH="$TEST_TMP/bin:$PATH" CLAUDE_RECORD="$TEST_TMP/claude.rec"
unset MYTHHELM_NONINTERACTIVE

grant() {  # grant <until offset>
  local now
  now="$(date +%s)"
  jq -n --argjson i $((now - 60)) --argjson u $((now + $1)) \
    '{schema_version:1,id:"g1",session_id:"sess-42",issued_epoch:$i,until_epoch:$u,until:"2099-01-01T00:00:00Z",
      scope:{cards:[],specs:["demo"]},max_continues:5,renewals:0}' > "$DATA/autonomy-grant.json"
}
tick() { rm -f "$CLAUDE_RECORD"; OUT="$("$HB" "$@" 2>&1)"; RC=$?; }
has() { CHECKS=$((CHECKS + 1)); [[ "$OUT" == *"$1"* ]] || fail "[$2] output lacks '$1': $OUT"; }
dv() { (cd "$R" && python3 scripts/orchestration/delivery.py "$@" >/dev/null 2>&1); }

echo "== heartbeat refusals =="
tick
has "refused: no-grant" "no grant"
check "nothing launched" test ! -e "$CLAUDE_RECORD"
grant 3600
tick
has "refused: nothing-actionable" "no run"
dv init --scope spec:demo
dv item spec:demo --stage run-spec
sleep 600 & HOLDER=$!
trap 'kill $HOLDER 2>/dev/null; cleanup_test_tmp' EXIT
(cd "$R" && scripts/orchestration/lane.sh acquire run:delivery --owner lane-orchestrator1 --pid "$HOLDER" >/dev/null)
tick
has "refused: session-live" "a live session holds the run lane"
kill "$HOLDER"; wait "$HOLDER" 2>/dev/null
grant -30
tick
has "refused: grant-expired" "expired grant"
grant 3600

echo "== heartbeat launch =="
tick --dry-run
has "claude -p --resume sess-42" "dry run prints the resume"
check "dry run launched nothing" test ! -e "$CLAUDE_RECORD"
tick
check "launched" test -e "$CLAUDE_RECORD"
check "resumes the granted session in print mode" grep -q '^argv: \[-p\] \[--resume\] \[sess-42\]' "$CLAUDE_RECORD"
check "never skips permissions" bash -c "! grep -q 'dangerously' '$CLAUDE_RECORD'"
check "marks the session unattended" grep -q '^noninteractive=1$' "$CLAUDE_RECORD"
check "decisions are audited" test "$(jq -s '[.[] | select(.event == "heartbeat")] | length' "$DATA/autonomy-audit.jsonl")" -ge 6
tick --print-install
has "systemctl --user enable --now" "install commands"
tick --bogus
check "bad usage exits 2" test "$RC" = 2

echo "== status =="
OUT="$(cd "$R" && "$ST" --no-fetch 2>&1)"
RC=$?
check "status exits 0" test "$RC" = 0
for s in AUTONOMY "DELIVERY RUN" LANES "SPECS IN FLIGHT" "OPEN PULL REQUESTS" "MAIN CI"; do
  has "== $s" "section $s"
done
has "autonomy: live, session sess-42" "grant state"
has "spec:demo" "run items"
has "#12 feat: demo task 1 [feat/demo-t1] unresolved=2" "unresolved thread count"
has "CI: success" "main CI"
OUT="$(cd "$R" && GH_OFFLINE=1 "$ST" --no-fetch 2>&1)"
RC=$?
check "offline status exits 0" test "$RC" = 0
has "(GitHub unavailable)" "degrades offline"

finish
