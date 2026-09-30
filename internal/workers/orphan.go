package workers

import (
	"errors"
	"time"
)

// AttemptMarker is the exact native environment marker for manual orphan
// inspection. A marker identifies observations, never authority to signal.
func AttemptMarker(attemptID string) string { return attemptMarker(attemptID) }

// ProcessIdentity binds an unresolved descendant to a particular process,
// so later PID reuse cannot make recovery signal an unrelated process.
type ProcessIdentity struct {
	PID       int       `json:"pid"`
	StartTime time.Time `json:"start_time"`
}

// processIdentities snapshots start times for the observed descendants.
// A process that vanished is omitted; unreadable identities fail closed.
func processIdentities(pids []int) ([]ProcessIdentity, error) {
	identities := make([]ProcessIdentity, 0, len(pids))
	for _, pid := range pids {
		start, err := ProcessStartTime(pid)
		if errors.Is(err, ErrNoProcess) {
			continue
		}
		if err != nil {
			return nil, err
		}
		identities = append(identities, ProcessIdentity{PID: pid, StartTime: start})
	}
	return identities, nil
}
