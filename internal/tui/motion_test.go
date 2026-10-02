package tui

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/tui/caps"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
)

// motionFeed drives one live event through Update and returns the model and
// command; a non-Model return fails the test.
func motionFeed(t *testing.T, m *Model, ev journal.Event) (*Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(eventMsg{Event: ev})
	mm, ok := updated.(*Model)
	if !ok {
		t.Fatalf("Update(eventMsg) returned %T, want *Model", updated)
	}
	return mm, cmd
}

// liveEvent builds one synthetic journaled event for the motion feed.
func liveEvent(typ, payload, attemptID string, seq int64) journal.Event {
	return journal.Event{
		Type: typ, Payload: json.RawMessage(payload),
		AttemptID: attemptID, RunSequence: seq,
	}
}

// motionFrame drives one motion frame through Update.
func motionFrame(t *testing.T, m *Model) (*Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(motionFrameMsg{})
	mm, ok := updated.(*Model)
	if !ok {
		t.Fatalf("Update(motionFrameMsg) returned %T, want *Model", updated)
	}
	return mm, cmd
}

// settleMotion drives motion frames until no further frame is scheduled and
// returns the frame count. A self-rescheduling tick fails instead of
// hanging the test.
func settleMotion(t *testing.T, m *Model) (*Model, int) {
	t.Helper()
	for i := 1; i <= 10; i++ {
		var cmd tea.Cmd
		m, cmd = motionFrame(t, m)
		if cmd == nil {
			return m, i
		}
	}
	t.Fatalf("motion still scheduling after 10 frames: self-rescheduling tick")
	return m, 10
}

var ansiSequence = regexp.MustCompile("\x1b\\[[0-9;]*m")

// hasDigit matches any decimal digit: waiting indicators and stage strips
// must never carry one.
var hasDigit = regexp.MustCompile(`[0-9]`)

// stripANSISequences removes SGR styling so tests match on row text.
func stripANSISequences(s string) string {
	return ansiSequence.ReplaceAllString(s, "")
}

// segments splits the stripped view into pane segments: multi-column rows
// join panes with the bar glyph, so a pane's row is a mid-line segment.
func segments(view string) []string {
	var out []string
	for _, row := range strings.Split(stripANSISequences(view), "\n") {
		parts := strings.Split(row, "│")
		if len(parts) == 1 {
			parts = strings.Split(row, "|")
		}
		for _, p := range parts {
			out = append(out, strings.TrimSpace(p))
		}
	}
	return out
}

// segmentWithPrefix returns the first pane segment with the given prefix.
func segmentWithPrefix(view, prefix string) (string, bool) {
	for _, seg := range segments(view) {
		if strings.HasPrefix(seg, prefix) {
			return seg, true
		}
	}
	return "", false
}

// asciiMotionCaps keeps full motion with ASCII icons and no colour, so
// marker tests isolate the icon set from the motion level.
var asciiMotionCaps = caps.Caps{Colour: caps.ColourNever, Motion: caps.MotionFull, Icons: caps.IconsASCII}

// dispatchSnapshot is an executing run whose attempt projection already
// reads running but whose launch acknowledgement this session has not
// consumed: no progress, no native result, nothing terminal.
func dispatchSnapshot() viewmodel.Snapshot {
	snap := richSnapshot()
	snap.Run.State = "executing"
	snap.Attempt.State = "running"
	snap.LatestProgress = nil
	snap.NativeExit = nil
	snap.Candidate = nil
	snap.Verification = nil
	return snap
}

// dispatchModel builds a wide model over the dispatch fixture.
func dispatchModel(t *testing.T, c caps.Caps) *Model {
	t.Helper()
	m := newModel(testConfig(c, t))
	m.setSnapshot(t.Context(), dispatchSnapshot())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 30})
	mm, ok := updated.(*Model)
	if !ok {
		t.Fatalf("Update returned %T, want *Model", updated)
	}
	return mm
}

const liveProgressPayload = `{"assistant_turns":7,"tool_uses":{"read":3},"retries":1}`

// TestDispatchNeedsLaunchAck pins the moment 3 truth rule: the lane renders
// running only after an attempt.launched event is consumed. A pre-launch
// tick, and an ack for another attempt, both keep the pre-running state.
func TestDispatchNeedsLaunchAck(t *testing.T) {
	t.Parallel()
	m := dispatchModel(t, fullCaps)
	if m.motion.launched {
		t.Fatal("fresh dispatch model already launched")
	}
	view := m.View()
	if !strings.Contains(view, "dispatching") {
		t.Errorf("pre-launch view lacks the dispatching label\n%s", view)
	}
	if matched, _ := regexp.MatchString(`\brunning\b`, stripANSISequences(view)); matched {
		t.Errorf("pre-launch view claims running before the ack\n%s", view)
	}

	// A pre-launch tick settles the arrival, never the launch.
	m, _ = motionFrame(t, m)
	if m.motion.launched {
		t.Error("a motion tick launched the lane without the ack")
	}
	if view := m.View(); !strings.Contains(view, "dispatching") {
		t.Errorf("post-tick view lost the dispatching label\n%s", view)
	}

	// An ack for another attempt is not this lane's ack.
	m, _ = motionFeed(t, m, liveEvent("attempt.launched", `{}`, "att_other", 20))
	if m.motion.launched {
		t.Error("another attempt's ack launched the lane")
	}
	if view := m.View(); !strings.Contains(view, "dispatching") {
		t.Errorf("foreign-ack view lost the dispatching label\n%s", view)
	}

	// The lane's own ack flips it to running with one settling frame.
	var cmd tea.Cmd
	m, cmd = motionFeed(t, m, liveEvent("attempt.launched", `{}`, "att_1", 21))
	if !m.motion.launched {
		t.Error("the lane's own ack did not launch it")
	}
	if cmd == nil {
		t.Error("launch ack scheduled no dispatch frame under full motion")
	}
	if view := m.View(); !strings.Contains(view, "state: running") {
		t.Errorf("post-ack view lacks the running label\n%s", view)
	}
	if view := m.View(); strings.Contains(view, "dispatching") {
		t.Errorf("post-ack view still dispatching\n%s", view)
	}
	m, cmd = motionFrame(t, m)
	if cmd != nil {
		t.Error("dispatch did not settle in one 80 ms frame")
	}
	if view := m.View(); !strings.Contains(view, "running") {
		t.Errorf("settled view lost the running label\n%s", view)
	}

	// The ASCII twin gates the same label with its own dash.
	ascii := dispatchModel(t, asciiMotionCaps).View()
	if !strings.Contains(stripANSISequences(ascii), "dispatching - waiting for launch acknowledgement") {
		t.Errorf("ASCII pre-launch view lacks the dispatching label\n%s", ascii)
	}
}

// TestRouteCardShowsRecordedAdmission pins the moment 2 truth rule: the
// route card shows the adapter id/version/surface from Snapshot.Admission,
// the profile from the run, and the static pinned-route reason — with no
// deliberation spinner and no invented rationale.
func TestRouteCardShowsRecordedAdmission(t *testing.T) {
	t.Parallel()
	view := readyModel(t, fullCaps, 160, 30).View()
	route, ok := segmentWithPrefix(view, "route:")
	if !ok {
		t.Fatalf("wide view lacks the route card\n%s", view)
	}
	for _, want := range []string{"builtin/fake", "1.0", "local"} {
		if !strings.Contains(route, want) {
			t.Errorf("route card lacks recorded %q: %q", want, route)
		}
	}
	if !strings.Contains(view, "trusted-host") {
		t.Errorf("view lacks the run's execution profile\n%s", view)
	}
	reason, ok := segmentWithPrefix(view, "route reason:")
	if !ok {
		t.Fatalf("route card lacks its reason line\n%s", view)
	}
	if reason != "route reason: pinned by --adapter" {
		t.Errorf("route reason = %q, want exactly the static pinned text", reason)
	}
	card := route + "\n" + reason
	if matched, _ := regexp.MatchString("[\u2800-\u28ff◐◑◒◓]", card); matched {
		t.Errorf("route card renders a deliberation spinner: %q", card)
	}
	for _, bad := range []string{"selecting", "deliberating", "choosing", "thinking", "spinner"} {
		if strings.Contains(strings.ToLower(card), bad) {
			t.Errorf("route card invents deliberation %q: %q", bad, card)
		}
	}

	// The ASCII twin carries the same recorded facts byte-clean.
	ascii := readyModel(t, asciiMotionCaps, 160, 30).View()
	asciiRoute, ok := segmentWithPrefix(ascii, "route:")
	if !ok || !strings.Contains(asciiRoute, "builtin/fake") {
		t.Errorf("ASCII view lacks the recorded route card\n%s", ascii)
	}
	for i := 0; i < len(asciiRoute); i++ {
		if asciiRoute[i] >= 0x80 {
			t.Errorf("ASCII route card holds non-ASCII byte: %q", asciiRoute)
			break
		}
	}

	// Without an admission the card reads unknown, never blank or invented.
	m := newModel(testConfig(fullCaps, t))
	snap := richSnapshot()
	snap.Admission = nil
	m.setSnapshot(t.Context(), snap)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 30})
	if view := updated.(*Model).View(); !strings.Contains(view, "route: unknown") {
		t.Errorf("admission-less view lacks route: unknown\n%s", view)
	}
}

// TestLivePulseHasNoTypewriterDelay pins the moment 4 truth rule: the
// activity pulse renders the full counters in the same update the
// attempt.progress event is consumed. A character-at-a-time reveal fails,
// because the complete strings are already present after one Update.
func TestLivePulseHasNoTypewriterDelay(t *testing.T) {
	t.Parallel()
	m := dispatchModel(t, fullCaps)
	var cmd tea.Cmd
	m, cmd = motionFeed(t, m, liveEvent("attempt.progress", liveProgressPayload, "att_1", 22))
	if cmd == nil {
		t.Error("progress event scheduled no pulse frame under full motion")
	}
	p := m.snap.LatestProgress
	if p == nil || p.AssistantTurns != 7 || p.Retries != 1 || p.ToolUses["read"] != 3 {
		t.Fatalf("folded progress = %+v, want the full counters", p)
	}
	view := m.View()
	for _, want := range []string{"7 assistant turns (native-reported)", "retries: 1"} {
		if !strings.Contains(view, want) {
			t.Errorf("same-update view lacks full counter %q\n%s", want, view)
		}
	}
	activity, ok := segmentWithPrefix(view, "●")
	if !ok || !strings.Contains(activity, "7 assistant turns") {
		t.Errorf("pulse marker missing from the activity row\n%s", view)
	}

	// Malformed progress keeps the latest good value (advisory, like Load)
	// while the pulse still fires: it means transport alive, not proven.
	m, cmd = motionFeed(t, m, liveEvent("attempt.progress", `{"assistant_turns":`, "att_1", 23))
	if cmd == nil {
		t.Error("malformed progress scheduled no pulse frame")
	}
	if got := m.snap.LatestProgress.AssistantTurns; got != 7 {
		t.Errorf("malformed progress overwrote the latest good value: %d", got)
	}

	// The pulse settles within two frames with the counters unchanged.
	m, frames := settleMotion(t, m)
	if frames > motionPulseFrames {
		t.Errorf("pulse settled in %d frames, want at most %d", frames, motionPulseFrames)
	}
	if view := m.View(); !strings.Contains(view, "7 assistant turns (native-reported)") {
		t.Errorf("settled view lost the counters\n%s", view)
	}
	if activity, _ := segmentWithPrefix(m.View(), "●"); strings.Contains(activity, "assistant turns") {
		t.Errorf("settled view still pulses: %q", activity)
	}

	// The ASCII twin pulses with its own marker and the same full counters.
	ascii := dispatchModel(t, asciiMotionCaps)
	ascii, _ = motionFeed(t, ascii, liveEvent("attempt.progress", liveProgressPayload, "att_1", 22))
	asciiView := ascii.View()
	if !strings.Contains(asciiView, "7 assistant turns (native-reported)") {
		t.Errorf("ASCII same-update view lacks the full counters\n%s", asciiView)
	}
	if activity, ok := segmentWithPrefix(asciiView, "*"); !ok || !strings.Contains(activity, "assistant turns") {
		t.Errorf("ASCII pulse marker missing from the activity row\n%s", asciiView)
	}
}

// TestDeliveryRevealsActualResult pins the moment 9 truth rule: the
// ready-for-review reveal renders the recorded outcome — verified,
// unverified or failed — and settles within 200 ms simulated. Generic
// success copy on a failed run fails.
func TestDeliveryRevealsActualResult(t *testing.T) {
	t.Parallel()
	commit := strings.Repeat("c", 40)
	verifiedSnap := func() viewmodel.Snapshot {
		s := richSnapshot()
		s.Run.State = "ready_for_review"
		return s
	}
	unverifiedSnap := func() viewmodel.Snapshot {
		s := richSnapshot()
		s.Run.State = "ready_for_review"
		s.Verification = nil
		return s
	}
	failedSnap := func() viewmodel.Snapshot {
		s := richSnapshot()
		s.Goal = "Ship the fix"
		s.Run.State = "failed"
		s.Run.Reason = "worker lost"
		s.Verification = nil
		s.Candidate = nil
		return s
	}
	cases := []struct {
		name    string
		snap    func() viewmodel.Snapshot
		trigger journal.Event
		outcome string
	}{
		{"verified", verifiedSnap, liveEvent("verification.completed", `{}`, "att_1", 30), "outcome: verified " + commit[:7]},
		{"unverified", unverifiedSnap, liveEvent("receipt.written", `{}`, "", 31), "outcome: unverified"},
		{"failed", failedSnap, liveEvent("attempt.native_result", `{"exit_code":1,"result_observed":true}`, "att_1", 32), "outcome: failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := newModel(testConfig(fullCaps, t))
			m.loadDiff = stubDiff(mustParseDiff(t, smallDiffInput))
			m.setSnapshot(t.Context(), tc.snap())
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 30})
			m = updated.(*Model)
			var cmd tea.Cmd
			m, cmd = motionFeed(t, m, tc.trigger)
			if cmd == nil {
				t.Fatal("delivery trigger scheduled no reveal frame")
			}
			outcome, ok := segmentWithPrefix(m.View(), "outcome:")
			if !ok {
				t.Fatalf("reveal lacks the outcome line\n%s", m.View())
			}
			if outcome != tc.outcome {
				t.Errorf("outcome = %q, want %q", outcome, tc.outcome)
			}
			m, frames := settleMotion(t, m)
			if simulated := time.Duration(frames) * motionFrameInterval; simulated > 200*time.Millisecond {
				t.Errorf("reveal settled in %v simulated, want ≤200 ms", simulated)
			}
			if _, ok := segmentWithPrefix(m.View(), "outcome:"); ok {
				t.Errorf("settled view still reveals\n%s", m.View())
			}
		})
	}

	// The failed reveal never borrows success copy: no verified label, no
	// success word — the recorded failure stands on its own.
	m := newModel(testConfig(fullCaps, t))
	m.setSnapshot(t.Context(), failedSnap())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 30})
	m = updated.(*Model)
	m, _ = motionFeed(t, m, liveEvent("attempt.native_result", `{"exit_code":1,"result_observed":true}`, "att_1", 32))
	stripped := stripANSISequences(m.View())
	if matched, _ := regexp.MatchString(`\bverified\b`, stripped); matched {
		t.Errorf("failed reveal renders a verified label\n%s", m.View())
	}
	if strings.Contains(strings.ToLower(stripped), "success") {
		t.Errorf("failed reveal renders success copy\n%s", m.View())
	}
	if !strings.Contains(stripped, "worker lost") {
		t.Errorf("failed reveal hides the recorded reason\n%s", m.View())
	}
}

// TestArrivalInterruptible pins the moment 1 truth rule: any key skips the
// arrival moment to the loaded view, and the first paint never waits on it.
func TestArrivalInterruptible(t *testing.T) {
	t.Parallel()
	// The arrival's settling tick fires once, then never again.
	m := readyModel(t, fullCaps, 160, 30)
	if !m.motion.arrival {
		t.Fatal("fresh full-motion model has no pending arrival")
	}
	if m.Init() == nil {
		t.Fatal("Init scheduled no arrival tick under full motion")
	}
	m, cmd := motionFrame(t, m)
	if m.motion.arrival {
		t.Error("one frame did not settle the arrival")
	}
	if cmd != nil {
		t.Error("settled arrival scheduled a further frame")
	}

	// Every key skips a pending arrival and still acts; the first paint —
	// taken before any frame — already shows the loaded view.
	keys := []struct {
		name  string
		msg   tea.KeyMsg
		check func(*Model) bool
	}{
		{"slash opens the filter", runeMsg("/"), func(m *Model) bool { return m.filterOn }},
		{"q opens exit options", runeMsg("q"), func(m *Model) bool { return m.dialog == dialogExitOptions }},
		{"tab moves focus", specialMsg(tea.KeyTab), func(m *Model) bool { return m.focusPane == paneAgents }},
		{"enter opens details", specialMsg(tea.KeyEnter), func(m *Model) bool { return m.focusPane == paneDetail }},
		{"esc clears the arrival", specialMsg(tea.KeyEsc), func(m *Model) bool { return true }},
	}
	for _, tc := range keys {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := readyModel(t, fullCaps, 160, 30)
			first := m.View()
			for _, want := range []string{"Add pagination to the audit log", "Next action:"} {
				if !strings.Contains(first, want) {
					t.Fatalf("first paint waits on the arrival: missing %q\n%s", want, first)
				}
			}
			m, _ = pressKey(t, m, tc.msg)
			if m.motion.arrival {
				t.Errorf("key %q did not skip the arrival", tc.name)
			}
			if !tc.check(m) {
				t.Errorf("key %q was swallowed by the arrival", tc.name)
			}
			// The dialog overlay takes over the whole view by design; the
			// snapshot underneath stays loaded either way.
			if m.dialog == "" {
				if view := m.View(); !strings.Contains(view, "Add pagination to the audit log") {
					t.Errorf("post-key view lost the loaded snapshot\n%s", view)
				}
			} else if !m.ready {
				t.Errorf("key %q left the model unready", tc.name)
			}
		})
	}

	// Reduced and off never pend an arrival: the paint is the reveal.
	for _, level := range []caps.Caps{
		{Colour: caps.ColourFull, Motion: caps.MotionReduced, Icons: caps.IconsUnicode},
		{Colour: caps.ColourFull, Motion: caps.MotionOff, Icons: caps.IconsUnicode},
	} {
		m := newModel(testConfig(level, t))
		if m.motion.arrival {
			t.Errorf("caps %+v pends an arrival", level)
		}
		if m.Init() != nil {
			t.Errorf("caps %+v schedules an arrival tick", level)
		}
	}
}

// TestNoWaitingCountdown pins the moment 6 truth rule: the waiting
// indicator for a run with no known reset or deadline carries a static
// reason and no digits-based countdown.
func TestNoWaitingCountdown(t *testing.T) {
	t.Parallel()
	waiting := []struct {
		state  string
		reason string
	}{
		{"created", "admission in progress"},
		{"admission", "admission in progress"},
		{"stopping", "stop requested, worker confirming"},
	}
	for _, tc := range waiting {
		t.Run(tc.state, func(t *testing.T) {
			t.Parallel()
			snap := richSnapshot()
			snap.Run.State = tc.state
			snap.Candidate = nil
			snap.Verification = nil
			if tc.state != "stopping" {
				snap.Attempt = nil
				snap.LatestProgress = nil
			}
			m := newModel(testConfig(fullCaps, t))
			m.setSnapshot(t.Context(), snap)
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 30})
			indicator, ok := segmentWithPrefix(updated.(*Model).View(), "waiting")
			if !ok {
				t.Fatalf("%s view lacks the waiting indicator\n%s", tc.state, updated.(*Model).View())
			}
			if !strings.Contains(indicator, tc.reason) {
				t.Errorf("indicator = %q, want the static reason %q", indicator, tc.reason)
			}
			if hasDigit.MatchString(indicator) {
				t.Errorf("indicator carries a digits-based countdown: %q", indicator)
			}
			for _, bad := range []string{"countdown", "remaining", "ETA", "sec"} {
				if strings.Contains(indicator, bad) {
					t.Errorf("indicator carries countdown copy %q: %q", bad, indicator)
				}
			}
		})
	}

	// The ASCII twin counts down no more than the unicode one.
	snap := richSnapshot()
	snap.Run.State = "stopping"
	snap.Candidate = nil
	snap.Verification = nil
	m := newModel(testConfig(asciiMotionCaps, t))
	m.setSnapshot(t.Context(), snap)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 30})
	indicator, ok := segmentWithPrefix(updated.(*Model).View(), "waiting")
	if !ok {
		t.Fatalf("ASCII stopping view lacks the indicator\n%s", updated.(*Model).View())
	}
	if hasDigit.MatchString(indicator) {
		t.Errorf("ASCII indicator carries digits: %q", indicator)
	}

	// Non-waiting states show no waiting indicator at all.
	for _, state := range []string{"executing", "verifying", "ready_for_review", "blocked", "failed", "interrupted", "recovering", "applying", "completed", "cancelled"} {
		t.Run("no indicator when "+state, func(t *testing.T) {
			t.Parallel()
			snap := richSnapshot()
			snap.Run.State = state
			if state != "ready_for_review" {
				snap.Verification = nil
			}
			m := newModel(testConfig(fullCaps, t))
			m.setSnapshot(t.Context(), snap)
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 30})
			if line, ok := segmentWithPrefix(updated.(*Model).View(), "waiting"); ok {
				t.Errorf("%s view shows a waiting indicator: %q", state, line)
			}
		})
	}
}

// TestNoIntegrationPercentage pins the moment 8 truth rule: the
// integration strip lights frozen, checked and ready strictly from
// recorded rows, with stage names only — never a percentage.
func TestNoIntegrationPercentage(t *testing.T) {
	t.Parallel()
	strip := func(snap viewmodel.Snapshot) string {
		t.Helper()
		m := newModel(testConfig(fullCaps, t))
		m.setSnapshot(t.Context(), snap)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 30})
		line, ok := segmentWithPrefix(updated.(*Model).View(), "stages:")
		if !ok {
			t.Fatalf("view lacks the integration strip\n%s", updated.(*Model).View())
		}
		return line
	}
	t.Run("nothing recorded", func(t *testing.T) {
		t.Parallel()
		snap := richSnapshot()
		snap.Run.State = "executing"
		snap.Candidate = nil
		snap.Verification = nil
		if got, want := strip(snap), "stages: [wait] frozen  [wait] checked  [wait] ready"; got != want {
			t.Errorf("strip = %q, want %q", got, want)
		}
	})
	t.Run("frozen only", func(t *testing.T) {
		t.Parallel()
		snap := richSnapshot()
		snap.Run.State = "verifying"
		snap.Verification = nil
		if got, want := strip(snap), "stages: [done] frozen  [wait] checked  [wait] ready"; got != want {
			t.Errorf("strip = %q, want %q", got, want)
		}
	})
	t.Run("frozen checked ready", func(t *testing.T) {
		t.Parallel()
		if got, want := strip(richSnapshot()), "stages: [done] frozen  [done] checked  [done] ready"; got != want {
			t.Errorf("strip = %q, want %q", got, want)
		}
	})
	t.Run("never a percentage", func(t *testing.T) {
		t.Parallel()
		for _, snap := range []viewmodel.Snapshot{richSnapshot(), dispatchSnapshot()} {
			line := strip(snap)
			for _, name := range []string{"frozen", "checked", "ready"} {
				if !strings.Contains(line, name) {
					t.Errorf("strip lacks stage %q: %q", name, line)
				}
			}
			if strings.Contains(line, "%") {
				t.Errorf("strip renders a percentage: %q", line)
			}
			if hasDigit.MatchString(line) {
				t.Errorf("strip renders digits without a denominator: %q", line)
			}
		}
	})
}

// TestFailureWithoutSpectacle pins the moment 10 truth rule: failure
// rendering moves focus to the pane holding the actionable error plus the
// preserved artefact, emphasised statically — with no ANSI blink or
// reverse-flash sequences anywhere in the view.
func TestFailureWithoutSpectacle(t *testing.T) {
	t.Parallel()
	failedSnap := func(state string) viewmodel.Snapshot {
		snap := richSnapshot()
		snap.Goal = "Ship the fix"
		snap.Run.State = state
		snap.Run.Reason = "worker lost"
		snap.Verification = nil
		snap.Candidate = nil
		return snap
	}
	for _, state := range []string{"failed", "blocked"} {
		t.Run("transition focuses the error when "+state, func(t *testing.T) {
			t.Parallel()
			m := newModel(testConfig(fullCaps, t))
			m.setSnapshot(t.Context(), dispatchSnapshot())
			m.focusPane = paneDetail
			m.setSnapshot(t.Context(), failedSnap(state))
			if m.focusPane != paneTasks {
				t.Errorf("failure transition left focus on %q, want tasks", m.focusPane)
			}
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 30})
			m = updated.(*Model)
			view := m.View()
			for _, want := range []string{"worker lost", "preserved: /tmp/tui ws"} {
				if !strings.Contains(stripANSISequences(view), want) {
					t.Errorf("failure view lacks %q\n%s", want, view)
				}
			}
			if strings.Contains(view, "\x1b[5m") || strings.Contains(view, "\x1b[7m") {
				t.Errorf("failure view flashes or blinks\n%q", view)
			}
			if !strings.Contains(view, "\x1b[38;2") {
				t.Errorf("failure view carries no static colour emphasis\n%q", view)
			}
		})
	}
	t.Run("first failed load focuses the error", func(t *testing.T) {
		t.Parallel()
		m := newModel(testConfig(fullCaps, t))
		m.setSnapshot(t.Context(), failedSnap("failed"))
		if m.focusPane != paneTasks {
			t.Errorf("first failed load left focus on %q, want tasks", m.focusPane)
		}
	})
	t.Run("steady reloads preserve focus", func(t *testing.T) {
		t.Parallel()
		m := newModel(testConfig(fullCaps, t))
		m.setSnapshot(t.Context(), failedSnap("failed"))
		m.focusPane = paneDetail
		m.setSnapshot(t.Context(), failedSnap("failed"))
		if m.focusPane != paneDetail {
			t.Errorf("steady failed reload yanked focus to %q", m.focusPane)
		}
	})
	t.Run("ascii failure is plain text", func(t *testing.T) {
		t.Parallel()
		m := newModel(testConfig(asciiCaps, t))
		m.setSnapshot(t.Context(), failedSnap("failed"))
		updated, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 30})
		view := updated.(*Model).View()
		if strings.Contains(view, "\x1b") {
			t.Errorf("ASCII failure view emits escapes\n%q", view)
		}
		for _, want := range []string{"worker lost", "preserved: /tmp/tui ws"} {
			if !strings.Contains(view, want) {
				t.Errorf("ASCII failure view lacks %q\n%s", want, view)
			}
		}
	})
}

// TestReducedMotionStatic pins AC-4.2: with MotionReduced or MotionOff,
// triggering every moment applies its state change immediately with static
// emphasis and schedules zero tick-driven frames.
func TestReducedMotionStatic(t *testing.T) {
	t.Parallel()
	levels := []struct {
		name string
		caps caps.Caps
	}{
		{"reduced", caps.Caps{Colour: caps.ColourFull, Motion: caps.MotionReduced, Icons: caps.IconsUnicode}},
		{"off", caps.Caps{Colour: caps.ColourFull, Motion: caps.MotionOff, Icons: caps.IconsUnicode}},
	}
	for _, level := range levels {
		t.Run(level.name, func(t *testing.T) {
			t.Parallel()
			snap := dispatchSnapshot()
			snap.Admission = nil
			m := newModel(testConfig(level.caps, t))
			m.setSnapshot(t.Context(), snap)
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 220, Height: 30})
			m = updated.(*Model)
			if m.Init() != nil {
				t.Error("Init scheduled a frame")
			}
			triggers := []journal.Event{
				liveEvent("attempt.launched", `{}`, "att_1", 40),
				liveEvent("attempt.progress", liveProgressPayload, "att_1", 41),
				liveEvent("attempt.native_result", `{"exit_code":0,"result_observed":true}`, "att_1", 42),
				liveEvent("verification.completed", `{}`, "att_1", 43),
				liveEvent("receipt.written", `{}`, "", 44),
				liveEvent("admission.decided", `{"adapter":{"id":"builtin/fake","version":"2.0","surface":"cli"},"billing":{"qualified":false,"paid_continuation":"off"}}`, "", 45),
			}
			for _, ev := range triggers {
				var cmd tea.Cmd
				m, cmd = motionFeed(t, m, ev)
				if cmd != nil {
					t.Errorf("%s scheduled a frame under %s motion", ev.Type, level.name)
				}
			}
			var cmd tea.Cmd
			m, cmd = motionFrame(t, m)
			if cmd != nil {
				t.Errorf("stray frame scheduled a further frame under %s motion", level.name)
			}
			// Every state change applied immediately, with static emphasis.
			if !m.motion.launched {
				t.Errorf("%s motion did not apply the launch", level.name)
			}
			view := m.View()
			for _, want := range []string{
				"7 assistant turns (native-reported)",
				"retries: 1",
				"route: builtin/fake 2.0 (cli)",
				"route reason: pinned by --adapter",
				"native exit: 0",
			} {
				if !strings.Contains(stripANSISequences(view), want) {
					t.Errorf("%s motion lacks immediate %q\n%s", level.name, want, view)
				}
			}
			if activity, _ := segmentWithPrefix(view, "●"); strings.Contains(activity, "assistant turns") {
				t.Errorf("%s motion pulses: %q", level.name, activity)
			}
			if _, ok := segmentWithPrefix(view, "outcome:"); ok {
				t.Errorf("%s motion runs a delivery reveal\n%s", level.name, view)
			}
			// The recorded outcome still shows statically, without a reveal.
			m.setSnapshot(t.Context(), richSnapshot())
			m, cmd = motionFeed(t, m, liveEvent("verification.completed", `{}`, "att_1", 46))
			if cmd != nil {
				t.Errorf("verified delivery scheduled a frame under %s motion", level.name)
			}
			if view := m.View(); !strings.Contains(view, "verified ccccccc") {
				t.Errorf("%s motion hides the recorded outcome\n%s", level.name, view)
			}
			// Failure still focuses the error immediately.
			failed := richSnapshot()
			failed.Run.State = "failed"
			failed.Run.Reason = "worker lost"
			failed.Verification = nil
			m.focusPane = paneDetail
			m.setSnapshot(t.Context(), failed)
			if m.focusPane != paneTasks {
				t.Errorf("%s motion did not focus the error", level.name)
			}
		})
	}
}

// TestNoPermanentLoop pins AC-6.1: with MotionFull and no new events, the
// program schedules no further motion frames after the transitions settle
// (~200 ms simulated). Only motion commands are counted: the permanent
// 200 ms poll tick (task 10, design §13) is a snapshot reload on a
// distinct message, excluded by construction — counting it would fail a
// correct implementation.
func TestNoPermanentLoop(t *testing.T) {
	t.Parallel()
	m := readyModel(t, fullCaps, 160, 30)
	if m.Init() == nil {
		t.Fatal("Init scheduled no arrival tick under full motion")
	}
	m, cmd := motionFrame(t, m)
	if cmd != nil {
		t.Fatal("arrival did not settle in one frame")
	}
	// Fire every arming moment at once; the shared tick chain counts them
	// down concurrently, so the worst case stays two frames.
	for _, ev := range []journal.Event{
		liveEvent("attempt.launched", `{}`, "att_1", 50),
		liveEvent("attempt.progress", liveProgressPayload, "att_1", 51),
		liveEvent("attempt.native_result", `{"exit_code":0,"result_observed":true}`, "att_1", 52),
		liveEvent("verification.completed", `{}`, "att_1", 53),
		liveEvent("receipt.written", `{}`, "", 54),
	} {
		var cmd tea.Cmd
		m, cmd = motionFeed(t, m, ev)
		if cmd == nil {
			t.Errorf("%s armed no frame under full motion", ev.Type)
		}
	}
	m, frames := settleMotion(t, m)
	if simulated := time.Duration(frames) * motionFrameInterval; simulated > 200*time.Millisecond {
		t.Errorf("moments settled in %v simulated, want ≤200 ms", simulated)
	}
	if m.motion.pending() {
		t.Errorf("settled motion still pends: %+v", m.motion)
	}
	// After the settle, silence: further frames schedule nothing.
	for i := 0; i < 3; i++ {
		var cmd tea.Cmd
		m, cmd = motionFrame(t, m)
		if cmd != nil {
			t.Fatalf("post-settle frame %d rescheduled motion", i+1)
		}
	}
}
