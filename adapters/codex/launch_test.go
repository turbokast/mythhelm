package codex

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/turbokast/mythhelm/internal/adapter"
)

// writeNative writes a stand-in executable and returns the probe that pins it.
func writeNative(t *testing.T, content string) adapter.Probe {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil { // #nosec G306 -- Test executable must be runnable.
		t.Fatal(err)
	}
	sum, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	return adapter.Probe{Executable: path, Version: fixtureVersion, SHA256: sum, OS: "linux", Arch: "amd64"}
}

func prepareInput(t *testing.T) adapter.PrepareInput {
	t.Helper()
	return adapter.PrepareInput{
		Workdir:   t.TempDir(),
		AttemptID: "att_codex",
		Env:       []string{"PATH=/usr/bin", "HOME=/home/test"},
		Prompt:    strings.NewReader("task\n"),
		Probe:     writeNative(t, "#!/bin/sh\n"),
	}
}

func blockedCode(t *testing.T, err error) string {
	t.Helper()
	be, ok := errors.AsType[*adapter.BlockedError](err)
	if !ok {
		t.Fatalf("err = %v, want *adapter.BlockedError", err)
	}
	return be.Code
}

func TestPrepareRefusesChangedHash(t *testing.T) { // AC-2.4
	t.Parallel()
	in := prepareInput(t)
	if err := os.WriteFile(in.Probe.Executable, []byte("#!/bin/sh\nswapped\n"), 0o700); err != nil { // #nosec G306 -- Test executable must be runnable.
		t.Fatal(err)
	}
	_, err := New().Prepare(context.Background(), in)
	if got := blockedCode(t, err); got != "native_binary_changed" {
		t.Fatalf("code = %q, want native_binary_changed", got)
	}
}

func TestPrepareRefusesToolRulesAndPassthrough(t *testing.T) { // AC-1.4, D15
	t.Parallel()
	in := prepareInput(t)
	in.AllowedTools = []string{"Bash(go build *)"}
	_, err := New().Prepare(context.Background(), in)
	if got := blockedCode(t, err); got != "native_tool_rules_unsupported" {
		t.Fatalf("tool rules code = %q", got)
	}
	in = prepareInput(t)
	in.Passthrough = []string{"GOPATH"}
	_, err = New().Prepare(context.Background(), in)
	if got := blockedCode(t, err); got != "native_passthrough_unsupported" {
		t.Fatalf("passthrough code = %q", got)
	}

	in = prepareInput(t)
	lp, err := New().Prepare(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if want := Argv(in.Probe.Executable)[1:]; !slices.Equal(lp.Spec.Args, want) {
		t.Fatalf("args = %q, want golden %q", lp.Spec.Args, want)
	}
	if lp.Spec.Path != in.Probe.Executable || lp.Spec.Dir != in.Workdir || lp.Spec.Stdin != in.Prompt {
		t.Fatalf("spec = %+v, want the probed path, workdir and prompt reader", lp.Spec)
	}
	wantEnv := []string{"HOME=/home/test", "MYTHHELM_ATTEMPT_ID=att_codex", "PATH=/usr/bin"}
	if got := slices.Sorted(slices.Values(lp.Spec.Env)); !slices.Equal(got, wantEnv) {
		t.Fatalf("env = %q, want golden %q", got, wantEnv)
	}
}

func TestArgvGolden(t *testing.T) { // D5: placeholder constant until OQ1
	t.Parallel()
	if got, want := Argv("/x/codex"), []string{"/x/codex", "app-server"}; !slices.Equal(got, want) {
		t.Fatalf("argv = %q, want %q", got, want)
	}
}

func TestPrepareRefusesWindows(t *testing.T) { // N6
	t.Parallel()
	_, err := (&codexAdapter{goos: "windows"}).Prepare(context.Background(), prepareInput(t))
	if !errors.Is(err, ErrCapability) {
		t.Fatalf("windows err = %v, want ErrCapability", err)
	}
	if _, err := (&codexAdapter{goos: "linux"}).Prepare(context.Background(), prepareInput(t)); err != nil {
		t.Fatalf("linux err = %v, want nil", err)
	}
}

func TestPrepareRecordsFidelityDeltas(t *testing.T) { // AC-3.2, D16
	t.Parallel()
	lp, err := New().Prepare(context.Background(), prepareInput(t))
	if err != nil {
		t.Fatal(err)
	}
	var aspects []string
	for _, d := range lp.Overrides {
		if d.Kind != "fidelity" {
			continue
		}
		if d.Value != "unobservable" || d.Reason == "" {
			t.Fatalf("fidelity delta %+v, want value unobservable with a reason", d)
		}
		aspects = append(aspects, d.Name)
	}
	slices.Sort(aspects)
	want := []string{"children", "context", "hooks", "instructions", "mcp", "skills"}
	if !slices.Equal(aspects, want) {
		t.Fatalf("fidelity aspects = %q, want %q", aspects, want)
	}
}

func TestPrepareEnvAllowlistDropsCanary(t *testing.T) { // NFR-1
	t.Parallel()
	in := prepareInput(t)
	in.Env = append(in.Env, "OPENAI_API_KEY=canary-secret", "CODEX_ACCESS_TOKEN=canary-secret", "MYTHHELM_ATTEMPT_ID=forged")
	lp, err := New().Prepare(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(lp.Spec.Env, "\n")
	if strings.Contains(joined, "canary-secret") {
		t.Fatalf("env leaks the canary: %q", lp.Spec.Env)
	}
	if !slices.Contains(lp.Spec.Env, "MYTHHELM_ATTEMPT_ID=att_codex") || strings.Contains(joined, "forged") {
		t.Fatalf("env = %q, want the attempt id set by Prepare", lp.Spec.Env)
	}
}

func TestStopLadderValues(t *testing.T) { // D8
	t.Parallel()
	lp, err := New().Prepare(context.Background(), prepareInput(t))
	if err != nil {
		t.Fatal(err)
	}
	want := []adapter.StopStep{
		{Signal: adapter.StopInterrupt, Grace: 5_000_000_000},
		{Signal: adapter.StopTerminate, Grace: 10_000_000_000},
		{Signal: adapter.StopKill, Grace: 5_000_000_000},
	}
	if !reflect.DeepEqual(lp.StopLadder, want) {
		t.Fatalf("ladder = %+v, want %+v", lp.StopLadder, want)
	}
}

// countingLauncher counts Launch calls and serves a fixed stdout.
type countingLauncher struct {
	launches atomic.Int32
	stdout   string
}

func (l *countingLauncher) Launch(context.Context, adapter.ProcSpec) (adapter.OwnedProc, error) {
	l.launches.Add(1)
	return &memProc{stdout: strings.NewReader(l.stdout)}, nil
}

type memProc struct{ stdout io.Reader }

func (p *memProc) Stdout() io.Reader             { return p.stdout }
func (*memProc) Signal(adapter.StopSignal) error { return nil }
func (*memProc) Wait() adapter.NativeExit        { return adapter.NativeExit{} }
func (*memProc) GroupGone() bool                 { return true }

func drain(t *testing.T, s adapter.Session) ([]adapter.Observation, adapter.NativeExit) {
	t.Helper()
	var obs []adapter.Observation
	for ob := range s.Observations() {
		obs = append(obs, ob)
	}
	return obs, <-s.Done()
}

func TestSecondStartOnSameAdapterRefused(t *testing.T) { // AC-5.3, D10
	t.Parallel()
	a := New()
	lp, err := a.Prepare(context.Background(), prepareInput(t))
	if err != nil {
		t.Fatal(err)
	}
	l := &countingLauncher{stdout: fixture(t, "success")}
	s, err := a.Start(context.Background(), lp, l)
	if err != nil {
		t.Fatal(err)
	}
	drain(t, s)
	_, err = a.Start(context.Background(), lp, l)
	if got := blockedCode(t, err); got != "input_writer_conflict" {
		t.Fatalf("second Start code = %q, want input_writer_conflict", got)
	}
	if n := l.launches.Load(); n != 1 {
		t.Fatalf("Launch calls = %d, want 1", n)
	}

	var names []string
	for m := range reflect.TypeOf(s).Methods() {
		names = append(names, m.Name)
	}
	if want := []string{"Done", "Interrupt", "Observations"}; !slices.Equal(names, want) {
		t.Fatalf("Session methods = %q, want %q: no write method", names, want)
	}
}

func TestAdapterHasNoReplayPath(t *testing.T) { // AC-5.4, AC-6.2
	t.Parallel()
	var got, want []string
	for m := range reflect.TypeOf(New()).Methods() {
		got = append(got, m.Name)
	}
	for m := range reflect.TypeFor[adapter.Adapter]().Methods() {
		want = append(want, m.Name)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("exported methods = %q, want exactly the adapter.Adapter set %q", got, want)
	}
}

func TestStartLaunchesOnlyThroughLauncher(t *testing.T) {
	t.Parallel()
	for _, file := range []string{"launch.go", "decode.go"} {
		f, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			if imp.Path.Value == `"os/exec"` {
				t.Errorf("%s imports os/exec", file)
			}
		}
	}
	a := New()
	lp, err := a.Prepare(context.Background(), prepareInput(t))
	if err != nil {
		t.Fatal(err)
	}
	l := &countingLauncher{stdout: fixture(t, "success")}
	s, err := a.Start(context.Background(), lp, l)
	if err != nil {
		t.Fatal(err)
	}
	obs, exit := drain(t, s)
	if n := l.launches.Load(); n != 1 {
		t.Fatalf("Launch calls = %d, want 1", n)
	}
	if exit.Err != nil || exit.Code != 0 {
		t.Fatalf("exit = %+v, want clean", exit)
	}
	if _, ok := obs[0].(adapter.SessionStarted); !ok {
		t.Fatalf("first observation = %T, want SessionStarted", obs[0])
	}
	if _, ok := obs[len(obs)-1].(adapter.ProtocolCounters); !ok {
		t.Fatalf("last observation = %T, want ProtocolCounters", obs[len(obs)-1])
	}
}

func TestStartMergesStreamErrorIntoExit(t *testing.T) {
	t.Parallel()
	a := New()
	lp, err := a.Prepare(context.Background(), prepareInput(t))
	if err != nil {
		t.Fatal(err)
	}
	s, err := a.Start(context.Background(), lp, &countingLauncher{stdout: fixture(t, "truncated")})
	if err != nil {
		t.Fatal(err)
	}
	_, exit := drain(t, s)
	if exit.Err == nil {
		t.Fatal("exit.Err = nil, want the incomplete-stream error from Decode")
	}
}
