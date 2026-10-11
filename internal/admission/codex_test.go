package admission_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/adapters/claudecode"
	"github.com/turbokast/mythhelm/adapters/codex"
	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/qualify"
)

const nativeManaged = "native-managed"

// codexWorld is a runnable codex admission: a fake native on PATH that logs
// its invocations, an empty home, and a task file. repo is created by
// repoWith.
type codexWorld struct {
	home string
	log  string
	task string
}

func newCodexWorld(t *testing.T) codexWorld {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CODEX_HOME", "")
	return codexWorld{home: home, log: installFakeCodex(t), task: writeTask(t)}
}

// invocations is the number of times the fake native ran.
func (w codexWorld) invocations(t *testing.T) int {
	t.Helper()
	raw, err := os.ReadFile(w.log)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(strings.Fields(strings.ReplaceAll(string(raw), "--version", "v")))
}

// repoWith creates a clean repository committing files (path -> content).
func repoWith(t *testing.T, files map[string]string) string {
	t.Helper()
	repo := initRepo(t)
	if len(files) == 0 {
		return repo
	}
	for path, content := range files {
		full := filepath.Join(repo, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "files"}} {
		cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false"}, args...)...) // #nosec G204 -- fixed test argv only
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return repo
}

func (w codexWorld) request(repo, stateDir string) admission.Request {
	return admission.Request{
		StateDir:         stateDir,
		Repo:             repo,
		TaskFile:         w.task,
		Adapter:          admission.AdapterCodex,
		Billing:          admission.BillingSubscriptionOnly,
		ExecutionProfile: admission.ProfileTrustedHost,
		Env:              os.Environ(),
		NoChecks:         true,
	}
}

func requireBlock(t *testing.T, err error, code string) *admission.BlockedError {
	t.Helper()
	var blocked *admission.BlockedError
	if !errors.As(err, &blocked) || blocked.Code != code {
		t.Fatalf("err = %v, want a block with code %q", err, code)
	}
	return blocked
}

const hookConfig = `{"hooks":{"PreToolUse":[{"command":"never-run-this-hook"}]}}`

func TestCodexStrictKeyLevel(t *testing.T) { // I02, I15 (v2 §2), AC-4.1, AC-4.3, AC-4.5
	probe, manifest := fixtureProbe(), fixtureManifest()
	key := codex.ObservedKey(probe, manifest, nativeManaged, admission.ProfileTrustedHost)
	consult := func(t *testing.T, recs ...qualify.Record) admission.Eligibility {
		t.Helper()
		reg := openRegistry(t, t.TempDir())
		for _, rec := range recs {
			storeRecord(t, reg, rec)
		}
		elig, err := admission.ResolveQualificationKey(t.Context(), reg, key, admission.BillingSubscriptionOnly)
		if err != nil {
			t.Fatal(err)
		}
		return elig
	}

	noRegistry, err := admission.ResolveQualificationKey(t.Context(), nil, key, admission.BillingSubscriptionOnly)
	if err != nil {
		t.Fatal(err)
	}
	requireVerdict(t, noRegistry, admission.Blocked, "no_qualification_record")

	planned := liveRecord(key, liveEvidence(true), liveStop(true))
	planned.Progress = qualify.ProgressPlanned
	planned.NextTest = "fixture; authority: test"
	requireVerdict(t, consult(t, planned), admission.Blocked, "entitlement_not_proven")

	draft, err := codex.FixtureDraft(key, codex.FixtureEvidence{})
	if err != nil {
		t.Fatal(err)
	}
	fixtureElig := consult(t, draft)
	requireVerdict(t, fixtureElig, admission.Blocked, "entitlement_not_proven")

	noStop := liveRecord(key, liveEvidence(true), qualify.Capability{Value: "unknown", Evidence: "unknown", Scope: "unknown"})
	requireVerdict(t, consult(t, noStop), admission.Blocked, "stop_at_exhaustion_unproven")

	// AC-4.5: no quota datum at all still admits.
	admitting := liveRecord(key, liveEvidence(true), liveStop(true))
	admitting.Quota = qualify.Datum{}
	elig := consult(t, admitting)
	requireVerdict(t, elig, admission.Eligible, "")
	posture, err := admission.ResolveBillingStrict(t.Context(), elig)
	if err != nil {
		t.Fatalf("ResolveBillingStrict on the admitting record: %v", err)
	}
	if posture.Mode != admission.BillingSubscriptionOnly || !posture.Qualified || posture.PaidContinuation != "prevented" {
		t.Fatalf("posture = %+v, want a qualified subscription-only posture with paid continuation prevented", posture)
	}
	if _, err := admission.ResolveBillingStrict(t.Context(), fixtureElig); err == nil {
		t.Fatal("ResolveBillingStrict admitted a blocked eligibility")
	}
	if _, err := admission.ResolveBillingStrict(t.Context(), admission.Eligibility{Verdict: admission.Eligible, Record: &draft}); err == nil {
		t.Fatal("ResolveBillingStrict admitted an eligible verdict over a fixture-tested record")
	}
}

func TestPurchasedCreditPreventableNeedsUnexpiredPreventableEvidence(t *testing.T) { // I15 (v2 §2), AC-4.3
	now := time.Now()
	preventable := func(future bool, result string) qualify.Record {
		ev := liveEvidence(future)
		ev[0].Result = result
		return liveRecord(codex.ObservedKey(fixtureProbe(), fixtureManifest(), nativeManaged, admission.ProfileTrustedHost), ev, liveStop(true))
	}
	for _, tc := range []struct {
		name string
		rec  qualify.Record
		want bool
	}{
		{"unexpired preventable", preventable(true, "purchased_credit:preventable"), true},
		{"expired preventable", preventable(false, "purchased_credit:preventable"), false},
		{"unexpired but not preventable", preventable(true, "pass"), false},
		{"unexpired unknown", preventable(true, "purchased_credit:unknown"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := admission.PurchasedCreditPreventable(tc.rec, now); got != tc.want {
				t.Fatalf("PurchasedCreditPreventable = %t, want %t", got, tc.want)
			}
		})
	}
	// A fixture draft can never satisfy it.
	draft, err := codex.FixtureDraft(codex.ObservedKey(fixtureProbe(), fixtureManifest(), nativeManaged, admission.ProfileTrustedHost), codex.FixtureEvidence{})
	if err != nil {
		t.Fatal(err)
	}
	if admission.PurchasedCreditPreventable(draft, now) {
		t.Fatal("a fixture draft satisfied the purchased-credit check")
	}
}

func TestCodexKeyVariationBlocks(t *testing.T) { // I04 (v2 §2), AC-7.5, AC-4.7
	probe, manifest := fixtureProbe(), fixtureManifest()
	key := codex.ObservedKey(probe, manifest, nativeManaged, admission.ProfileTrustedHost)
	reg := openRegistry(t, t.TempDir())
	storeRecord(t, reg, liveRecord(key, liveEvidence(true), liveStop(true)))
	consult := func(k qualify.Key) admission.Eligibility {
		elig, err := admission.ResolveQualificationKey(t.Context(), reg, k, admission.BillingSubscriptionOnly)
		if err != nil {
			t.Fatal(err)
		}
		return elig
	}
	requireVerdict(t, consult(key), admission.Eligible, "")
	for name, vary := range map[string]func(*qualify.Key){
		"harness":           func(k *qualify.Key) { k.Harness = "opencode" },
		"provider endpoint": func(k *qualify.Key) { k.ProviderEndpoint = "first-party-subscription" },
		"auth category":     func(k *qualify.Key) { k.AuthCategory = "api-key" },
	} {
		t.Run(name, func(t *testing.T) {
			varied := key
			vary(&varied)
			requireVerdict(t, consult(varied), admission.Blocked, "no_qualification_record")
		})
	}
}

func TestCodexDecideOrder(t *testing.T) { // I02 (v2 §2), D11, D13, D15
	t.Run("unknown auth category blocks after the probe", func(t *testing.T) {
		w := newCodexWorld(t)
		_, err := admission.Decide(t.Context(), w.request(repoWith(t, nil), t.TempDir()))
		_ = requireBlock(t, err, "no_qualification_record")
		if n := w.invocations(t); n != 1 {
			t.Fatalf("fixture invocations = %d, want exactly the one version probe", n)
		}
	})
	t.Run("gap gate blocks first when the registry opens", func(t *testing.T) {
		w := newCodexWorld(t)
		stateDir := t.TempDir()
		openRegistry(t, stateDir)
		_, err := admission.DecideCodexWith(t.Context(), w.request(repoWith(t, nil), stateDir), nativeManaged)
		_ = requireBlock(t, err, "entitlement_not_proven")
	})
	t.Run("no registry skips the gap gate", func(t *testing.T) {
		w := newCodexWorld(t)
		_, err := admission.DecideCodexWith(t.Context(), w.request(repoWith(t, nil), missingStateDir(t)), nativeManaged)
		_ = requireBlock(t, err, "no_qualification_record")
	})
	for name, tc := range map[string]struct{ config, code string }{
		"passthrough": {"[environment]\npassthrough = [\"EXAMPLE_VAR\"]", "native_passthrough_unsupported"},
		"tool rules":  {"[adapters.claudecode]\nallowed_tools = [\"Read\"]", "native_tool_rules_unsupported"},
	} {
		t.Run(name+" refused before any fixture invocation", func(t *testing.T) {
			w := newCodexWorld(t)
			cfg := envelopesConfig(tc.config)
			_, digest, err := admission.ParseProjectConfig(cfg)
			if err != nil {
				t.Fatal(err)
			}
			req := w.request(repoWith(t, map[string]string{"mythhelm.toml": string(cfg)}), t.TempDir())
			req.NoChecks, req.TrustProjectConfig = false, "sha256:"+digest
			_, err = admission.DecideCodexWith(t.Context(), req, nativeManaged)
			_ = requireBlock(t, err, tc.code)
			if n := w.invocations(t); n != 0 {
				t.Fatalf("fixture invocations = %d, want 0 before the project rules are refused", n)
			}
		})
	}
}

func TestCodexUntrustedConfigBlocksBeforeProbe(t *testing.T) { // I01 (v2 §2), AC-3.1, AT-11
	w := newCodexWorld(t)
	repo := repoWith(t, map[string]string{".codex/config.json": hookConfig})
	_, err := admission.DecideCodexWith(t.Context(), w.request(repo, t.TempDir()), nativeManaged)
	blocked := requireBlock(t, err, "untrusted_native_config")
	if n := w.invocations(t); n != 0 {
		t.Fatalf("fixture invocations = %d, want 0 before native config is trusted", n)
	}

	// Control: with the exact digest trusted the run proceeds to the probe, so
	// the zero above is a real absence and not a broken counter.
	req := w.request(repo, missingStateDir(t))
	req.TrustNativeConfig = blocked.Field
	_, err = admission.DecideCodexWith(t.Context(), req, nativeManaged)
	_ = requireBlock(t, err, "no_qualification_record")
	if n := w.invocations(t); n != 1 {
		t.Fatalf("fixture invocations after trust = %d, want the one version probe", n)
	}
}

func TestCodexInventoryUsesChildEnvironment(t *testing.T) { // I01 (v2 §2), AC-3.2, AT-11, issue 389 F2
	t.Run("parent CODEX_HOME is not followed", func(t *testing.T) {
		w := newCodexWorld(t)
		elsewhere := t.TempDir()
		if err := os.WriteFile(filepath.Join(elsewhere, "config.json"), []byte(hookConfig), 0o600); err != nil {
			t.Fatal(err)
		}
		req := w.request(repoWith(t, nil), missingStateDir(t))
		req.Env = append(os.Environ(), "CODEX_HOME="+elsewhere)
		_, err := admission.DecideCodexWith(t.Context(), req, nativeManaged)
		_ = requireBlock(t, err, "no_qualification_record")
	})
	t.Run("home comes from the child HOME", func(t *testing.T) {
		w := newCodexWorld(t)
		childHome := t.TempDir()
		if err := os.MkdirAll(filepath.Join(childHome, ".codex"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(childHome, ".codex", "config.json"), []byte(hookConfig), 0o600); err != nil {
			t.Fatal(err)
		}
		req := w.request(repoWith(t, nil), t.TempDir())
		req.Env = append(os.Environ(), "HOME="+childHome)
		_, err := admission.DecideCodexWith(t.Context(), req, nativeManaged)
		_ = requireBlock(t, err, "untrusted_native_config")
	})
}

func TestCodexNonStrictBillingRefused(t *testing.T) { // I15 (v2 §2), D3
	for _, mode := range []string{admission.BillingSubscriptionDeclared, admission.BillingLocalScripted} {
		t.Run(mode, func(t *testing.T) {
			w := newCodexWorld(t)
			req := w.request(repoWith(t, nil), t.TempDir())
			req.Billing = mode
			_, err := admission.Decide(t.Context(), req)
			_ = requireBlock(t, err, "billing_unsupported_for_adapter")
			if n := w.invocations(t); n != 0 {
				t.Fatalf("fixture invocations = %d, want 0 for a refused billing mode", n)
			}
		})
	}
}

func TestNativeAdmissionErrorMapsCodexCapability(t *testing.T) { // AC-2.2
	err := admission.NativeAdmissionError(fmt.Errorf("probing adapter: %w: native_version_untested", codex.ErrCapability))
	blocked := requireBlock(t, err, "native_capability_unavailable")
	if !blocked.Capability {
		t.Fatal("codex capability refusal is not Capability (exit 7)")
	}
}

func TestCheckNativeTrustParity(t *testing.T) {
	digest := strings.Repeat("a", 64)
	for _, tc := range []struct {
		name     string
		manifest claudecode.Manifest
		repoID   string
		explicit string
	}{
		{"no trust needed", claudecode.Manifest{}, "repo", ""},
		{"explicit digest with nothing to trust", claudecode.Manifest{}, "repo", "sha256:" + digest},
		{"short digest", claudecode.Manifest{RequiresTrust: true, Digest: "abc"}, "repo", ""},
		{"no repo identity", claudecode.Manifest{RequiresTrust: true, Digest: digest}, "", ""},
		{"explicit mismatch", claudecode.Manifest{RequiresTrust: true, Digest: digest}, "repo", "sha256:" + strings.Repeat("b", 64)},
		{"explicit match", claudecode.Manifest{RequiresTrust: true, Digest: digest, Hooks: 1}, "repo", "sha256:" + digest},
		{"untrusted", claudecode.Manifest{RequiresTrust: true, Digest: digest, Hooks: 1}, "repo", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantOK, wantErr := admission.CheckNativeTrust(context.Background(), nil, tc.repoID, tc.manifest, tc.explicit)
			cfg := admission.NativeConfig{Digests: tc.manifest.Digests, Digest: tc.manifest.Digest, Hooks: tc.manifest.Hooks, MCPServers: tc.manifest.MCPServers, RequiresTrust: tc.manifest.RequiresTrust}
			gotOK, gotErr := admission.CheckNativeTrustConfig(context.Background(), nil, tc.repoID, cfg, tc.explicit)
			if gotOK != wantOK || fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
				t.Fatalf("CheckNativeTrustConfig = (%t, %v), CheckNativeTrust = (%t, %v)", gotOK, gotErr, wantOK, wantErr)
			}
		})
	}
}

func TestCodexBlockFieldsNameTheGate(t *testing.T) { // I02 (v2 §2), D11, D13
	w := newCodexWorld(t)
	repo := repoWith(t, nil)

	_, err := admission.Decide(t.Context(), w.request(repo, t.TempDir()))
	if f := requireBlock(t, err, "no_qualification_record").Field; !strings.Contains(f, "auth") {
		t.Fatalf("unknown-auth block Field = %q, want it to name the auth category", f)
	}

	stateDir := t.TempDir()
	openRegistry(t, stateDir)
	_, err = admission.DecideCodexWith(t.Context(), w.request(repo, stateDir), nativeManaged)
	if f := requireBlock(t, err, "entitlement_not_proven").Field; f != "missing-source:managed-policy" {
		t.Fatalf("gap-gate block Field = %q, want missing-source:managed-policy", f)
	}

	_, err = admission.DecideCodexWith(t.Context(), w.request(repo, missingStateDir(t)), nativeManaged)
	wantField := "codex × " + codex.New().Descriptor().Surface
	if f := requireBlock(t, err, "no_qualification_record").Field; f != wantField {
		t.Fatalf("consult block Field = %q, want %q", f, wantField)
	}
}

func TestCodexAdapterIDMatches(t *testing.T) {
	if got := "builtin/" + admission.AdapterCodex; got != codex.AdapterID {
		t.Fatalf("route %q derived from --adapter codex != codex.AdapterID %q", got, codex.AdapterID)
	}
	if got := admission.HarnessRoutes()[admission.AdapterCodex]; got != codex.AdapterID {
		t.Fatalf("admission table adapter ID %q != codex.AdapterID %q", got, codex.AdapterID)
	}
}
