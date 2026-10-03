package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
)

// TestLiveCallbackNeverBlocks pins the D18 drop-and-replay contract: the
// LiveFeed Event adapter sends non-blocking, so 10,000 rapid events against
// a full channel with no reader return immediately and stay dropped for the
// next poll tick to replay from the journal; the Notice adapter appends to
// the backlog with the same non-blocking guarantee, retaining all 1,000
// rapid notices in order (AC-6.2 slow-terminal case).
func TestLiveCallbackNeverBlocks(t *testing.T) {
	t.Parallel()
	events, notices, hooks := LiveFeed()
	if cap(events) != 256 {
		t.Fatalf("cap(events) = %d, want 256", cap(events))
	}
	if notices == nil || hooks.Event == nil || hooks.Notice == nil {
		t.Fatal("LiveFeed returned a nil backlog or callback")
	}
	if hooks.Interrupt != nil {
		t.Error("LiveFeed sets Interrupt; task 12 owns it")
	}
	// Fill the channel with no reader, then prove rapid Events drop
	// without blocking: a blocking send would hang past the timeout.
	fill := journal.Event{Type: "attempt.progress", Payload: json.RawMessage(`{}`)}
	for range cap(events) {
		events <- fill
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 10000 {
			hooks.Event(journal.Event{
				Type:        "attempt.progress",
				Payload:     json.RawMessage(fmt.Sprintf(`{"assistant_turns":%d}`, i)),
				RunSequence: int64(1000 + i),
			})
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("10,000 rapid Events blocked against a full channel")
	}
	if got := len(events); got != cap(events) {
		t.Errorf("len(events) = %d after 10,000 drops, want the full %d", got, cap(events))
	}
	// Notices bypass the full channel into the backlog: 1,000 rapid
	// notices return immediately and are all retained in order.
	noticeDone := make(chan struct{})
	go func() {
		defer close(noticeDone)
		for i := range 1000 {
			hooks.Notice(fmt.Sprintf("notice %04d", i))
		}
	}()
	select {
	case <-noticeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("1,000 rapid Notices blocked against a full channel")
	}
	drained := notices.Drain()
	if len(drained) != 1000 {
		t.Fatalf("Drain kept %d notices, want all 1,000", len(drained))
	}
	for i, s := range drained {
		if want := fmt.Sprintf("notice %04d", i); s != want {
			t.Fatalf("notice %d = %q, want %q (order lost)", i, s, want)
		}
	}
	if got := len(events); got != cap(events) {
		t.Errorf("len(events) = %d after Notices, want the full %d (notices must bypass the channel)", got, cap(events))
	}
	// The next poll replays dropped events from the journal: Load scans
	// from zero independently of the channel, so journaled events survive
	// any amount of dropping.
	b := newJournalBuilder(t)
	runID := "run_live_drop"
	taskSHA := b.writeTask(runID, []byte("# Live drop replay\n"))
	b.addRun(runID, "executing", taskSHA)
	b.addAttempt(runID, "running")
	b.append(runID, "attempt.progress", `{"assistant_turns":7}`, nil)
	b.close()
	snap, err := viewmodel.Load(t.Context(), b.dir, runID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if snap.LastRunSeq != 3 {
		t.Errorf("LastRunSeq = %d, want 3 journaled events replayed despite drops", snap.LastRunSeq)
	}
	if snap.LatestProgress == nil || snap.LatestProgress.AssistantTurns != 7 {
		t.Errorf("replayed progress = %+v, want the journaled 7 turns", snap.LatestProgress)
	}
}

// TestNoticeBacklogDrainsToNoticeLines pins the ui.notice path: Drain
// returns every retained notice in order and empties the backlog, and a
// model tick renders each drained string as a notice line from a synthetic
// event (RunSequence -1, payload {"text":...}), never as journal progress.
func TestNoticeBacklogDrainsToNoticeLines(t *testing.T) {
	t.Parallel()
	t.Run("drain order", func(t *testing.T) {
		t.Parallel()
		var b NoticeBacklog
		b.Add("first")
		b.Add("second")
		b.Add("third")
		got := b.Drain()
		if len(got) != 3 || got[0] != "first" || got[1] != "second" || got[2] != "third" {
			t.Fatalf("Drain = %q, want the three notices in order", got)
		}
		if again := b.Drain(); len(again) != 0 {
			t.Errorf("second Drain = %q, want empty", again)
		}
	})
	t.Run("nil safe", func(t *testing.T) {
		t.Parallel()
		var b *NoticeBacklog
		b.Add("dropped")
		if got := b.Drain(); got != nil {
			t.Errorf("nil Drain = %q, want nil", got)
		}
	})
	t.Run("tick renders notices", func(t *testing.T) {
		t.Parallel()
		m := newModel(testConfig(fullCaps, t))
		m.loadDiff = stubDiff(mustParseDiff(t, smallDiffInput))
		m.setSnapshot(t.Context(), richSnapshot())
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
		m = updated.(*Model)
		m.loadSnapshot = func(context.Context, string, string) (viewmodel.Snapshot, error) {
			return richSnapshot(), nil
		}
		backlog := &NoticeBacklog{}
		backlog.Add("first notice")
		backlog.Add("mythhelm recover run_t6")
		m.cfg.Notices = backlog
		if m.snap.LatestProgress == nil || m.snap.LatestProgress.AssistantTurns != 7 {
			t.Fatalf("pre-tick progress = %+v, want the 7 turns", m.snap.LatestProgress)
		}
		updated, cmd := m.Update(pollTickMsg{})
		mm, ok := updated.(*Model)
		if !ok {
			t.Fatalf("Update returned %T, want *Model", updated)
		}
		m = mm
		if cmd == nil {
			t.Error("poll tick scheduled no follow-up tick")
		}
		if len(m.notices) != 2 {
			t.Fatalf("notices = %d events, want 2 drained", len(m.notices))
		}
		for i, want := range []string{"first notice", "mythhelm recover run_t6"} {
			ev := m.notices[i]
			if ev.Type != "ui.notice" || ev.RunSequence != -1 {
				t.Errorf("notice %d = type %q seq %d, want ui.notice/-1", i, ev.Type, ev.RunSequence)
			}
			var p struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(ev.Payload, &p); err != nil || p.Text != want {
				t.Errorf("notice %d payload = %s, want {\"text\":%q}", i, ev.Payload, want)
			}
		}
		if p := m.snap.LatestProgress; p == nil || p.AssistantTurns != 7 {
			t.Errorf("post-tick progress = %+v, want the 7 turns (notices are never journal progress)", p)
		}
		if left := backlog.Drain(); len(left) != 0 {
			t.Errorf("backlog holds %d notices after the tick, want drained", len(left))
		}
		view := m.View()
		for _, want := range []string{"notice: first notice", "notice: mythhelm recover run_t6"} {
			if !strings.Contains(view, want) {
				t.Errorf("view lacks %q\n%s", want, view)
			}
		}
	})
}

// TestCoalescedReload pins AC-6.1 coalescing: rapid event bursts fold
// advisory without rebuilding, and one poll tick rebuilds exactly once.
// Per-event rebuilds fail the event-vs-reload count.
func TestCoalescedReload(t *testing.T) {
	t.Parallel()
	if livePollInterval != 200*time.Millisecond {
		t.Fatalf("livePollInterval = %v, want 200ms", livePollInterval)
	}
	m := newModel(testConfig(fullCaps, t))
	m.setSnapshot(t.Context(), dispatchSnapshot())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	m = updated.(*Model)
	reloads := 0
	reloaded := dispatchSnapshot()
	progress := &viewmodel.Progress{AssistantTurns: 7, ToolUses: map[string]int{"read": 3}, Retries: 1, RunSeq: 99}
	reloaded.LatestProgress = progress
	m.loadSnapshot = func(context.Context, string, string) (viewmodel.Snapshot, error) {
		reloads++
		return reloaded, nil
	}
	const bursts = 50
	for i := range bursts {
		var cmd tea.Cmd
		m, cmd = motionFeed(t, m, liveEvent("attempt.progress", liveProgressPayload, "att_1", int64(100+i)))
		_ = cmd
		if reloads != 0 {
			t.Fatalf("reloads = %d after %d events, want 0 (events must not rebuild)", reloads, i+1)
		}
	}
	if p := m.snap.LatestProgress; p == nil || p.AssistantTurns != 7 {
		t.Fatalf("folded progress = %+v, want the burst counters (events were ignored, not folded)", p)
	}
	updated, cmd := m.Update(pollTickMsg{})
	mm, ok := updated.(*Model)
	if !ok {
		t.Fatalf("Update returned %T, want *Model", updated)
	}
	m = mm
	if cmd == nil {
		t.Error("poll tick scheduled no follow-up tick")
	}
	if reloads != 1 {
		t.Fatalf("reloads = %d after %d events and one tick, want exactly 1", reloads, bursts)
	}
	if p := m.snap.LatestProgress; p == nil || p.AssistantTurns != 7 {
		t.Errorf("reloaded progress = %+v, want the converged 7 turns", p)
	}
	updated, cmd = m.Update(pollTickMsg{})
	if _, ok := updated.(*Model); !ok {
		t.Fatalf("Update returned %T, want *Model", updated)
	}
	if cmd == nil {
		t.Error("second poll tick scheduled no follow-up tick")
	}
	if reloads != 2 {
		t.Errorf("reloads = %d after two ticks, want exactly one rebuild per tick", reloads)
	}
}

// TestViewportVirtualises pins AC-6.2: a 10,000-line stream renders at most
// the pane's visible rows, with the view row count bounded by the pane
// height; notice tails and the truncation notice keep their rows.
func TestViewportVirtualises(t *testing.T) {
	t.Parallel()
	t.Run("diff stream", func(t *testing.T) {
		t.Parallel()
		const stream = 10000
		lines := make([]DiffLine, stream)
		for i := range lines {
			lines[i] = DiffLine{Kind: DiffContext, Text: fmt.Sprintf("context line %05d", i)}
		}
		m := newModel(testConfig(fullCaps, t))
		m.loadDiff = stubDiff(Diff{Lines: lines})
		m.setSnapshot(t.Context(), richSnapshot())
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
		view := updated.(*Model).View()
		rows := strings.Split(view, "\n")
		if len(rows) > 30 {
			t.Errorf("view has %d rows, height budget is 30", len(rows))
		}
		stripped := stripANSISequences(view)
		if !strings.Contains(stripped, "context line 00000") {
			t.Errorf("virtualised pane hides its head\n%s", view)
		}
		if strings.Contains(stripped, "context line 09999") {
			t.Errorf("virtualised pane renders the 10,000-line tail\n%s", view)
		}
		if !strings.Contains(stripped, "more lines") {
			t.Errorf("virtualised pane carries no truncation marker\n%s", view)
		}
	})
	t.Run("truncation notice keeps its rows", func(t *testing.T) {
		t.Parallel()
		lines := make([]DiffLine, 100)
		for i := range lines {
			lines[i] = DiffLine{Kind: DiffContext, Text: fmt.Sprintf("row %03d", i)}
		}
		d := Diff{Lines: lines, Truncated: true, TruncateCap: truncateLines}
		d.total = 20000
		m := newModel(testConfig(fullCaps, t))
		m.loadDiff = stubDiff(d)
		snap := richSnapshot()
		m.setSnapshot(t.Context(), snap)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
		view := updated.(*Model).View()
		rows := strings.Split(view, "\n")
		if len(rows) > 30 {
			t.Errorf("view has %d rows, height budget is 30", len(rows))
		}
		base, commit := snap.Candidate.BaseRev, snap.Candidate.Commit
		for _, want := range []string{"showing 100 of 20000 lines", "50,000-line limit", "git -C", base, commit} {
			if !strings.Contains(stripANSISequences(view), want) {
				t.Errorf("truncated view lacks %q\n%s", want, view)
			}
		}
	})
	t.Run("notice tail", func(t *testing.T) {
		t.Parallel()
		m := newModel(testConfig(fullCaps, t))
		m.loadDiff = stubDiff(mustParseDiff(t, smallDiffInput))
		m.setSnapshot(t.Context(), richSnapshot())
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
		m = updated.(*Model)
		for i := range 100 {
			m.notices = append(m.notices, noticeEvent(fmt.Sprintf("notice %02d", i), time.Now().UTC()))
		}
		view := m.View()
		rows := strings.Split(view, "\n")
		if len(rows) > 30 {
			t.Errorf("view has %d rows with 100 notices, height budget is 30", len(rows))
		}
		stripped := stripANSISequences(view)
		for _, want := range []string{"notice 99", "notice 98", "more notices"} {
			if !strings.Contains(stripped, want) {
				t.Errorf("notice tail lacks %q\n%s", want, view)
			}
		}
		if strings.Contains(stripped, "notice: notice 00\n") || strings.Contains(stripped, "notice: notice 00 ") {
			t.Errorf("notice tail renders the dropped head\n%s", view)
		}
	})
}

// TestCompactReservesNoticeRows pins notice reservation in the compact
// view: with a verification line and three notices at 160x9, every notice
// renders (detail rows yield, never notices); at 160x5 the five-row offer
// still renders whole.
func TestCompactReservesNoticeRows(t *testing.T) {
	t.Parallel()
	withNotices := func(height int) *Model {
		m := readyModel(t, fullCaps, 160, height)
		for _, s := range []string{"alpha", "beta", "gamma"} {
			m.notices = append(m.notices, noticeEvent(s, time.Now().UTC()))
		}
		return m
	}
	view := withNotices(9).View()
	stripped := stripANSISequences(view)
	for _, want := range []string{"notice: alpha", "notice: beta", "notice: gamma"} {
		if !strings.Contains(stripped, want) {
			t.Errorf("160x9 compact view drops %q\n%s", want, view)
		}
	}
	if rows := len(strings.Split(view, "\n")); rows > 9 {
		t.Errorf("160x9 compact view has %d rows", rows)
	}
	offer := withNotices(5).View()
	for _, want := range []string{"compact", "--accessible"} {
		if !strings.Contains(offer, want) {
			t.Errorf("160x5 compact view lost its offer %q\n%s", want, offer)
		}
	}
}

// TestHistorySearchComplete pins D17: history search replays the journal
// from zero, so a match in the oldest journaled event is found. A bounded
// ring-buffer copy of the tail would miss it.
func TestHistorySearchComplete(t *testing.T) {
	t.Parallel()
	b := newJournalBuilder(t)
	runID := "run_history"
	taskSHA := b.writeTask(runID, []byte("# History replay\n"))
	b.addRun(runID, "executing", taskSHA)
	b.addAttempt(runID, "running")
	b.append(runID, "custom.oldest", `{"text":"ancient-needle-xyz"}`, nil)
	for i := range 49 {
		b.append(runID, "custom.filler", fmt.Sprintf(`{"n":%d}`, i), nil)
	}
	b.close()
	results, err := SearchHistory(t.Context(), b.dir, runID, "ancient-needle")
	if err != nil {
		t.Fatalf("SearchHistory: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("SearchHistory found %d events, want the 1 oldest match", len(results))
	}
	if got := results[0].RunSequence; got != 3 {
		t.Errorf("match run_sequence = %d, want 3 (the oldest journaled event, outside any tail copy)", got)
	}
	if !strings.Contains(string(results[0].Payload), "ancient-needle-xyz") {
		t.Errorf("match payload = %s, want the needle", results[0].Payload)
	}
	upper, err := SearchHistory(t.Context(), b.dir, runID, "ANCIENT-NEEDLE")
	if err != nil || len(upper) != 1 {
		t.Errorf("case-insensitive search = %d events, err %v; want 1", len(upper), err)
	}
	all, err := SearchHistory(t.Context(), b.dir, runID, "")
	if err != nil {
		t.Fatalf("empty query: %v", err)
	}
	if len(all) != 52 {
		t.Errorf("empty query found %d events, want all 52", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].RunSequence <= all[i-1].RunSequence {
			t.Fatalf("history out of order at %d: %d after %d", i, all[i].RunSequence, all[i-1].RunSequence)
		}
	}
	none, err := SearchHistory(t.Context(), b.dir, runID, "no-such-needle")
	if err != nil || len(none) != 0 {
		t.Errorf("missing needle = %d events, err %v; want 0", len(none), err)
	}
}

// TestQuitJoinsGoroutines pins the pump ownership rule: quitting with live
// mode active joins the channel pump, leaving no goroutine behind. It
// asserts the NumGoroutine delta around start/stop, and never runs parallel
// with other tests sharing the runtime count.
func TestQuitJoinsGoroutines(t *testing.T) {
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	baseline := runtime.NumGoroutine()
	events, _, hooks := LiveFeed()
	cfg := testConfig(fullCaps, t)
	cfg.Events = events
	m := newModel(cfg)
	m.runCtx = context.Background()
	received := make(chan journal.Event, 16)
	m.startLivePump(func(ev journal.Event) { received <- ev })
	started := false
	for range 100 {
		if runtime.NumGoroutine() >= baseline+1 {
			started = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !started {
		m.stopLivePump()
		t.Fatalf("pump never started: NumGoroutine = %d, baseline %d", runtime.NumGoroutine(), baseline)
	}
	for i := range 3 {
		hooks.Event(journal.Event{Type: "attempt.progress", Payload: json.RawMessage(`{}`), RunSequence: int64(i + 1)})
	}
	for range 3 {
		select {
		case ev := <-received:
			if ev.Type != "attempt.progress" {
				t.Errorf("pumped type = %q, want attempt.progress", ev.Type)
			}
		case <-time.After(2 * time.Second):
			m.stopLivePump()
			t.Fatal("pump did not forward a live event")
		}
	}
	// Quitting joins the pump: Run calls stopLivePump after the program
	// exits, and the join must return the count to baseline.
	m.stopLivePump()
	joined := false
	for range 100 {
		if runtime.NumGoroutine() == baseline {
			joined = true
			break
		}
		time.Sleep(20 * time.Millisecond)
		runtime.GC()
	}
	if !joined {
		t.Fatalf("NumGoroutine = %d after quit, baseline %d (pump leaked)", runtime.NumGoroutine(), baseline)
	}
	// Stopping twice, or without a pump, is safe and starts nothing.
	m.stopLivePump()
	plain := newModel(testConfig(fullCaps, t))
	plain.startLivePump(func(journal.Event) {})
	plain.stopLivePump()
	if got := runtime.NumGoroutine(); got != baseline {
		t.Errorf("NumGoroutine = %d after nil-channel pump, baseline %d", got, baseline)
	}
}
