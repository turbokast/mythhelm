package claudecode

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/turbokast/mythhelm/internal/adapter"
)

func TestArgvExact(t *testing.T) {
	t.Parallel()
	bin := "/resolved/claude-2.1.284"
	got := Argv(bin, nil)
	want := []string{bin, "-p", "--output-format", "stream-json", "--verbose",
		"--input-format", "text", "--permission-mode", "acceptEdits",
		"--permission-prompts", "none"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv = %q, want %q", got, want)
	}
	rules := []string{"Bash(go build *)", "Bash(git status)"}
	got = Argv(bin, rules)
	want = append(want, "--allowedTools", "Bash(go build *)", "Bash(git status)")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv with rules = %q, want %q", got, want)
	}
	joined := strings.Join(got, " ")
	for _, forbidden := range []string{"--bare", "--dangerously-skip-permissions", "bypassPermissions", "auto"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("argv %q contains forbidden %q", got, forbidden)
		}
	}
}

// stubLauncher runs the proposal's spec for real through os/exec, with extra
// test-only control variables appended. The extras drive fakeclaude; the
// proposal's own env is asserted by it, not by the parent.
type stubLauncher struct {
	t     *testing.T
	extra []string
}

func (l *stubLauncher) Launch(_ context.Context, spec adapter.ProcSpec) (adapter.OwnedProc, error) {
	l.t.Helper()
	cmd := exec.Command(spec.Path, spec.Args...) // #nosec G204 -- Test launches the proposal's own resolved binary.
	cmd.Dir = spec.Dir
	cmd.Env = append(append([]string{}, spec.Env...), append(l.extra, "GORACE=atexit_sleep_ms=0")...)
	cmd.Stdin = spec.Stdin
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		l.t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &stubProc{cmd: cmd, stdout: stdout}, nil
}

type stubProc struct {
	cmd    *exec.Cmd
	stdout io.Reader
	mu     sync.Mutex
	exited bool
}

func (p *stubProc) Stdout() io.Reader { return p.stdout }

func (p *stubProc) Signal(sig adapter.StopSignal) error {
	if sig == adapter.StopKill {
		return p.cmd.Process.Kill()
	}
	return p.cmd.Process.Signal(os.Interrupt)
}

func (p *stubProc) Wait() adapter.NativeExit {
	err := p.cmd.Wait()
	p.mu.Lock()
	p.exited = true
	p.mu.Unlock()
	if err == nil {
		return adapter.NativeExit{Code: 0}
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return adapter.NativeExit{Code: exitErr.ExitCode()}
	}
	return adapter.NativeExit{Code: -1, Err: err}
}

func (p *stubProc) GroupGone() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exited
}

func launchProbe(t *testing.T) adapter.Probe {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sum, err := fileSHA256(exe)
	if err != nil {
		t.Fatal(err)
	}
	return adapter.Probe{Executable: exe, Version: "2.1.284", SHA256: sum, OS: runtime.GOOS, Arch: runtime.GOARCH}
}

// runLaunch prepares and starts through the stub launcher, drains the
// session and returns its observations and exit.
func runLaunch(t *testing.T, in adapter.PrepareInput, extra []string) ([]adapter.Observation, adapter.NativeExit) {
	t.Helper()
	lp, err := New().Prepare(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := New().Start(context.Background(), lp, &stubLauncher{t: t, extra: extra})
	if err != nil {
		t.Fatal(err)
	}
	var obs []adapter.Observation
	for ob := range sess.Observations() {
		obs = append(obs, ob)
	}
	return obs, <-sess.Done()
}

func TestPromptOnStdinOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Prepare refuses Windows by design; the probe tests pin the refusal")
	}
	t.Parallel()
	const prompt = "# Task PROMPT-MARKER-7f3a\n\nDo the thing.\n"
	promptPath := filepath.Join(t.TempDir(), "prompt.md")
	if err := os.WriteFile(promptPath, []byte(prompt), 0o600); err != nil {
		t.Fatal(err)
	}
	stream, err := filepath.Abs(filepath.Join("testdata", "streams", "success.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	obs, exit := runLaunch(t, adapter.PrepareInput{
		Workdir:   t.TempDir(),
		AttemptID: "att_test",
		Env:       []string{"PATH=/usr/bin", "HOME=/home/test"},
		Prompt:    strings.NewReader(prompt),
		Probe:     launchProbe(t),
	}, []string{"GO_WANT_FAKECLAUDE=1", "FAKE_PROMPT=" + promptPath,
		"FAKE_PROMPT_MARKER=PROMPT-MARKER-7f3a", "FAKE_ATTEMPT=att_test", "FAKE_STREAM=" + stream})
	if exit.Code != 0 {
		t.Fatalf("fakeclaude exit = %+v, want 0 (an assertion failed; see stderr)", exit)
	}
	if _, ok := obs[0].(adapter.SessionStarted); !ok {
		t.Fatalf("first observation is %T, want SessionStarted", obs[0])
	}
}

func TestChildEnvIsAllowlisted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Prepare refuses Windows by design; the probe tests pin the refusal")
	}
	t.Parallel()
	promptPath := filepath.Join(t.TempDir(), "prompt.md")
	if err := os.WriteFile(promptPath, []byte("task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	in := adapter.PrepareInput{
		Workdir:   t.TempDir(),
		AttemptID: "att_env",
		Env: []string{"PATH=/usr/bin", "HOME=/home/test", "GOPATH=/go",
			"ANTHROPIC_API_KEY=planted-secret", "AWS_SECRET_ACCESS_KEY=planted-secret",
			"SHELL=/bin/sh"},
		Prompt:      strings.NewReader("task\n"),
		Probe:       launchProbe(t),
		Passthrough: []string{"GOPATH"},
	}
	lp, err := New().Prepare(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(lp.Spec.Env, "\n")
	if strings.Contains(joined, "planted-secret") {
		t.Fatalf("proposal env leaks planted secrets: %q", lp.Spec.Env)
	}
	for _, kv := range []string{"DISABLE_AUTOUPDATER=1", "CLAUDE_CODE_STARTUP_FAILURE_RESULTS=1",
		"MYTHHELM_ATTEMPT_ID=att_env", "GOPATH=/go", "PATH=/usr/bin"} {
		if !strings.Contains(joined+"\n", kv+"\n") {
			t.Fatalf("proposal env %q misses %q", lp.Spec.Env, kv)
		}
	}
	// The child process itself confirms what it received.
	_, exit := runLaunch(t, in, []string{"GO_WANT_FAKECLAUDE=1", "FAKE_PROMPT=" + promptPath,
		"FAKE_ATTEMPT=att_env", "FAKE_PASSTHROUGH=GOPATH=/go"})
	if exit.Code != 0 {
		t.Fatalf("fakeclaude exit = %+v, want 0 (an assertion failed; see stderr)", exit)
	}
}

// canaryVerdict classifies a drained canary stream. A missing session or
// result fails, unless the only barrier was a provider rate limit: retry
// after the reset is the maintainer's call, not a product verdict. A
// result after a mere limit still passes, matching the worker's success
// row (design §6.4).
func canaryVerdict(sawSession, sawResult, sawRateLimit bool) string {
	switch {
	case sawSession && sawResult:
		return "pass"
	case sawRateLimit && !sawResult:
		return "limited"
	default:
		return "fail"
	}
}

func TestCanaryVerdict(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		session, result, limited bool
		want                     string
	}{
		{true, true, false, "pass"},
		{true, true, true, "pass"},
		{true, false, true, "limited"},
		{false, false, true, "limited"},
		{true, false, false, "fail"},
		{false, true, false, "fail"},
		{false, false, false, "fail"},
		{false, true, true, "fail"},
	} {
		if got := canaryVerdict(tc.session, tc.result, tc.limited); got != tc.want {
			t.Fatalf("canaryVerdict(%v,%v,%v) = %q, want %q",
				tc.session, tc.result, tc.limited, got, tc.want)
		}
	}
}

func TestCINeverPassesLiveTag(t *testing.T) {
	t.Parallel()
	entries, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*.yml"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("workflows not found: %v", err)
	}
	for _, path := range entries {
		raw, err := os.ReadFile(path) // #nosec G304 -- Test reads only the repo's own workflow files from the fixed glob.
		if err != nil {
			t.Fatal(err)
		}
		for line := range strings.SplitSeq(string(raw), "\n") {
			if strings.Contains(line, "-tags") && strings.Contains(line, "live") {
				t.Fatalf("%s passes the live tag: %q (the canary consumes the maintainer's allowance)", path, line)
			}
		}
	}
}

func TestPrepareRejectsSwappedBinary(t *testing.T) {
	t.Parallel()
	in := adapter.PrepareInput{
		Workdir:   t.TempDir(),
		AttemptID: "att_test",
		Env:       []string{"PATH=/usr/bin"},
		Prompt:    strings.NewReader("task\n"),
		Probe:     launchProbe(t),
	}
	in.Probe.SHA256 = strings.Repeat("0", 64)
	if _, err := New().Prepare(context.Background(), in); err == nil {
		t.Fatal("Prepare must refuse a binary whose hash changed since probe")
	}
}

func TestPrepareRejectsBadInput(t *testing.T) {
	t.Parallel()
	base := adapter.PrepareInput{
		Workdir:   t.TempDir(),
		AttemptID: "att_test",
		Env:       []string{"PATH=/usr/bin"},
		Prompt:    strings.NewReader("task\n"),
		Probe:     launchProbe(t),
	}
	cases := map[string]func(*adapter.PrepareInput){
		"relative workdir": func(in *adapter.PrepareInput) { in.Workdir = "relative" },
		"missing attempt":  func(in *adapter.PrepareInput) { in.AttemptID = "" },
		"missing probe":    func(in *adapter.PrepareInput) { in.Probe = adapter.Probe{} },
		"missing prompt":   func(in *adapter.PrepareInput) { in.Prompt = nil },
		"empty rule":       func(in *adapter.PrepareInput) { in.AllowedTools = []string{""} },
		"dash rule":        func(in *adapter.PrepareInput) { in.AllowedTools = []string{"--dangerously-skip-permissions"} },
		"short dash rule":  func(in *adapter.PrepareInput) { in.AllowedTools = []string{"-p"} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in := base
			mutate(&in)
			if _, err := New().Prepare(context.Background(), in); err == nil {
				t.Fatalf("%s must be refused", name)
			}
		})
	}
}

func TestPluginsNeverWidenArgv(t *testing.T) {
	t.Parallel()
	// Hostile plugin/MCP names shaped like argv flags, tool rules and env
	// assignments: none may reach the native argv or environment.
	manifest, err := InventoryAdmittedProject(t.TempDir(), t.TempDir(), map[string][]byte{
		"project": []byte(`{"enabledPlugins":{"--dangerously-skip-permissions":true,"Bash(rm -rf /tmp/pwned)":true,"EVIL_SMUGGLER=1":true},"mcpServers":{"evil-server":{},"smuggled-env":{}}}`),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.EnabledPlugins) != 3 || len(manifest.MCPServers) != 2 {
		t.Fatalf("manifest = %+v, want 3 enabled plugins and 2 MCP servers", manifest)
	}
	hostile := append(append([]string{}, manifest.EnabledPlugins...), "evil-server", "smuggled-env")
	newInput := func() adapter.PrepareInput {
		return adapter.PrepareInput{
			Workdir:   t.TempDir(),
			AttemptID: "att_plugins",
			Env:       []string{"PATH=/usr/bin", "HOME=/home/test"},
			Prompt:    strings.NewReader("task\n"),
			Probe:     launchProbe(t),
		}
	}
	if runtime.GOOS == "windows" {
		// Prepare refuses Windows by design; the probe tests pin the
		// refusal. The manifest and seam-pin assertions still run below.
		if _, err := New().Prepare(context.Background(), newInput()); err == nil {
			t.Fatal("Prepare must refuse Windows by design")
		}
	} else {
		// One Prepare call: its input carries no manifest, so the
		// plugin/MCP-bearing inventory above cannot reach the launch.
		// The test pins the exact argv and scans every launch element
		// for the hostile names.
		proposal, err := New().Prepare(context.Background(), newInput())
		if err != nil {
			t.Fatal(err)
		}
		if want := Argv(newInput().Probe.Executable, nil); !reflect.DeepEqual(proposal.Spec.Args, want[1:]) {
			t.Fatalf("argv = %q, want exactly %q", proposal.Spec.Args, want[1:])
		}
		for _, element := range append(append([]string{}, proposal.Spec.Args...), proposal.Spec.Env...) {
			for _, smuggled := range hostile {
				if strings.Contains(element, smuggled) {
					t.Fatalf("launch element %q carries plugin/MCP content %q", element, smuggled)
				}
			}
		}
	}
	// The seam pin: every PrepareInput field must be allowlisted here, so
	// a new field fails until it is reviewed for plugin/MCP carriage.
	// Each listed field is reviewed: none carries manifest, plugin or
	// MCP content into Prepare.
	allowed := map[string]bool{
		"Workdir": true, "AttemptID": true, "Env": true, "Prompt": true,
		"Scenario": true, "Probe": true, "AllowedTools": true, "Passthrough": true,
	}
	for field := range reflect.TypeFor[adapter.PrepareInput]().Fields() {
		if !allowed[field.Name] {
			t.Fatalf("PrepareInput.%s is not allowlisted; review it for plugin/MCP carriage and extend this test", field.Name)
		}
	}
}

func TestSurplusRoutesStayUnknown(t *testing.T) {
	t.Parallel()
	m := Manifest{
		EnabledPlugins: []string{"alpha", "beta", "gamma"},
		MCPServers:     []string{"user:files", "project:search"},
	}
	routes := InventoryAuxiliary(m, adapter.SessionStarted{
		PluginCount: 5,
		MCPServers: []adapter.MCPServer{
			{Name: "files", Status: "connected"},
			{Name: "surprise", Status: "connected"},
		},
	})
	var pluginUnknown, mcpUnknown int
	for _, r := range routes {
		if r.Funding != "unknown" {
			t.Errorf("route %q funding = %q, want unknown (static inventory proves no funding)", r.Name, r.Funding)
		}
		switch r.Name {
		case "plugin:unknown":
			pluginUnknown++
			if r.Evidence != "plugin-count:2" {
				t.Errorf("plugin:unknown evidence = %q, want plugin-count:2", r.Evidence)
			}
		case "mcp:unknown":
			mcpUnknown++
			if r.Evidence != "mcp-session:surprise" {
				t.Errorf("mcp:unknown evidence = %q, want mcp-session:surprise", r.Evidence)
			}
		}
	}
	if pluginUnknown != 1 {
		t.Errorf("plugin:unknown routes = %d, want 1 (session reports 5 plugins, manifest names 3)", pluginUnknown)
	}
	if mcpUnknown != 1 {
		t.Errorf("mcp:unknown routes = %d, want 1 (session reports an unnamed server)", mcpUnknown)
	}
}
