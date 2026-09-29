package fake

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
)

// TestMain lets the test binary play the fake agent: the adapter runs
// os.Executable() as "__fake-agent ...", which here is this binary.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == AgentCommand {
		os.Exit(AgentMain(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

// execLauncher stands in for the worker's launcher. Test files may use
// os/exec; the adapter itself may not.
type execLauncher struct{ stderr bytes.Buffer }

func (l *execLauncher) Launch(_ context.Context, spec adapter.ProcSpec) (adapter.OwnedProc, error) {
	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.Dir, cmd.Env, cmd.Stdin, cmd.Stderr = spec.Dir, spec.Env, spec.Stdin, &l.stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &execProc{cmd: cmd, out: out}, nil
}

type execProc struct {
	cmd    *exec.Cmd
	out    io.Reader
	waited atomic.Bool
}

func (p *execProc) Stdout() io.Reader { return p.out }

func (p *execProc) Signal(sig adapter.StopSignal) error {
	switch sig {
	case adapter.StopInterrupt:
		return p.cmd.Process.Signal(os.Interrupt)
	case adapter.StopTerminate:
		return p.cmd.Process.Signal(syscall.SIGTERM)
	default:
		return p.cmd.Process.Kill()
	}
}

func (p *execProc) Wait() adapter.NativeExit {
	err := p.cmd.Wait()
	p.waited.Store(true)
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		return adapter.NativeExit{Code: -1, Err: err}
	}
	exit := adapter.NativeExit{Code: p.cmd.ProcessState.ExitCode()}
	if sig, ok := strings.CutPrefix(p.cmd.ProcessState.String(), "signal: "); ok {
		exit.Signal = sig
	}
	return exit
}

func (p *execProc) GroupGone() bool { return p.waited.Load() }

type outcome struct {
	obs    []adapter.Observation
	exit   adapter.NativeExit
	stderr string
}

func (o outcome) find(t *testing.T, want adapter.Observation) adapter.Observation {
	t.Helper()
	for _, ob := range o.obs {
		if reflect.TypeOf(ob) == reflect.TypeOf(want) {
			return ob
		}
	}
	t.Fatalf("no %T observed; got %#v (stderr %q)", want, o.obs, o.stderr)
	return nil
}

func (o outcome) counters(t *testing.T) adapter.ProtocolCounters {
	t.Helper()
	last, ok := o.obs[len(o.obs)-1].(adapter.ProtocolCounters)
	if !ok {
		t.Fatalf("last observation = %T, want ProtocolCounters", o.obs[len(o.obs)-1])
	}
	return last
}

func prepare(t *testing.T, scenario, workdir string) adapter.LaunchProposal {
	t.Helper()
	return prepareAttempt(t, scenario, workdir, "att_test")
}

func prepareAttempt(t *testing.T, scenario, workdir, attemptID string) adapter.LaunchProposal {
	t.Helper()
	lp, err := New().Prepare(context.Background(), adapter.PrepareInput{
		Workdir:   workdir,
		AttemptID: attemptID,
		// The race detector otherwise sleeps a second before a clean exit.
		Env:      append(os.Environ(), "GORACE=atexit_sleep_ms=0"),
		Prompt:   strings.NewReader("# Task\nedit a file\n"),
		Scenario: scenario,
	})
	if err != nil {
		t.Fatalf("Prepare(%s): %v", scenario, err)
	}
	return lp
}

func start(t *testing.T, lp adapter.LaunchProposal) (adapter.Session, *execLauncher) {
	t.Helper()
	l := &execLauncher{}
	s, err := New().Start(context.Background(), lp, l)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return s, l
}

func runScenario(t *testing.T, scenario string) (outcome, string) {
	t.Helper()
	dir := t.TempDir()
	return collect(t, prepare(t, scenario, dir)), dir
}

func collect(t *testing.T, lp adapter.LaunchProposal) outcome {
	t.Helper()
	s, l := start(t, lp)
	var o outcome
	for ob := range s.Observations() {
		o.obs = append(o.obs, ob)
	}
	select {
	case o.exit = <-s.Done():
	case <-time.After(30 * time.Second):
		t.Fatalf("%v did not exit", lp.Spec.Args)
	}
	o.stderr = l.stderr.String()
	return o
}

func TestFakeScenarioHappyEditsFile(t *testing.T) {
	o, dir := runScenario(t, "happy")
	if o.exit.Code != 0 || o.exit.Err != nil {
		t.Fatalf("exit = %+v, want code 0 (stderr %q)", o.exit, o.stderr)
	}
	got, err := fs.ReadFile(os.DirFS(dir), "demo.txt")
	if err != nil || string(got) != "ok\n" {
		t.Errorf("demo.txt = %q, %v; want %q", got, err, "ok\n")
	}
	started := o.find(t, adapter.SessionStarted{}).(adapter.SessionStarted)
	if started.SessionID != "fake-happy" || started.APIKeySource != "none" {
		t.Errorf("SessionStarted = %+v, want session fake-happy with apiKeySource none", started)
	}
	if p := o.find(t, adapter.Progress{}).(adapter.Progress); p != (adapter.Progress{Turn: 1, Tool: "Write"}) {
		t.Errorf("Progress = %+v, want turn 1 tool Write", p)
	}
	res := o.find(t, adapter.Result{}).(adapter.Result)
	if res.Subtype != "success" || res.IsError || res.NumTurns == nil || *res.NumTurns != 1 {
		t.Errorf("Result = %+v, want a successful result of 1 turn", res)
	}
	if res.CostUSD != "" || res.Tokens != nil {
		t.Errorf("Result reports cost %q and tokens %v; the fake has neither, so both must stay unknown", res.CostUSD, res.Tokens)
	}
	if c := o.counters(t); c.Frames != 3 || c.Malformed+c.Oversized+c.InvalidUTF8+c.DepthExceeded != 0 {
		t.Errorf("counters = %+v, want 3 frames and nothing dropped", c)
	}
}

func TestFakeScenarioOutcomes(t *testing.T) {
	t.Run("native-fails", func(t *testing.T) {
		o, _ := runScenario(t, "native-fails")
		res := o.find(t, adapter.Result{}).(adapter.Result)
		if !res.IsError || res.Subtype != "error_during_execution" || o.exit.Code != 1 {
			t.Errorf("Result = %+v, exit = %+v; want error_during_execution and exit 1", res, o.exit)
		}
	})
	t.Run("check-fails", func(t *testing.T) {
		o, dir := runScenario(t, "check-fails")
		got, _ := fs.ReadFile(os.DirFS(dir), "demo.txt")
		if res := o.find(t, adapter.Result{}).(adapter.Result); res.Subtype != "success" || string(got) != "fail\n" {
			t.Errorf("Result = %+v, demo.txt = %q; want a native success that wrote %q", res, got, "fail\n")
		}
	})
	t.Run("denied", func(t *testing.T) {
		o, _ := runScenario(t, "denied")
		if d := o.find(t, adapter.PermissionDenied{}).(adapter.PermissionDenied); d.ToolName != "Bash" {
			t.Errorf("PermissionDenied = %+v, want Bash", d)
		}
		if res := o.find(t, adapter.Result{}).(adapter.Result); !slices.Equal(res.PermissionDenials, []string{"Bash"}) {
			t.Errorf("Result.PermissionDenials = %q, want [Bash]", res.PermissionDenials)
		}
	})
	t.Run("exit-before-result", func(t *testing.T) {
		o, _ := runScenario(t, "exit-before-result")
		for _, ob := range o.obs {
			if _, ok := ob.(adapter.Result); ok {
				t.Fatalf("observed a Result; the scenario exits without one")
			}
		}
		if o.exit.Code != 0 {
			t.Errorf("exit = %+v, want 0", o.exit)
		}
	})
}

func TestFakeScenarioProtocolFaultsCounted(t *testing.T) {
	tests := []struct {
		scenario string
		field    func(adapter.ProtocolCounters) int64
		want     int64
	}{
		{"malformed", func(c adapter.ProtocolCounters) int64 { return c.Malformed }, 3},
		{"deep", func(c adapter.ProtocolCounters) int64 { return c.DepthExceeded }, 1},
		{"oversized", func(c adapter.ProtocolCounters) int64 { return c.Oversized }, 1},
		{"bad-utf8", func(c adapter.ProtocolCounters) int64 { return c.InvalidUTF8 }, 1},
	}
	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			o, _ := runScenario(t, tt.scenario)
			c := o.counters(t)
			if got := tt.field(c); got != tt.want {
				t.Errorf("counter = %d, want %d (all counters %+v)", got, tt.want, c)
			}
			// The fault is dropped and the stream carries on to its result.
			if res := o.find(t, adapter.Result{}).(adapter.Result); res.Subtype != "success" {
				t.Errorf("Result = %+v, want success after the dropped frame", res)
			}
		})
	}
}

func TestDecodeUnknownTypesBounded(t *testing.T) {
	d := newDecoder()
	for i := range maxUnknownTypes + 5 {
		d.decode([]byte(`{"type":"fake.x` + strings.Repeat("y", i) + `"}`))
	}
	d.decode([]byte(`{"type":"` + strings.Repeat("z", 100) + `"}`))
	d.decode([]byte(`{"type":"bad\u001btype"}`))
	if len(d.unknown) != maxUnknownTypes+2 {
		t.Errorf("distinct unknown-type keys = %d, want %d named plus other and invalid", len(d.unknown), maxUnknownTypes+2)
	}
	if d.unknown["vendor.fake.other"] != 5 || d.unknown["vendor.fake.invalid"] != 2 {
		t.Errorf("other = %d, invalid = %d; want 5 and 2", d.unknown["vendor.fake.other"], d.unknown["vendor.fake.invalid"])
	}
}

func TestPrepareProposal(t *testing.T) {
	dir := t.TempDir()
	lp := prepare(t, "happy", dir)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		t.Fatal(err)
	}
	if lp.Spec.Path != exe || lp.Spec.Dir != dir {
		t.Errorf("Spec path, dir = %q, %q; want %q, %q", lp.Spec.Path, lp.Spec.Dir, exe, dir)
	}
	if want := []string{AgentCommand, "--scenario", "happy", "--workdir", dir}; !slices.Equal(lp.Spec.Args, want) {
		t.Errorf("Spec.Args = %q, want %q", lp.Spec.Args, want)
	}
	if !slices.Contains(lp.Spec.Env, "MYTHHELM_ATTEMPT_ID=att_test") {
		t.Errorf("child env lacks the MYTHHELM_ATTEMPT_ID marker")
	}
	if lp.Spec.Stdin == nil {
		t.Errorf("Spec.Stdin is nil; the prompt goes on stdin")
	}
	if lp.Billing.Mode != "local-scripted" || lp.Billing.Qualified {
		t.Errorf("Billing = %+v, want unqualified local-scripted", lp.Billing)
	}
	if len(lp.Overrides) != 1 || lp.Overrides[0].Name != "MYTHHELM_ATTEMPT_ID" {
		t.Errorf("Overrides = %+v, want the attempt marker recorded", lp.Overrides)
	}
}

func TestPrepareRejectsBadInput(t *testing.T) {
	for _, in := range []adapter.PrepareInput{
		{Workdir: t.TempDir(), AttemptID: "att_1", Scenario: "nope"},
		{Workdir: t.TempDir(), AttemptID: "att_1"},
		{AttemptID: "att_1", Scenario: "happy"},
		{Workdir: t.TempDir(), Scenario: "happy"},
		{Workdir: "relative/dir", AttemptID: "att_1", Scenario: "happy"},
	} {
		if _, err := New().Prepare(context.Background(), in); err == nil {
			t.Errorf("Prepare(%+v) succeeded, want an error", in)
		}
	}
	_, err := New().Prepare(context.Background(), adapter.PrepareInput{Workdir: t.TempDir(), AttemptID: "att_1", Scenario: "nope"})
	if !errors.Is(err, ErrUnknownScenario) {
		t.Errorf("error = %v, want ErrUnknownScenario", err)
	}
}

func TestStopLadder(t *testing.T) {
	unix := []adapter.StopStep{{Signal: adapter.StopInterrupt, Grace: 3 * time.Second}, {Signal: adapter.StopTerminate, Grace: 3 * time.Second}, {Signal: adapter.StopKill, Grace: 5 * time.Second}}
	for goos, want := range map[string][]adapter.StopStep{
		"linux":   unix,
		"darwin":  unix,
		"windows": {{Signal: adapter.StopKill, Grace: 5 * time.Second}},
	} {
		if got := stopLadder(goos); !slices.Equal(got, want) {
			t.Errorf("stopLadder(%s) = %+v, want %+v", goos, got, want)
		}
	}
}

// scriptedProc is an OwnedProc whose group disappears after a chosen signal.
type scriptedProc struct {
	mu     sync.Mutex
	diesOn adapter.StopSignal
	sent   []adapter.StopSignal
	gone   bool
}

func (p *scriptedProc) Stdout() io.Reader { return strings.NewReader("") }
func (p *scriptedProc) Wait() adapter.NativeExit {
	return adapter.NativeExit{Code: -1, Signal: "killed"}
}

func (p *scriptedProc) Signal(sig adapter.StopSignal) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sent = append(p.sent, sig)
	if sig == p.diesOn {
		p.gone = true
	}
	return nil
}

func (p *scriptedProc) GroupGone() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.gone
}

type procLauncher struct{ proc adapter.OwnedProc }

func (l procLauncher) Launch(context.Context, adapter.ProcSpec) (adapter.OwnedProc, error) {
	return l.proc, nil
}

func TestInterruptRunsLadderUntilConfirmed(t *testing.T) {
	ladder := []adapter.StopStep{{Signal: adapter.StopInterrupt, Grace: 20 * time.Millisecond}, {Signal: adapter.StopTerminate, Grace: 20 * time.Millisecond}, {Signal: adapter.StopKill, Grace: 20 * time.Millisecond}}
	tests := []struct {
		diesOn    adapter.StopSignal
		sent      []adapter.StopSignal
		confirmed bool
	}{
		{adapter.StopInterrupt, []adapter.StopSignal{adapter.StopInterrupt}, true},
		{adapter.StopKill, []adapter.StopSignal{adapter.StopInterrupt, adapter.StopTerminate, adapter.StopKill}, true},
		{"never", []adapter.StopSignal{adapter.StopInterrupt, adapter.StopTerminate, adapter.StopKill}, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.diesOn), func(t *testing.T) {
			proc := &scriptedProc{diesOn: tt.diesOn}
			s, err := New().Start(context.Background(), adapter.LaunchProposal{StopLadder: ladder}, procLauncher{proc})
			if err != nil {
				t.Fatal(err)
			}
			rep := s.Interrupt(context.Background())
			if !slices.Equal(rep.Sent, tt.sent) || rep.Confirmed != tt.confirmed {
				t.Errorf("report = %+v, want sent %v confirmed %v", rep, tt.sent, tt.confirmed)
			}
			if !slices.Equal(proc.sent, tt.sent) {
				t.Errorf("signals delivered = %v, want %v", proc.sent, tt.sent)
			}
		})
	}
}

func TestInterruptAlreadyGoneSendsNothing(t *testing.T) {
	proc := &scriptedProc{gone: true}
	s, err := New().Start(context.Background(), adapter.LaunchProposal{StopLadder: stopLadder("linux")}, procLauncher{proc})
	if err != nil {
		t.Fatal(err)
	}
	if rep := s.Interrupt(context.Background()); len(rep.Sent) != 0 || !rep.Confirmed {
		t.Errorf("report = %+v, want nothing sent and confirmed", rep)
	}
}

// waitStarted drains observations until the session has started, so that
// the scenario's signal handling is in place, and keeps draining after.
func waitStarted(t *testing.T, s adapter.Session) {
	t.Helper()
	for ob := range s.Observations() {
		if _, ok := ob.(adapter.SessionStarted); ok {
			go func() {
				for range s.Observations() {
				}
			}()
			return
		}
	}
	t.Fatal("stream ended before the session started")
}

func TestFakeStopLadderAgainstRealChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows stops the fake with Process.Kill only; there is no ladder to climb")
	}
	tests := []struct {
		scenario string
		sent     []adapter.StopSignal
		exit     adapter.NativeExit
	}{
		{"slow", []adapter.StopSignal{adapter.StopInterrupt}, adapter.NativeExit{Code: 130}},
		{"ignore-sigint", []adapter.StopSignal{adapter.StopInterrupt, adapter.StopTerminate}, adapter.NativeExit{Code: 143}},
		{"ignore-term", []adapter.StopSignal{adapter.StopInterrupt, adapter.StopTerminate, adapter.StopKill}, adapter.NativeExit{Code: -1, Signal: "killed"}},
	}
	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			lp := prepare(t, tt.scenario, t.TempDir())
			for i := range lp.StopLadder {
				lp.StopLadder[i].Grace = 2 * time.Second
			}
			s, _ := start(t, lp)
			waitStarted(t, s)
			rep := s.Interrupt(context.Background())
			if !slices.Equal(rep.Sent, tt.sent) || !rep.Confirmed {
				t.Errorf("report = %+v, want sent %v and confirmed", rep, tt.sent)
			}
			if exit := <-s.Done(); exit != tt.exit {
				t.Errorf("exit = %+v, want %+v", exit, tt.exit)
			}
		})
	}
}

func TestEscapeeLeavesTheSession(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc; macOS coverage comes with the worker's descendant tests")
	}
	attemptID := "att_escapee_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	o := collect(t, prepareAttempt(t, "escapee", t.TempDir(), attemptID))
	if o.exit.Code != 0 {
		t.Fatalf("exit = %+v, stderr %q", o.exit, o.stderr)
	}
	pids := markedProcesses(t, "MYTHHELM_ATTEMPT_ID="+attemptID)
	if len(pids) != 1 {
		t.Fatalf("processes carrying the attempt marker = %v, want exactly the escapee", pids)
	}
	defer func() {
		if p, err := os.FindProcess(pids[0]); err == nil {
			_ = p.Kill()
		}
	}()
	if sid := sessionID(t, pids[0]); sid == sessionID(t, os.Getpid()) {
		t.Errorf("escapee session %d equals ours; it should have left with setsid", sid)
	}
}

func markedProcesses(t *testing.T, marker string) []int {
	t.Helper()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Fatal(err)
	}
	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == os.Getpid() {
			continue
		}
		env, err := os.ReadFile(filepath.Join("/proc", e.Name(), "environ"))
		if err == nil && slices.Contains(strings.Split(string(env), "\x00"), marker) {
			pids = append(pids, pid)
		}
	}
	return pids
}

func sessionID(t *testing.T, pid int) int {
	t.Helper()
	stat, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		t.Fatal(err)
	}
	// Fields after the parenthesised command: state ppid pgrp session ...
	fields := strings.Fields(string(stat[bytes.LastIndexByte(stat, ')')+1:]))
	sid, err := strconv.Atoi(fields[3])
	if err != nil {
		t.Fatal(err)
	}
	return sid
}

func TestScenariosEmbedded(t *testing.T) {
	want := []string{"bad-utf8", "check-fails", "deep", "denied", "escapee", "exit-before-result", "happy", "ignore-sigint", "ignore-term", "malformed", "native-fails", "oversized", "slow"}
	if got := Scenarios(); !slices.Equal(got, want) {
		t.Errorf("Scenarios() = %q, want %q", got, want)
	}
	for _, name := range want {
		if _, err := loadScenario(name); err != nil {
			t.Errorf("loadScenario(%s): %v", name, err)
		}
	}
}

func TestLoadScenarioRejectsInvalidSteps(t *testing.T) {
	for _, src := range []string{
		`{"steps":[{"op":"teleport"}]}`,
		`{"steps":[{"op":"write","path":"../escape","content":"x"}]}`,
		`{"steps":[{"op":"write","path":"/abs","content":"x"}]}`,
		`{"steps":[{"op":"emit"}]}`,
		`{"steps":[{"op":"emit","synth":"huge"}]}`,
		`{"steps":[{"op":"ignore_signals","signals":["HUP"]}]}`,
		`{"steps":[{"op":"exit","code":0,"extra":1}]}`,
	} {
		if _, err := parseScenario([]byte(src)); err == nil {
			t.Errorf("parseScenario(%s) succeeded, want an error", src)
		}
	}
}

func TestCapabilityRecordUnknownNotOptimistic(t *testing.T) {
	a := New()
	for _, goos := range []string{"linux", "darwin", "windows", "freebsd"} {
		rec := a.Capabilities(adapter.Probe{OS: goos, Version: "devel"})
		if rec.Billing.Entitlement != "local-scripted" {
			t.Errorf("%s: billing.entitlement = %q, want local-scripted", goos, rec.Billing.Entitlement)
		}
		if rec.Capabilities.HardMonetaryLimit != adapter.Unsupported {
			t.Errorf("%s: hard_monetary_limit = %q, want unsupported", goos, rec.Capabilities.HardMonetaryLimit)
		}
		// Billing and containment safety are never claimed by the fake.
		for name, v := range map[string]adapter.Tri{
			"included_only_supported": rec.Billing.IncludedOnlySupported,
			"paid_overage_prevention": rec.Billing.PaidOveragePrevention,
			"sandbox.status":          rec.Sandbox.Status,
			"worker_detachment":       rec.Platform.WorkerDetachment,
			"process_tree_ownership":  rec.Platform.ProcessTreeOwnership,
		} {
			if v == adapter.Supported {
				t.Errorf("%s: %s = supported; nothing yet proves it", goos, name)
			}
		}
		if rec.Platform.OS != goos {
			t.Errorf("platform.os = %q, want %q", rec.Platform.OS, goos)
		}
		// Every tri-state field holds one of the three values.
		raw, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		var generic map[string]any
		if err := json.Unmarshal(raw, &generic); err != nil {
			t.Fatal(err)
		}
		if ev, ok := generic["billing"].(map[string]any)["evidence_id"]; !ok || ev != nil {
			t.Errorf("billing.evidence_id = %v (present %v), want an explicit null", ev, ok)
		}
		checkTri(t, reflect.ValueOf(rec), "record")
	}
	if got := a.Capabilities(adapter.Probe{OS: "windows"}).Platform.ProcessTreeOwnership; got != adapter.Unsupported {
		t.Errorf("windows process_tree_ownership = %q, want unsupported", got)
	}
}

func checkTri(t *testing.T, v reflect.Value, path string) {
	t.Helper()
	switch {
	case v.Type() == reflect.TypeFor[adapter.Tri]():
		if s := adapter.Tri(v.String()); s != adapter.Supported && s != adapter.Unsupported && s != adapter.Unknown {
			t.Errorf("%s = %q, not a tri-state value", path, s)
		}
	case v.Kind() == reflect.Struct:
		for i := range v.NumField() {
			checkTri(t, v.Field(i), path+"."+v.Type().Field(i).Name)
		}
	}
}

func TestProbeIdentifiesTheRunningBinary(t *testing.T) {
	p, err := New().Probe(context.Background(), adapter.ProbeInput{})
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	exe, _ = filepath.EvalSymlinks(exe)
	if p.Executable != exe || len(p.SHA256) != 64 || p.OS != runtime.GOOS || p.Version == "" {
		t.Errorf("Probe = %+v, want this binary's path, a sha256, the OS and a version", p)
	}
}

// importsOsExec lists the non-test Go files under root that import os/exec,
// relative to root and slash-separated.
func importsOsExec(t *testing.T, fsys fs.FS) (scanned int, offenders []string) {
	t.Helper()
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, src, parser.ImportsOnly)
		if err != nil {
			return err
		}
		scanned++
		for _, imp := range f.Imports {
			if imp.Path.Value == `"os/exec"` && path != "claudecode/probe.go" {
				offenders = append(offenders, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return scanned, offenders
}

func TestAdaptersDoNotImportOsExec(t *testing.T) {
	scanned, offenders := importsOsExec(t, os.DirFS(".."))
	if scanned == 0 {
		t.Fatal("scanned no files under adapters/")
	}
	if len(offenders) > 0 {
		t.Errorf("non-test files importing os/exec: %q; adapters launch through adapter.Launcher", offenders)
	}
}

func TestImportsOsExecDetects(t *testing.T) {
	fsys := fstest.MapFS{
		"x/bad.go":             {Data: []byte("package x\nimport \"os/exec\"\n")},
		"x/aliased.go":         {Data: []byte("package x\nimport run \"os/exec\"\n")},
		"x/ok.go":              {Data: []byte("package x\nimport \"os\"\n")},
		"x/bad_test.go":        {Data: []byte("package x\nimport \"os/exec\"\n")},
		"claudecode/probe.go":  {Data: []byte("package claudecode\nimport \"os/exec\"\n")},
		"claudecode/launch.go": {Data: []byte("package claudecode\nimport (\n\t\"fmt\"\n\t\"os/exec\"\n)\n")},
	}
	_, offenders := importsOsExec(t, fsys)
	slices.Sort(offenders)
	if want := []string{"claudecode/launch.go", "x/aliased.go", "x/bad.go"}; !slices.Equal(offenders, want) {
		t.Errorf("offenders = %q, want %q", offenders, want)
	}
}

func TestAgentMainArgumentErrorsExit2(t *testing.T) {
	for _, args := range [][]string{
		{"--scenario", "nope", "--workdir", t.TempDir()},
		{"--scenario", "happy"},
		{"--no-such-flag"},
	} {
		var stderr bytes.Buffer
		if code := AgentMain(args, strings.NewReader(""), io.Discard, &stderr); code != 2 || stderr.Len() == 0 {
			t.Errorf("AgentMain(%q) = %d with stderr %q, want 2 and a message", args, code, stderr.String())
		}
	}
}
