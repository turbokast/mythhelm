package admission

import (
	"context"
	"fmt"
	"os"
	"strings"

	ab "github.com/turbokast/mythhelm/internal/admissionbinding"
)

// PrepareService resolves a trusted service request without persisting a grant.
// Required decisions become closed challenges. This result is not authority;
// supervisor seals it and requires authenticated exact answers before use.
func PrepareService(ctx context.Context, req Request) (Decision, []ab.ChallengeV1, error) {
	if req.Confirm != nil || req.ProbeBoundary != nil {
		return Decision{}, nil, &BlockedError{Code: "service_input_unsupported", Capability: true, Action: "use service-owned policy inputs"}
	}
	// No actual service native-policy source is connected. Do not start a native
	// probe using the disconnected observation contracts or CLI defaults.
	if req.Adapter != AdapterFake {
		return Decision{}, nil, &BlockedError{Code: "native_policy_unavailable", Capability: true, Action: "a qualified service-owned native policy is required"}
	}
	info, err := os.Stat(req.TaskFile)
	if err != nil || !info.Mode().IsRegular() {
		return Decision{}, nil, &BlockedError{Code: "service_task_unavailable", Capability: true, Action: "use a bounded regular task file"}
	}
	challenges := []ab.ChallengeV1{}
	// This collector is internal staged resolution, never a caller confirmation
	// function and never a grant. Native routes have already refused above.
	req.Confirm = func(text string) (bool, error) {
		kind := ""
		switch {
		case strings.HasPrefix(text, "Trust committed mythhelm.toml sha256:"):
			kind = "project-config"
		case strings.HasPrefix(text, "The checkout has uncommitted changes,"):
			kind = "committed-revision"
		default:
			return false, &BlockedError{Code: "service_challenge_unsupported", Capability: true, Action: "this decision has no closed service authority mapping"}
		}
		challenges = append(challenges, ab.ChallengeV1{ID: fmt.Sprintf("planned_%d", len(challenges)), Kind: kind, Text: text})
		return true, nil
	}
	d, err := Decide(ctx, req)
	if err != nil {
		return Decision{}, nil, err
	}
	return d, challenges, nil
}
