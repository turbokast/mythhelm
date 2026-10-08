package claudecode

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/qualify"
)

// ErrEvidenceIncomplete reports assessed inputs that cannot support a
// live-qualified record: a remaining gap, unknown funding, enabled extra
// usage or purchased credits that are not established as non-paid.
var ErrEvidenceIncomplete = errors.New("claudecode: live evidence incomplete")

const (
	fundingIncluded = "included"
	fundingUnknown  = "unknown"
)

// at03Names are the seven AT-03 model-using routes of the first route.
var at03Names = []string{"implementer", "planner", "reviewer", "summary", "router", "child", "experiment"}

// AuxiliaryRoute is one model-using route that shares the funding tree.
type AuxiliaryRoute struct {
	Name     string // an AT-03 name, "plugin:<name>", "plugin:unknown" or "mcp:<server>"
	Funding  string // "included" (proven live) or "unknown"
	Evidence string // evidence id, "plugin-count:<n>" or "unknown"
}

// InventoryAuxiliary lists every known model-using route for the first-route
// surface. Static inputs prove no funding, so every route is "unknown"; only
// the authorised live suite assesses it. A bare install yields exactly the
// AT-03 routes. Plugins the session reports beyond the manifest's named ones
// collapse to one plugin:unknown route.
func InventoryAuxiliary(m Manifest, s adapter.SessionStarted) []AuxiliaryRoute {
	routes := make([]AuxiliaryRoute, 0, len(at03Names)+len(m.EnabledPlugins)+len(m.MCPServers)+1)
	for _, name := range at03Names {
		routes = append(routes, AuxiliaryRoute{Name: name, Funding: fundingUnknown, Evidence: "unknown"})
	}
	for _, name := range m.EnabledPlugins {
		routes = append(routes, AuxiliaryRoute{Name: "plugin:" + name, Funding: fundingUnknown, Evidence: "unknown"})
	}
	if surplus := s.PluginCount - len(m.EnabledPlugins); surplus > 0 {
		routes = append(routes, AuxiliaryRoute{Name: "plugin:unknown", Funding: fundingUnknown, Evidence: "plugin-count:" + strconv.Itoa(surplus)})
	}
	for _, server := range m.MCPServers {
		routes = append(routes, AuxiliaryRoute{Name: "mcp:" + server, Funding: fundingUnknown, Evidence: "unknown"})
	}
	return routes
}

// LiveRecord builds the live-qualified first-route record from assessed
// inputs. The live suite observes; this constructor only gates and assembles.
// It refuses a key outside the first route (ErrNotFirstRoute) and any input
// that leaves the funding tree unproven (ErrEvidenceIncomplete). The evidence
// Suite carries the executable digest, the only version identity in the key.
func LiveRecord(key qualify.Key, cfg EffectiveConfig, aux []AuxiliaryRoute) (qualify.Record, error) {
	if key.Harness != firstRouteHarness || key.Surface != firstRouteSurface {
		return qualify.Record{}, fmt.Errorf("claudecode: live record: key %s × %s: %w",
			key.Harness, key.Surface, ErrNotFirstRoute)
	}
	if err := checkAssessed(cfg, aux); err != nil {
		return qualify.Record{}, fmt.Errorf("claudecode: live record: %w: %w", ErrEvidenceIncomplete, err)
	}
	const evidenceID = "ev_live_entitlement"
	supported := func(scope string) qualify.Capability {
		return qualify.Capability{Value: "supported", Evidence: evidenceID, Scope: scope}
	}
	rec := qualify.Record{
		SchemaVersion: 2,
		Revision:      1,
		Key:           key,
		Progress:      qualify.ProgressLiveQualified,
		Fidelity:      qualify.Column{Verdict: qualify.Unknown, Evidence: []qualify.Evidence{}},
		Entitlement: qualify.Column{Verdict: qualify.Proven, Evidence: []qualify.Evidence{{
			ID:          evidenceID,
			Method:      "authorised-live",
			Suite:       "live-qualify:" + key.ExecutableDigest,
			Result:      "pass",
			Uncertainty: "point-in-time; covers the inventoried bare configuration",
			At:          time.Now().UTC().Truncate(time.Second),
			Label:       qualify.Observed,
			Source:      "live-qualify",
		}}},
		Lifecycle: qualify.Column{Verdict: qualify.Unknown, Evidence: []qualify.Evidence{}},
		Capabilities: map[string]qualify.Capability{
			"credential-precedence": supported("effective configuration"),
			"managed-policy":        supported("file-inventoried sources, no open gaps"),
			"extra-usage":           supported("extra usage disabled"),
			"purchased-credits":     supported("credits " + cfg.PurchasedCredits),
			"stop_at_exhaustion": {
				Value:    "supported",
				Evidence: "documented-mechanism",
				Scope:    "vendor exhaustion behaviour: usage stops at the plan limit while extra usage is disabled",
			},
		},
		Quota: qualify.Datum{
			Quantity: "unknown",
			Label:    qualify.DatumUnknown,
			Unit:     "unknown",
			Scope:    "unknown",
			Source:   "unknown",
		},
		NextTest: "none: re-run the live suite on drift",
	}
	digest, err := qualify.CanonicalDigest(rec)
	if err != nil {
		return qualify.Record{}, fmt.Errorf("claudecode: live record digest: %w", err)
	}
	rec.Digest = digest
	return rec, nil
}

func checkAssessed(cfg EffectiveConfig, aux []AuxiliaryRoute) error {
	if len(cfg.CredentialPrecedence) == 0 {
		return errors.New("no credential precedence established")
	}
	if len(cfg.ManagedPolicy.Gaps) > 0 {
		return fmt.Errorf("managed-policy gaps %v", cfg.ManagedPolicy.Gaps)
	}
	if cfg.ExtraUsage != "disabled" {
		return fmt.Errorf("extra usage %q, want disabled", cfg.ExtraUsage)
	}
	if cfg.PurchasedCredits != "non-consumable" && cfg.PurchasedCredits != "plan-granted" {
		return fmt.Errorf("purchased credits %q not established as non-paid", cfg.PurchasedCredits)
	}
	have := map[string]bool{}
	for _, r := range aux {
		if r.Funding != fundingIncluded {
			return fmt.Errorf("route %q funding %q", r.Name, r.Funding)
		}
		have[r.Name] = true
	}
	for _, name := range at03Names {
		if !have[name] {
			return fmt.Errorf("AT-03 route %q not assessed", name)
		}
	}
	return nil
}
