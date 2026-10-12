package supervisor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/turbokast/mythhelm/internal/acceptance"
	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/contain"
	"github.com/turbokast/mythhelm/internal/integration"
	"github.com/turbokast/mythhelm/internal/journal"
)

// AdmissionPayload is the admission.decided payload: the admission record
// plus the run's acceptance pin (design §2.2). Decoders that predate the pin
// read the embedded record and ignore the extra key.
type AdmissionPayload struct {
	admission.Record
	AcceptancePin acceptance.Pin `json:"acceptance_pin"`
}

// checkOptions is the option set the verification stage runs the checks
// with: the restricted policy for a contained run, host authority otherwise.
// Admission-time pinning and conclude both call it, so the pinned evaluator
// is the one that runs (I07).
func checkOptions(d admission.Decision) integration.RunOptions {
	opts := integration.RunOptions{KeepGoing: d.KeepGoing}
	if d.Profile.Contained {
		opts.Policy = &contain.Policy{Profile: admission.ProfileRestricted}
	}
	return opts
}

// NewAdmissionPayload builds the record admission journals: d.Record() and the
// run's P1 pin. The check definition, evaluator and timeouts are pinned only
// when checks will run (not waived, not refused under inspect); otherwise they
// stay empty and render as unknown (I09).
func NewAdmissionPayload(d admission.Decision) (AdmissionPayload, error) {
	pin := acceptance.NewPin(d.TaskID, d.Task.SHA256)
	if !d.NoChecks && d.Profile.Name != admission.ProfileInspect {
		ev, err := integration.EvaluatorFor(checkOptions(d), d.ProjectConfig.Checks)
		if err != nil {
			return AdmissionPayload{}, fmt.Errorf("computing the evaluator to pin: %w", err)
		}
		timeouts := make(map[string]string, len(d.ProjectConfig.Checks))
		for _, c := range d.ProjectConfig.Checks {
			timeouts[c.Name] = c.Timeout
		}
		pin = pin.WithChecks(d.ConfigDigest, ev.Name, ev.Digest, timeouts)
	}
	return AdmissionPayload{Record: d.Record(), AcceptancePin: pin}, nil
}

// ReadPin returns the pin journaled with the run's admission. It reports false
// with no error when the admission predates the pin or the run has none; an
// error is only a read or decode failure, which callers never treat as a
// missing pin (I02).
func ReadPin(ctx context.Context, j *journal.Journal, runID string) (acceptance.Pin, bool, error) {
	events, err := j.Events(ctx, runID, 0)
	if err != nil {
		return acceptance.Pin{}, false, err
	}
	var pin *acceptance.Pin
	for _, ev := range events {
		if ev.Type != "admission.decided" {
			continue
		}
		var payload struct {
			AcceptancePin *acceptance.Pin `json:"acceptance_pin"`
		}
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			return acceptance.Pin{}, false, fmt.Errorf("decoding the acceptance pin of %s: %w", runID, err)
		}
		pin = payload.AcceptancePin
	}
	if pin == nil {
		return acceptance.Pin{}, false, nil
	}
	return *pin, true, nil
}
