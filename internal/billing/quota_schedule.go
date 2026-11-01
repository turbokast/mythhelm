package billing

import "time"

// EvaluateQuotaSchedule evaluates the fixed exhausted-bucket retry policy.
// It has no storage effects; the reservation transaction counts due retries.
func EvaluateQuotaSchedule(exhaustedAt string, resetAt *string, retriesUsed int64, now time.Time) (admit bool, wait time.Duration, giveUp bool) {
	if retriesUsed < 0 {
		return false, 0, false
	}
	var reset *time.Time
	if resetAt != nil {
		t, err := time.Parse(time.RFC3339Nano, *resetAt)
		if err != nil {
			return false, 0, false
		}
		reset = &t
	}
	// Compare the int64 before indexing; narrowing corrupt counters can wrap.
	if retriesUsed >= 3 {
		return false, 0, true
	}
	var next time.Time
	if reset != nil {
		next = *reset
	} else {
		exhausted, err := time.Parse(time.RFC3339Nano, exhaustedAt)
		if err != nil {
			return false, 0, false
		}
		backoff := [...]time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute}
		next = exhausted.Add(backoff[retriesUsed])
	}
	if now.Before(next) {
		return false, next.Sub(now), false
	}
	return true, 0, false
}
