package tui

import (
	"context"

	"github.com/turbokast/mythhelm/internal/supervisor"
)

// Actions carries the injected supervisor entry points the palette acts
// through (design §10). Tests supply fakes; production wiring (task 12)
// supplies closures over the same calls the CLI verbs use, so the TUI never
// grows a second writer (I18). This task defines the struct only — dispatch,
// confirmations and export behaviour arrive in task 9.
type Actions struct {
	Stop    func(ctx context.Context, runID string) (string, error)
	Recover func(ctx context.Context, runID string, h supervisor.Hooks) (supervisor.RecoveryOutcome, error)
	Apply   func(ctx context.Context, runID, branch string, acceptFlags, acceptUnverified bool) (supervisor.Receipt, error)
}
