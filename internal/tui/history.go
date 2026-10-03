// History search (design §13, D17): journal-replay search over the full
// event stream. The live stream tail on screen shows at most the pane's
// visible rows, while history stays complete because search replays the
// journal from zero with a caller-side filter — never a bounded memory
// copy a ring buffer would truncate.
package tui

import (
	"context"
	"strings"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
)

// SearchHistory replays runID's full journal from run_sequence zero and
// returns the events matching query in run_sequence order. The match is a
// case-insensitive substring over the event type plus its JSON payload; an
// empty query returns the whole stream. Replay-from-zero keeps the history
// complete however long the run grows.
func SearchHistory(ctx context.Context, dir, runID, query string) ([]journal.Event, error) {
	events, err := viewmodel.EventsSince(ctx, dir, runID, 0)
	if err != nil {
		return nil, err
	}
	if query == "" {
		return events, nil
	}
	needle := strings.ToLower(query)
	var out []journal.Event
	for _, ev := range events {
		if strings.Contains(strings.ToLower(ev.Type+" "+string(ev.Payload)), needle) {
			out = append(out, ev)
		}
	}
	return out, nil
}
