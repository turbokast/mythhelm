package admission

import (
	"errors"
	"fmt"
	"strings"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/contain"
)

// containedDimensions are the dimensions a contained profile requires.
var containedDimensions = []contain.Dimension{contain.DimFilesystem, contain.DimProcess, contain.DimNetwork, contain.DimCredential}

// BoundaryConsult looks up the recorded boundary evidence for a contained
// profile on an OS and route. Supported evidence is returned; anything else is
// an exit-7 refusal naming every dimension the boundary does not enforce
// (AC-1.2), and an unknown combination is also contain.ErrMissingCoverage:
// unknown evidence blocks and never falls back to host authority (I02).
func BoundaryConsult(profile, os, route string) (contain.Evidence, error) {
	return ConsultRegistry(evidenceMap(contain.SeedV1()), profile, os, route)
}

// evidenceMap is a contain.Registry over SeedV1's records.
type evidenceMap map[string]contain.Evidence

func (m evidenceMap) Lookup(profile, os, route string) (contain.Evidence, bool) {
	ev, ok := m[profile+"/"+os+"/"+route]
	return ev, ok
}

// ConsultRegistry is BoundaryConsult over any registry. A record is accepted
// only when it enforces every contained dimension, the filesystem included:
// inspect's read-only workspace never rests on a prompt or a tool label
// (AC-2.2).
func ConsultRegistry(reg contain.Registry, profile, os, route string) (contain.Evidence, error) {
	ev, known := reg.Lookup(profile, os, route)
	missing := containedDimensions
	if known {
		missing = ev.Coverage.Missing(containedDimensions)
	}
	if len(missing) == 0 {
		return ev, nil
	}
	names := make([]string, len(missing))
	for i, d := range missing {
		names[i] = string(d)
	}
	blocked := &BlockedError{
		Code:       "execution_profile_unavailable",
		Field:      fmt.Sprintf("--execution-profile %s on %s/%s: missing coverage: %s", profile, os, route, strings.Join(names, ", ")),
		Action:     "pass --execution-profile trusted-host to run with your host authority, not contained",
		Capability: true,
	}
	if !known {
		return contain.Evidence{}, errors.Join(blocked, contain.ErrMissingCoverage)
	}
	return contain.Evidence{}, blocked
}

// BoundaryDeltas reports each user-visible restriction the admitted boundary
// imposes on native features as a fidelity delta (AC-4.3): the read-only
// mounts and the proxy pin. A claim that is not enforced gets no delta, and
// unattributed evidence (no boundary name) gets none: a delta must never
// claim a restriction nothing enforces. Process and credential claims get
// none by design: dropping capabilities and hiding ambient credentials
// restrict authority, not native features, and admitted auth binds keep
// native authentication working (AC-7.1).
func BoundaryDeltas(ev contain.Evidence) []adapter.ConfigDelta {
	if ev.Boundary == "" {
		return nil
	}
	var deltas []adapter.ConfigDelta
	if ev.Coverage.Filesystem.Enforced {
		workdir := "only the workdir stays writable"
		if ev.Profile == contain.ProfileInspect {
			workdir = "the workdir is read-only"
		}
		deltas = append(deltas, adapter.ConfigDelta{Kind: "boundary", Name: "read-only mounts",
			Reason: ev.Boundary + " remounts / read-only; " + workdir})
	}
	if ev.Coverage.Network.Enforced {
		deltas = append(deltas, adapter.ConfigDelta{Kind: "boundary", Name: "proxy pin",
			Reason: ev.Boundary + " pins HTTPS_PROXY/HTTP_PROXY to the worker's filtering proxy; direct egress is not blocked"})
	}
	return deltas
}
