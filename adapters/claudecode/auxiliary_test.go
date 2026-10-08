package claudecode

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/qualify"
)

func routeNames(routes []AuxiliaryRoute) []string {
	names := make([]string, 0, len(routes))
	for _, r := range routes {
		names = append(names, r.Name)
	}
	return names
}

func TestAuxiliaryCoversAT03(t *testing.T) {
	m := Manifest{
		EnabledPlugins: []string{"alpha", "beta"},
		MCPServers:     []string{"user:files", "project:search"},
	}
	routes := InventoryAuxiliary(m, adapter.SessionStarted{PluginCount: 2})
	got := routeNames(routes)
	want := append(slices.Clone(at03Routes), "plugin:alpha", "plugin:beta", "mcp:user:files", "mcp:project:search")
	if !slices.Equal(got, want) {
		t.Fatalf("routes = %v, want %v", got, want)
	}
	for _, r := range routes {
		if r.Funding != "unknown" {
			t.Errorf("route %q funding = %q, want unknown", r.Name, r.Funding)
		}
	}
	surplus := InventoryAuxiliary(Manifest{EnabledPlugins: []string{"alpha"}}, adapter.SessionStarted{PluginCount: 4})
	var found []AuxiliaryRoute
	for _, r := range surplus {
		if r.Name == "plugin:unknown" {
			found = append(found, r)
		}
	}
	if len(found) != 1 || found[0].Evidence != "plugin-count:3" || found[0].Funding != "unknown" {
		t.Fatalf("surplus routes = %+v, want one plugin:unknown with plugin-count:3", found)
	}
}

// I02 (v2 §2): a session-reported MCP server no manifest entry names is an
// unassessed route, compared by identity rather than count.
func TestAuxiliaryFlagsSessionMCPAbsentFromManifest(t *testing.T) {
	t.Parallel()
	m := Manifest{MCPServers: []string{"user:files", "project:search"}}
	cases := []struct {
		name    string
		session []adapter.MCPServer
		want    []string
	}{
		{name: "all session servers named by the manifest", session: []adapter.MCPServer{{Name: "files"}, {Name: "search"}}},
		{name: "unnamed server beside named ones", session: []adapter.MCPServer{{Name: "files"}, {Name: "rogue"}}, want: []string{"mcp-session:rogue"}},
		{name: "same count different identity", session: []adapter.MCPServer{{Name: "files"}, {Name: "other"}}, want: []string{"mcp-session:other"}},
		{name: "suffix is not identity", session: []adapter.MCPServer{{Name: "iles"}}, want: []string{"mcp-session:iles"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got []string
			for _, r := range InventoryAuxiliary(m, adapter.SessionStarted{MCPServers: tc.session}) {
				if r.Name != "mcp:unknown" {
					continue
				}
				if r.Funding != "unknown" {
					t.Errorf("route %+v funding, want unknown", r)
				}
				got = append(got, r.Evidence)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("mcp:unknown evidence = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAT03NamesMatchFixtureInventory(t *testing.T) {
	var names []string
	for _, r := range embeddedRoutes.Routes {
		names = append(names, r.Route)
	}
	if !slices.Equal(names, at03Routes) {
		t.Fatalf("fixture inventory routes %v differ from test list %v", names, at03Routes)
	}
	if got := routeNames(InventoryAuxiliary(Manifest{}, adapter.SessionStarted{})); !slices.Equal(got, names) {
		t.Fatalf("bare inventory = %v, want the fixture AT-03 routes %v", got, names)
	}
}

func TestAuxiliaryBareInstallEmitsOnlyAT03(t *testing.T) {
	routes := InventoryAuxiliary(Manifest{EnabledPlugins: []string{}, MCPServers: nil}, adapter.SessionStarted{})
	if len(routes) != len(at03Routes) {
		t.Fatalf("bare install routes = %v, want exactly the %d AT-03 routes", routeNames(routes), len(at03Routes))
	}
	for _, r := range routes {
		if strings.HasPrefix(r.Name, "plugin:") || strings.HasPrefix(r.Name, "mcp:") {
			t.Errorf("bare install emitted route %q", r.Name)
		}
	}
}

type liveShapes struct {
	Synthetic bool `json:"synthetic"`
	Cases     []struct {
		Name             string   `json:"name"`
		ExtraUsage       string   `json:"extra_usage"`
		PurchasedCredits string   `json:"purchased_credits"`
		Funding          string   `json:"funding"`
		Gaps             []string `json:"gaps"`
		KeyMutation      string   `json:"key_mutation"`
		Expect           string   `json:"expect"`
	} `json:"cases"`
}

func loadLiveShapes(t *testing.T) liveShapes {
	t.Helper()
	raw, err := os.ReadFile("testdata/qualification/live-shapes.json")
	if err != nil {
		t.Fatal(err)
	}
	var s liveShapes
	if err = json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	if !s.Synthetic {
		t.Fatal("live-shapes.json is not marked synthetic")
	}
	return s
}

func TestLiveShapesCoverVariants(t *testing.T) {
	have := map[string]bool{}
	for _, c := range loadLiveShapes(t).Cases {
		have[c.Name] = true
	}
	for _, name := range []string{
		"proven-record", "drifted-executable", "drifted-config",
		"purchased-non-consumable", "purchased-plan-granted",
		"purchased-separately-purchased", "purchased-unknown",
	} {
		if !have[name] {
			t.Errorf("live-shapes.json lacks case %q", name)
		}
	}
}

func TestLiveShapesDriveLiveRecord(t *testing.T) {
	for _, c := range loadLiveShapes(t).Cases {
		t.Run(c.Name, func(t *testing.T) {
			key := firstRouteFixtureKey()
			switch c.KeyMutation {
			case "executable-digest":
				key.ExecutableDigest = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
			case "config-digest":
				key.ConfigDigest = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
			}
			cfg := assessedConfig()
			cfg.ManagedPolicy.Gaps = c.Gaps
			cfg.ExtraUsage = c.ExtraUsage
			cfg.PurchasedCredits = c.PurchasedCredits
			rec, err := LiveRecord(key, cfg, assessedRoutes(c.Funding))
			switch c.Expect {
			case "live-qualified":
				if err != nil || rec.Progress != qualify.ProgressLiveQualified {
					t.Fatalf("LiveRecord = %v, %v; want live-qualified", rec.Progress, err)
				}
			default:
				if !errors.Is(err, ErrEvidenceIncomplete) || !reflect.DeepEqual(rec, qualify.Record{}) {
					t.Fatalf("LiveRecord = %+v, %v; want ErrEvidenceIncomplete and no record", rec, err)
				}
			}
		})
	}
}

func assessedConfig() EffectiveConfig {
	// #nosec G101 -- Synthetic fixture: PurchasedCredits is account state, not a credential.
	return EffectiveConfig{
		CredentialPrecedence: []string{"native-login"},
		ManagedPolicy:        PolicySummary{Digest: "d", Sources: []string{}, Gaps: []string{}},
		ChildrenRoutes:       []string{},
		ExtraUsage:           "disabled",
		PurchasedCredits:     "non-consumable",
	}
}

func assessedRoutes(funding string) []AuxiliaryRoute {
	routes := InventoryAuxiliary(Manifest{}, adapter.SessionStarted{})
	for i := range routes {
		routes[i].Funding = funding
		routes[i].Evidence = "ev_live_" + routes[i].Name
	}
	return routes
}

func TestLiveRecordRefusesIncomplete(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*EffectiveConfig, *[]AuxiliaryRoute)
	}{
		{"managed-policy gap", func(c *EffectiveConfig, _ *[]AuxiliaryRoute) {
			c.ManagedPolicy.Gaps = []string{"managed-remote-cache"}
		}},
		{"unknown funding", func(_ *EffectiveConfig, a *[]AuxiliaryRoute) { (*a)[2].Funding = "unknown" }},
		{"missing AT-03 route", func(_ *EffectiveConfig, a *[]AuxiliaryRoute) { *a = (*a)[1:] }},
		{"extra usage enabled", func(c *EffectiveConfig, _ *[]AuxiliaryRoute) { c.ExtraUsage = "enabled" }},
		{"extra usage unknown", func(c *EffectiveConfig, _ *[]AuxiliaryRoute) { c.ExtraUsage = "unknown" }},
		{"separately purchased credits", func(c *EffectiveConfig, _ *[]AuxiliaryRoute) { c.PurchasedCredits = "separately-purchased" }},
		{"unknown purchased credits", func(c *EffectiveConfig, _ *[]AuxiliaryRoute) { c.PurchasedCredits = "unknown" }},
		{"no credential precedence", func(c *EffectiveConfig, _ *[]AuxiliaryRoute) { c.CredentialPrecedence = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, routes := assessedConfig(), assessedRoutes("included")
			tc.mutate(&cfg, &routes)
			rec, err := LiveRecord(firstRouteFixtureKey(), cfg, routes)
			if !errors.Is(err, ErrEvidenceIncomplete) {
				t.Fatalf("LiveRecord error %v, want ErrEvidenceIncomplete", err)
			}
			if !reflect.DeepEqual(rec, qualify.Record{}) {
				t.Fatalf("LiveRecord returned a record: %+v", rec)
			}
		})
	}
}

func TestLiveRecordRefusesOtherKeys(t *testing.T) {
	base := firstRouteFixtureKey()
	mutations := []func(*qualify.Key){
		func(*qualify.Key) {},
		func(k *qualify.Key) { k.Harness = "codex" },
		func(k *qualify.Key) { k.Surface = "native-cli-structured (print, text)" },
		func(k *qualify.Key) { k.Harness, k.Surface = "", "" },
		func(k *qualify.Key) { k.ExecutableDigest = "sha256:2222" },
		func(k *qualify.Key) { k.ConfigDigest = "sha256:3333" },
	}
	for i, mutate := range mutations {
		key := base
		mutate(&key)
		_, draftErr := RecordDraft(key)
		rec, liveErr := LiveRecord(key, assessedConfig(), assessedRoutes("included"))
		if (draftErr == nil) != (liveErr == nil) {
			t.Fatalf("mutation %d: RecordDraft err %v, LiveRecord err %v disagree", i, draftErr, liveErr)
		}
		if draftErr != nil {
			if !errors.Is(liveErr, ErrNotFirstRoute) {
				t.Fatalf("mutation %d: LiveRecord error %v, want ErrNotFirstRoute", i, liveErr)
			}
			if !reflect.DeepEqual(rec, qualify.Record{}) {
				t.Fatalf("mutation %d: LiveRecord returned a record for a refused key", i)
			}
		}
	}
}

func TestLiveRecordSuccessPath(t *testing.T) {
	rec, err := LiveRecord(firstRouteFixtureKey(), assessedConfig(), assessedRoutes("included"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Progress != qualify.ProgressLiveQualified {
		t.Errorf("progress = %q, want live-qualified", rec.Progress)
	}
	if rec.Entitlement.Verdict != qualify.Proven || len(rec.Entitlement.Evidence) != 1 {
		t.Fatalf("entitlement = %+v, want proven with one evidence entry", rec.Entitlement)
	}
	ev := rec.Entitlement.Evidence[0]
	if ev.Method != "authorised-live" || ev.Label != qualify.Observed || ev.Source != "live-qualify" || ev.At.IsZero() {
		t.Errorf("evidence = %+v, want observed authorised-live from live-qualify with a timestamp", ev)
	}
	for _, dim := range []string{"credential-precedence", "managed-policy", "extra-usage", "purchased-credits"} {
		c, ok := rec.Capabilities[dim]
		if !ok || c.Value != "supported" || c.Evidence != ev.ID {
			t.Errorf("capability %q = %+v, want supported by %q", dim, c, ev.ID)
		}
	}
	stop := rec.Capabilities["stop_at_exhaustion"]
	if stop.Value != "supported" || stop.Evidence != "documented-mechanism" || stop.Expiry != nil {
		t.Errorf("stop_at_exhaustion = %+v, want supported by documented-mechanism", stop)
	}
	if rec.Quota.Label != qualify.DatumUnknown || rec.Quota.Quantity != "unknown" {
		t.Errorf("quota = %+v, want unknown", rec.Quota)
	}
	digest, err := qualify.CanonicalDigest(rec)
	if err != nil || rec.Digest != digest {
		t.Errorf("digest = %q, canonical %q (%v)", rec.Digest, digest, err)
	}
}

func TestLiveRecordAcceptsPlanGranted(t *testing.T) {
	cfg := assessedConfig()
	cfg.PurchasedCredits = "plan-granted"
	rec, err := LiveRecord(firstRouteFixtureKey(), cfg, assessedRoutes("included"))
	if err != nil || rec.Progress != qualify.ProgressLiveQualified || rec.Entitlement.Verdict != qualify.Proven {
		t.Fatalf("LiveRecord = %+v, %v; want a live-qualified proven record", rec, err)
	}
}

func TestCompatibilityLiveSection(t *testing.T) {
	section := liveSection(t, "COMPATIBILITY.md")
	for _, token := range []string{"authorised-live", "Task 7"} {
		if !strings.Contains(section, token) {
			t.Errorf("## Live record section names no %q", token)
		}
	}
	raw, err := os.ReadFile("COMPATIBILITY.md")
	if err != nil {
		t.Fatal(err)
	}
	// Red variant: the file without the section fails the scoped match.
	stripped := strings.Replace(string(raw), "## Live record", "## Renamed", 1)
	if section := extractSection(stripped, "## Live record"); section != "" {
		t.Fatal("extractSection matched a removed heading")
	}
}

func liveSection(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- Test-literal path to the adapter's own COMPATIBILITY.md.
	if err != nil {
		t.Fatal(err)
	}
	section := extractSection(string(raw), "## Live record")
	if section == "" {
		t.Fatalf("%s has no ## Live record section", path)
	}
	return section
}

// extractSection returns the body under heading up to the next "## " heading.
func extractSection(doc, heading string) string {
	_, rest, found := strings.Cut(doc, heading+"\n")
	if !found {
		return ""
	}
	body, _, _ := strings.Cut(rest, "\n## ")
	return body
}

// observationScan is what the live entitlement leg learns from a session's
// observation stream. It lives here, untagged, so the scan runs without the
// grant.
type observationScan struct {
	started                             adapter.SessionStarted
	result                              adapter.Result
	sawStarted, sawResult, sawRateLimit bool
}

func scanObservations(obs <-chan adapter.Observation) observationScan {
	var scan observationScan
	for ob := range obs {
		switch ob := ob.(type) {
		case adapter.SessionStarted:
			scan.started = ob
			scan.sawStarted = true
		case adapter.Result:
			scan.result = ob
			scan.sawResult = true
		case adapter.NativeError:
			scan.sawRateLimit = scan.sawRateLimit || ob.Class == "rate_limit"
		}
	}
	return scan
}

func TestScanObservations(t *testing.T) {
	t.Parallel()
	started := adapter.SessionStarted{SessionID: "s1", PluginCount: 2}
	tests := []struct {
		name string
		obs  []adapter.Observation
		want observationScan
	}{
		{
			name: "session start with result is a completed run",
			obs:  []adapter.Observation{started, adapter.Result{}},
			want: observationScan{started: started, sawStarted: true, sawResult: true},
		},
		{
			name: "error result is retained so the completion gate can reject it",
			obs:  []adapter.Observation{started, adapter.Result{IsError: true, Subtype: "error_during_execution"}},
			want: observationScan{
				started: started, sawStarted: true, sawResult: true,
				result: adapter.Result{IsError: true, Subtype: "error_during_execution"},
			},
		},
		{
			name: "missing session start leaves the inventory unassessed",
			obs:  []adapter.Observation{adapter.Result{}},
			want: observationScan{sawResult: true},
		},
		{
			name: "rate-limit error is recorded alongside a result",
			obs:  []adapter.Observation{started, adapter.NativeError{Class: "rate_limit"}, adapter.Result{}},
			want: observationScan{started: started, sawStarted: true, sawResult: true, sawRateLimit: true},
		},
		{
			name: "other error class is not a rate limit",
			obs:  []adapter.Observation{started, adapter.NativeError{Class: "auth"}},
			want: observationScan{started: started, sawStarted: true},
		},
		{
			name: "later error does not clear an earlier rate limit",
			obs:  []adapter.Observation{adapter.NativeError{Class: "rate_limit"}, adapter.NativeError{Class: "auth"}},
			want: observationScan{sawRateLimit: true},
		},
		{
			name: "empty stream observes nothing",
			want: observationScan{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ch := make(chan adapter.Observation, len(tt.obs))
			for _, ob := range tt.obs {
				ch <- ob
			}
			close(ch)
			if got := scanObservations(ch); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("scan = %+v, want %+v", got, tt.want)
			}
		})
	}
}
