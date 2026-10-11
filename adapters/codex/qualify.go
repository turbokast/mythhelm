package codex

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/qualify"
)

//go:embed testdata/qualification/routes.json
var routesJSON []byte

const (
	unknownValue = "unknown"

	// fixtureSuite names the evidence every FixtureDraft column cites.
	fixtureSuite = "codex-synthetic-fixtures"

	evidenceSource = "codex offline-fixture draft"
)

var (
	// ErrNotCodexKey reports a FixtureDraft key for another harness: evidence
	// for one harness never labels another.
	ErrNotCodexKey = errors.New("codex: not a codex key")

	// ErrFixtureCannotProve reports a FixtureDraft input that asks a fixture
	// to prove a column; only authorised live evidence can.
	ErrFixtureCannotProve = errors.New("codex: fixtures cannot prove a qualification column")

	// ErrUnknownRoute reports a model-using route outside the closed
	// inventory; such a route is unknown and blocks.
	ErrUnknownRoute = errors.New("codex: route outside the closed inventory")
)

// FixtureEvidence is the verdict each column asks a fixture draft to record.
// The zero value means unknown.
type FixtureEvidence struct {
	Fidelity    qualify.Verdict
	Lifecycle   qualify.Verdict
	Entitlement qualify.Verdict
}

// Route is one model-using auxiliary or descendant route of a Codex attempt.
type Route struct {
	Name     string `json:"name"`
	Coverage string `json:"coverage"`
}

type routesFile struct {
	SchemaVersion int     `json:"schema_version"`
	Routes        []Route `json:"routes"`
}

// ObservedKey builds the qualification key for a probed executable. Fields a
// probe cannot establish stay "unknown" (I09).
func ObservedKey(p adapter.Probe, m adapter.ConfigManifest, authCategory, profile string) qualify.Key {
	return qualify.Key{
		Harness:          "codex",
		Surface:          descriptor().Surface,
		ExecutableDigest: "sha256:" + p.SHA256,
		AdapterProtocol:  AdapterID + "+app-server-fixture",
		OS:               p.OS,
		Arch:             p.Arch,
		ProviderEndpoint: unknownValue,
		ModelSnapshot:    unknownValue,
		EffortSettings:   unknownValue,
		AuthCategory:     authCategory,
		ConfigDigest:     qualify.ConfigDigestOf(m.Digests),
		TrustProfile:     profile,
		WorkspaceClass:   "local-checkout",
		EntitlementClass: unknownValue,
	}
}

// AuthCategory reports the observed sign-in category. No non-secret status
// source is defined yet (design §5), and a credential's presence in env is not
// a category (I15), so it is always "unknown".
func AuthCategory([]string) string { return unknownValue }

// FixtureDraft returns the fixture-tested record for a Codex key. Fixtures
// prove no column: a proven verdict is refused, fidelity and lifecycle keep
// the requested unknown or not-proven verdict, and entitlement is not-proven
// with purchased_credit unknown (I14, I15). Digest stays unset: the registry
// assigns the revision, then computes it.
func FixtureDraft(k qualify.Key, ev FixtureEvidence) (qualify.Record, error) {
	if k.Harness != "codex" {
		return qualify.Record{}, fmt.Errorf("codex: qualify: key harness %q: %w", k.Harness, ErrNotCodexKey)
	}
	fidelity, err := requestedVerdict("fidelity", ev.Fidelity)
	if err != nil {
		return qualify.Record{}, err
	}
	lifecycle, err := requestedVerdict("lifecycle", ev.Lifecycle)
	if err != nil {
		return qualify.Record{}, err
	}
	if _, err := requestedVerdict("entitlement", ev.Entitlement); err != nil {
		return qualify.Record{}, err
	}

	rec := qualify.Record{
		SchemaVersion: 2,
		Revision:      1,
		Key:           k,
		Progress:      qualify.ProgressFixtureTested,
		Fidelity:      fixtureColumn(fidelity, string(fidelity)),
		Entitlement:   fixtureColumn(qualify.NotProven, "purchased_credit:unknown"),
		Lifecycle:     fixtureColumn(lifecycle, string(lifecycle)),
		Capabilities: map[string]qualify.Capability{
			"stop_at_exhaustion": {Value: unknownValue, Evidence: unknownValue, Scope: unknownValue},
		},
		Quota: qualify.Datum{
			Quantity: unknownValue,
			Label:    qualify.DatumUnknown,
			Unit:     unknownValue,
			Scope:    unknownValue,
			Source:   unknownValue,
		},
		NextTest: "Run the authorised live Codex suite on the native-managed sign-in and record " +
			"live evidence; authority: maintainer live-test grant (MH-12)",
	}
	return rec, nil
}

// requestedVerdict validates the verdict a draft input asks for. The empty
// verdict means unknown; proven, or any out-of-scale value, is refused.
func requestedVerdict(name string, v qualify.Verdict) (qualify.Verdict, error) {
	switch v {
	case "", qualify.Unknown:
		return qualify.Unknown, nil
	case qualify.NotProven:
		return v, nil
	}
	return "", fmt.Errorf("codex: qualify: %s verdict %q: %w", name, v, ErrFixtureCannotProve)
}

func fixtureColumn(v qualify.Verdict, result string) qualify.Column {
	return qualify.Column{
		Verdict: v,
		Evidence: []qualify.Evidence{{
			ID:          "ev_fixture_" + string(v),
			Method:      "offline-fixture",
			Suite:       fixtureSuite,
			Result:      result,
			Uncertainty: "synthetic fixtures establish no vendor behaviour",
			Label:       qualify.DatumUnknown,
			Source:      evidenceSource,
		}},
	}
}

// RouteInventory returns the closed list of model-using routes (AC-4.2). A
// fresh slice is returned on every call.
func RouteInventory() ([]Route, error) {
	return parseRoutes(routesJSON)
}

// CheckRoute returns the named route, or ErrUnknownRoute for one outside the
// closed list.
func CheckRoute(name string) (Route, error) {
	routes, err := RouteInventory()
	if err != nil {
		return Route{}, err
	}
	i := slices.IndexFunc(routes, func(r Route) bool { return r.Name == name })
	if i < 0 {
		return Route{}, fmt.Errorf("codex: route %q: %w", name, ErrUnknownRoute)
	}
	return routes[i], nil
}

func parseRoutes(raw []byte) ([]Route, error) {
	var rf routesFile
	if err := json.Unmarshal(raw, &rf); err != nil {
		return nil, fmt.Errorf("codex: parsing route inventory: %w", err)
	}
	if rf.SchemaVersion != 1 {
		return nil, fmt.Errorf("codex: route inventory schema version %d, want 1", rf.SchemaVersion)
	}
	if len(rf.Routes) == 0 {
		return nil, errors.New("codex: route inventory lists no routes")
	}
	seen := map[string]bool{}
	for _, r := range rf.Routes {
		if r.Name == "" || r.Coverage == "" {
			return nil, fmt.Errorf("codex: route inventory entry %+v names no route or coverage", r)
		}
		if seen[r.Name] {
			return nil, fmt.Errorf("codex: route inventory repeats %q", r.Name)
		}
		seen[r.Name] = true
	}
	return rf.Routes, nil
}
