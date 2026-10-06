package claudecode

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
	"github.com/turbokast/mythhelm/internal/adapter/ndjson"
	"github.com/turbokast/mythhelm/internal/security"
)

// Child variables MYTHHELM sets on every native launch (design §6.3). Each
// is recorded as a configuration delta on the launch proposal.
const (
	envAttemptID       = "MYTHHELM_ATTEMPT_ID"
	envNoAutoUpdate    = "DISABLE_AUTOUPDATER"
	envStartupFailures = "CLAUDE_CODE_STARTUP_FAILURE_RESULTS"
)

const (
	// observationBuffer bounds the observations queued for the consumer.
	observationBuffer = 64
	// MaxAllowedToolRule bounds one trusted allowed-tools rule. Admission
	// validates rules against it too, so a bad rule fails at the config
	// gate (exit 2) rather than at launch.
	MaxAllowedToolRule = 1024
)

// stopLadder is SIGTERM, then SIGKILL (AC-5.6). TERM gives the native a
// bounded window to flush its transcript; KILL bounds the stop itself.
func stopLadder() []adapter.StopStep {
	return []adapter.StopStep{
		{Signal: adapter.StopTerminate, Grace: 10 * time.Second},
		{Signal: adapter.StopKill, Grace: 5 * time.Second},
	}
}

// Argv is the exact native invocation (design §6.3): print mode with
// stream-json, stdin prompt, edits accepted inside the working directory,
// and anything that would prompt denied rather than waited on. Allowed
// tool rules come last, only when the trusted project config lists them.
// The prompt never appears here; it is delivered on stdin (AC-5.3).
func Argv(bin string, allowedTools []string) []string {
	argv := []string{bin, "-p",
		"--output-format", "stream-json",
		"--verbose", // required with stream-json in print mode
		"--input-format", "text",
		"--permission-mode", "acceptEdits",
		"--permission-prompts", "none"}
	if len(allowedTools) > 0 {
		argv = append(argv, "--allowedTools")
		argv = append(argv, allowedTools...)
	}
	return argv
}

// Prepare resolves the pinned launch: the exact argv, the allowlisted child
// environment plus the three MYTHHELM variables, the stdin prompt and the
// stop ladder. It re-checks the executable hash, so a binary swapped
// between probe and launch fails closed. Billing and the config manifest
// stay zero here: admission resolves them (auth evidence, declaration,
// native inventory) and records them on the proposal it journals.
func (a *claudeAdapter) Prepare(_ context.Context, in adapter.PrepareInput) (adapter.LaunchProposal, error) {
	if runtime.GOOS == "windows" {
		return adapter.LaunchProposal{}, fmt.Errorf("%w: process_tree_ownership unsupported on Windows", ErrCapability)
	}
	if !filepath.IsAbs(in.Workdir) {
		return adapter.LaunchProposal{}, fmt.Errorf("claudecode: workdir %q is not an absolute path", in.Workdir)
	}
	if in.AttemptID == "" || strings.ContainsRune(in.AttemptID, 0) {
		return adapter.LaunchProposal{}, errors.New("claudecode: attempt ID is required")
	}
	if !filepath.IsAbs(in.Probe.Executable) {
		return adapter.LaunchProposal{}, errors.New("claudecode: probed executable is required")
	}
	if in.Prompt == nil {
		return adapter.LaunchProposal{}, errors.New("claudecode: prompt is required")
	}
	for _, rule := range in.AllowedTools {
		// Dash-prefixed rules are refused: past --allowedTools the
		// native would parse them as its own options, so a rule must
		// never look like one.
		if rule == "" || len(rule) > MaxAllowedToolRule || strings.ContainsRune(rule, 0) || strings.HasPrefix(rule, "-") {
			return adapter.LaunchProposal{}, fmt.Errorf("claudecode: invalid allowed-tools rule %q", rule)
		}
	}
	digest, err := fileSHA256(in.Probe.Executable)
	if err != nil || digest != in.Probe.SHA256 {
		return adapter.LaunchProposal{}, fmt.Errorf("%w: native_executable_changed", ErrCapability)
	}
	env, err := childEnv(in.Env, in.Passthrough, in.AttemptID)
	if err != nil {
		return adapter.LaunchProposal{}, err
	}
	argv := Argv(in.Probe.Executable, in.AllowedTools)
	return adapter.LaunchProposal{
		Spec: adapter.ProcSpec{
			Path:  in.Probe.Executable,
			Args:  argv[1:],
			Dir:   in.Workdir,
			Env:   env,
			Stdin: in.Prompt,
		},
		StopLadder: stopLadder(),
		Overrides: []adapter.ConfigDelta{
			{Kind: "env", Name: envNoAutoUpdate, Value: "1", Reason: "keep the probed native version pinned for the attempt"},
			{Kind: "env", Name: envStartupFailures, Value: "1", Reason: "startup failures arrive as structured results"},
			{Kind: "env", Name: envAttemptID, Value: in.AttemptID, Reason: "orphan-scan marker"},
		},
		Capabilities: a.Capabilities(in.Probe),
	}, nil
}

// childEnv rebuilds the child environment from the admission-built base: the
// platform allowlist plus the trusted passthrough names, overlaid with the
// three MYTHHELM variables. The credential denylist always wins, so a
// caller that smuggles a route variable into the base cannot reach the
// native through Prepare. An opted-in CLAUDE_CODE_OAUTH_TOKEN entry is
// preserved opaquely: the opt-in decision is admission's (AC-4.7), and
// Prepare never adds the entry itself.
func childEnv(base, passthrough []string, attemptID string) ([]string, error) {
	var oauth string
	filtered := make([]string, 0, len(base))
	for _, kv := range base {
		name := kv
		if before, _, ok := strings.Cut(kv, "="); ok {
			name = before
		}
		switch name {
		case envAttemptID, envNoAutoUpdate, envStartupFailures:
			continue
		case "CLAUDE_CODE_OAUTH_TOKEN":
			oauth = kv
			continue
		}
		filtered = append(filtered, kv)
	}
	env, err := security.BuildEnv(filtered, passthrough, map[string]string{
		envNoAutoUpdate:    "1",
		envStartupFailures: "1",
		envAttemptID:       attemptID,
	})
	if err != nil {
		return nil, err
	}
	if oauth != "" {
		env = append(env, oauth)
	}
	return env, nil
}

// Start launches the proposal once through l. Only the worker calls it,
// and l owns the process; the adapter never creates processes itself.
func (*claudeAdapter) Start(ctx context.Context, lp adapter.LaunchProposal, l adapter.Launcher) (adapter.Session, error) {
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

// run decodes stdout until it ends, then reaps the process.
func (s *launchSession) run() {
	rd := ndjson.NewReader(s.proc.Stdout(), ndjson.Limits{})
	dec := newStreamDecoder()
	var streamErr error
	for {
		frame, err := rd.Next()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				streamErr = err
			}
			break
		}
		for _, ob := range dec.decode(frame) {
			s.send(ob)
		}
	}
	s.obs <- adapter.ProtocolCounters{Counters: rd.Counters(), Malformed: dec.malformed, UnknownTypes: dec.unknown, ProgressDropped: s.dropped}
	close(s.obs)
	exit := s.proc.Wait()
	if exit.Err == nil && streamErr != nil {
		exit.Err = fmt.Errorf("reading native stdout: %w", streamErr)
	}
	s.done <- exit
	close(s.done)
}

// send queues ob. Only Progress may be dropped when the queue is full.
func (s *launchSession) send(ob adapter.Observation) {
	if _, ok := ob.(adapter.Progress); ok {
		select {
		case s.obs <- ob:
		default:
			s.dropped++
		}
		return
	}
	s.obs <- ob
}

// Interrupt climbs the stop ladder until the process group is confirmed
// gone, the ladder ends or ctx is done.
func (s *launchSession) Interrupt(ctx context.Context) adapter.InterruptReport {
	return adapter.ClimbLadder(ctx, s.proc, s.ladder)
}
