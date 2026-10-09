package admission

import (
	"errors"
	"fmt"
	"strings"

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
	ev, known := contain.SeedV1()[profile+"/"+os+"/"+route]
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
