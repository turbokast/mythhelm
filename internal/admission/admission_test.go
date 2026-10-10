package admission_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/billing"
)

func envelopeRepo(t *testing.T, configTable string) (repo, task string) {
	t.Helper()
	repo, home := t.TempDir(), t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...) // #nosec G204 -- fixed test argv only
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+home, "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "--quiet", "--initial-branch=main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	cfg := envelopesConfig(configTable)
	if err := os.WriteFile(filepath.Join(repo, "mythhelm.toml"), cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "mythhelm.toml")
	git("commit", "--quiet", "-m", "config")
	task = filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(task, []byte("# Demo task\n\nWrite demo.txt.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return repo, task
}

func decideEnvelopes(t *testing.T, configTable string, flags *billing.Ceilings) admission.Decision {
	t.Helper()
	repo, task := envelopeRepo(t, configTable)
	_, digest, err := admission.ParseProjectConfig(envelopesConfig(configTable))
	if err != nil {
		t.Fatal(err)
	}
	d, err := admission.Decide(t.Context(), admission.Request{
		StateDir: t.TempDir(), Repo: repo, TaskFile: task, Adapter: admission.AdapterFake,
		Billing: admission.BillingLocalScripted, ExecutionProfile: "trusted-host",
		TrustProjectConfig: "sha256:" + digest, Env: os.Environ(), EnvelopeFlags: flags,
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDecideCarriesEnvelopeLayers(t *testing.T) {
	flags := &billing.Ceilings{Execution: -1, Repairs: 5, Replans: -1, TransportRetries: -1}
	d := decideEnvelopes(t, "[envelopes]\nexecution = \"10m\"\nrepairs = 1", flags)

	// The flag layer is carried exactly as given: not merged with the file
	// layer and not filled from the built-ins.
	if d.EnvelopeFlags == nil || *d.EnvelopeFlags != *flags {
		t.Fatalf("Decision.EnvelopeFlags = %+v, want %+v unresolved", d.EnvelopeFlags, flags)
	}
	file, err := d.ProjectConfig.Envelopes.ToCeilings()
	want := billing.Ceilings{Execution: 10 * time.Minute, Repairs: 1, Replans: -1, TransportRetries: -1}
	if err != nil || file == nil || *file != want {
		t.Fatalf("file layer = %+v, %v; want %+v unresolved", file, err, want)
	}

	// No flags given: the flag layer stays nil, never a resolved default.
	d = decideEnvelopes(t, "[envelopes]\nrepairs = 1", nil)
	if d.EnvelopeFlags != nil {
		t.Fatalf("Decision.EnvelopeFlags = %+v, want nil when no flag was given", d.EnvelopeFlags)
	}
}

// stubAdapter is the adapter a stub table row prepares; it counts Prepare calls.
type stubAdapter struct{ prepares int }

func (*stubAdapter) Descriptor() adapter.Descriptor {
	return adapter.Descriptor{ID: "builtin/stub", Version: "1", Harness: "stub", Surface: "stub"}
}

func (*stubAdapter) Probe(context.Context, adapter.ProbeInput) (adapter.Probe, error) {
	return adapter.Probe{}, errors.New("stub: not probed")
}

func (*stubAdapter) Capabilities(adapter.Probe) adapter.CapabilityRecord {
	return adapter.CapabilityRecord{}
}

func (s *stubAdapter) Prepare(context.Context, adapter.PrepareInput) (adapter.LaunchProposal, error) {
	s.prepares++
	return adapter.LaunchProposal{
		Spec:         adapter.ProcSpec{Path: "/stub/native"},
		Billing:      adapter.BillingPosture{Mode: admission.BillingLocalScripted},
		Capabilities: adapter.CapabilityRecord{Capabilities: adapter.Capabilities{StructuredEvents: adapter.Supported}},
	}, nil
}

func (*stubAdapter) Start(context.Context, adapter.LaunchProposal, adapter.Launcher) (adapter.Session, error) {
	return nil, errors.New("stub: not started")
}

func stubRequest(t *testing.T, name string) admission.Request {
	t.Helper()
	task := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(task, []byte("# Stub task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return admission.Request{StateDir: t.TempDir(), TaskFile: task, Adapter: name,
		Billing: admission.BillingLocalScripted, ExecutionProfile: admission.ProfileTrustedHost}
}

func TestDecideDispatchesByTable(t *testing.T) {
	t.Parallel()
	stub := &stubAdapter{}
	decide := func(ctx context.Context, _ admission.Request, d admission.Decision) (admission.Decision, error) {
		var err error
		d.Adapter = stub.Descriptor()
		d.Proposal, err = stub.Prepare(ctx, adapter.PrepareInput{})
		return d, err
	}

	d, err := admission.DecideWithHarness(t.Context(), stubRequest(t, "stub"), "stub", decide)
	if err != nil {
		t.Fatalf("Decide through a stub row: %v", err)
	}
	if d.Proposal.Spec.Path != "/stub/native" || d.Adapter.ID != "builtin/stub" || stub.prepares != 1 {
		t.Errorf("proposal path %q, adapter %q, Prepare calls %d; want the stub row's decide to run once",
			d.Proposal.Spec.Path, d.Adapter.ID, stub.prepares)
	}

	// Without the row the same request is an unknown adapter.
	_, err = admission.Decide(t.Context(), stubRequest(t, "stub"))
	if !errors.Is(err, admission.ErrInvalid) || !strings.Contains(err.Error(), `got "stub"`) {
		t.Errorf("Decide with the row absent: err = %v, want ErrInvalid naming the adapter", err)
	}
}

func validRequest(adapterName string) admission.Request {
	return admission.Request{StateDir: filepath.Join(os.TempDir(), "state"), TaskFile: "task.md", Adapter: adapterName,
		Billing: admission.BillingSubscriptionOnly}
}

func TestValidateAdapterText(t *testing.T) {
	t.Parallel()
	req := validRequest("x")
	err := admission.Validate(&req)
	if !errors.Is(err, admission.ErrInvalid) || !strings.Contains(err.Error(), `--adapter must be claudecode, codex or fake, got "x"`) {
		t.Errorf("unknown adapter: err = %v", err)
	}
	req = validRequest("")
	err = admission.Validate(&req)
	if err == nil || !strings.Contains(err.Error(), "--adapter is required (claudecode, codex or fake)") {
		t.Errorf("missing adapter: err = %v", err)
	}
	req = validRequest("codex")
	if err := admission.Validate(&req); err != nil {
		t.Errorf("--adapter codex: %v", err)
	}
}

func TestValidateClaudeFlagRules(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		flag                      string
		set                       func(*admission.Request)
		claudecode, codex, fakeOK bool
	}{
		{"--strip-credential-env", func(r *admission.Request) { r.StripCredentialEnv = true }, true, false, false},
		{"--trust-native-config", func(r *admission.Request) { r.TrustNativeConfig = "sha256:abc" }, true, true, false},
		{"--declare-entitlement", func(r *admission.Request) { r.DeclareEntitlement = "plan=pro,extra-usage=disabled" }, true, false, false},
		{"--allow-untested-native-version", func(r *admission.Request) { r.AllowUntestedNativeVersion = true }, true, true, false},
		{"--scenario", func(r *admission.Request) { r.Scenario = "happy" }, false, false, true},
	} {
		for adapterName, want := range map[string]bool{"claudecode": tc.claudecode, "codex": tc.codex, "fake": tc.fakeOK} {
			t.Run(tc.flag+" with "+adapterName, func(t *testing.T) {
				t.Parallel()
				req := validRequest(adapterName)
				tc.set(&req)
				err := admission.Validate(&req)
				if want && err != nil {
					t.Errorf("rejected: %v", err)
				}
				if !want && (!errors.Is(err, admission.ErrInvalid) || !strings.Contains(err.Error(), tc.flag+" applies only to --adapter")) {
					t.Errorf("accepted or wrong error: %v", err)
				}
			})
		}
	}
}

func TestValidateFakeDefaultsScenarioToHappy(t *testing.T) {
	t.Parallel()
	req := validRequest("fake")
	if err := admission.Validate(&req); err != nil {
		t.Fatal(err)
	}
	if req.Scenario != "happy" {
		t.Errorf("Scenario = %q, want happy", req.Scenario)
	}
}

func TestCodexDeclarationFlagRefused(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		flag string
		set  func(*admission.Request)
	}{
		{"--declare-entitlement", func(r *admission.Request) { r.DeclareEntitlement = "plan=pro,extra-usage=disabled" }},
		{"--strip-credential-env", func(r *admission.Request) { r.StripCredentialEnv = true }},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			t.Parallel()
			req := validRequest("codex")
			tc.set(&req)
			err := admission.Validate(&req)
			if !errors.Is(err, admission.ErrInvalid) || !strings.Contains(err.Error(), tc.flag+" applies only to --adapter claudecode") {
				t.Errorf("err = %v, want ErrInvalid naming %s", err, tc.flag)
			}
		})
	}
}

func TestDecideCodexIsNotAvailableYet(t *testing.T) {
	t.Parallel()
	_, err := admission.Decide(t.Context(), stubRequest(t, "codex"))
	blocked, ok := errors.AsType[*admission.BlockedError](err)
	if !ok || blocked.Code != "adapter_not_available" || !blocked.Capability {
		t.Errorf("err = %v, want a capability (exit 7) adapter_not_available refusal", err)
	}
}

// consentProfile derives the boundary route as builtin/<name>; a row whose
// adapter ID differs would be consulted under one route and launched under
// another (I02).
func TestHarnessRowsMatchBoundaryRoute(t *testing.T) {
	t.Parallel()
	routes := admission.HarnessRoutes()
	if len(routes) != 3 {
		t.Fatalf("default table has %d rows, want claudecode, codex and fake: %v", len(routes), routes)
	}
	for name, id := range routes {
		if id != "builtin/"+name {
			t.Errorf("row %s has adapter ID %q, want builtin/%s", name, id, name)
		}
	}
}
