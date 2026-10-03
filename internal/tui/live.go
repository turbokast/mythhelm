// Live render loop (design §13): the bounded live channel, the 200 ms
// coalesced poll tick, viewport virtualisation for notice lines, and the
// channel pump the program joins on quit. Live journaled events arrive
// through LiveFeed's non-blocking channel and are folded advisory; the
// poll tick replays dropped events from the journal with a fresh Load,
// so a paused terminal never stalls native event consumption (I06) and
// read-only polling adds no writer (I05).
package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
)

// liveEventsCap bounds the live journaled-event channel. A paused TUI lets
// it fill, the pipeline drops, and the next poll tick replays from the
// journal (D18).
const liveEventsCap = 256

// livePollInterval is the coalesced snapshot reload cadence: at most 5 Hz
// (D4). Event bursts never rebuild per event; one tick rebuilds once.
const livePollInterval = 200 * time.Millisecond

// maxNoticeRows bounds the full-width notice section in every layout. The
// tail (most recent) shows with a leading marker when older notices drop,
// so the live stream never pushes chrome out of view.
const maxNoticeRows = 3

// NoticeBacklog retains unjournaled notices in arrival order. Add never
// blocks and never drops; Drain takes every pending notice atomically.
type NoticeBacklog struct {
	mu      sync.Mutex
	pending []string
}

// Add appends a notice; it never blocks and never drops.
func (b *NoticeBacklog) Add(s string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pending = append(b.pending, s)
}

// Drain returns every retained notice in order and empties the backlog.
func (b *NoticeBacklog) Drain() []string {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.pending
	b.pending = nil
	return out
}

// LiveFeed builds the live-mode feed the launch wiring (task 12) threads
// into the pipeline's Hooks: a capacity-256 events channel, the notices
// backlog, and hooks whose Event sends as-is with a non-blocking
// drop-on-full and whose Notice appends to the backlog, never blocking or
// dropping. Interrupt is left unset; the launch wiring sets it from
// signal.Notify exactly as executeRun does.
func LiveFeed() (chan journal.Event, *NoticeBacklog, supervisor.Hooks) {
	events := make(chan journal.Event, liveEventsCap)
	notices := &NoticeBacklog{}
	hooks := supervisor.Hooks{
		Event: func(ev journal.Event) {
			select {
			case events <- ev:
			default:
				// The TUI lags; drop and let the next poll tick replay
				// from the journal (I06).
			}
		},
		Notice: func(s string) {
			notices.Add(s)
		},
	}
	return events, notices, hooks
}

// pollTickMsg is the 200 ms coalesced reload tick. It is deliberately
// distinct from motionFrameMsg (a settling animation frame), so the
// no-permanent-loop assertion counts motion frames without tripping on the
// permanent poll.
type pollTickMsg struct{}

// pollCmd schedules the next poll tick.
func (m *Model) pollCmd() tea.Cmd {
	return tea.Tick(livePollInterval, func(time.Time) tea.Msg { return pollTickMsg{} })
}

// noticeEvent maps one drained notice string to its synthetic event:
// Type ui.notice, payload {"text":...}, RunSequence -1 (no journal row ever
// carries it), ObservedAt the drain time. The model renders it as a notice
// line, never as journal progress.
func noticeEvent(text string, at time.Time) journal.Event {
	payload, _ := json.Marshal(map[string]string{"text": text})
	return journal.Event{Type: "ui.notice", Payload: payload, RunSequence: -1, ObservedAt: at.UTC()}
}

// drainNotices moves every pending notice from the live backlog and the
// in-flight recover feed (when one runs) into the model's synthetic notice
// events, preserving arrival order within each backlog.
func (m *Model) drainNotices(at time.Time) {
	at = at.UTC()
	if m.cfg.Notices != nil {
		for _, s := range m.cfg.Notices.Drain() {
			m.notices = append(m.notices, noticeEvent(s, at))
		}
	}
	if m.recoverNotices != nil {
		for _, s := range m.recoverNotices.Drain() {
			m.notices = append(m.notices, noticeEvent(s, at))
		}
	}
}

// noticeRows renders the synthetic notice events as attention lines,
// virtualised to a tail of at most max rows: the most recent notices stay
// visible with a leading marker counting the dropped older ones. A nil
// result renders nothing, so empty models are byte-identical to before.
func (m *Model) noticeRows(max int) []line {
	if max < 1 || len(m.notices) == 0 {
		return nil
	}
	all := make([]line, 0, len(m.notices))
	for _, ev := range m.notices {
		var p struct {
			Text string `json:"text"`
		}
		text := ""
		if err := json.Unmarshal(ev.Payload, &p); err == nil {
			text = p.Text
		}
		all = append(all, line{text: "notice: " + cell(text), colour: m.cfg.Tokens.Attention})
	}
	if len(all) <= max {
		return all
	}
	if max == 1 {
		return all[len(all)-1:]
	}
	dropped := len(all) - max + 1
	marker := line{
		text:   ellGlyph(m.cfg.Caps) + fmt.Sprintf(" and %d more notices", dropped),
		colour: m.cfg.Tokens.TextMuted,
	}
	return append([]line{marker}, all[len(all)-max+1:]...)
}

// reloadSnapshot rebuilds the snapshot once via the Load seam (a full scan
// included), converging the advisory live fold. A missing database shows
// the empty state as in Run; any other failure surfaces as the load error
// with the previous snapshot kept.
func (m *Model) reloadSnapshot() {
	load := m.loadSnapshot
	if load == nil {
		load = viewmodel.Load
	}
	ctx := m.runCtx
	if ctx == nil {
		ctx = context.Background()
	}
	snap, err := load(ctx, m.cfg.StateDir, m.cfg.RunID)
	if err != nil {
		if errors.Is(err, journal.ErrNoDatabase) {
			m.empty = true
			m.loadErr = nil
			return
		}
		m.loadErr = err
		return
	}
	m.empty = false
	m.setSnapshot(ctx, snap)
}

// drainRecoverEvents folds any pending recover-feed events advisory, arming
// their motion moments for the poll tick's motion schedule. The reload that
// follows converges them.
func (m *Model) drainRecoverEvents() {
	ch := m.recoverEvents
	if ch == nil {
		return
	}
	for {
		select {
		case ev := <-ch:
			m.consumeEvent(ev)
		default:
			return
		}
	}
}

// onPollTick runs one coalesced tick: drain notices, fold recover events,
// then rebuild the snapshot once.
func (m *Model) onPollTick() {
	m.drainNotices(time.Now().UTC())
	m.drainRecoverEvents()
	m.reloadSnapshot()
}

// finishRecoverFeed drains the recover feed remainder into the view and
// clears it, so a later recover starts fresh. It runs on-loop when the
// recover result lands.
func (m *Model) finishRecoverFeed() {
	at := time.Now().UTC()
	if m.recoverNotices != nil {
		for _, s := range m.recoverNotices.Drain() {
			m.notices = append(m.notices, noticeEvent(s, at))
		}
		m.recoverNotices = nil
	}
	if m.recoverEvents != nil {
		for {
			select {
			case ev := <-m.recoverEvents:
				m.consumeEvent(ev)
			default:
				m.recoverEvents = nil
				return
			}
		}
	}
}

// startLivePump forwards the live events channel to send, one eventMsg per
// journaled event, until stopLivePump joins it, the run context ends, or
// the channel closes. A slow consumer lets the channel fill so the pipeline
// drops; the poll tick replays from the journal. It starts at most once;
// a nil channel or send is a no-op.
func (m *Model) startLivePump(send func(journal.Event)) {
	if m.cfg.Events == nil || send == nil {
		return
	}
	m.liveMu.Lock()
	defer m.liveMu.Unlock()
	if m.liveQuit != nil {
		return
	}
	quit := make(chan struct{})
	done := make(chan struct{})
	m.liveQuit, m.liveDone = quit, done
	ch := m.cfg.Events
	var ctxDone <-chan struct{}
	if m.runCtx != nil {
		ctxDone = m.runCtx.Done()
	}
	go func() {
		defer close(done)
		for {
			select {
			case <-quit:
				return
			case <-ctxDone:
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				send(ev)
			}
		}
	}()
}

// stopLivePump joins the channel pump started by startLivePump. It is safe
// to call without a pump, twice, or after the channel closed. Run calls it
// after the program exits; it must never run inside Update, where the event
// loop already holds the update path the pump's Send waits on.
func (m *Model) stopLivePump() {
	m.liveMu.Lock()
	quit, done := m.liveQuit, m.liveDone
	m.liveQuit, m.liveDone = nil, nil
	m.liveMu.Unlock()
	if quit == nil {
		return
	}
	close(quit)
	<-done
}
