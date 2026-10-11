package codex

import (
	"errors"
	"reflect"
	"testing"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/qualify"
)

func testProbe() adapter.Probe {
	return adapter.Probe{
		Executable: "/opt/codex/bin/codex",
		Version:    fixtureVersion,
		SHA256:     "abc123",
		OS:         "linux",
		Arch:       "amd64",
	}
}

func testKey() qualify.Key {
	m := adapter.ConfigManifest{Digests: map[string]string{"user": "d1"}}
	return ObservedKey(testProbe(), m, "native-managed", "trusted")
}

func fixtureInput() FixtureEvidence {
	return FixtureEvidence{Fidelity: qualify.Unknown, Lifecycle: qualify.Unknown, Entitlement: qualify.NotProven}
}

func TestObservedKeyFields(t *testing.T) {
	m := adapter.ConfigManifest{Digests: map[string]string{"user": "d1"}}
	want := qualify.Key{
		Harness:          "codex",
		Surface:          descriptor().Surface,
		ExecutableDigest: "sha256:abc123",
		AdapterProtocol:  AdapterID + "+app-server-fixture",
		OS:               "linux",
		Arch:             "amd64",
		ProviderEndpoint: "unknown",
		ModelSnapshot:    "unknown",
		EffortSettings:   "unknown",
		AuthCategory:     "native-managed",
		ConfigDigest:     qualify.ConfigDigestOf(m.Digests),
		TrustProfile:     "trusted",
		WorkspaceClass:   "local-checkout",
		EntitlementClass: "unknown",
	}
	if got := testKey(); got != want {
		t.Fatalf("ObservedKey = %+v, want %+v", got, want)
	}
}

func TestFixtureDraftRefusesOtherHarness(t *testing.T) { // I14 (v2 §2)
	other := testKey()
	other.Harness = "claude-code"
	if _, err := FixtureDraft(other, fixtureInput()); !errors.Is(err, ErrNotCodexKey) {
		t.Fatalf("FixtureDraft(claude-code key) error = %v, want ErrNotCodexKey", err)
	}
	rec, err := FixtureDraft(testKey(), fixtureInput())
	if err != nil {
		t.Fatalf("FixtureDraft(codex key): %v", err)
	}
	if rec.Key != testKey() {
		t.Fatalf("draft key = %+v, want the codex key", rec.Key)
	}
}

func TestFixtureDraftCannotProve(t *testing.T) { // I14, I15 (v2 §2)
	for _, column := range []string{"Fidelity", "Lifecycle", "Entitlement"} {
		in := fixtureInput()
		reflect.ValueOf(&in).Elem().FieldByName(column).SetString(string(qualify.Proven))
		if _, err := FixtureDraft(testKey(), in); !errors.Is(err, ErrFixtureCannotProve) {
			t.Errorf("FixtureDraft with proven %s error = %v, want ErrFixtureCannotProve", column, err)
		}
	}

	rec, err := FixtureDraft(testKey(), fixtureInput())
	if err != nil {
		t.Fatalf("FixtureDraft: %v", err)
	}
	if rec.Progress != qualify.ProgressFixtureTested {
		t.Errorf("Progress = %q, want fixture-tested, never live-qualified", rec.Progress)
	}
	if rec.Entitlement.Verdict != qualify.NotProven {
		t.Errorf("Entitlement.Verdict = %q, want not-proven", rec.Entitlement.Verdict)
	}
	if len(rec.Entitlement.Evidence) != 1 || rec.Entitlement.Evidence[0].Result != "purchased_credit:unknown" {
		t.Errorf("Entitlement.Evidence = %+v, want one entry with Result purchased_credit:unknown", rec.Entitlement.Evidence)
	}
}

func TestFixtureDraftRecordsEveryRevisionThroughRegistry(t *testing.T) { // I14 (v2 §2)
	ctx := t.Context()
	reg, err := qualify.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reg.Close() }()

	for want := 1; want <= 2; want++ {
		draft, err := FixtureDraft(testKey(), fixtureInput())
		if err != nil {
			t.Fatalf("FixtureDraft: %v", err)
		}
		if err := reg.Record(ctx, draft); err != nil {
			t.Fatalf("recording draft %d of the same key: %v", want, err)
		}
		got, err := reg.Lookup(ctx, draft.Key)
		if err != nil {
			t.Fatalf("looking up the recorded draft: %v", err)
		}
		if got.Revision != want {
			t.Fatalf("recorded revision %d, want %d", got.Revision, want)
		}
		recomputed, err := qualify.CanonicalDigest(got)
		if err != nil || got.Digest != recomputed {
			t.Fatalf("stored digest %q does not recompute to %q (err %v)", got.Digest, recomputed, err)
		}
	}
}

func TestFixtureDraftColumnsIndependent(t *testing.T) { // I14, AC-7.1
	base, err := FixtureDraft(testKey(), fixtureInput())
	if err != nil {
		t.Fatalf("FixtureDraft: %v", err)
	}
	in := fixtureInput()
	in.Fidelity = qualify.NotProven
	changed, err := FixtureDraft(testKey(), in)
	if err != nil {
		t.Fatalf("FixtureDraft: %v", err)
	}
	if reflect.DeepEqual(base.Fidelity, changed.Fidelity) {
		t.Fatalf("fidelity column unchanged by the fidelity input: %+v", changed.Fidelity)
	}
	if !reflect.DeepEqual(base.Lifecycle, changed.Lifecycle) {
		t.Errorf("Lifecycle changed with the fidelity input: %+v vs %+v", base.Lifecycle, changed.Lifecycle)
	}
	if !reflect.DeepEqual(base.Entitlement, changed.Entitlement) {
		t.Errorf("Entitlement changed with the fidelity input: %+v vs %+v", base.Entitlement, changed.Entitlement)
	}

	in = fixtureInput()
	in.Lifecycle = qualify.NotProven
	changed, err = FixtureDraft(testKey(), in)
	if err != nil {
		t.Fatalf("FixtureDraft: %v", err)
	}
	if reflect.DeepEqual(base.Lifecycle, changed.Lifecycle) {
		t.Errorf("lifecycle column unchanged by the lifecycle input")
	}
	if !reflect.DeepEqual(base.Fidelity, changed.Fidelity) || !reflect.DeepEqual(base.Entitlement, changed.Entitlement) {
		t.Errorf("fidelity or entitlement changed with the lifecycle input")
	}
}

func TestRouteInventoryClosedAndUnknownBlocks(t *testing.T) { // I15 (v2 §2), AC-4.2
	routes, err := RouteInventory()
	if err != nil {
		t.Fatalf("RouteInventory: %v", err)
	}
	want := []string{"title", "summary-compaction", "review", "child", "native-observer"}
	var got []string
	for _, r := range routes {
		got = append(got, r.Name)
		if r.Coverage != "unknown" {
			t.Errorf("route %q coverage = %q, want unknown", r.Name, r.Coverage)
		}
		checked, err := CheckRoute(r.Name)
		if err != nil || checked != r {
			t.Errorf("CheckRoute(%q) = %+v, %v; want %+v", r.Name, checked, err, r)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("route names = %v, want %v", got, want)
	}
	if _, err := CheckRoute("telemetry"); !errors.Is(err, ErrUnknownRoute) {
		t.Errorf("CheckRoute(telemetry) error = %v, want ErrUnknownRoute", err)
	}

	routes[0].Coverage = "established"
	again, err := RouteInventory()
	if err != nil || again[0].Coverage != "unknown" {
		t.Errorf("mutating a returned inventory changed the closed list: %+v, %v", again[0], err)
	}
}

func TestParseRoutesRefusesBrokenInventory(t *testing.T) {
	for name, raw := range map[string]string{
		"not json":        `{`,
		"no routes":       `{"schema_version":1,"routes":[]}`,
		"unnamed route":   `{"schema_version":1,"routes":[{"name":"","coverage":"unknown"}]}`,
		"no coverage":     `{"schema_version":1,"routes":[{"name":"title","coverage":""}]}`,
		"duplicate route": `{"schema_version":1,"routes":[{"name":"title","coverage":"unknown"},{"name":"title","coverage":"unknown"}]}`,
		"wrong schema":    `{"schema_version":2,"routes":[{"name":"title","coverage":"unknown"}]}`,
	} {
		if _, err := parseRoutes([]byte(raw)); err == nil {
			t.Errorf("parseRoutes(%s) accepted a broken inventory", name)
		}
	}
}

func TestAuthCategoryUnknownByDefault(t *testing.T) { // I15, AC-4.7
	for name, env := range map[string][]string{
		"no status":       nil,
		"api key present": {"OPENAI_API_KEY=synthetic", "HOME=/tmp/home"},
	} {
		if got := AuthCategory(env); got != "unknown" {
			t.Errorf("AuthCategory(%s) = %q, want unknown", name, got)
		}
	}

	m := adapter.ConfigManifest{Digests: map[string]string{"user": "d1"}}
	observed := ObservedKey(testProbe(), m, AuthCategory(nil), "trusted")
	managed := ObservedKey(testProbe(), m, "native-managed", "trusted")
	if qualify.KeyHash(observed) == qualify.KeyHash(managed) {
		t.Fatalf("an unknown auth category hashes like a native-managed key")
	}
}
