#!/usr/bin/env bash
# shellcheck disable=SC2016  # fixture text is literal data
# test_check_public_hygiene.sh: check-public-hygiene.sh flags each leak category,
# passes the allowed forms, redacts secrets, and skips LICENSE, binary files and
# ignored files. Leak fixtures are assembled at run time, so this file itself
# stays clean for the real check.

# shellcheck source=../../../.claude/hooks/tests/lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/../../../.claude/hooks/tests/lib.sh"
CHECK="$REPO_ROOT/scripts/ci/check-public-hygiene.sh"

R="$TEST_TMP/repo"
new_repo "$R"
run_check() { OUT="$("$CHECK" "$R" 2>&1)"; RC=$?; }

# expect_flag <category> <description> <line>: that line alone is flagged.
expect_flag() {
  printf '%s\n' "$3" > "$R/probe.txt"
  run_check
  CHECKS=$((CHECKS + 1))
  [[ "$RC" == 1 && "$OUT" == *"probe.txt:1: $1:"* ]] || fail "[$2] expected a $1 finding (rc $RC): $OUT"
}
# expect_clean <description> <line>
expect_clean() {
  printf '%s\n' "$2" > "$R/probe.txt"
  run_check
  CHECKS=$((CHECKS + 1))
  [[ "$RC" == 0 ]] || fail "[$1] expected clean (rc $RC): $OUT"
}

A36="$(printf 'a%.0s' {1..36})"
H="ho""me"
U="Us""ers"

echo "== flagged =="
expect_flag home-path "linux home path" "see /$H/alice/project/x"
expect_flag home-path "macOS home path" "cd /$U/alice/src"
expect_flag email "personal email" "contact alice@corp""mail.io"
expect_flag email "project domain, unlisted" "ops@turbo""kast.com"
expect_flag private-key "RSA key" "-----BEGIN RSA PRIV""ATE KEY-----"
expect_flag private-key "OpenSSH key" "-----BEGIN OPENSSH PRIV""ATE KEY-----"
expect_flag token "GitHub classic token" "token: gh""p_$A36"
expect_flag token "GitHub OAuth token" "gh""o_$A36"
expect_flag token "fine-grained PAT" "github_""pat_11ABCDEFG0123456789_abcdefghijklmnop"
expect_flag token "GitLab token" "gl""pat-abcdefghijklmnopqrst"
expect_flag token "Anthropic key" "sk-""ant-api03-abcdefghijklmnopqrstuvwxyz"
expect_flag token "sk- key" "OPENAI=sk""-abcdefghijklmnopqrstuvwx"
expect_flag token "AWS key id" "AK""IAABCDEFGHIJKLMNOP"
expect_flag token "Slack bot token" "xo""xb-1234567890-abcdefghij"
expect_flag host "fly.dev host" "https://myapp.fly"".dev/health"
expect_flag host "internal host" "db.prod"".internal:5432"
expect_flag ipv4 "public IPv4" "server at 8.8.""8.8"
expect_flag ipv4 "CGNAT is not exempt" "peer 100.64.""1.2"

echo "== allowed =="
expect_clean "published contacts" "security@turbokast.com liam@turbokast.com noreply@anthropic.com"
expect_clean "example domains" "you@example.com a@b.example.org"
expect_clean "git ssh remote" "git@github.com:turbokast/mythhelm.git"
expect_clean "placeholder home path" "never commit /home/<user> paths"
expect_clean "HOME-relative path" 'see $HOME/.config/x'
expect_clean "documentation IPs" "192.0.2.10 198.51.100.7 203.0.113.99"
expect_clean "loopback and private" "127.0.0.1 10.1.2.3 172.16.0.1 192.168.1.1 0.0.0.0 169.254.169.254"
expect_clean "version strings" "go 1.23.4 and v1.2.3.4 and 999.1.1.1"
expect_clean "Go internal package path" "github.com/turbokast/mythhelm/internal/tui"
expect_clean "words containing sk-" "task-abcdefghijklmnopqrstuvwxyz disk-abcdefghijklmnopqrstuvwxyz"
expect_clean "bare token prefixes" "tokens start with gh""p_ or gl""pat-"
expect_clean "action pin" "uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1"

echo "== redaction and exclusions =="
printf '%s\n' "gh""p_$A36" > "$R/probe.txt"
run_check
check "token is redacted" [ "${OUT/$A36/}" == "$OUT" ]
rm -f "$R/probe.txt"
printf 'Copyright alice@corp''mail.io\n' > "$R/LICENSE"
run_check
check "LICENSE is skipped" [ "$RC" == 0 ]
printf 'x\0alice@corp''mail.io\n' > "$R/blob.bin"
run_check
check "binary files are not scanned for emails" [ "$RC" == 0 ]
printf 'x\0token=%s\n' "gh""p_$A36" > "$R/blob.bin"
run_check
check "binary files are still scanned for tokens" grep -q '^blob.bin:1: token:' <<< "$OUT"
printf 'x\0%s\n' "-----BEGIN EC PRIV""ATE KEY-----" > "$R/blob.bin"
run_check
check "binary files are still scanned for private keys" grep -q '^blob.bin:1: private-key:' <<< "$OUT"
rm -f "$R/blob.bin"
printf 'secret.txt\n' > "$R/.gitignore"
printf 'alice@corp''mail.io\n' > "$R/secret.txt"
run_check
check "ignored files are skipped" [ "$RC" == 0 ]
printf 'alice@corp''mail.io\n' > "$R/new-untracked.md"
run_check
check "untracked files are scanned" [ "$RC" == 1 ]
check "finding names the file" grep -q '^new-untracked.md:1: email:' <<< "$OUT"
rm -f "$R/new-untracked.md"
mkdir -p "$R/sub"
printf 'alice@corp''mail.io\n' > "$R/sub/tracked.md"
git -C "$R" add sub/tracked.md
run_check
check "tracked files in subdirectories are scanned" grep -q '^sub/tracked.md:1: email:' <<< "$OUT"

finish
