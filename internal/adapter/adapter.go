// Package adapter defines the internal seam between the supervisor and a
// native harness (§9.1). It lives under internal/ so that no code outside
// this module can register an adapter (N5, I11).
package adapter

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter/ndjson"
)

// Adapter translates supervisory intents into native operations and native
// output into typed observations. It never simulates a capability it lacks.
type Adapter interface {
	// Descriptor identifies the adapter.
	Descriptor() Descriptor
	// Probe resolves the native executable's identity without launching a
	// paid task, in bounded time.
	Probe(ctx context.Context, in ProbeInput) (Probe, error)
	// Capabilities returns the §9.2 record for a probed installation.
	// A value that has not been established stays "unknown".
	Capabilities(p Probe) CapabilityRecord
	// Prepare resolves the invocation, environment, stop ladder and billing
	// posture. A policy block is returned as a *BlockedError.
	Prepare(ctx context.Context, in PrepareInput) (LaunchProposal, error)
	// Start launches the proposal once through l. Only the worker calls it,
	// and l owns the process; adapters never create processes themselves.
	Start(ctx context.Context, lp LaunchProposal, l Launcher) (Session, error)
}

// Descriptor identifies an adapter.
type Descriptor struct {
	ID      string `json:"id"`      // e.g. "builtin/fake"
	Version string `json:"version"` // adapter version, not the native one
	Harness string `json:"harness"` // e.g. "claude-code"
	Surface string `json:"surface"` // e.g. "native-cli-structured (print, stream-json)"
}

// ProbeInput carries what probing needs from the caller. The fake adapter
// needs nothing; native adapters add fields as they need them.
type ProbeInput struct {
	// ExecutableOverride is rejected outside tests. Production resolves claude on PATH.
	ExecutableOverride         string
	Env                        []string
	Workdir                    string
	AllowUntestedNativeVersion bool
}

// Probe is the resolved identity of a native executable.
type Probe struct {
	Executable    string `json:"executable"` // absolute, symlinks resolved
	Version       string `json:"version"`
	SHA256        string `json:"sha256"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	Compatibility string `json:"compatibility"` // e.g. "fixture-tested on 2.1.284"
}

// PrepareInput is what the supervisor has resolved before launch.
type PrepareInput struct {
	Workdir   string    // the managed clone; the native working directory
	AttemptID string    // exported to the child as MYTHHELM_ATTEMPT_ID (orphan-scan marker)
	Env       []string  // the allowlisted child environment, as KEY=value
	Prompt    io.Reader // the task text, delivered on stdin
	Scenario  string    // fake adapter only
}

// LaunchProposal is everything needed to start one native attempt.
type LaunchProposal struct {
	Spec         ProcSpec
	StopLadder   []StopStep
	Overrides    []ConfigDelta
	Billing      BillingPosture
	Manifest     ConfigManifest
	Capabilities CapabilityRecord
}

// ProcSpec describes a native process. Args excludes argv[0]. No shell is
// involved: Path is executed directly with Args.
type ProcSpec struct {
	Path  string
	Args  []string
	Dir   string
	Env   []string
	Stdin io.Reader
}

// StopSignal is a step of a stop ladder. The worker delivers it to the
// native process group.
type StopSignal string

const (
	StopInterrupt StopSignal = "interrupt" // SIGINT
	StopTerminate StopSignal = "terminate" // SIGTERM
	StopKill      StopSignal = "kill"      // SIGKILL; Process.Kill on Windows
)

// StopStep sends Signal and then waits up to Grace for the process group to
// be gone before the next step.
type StopStep struct {
	Signal StopSignal    `json:"signal"`
	Grace  time.Duration `json:"grace"`
}

// ConfigDelta records one configuration change MYTHHELM makes to the native
// launch (§9.6).
type ConfigDelta struct {
	Kind   string `json:"kind"` // "env"
	Name   string `json:"name"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

// ConfigManifest records digests of the native configuration that can run
// code or send data, keyed by source (for example "user", "project").
type ConfigManifest struct {
	Digests map[string]string `json:"digests"`
}

// BillingPosture is the billing route admission recorded (§6.2, FR-4).
type BillingPosture struct {
	Mode                            string `json:"mode"`
	CredentialProvenance            string `json:"credential_provenance"`
	EntitlementClass                string `json:"entitlement_class"`
	EntitlementSource               string `json:"entitlement_source"`
	PaidContinuation                string `json:"paid_continuation"`
	PaidContinuationUserDeclaration string `json:"paid_continuation_user_declaration"`
	Qualified                       bool   `json:"qualified"`
	G05                             string `json:"g05"`
}

// Launcher is implemented by the worker. It is the only way an adapter's
// native process is created.
type Launcher interface {
	Launch(ctx context.Context, spec ProcSpec) (OwnedProc, error)
}

// OwnedProc is a native process owned by the worker.
type OwnedProc interface {
	// Stdout is the native standard output pipe.
	Stdout() io.Reader
	// Signal delivers sig to the process group.
	Signal(sig StopSignal) error
	// Wait blocks until the process exits and reports how. The session
	// calls it once, after Stdout returns EOF or an error.
	Wait() NativeExit
	// GroupGone reports whether no process of the group remains (Unix:
	// kill(-pgid, 0) fails with ESRCH; Windows: the process was waited).
	GroupGone() bool
}

// NativeExit is how a native process ended.
type NativeExit struct {
	Code   int    `json:"exit_code"` // -1 when ended by a signal
	Signal string `json:"signal"`    // empty unless ended by a signal
	Err    error  `json:"-"`         // the exit or the output stream could not be fully observed
}

// Session is one running native attempt.
type Session interface {
	// Observations is closed at the end of the stream. It is bounded and
	// lossy only for Progress; the consumer must drain it.
	Observations() <-chan Observation
	// Interrupt runs the stop ladder and reports what was confirmed.
	Interrupt(ctx context.Context) InterruptReport
	// Done delivers the native exit once, after Observations is closed.
	Done() <-chan NativeExit
}

// InterruptReport says which stop signals were sent and whether the process
// group was confirmed gone. A sent signal is not a confirmed stop (I06).
type InterruptReport struct {
	Sent      []StopSignal `json:"sent"`
	Confirmed bool         `json:"confirmed"`
	Errors    []string     `json:"errors,omitempty"`
}

// Observation is a closed sum type: SessionStarted, Progress, Retry,
// PermissionDenied, NativeError, Result or ProtocolCounters.
type Observation interface{ observation() }

// SessionStarted reports the native session's identity and configuration.
type SessionStarted struct {
	SessionID      string
	Model          string
	NativeVersion  string
	PermissionMode string
	APIKeySource   string
	ToolCount      int
	MCPServers     []MCPServer
	PluginCount    int
}

// MCPServer is an MCP server's name and connection status.
type MCPServer struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// Progress reports an assistant turn, or a tool use within it when Tool is
// set.
type Progress struct {
	Turn int
	Tool string
}

// Retry reports a native API retry.
type Retry struct {
	Attempt      int
	RetryDelayMS int64
	ErrorStatus  int
}

// PermissionDenied reports a tool the native harness refused to run.
type PermissionDenied struct {
	ToolName string
}

// NativeError reports a classified native error, such as "rate_limit".
type NativeError struct {
	Class string
}

// Result is the native result frame. Nil pointers and empty strings mean the
// native did not report the value: unknown, never zero (I09).
type Result struct {
	Subtype              string
	IsError              bool
	NumTurns             *int
	DurationMS           *int64
	StopReason           string
	Tokens               map[string]TokenUsage // by model; nil when not reported
	CostUSD              string                // native-reported retail-equivalent estimate, decimal
	PermissionDenials    []string
	ErrorCount           int
	StartupFailureReason string
}

// TokenUsage is native-reported token usage for one model.
type TokenUsage struct {
	Input         int64 `json:"input"`
	Output        int64 `json:"output"`
	CacheRead     int64 `json:"cache_read"`
	CacheCreation int64 `json:"cache_creation"`
}

// ProtocolCounters summarises what the stream decoder dropped. It is sent
// once, as the last observation.
type ProtocolCounters struct {
	ndjson.Counters
	Malformed       int64            `json:"malformed"`
	UnknownTypes    map[string]int64 `json:"unknown_types"`
	ProgressDropped int64            `json:"progress_dropped"`
}

func (SessionStarted) observation()   {}
func (Progress) observation()         {}
func (Retry) observation()            {}
func (PermissionDenied) observation() {}
func (NativeError) observation()      {}
func (Result) observation()           {}
func (ProtocolCounters) observation() {}

// Tri is a capability value. Security-critical values that have not been
// established stay Unknown, never an optimistic default (§9.2).
type Tri string

const (
	Supported   Tri = "supported"
	Unsupported Tri = "unsupported"
	Unknown     Tri = "unknown"
)

// CapabilityRecord is the §9.2 capability record, scoped to one adapter
// version, native version, OS and launch mode. Platform extends §9.2 with
// the per-platform facts NFR-4 requires.
type CapabilityRecord struct {
	SchemaVersion         int                 `json:"schema_version"`
	AdapterID             string              `json:"adapter_id"`
	AdapterVersion        string              `json:"adapter_version"`
	RuntimeVersion        string              `json:"runtime_version"`
	Mode                  string              `json:"mode"`
	ExecutionSurface      string              `json:"execution_surface"`
	HarnessID             string              `json:"harness_id"`
	FidelityQualification string              `json:"fidelity_qualification"`
	Billing               BillingCapabilities `json:"billing"`
	HostIntegration       HostIntegration     `json:"host_integration"`
	Capabilities          Capabilities        `json:"capabilities"`
	Sandbox               Sandbox             `json:"sandbox"`
	Platform              Platform            `json:"platform"`
	Qualification         string              `json:"qualification"`
}

// BillingCapabilities is the billing part of a capability record.
type BillingCapabilities struct {
	Entitlement           string  `json:"entitlement"`
	IncludedOnlySupported Tri     `json:"included_only_supported"`
	PaidOveragePrevention Tri     `json:"paid_overage_prevention"`
	EvidenceID            *string `json:"evidence_id"`
}

// HostIntegration is the host part of a capability record.
type HostIntegration struct {
	HerdrEmbedded           Tri `json:"herdr_embedded"`
	HerdrStateBridge        Tri `json:"herdr_state_bridge"`
	NativeInteractiveAttach Tri `json:"native_interactive_attach"`
}

// Capabilities lists the operation-level capabilities of §9.2.
type Capabilities struct {
	StructuredEvents  Tri `json:"structured_events"`
	Resume            Tri `json:"resume"`
	LiveSteer         Tri `json:"live_steer"`
	ApprovalBridge    Tri `json:"approval_bridge"`
	UsageTokens       Tri `json:"usage_tokens"`
	QuotaRemaining    Tri `json:"quota_remaining"`
	HardMonetaryLimit Tri `json:"hard_monetary_limit"`
	NativeSubagents   Tri `json:"native_subagents"`
}

// Sandbox is the containment part of a capability record.
type Sandbox struct {
	Status Tri    `json:"status"`
	Scope  string `json:"scope"`
}

// Platform states, for one OS, whether the worker can outlive the CLI and
// whether stopping the native process group is confirmed.
type Platform struct {
	OS                   string `json:"os"`
	WorkerDetachment     Tri    `json:"worker_detachment"`
	ProcessTreeOwnership Tri    `json:"process_tree_ownership"`
}

// BlockedError is a policy block: Code names the reason, Field the input
// that caused it, and Action what the user can do about it.
type BlockedError struct {
	Code   string
	Field  string
	Action string
}

func (e *BlockedError) Error() string {
	msg := e.Code
	if e.Field != "" {
		msg = fmt.Sprintf("%s (%s)", msg, e.Field)
	}
	if e.Action != "" {
		msg += ": " + e.Action
	}
	return msg
}
