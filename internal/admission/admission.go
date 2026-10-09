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
	"runtime"
	"strings"

	"github.com/turbokast/mythhelm/adapters/claudecode"
	"github.com/turbokast/mythhelm/adapters/fake"
	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/contain"
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

// TrustedHostDisclosure is shown and recorded whenever trusted-host is
// admitted (AC-2.3). It is the verbatim sentence every honesty surface
// carries; softening it breaks the anchored disclosure test.
const TrustedHostDisclosure = "runs with your host authority and is not adversarially contained"

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
	ExecutionProfile   string   // empty when not given; restricted is then admitted
	UseCommitted       bool     // run on the committed HEAD of a dirty checkout
	Rev                string   // run on this committed revision instead of HEAD
	Host               string   // empty or "standalone"; "herdr" is unavailable
	Scenario           string   // fake adapter only; default "happy"
	Env                []string // the parent environment the child's is built from
	TrustProjectConfig string   // sha256:<digest> of the admitted config
	NoChecks           bool
	KeepGoing          bool
	// EnvelopeFlags is the explicit --envelope-* layer, nil when no flag was
	// given; a negative field is unset. Decide carries it unresolved.
	EnvelopeFlags *billing.Ceilings
	// The remaining flags apply only to --adapter claudecode.
	StripCredentialEnv         bool   // remove credential routes from the child only (AC-4.3)
	TrustNativeConfig          string // sha256:<digest> of the inventoried native config (AC-2.5)
	DeclareEntitlement         string // plan=<class>,extra-usage=disabled (AC-4.2)
	AllowUntestedNativeVersion bool   // run a native version without fixtures (experimental)
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
	// Boundary is the evidence the consult admitted, or nil when no
	// boundary was consulted (trusted-host). The receipt renders it.
	Boundary *contain.Evidence `json:"boundary,omitempty"`
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
	// EnvelopeFlags is Request.EnvelopeFlags, unresolved; the file layer is
	// ProjectConfig.Envelopes. Resolution happens when the run is admitted.
	EnvelopeFlags *billing.Ceilings
	// Declaration is the entitlement assertion the supervisor persists
	// with the admitted event: a fresh --declare-entitlement row, or nil
	// when billing used a stored row. RecordNativeTrust likewise persists
	// the explicit native-config grant for NativeConfigDigest.
	Declaration        *Declaration
	RecordNativeTrust  bool
	NativeConfigDigest string
	// NativeHooks counts the inventoried settings hooks (zero is a real
	// measurement once a native config was inventoried). NativeTrustGrant
	// names the grant backing the run, or is empty when nothing needed
	// trusting or no native config was inventoried.
	NativeHooks      int
	NativeTrustGrant string

	NativeAuth *AuthEvidence
	Task       Task
	Snapshot   Snapshot
	Profile    Profile
	Adapter    adapter.Descriptor
	Probe      adapter.Probe
	Proposal   adapter.LaunchProposal
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
		NoChecks: req.NoChecks, KeepGoing: req.KeepGoing, EnvelopeFlags: req.EnvelopeFlags}
	task, err := readTask(req.TaskFile)
	if err != nil {
		return Decision{}, err
	}
	d.Task = task
	if err := checkCapabilityFlags(req); err != nil {
		return Decision{}, err
	}
	if d.Profile, err = consentProfile(req); err != nil {
		return Decision{}, err
	}

	if req.Adapter == AdapterClaudeCode {
		d, err = decideClaudeCode(ctx, req, d)
	} else {
		d, err = decideFake(ctx, req, d)
	}
	if err != nil {
		return Decision{}, err
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

// admitRepo resolves the snapshot, the Git identity and the project config.
// Both adapters share it: checks, tool rules and passthrough come from the
// same admitted revision.
func admitRepo(ctx context.Context, req Request, d *Decision) error {
	var err error
	if d.Snapshot, err = chooseSnapshot(ctx, req); err != nil {
		return err
	}
	name, nameErr := workspace.Git(ctx, d.Snapshot.SourceRepo, true, "config", "--get", "user.name")
	email, emailErr := workspace.Git(ctx, d.Snapshot.SourceRepo, true, "config", "--get", "user.email")
	if nameErr != nil || emailErr != nil || strings.TrimSpace(string(name)) == "" || strings.TrimSpace(string(email)) == "" {
		return &BlockedError{Code: "git_identity_unavailable", Field: d.Snapshot.SourceRepo,
			Action: "configure Git user.name and user.email before admitting a run"}
	}
	d.GitName, d.GitEmail = strings.TrimSpace(string(name)), strings.TrimSpace(string(email))
	return d.admitProjectConfig(ctx, req)
}

func decideFake(ctx context.Context, req Request, d Decision) (Decision, error) {
	a := fake.New()
	d.Adapter = a.Descriptor()
	var err error
	if d.Probe, err = a.Probe(ctx, adapter.ProbeInput{}); err != nil {
		return Decision{}, fmt.Errorf("probing adapter %s: %w", d.Adapter.ID, err)
	}
	if err := admitRepo(ctx, req, &d); err != nil {
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
	return d, nil
}

// decideClaudeCode admits a native run through the design §6.2 steps: the
// repository, the credential-route screen, the probe, the native inventory
// and trust, the auth evidence, the billing posture and the launch.
func decideClaudeCode(ctx context.Context, req Request, d Decision) (Decision, error) {
	a := claudecode.New()
	d.Adapter = a.Descriptor()
	if err := admitRepo(ctx, req, &d); err != nil {
		return Decision{}, err
	}
	// Step 1 runs before any native process starts, so the version probe
	// never inherits a credential route. The opt-in stays false: the
	// slice has no trusted user-level config loader yet, so without it
	// the token route blocks or strips like any other override (AC-4.7).
	childEnv, deltas, err := ResolveCredentialEnv(req.Env, req.StripCredentialEnv, false, d.ProjectConfig.Environment.Passthrough)
	if err != nil {
		return Decision{}, err
	}
	if d.Probe, err = a.Probe(ctx, adapter.ProbeInput{Env: childEnv, Workdir: d.Snapshot.SourceRepo, AllowUntestedNativeVersion: req.AllowUntestedNativeVersion}); err != nil {
		return Decision{}, NativeAdmissionError(fmt.Errorf("probing adapter %s: %w", d.Adapter.ID, err))
	}
	d.RunID, d.TaskID, d.AttemptID = ids.New("run"), ids.New("task"), ids.New("att")
	d.RunDir = filepath.Join(req.StateDir, "runs", d.RunID)
	d.Workdir = filepath.Join(d.RunDir, "workspace")
	manifest, err := d.admitNativeConfig(ctx, req, childEnv)
	if err != nil {
		return Decision{}, err
	}
	// Auth status runs with the exact child environment and, for its
	// working directory, the source checkout: the workspace clone does
	// not exist yet, and it carries the same committed project settings.
	authEnv := append(append([]string{}, childEnv...),
		"DISABLE_AUTOUPDATER=1", "CLAUDE_CODE_STARTUP_FAILURE_RESULTS=1", "MYTHHELM_ATTEMPT_ID="+d.AttemptID)
	evidence, err := claudecode.AuthStatus(ctx, d.Probe, authEnv, d.Snapshot.SourceRepo)
	if err != nil {
		return Decision{}, NativeAdmissionError(err)
	}
	d.NativeAuth = &evidence
	// The qualification consult follows AuthStatus, so provider and account
	// class are known, and precedes billing: registry eligibility (this
	// route is qualified) comes before the billing posture (this run's
	// funding), and either can block (D7). The consult never migrates or
	// seeds; admission writes nothing.
	reg, err := OpenQualificationRegistry(ctx, req.StateDir)
	if err != nil {
		return Decision{}, err
	}
	if reg != nil {
		defer func() { _ = reg.Close() }()
		// Gaps are assessed at consult time, never stored in a record; a
		// missing registry has no consult to qualify and skips the check.
		if gaps := claudecode.GapSources(claudecode.UnresolvedSources); req.Billing == BillingSubscriptionOnly && len(gaps) > 0 {
			return Decision{}, &BlockedError{Code: reasonEntitlementUnp, Field: "missing-source:" + gaps[0],
				Action: "the effective managed policy has a source MYTHHELM cannot inventory; see ADR-0002"}
		}
	}
	elig, err := ResolveQualification(ctx, reg, d.Probe,
		adapter.ConfigManifest{Digests: manifest.Digests}, evidence, req.Billing, d.Profile.Name)
	if err != nil {
		return Decision{}, err
	}
	switch elig.Verdict {
	case Eligible:
	case Blocked:
		return Decision{}, &BlockedError{Code: elig.Reason,
			Field:  d.Adapter.Harness + " × " + d.Adapter.Surface,
			Action: "run 'mythhelm doctor' to inspect the qualification records"}
	case Unsupported:
		return Decision{}, &BlockedError{Code: elig.Reason, Capability: true,
			Field:  d.Adapter.Harness + " × " + d.Adapter.Surface,
			Action: "this surface is marked unsupported; run 'mythhelm doctor' for details"}
	default:
		return Decision{}, fmt.Errorf("admission: unknown qualification verdict %q", elig.Verdict)
	}
	decl, err := resolveDeclaration(ctx, req, evidence)
	if err != nil {
		return Decision{}, err
	}
	posture, err := ResolveBilling(ctx, req.Billing, evidence, decl, elig)
	if err != nil {
		return Decision{}, err
	}
	d.Proposal, err = a.Prepare(ctx, claudePrepareInput(&d, childEnv))
	if err != nil {
		return Decision{}, NativeAdmissionError(fmt.Errorf("preparing adapter %s: %w", d.Adapter.ID, err))
	}
	d.Proposal.Billing = posture
	d.Proposal.Overrides = append(deltas, d.Proposal.Overrides...)
	d.Proposal.Manifest = adapter.ConfigManifest{Digests: manifest.Digests}
	if req.DeclareEntitlement != "" {
		d.Declaration = decl
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
		if req.StripCredentialEnv {
			problems = append(problems, "--strip-credential-env applies only to --adapter claudecode")
		}
		if req.TrustNativeConfig != "" {
			problems = append(problems, "--trust-native-config applies only to --adapter claudecode")
		}
		if req.DeclareEntitlement != "" {
			problems = append(problems, "--declare-entitlement applies only to --adapter claudecode")
		}
		if req.AllowUntestedNativeVersion {
			problems = append(problems, "--allow-untested-native-version applies only to --adapter claudecode")
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
	if req.Host == HostHerdr {
		return &BlockedError{Code: "host_unavailable", Field: "--host herdr", Capability: true,
			Action: "the Herdr bridge is not in this build; run standalone"}
	}
	return nil
}

// consentProfile resolves the execution profile. An empty flag admits the
// safe default, restricted, with no question; trusted-host widens the posture
// and needs the explicit flag (I04). A contained profile is admitted only on
// recorded boundary evidence for the host OS and the adapter route.
func consentProfile(req Request) (Profile, error) {
	switch req.ExecutionProfile {
	case ProfileTrustedHost:
		return Profile{Name: ProfileTrustedHost, Consent: "--execution-profile", Disclosure: TrustedHostDisclosure}, nil
	case "", ProfileRestricted, ProfileInspect:
		p := Profile{Name: ProfileRestricted, Contained: true, Consent: "--execution-profile"}
		if req.ExecutionProfile == ProfileInspect {
			p.Name = ProfileInspect
		}
		if req.ExecutionProfile == "" {
			p.Consent = "default"
		}
		ev, err := BoundaryConsult(p.Name, runtime.GOOS, "builtin/"+req.Adapter)
		if err != nil {
			return Profile{}, err
		}
		p.Boundary = &ev
		return p, nil
	}
	return Profile{}, fmt.Errorf("%w: unreachable execution profile %q", ErrInvalid, req.ExecutionProfile)
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
	return Task{Content: b, SHA256: hex.EncodeToString(sum[:]), Title: TaskTitle(b)}, nil
}

// TaskTitle is the first Markdown heading, or the first non-empty line.
func TaskTitle(b []byte) string {
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
	NativeAuth           *AuthEvidence            `json:"native_auth,omitempty"`
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
	AllowedTools         []string                 `json:"allowed_tools,omitempty"`
	NativeConfigDigest   string                   `json:"native_config_digest,omitempty"`
	NativeHooks          int                      `json:"native_hooks,omitempty"`
	NativeTrustGrant     string                   `json:"native_trust_grant,omitempty"`
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
		NativeAuth:           d.NativeAuth,
		Snapshot:             d.Snapshot,
		Task:                 TaskRecord{SHA256: d.Task.SHA256, Bytes: len(d.Task.Content)},
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
		AllowedTools:         d.ProjectConfig.Adapters.ClaudeCode.AllowedTools,
		NativeConfigDigest:   d.NativeConfigDigest,
		NativeHooks:          d.NativeHooks,
		NativeTrustGrant:     d.NativeTrustGrant,
	}
}

func dataDestinations(adapterID string) []string {
	out := []string{"local state directory"}
	switch adapterID {
	case fake.New().Descriptor().ID:
		out = append(out, "no network (scripted fake agent)")
	case claudecode.AdapterID:
		out = append(out, "first-party native API over the network")
	}
	return out
}
