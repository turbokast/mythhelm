package viewmodel

import "time"

func classifySupervision(at, now time.Time, beatKnown bool, owner string) string {
	// A separately proven absent owner overrides even a recently written beat.
	if owner == "unheld" {
		return "unsupervised"
	}
	age := now.Sub(at)
	if beatKnown && !at.IsZero() && age >= 0 && age <= time.Second {
		return "supervised"
	}
	return "unknown"
}
