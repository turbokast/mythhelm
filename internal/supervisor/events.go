package supervisor

import "slices"

// StreamEventTypes returns, sorted, the closed set of event types the JSONL
// stream documents: design §5's twenty-three and the six other types a run's
// journal can hold (attempt.native_error, recovery.ownership_resolved,
// attempt.recovery_decided, run.extension_granted, migration.imported and
// migration.quarantined). run.result comes from the renderer, not the journal.
func StreamEventTypes() []string {
	return []string{
		"admission.decided",
		"apply.completed",
		"apply.intent_recorded",
		"attempt.admission_pinned",
		"attempt.launch_intent_recorded",
		"attempt.launched",
		"attempt.native_error",
		"attempt.native_result",
		"attempt.native_session",
		"attempt.permission_denied",
		"attempt.progress",
		"attempt.protocol_counters",
		"attempt.recovery_decided",
		"attempt.state_changed",
		"attempt.stop_requested",
		"attempt.stopped",
		"candidate.frozen",
		"check.completed",
		"migration.imported",
		"migration.quarantined",
		"receipt.written",
		"recovery.ownership_resolved",
		"run.created",
		"run.extension_granted",
		"run.result",
		"run.state_changed",
		"verification.completed",
		"verification.started",
		"workspace.snapshot_created",
	}
}

// IsStreamEvent reports whether typ is a documented stream event type.
func IsStreamEvent(typ string) bool {
	return slices.Contains(StreamEventTypes(), typ)
}
