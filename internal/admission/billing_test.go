package admission_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/adapters/claudecode"
	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/qualify"
	"github.com/turbokast/mythhelm/internal/statedir"
)

func evidence(org, dir string) admission.AuthEvidence {
	sum := sha256.Sum256([]byte(org + "\x00" + dir))
	return admission.AuthEvidence{LoggedIn: true, AuthMethod: "claude.ai", APIProvider: "firstParty", SubscriptionType: "max", ConfigDirectory: dir, IdentityRef: hex.EncodeToString(sum[:])}
}
func declaration(e admission.AuthEvidence) *admission.Declaration {
	return &admission.Declaration{AdapterID: claudecode.AdapterID, PlanClass: "max", ExtraUsage: "disabled", IdentityRef: e.IdentityRef, DeclaredAt: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)}
}
func blocked(t *testing.T, err error, code string) {
	t.Helper()
	var b *admission.BlockedError
	if !errors.As(err, &b) || b.Code != code {
		t.Fatalf("want admission block %s, got %v", code, err)
	}
}

func TestResolveBillingStrictDefenseInDepth(t *testing.T) {
	t.Parallel()
	e := evidence("test-identity", "test-config")
	live := liveRecord(qualify.Key{}, liveEvidence(true), liveStop(true))
	notLive := live
	notLive.Progress = qualify.ProgressFixtureTested
	unproven := live
	unproven.Entitlement.Verdict = qualify.NotProven
	expiredEvidence := liveRecord(qualify.Key{}, liveEvidence(false), liveStop(true))
	expiredStop := liveRecord(qualify.Key{}, liveEvidence(true), liveStop(false))
	noStop := live
	noStop.Capabilities = nil
	for _, tc := range []struct {
		name string
		elig admission.Eligibility
	}{
		{"zero eligibility", admission.Eligibility{}},
		{"blocked verdict", admission.Eligibility{Verdict: admission.Blocked, Reason: "no_qualification_record"}},
		{"unsupported verdict", admission.Eligibility{Verdict: admission.Unsupported}},
		{"eligible without a record", admission.Eligibility{Verdict: admission.Eligible}},
		{"eligible on a record that is not live-qualified", admission.Eligibility{Verdict: admission.Eligible, Record: &notLive}},
		{"eligible on an unproven entitlement", admission.Eligibility{Verdict: admission.Eligible, Record: &unproven}},
		{"eligible after the entitlement evidence expired", admission.Eligibility{Verdict: admission.Eligible, Record: &expiredEvidence}},
		{"eligible after the stop capability expired", admission.Eligibility{Verdict: admission.Eligible, Record: &expiredStop}},
		{"eligible without a stop capability", admission.Eligibility{Verdict: admission.Eligible, Record: &noStop}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := admission.ResolveBilling(context.Background(), "subscription-only", e, declaration(e), tc.elig)
			blocked(t, err, "entitlement_qualification_unavailable")
		})
	}
	if _, err := admission.ResolveBilling(context.Background(), "subscription-only", e, nil, admission.Eligibility{Verdict: admission.Eligible, Record: &live}); err != nil {
		t.Fatalf("eligible live-qualified proven record blocked: %v", err)
	}
}
func TestDeclaredLabelsUnchanged(t *testing.T) {
	t.Parallel()
	e := evidence("test-identity", "test-config")
	live := liveRecord(qualify.Key{}, liveEvidence(true), liveStop(true))
	for _, elig := range []admission.Eligibility{{}, {Verdict: admission.Eligible}, {Verdict: admission.Eligible, Record: &live}} {
		p, err := admission.ResolveBilling(context.Background(), "subscription-declared", e, declaration(e), elig)
		if err != nil {
			t.Fatal(err)
		}
		if p.Qualified || p.PaidContinuation != "unknown" || p.G05 != "not-passed" || p.PaidContinuationUserDeclaration != "disabled" {
			t.Fatalf("declared posture with eligibility %+v = %+v, want the unverified labels", elig, p)
		}
	}
}
func TestDeclaredPostureLabelledUnqualified(t *testing.T) {
	t.Parallel()
	e := evidence("test-identity", "test-config")
	p, err := admission.ResolveBilling(context.Background(), "subscription-declared", e, declaration(e), admission.Eligibility{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != "subscription-declared" || p.Qualified || p.PaidContinuation != "unknown" || p.G05 != "not-passed" || p.PaidContinuationUserDeclaration != "disabled" || p.EntitlementClass != "included-plan" {
		t.Fatalf("dishonest or missing posture: %+v", p)
	}
	_, err = admission.ResolveBilling(context.Background(), "subscription-declared", e, nil, admission.Eligibility{})
	blocked(t, err, "entitlement_declaration_required")
}
func TestAPIKeyEnvBlocksNamesOnly(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY", "ANTHROPIC_BASE_URL", "CLAUDE_CODE_OAUTH_TOKEN"} {
		t.Run(name, func(t *testing.T) {
			_, _, err := admission.ResolveCredentialEnv([]string{"PATH=/fixture", name + "=planted-secret"}, false, false, nil)
			blocked(t, err, "credential_route_override")
			if !strings.Contains(err.Error(), name) || strings.Contains(err.Error(), "planted-secret") {
				t.Fatalf("override message must name only variable: %v", err)
			}
		})
	}
}
func TestStripCredentialEnvRecordedAsOverride(t *testing.T) {
	t.Parallel()
	env, overrides, err := admission.ResolveCredentialEnv([]string{"HOME=/fixture", "ANTHROPIC_API_KEY=planted-secret"}, true, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(env, " ") != "HOME=/fixture" || len(overrides) != 1 || overrides[0].Name != "ANTHROPIC_API_KEY" || overrides[0].Kind != "env_remove" || overrides[0].Value != "" {
		t.Fatalf("stripped child and override = %v, %+v", env, overrides)
	}
}
func TestCredentialEnvPassthroughKeepsTrustedNames(t *testing.T) {
	t.Parallel()
	env, _, err := admission.ResolveCredentialEnv(
		[]string{"HOME=/fixture", "GOPATH=/go", "ANTHROPIC_API_KEY=planted-secret"},
		true, false, []string{"GOPATH"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(env, " ")
	if !strings.Contains(joined, "GOPATH=/go") || strings.Contains(joined, "planted-secret") {
		t.Fatalf("passthrough child = %v", env)
	}
	if _, _, err := admission.ResolveCredentialEnv([]string{"HOME=/fixture"}, true, false, []string{"ANTHROPIC_API_KEY"}); err == nil {
		t.Fatal("passthrough must not restore a denied credential route")
	}
}
func TestBillingMismatchNeedsNativeSetup(t *testing.T) {
	t.Parallel()
	e := evidence("test-identity", "test-config")
	e.AuthMethod = "console"
	_, err := admission.ResolveBilling(context.Background(), "subscription-declared", e, declaration(e), admission.Eligibility{})
	blocked(t, err, "needs_native_setup")
	if !strings.Contains(err.Error(), "/login") {
		t.Fatalf("missing native setup action: %v", err)
	}
}

func newBillingJournal(t *testing.T) *journal.Journal {
	t.Helper()
	dir := t.TempDir()
	if err := statedir.Ensure(dir); err != nil {
		t.Fatal(err)
	}
	j, err := journal.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return j
}
func appendDeclaration(t *testing.T, j *journal.Journal, d admission.Declaration, sequence int64) {
	t.Helper()
	payload, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	ev := journal.Event{SchemaVersion: 1, EventID: ids.New("evt"), RunID: "run_test_declaration", ProducerID: "declaration-test", ProducerSequence: sequence, Generation: 1, ObservedAt: d.DeclaredAt, Type: "entitlement.declared", Payload: payload}
	if err := j.Append(context.Background(), ev, func(tx *sql.Tx) error { return journal.InsertDeclaration(context.Background(), tx, d) }); err != nil {
		t.Fatal(err)
	}
}
func TestSingleCurrentDeclaration(t *testing.T) {
	t.Parallel()
	j := newBillingJournal(t)
	e := evidence("test-identity", "test-config")
	d := declaration(e)
	appendDeclaration(t, j, *d, 1)
	d.PlanClass = "pro"
	d.DeclaredAt = d.DeclaredAt.Add(time.Second)
	appendDeclaration(t, j, *d, 2)
	current, err := j.CurrentDeclaration(context.Background(), claudecode.AdapterID, e.IdentityRef)
	if err != nil {
		t.Fatal(err)
	}
	if current.PlanClass != "pro" || !current.DeclaredAt.Equal(d.DeclaredAt) {
		t.Fatalf("declaration did not supersede: %+v", current)
	}
	p := filepath.ToSlash(filepath.Join(j.StateDir(), journal.DBName))
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	uri := (&url.URL{Scheme: "file", Path: p}).String()
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM declarations WHERE adapter_id=? AND identity_ref=? AND superseded_at IS NULL`, claudecode.AdapterID, e.IdentityRef).Scan(&count); err != nil || count != 1 {
		t.Fatalf("current declaration count=%d, %v", count, err)
	}
	if _, err = db.Exec(`INSERT INTO declarations (adapter_id,plan_class,extra_usage,identity_ref,declared_at) VALUES (?,?,?,?,?)`, claudecode.AdapterID, "max", "disabled", e.IdentityRef, d.DeclaredAt.Format(time.RFC3339Nano)); err == nil || !strings.Contains(err.Error(), "UNIQUE constraint failed") {
		t.Fatalf("direct competing current insert must violate declarations_current, got %v", err)
	}
}
func writeNative(t *testing.T, root, relative, raw string) {
	t.Helper()
	p := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}
func TestProjectHooksRequireNativeTrust(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	home, workspace := t.TempDir(), t.TempDir()
	writeNative(t, workspace, filepath.Join(".claude", "settings.json"), `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"never-run-this-hook"}]}]}}`)
	manifest, err := claudecode.InventorySettings(home, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Hooks != 1 || !manifest.RequiresTrust || manifest.Digest == "" {
		t.Fatalf("missing hook inventory: %+v", manifest)
	}
	_, err = admission.CheckNativeTrust(context.Background(), nil, workspace, manifest, "")
	blocked(t, err, "untrusted_native_config")
	granted, err := admission.CheckNativeTrust(context.Background(), nil, workspace, manifest, "sha256:"+manifest.Digest)
	if err != nil || !granted {
		t.Fatalf("exact explicit native trust failed: %t, %v", granted, err)
	}
	writeNative(t, workspace, filepath.Join(".claude", "settings.json"), `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"changed-hook"}]}]}}`)
	changed, err := claudecode.InventorySettings(home, workspace)
	if err != nil {
		t.Fatal(err)
	}
	_, err = admission.CheckNativeTrust(context.Background(), nil, workspace, changed, "sha256:"+manifest.Digest)
	if !errors.Is(err, admission.ErrInvalid) {
		t.Fatalf("stale grant must not trust changed hooks, got %v", err)
	}
}
func TestUserScopeMCPRequiresNativeTrust(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	for _, scope := range []string{"user", "project"} {
		t.Run(scope, func(t *testing.T) {
			home, workspace := t.TempDir(), t.TempDir()
			var raw string
			if scope == "user" {
				raw = `{"mcpServers":{"fixture-mcp":{"command":"never-execute"}}}`
			} else {
				raw = fmt.Sprintf(`{"projects":{%q:{"mcpServers":{"fixture-mcp":{"command":"never-execute"}}}}}`, workspace)
			}
			writeNative(t, home, ".claude.json", raw)
			manifest, err := claudecode.InventorySettings(home, workspace)
			if err != nil {
				t.Fatal(err)
			}
			if len(manifest.MCPServers) != 1 || !manifest.RequiresTrust {
				t.Fatalf("MCP source omitted: %+v", manifest)
			}
			_, err = admission.CheckNativeTrust(context.Background(), nil, workspace, manifest, "")
			blocked(t, err, "untrusted_native_config")
		})
	}
}
