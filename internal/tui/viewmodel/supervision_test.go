package viewmodel

import (
	"testing"
	"time"
)

func TestSupervisionClassification(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name        string
		age         time.Duration
		known       bool
		owner, want string
	}{
		{"fresh unknown", 0, true, "unknown", "supervised"},
		{"threshold held", time.Second, true, "held", "supervised"},
		{"stale held", time.Second + 1, true, "held", "unknown"},
		{"stale unknown", 2 * time.Second, true, "unknown", "unknown"},
		{"missing held", 0, false, "held", "unknown"},
		{"missing unknown", 0, false, "unknown", "unknown"},
		{"stale proven unheld", 2 * time.Second, true, "unheld", "unsupervised"},
		{"missing proven unheld", 0, false, "unheld", "unsupervised"},
		{"fresh proven unheld", 0, true, "unheld", "unsupervised"},
		{"future unknown", -time.Second, true, "unknown", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := classifySupervision(now.Add(-tc.age), now, tc.known, tc.owner); got != tc.want {
				t.Fatalf("classification %q, want %q", got, tc.want)
			}
		})
	}
}
