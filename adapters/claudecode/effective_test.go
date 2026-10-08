package claudecode_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/adapters/claudecode"
	"github.com/turbokast/mythhelm/internal/adapter"
)

type effectiveFixture struct {
	Synthetic bool                    `json:"synthetic"`
	Probe     adapter.Probe           `json:"probe"`
	Manifest  claudecode.Manifest     `json:"manifest"`
	Auth      claudecode.AuthEvidence `json:"auth"`
}

func loadEffective(t *testing.T, name string) effectiveFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "effective", name+".json")) // #nosec G304 -- Fixed testdata directory; name is a test-literal.
	if err != nil {
		t.Fatal(err)
	}
	var f effectiveFixture
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if !f.Synthetic {
		t.Fatalf("%s: fixture is not marked synthetic", name)
	}
	return f
}

func inventory(t *testing.T, name string) claudecode.EffectiveConfig {
	t.Helper()
	f := loadEffective(t, name)
	cfg, err := claudecode.InventoryEffective(f.Probe, f.Manifest, f.Auth)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// wantDefaultGaps is the gap list the production unresolved list yields on this platform.
func wantDefaultGaps() []string {
	gaps := []string{"managed-remote-cache"}
	if runtime.GOOS == "darwin" {
		gaps = append(gaps, "macos-mdm-policy")
	}
	return gaps
}

func TestInventoryEffectiveNone(t *testing.T) { // I09 (v2 §7.3): gaps are explicit unknowns
	t.Parallel()
	cfg := inventory(t, "none")
	if !slices.Equal(cfg.ManagedPolicy.Gaps, wantDefaultGaps()) {
		t.Fatalf("Gaps = %v, want %v", cfg.ManagedPolicy.Gaps, wantDefaultGaps())
	}
	if len(cfg.ManagedPolicy.Sources) != 0 {
		t.Fatalf("Sources = %v, want none inventoried", cfg.ManagedPolicy.Sources)
	}
	if cfg.ExtraUsage != "unknown" || cfg.PurchasedCredits != "unknown" {
		t.Fatalf("ExtraUsage=%q PurchasedCredits=%q, want unknown", cfg.ExtraUsage, cfg.PurchasedCredits)
	}
}

func TestInventoryEffectiveGapIsData(t *testing.T) { // I02 (v2 §7.3): unknown mandatory sources are listed
	t.Parallel()
	f := loadEffective(t, "file-gap")
	cfg, err := claudecode.InventoryEffective(f.Probe, f.Manifest, f.Auth)
	if err != nil {
		t.Fatalf("a gap must be data, not an error: %v", err)
	}
	if !slices.Contains(cfg.ManagedPolicy.Gaps, "managed-remote-cache") {
		t.Fatalf("Gaps = %v, want managed-remote-cache", cfg.ManagedPolicy.Gaps)
	}
	want := []string{"managed", "managed_fragment_6", "managed_mcp"}
	if !slices.Equal(cfg.ManagedPolicy.Sources, want) {
		t.Fatalf("Sources = %v, want %v", cfg.ManagedPolicy.Sources, want)
	}
}

func TestInventoryEffectiveFileOnly(t *testing.T) {
	t.Parallel()
	f := loadEffective(t, "file")
	cfg, err := claudecode.InventoryEffective(f.Probe, f.Manifest, f.Auth)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cfg.ManagedPolicy.Gaps, wantDefaultGaps()) {
		t.Fatalf("Gaps = %v, want only the default unresolved sources %v", cfg.ManagedPolicy.Gaps, wantDefaultGaps())
	}
	if !slices.Equal(cfg.ManagedPolicy.Sources, []string{"managed"}) {
		t.Fatalf("Sources = %v, want [managed]", cfg.ManagedPolicy.Sources)
	}
	encoded, err := json.Marshal(map[string]string{"managed": f.Manifest.Digests["managed"]})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encoded)
	if want := hex.EncodeToString(sum[:]); cfg.ManagedPolicy.Digest != want {
		t.Fatalf("Digest = %s, want recomputed %s", cfg.ManagedPolicy.Digest, want)
	}
	delete(f.Manifest.Digests, "managed")
	dropped, err := claudecode.InventoryEffective(f.Probe, f.Manifest, f.Auth)
	if err != nil {
		t.Fatal(err)
	}
	if dropped.ManagedPolicy.Digest == cfg.ManagedPolicy.Digest {
		t.Fatal("dropping a managed source did not change the digest")
	}
}

func TestInventoryPrecedenceAndChildren(t *testing.T) {
	t.Parallel()
	cfg := inventory(t, "none")
	if !slices.Equal(cfg.CredentialPrecedence, []string{"native-login"}) {
		t.Fatalf("CredentialPrecedence = %v, want [native-login]", cfg.CredentialPrecedence)
	}
	if len(cfg.ChildrenRoutes) != 0 {
		t.Fatalf("ChildrenRoutes = %v, want none from static inputs", cfg.ChildrenRoutes)
	}
}

func TestInventoryRefusesUnknownAuthMethod(t *testing.T) { // I02 (v2 §7.3)
	t.Parallel()
	f := loadEffective(t, "none")
	f.Auth.AuthMethod = "console"
	cfg, err := claudecode.InventoryEffective(f.Probe, f.Manifest, f.Auth)
	if !errors.Is(err, claudecode.ErrCapability) || len(cfg.CredentialPrecedence) != 0 {
		t.Fatalf("got %+v, %v; want ErrCapability and no precedence", cfg, err)
	}
}

func TestGapSourcesRules(t *testing.T) {
	t.Parallel()
	mdm := []string{}
	if runtime.GOOS == "darwin" {
		mdm = []string{"macos-mdm-policy"}
	}
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"both", []string{"managed-remote-cache", "macos-mdm-policy"}, append([]string{"managed-remote-cache"}, mdm...)},
		{"reversed", []string{"macos-mdm-policy", "managed-remote-cache"}, append(slices.Clone(mdm), "managed-remote-cache")},
		{"remote only", []string{"managed-remote-cache"}, []string{"managed-remote-cache"}},
		{"mdm only", []string{"macos-mdm-policy"}, mdm},
		{"plugins are never a gap", []string{"plugins"}, []string{}},
		{"empty", nil, []string{}},
	}
	for _, tc := range cases {
		got := claudecode.GapSources(tc.in)
		if len(got) == 0 && len(tc.want) == 0 {
			continue
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: GapSources(%v) = %v, want %v", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestUnresolvedSourcesDefault(t *testing.T) {
	t.Parallel()
	if want := []string{"managed-remote-cache", "macos-mdm-policy"}; !slices.Equal(claudecode.UnresolvedSources, want) {
		t.Fatalf("UnresolvedSources = %v, want %v", claudecode.UnresolvedSources, want)
	}
}

func TestInventoryReadsNoSecrets(t *testing.T) { // I19 (v2 §7.1): no secret reads
	f := loadEffective(t, "file-gap")
	run := func() string {
		cfg, err := claudecode.InventoryEffective(f.Probe, f.Manifest, f.Auth)
		if err != nil {
			t.Fatal(err)
		}
		out, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	baseline := run()
	home := t.TempDir()
	const planted = "planted-synthetic-secret"
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", ".credentials.json"), []byte(`{"token":"`+planted+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"} {
		t.Setenv(name, "http://192.0.2.1:9")
	}
	poisoned := run()
	if poisoned != baseline {
		t.Fatalf("output changed with HOME and proxies poisoned:\n%s\n%s", baseline, poisoned)
	}
	if strings.Contains(poisoned, planted) {
		t.Fatal("output carries planted credential content")
	}
}

func TestInventoryRefusesBadProbe(t *testing.T) {
	t.Parallel()
	f := loadEffective(t, "none")
	f.Probe.Executable = "claude"
	cfg, err := claudecode.InventoryEffective(f.Probe, f.Manifest, f.Auth)
	if !errors.Is(err, claudecode.ErrCapability) {
		t.Fatalf("err = %v, want ErrCapability", err)
	}
	if len(cfg.CredentialPrecedence) != 0 || len(cfg.ManagedPolicy.Gaps) != 0 {
		t.Fatalf("config returned with the refusal: %+v", cfg)
	}
}

func TestInventoryLiveSignalsAlwaysUnknown(t *testing.T) { // I09 (v2 §7.3)
	t.Parallel()
	for _, name := range []string{"none", "file", "file-gap"} {
		cfg := inventory(t, name)
		if cfg.ExtraUsage != "unknown" || cfg.PurchasedCredits != "unknown" {
			t.Errorf("%s: ExtraUsage=%q PurchasedCredits=%q, want unknown", name, cfg.ExtraUsage, cfg.PurchasedCredits)
		}
	}
}
