package claudecode

import (
	"context"
	"errors"
	"fmt"
)

// ErrPolicyObservation marks an incomplete or invalid observation operation.
var ErrPolicyObservation = errors.New("policy observation unavailable")

// PolicySourceKind identifies one of the eight reviewed managed-policy sources.
// Its closed vocabulary is validated by ObservePolicySources.
type PolicySourceKind string

// PolicySourceState is a report of present, absent, unreadable, unsupported or
// unknown. These reports do not establish effective policy or entitlement.
type PolicySourceState string

// PolicySourceObservation keeps source identity separate from reported state.
// It intentionally carries no source content, credentials or qualification.
type PolicySourceObservation struct {
	Kind  PolicySourceKind
	State PolicySourceState
}

// PolicySourceReader is the injected observation seam. This package supplies no
// production reader. Implementations must return promptly on cancellation.
type PolicySourceReader interface {
	Observe(context.Context, PolicySourceKind) (PolicySourceObservation, error)
}

// ObservePolicySources observes a unique, canonically ordered subset of sources.
// The order is remote cache, macOS preferences, Windows machine policy, Windows
// user policy, managed files, managed MCP, policy helper, host parent settings.
// A nil reader reports unknown; an empty request asserts no completeness.
// It returns no partial data on failure. The context must not be nil; a reader
// that blocks cannot be interrupted by this synchronous observation function.
func ObservePolicySources(ctx context.Context, reader PolicySourceReader, kinds []PolicySourceKind) ([]PolicySourceObservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, policyObservationFailure(err)
	}
	if len(kinds) > 8 {
		return nil, fmt.Errorf("%w: too many sources", ErrPolicyObservation)
	}
	requested := append([]PolicySourceKind{}, kinds...)
	previous := -1
	for _, kind := range requested {
		rank := policySourceRank(kind)
		if rank < 0 || rank <= previous {
			return nil, fmt.Errorf("%w: unknown, duplicate or unordered source", ErrPolicyObservation)
		}
		previous = rank
	}
	observations := make([]PolicySourceObservation, 0, len(kinds))
	for _, kind := range requested {
		if err := ctx.Err(); err != nil {
			return nil, policyObservationFailure(err)
		}
		observation := PolicySourceObservation{Kind: kind, State: "unknown"}
		if reader != nil {
			var err error
			observation, err = reader.Observe(ctx, kind)
			if canceled := ctx.Err(); canceled != nil {
				return nil, policyObservationFailure(canceled)
			}
			if err != nil {
				return nil, policyObservationFailure(err)
			}
		}
		if observation.Kind != kind || !policySourceStateValid(observation.State) {
			return nil, fmt.Errorf("%w: mismatched source or invalid state", ErrPolicyObservation)
		}
		observations = append(observations, observation)
	}
	if err := ctx.Err(); err != nil {
		return nil, policyObservationFailure(err)
	}
	return observations, nil
}

func policyObservationFailure(err error) error {
	return fmt.Errorf("%w: %w", ErrPolicyObservation, err)
}
func policySourceRank(kind PolicySourceKind) int {
	switch kind {
	case "remote-cache":
		return 0
	case "macos-managed-preferences":
		return 1
	case "windows-machine-policy":
		return 2
	case "windows-user-policy":
		return 3
	case "managed-files":
		return 4
	case "managed-mcp":
		return 5
	case "policy-helper":
		return 6
	case "host-parent-settings":
		return 7
	default:
		return -1
	}
}
func policySourceStateValid(state PolicySourceState) bool {
	switch state {
	case "present", "absent", "unreadable", "unsupported", "unknown":
		return true
	default:
		return false
	}
}
