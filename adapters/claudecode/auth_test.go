package claudecode_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/adapters/claudecode"
	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/security"
	"github.com/turbokast/mythhelm/internal/statedir"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

func fakeAuth(t *testing.T, org, config, method string) (claudecode.AuthEvidence, error) {
	t.Helper()
	if !filepath.IsAbs(config) {
		config = filepath.Join(os.TempDir(), "mythhelm-fake-auth", config)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(exe) // #nosec G304 -- Reads this test binary from os.Executable, never user input.
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(bytes)
	raw := fmt.Sprintf(`{"loggedIn":true,"authMethod":%q,"apiProvider":"firstParty","subscriptionType":"max","configDirectory":%q,"orgId":%q,"email":"planted-email","orgName":"planted-org-name"}`, method, config, org)
	env := []string{"GO_WANT_FAKECLAUDE=1", "GORACE=atexit_sleep_ms=0", "FAKE_AUTH=" + raw}
	return claudecode.AuthStatus(context.Background(), adapter.Probe{Executable: exe, SHA256: hex.EncodeToString(sum[:])}, env, t.TempDir())
}
func TestAuthStatusMismatchNeedsNativeSetup(t *testing.T) {
	t.Parallel()
	_, err := fakeAuth(t, "test-identity", "test-config", "console")
	var blocked *adapter.BlockedError
	if !errors.As(err, &blocked) || blocked.Code != "needs_native_setup" || !strings.Contains(err.Error(), "/login") {
		t.Fatalf("console fixture must require native setup, got %v", err)
	}
	good, err := fakeAuth(t, "test-identity", "test-config", "claude.ai")
	if err != nil || !good.LoggedIn || good.AuthMethod != "claude.ai" {
		t.Fatalf("native subscription fixture = %+v, %v", good, err)
	}
}
func TestDeclarationBoundToIdentity(t *testing.T) {
	t.Parallel()
	original, err := fakeAuth(t, "test-identity", "test-config", "claude.ai")
	if err != nil {
		t.Fatal(err)
	}
	decl := admission.Declaration{AdapterID: claudecode.AdapterID, IdentityRef: original.IdentityRef, PlanClass: "max", ExtraUsage: "disabled", DeclaredAt: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)}
	if _, err := admission.ResolveBilling(t.Context(), "subscription-declared", original, &decl, admission.Eligibility{}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, org, config string }{{"orgId", "different-identity", "test-config"}, {"configDirectory", "test-identity", "different-config"}} {
		t.Run(tc.name, func(t *testing.T) {
			changed, err := fakeAuth(t, tc.org, tc.config, "claude.ai")
			if err != nil {
				t.Fatal(err)
			}
			_, err = admission.ResolveBilling(t.Context(), "subscription-declared", changed, &decl, admission.Eligibility{})
			var blocked *admission.BlockedError
			if !errors.As(err, &blocked) || blocked.Code != "declaration_identity_mismatch" {
				t.Fatalf("old declaration admitted after %s changed: %v", tc.name, err)
			}
		})
	}
}
func TestAuthStatusPIIDropped(t *testing.T) {
	t.Parallel()
	rawOrg := "planted-raw-org-id"
	e, err := fakeAuth(t, rawOrg, "test-config", "claude.ai")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = statedir.Ensure(dir); err != nil {
		t.Fatal(err)
	}
	j, err := journal.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	runID := ids.New("run")
	producer := supervisor.NewProducer(ids.New("sup"), 1)
	if err = supervisor.CreateRun(t.Context(), j, journal.RunRow{RunID: runID, AdapterID: claudecode.AdapterID, SourceRepo: t.TempDir(), TaskSHA256: strings.Repeat("a", 64), BillingPosture: "subscription-declared", ExecutionProfile: "trusted-host"}, producer); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(admission.Record{Adapter: claudecode.New().Descriptor(), NativeAuth: &e})
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Append(t.Context(), journal.Event{SchemaVersion: 1, EventID: ids.New("evt"), RunID: runID, ProducerID: ids.New("sup"), ProducerSequence: 1, Generation: 1, ObservedAt: time.Now().UTC(), Type: "admission.decided", Payload: payload}, nil); err != nil {
		t.Fatal(err)
	}
	receipt, err := supervisor.BuildReceipt(t.Context(), j, runID)
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(dir, "runs", runID)
	if err = os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err = supervisor.WriteReceipt(runDir, receipt); err != nil {
		t.Fatal(err)
	}
	log, err := os.OpenFile(filepath.Join(runDir, "supervisor.log"), os.O_WRONLY|os.O_CREATE, 0o600) // #nosec G304 -- Private t.TempDir state fixture.
	if err != nil {
		t.Fatal(err)
	}
	slog.New(security.NewRedactingHandler(slog.NewJSONHandler(log, nil))).Info("admitted native auth", slog.String("identity_ref", e.IdentityRef))
	if err = log.Close(); err != nil {
		t.Fatal(err)
	}
	// Scan the real SQLite main/WAL files, receipt and log, not a synthetic JSON
	// serializer. A leak in either durable admission or receipt output fails.
	if err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		b, readErr := os.ReadFile(path) // #nosec G304 G122 -- Walks only private t.TempDir files created and owned by this test; no concurrent writer or symlink.
		if readErr != nil {
			return readErr
		}
		for _, planted := range []string{rawOrg, "planted-email", "planted-org-name"} {
			if strings.Contains(string(b), planted) {
				t.Errorf("PII persisted in %s: %s", filepath.Base(path), planted)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	billing := receipt["billing"].(map[string]any)
	status, ok := billing["native_auth"].(map[string]any)
	if !ok || status["identity_ref"] != e.IdentityRef || len(e.IdentityRef) != 64 {
		t.Fatalf("receipt must retain hashed account reference only: %+v", billing)
	}
	events, err := j.Events(t.Context(), runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(events[len(events)-1].Payload), e.IdentityRef) {
		t.Fatal("durable native status missing identity_ref")
	}
}

func TestAuthStatusRejectsUnknownOrMalformedEvidence(t *testing.T) {
	t.Parallel()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.ReadFile(exe) // #nosec G304 -- Reads this test binary from os.Executable, never user input.
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(binary)
	good := fmt.Sprintf(`{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty","subscriptionType":"max","configDirectory":%q,"orgId":"fixture-id"}`, t.TempDir())
	for name, raw := range map[string]string{
		"duplicate auth route":              strings.Replace(good, `"authMethod":"claude.ai"`, `"authMethod":"console","authMethod":"claude.ai"`, 1),
		"duplicate loggedIn":                strings.Replace(good, `"loggedIn":true`, `"loggedIn":false,"loggedIn":true`, 1),
		"case-folded auth route":            strings.Replace(good, `"authMethod":"claude.ai"`, `"authMethod":"console","AuthMethod":"claude.ai"`, 1),
		"case-folded loggedIn":              strings.Replace(good, `"loggedIn":true`, `"loggedIn":false,"LoggedIn":true`, 1),
		"case-folded subscription (long-s)": strings.Replace(good, `"subscriptionType":"max"`, `"subscriptionType":"planted","ſubscriptionType":"max"`, 1),
		"unknown subscription":              strings.Replace(good, `"subscriptionType":"max"`, `"subscriptionType":"max-planted-secret"`, 1),
		"relative config":                   strings.Replace(good, fmt.Sprintf(`"configDirectory":%q`, extractConfigDirectory(t, good)), `"configDirectory":"relative"`, 1),
		"wrong loggedIn type":               strings.Replace(good, `"loggedIn":true`, `"loggedIn":"true"`, 1),
		"deep unknown field":                strings.TrimSuffix(good, "}") + `,"unknown":` + strings.Repeat("[", 65) + `0` + strings.Repeat("]", 65) + `}`,
	} {
		t.Run(name, func(t *testing.T) {
			env := []string{"GO_WANT_FAKECLAUDE=1", "GORACE=atexit_sleep_ms=0", "FAKE_AUTH=" + raw}
			e, err := claudecode.AuthStatus(t.Context(), adapter.Probe{Executable: exe, SHA256: hex.EncodeToString(sum[:])}, env, t.TempDir())
			var block *adapter.BlockedError
			if !errors.As(err, &block) || block.Code != "needs_native_setup" || e.IdentityRef != "" {
				t.Fatalf("malformed native status accepted: %+v, %v", e, err)
			}
		})
	}
}
func extractConfigDirectory(t *testing.T, raw string) string {
	t.Helper()
	var value struct {
		ConfigDirectory string `json:"configDirectory"`
	}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return value.ConfigDirectory
}
