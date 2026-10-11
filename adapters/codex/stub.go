package codex

import (
	"context"
	"errors"

	"github.com/turbokast/mythhelm/internal/adapter"
)

// ErrNotImplemented is returned by Prepare and Start until Task 5 of the
// codex-native-adapter spec replaces this file.
var ErrNotImplemented = errors.New("codex: not implemented")

func (*codexAdapter) Prepare(context.Context, adapter.PrepareInput) (adapter.LaunchProposal, error) {
	return adapter.LaunchProposal{}, ErrNotImplemented
}

func (*codexAdapter) Start(context.Context, adapter.LaunchProposal, adapter.Launcher) (adapter.Session, error) {
	return nil, ErrNotImplemented
}
