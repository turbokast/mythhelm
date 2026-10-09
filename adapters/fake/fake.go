// Package fake is a scripted adapter for tests and the offline demo. Its
// agent is mythhelm itself, run as a real child process (__fake-agent), so
// the worker, ownership and stop paths are the production ones (AC-11.2).
// It makes no inference and no network request.
//
// This package is not a stable API: adapters are internal until the plugin
// protocol exists.
package fake

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/adapter/ndjson"
	"github.com/turbokast/mythhelm/internal/buildinfo"
)

const (
	adapterID      = "builtin/fake"
	adapterVersion = "0.1.0"
	attemptEnvVar  = "MYTHHELM_ATTEMPT_ID"

	// observationBuffer bounds the observations queued for the consumer.
	observationBuffer = 64
	// maxUnknownTypes bounds the distinct unknown frame types counted by name.
	maxUnknownTypes = 32
	// maxToolName is the longest tool name reported as is.
	maxToolName = 64
)

// ErrUnknownScenario is returned by Prepare for a scenario that is not
// embedded.
var ErrUnknownScenario = errors.New("unknown fake scenario")

type fakeAdapter struct{}

// New returns the fake adapter.
func New() adapter.Adapter { return fakeAdapter{} }

func (fakeAdapter) Descriptor() adapter.Descriptor {
	return adapter.Descriptor{ID: adapterID, Version: adapterVersion, Harness: "fake", Surface: "scripted-child-process (ndjson)"}
}

// Probe identifies the running mythhelm binary, which is the fake's agent.
func (fakeAdapter) Probe(_ context.Context, _ adapter.ProbeInput) (adapter.Probe, error) {
	exe, err := executable()
	if err != nil {
		return adapter.Probe{}, err
	}
	sum, err := fileSHA256(exe)
	if err != nil {
		return adapter.Probe{}, err
	}
	return adapter.Probe{
		Executable:    exe,
		Version:       buildinfo.Get().Version,
		SHA256:        sum,
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		Compatibility: "scripted; no native harness",
	}, nil
}

// Capabilities never claims billing or containment safety. Worker
// detachment and process-group ownership are supported only where the
// worker's platform tests establish them, Linux and macOS (internal/workers,
// ADR 0004); they stay unknown elsewhere (I14), and Windows has no group
// ownership.
func (fakeAdapter) Capabilities(p adapter.Probe) adapter.CapabilityRecord {
	platform := adapter.Platform{OS: p.OS, WorkerDetachment: adapter.Unknown, ProcessTreeOwnership: adapter.Unknown}
	switch p.OS {
	case "linux", "darwin":
		platform.WorkerDetachment, platform.ProcessTreeOwnership = adapter.Supported, adapter.Supported
	case "windows":
		platform.ProcessTreeOwnership = adapter.Unsupported
	}
	return adapter.CapabilityRecord{
		SchemaVersion:         1,
		AdapterID:             adapterID,
		AdapterVersion:        adapterVersion,
		RuntimeVersion:        p.Version,
		Mode:                  "headless",
		ExecutionSurface:      "scripted-child-process",
		HarnessID:             "fake",
		FidelityQualification: "not-applicable (no native harness)",
		Billing: adapter.BillingCapabilities{
			Entitlement:           "local-scripted",
			IncludedOnlySupported: adapter.Unsupported,
			PaidOveragePrevention: adapter.Unsupported,
		},
		HostIntegration: adapter.HostIntegration{
			HerdrEmbedded:           adapter.Unsupported,
			HerdrStateBridge:        adapter.Unsupported,
			NativeInteractiveAttach: adapter.Unsupported,
		},
		Capabilities: adapter.Capabilities{
			StructuredEvents:  adapter.Supported,
			Resume:            adapter.Unsupported,
			LiveSteer:         adapter.Unsupported,
			ApprovalBridge:    adapter.Unsupported,
			UsageTokens:       adapter.Unsupported,
			QuotaRemaining:    adapter.Unsupported,
			HardMonetaryLimit: adapter.Unsupported,
			NativeSubagents:   adapter.Unsupported,
		},
		Sandbox:       adapter.Sandbox{Status: adapter.Unsupported, Scope: "not-established"},
		Platform:      platform,
		Qualification: "scripted test adapter: no real agent, no inference, no network",
	}
}

func (f fakeAdapter) Prepare(_ context.Context, in adapter.PrepareInput) (adapter.LaunchProposal, error) {
	switch {
	case !filepath.IsAbs(in.Workdir):
		return adapter.LaunchProposal{}, fmt.Errorf("fake adapter: workdir %q is not an absolute path", in.Workdir)
	case in.AttemptID == "":
		return adapter.LaunchProposal{}, errors.New("fake adapter: attempt ID is required")
	}
	if _, err := loadScenario(in.Scenario); err != nil {
		return adapter.LaunchProposal{}, err
	}
	exe, err := executable()
	if err != nil {
		return adapter.LaunchProposal{}, err
	}
	env := make([]string, 0, len(in.Env)+1)
	for _, kv := range in.Env {
		if !strings.HasPrefix(kv, attemptEnvVar+"=") {
			env = append(env, kv)
		}
	}
	env = append(env, attemptEnvVar+"="+in.AttemptID)
	return adapter.LaunchProposal{
		Spec: adapter.ProcSpec{
			Path:  exe,
			Args:  []string{AgentCommand, "--scenario", in.Scenario, "--workdir", in.Workdir},
			Dir:   in.Workdir,
			Env:   env,
			Stdin: in.Prompt,
		},
		StopLadder: stopLadder(runtime.GOOS),
		Overrides:  []adapter.ConfigDelta{{Kind: "env", Name: attemptEnvVar, Value: in.AttemptID, Reason: "orphan-scan marker"}},
		Billing: adapter.BillingPosture{
			Mode:                            "local-scripted",
			CredentialProvenance:            "none",
			EntitlementClass:                "none (no inference, no network)",
			EntitlementSource:               "not-applicable",
			PaidContinuation:                "not-applicable",
			PaidContinuationUserDeclaration: "not-applicable",
			G05:                             "not-applicable",
		},
		Manifest:     adapter.ConfigManifest{Digests: map[string]string{}},
		Capabilities: f.Capabilities(adapter.Probe{OS: runtime.GOOS, Version: buildinfo.Get().Version}),
	}, nil
}

// stopLadder climbs SIGINT, SIGTERM, SIGKILL on Unix so every rung is
// exercised. On Windows the fake is a single process stopped by Kill.
func stopLadder(goos string) []adapter.StopStep {
	if goos == "windows" {
		return []adapter.StopStep{{Signal: adapter.StopKill, Grace: 5 * time.Second}}
	}
	return []adapter.StopStep{
		{Signal: adapter.StopInterrupt, Grace: 3 * time.Second},
		{Signal: adapter.StopTerminate, Grace: 3 * time.Second},
		{Signal: adapter.StopKill, Grace: 5 * time.Second},
	}
}

func (fakeAdapter) Start(ctx context.Context, lp adapter.LaunchProposal, l adapter.Launcher) (adapter.Session, error) {
	proc, err := l.Launch(ctx, lp.Spec)
	if err != nil {
		return nil, err
	}
	s := &session{
		proc:   proc,
		ladder: lp.StopLadder,
		obs:    make(chan adapter.Observation, observationBuffer),
		done:   make(chan adapter.NativeExit, 1),
	}
	go s.run()
	return s, nil
}

func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("fake adapter: locating mythhelm: %w", err)
	}
	return filepath.EvalSymlinks(exe)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // G304: path is this binary, from os.Executable
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type session struct {
	proc    adapter.OwnedProc
	ladder  []adapter.StopStep
	obs     chan adapter.Observation
	done    chan adapter.NativeExit
	dropped int64 // Progress observations dropped because the consumer lagged
}

func (s *session) Observations() <-chan adapter.Observation { return s.obs }
func (s *session) Done() <-chan adapter.NativeExit          { return s.done }

// run decodes stdout until it ends, then reaps the process.
func (s *session) run() {
	rd := ndjson.NewReader(s.proc.Stdout(), ndjson.Limits{})
	dec := newDecoder()
	var streamErr error
	for {
		frame, err := rd.Next()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				streamErr = err
			}
			break
		}
		if ob := dec.decode(frame); ob != nil {
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
func (s *session) send(ob adapter.Observation) {
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
func (s *session) Interrupt(ctx context.Context) adapter.InterruptReport {
	return adapter.ClimbLadder(ctx, s.proc, s.ladder)
}

// Frame types written by the fake agent.
const (
	frameInit     = "fake.init"
	frameProgress = "fake.progress"
	frameDenied   = "fake.denied"
	frameResult   = "fake.result"
	frameError    = "fake.error"
)

type initFrame struct {
	SessionID      string `json:"session_id"`
	Model          string `json:"model"`
	NativeVersion  string `json:"native_version"`
	PermissionMode string `json:"permission_mode"`
	APIKeySource   string `json:"api_key_source"`
}

type progressFrame struct {
	Turn int    `json:"turn"`
	Tool string `json:"tool"`
}

type deniedFrame struct {
	ToolName string `json:"tool_name"`
}

type errorFrame struct {
	Class string `json:"class"`
}

type resultFrame struct {
	Subtype           string   `json:"subtype"`
	IsError           bool     `json:"is_error"`
	NumTurns          *int     `json:"num_turns"`
	DurationMS        *int64   `json:"duration_ms"`
	StopReason        string   `json:"stop_reason"`
	PermissionDenials []string `json:"permission_denials"`
}

// decoder turns fake frames into observations, counting what it cannot use.
type decoder struct {
	malformed int64
	unknown   map[string]int64
	named     int // distinct unknown types counted under their own name
}

func newDecoder() *decoder { return &decoder{unknown: map[string]int64{}} }

// decode returns the frame's observation, or nil when the frame is only
// counted.
func (d *decoder) decode(frame []byte) adapter.Observation {
	var head struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(frame, &head) != nil {
		d.malformed++
		return nil
	}
	switch head.Type {
	case frameInit:
		var f initFrame
		if json.Unmarshal(frame, &f) != nil || f.SessionID == "" || f.APIKeySource == "" {
			break
		}
		return adapter.SessionStarted{SessionID: f.SessionID, Model: f.Model, NativeVersion: f.NativeVersion, PermissionMode: f.PermissionMode, APIKeySource: f.APIKeySource}
	case frameProgress:
		var f progressFrame
		if json.Unmarshal(frame, &f) != nil {
			break
		}
		return adapter.Progress{Turn: f.Turn, Tool: toolName(f.Tool)}
	case frameDenied:
		var f deniedFrame
		if json.Unmarshal(frame, &f) != nil || f.ToolName == "" {
			break
		}
		return adapter.PermissionDenied{ToolName: toolName(f.ToolName)}
	case frameError:
		// Only the classes the scripted scenarios stage are known; any other
		// name is reported as an unrecognised native error.
		var f errorFrame
		if json.Unmarshal(frame, &f) != nil {
			break
		}
		if f.Class != "allowance_exhausted" && f.Class != "rate_limit" {
			f.Class = "native_error"
		}
		return adapter.NativeError{Class: f.Class}
	case frameResult:
		var f resultFrame
		if json.Unmarshal(frame, &f) != nil || f.Subtype == "" {
			break
		}
		denials := make([]string, len(f.PermissionDenials))
		for i, name := range f.PermissionDenials {
			denials[i] = toolName(name)
		}
		return adapter.Result{Subtype: f.Subtype, IsError: f.IsError, NumTurns: f.NumTurns, DurationMS: f.DurationMS, StopReason: f.StopReason, PermissionDenials: denials}
	default:
		d.countUnknown(head.Type)
		return nil
	}
	d.malformed++
	return nil
}

// countUnknown counts an unknown frame type under a bounded set of names.
func (d *decoder) countUnknown(typ string) {
	key := "vendor.fake." + typ
	switch {
	case !printable(typ, maxToolName):
		key = "vendor.fake.invalid"
	case d.unknown[key] > 0:
	case d.named >= maxUnknownTypes:
		key = "vendor.fake.other"
	default:
		d.named++
	}
	d.unknown[key]++
}

// toolName returns name, or "invalid" when it is too long or not printable.
func toolName(name string) string {
	if name != "" && !printable(name, maxToolName) {
		return "invalid"
	}
	return name
}

func printable(s string, maxLen int) bool {
	if s == "" || len(s) > maxLen {
		return false
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}
