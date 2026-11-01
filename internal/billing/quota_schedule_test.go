package billing_test

import (
	"math"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/qualify"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

// Compile-time alias compatibility preserves both directions of the existing
// billing/qualification API while billing no longer imports its registry.
var (
	_ qualify.DatumLabel = billing.ScopeTotal{}.Label
	_                    = billing.ScopeTotal{Label: qualify.Reported}
)

// Independent fixtures pin I02/I09/I21 (v2 §2): damaged schedules refuse,
// unknown remains unknown and the three-retry envelope cannot restart.
func TestFixedQuotaScheduleMatchesLegacy(t *testing.T) {
	t.Parallel()
	at := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	exhausted := at.Format(time.RFC3339Nano)
	reset := at.Add(time.Hour).Format(time.RFC3339Nano)
	for _, tc := range []struct {
		name, exhausted string
		reset           *string
		used            int64
		now             time.Time
		admit           bool
		wait            time.Duration
		giveUp          bool
		legacy          bool
	}{
		{"first early", exhausted, nil, 0, at.Add(59 * time.Second), false, time.Second, false, true},
		{"first due", exhausted, nil, 0, at.Add(time.Minute), true, 0, false, true},
		{"second early", exhausted, nil, 1, at.Add(4 * time.Minute), false, time.Minute, false, true},
		{"second due", exhausted, nil, 1, at.Add(5 * time.Minute), true, 0, false, true},
		{"third early", exhausted, nil, 2, at.Add(14 * time.Minute), false, time.Minute, false, true},
		{"third due", exhausted, nil, 2, at.Add(15 * time.Minute), true, 0, false, true},
		{"spent", exhausted, nil, 3, at.Add(24 * time.Hour), false, 0, true, true},
		{"reset future", exhausted, &reset, 0, at, false, time.Hour, false, true},
		{"reset due", exhausted, &reset, 0, at.Add(time.Hour), true, 0, false, true},
		{"reset future spent", exhausted, &reset, 3, at, false, 0, true, true},
		{"reset due spent policy", exhausted, &reset, 3, at.Add(time.Hour), false, 0, true, true},
		{"bad exhaustion", "soon", nil, 0, at, false, 0, false, true},
		{"bad reset", exhausted, new("tomorrow"), 0, at, false, 0, false, true},
		{"negative", exhausted, nil, -1, at, false, 0, false, false},
		{"overflow count", exhausted, nil, math.MaxInt64, at, false, 0, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, w, g := billing.EvaluateQuotaSchedule(tc.exhausted, tc.reset, tc.used, tc.now)
			if a != tc.admit || w != tc.wait || g != tc.giveUp {
				t.Fatalf("schedule = %t %v %t; want %t %v %t", a, w, g, tc.admit, tc.wait, tc.giveUp)
			}
			if tc.legacy {
				la, lw, lg := supervisor.EvaluateBucket(journal.BucketRow{ExhaustedAt: tc.exhausted, ResetAt: tc.reset, RetriesUsed: tc.used}, tc.now)
				if a != la || w != lw || g != lg {
					t.Fatalf("legacy = %t %v %t; fixed = %t %v %t", la, lw, lg, a, w, g)
				}
			}
		})
	}
}
