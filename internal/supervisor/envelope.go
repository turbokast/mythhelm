package supervisor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
)

// EventExtensionGranted is the control event that records a user's decision
// to raise one envelope ceiling (AC-4.2).
const EventExtensionGranted = "run.extension_granted"

// blockReasons maps each extendable ceiling to the run block it answers.
var blockReasons = map[string]string{
	"execution":         "envelope_deadline_exceeded",
	"repairs":           "envelope_repairs_exhausted",
	"replans":           "envelope_replans_exhausted",
	"transport_retries": "envelope_transport_retries_exhausted",
}

// ExtensionGrant is the operator's decision to raise one ceiling, in the
// envelope row's units: seconds for execution, counts otherwise.
type ExtensionGrant struct {
	Kind      string
	RaisedTo  int64
	DecidedBy string
}

type extensionPayload struct {
	Kind      string `json:"kind"`
	RaisedTo  int64  `json:"raised_to"`
	DecidedBy string `json:"decided_by"`
}

// RequestExtension journals run.extension_granted and raises the run's
// envelope row in one transaction. An unknown kind, a run not blocked for
// that kind, a value that does not exceed the current ceiling or a missing
// decider errors and journals nothing.
func RequestExtension(ctx context.Context, j *journal.Journal, runID, kind string, raisedTo int64, decidedBy string) error {
	reason, ok := blockReasons[kind]
	if !ok {
		return fmt.Errorf("unknown extension kind %q", kind)
	}
	if decidedBy == "" {
		return errors.New("extension needs a decided_by")
	}
	run, err := j.Run(ctx, runID)
	if err != nil {
		return err
	}
	if RunState(run.State) != RunBlocked || run.Reason != reason {
		return fmt.Errorf("run %s is %s (%s), not blocked for %s", runID, run.State, run.Reason, kind)
	}
	env, err := j.RunEnvelope(ctx, runID)
	if err != nil {
		return err
	}
	ceiling := map[string]*int64{"execution": &env.ExecutionSeconds, "repairs": &env.Repairs, "replans": &env.Replans, "transport_retries": &env.TransportRetries}[kind]
	if raisedTo <= *ceiling {
		return fmt.Errorf("extension of %s to %d must exceed the current ceiling %d", kind, raisedTo, *ceiling)
	}
	*ceiling = raisedTo
	now := time.Now().UTC()
	env.UpdatedAt = now.Format(time.RFC3339Nano)
	ev, err := newEvent(runID, "", "", EventExtensionGranted, extensionPayload{Kind: kind, RaisedTo: raisedTo, DecidedBy: decidedBy}, now)
	if err != nil {
		return err
	}
	return NewProducer(ids.New("sup"), 1).append(ctx, j, ev, func(tx *sql.Tx) error {
		return journal.UpsertRunEnvelope(ctx, tx, env)
	})
}

// CheckExtension reports whether a run.extension_granted event for kind
// follows the run's latest block for that kind, so a grant answers one block
// and never a later one. A journal read failure is an error: no grant is
// assumed (I02).
func CheckExtension(ctx context.Context, j *journal.Journal, runID, kind string) (bool, error) {
	reason, ok := blockReasons[kind]
	if !ok {
		return false, fmt.Errorf("unknown extension kind %q", kind)
	}
	events, err := j.Events(ctx, runID, 0)
	if err != nil {
		return false, err
	}
	var lastBlock, lastGrant int64
	for _, ev := range events {
		switch ev.Type {
		case "run.state_changed":
			var p struct {
				State  string  `json:"state"`
				Reason *string `json:"reason"`
			}
			if err := json.Unmarshal(ev.Payload, &p); err != nil {
				return false, fmt.Errorf("decoding %s: %w", ev.EventID, err)
			}
			if p.State == string(RunBlocked) && p.Reason != nil && *p.Reason == reason {
				lastBlock = ev.RunSequence
			}
		case EventExtensionGranted:
			var p extensionPayload
			if err := json.Unmarshal(ev.Payload, &p); err != nil {
				return false, fmt.Errorf("decoding %s: %w", ev.EventID, err)
			}
			if p.Kind == kind {
				lastGrant = ev.RunSequence
			}
		}
	}
	return lastGrant > lastBlock, nil
}
