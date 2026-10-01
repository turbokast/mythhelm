package supervisor_test

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/adapters/claudecode"
	"github.com/turbokast/mythhelm/internal/admission"
)

// fakeClaudeConfig is the fakeclaude.json sidecar beside the `claude` copy:
// the version and auth answers plus the stream lines the launch replays.
type fakeClaudeConfig struct {
	Version         string         `json:"version"`
	Auth            map[string]any `json:"auth"`
	Stream          []string       `json:"stream"`
	Exit            int            `json:"exit"`
	SleepAfterLines int            `json:"sleep_after_lines"`
	SleepMs         int            `json:"sleep_ms"`
	IgnoreTerm      bool           `json:"ignore_term"`
	ArgvFile        string         `json:"argv_file"`
	DelayMs         int            `json:"delay_ms"`
}

func fakeClaudeMain() int {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fakeclaude: %v\n", err)
		return 9
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "fakeclaude.json")) // #nosec G304 -- test sidecar beside the copied binary
	if err != nil {
		fmt.Fprintf(os.Stderr, "fakeclaude: %v\n", err)
		return 9
	}
	var cfg fakeClaudeConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "fakeclaude: %v\n", err)
		return 9
	}
	switch strings.Join(os.Args[1:], " ") {
	case "--version":
		fmt.Printf("%s (Claude Code)\n", cfg.Version)
		return 0
	case "auth status":
		out, _ := json.Marshal(cfg.Auth)
		fmt.Print(string(out))
		return 0
	}
	// Ignoring SIGTERM forces the worker's ladder through its full TERM
	// grace, so a test signal to the CLI always lands before conclude.
	if cfg.IgnoreTerm {
		signal.Ignore(syscall.SIGTERM)
	}
	// Only the launch invocation reports its argv: probes answer above.
	if cfg.ArgvFile != "" {
		_ = os.WriteFile(cfg.ArgvFile, []byte(strings.Join(os.Args, "\n")), 0o600) // #nosec G703 -- test-declared path under t.TempDir
	}
	// A start delay holds init back so a test signal lands first.
	if cfg.DelayMs > 0 {
		time.Sleep(time.Duration(cfg.DelayMs) * time.Millisecond)
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	for i, line := range cfg.Stream {
		fmt.Println(line)
		if cfg.SleepMs > 0 && i+1 == cfg.SleepAfterLines {
			time.Sleep(time.Duration(cfg.SleepMs) * time.Millisecond)
		}
	}
	return cfg.Exit
}

// installFakeClaude copies the test binary to dir as `claude` with its
// sidecar, and prepends dir to PATH. HOME stays the fixture's.
func installFakeClaude(t *testing.T, cfg fakeClaudeConfig) {
	t.Helper()
	dir := t.TempDir()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(self) // #nosec G304 -- Test copies its own binary from os.Executable.
	if err != nil {
		t.Fatal(err)
	}
	name := "claude"
	if runtime.GOOS == "windows" {
		name = "claude.exe"
	}
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o700); err != nil { // #nosec G306 G703 -- executable test fixture under t.TempDir
		t.Fatal(err)
	}
	sidecar, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fakeclaude.json"), sidecar, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func fakeAuth(t *testing.T) map[string]any {
	t.Helper()
	return map[string]any{
		"loggedIn": true, "authMethod": "claude.ai", "apiProvider": "firstParty",
		"subscriptionType": "pro", "configDirectory": t.TempDir(), "orgId": "org-test-123",
	}
}

func claudeInit(source string) string {
	return fmt.Sprintf(`{"type":"system","subtype":"init","session_id":"run-test","model":"fake","claude_code_version":"2.1.284","permissionMode":"acceptEdits","apiKeySource":%q,"tools":[],"mcp_servers":[],"plugins":[]}`, source)
}

func claudeResult(subtype string, isError bool, extra string) string {
	frame := fmt.Sprintf(`{"type":"result","subtype":%q,"is_error":%t`, subtype, isError)
	if extra != "" {
		frame += "," + extra
	}
	return frame + `}`
}

// claudeRun is a complete claudecode command line; extra flags are appended.
func (f fixture) claudeRun(extra ...string) []string {
	return append([]string{"run", "--task-file", f.task, "--adapter", "claudecode",
		"--billing", "subscription-declared", "--execution-profile", "trusted-host",
		"--non-interactive", "--no-checks",
		"--declare-entitlement", "plan=pro,extra-usage=disabled"}, extra...)
}

func terminalAttempt(t *testing.T, f fixture, runID string) (state, reason string) {
	t.Helper()
	for _, ev := range f.events(t, runID) {
		if ev.Type != "attempt.state_changed" {
			continue
		}
		p := payloadOf(t, ev)
		if s, _ := p["state"].(string); s != "stop_requested" && s != "launching" && s != "running" {
			state, _ = p["state"].(string)
			reason, _ = p["reason"].(string)
		}
	}
	if state == "" {
		t.Fatalf("no terminal attempt.state_changed for %s", runID)
	}
	return state, reason
}

func eventPayload(t *testing.T, f fixture, runID, typ string) map[string]any {
	t.Helper()
	for _, ev := range f.events(t, runID) {
		if ev.Type == typ {
			return payloadOf(t, ev)
		}
	}
	t.Fatalf("no %s event for %s", typ, runID)
	return nil
}

// requireUnixClaude skips run-level tests where the probe refuses by
// design: process-tree ownership is unsupported on Windows, so admission
// ends 7 before any native starts (pinned by TestClaudeWindowsRefuses).
func requireUnixClaude(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("claudecode runs are unsupported on Windows; the refusal is pinned separately")
	}
}

func TestClaudeWindowsRefuses(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the Windows probe refusal runs on Windows only")
	}
	installFakeClaude(t, fakeClaudeConfig{Version: "2.1.284", Auth: fakeAuth(t)})
	f := newFixture(t)
	code, _, stderr := f.run(t, f.claudeRun()...)
	if code != 7 || !strings.Contains(stderr, "native_capability_unavailable") {
		t.Fatalf("exit %d, stderr %q; want 7 naming the refused capability", code, stderr)
	}
}

func TestApiKeySourceMismatchStopsAttempt(t *testing.T) {
	requireUnixClaude(t)
	installFakeClaude(t, fakeClaudeConfig{Version: "2.1.284", Auth: fakeAuth(t), Stream: []string{
		claudeInit("ANTHROPIC_API_KEY"),
		claudeResult("success", false, `"num_turns":1`),
	}})
	f := newFixture(t)
	// The strip flag proves the parent route was removed from the child;
	// the mismatch is then the native's own report, and still blocks.
	t.Setenv("ANTHROPIC_API_KEY", "planted-secret")
	code, stdout, stderr := f.run(t, f.claudeRun("--format", "jsonl", "--strip-credential-env")...)
	if code != 3 {
		t.Fatalf("exit %d, stderr %q; want 3", code, stderr)
	}
	run := f.onlyRun(t)
	if run.State != "blocked" || run.Reason != "billing_route_mismatch" {
		t.Fatalf("run projected %s/%s, want blocked/billing_route_mismatch", run.State, run.Reason)
	}
	res := lastResult(t, stdout)
	if res["exit_code"] != float64(3) || res["error_category"] != "admission_blocked" ||
		res["state"] != "blocked" || res["attempt_reason"] != "billing_route_mismatch" {
		t.Fatalf("run.result = %v", res)
	}
	if state, reason := terminalAttempt(t, f, run.RunID); state != "stopped" || reason != "billing_route_mismatch" {
		t.Fatalf("attempt %s/%s, want stopped/billing_route_mismatch", state, reason)
	}
	// The violating source is preserved as evidence, and the ladder ran.
	session := eventPayload(t, f, run.RunID, "attempt.native_session")
	if session["auth_source"] != "ANTHROPIC_API_KEY" {
		t.Fatalf("native_session.auth_source = %v", session["auth_source"])
	}
	stopped := eventPayload(t, f, run.RunID, "attempt.stopped")
	if confirmed, _ := stopped["confirmed"].(bool); !confirmed {
		t.Fatalf("attempt.stopped = %v, want confirmed", stopped)
	}
}

func TestApiKeySourceMismatchInterruptsLiveNative(t *testing.T) {
	requireUnixClaude(t)
	installFakeClaude(t, fakeClaudeConfig{Version: "2.1.284", Auth: fakeAuth(t), SleepAfterLines: 1, SleepMs: 30000, Stream: []string{
		claudeInit("ANTHROPIC_API_KEY"),
		claudeResult("success", false, `"num_turns":1`),
	}})
	f := newFixture(t)
	code, _, stderr := f.run(t, f.claudeRun("--format", "jsonl")...)
	if code != 3 {
		t.Fatalf("exit %d, stderr %q; want 3", code, stderr)
	}
	run := f.onlyRun(t)
	if run.State != "blocked" || run.Reason != "billing_route_mismatch" {
		t.Fatalf("run projected %s/%s, want blocked/billing_route_mismatch", run.State, run.Reason)
	}
	// The native was still running when init arrived, so the worker's
	// ladder must have signalled it: nothing here waited for an exit.
	stopped := eventPayload(t, f, run.RunID, "attempt.stopped")
	sent, _ := stopped["signals_sent"].([]any)
	if len(sent) == 0 {
		t.Fatalf("live native drew no signals: %v", stopped)
	}
	if confirmed, _ := stopped["confirmed"].(bool); !confirmed {
		t.Fatalf("attempt.stopped = %v, want confirmed", stopped)
	}
}

func TestBillingMismatchDuringUserStopStillBlocks(t *testing.T) {
	requireSignals(t)
	// The worker records its billing stop at init; the fake's ignored
	// SIGTERM holds the ladder in its grace so the user's interrupt
	// always moves the run to stopping before conclude runs.
	installFakeClaude(t, fakeClaudeConfig{Version: "2.1.284", Auth: fakeAuth(t),
		IgnoreTerm: true, SleepAfterLines: 1, SleepMs: 60000, Stream: []string{
			claudeInit("ANTHROPIC_API_KEY"),
			claudeResult("success", false, `"num_turns":1`),
		}})
	f := newFixture(t)
	r := startLive(t, f, f.claudeRun("--format", "jsonl")...)
	r.waitStdout(t, "the native session", isType("attempt.native_session"))
	if err := r.cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := r.wait(t)
	if code != 3 {
		t.Fatalf("exit %d, stderr %q; want 3", code, stderr)
	}
	run := f.onlyRun(t)
	if run.State != "blocked" || run.Reason != "billing_route_mismatch" {
		t.Fatalf("run projected %s/%s, want blocked/billing_route_mismatch", run.State, run.Reason)
	}
	// The run really passed through stopping: without the stopping to
	// blocked transition conclude would have failed instead.
	evs, err := decodeEnvelopes(strings.Join(stdout, "\n") + "\n")
	if err != nil {
		t.Fatal(err)
	}
	var stopping, blocked bool
	for _, ev := range evs {
		if ev.Type != "run.state_changed" {
			continue
		}
		switch payloadOf(t, ev)["state"] {
		case "stopping":
			stopping = true
		case "blocked":
			blocked = stopping
		}
	}
	if !stopping || !blocked {
		t.Fatalf("stopping=%t blocked-after-stopping=%t; want the run to stop first, then block", stopping, blocked)
	}
}

func TestReceiptReportsToolsHooksAndTrustGrant(t *testing.T) {
	requireUnixClaude(t)
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{"hooks":{"PreToolUse":[{"hooks":[{}]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := claudecode.InventoryAdmittedProject(home, filepath.Join(t.TempDir(), "no-such-workspace"), map[string][]byte{}, nil)
	if err != nil || !manifest.RequiresTrust {
		t.Fatalf("manifest = %+v, err = %v", manifest, err)
	}
	installFakeClaude(t, fakeClaudeConfig{Version: "2.1.284", Auth: fakeAuth(t), Stream: []string{
		claudeInit("none"),
		claudeResult("success", false, `"num_turns":1`),
	}})
	f := newFixture(t)
	f.home = home
	contents := "schema_version = 1\n[adapters.claudecode]\nallowed_tools = [\"Bash(go version)\"]\n" +
		"[[checks]]\nname = \"test\"\nargv = [" + strconv.Quote(os.Args[0]) + ", \"__check\", \"pass\"]\ntimeout = \"5s\"\n"
	if err := os.WriteFile(filepath.Join(f.repo, "mythhelm.toml"), []byte(contents), 0o600); err != nil { // #nosec G703 -- fixture repo is t.TempDir
		t.Fatal(err)
	}
	f.git(t, "add", "mythhelm.toml")
	f.git(t, "commit", "--quiet", "-m", "add checks and tool rules")
	_, digest, err := admission.ParseProjectConfig([]byte(contents))
	if err != nil {
		t.Fatal(err)
	}
	args := slices.DeleteFunc(f.claudeRun("--format", "jsonl"), func(s string) bool { return s == "--no-checks" })
	args = append(args, "--trust-project-config", "sha256:"+digest, "--trust-native-config", "sha256:"+manifest.Digest)
	if code, _, stderr := f.run(t, args...); code != 0 {
		t.Fatalf("exit %d, stderr %q; want 0", code, stderr)
	}
	raw, err := os.ReadFile(filepath.Join(f.state, "runs", f.onlyRun(t).RunID, "receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var r map[string]any
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	bundle, _ := r["execution_bundle"].(map[string]any)
	tools, _ := bundle["allowed_tools"].([]any)
	if len(tools) != 1 || tools[0] != "Bash(go version)" {
		t.Fatalf("execution_bundle.allowed_tools = %v, want the trusted rule", bundle["allowed_tools"])
	}
	native, _ := r["native_configuration"].(map[string]any)
	if native["hooks"] != float64(1) {
		t.Fatalf("native_configuration.hooks = %v, want 1", native["hooks"])
	}
	if want := "native_config:sha256:" + manifest.Digest; native["trust_grant"] != want {
		t.Fatalf("native_configuration.trust_grant = %v, want %s", native["trust_grant"], want)
	}
}

func TestTrustedAllowedToolsReachChildArgv(t *testing.T) {
	requireUnixClaude(t)
	argvFile := filepath.Join(t.TempDir(), "argv")
	installFakeClaude(t, fakeClaudeConfig{Version: "2.1.284", Auth: fakeAuth(t), ArgvFile: argvFile, Stream: []string{
		claudeInit("none"),
		claudeResult("success", false, `"num_turns":1`),
	}})
	f := newFixture(t)
	contents := "schema_version = 1\n[adapters.claudecode]\nallowed_tools = [\"Bash(go version)\"]\n" +
		"[[checks]]\nname = \"test\"\nargv = [" + strconv.Quote(os.Args[0]) + ", \"__check\", \"pass\"]\ntimeout = \"5s\"\n"
	if err := os.WriteFile(filepath.Join(f.repo, "mythhelm.toml"), []byte(contents), 0o600); err != nil { // #nosec G703 -- fixture repo is t.TempDir
		t.Fatal(err)
	}
	f.git(t, "add", "mythhelm.toml")
	f.git(t, "commit", "--quiet", "-m", "add checks and tool rules")
	_, digest, err := admission.ParseProjectConfig([]byte(contents))
	if err != nil {
		t.Fatal(err)
	}
	args := slices.DeleteFunc(f.claudeRun("--format", "jsonl"), func(s string) bool { return s == "--no-checks" })
	code, _, stderr := f.run(t, append(args, "--trust-project-config", "sha256:"+digest)...)
	if code != 0 {
		t.Fatalf("exit %d, stderr %q; want 0", code, stderr)
	}
	raw, err := os.ReadFile(argvFile) // #nosec G304 -- test-declared path under t.TempDir
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSpace(string(raw)), "\n")
	want := []string{"--allowedTools", "Bash(go version)"}
	if !slices.Equal(got[len(got)-len(want):], want) {
		t.Fatalf("child argv tail = %q, want %q last", got, want)
	}
}

func TestUserStopBeforeInitKeepsBillingBlock(t *testing.T) {
	requireSignals(t)
	// The native holds init back, so the user's interrupt is recorded
	// first (250 ms poll), and ignores SIGTERM, so it survives the
	// user's ladder long enough to report its mismatching source.
	// AC-4.4 is unconditional: the late mismatch still ends the run
	// blocked, with the user kept as requester.
	installFakeClaude(t, fakeClaudeConfig{Version: "2.1.284", Auth: fakeAuth(t), DelayMs: 3000, IgnoreTerm: true, Stream: []string{
		claudeInit("ANTHROPIC_API_KEY"),
		claudeResult("success", false, `"num_turns":1`),
	}})
	f := newFixture(t)
	r := startLive(t, f, f.claudeRun("--format", "jsonl")...)
	r.waitStdout(t, "the launch", isType("attempt.launched"))
	if err := r.cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := r.wait(t)
	if code != 3 {
		t.Fatalf("exit %d, stderr %q; want 3", code, stderr)
	}
	run := f.onlyRun(t)
	if run.State != "blocked" || run.Reason != "billing_route_mismatch" {
		t.Fatalf("run projected %s/%s, want blocked/billing_route_mismatch", run.State, run.Reason)
	}
	if state, reason := terminalAttempt(t, f, run.RunID); state != "stopped" || reason != "billing_route_mismatch" {
		t.Fatalf("attempt %s/%s, want stopped/billing_route_mismatch", state, reason)
	}
	// The first requester stands even as the policy reason applies.
	requested := eventPayload(t, f, run.RunID, "attempt.stop_requested")
	if requested["requested_by"] != "user-interrupt" {
		t.Fatalf("attempt.stop_requested = %v, want the user recorded first", requested)
	}
	evs, err := decodeEnvelopes(strings.Join(stdout, "\n") + "\n")
	if err != nil {
		t.Fatal(err)
	}
	var stopping, blocked bool
	for _, ev := range evs {
		if ev.Type != "run.state_changed" {
			continue
		}
		switch payloadOf(t, ev)["state"] {
		case "stopping":
			stopping = true
		case "blocked":
			blocked = stopping
		}
	}
	if !stopping || !blocked {
		t.Fatalf("stopping=%t blocked-after-stopping=%t; want the user stop first, then the block", stopping, blocked)
	}
}

func TestAuthFailedMapsToBlocked(t *testing.T) {
	requireUnixClaude(t)
	installFakeClaude(t, fakeClaudeConfig{Version: "2.1.284", Auth: fakeAuth(t), Exit: 1, Stream: []string{
		claudeInit("none"),
		`{"type":"assistant","error":"authentication_failed"}`,
		claudeResult("error_auth", true, ""),
	}})
	f := newFixture(t)
	code, stdout, stderr := f.run(t, f.claudeRun("--format", "jsonl")...)
	if code != 3 {
		t.Fatalf("exit %d, stderr %q; want 3", code, stderr)
	}
	run := f.onlyRun(t)
	if run.State != "blocked" || run.Reason != "native_auth_or_billing" {
		t.Fatalf("run projected %s/%s, want blocked/native_auth_or_billing", run.State, run.Reason)
	}
	res := lastResult(t, stdout)
	if res["exit_code"] != float64(3) || res["attempt_reason"] != "authentication_failed" {
		t.Fatalf("run.result = %v", res)
	}
	if state, reason := terminalAttempt(t, f, run.RunID); state != "failed_native" || reason != "authentication_failed" {
		t.Fatalf("attempt %s/%s, want failed_native/authentication_failed", state, reason)
	}
	// The class survives as journaled evidence, not just a reason string.
	if got := eventPayload(t, f, run.RunID, "attempt.native_error")["class"]; got != "authentication_failed" {
		t.Fatalf("attempt.native_error class = %v, want authentication_failed", got)
	}
}

func TestRateLimitNoInventedCountdown(t *testing.T) {
	requireUnixClaude(t)
	installFakeClaude(t, fakeClaudeConfig{Version: "2.1.284", Auth: fakeAuth(t), Exit: 1, Stream: []string{
		claudeInit("none"),
		`{"type":"assistant","error":{"class":"rate_limit"}}`,
		claudeResult("error_retry", true, ""),
	}})
	f := newFixture(t)
	code, stdout, stderr := f.run(t, f.claudeRun("--format", "jsonl")...)
	if code != 4 {
		t.Fatalf("exit %d, stderr %q; want 4", code, stderr)
	}
	run := f.onlyRun(t)
	if run.State != "failed" || run.Reason != "provider_limit" {
		t.Fatalf("run projected %s/%s, want failed/provider_limit", run.State, run.Reason)
	}
	res := lastResult(t, stdout)
	if res["exit_code"] != float64(4) || res["error_category"] != "native_failed" || res["attempt_reason"] != "provider_limit" {
		t.Fatalf("run.result = %v", res)
	}
	// No retry time is invented: the durable record carries none.
	native := eventPayload(t, f, run.RunID, "attempt.native_result")
	for key := range native {
		if strings.Contains(strings.ToLower(key), "retry") || strings.Contains(strings.ToLower(key), "countdown") {
			t.Fatalf("native_result invents %q: %v", key, native)
		}
	}
	// The receipt agrees with the CLI about the exit.
	receiptRaw, err := os.ReadFile(filepath.Join(f.state, "runs", run.RunID, "receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt map[string]any
	if err := json.Unmarshal(receiptRaw, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt["exit_code"] != float64(4) {
		t.Fatalf("receipt exit_code = %v, want 4", receipt["exit_code"])
	}
}

func TestRateLimitThenSuccessSucceeds(t *testing.T) {
	requireUnixClaude(t)
	installFakeClaude(t, fakeClaudeConfig{Version: "2.1.284", Auth: fakeAuth(t), Stream: []string{
		claudeInit("none"),
		`{"type":"assistant","error":"rate_limit"}`,
		claudeResult("success", false, `"num_turns":1`),
	}})
	f := newFixture(t)
	code, _, stderr := f.run(t, f.claudeRun("--format", "jsonl")...)
	if code != 5 { // --no-checks: ready_for_review/unverified
		t.Fatalf("exit %d, stderr %q; want 5 (a success after a mere rate limit is still a success)", code, stderr)
	}
	run := f.onlyRun(t)
	if run.State != "ready_for_review" || run.Reason != "unverified" {
		t.Fatalf("run projected %s/%s, want ready_for_review/unverified", run.State, run.Reason)
	}
	if state, _ := terminalAttempt(t, f, run.RunID); state != "succeeded_native" {
		t.Fatalf("attempt %s, want succeeded_native", state)
	}
}

func TestCostStoredAsEstimateString(t *testing.T) {
	requireUnixClaude(t)
	installFakeClaude(t, fakeClaudeConfig{Version: "2.1.284", Auth: fakeAuth(t), Stream: []string{
		claudeInit("none"),
		`{"type":"assistant","message":{"content":[{"type":"text","text":"synthetic"}]}}`,
		`{"type":"result","subtype":"success","is_error":false,"num_turns":1,"modelUsage":{"synthetic":{"inputTokens":100,"outputTokens":50,"cacheReadInputTokens":0,"cacheCreationInputTokens":0}},"total_cost_usd":0.0123,"permission_denials":[],"errors":[]}`,
	}})
	f := newFixture(t)
	code, _, stderr := f.run(t, f.claudeRun("--format", "jsonl")...)
	if code != 5 { // --no-checks: ready_for_review/unverified
		t.Fatalf("exit %d, stderr %q; want 5", code, stderr)
	}
	run := f.onlyRun(t)
	native := eventPayload(t, f, run.RunID, "attempt.native_result")
	// 0.0123 has no exact float64: only a literal-preserving decode keeps
	// the string, and the journal type pins that no float64 intervened.
	cost, ok := native["retail_equivalent_estimate_usd"].(string)
	if !ok || cost != "0.0123" {
		t.Fatalf("retail_equivalent_estimate_usd = %#v, want string \"0.0123\"", native["retail_equivalent_estimate_usd"])
	}
	usage, ok := native["usage_native_reported"].(map[string]any)
	if !ok {
		t.Fatalf("usage_native_reported = %#v, want the native-reported tokens", native["usage_native_reported"])
	}
	models, ok := usage["synthetic"].(map[string]any)
	if !ok || models["input"] != float64(100) || models["output"] != float64(50) {
		t.Fatalf("usage tokens = %#v", usage)
	}
}

func TestProtocolErrorMapsToFailed(t *testing.T) {
	requireUnixClaude(t)
	installFakeClaude(t, fakeClaudeConfig{Version: "2.1.284", Auth: fakeAuth(t), Stream: []string{
		`{"type":"assistant","message":{"content":[]}}`,
		claudeResult("success", false, `"num_turns":1`),
	}})
	f := newFixture(t)
	code, stdout, stderr := f.run(t, f.claudeRun("--format", "jsonl")...)
	if code != 4 {
		t.Fatalf("exit %d, stderr %q; want 4", code, stderr)
	}
	run := f.onlyRun(t)
	if run.State != "failed" || run.Reason != "protocol_error" {
		t.Fatalf("run projected %s/%s, want failed/protocol_error", run.State, run.Reason)
	}
	res := lastResult(t, stdout)
	if res["attempt_reason"] != "protocol_error" {
		t.Fatalf("run.result = %v", res)
	}
}

func TestClaudeCredentialOverrideBlocks(t *testing.T) {
	installFakeClaude(t, fakeClaudeConfig{Version: "2.1.284", Auth: fakeAuth(t)})
	f := newFixture(t)
	t.Setenv("ANTHROPIC_API_KEY", "planted-secret")
	code, _, stderr := f.run(t, f.claudeRun()...)
	if code != 3 || !strings.Contains(stderr, "ANTHROPIC_API_KEY") || strings.Contains(stderr, "planted-secret") {
		t.Fatalf("exit %d, stderr %q; want 3 naming only the variable", code, stderr)
	}
	if runs := f.runs(t); len(runs) != 0 {
		t.Fatalf("blocked admission recorded %d runs, want 0", len(runs))
	}
}

func TestClaudeRequiresDeclaration(t *testing.T) {
	requireUnixClaude(t)
	installFakeClaude(t, fakeClaudeConfig{Version: "2.1.284", Auth: fakeAuth(t)})
	f := newFixture(t)
	args := f.claudeRun()
	var stripped []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--declare-entitlement" {
			i++
			continue
		}
		stripped = append(stripped, args[i])
	}
	code, _, stderr := f.run(t, stripped...)
	if code != 3 || !strings.Contains(stderr, "entitlement_declaration_required") {
		t.Fatalf("exit %d, stderr %q; want 3 requiring a declaration", code, stderr)
	}
	if runs := f.runs(t); len(runs) != 0 {
		t.Fatalf("blocked admission recorded %d runs, want 0", len(runs))
	}
}

func TestClaudeBadDeclarationExits2(t *testing.T) {
	requireUnixClaude(t)
	installFakeClaude(t, fakeClaudeConfig{Version: "2.1.284", Auth: fakeAuth(t)})
	f := newFixture(t)
	args := f.claudeRun()
	for i, arg := range args {
		if arg == "--declare-entitlement" {
			args[i+1] = "plan=pro"
		}
	}
	code, _, stderr := f.run(t, args...)
	if code != 2 || !strings.Contains(stderr, "--declare-entitlement") {
		t.Fatalf("exit %d, stderr %q; want 2 for a malformed declaration", code, stderr)
	}
}

func TestClaudeTrustNativeConfigGrantLifecycle(t *testing.T) {
	requireUnixClaude(t)
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{"hooks":{"PreToolUse":[{"hooks":[{}]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// The manifest digest does not depend on the run's future workspace
	// while no project source selects on it, so the test can compute the
	// exact digest the run must require.
	manifest, err := claudecode.InventoryAdmittedProject(home, filepath.Join(t.TempDir(), "no-such-workspace"), map[string][]byte{}, nil)
	if err != nil || !manifest.RequiresTrust {
		t.Fatalf("manifest = %+v, err = %v", manifest, err)
	}
	installFakeClaude(t, fakeClaudeConfig{Version: "2.1.284", Auth: fakeAuth(t), Stream: []string{
		claudeInit("none"),
		claudeResult("success", false, `"num_turns":1`),
	}})
	f := newFixture(t)
	f.home = home
	code, _, stderr := f.run(t, f.claudeRun("--trust-native-config", "sha256:"+manifest.Digest)...)
	if code != 5 {
		t.Fatalf("explicit grant exit %d, stderr %q; want 5 (ready_for_review/unverified)", code, stderr)
	}
	// The grant persisted: a second run without the flag is admitted.
	code, _, stderr = f.run(t, f.claudeRun()...)
	if code != 5 {
		t.Fatalf("stored grant exit %d, stderr %q; want 5", code, stderr)
	}
	if n := len(f.runs(t)); n != 2 {
		t.Fatalf("recorded runs = %d, want 2", n)
	}
}

func TestClaudeUntrustedNativeConfigBlocks(t *testing.T) {
	requireUnixClaude(t)
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{"hooks":{"PreToolUse":[{"hooks":[{}]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	installFakeClaude(t, fakeClaudeConfig{Version: "2.1.284", Auth: fakeAuth(t)})
	f := newFixture(t)
	f.home = home
	code, _, stderr := f.run(t, f.claudeRun()...)
	if code != 3 || !strings.Contains(stderr, "untrusted_native_config") || !strings.Contains(stderr, "--trust-native-config") {
		t.Fatalf("exit %d, stderr %q; want 3 naming --trust-native-config", code, stderr)
	}
}
