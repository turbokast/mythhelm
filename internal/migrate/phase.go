// Package migrate moves a dogfood-slice state directory to the per-user
// supervisor service (supervisor-migration design §2, D1): legacy importer,
// drain orchestrator and backup/restore, plus the hermetic preview planner.
// Every ledger-mutating write runs inside control.Mutate; this package never
// opens SQLite itself outside the supervisor's transaction path.
package migrate

// Phase is the durable migration phase in migration_state.
type Phase string

const (
	PhaseNotStarted Phase = "not_started"
	PhasePreviewed  Phase = "previewed"
	PhaseDrained    Phase = "drained"
	PhaseImported   Phase = "imported"
	PhaseAdopted    Phase = "adopted" // service owns the ledger
)
