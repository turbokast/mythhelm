package billing

import (
	"fmt"
	"time"
)

// Ceilings are the per-run envelope limits. A negative field means unset for
// the layer that carries it.
type Ceilings struct {
	Execution                          time.Duration
	Repairs, Replans, TransportRetries int
}

// BuiltInCeilings is the AC-4.1 fallback: 30 minutes, 3 repairs, 2 replans
// and 5 transport retries.
func BuiltInCeilings() Ceilings {
	return Ceilings{Execution: 30 * time.Minute, Repairs: 3, Replans: 2, TransportRetries: 5}
}

// ResolveCeilings applies explicit run flags over run configuration over the
// built-ins, per field. A nil layer, or a negative field, is unset.
func ResolveCeilings(flags, file *Ceilings) Ceilings {
	out := BuiltInCeilings()
	for _, layer := range []*Ceilings{file, flags} {
		if layer == nil {
			continue
		}
		if layer.Execution >= 0 {
			out.Execution = layer.Execution
		}
		if layer.Repairs >= 0 {
			out.Repairs = layer.Repairs
		}
		if layer.Replans >= 0 {
			out.Replans = layer.Replans
		}
		if layer.TransportRetries >= 0 {
			out.TransportRetries = layer.TransportRetries
		}
	}
	return out
}

// PauseSpan is one pause request. Only the span from QuiescedAt to ResumedAt
// stops the clock; a zero QuiescedAt means the turn was still running.
type PauseSpan struct{ RequestedAt, QuiescedAt, ResumedAt time.Time }

// Deadline is first start plus the execution ceiling, extended by the
// quiesced length of every pause (AC-4.3). A quiesced span not yet resumed
// extends to now. The deadline is reached when now is not before it.
func Deadline(firstStart time.Time, c Ceilings, pauses []PauseSpan, now time.Time) (deadline time.Time, expired bool) {
	deadline = firstStart.Add(c.Execution)
	for _, p := range pauses {
		if p.QuiescedAt.IsZero() {
			continue
		}
		end := p.ResumedAt
		if end.IsZero() {
			end = now
		}
		if end.After(p.QuiescedAt) {
			deadline = deadline.Add(end.Sub(p.QuiescedAt))
		}
	}
	return deadline, !now.Before(deadline)
}

// AttemptCounts are the counts a run has used against its ceilings.
type AttemptCounts struct{ Repairs, Replans, TransportRetries int }

// Check returns ErrBudgetExhausted naming the first kind whose count is over
// its ceiling; a count at the ceiling passes.
func (c Ceilings) Check(counts AttemptCounts) error {
	switch {
	case counts.Repairs > c.Repairs:
		return fmt.Errorf("%w: repairs %d over ceiling %d", ErrBudgetExhausted, counts.Repairs, c.Repairs)
	case counts.Replans > c.Replans:
		return fmt.Errorf("%w: replans %d over ceiling %d", ErrBudgetExhausted, counts.Replans, c.Replans)
	case counts.TransportRetries > c.TransportRetries:
		return fmt.Errorf("%w: transport_retries %d over ceiling %d", ErrBudgetExhausted, counts.TransportRetries, c.TransportRetries)
	}
	return nil
}
