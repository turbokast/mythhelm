package control

import (
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
)

// StopLadder is one versioned stop ladder. Version is pinned at admission
// and recorded in the stop receipt; Steps climb in order.
type StopLadder struct {
	Version string             `json:"version"` // e.g. "stop-ladder/v1"
	Steps   []adapter.StopStep `json:"steps"`
}

// MaxStopRungGrace bounds one rung; MaxStopDeadline bounds the whole ladder.
const (
	MaxStopRungGrace = 30 * time.Second
	MaxStopDeadline  = 2 * time.Minute
)

// Validate checks the ladder shape. Failure cases: invalid_contract for an
// empty version, zero steps, an unknown signal, a non-positive grace, a
// rung grace above MaxStopRungGrace, or a total above MaxStopDeadline.
func (l StopLadder) Validate() error {
	if l.Version == "" {
		return newError(CodeInvalidContract, "stop ladder version is empty")
	}
	if len(l.Steps) == 0 {
		return newError(CodeInvalidContract, "stop ladder steps is empty")
	}
	var total time.Duration
	for i, s := range l.Steps {
		switch s.Signal {
		case adapter.StopInterrupt, adapter.StopTerminate, adapter.StopKill:
		default:
			return newError(CodeInvalidContract, "stop ladder steps[%d].signal %q is unknown", i, s.Signal)
		}
		if s.Grace <= 0 {
			return newError(CodeInvalidContract, "stop ladder steps[%d].grace is not positive", i)
		}
		if s.Grace > MaxStopRungGrace {
			return newError(CodeInvalidContract, "stop ladder steps[%d].grace %s exceeds max rung grace %s", i, s.Grace, MaxStopRungGrace)
		}
		total += s.Grace
	}
	if total > MaxStopDeadline {
		return newError(CodeInvalidContract, "stop ladder total grace %s exceeds max stop deadline %s", total, MaxStopDeadline)
	}
	return nil
}

// StopParams is the params object of the "stop" intent.
type StopParams struct {
	AttemptID     string `json:"attempt_id"`
	LadderVersion string `json:"ladder_version"` // must equal the pinned version
}

// StopReceipt records what the ladder actually did (AC-1.2). Sent holds the
// signals delivered with the grace each was given; Confirmed is true only
// when the group was observed gone; UnresolvedPIDs names the remainder.
type StopReceipt struct {
	AttemptID      string             `json:"attempt_id"`
	LadderVersion  string             `json:"ladder_version"`
	Sent           []adapter.StopStep `json:"sent"`
	Confirmed      bool               `json:"confirmed"`
	UnresolvedPIDs []int              `json:"unresolved_pids,omitempty"`
	Quarantined    bool               `json:"quarantined"`
}
