package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/security"
)

const (
	envAttemptID = "MYTHHELM_ATTEMPT_ID"
	// observationBuffer bounds the observations queued for the consumer.
	observationBuffer = 64
	// acknowledgedUnknown is the Interrupt report value: no native interrupt
	// acknowledgement is observable until OQ9 is answered (D8).
	acknowledgedUnknown = "unknown"
)

// fidelityAspects are the behaviours MYTHHELM cannot observe in the native
// without a direct-native comparison (D16).
var fidelityAspects = []string{"instructions", "skills", "hooks", "mcp", "children", "context"}

// stopLadder is interrupt, terminate, kill with graces 5s, 10s, 5s (D8).
func stopLadder() []adapter.StopStep {
	return []adapter.StopStep{
		{Signal: adapter.StopInterrupt, Grace: 5 * time.Second},
		{Signal: adapter.StopTerminate, Grace: 10 * time.Second},
		{Signal: adapter.StopKill, Grace: 5 * time.Second},
	}
}

// Argv is the native invocation. The app-server subcommand is a placeholder
// fixed by the answer to OQ1 (D5); the prompt is delivered on stdin, never
// here.
func Argv(bin string) []string { return []string{bin, "app-server"} }

// Prepare resolves the pinned launch: argv, allowlisted environment, stdin
// prompt and stop ladder. It refuses what Codex cannot honour (tool rules,
// passthrough) and a binary swapped since the probe. Billing and the
// manifest stay zero for admission to fill.
func (a *codexAdapter) Prepare(_ context.Context, in adapter.PrepareInput) (adapter.LaunchProposal, error) {
	goos := a.goos
	if goos == "" {
		goos = runtime.GOOS
	}
	if goos == "windows" {
		return adapter.LaunchProposal{}, fmt.Errorf("%w: process_tree_ownership unsupported on Windows", ErrCapability)
	}
	if !filepath.IsAbs(in.Workdir) {
		return adapter.LaunchProposal{}, fmt.Errorf("codex: workdir %q is not an absolute path", in.Workdir)
	}
	if in.AttemptID == "" || strings.ContainsRune(in.AttemptID, 0) {
		return adapter.LaunchProposal{}, errors.New("codex: attempt ID is required")
	}
	if !filepath.IsAbs(in.Probe.Executable) {
		return adapter.LaunchProposal{}, errors.New("codex: probed executable is required")
	}
	if in.Prompt == nil {
		return adapter.LaunchProposal{}, errors.New("codex: prompt is required")
	}
	if len(in.AllowedTools) > 0 {
		return adapter.LaunchProposal{}, &adapter.BlockedError{Code: "native_tool_rules_unsupported", Field: "allowed_tools", Action: "remove the project's tool rules; Codex has no equivalent"}
	}
	if len(in.Passthrough) > 0 {
		return adapter.LaunchProposal{}, &adapter.BlockedError{Code: "native_passthrough_unsupported", Field: "passthrough", Action: "remove the project's environment passthrough; Codex credential variables are not yet screened"}
	}
	digest, err := fileSHA256(in.Probe.Executable)
	if err != nil || digest != in.Probe.SHA256 {
		return adapter.LaunchProposal{}, &adapter.BlockedError{Code: "native_binary_changed", Field: "executable", Action: "run the probe again"}
	}
	env, err := security.BuildEnv(in.Env, nil, map[string]string{envAttemptID: in.AttemptID})
	if err != nil {
		return adapter.LaunchProposal{}, err
	}
	overrides := []adapter.ConfigDelta{{Kind: "env", Name: envAttemptID, Value: in.AttemptID, Reason: "orphan-scan marker"}}
	for _, aspect := range fidelityAspects {
		overrides = append(overrides, adapter.ConfigDelta{Kind: "fidelity", Name: aspect, Value: "unobservable", Reason: "no direct-native comparison has been made"})
	}
	return adapter.LaunchProposal{
		Spec:         adapter.ProcSpec{Path: in.Probe.Executable, Args: Argv(in.Probe.Executable)[1:], Dir: in.Workdir, Env: env, Stdin: in.Prompt},
		StopLadder:   stopLadder(),
		Overrides:    overrides,
		Capabilities: a.Capabilities(in.Probe),
	}, nil
}

// Start launches the proposal once through l. A second Start on the same
// adapter value is refused before Launch: the worker-owned stdin pipe is the
// only writer (D10).
func (a *codexAdapter) Start(ctx context.Context, lp adapter.LaunchProposal, l adapter.Launcher) (adapter.Session, error) {
	if !a.started.CompareAndSwap(false, true) {
		return nil, &adapter.BlockedError{Code: "input_writer_conflict", Action: "build a fresh adapter for a new attempt"}
	}
	proc, err := l.Launch(ctx, lp.Spec)
	if err != nil {
		return nil, err
	}
	s := &launchSession{
		proc:   proc,
		ladder: lp.StopLadder,
		obs:    make(chan adapter.Observation, observationBuffer),
		done:   make(chan adapter.NativeExit, 1),
	}
	go s.run()
	return s, nil
}

type launchSession struct {
	proc    adapter.OwnedProc
	ladder  []adapter.StopStep
	obs     chan adapter.Observation
	done    chan adapter.NativeExit
	dropped int64 // Progress observations dropped because the consumer lagged
}

func (s *launchSession) Observations() <-chan adapter.Observation { return s.obs }
func (s *launchSession) Done() <-chan adapter.NativeExit          { return s.done }

// run decodes stdout until it ends, then reaps the process. Decode can stop
// before EOF, so stdout is drained: a native blocked on a full pipe could
// otherwise never exit for Wait to return.
func (s *launchSession) run() {
	stdout := s.proc.Stdout()
	decoded := Decode(stdout, s.send)
	close(s.obs)
	_, _ = io.Copy(io.Discard, stdout)
	exit := s.proc.Wait()
	if exit.Err == nil {
		exit.Err = decoded.Err
	}
	s.done <- exit
	close(s.done)
}

// send queues ob. Only Progress may be dropped when the queue is full.
func (s *launchSession) send(ob adapter.Observation) {
	switch o := ob.(type) {
	case adapter.Progress:
		select {
		case s.obs <- ob:
		default:
			s.dropped++
		}
		return
	case adapter.ProtocolCounters:
		o.ProgressDropped = s.dropped
		ob = o
	}
	s.obs <- ob
}

// Interrupt climbs the stop ladder. The native's acknowledgement is not
// observable, so the report says unknown (D8).
func (s *launchSession) Interrupt(ctx context.Context) adapter.InterruptReport {
	rep := adapter.ClimbLadder(ctx, s.proc, s.ladder)
	rep.Acknowledged = acknowledgedUnknown
	return rep
}
