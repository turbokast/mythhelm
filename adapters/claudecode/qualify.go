package claudecode

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/turbokast/mythhelm/internal/qualify"
)

//go:embed testdata/qualification/routes.json
var routesJSON []byte

const (
	// firstRouteHarness and firstRouteSurface identify the FR-3 first route.
	// They track the adapter descriptor; the refusal test builds its passing
	// key from the descriptor so a rename fails loudly.
	firstRouteHarness = "claude-code"
	firstRouteSurface = "native-cli-structured (print, stream-json)"

	// evidenceSuitePrefix prefixes the native version carried by the fixture
	// inventory to form each evidence Suite.
	evidenceSuitePrefix = "claudecode-at03-fixtures-"

	// evidenceSource names the mechanism that produced the entitlement
	// evidence: the offline AT-03 inventory, never a live call.
	evidenceSource = "claudecode AT-03 offline-fixture inventory"
)

// ErrNotFirstRoute reports a RecordDraft key that is not the first-route key.
// Evidence for one surface can never label another.
var ErrNotFirstRoute = errors.New("claudecode: not the first-route key")

// routeEntry is one AT-03 route of the embedded fixture inventory: the route,
// its passing condition and the auxiliary inventory it covers.
type routeEntry struct {
	Route              string   `json:"route"`
	PassingCondition   string   `json:"passing_condition"`
	AuxiliaryInventory []string `json:"auxiliary_inventory"`
	PaidAuxiliary      []string `json:"paid_auxiliary"`
}

// routesFile is the embedded AT-03 fixture inventory.
type routesFile struct {
	SchemaVersion int          `json:"schema_version"`
	Authority     string       `json:"authority"`
	Surface       string       `json:"surface"`
	NativeVersion string       `json:"native_version"`
	Routes        []routeEntry `json:"routes"`
}

// embeddedRoutes is the fixture inventory parsed once at load. An invalid
// inventory fails fast: every test in the package exercises it.
var embeddedRoutes = mustParseRoutes(routesJSON)

// mustParseRoutes decodes the embedded inventory, refusing any entry that
// cannot support evidence.
func mustParseRoutes(raw []byte) routesFile {
	var rf routesFile
	if err := json.Unmarshal(raw, &rf); err != nil {
		panic(fmt.Sprintf("claudecode: parsing embedded AT-03 inventory: %v", err))
	}
	if rf.SchemaVersion != 1 {
		panic(fmt.Sprintf("claudecode: embedded AT-03 inventory schema version %d, want 1", rf.SchemaVersion))
	}
	if len(rf.Routes) == 0 {
		panic("claudecode: embedded AT-03 inventory lists no routes")
	}
	for _, r := range rf.Routes {
		if r.Route == "" || r.PassingCondition == "" {
			panic(fmt.Sprintf("claudecode: embedded AT-03 inventory route %+v names no route or passing condition", r))
		}
	}
	return rf
}

// EntitlementEvidence returns the offline-fixture entitlement evidence for the
// claudecode first route: every AT-03 route covered, no hidden paid
// auxiliary, paid continuation unknown. It performs no live call and reads no
// secret: the evidence derives from the embedded inventory alone, so the
// output is identical on every call.
func EntitlementEvidence() []qualify.Evidence {
	rf := embeddedRoutes
	out := make([]qualify.Evidence, 0, len(rf.Routes))
	for _, r := range rf.Routes {
		out = append(out, qualify.Evidence{
			ID:          "ev_at03_" + r.Route,
			Method:      "offline-fixture",
			Suite:       evidenceSuitePrefix + rf.NativeVersion,
			Result:      "pass",
			Uncertainty: r.PassingCondition + "; paid continuation unknown",
			Label:       qualify.DatumUnknown,
			Source:      evidenceSource,
		})
	}
	return out
}

// RecordDraft returns the first-route Record at fixture-tested with the
// entitlement evidence attached and NextTest naming the authorised live
// suite. Callers build key per the design §4 key table; Quota is the unknown
// Datum because fixtures observe no allowance quantity. The digest is
// computed for revision 1, matching what Registry.Record stores for a new
// key; a key for any other harness or surface returns ErrNotFirstRoute and no
// record, whatever its digests.
func RecordDraft(key qualify.Key) (qualify.Record, error) {
	if key.Harness != firstRouteHarness || key.Surface != firstRouteSurface {
		return qualify.Record{}, fmt.Errorf("claudecode: qualify: key %s × %s: %w",
			key.Harness, key.Surface, ErrNotFirstRoute)
	}
	rec := qualify.Record{
		SchemaVersion: 2,
		Revision:      1,
		Key:           key,
		Progress:      qualify.ProgressFixtureTested,
		Fidelity:      qualify.Column{Verdict: qualify.Unknown, Evidence: []qualify.Evidence{}},
		Entitlement:   qualify.Column{Verdict: qualify.Proven, Evidence: EntitlementEvidence()},
		Lifecycle:     qualify.Column{Verdict: qualify.Unknown, Evidence: []qualify.Evidence{}},
		Capabilities: map[string]qualify.Capability{
			"stop_at_exhaustion": {Value: "unknown", Evidence: "unknown", Scope: "unknown"},
		},
		Quota: qualify.Datum{
			Quantity: "unknown",
			Label:    qualify.DatumUnknown,
			Unit:     "unknown",
			Scope:    "unknown",
			Source:   "unknown",
		},
		NextTest: "Run the authorised live included-only suite across the AT-03 routes " +
			"and record authorised-live evidence; authority: maintainer live-test grant (MH-12)",
	}
	digest, err := qualify.CanonicalDigest(rec)
	if err != nil {
		return qualify.Record{}, fmt.Errorf("claudecode: qualify: draft digest: %w", err)
	}
	rec.Digest = digest
	return rec, nil
}
