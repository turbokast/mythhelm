package v2contract

import (
	"errors"
	"fmt"
	"time"
)

// Run is the v2 §4.1 Run record (the UI's Mission).
type Run struct {
	SchemaVersion        int       `json:"schema_version" toml:"schema_version"`
	RunID                string    `json:"run_id" toml:"run_id"`
	RepoID               string    `json:"repo_id" toml:"repo_id"`
	ExecutionHost        string    `json:"execution_host" toml:"execution_host"`
	Goal                 string    `json:"goal" toml:"goal"`
	GoalRevision         int       `json:"goal_revision" toml:"goal_revision"`
	RequestedDeliverable string    `json:"requested_deliverable" toml:"requested_deliverable"`
	SourceSnapshot       GitObject `json:"source_snapshot" toml:"source_snapshot"`
	PlanRevision         int       `json:"plan_revision" toml:"plan_revision"`
	PinnedPolicy         string    `json:"pinned_policy" toml:"pinned_policy"`
	GrantIDs             []string  `json:"grants" toml:"grants"`
	Budgets              []Budget  `json:"budgets" toml:"budgets"`
	State                RunState  `json:"lifecycle" toml:"lifecycle"`
	Reasons              []string  `json:"reasons" toml:"reasons"`
}

// Validate requires schema_version 2, a non-empty run_id and a lifecycle
// in the v2 §6.1 vocabulary.
func (r Run) Validate() error {
	if err := checkSchemaVersion("run", r.SchemaVersion); err != nil {
		return err
	}
	if r.RunID == "" {
		return errors.New("v2contract: run run_id is empty")
	}
	if !knownRunState(r.State) {
		return fmt.Errorf("v2contract: run lifecycle %q is not a run state", string(r.State))
	}
	return nil
}

// TaskRevision is immutable once dispatched (I20): a material change is a new
// Revision, never an edit.
type TaskRevision struct {
	SchemaVersion            int           `json:"schema_version" toml:"schema_version"`
	TaskID                   string        `json:"task_id" toml:"task_id"`
	Revision                 int           `json:"revision" toml:"revision"`
	Deliverable              string        `json:"deliverable" toml:"deliverable"`
	AcceptanceContractDigest string        `json:"acceptance_contract_digest" toml:"acceptance_contract_digest"`
	Dependencies             []RevisionRef `json:"dependencies" toml:"dependencies"`
	WriteScope               []string      `json:"write_scope" toml:"write_scope"`
	Risk                     string        `json:"risk" toml:"risk"`
	ResourceClaims           []string      `json:"resource_claims" toml:"resource_claims"`
	IntegrationDestination   string        `json:"integration_destination" toml:"integration_destination"`
	State                    TaskState     `json:"lifecycle" toml:"lifecycle"`
}

// Validate requires schema_version 2, a task_id, revision >= 1, a task-state
// lifecycle and a valid RevisionRef for every dependency.
func (t TaskRevision) Validate() error {
	if err := checkSchemaVersion("task revision", t.SchemaVersion); err != nil {
		return err
	}
	if t.TaskID == "" {
		return errors.New("v2contract: task revision task_id is empty")
	}
	if t.Revision < 1 {
		return fmt.Errorf("v2contract: task revision revision %d below 1", t.Revision)
	}
	if !knownTaskState(t.State) {
		return fmt.Errorf("v2contract: task revision lifecycle %q is not a task state", string(t.State))
	}
	for i, d := range t.Dependencies {
		if err := d.Validate(); err != nil {
			return fmt.Errorf("v2contract: task revision dependencies[%d]: %w", i, err)
		}
	}
	return nil
}

// Attempt is one launch attempt of a task revision (v2 §4.1).
type Attempt struct {
	SchemaVersion     int          `json:"schema_version" toml:"schema_version"`
	AttemptID         string       `json:"attempt_id" toml:"attempt_id"`
	TaskID            string       `json:"task_id" toml:"task_id"`
	TaskRevision      int          `json:"task_revision" toml:"task_revision"`
	Number            int          `json:"number" toml:"number"`
	RouteBundle       string       `json:"route_bundle" toml:"route_bundle"`
	WorkerID          string       `json:"worker_id" toml:"worker_id"`
	LaunchID          string       `json:"launch_id" toml:"launch_id"`
	Workspace         string       `json:"workspace" toml:"workspace"`
	NativeSessionRef  string       `json:"native_session_ref" toml:"native_session_ref"`
	ContextManifestID string       `json:"context_manifest_id" toml:"context_manifest_id"`
	GrantIDs          []string     `json:"grants" toml:"grants"`
	ReservationIDs    []string     `json:"reservations" toml:"reservations"`
	State             AttemptState `json:"lifecycle" toml:"lifecycle"`
	CreatedAt         time.Time    `json:"created_at" toml:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at" toml:"updated_at"`
}

// Validate requires schema_version 2, non-empty attempt_id, task_id and
// launch_id (the identity reconcile compares, I12), task_revision >= 1 and an
// attempt-state lifecycle.
func (a Attempt) Validate() error {
	if err := checkSchemaVersion("attempt", a.SchemaVersion); err != nil {
		return err
	}
	if a.AttemptID == "" {
		return errors.New("v2contract: attempt attempt_id is empty")
	}
	if a.TaskID == "" {
		return errors.New("v2contract: attempt task_id is empty")
	}
	if a.TaskRevision < 1 {
		return fmt.Errorf("v2contract: attempt task_revision %d below 1", a.TaskRevision)
	}
	if a.LaunchID == "" {
		return errors.New("v2contract: attempt launch_id is empty")
	}
	if !knownAttemptState(a.State) {
		return fmt.Errorf("v2contract: attempt lifecycle %q is not an attempt state", string(a.State))
	}
	return nil
}

func checkSchemaVersion(record string, got int) error {
	if got != SchemaVersion {
		return fmt.Errorf("v2contract: %s schema_version %d, want %d", record, got, SchemaVersion)
	}
	return nil
}
