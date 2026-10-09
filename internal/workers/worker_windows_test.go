//go:build windows

package workers

import (
	"slices"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
)

func TestWorkerClimbsPinnedLadder(t *testing.T) {
	// The Windows ladder is Process.Kill alone: Signal rejects every other
	// rung, and without a Job Object (N7) the worker owns a single process,
	// so multi-rung confirmation stays blocked on this surface (OQ-SR2).
	ladder := []adapter.StopStep{{Signal: adapter.StopKill, Grace: 5 * time.Second}}
	p := climbPinnedLadder(t, "slow", ladder)
	if !p.Confirmed || !slices.Equal(p.Sent, ladder) || p.LadderVersion != "stop-ladder/v1" {
		t.Errorf("attempt.stopped = %+v, want confirmed with sent %v under stop-ladder/v1", p, ladder)
	}
	if p.DescendantScan != "unavailable" {
		t.Errorf("attempt.stopped descendant_scan = %q, want unavailable: without a Job Object the worker claims nothing about descendants", p.DescendantScan)
	}
}
