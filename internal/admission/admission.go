// Package admission decides whether a run may start (§8.3, I02): it resolves
// the adapter, the native executable, the workspace snapshot, the execution
// profile, the billing posture and the required capabilities, and refuses
// the run while any of them is unknown or disallowed. It writes nothing; the
// supervisor persists the decision before any worker is spawned.
package admission

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/turbokast/mythhelm/adapters/fake"
	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/security"
	"github.com/turbokast/mythhelm/internal/workspace"
)

// Request values accepted on the command line (design §10).
const (
	AdapterFake       = "fake"
	AdapterClaudeCode = "claudecode"

	BillingSubscriptionOnly     = "subscription-only"
	BillingSubscriptionDeclared = "subscription-declared"
	BillingLocalScripted        = "local-scripted"

	ProfileTrustedHost = "trusted-host"
	ProfileRestricted  = "restricted"
	ProfileInspect     = "inspect"

	HostStandalone = "standalone"
	HostHerdr      = "herdr"
)

// MaxTaskBytes bounds the task file, which reaches the native on stdin.
const MaxTaskBytes = 1 << 20

// trustedHostDisclosure is shown and recorded whenever trusted-host is
// admitted (AC-2.3).
const trustedHostDisclosure = "not contained: native tools and checks run with your host authority"

// ErrInvalid reports a request that is malformed rather than refused: a
// missing or unknown flag value, or an unreadable task file (exit 2).
var ErrInvalid = errors.New("invalid run request")

// BlockedError is an admission refusal. Code is the reason code, Field the
// input or fact that caused it, and Action what the user can do. Capability
// is set when a required capability is unavailable (exit 7) rather than a
// policy blocking the run (exit 3).
type BlockedError struct {
	Code       string
	Field      string
	Action     string
	Capability bool
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

// Request is what the user asked for.
type Request struct {
	StateDir           string   // absolute state directory
	Repo               string   // a directory inside the user's repository
	TaskFile           string   // the task, delivered to the native on stdin
	Adapter            string   // "claudecode" or "fake"; no default (AC-2.2)
	Billing            string   // no default (AC-4.1)
	ExecutionProfile   string   // empty when not given; consent is then asked for
	UseCommitted       bool     // run on the committed HEAD of a dirty checkout
	Rev                string   // run on this committed revision instead of HEAD
	Host               string   // empty or "standalone"; "herdr" is unavailable
	Scenario           string   // fake adapter only; default "happy"
	Env                []string // the parent environment the child's is built from
	TrustProjectConfig string   // sha256:<digest> of the admitted config
	NoChecks           bool
	KeepGoing          bool
	// Confirm asks the user a yes/no question. It is nil under
	// --non-interactive, and then every question blocks instead (AC-1.4).
	Confirm func(question string) (bool, error)
}

// Task is the admitted task file.
type Task struct {
	Content []byte
	SHA256  string
	Title   string
}

// Snapshot is the admitted revision of the user's repository (AC-3.1).
type Snapshot struct {
	SourceRepo string `json:"source_repo"`
	Branch     string `json:"branch"`
	BaseRev    string `json:"base_rev"`
	Dirty      bool   `json:"dirty_at_admission"`
	Source     string `json:"selected_by"` // "HEAD", "--use-committed", "interactive" or "--rev"
}

// Profile is the admitted execution profile and how the user consented.
type Profile struct {
	Name       string `json:"name"`
	Contained  bool   `json:"contained"`
	Consent    string `json:"consent"` // "--execution-profile" or "interactive"
	Disclosure string `json:"disclosure"`
}

// Decision is an admitted run: everything the supervisor needs to record it
// and launch its one attempt, resolved before anything is written.
type Decision struct {
	StateDir      string
	RunID         string
	TaskID        string
	AttemptID     string
	RunDir        string // StateDir/runs/<run_id>
	Workdir       string // RunDir/workspace, the snapshot clone the native works in
	Host          string
	Scenario      string
	GitName       string // user's Git identity captured before the native starts
	GitEmail      string
	ProjectConfig ProjectConfig
	ConfigDigest  string
	RepoIdentity  string
	RecordTrust   bool
	NoChecks      bool
	KeepGoing     bool

	Task     Task
	Snapshot Snapshot
	Profile  Profile
	Adapter  adapter.Descriptor
	Probe    adapter.Probe
	Proposal adapter.LaunchProposal
}

// requiredCapabilities are the capabilities a run cannot proceed without; an
// adapter reporting anything but supported for one blocks with exit 7.
func requiredCapabilities(c adapter.CapabilityRecord) map[string]adapter.Tri {
	return map[string]adapter.Tri{"structured_events": c.Capabilities.StructuredEvents}
}

// Decide admits or refuses req. A refusal is a *BlockedError, an error
// wrapping ErrInvalid or workspace.ErrGitTooOld, or an unexpected error.
func Decide(ctx context.Context, req Request) (Decision, error) {
	if err := validate(&req); err != nil {
		return Decision{}, err
	}
	d := Decision{StateDir: req.StateDir, Host: req.Host, Scenario: req.Scenario,
		NoChecks: req.NoChecks, KeepGoing: req.KeepGoing}
	task, err := readTask(req.TaskFile)
	if err != nil {
		return Decision{}, err
	}
	d.Task = task
	if err := checkCapabilityFlags(req); err != nil {
		return Decision{}, err
	}
	if req.Billing == BillingSubscriptionOnly {
		return Decision{}, &BlockedError{Code: "entitlement_qualification_unavailable", Field: "--billing subscription-only",
			Action: "MYTHHELM cannot yet verify an included-only entitlement, so strict subscription-only never admits a run"}
	}
	if d.Profile, err = consentProfile(req); err != nil {
		return Decision{}, err
	}

	a := fake.New()
	d.Adapter = a.Descriptor()
	if d.Probe, err = a.Probe(ctx, adapter.ProbeInput{}); err != nil {
		return Decision{}, fmt.Errorf("probing adapter %s: %w", d.Adapter.ID, err)
	}

	if d.Snapshot, err = chooseSnapshot(ctx, req); err != nil {
		return Decision{}, err
	}
	name, nameErr := workspace.Git(ctx, d.Snapshot.SourceRepo, true, "config", "--get", "user.name")
	email, emailErr := workspace.Git(ctx, d.Snapshot.SourceRepo, true, "config", "--get", "user.email")
	if nameErr != nil || emailErr != nil || strings.TrimSpace(string(name)) == "" || strings.TrimSpace(string(email)) == "" {
		return Decision{}, &BlockedError{Code: "git_identity_unavailable", Field: d.Snapshot.SourceRepo,
			Action: "configure Git user.name and user.email before admitting a run"}
	}
	d.GitName, d.GitEmail = strings.TrimSpace(string(name)), strings.TrimSpace(string(email))
	if err := d.admitProjectConfig(ctx, req); err != nil {
		return Decision{}, err
	}

	d.RunID, d.TaskID, d.AttemptID = ids.New("run"), ids.New("task"), ids.New("att")
	d.RunDir = filepath.Join(req.StateDir, "runs", d.RunID)
	d.Workdir = filepath.Join(d.RunDir, "workspace")
	env, err := security.BuildEnv(req.Env, d.ProjectConfig.Environment.Passthrough, nil)
	if err != nil {
		return Decision{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	d.Proposal, err = a.Prepare(ctx, adapter.PrepareInput{Workdir: d.Workdir, AttemptID: d.AttemptID, Env: env, Scenario: req.Scenario})
	var blocked *adapter.BlockedError
	switch {
	case errors.As(err, &blocked):
		return Decision{}, &BlockedError{Code: blocked.Code, Field: blocked.Field, Action: blocked.Action}
	case errors.Is(err, fake.ErrUnknownScenario):
		return Decision{}, fmt.Errorf("%w: --scenario: %w", ErrInvalid, err)
	case err != nil:
		return Decision{}, fmt.Errorf("preparing adapter %s: %w", d.Adapter.ID, err)
	}
	if mode := d.Proposal.Billing.Mode; mode != req.Billing {
		return Decision{}, &BlockedError{Code: "billing_posture_mismatch", Field: "--billing " + req.Billing,
			Action: fmt.Sprintf("adapter %s runs under the %s billing posture; pass --billing %s", d.Adapter.ID, mode, mode)}
	}
	// Gated on the proposal that is launched and journaled, so admission
	// checks exactly what admission.decided records.
	for name, v := range requiredCapabilities(d.Proposal.Capabilities) {
		if v != adapter.Supported {
			return Decision{}, &BlockedError{Code: "capability_unavailable", Field: fmt.Sprintf("%s=%s", name, v), Capability: true,
				Action: fmt.Sprintf("adapter %s does not support a capability this run requires", d.Adapter.ID)}
		}
	}
	return d, nil
}

func validate(req *Request) error {
	var problems []string
	switch req.Adapter {
	case "":
		problems = append(problems, "--adapter is required (claudecode or fake); there is no default and no fallback")
	case AdapterFake:
		if req.Scenario == "" {
			req.Scenario = "happy"
		}
	case AdapterClaudeCode:
		if req.Scenario != "" {
			problems = append(problems, "--scenario applies only to --adapter fake")
		}
	default:
		problems = append(problems, fmt.Sprintf("--adapter must be claudecode or fake, got %q", req.Adapter))
	}
	switch req.Billing {
	case "":
		problems = append(problems, "--billing is required (subscription-declared, subscription-only or local-scripted); there is no default")
	case BillingSubscriptionOnly, BillingSubscriptionDeclared, BillingLocalScripted:
	default:
		problems = append(problems, fmt.Sprintf("--billing must be subscription-declared, subscription-only or local-scripted, got %q", req.Billing))
	}
	switch req.ExecutionProfile {
	case "", ProfileTrustedHost, ProfileRestricted, ProfileInspect:
	default:
		problems = append(problems, fmt.Sprintf("--execution-profile must be trusted-host, restricted or inspect, got %q", req.ExecutionProfile))
	}
	switch req.Host {
	case "":
		req.Host = HostStandalone
	case HostStandalone, HostHerdr:
	default:
		problems = append(problems, fmt.Sprintf("--host must be standalone or herdr, got %q", req.Host))
	}
	if req.TaskFile == "" {
		problems = append(problems, "--task-file is required")
	}
	if req.UseCommitted && req.Rev != "" {
		problems = append(problems, "--use-committed and --rev are mutually exclusive")
	}
	if !filepath.IsAbs(req.StateDir) {
		problems = append(problems, fmt.Sprintf("state directory %q is not absolute", req.StateDir))
	}
	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", ErrInvalid, strings.Join(problems, "; "))
	}
	return nil
}

// checkCapabilityFlags refuses what the slice cannot do (exit 7).
func checkCapabilityFlags(req Request) error {
	switch {
	case req.Host == HostHerdr:
		return &BlockedError{Code: "host_unavailable", Field: "--host herdr", Capability: true,
			Action: "the Herdr bridge is not in this build; run standalone"}
	case req.ExecutionProfile == ProfileRestricted, req.ExecutionProfile == ProfileInspect:
		return &BlockedError{Code: "execution_profile_unavailable", Field: "--execution-profile " + req.ExecutionProfile, Capability: true,
			Action: "sandboxed profiles are not in this build; only trusted-host (not contained) is available"}
	case req.Adapter == AdapterClaudeCode:
		return &BlockedError{Code: "adapter_unavailable", Field: "--adapter claudecode", Capability: true,
			Action: "the Claude Code adapter is not in this build yet; only --adapter fake runs"}
	}
	return nil
}

func consentProfile(req Request) (Profile, error) {
	p := Profile{Name: ProfileTrustedHost, Disclosure: trustedHostDisclosure}
	if req.ExecutionProfile == ProfileTrustedHost {
		p.Consent = "--execution-profile"
		return p, nil
	}
	blocked := &BlockedError{Code: "consent_required", Field: "--execution-profile",
		Action: "pass --execution-profile trusted-host to run " + trustedHostDisclosure}
	ok, err := confirm(req, "Execution profile trusted-host: "+trustedHostDisclosure+". Continue?")
	if err != nil || !ok {
		return Profile{}, errors.Join(blocked, err)
	}
	p.Consent = "interactive"
	return p, nil
}

func confirm(req Request, question string) (bool, error) {
	if req.Confirm == nil {
		return false, nil
	}
	return req.Confirm(question)
}

func readTask(path string) (Task, error) {
	f, err := os.Open(path) //nolint:gosec // G304: the user names the task file
	if err != nil {
		return Task{}, fmt.Errorf("%w: --task-file: %w", ErrInvalid, err)
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, MaxTaskBytes+1))
	if err != nil {
		return Task{}, fmt.Errorf("%w: reading --task-file: %w", ErrInvalid, err)
	}
	if len(b) > MaxTaskBytes {
		return Task{}, fmt.Errorf("%w: --task-file is larger than %d bytes", ErrInvalid, MaxTaskBytes)
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return Task{}, fmt.Errorf("%w: --task-file is empty", ErrInvalid)
	}
	sum := sha256.Sum256(b)
	return Task{Content: b, SHA256: hex.EncodeToString(sum[:]), Title: title(b)}, nil
}

// title is the first Markdown heading, or the first non-empty line.
func title(b []byte) string {
	first := ""
	for line := range strings.Lines(string(b)) {
		line = strings.TrimSpace(line)
		if h, ok := strings.CutPrefix(line, "#"); ok {
			return strings.TrimSpace(strings.TrimLeft(h, "#"))
		}
		if first == "" {
			first = line
		}
	}
	return first
}

// chooseSnapshot runs preflight and picks the committed revision to snapshot.
// A dirty checkout is never stashed, reset or committed: the user chooses
// the committed revision instead, or the run is blocked (AC-3.2).
func chooseSnapshot(ctx context.Context, req Request) (Snapshot, error) {
	pre, err := workspace.Preflight(ctx, req.Repo)
	switch {
	case errors.Is(err, workspace.ErrGitTooOld):
		return Snapshot{}, err
	case err != nil:
		return Snapshot{}, fmt.Errorf("%w: preflight of %s: %w", ErrInvalid, req.Repo, err)
	case len(pre.Unsupported) > 0:
		return Snapshot{}, &BlockedError{Code: "unsupported_repository", Field: strings.Join(pre.Unsupported, ", "), Capability: true,
			Action: "MYTHHELM cannot snapshot repositories using these features yet"}
	}
	s := Snapshot{SourceRepo: pre.Top, Branch: pre.Branch, BaseRev: pre.HeadRev, Dirty: pre.Dirty, Source: "HEAD"}
	switch {
	case req.Rev != "":
		out, err := workspace.Git(ctx, pre.Top, true, "rev-parse", "--verify", "--end-of-options", req.Rev+"^{commit}")
		if err != nil {
			return Snapshot{}, fmt.Errorf("%w: --rev %q: %w", ErrInvalid, req.Rev, err)
		}
		s.BaseRev, s.Source = strings.TrimSpace(string(out)), "--rev"
		if s.BaseRev != pre.HeadRev {
			s.Branch = ""
		}
	case !pre.Dirty:
	case req.UseCommitted:
		s.Source = "--use-committed"
	default:
		ok, err := confirm(req, fmt.Sprintf("The checkout has uncommitted changes, which the run would not see. Run on the committed revision %s instead?", short(pre.HeadRev)))
		if err != nil || !ok {
			return Snapshot{}, errors.Join(&BlockedError{Code: "dirty_checkout", Field: pre.Top,
				Action: "commit or remove the changes, or pass --use-committed (or --rev <rev>) to run on the committed revision"}, err)
		}
		s.Source = "interactive"
	}
	return s, nil
}

func short(rev string) string {
	if len(rev) > 12 {
		return rev[:12]
	}
	return rev
}

// Record is the admission.decided payload (design §5): identifiers, digests
// and labels, never task content or credential values.
type Record struct {
	Adapter              adapter.Descriptor       `json:"adapter"`
	Native               adapter.Probe            `json:"native"`
	Snapshot             Snapshot                 `json:"snapshot"`
	Task                 TaskRecord               `json:"task"`
	DataDestinations     []string                 `json:"data_destinations"`
	ExecutionProfile     Profile                  `json:"execution_profile"`
	Billing              adapter.BillingPosture   `json:"billing"`
	RequiredCapabilities map[string]adapter.Tri   `json:"required_capabilities"`
	Capabilities         adapter.CapabilityRecord `json:"capabilities"`
	Overrides            []adapter.ConfigDelta    `json:"overrides"`
	ConfigManifest       adapter.ConfigManifest   `json:"config_manifest"`
	Host                 string                   `json:"host"`
	Scenario             string                   `json:"scenario,omitempty"`
	GitIdentity          GitIdentity              `json:"git_identity"`
	ProjectConfigDigest  string                   `json:"project_config_digest,omitempty"`
	NoChecks             bool                     `json:"no_checks"`
	KeepGoing            bool                     `json:"keep_going"`
}

// GitIdentity is the commit identity captured from the source repository at
// admission. Recovery can use it without consulting mutable Git config.
type GitIdentity struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// TaskRecord identifies the task without its content.
type TaskRecord struct {
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
	Title  string `json:"title"`
}

// Record returns what admission.decided journals.
func (d Decision) Record() Record {
	overrides := d.Proposal.Overrides
	if overrides == nil {
		overrides = []adapter.ConfigDelta{}
	}
	return Record{
		Adapter:              d.Adapter,
		Native:               d.Probe,
		Snapshot:             d.Snapshot,
		Task:                 TaskRecord{SHA256: d.Task.SHA256, Bytes: len(d.Task.Content), Title: d.Task.Title},
		DataDestinations:     dataDestinations(d.Adapter.ID),
		ExecutionProfile:     d.Profile,
		Billing:              d.Proposal.Billing,
		RequiredCapabilities: requiredCapabilities(d.Proposal.Capabilities),
		Capabilities:         d.Proposal.Capabilities,
		Overrides:            overrides,
		ConfigManifest:       d.Proposal.Manifest,
		Host:                 d.Host,
		Scenario:             d.Scenario,
		GitIdentity:          GitIdentity{Name: d.GitName, Email: d.GitEmail},
		ProjectConfigDigest:  d.ConfigDigest,
		NoChecks:             d.NoChecks,
		KeepGoing:            d.KeepGoing,
	}
}

func dataDestinations(adapterID string) []string {
	out := []string{"local state directory"}
	if adapterID == fake.New().Descriptor().ID {
		out = append(out, "no network (scripted fake agent)")
	}
	return out
}
