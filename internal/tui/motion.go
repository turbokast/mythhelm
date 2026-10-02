// Truthful motion moments (design §11): the §15.6 applicable subset with
// its truth rules plus reduced-motion collapse. Every moment renders only
// recorded state — arrival never delays first paint, dispatch shows running
// only after the launch acknowledgement is consumed, the live pulse never
// delays counters, the delivery reveal shows the recorded outcome, waiting
// carries no countdown, integration carries no percentage, and failure
// moves focus without spectacle. Reduced/off motion collapses every moment
// to an immediate state change with static emphasis (AC-4.2); full motion
// settles within two 80 ms frames (160 ms ≤ 200 ms, AC-6.1) and schedules
// no permanent loop.
package tui

import (
	"encoding/json"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/tui/caps"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
)

// motionFrameInterval is the single cadence every moment ticks on; two
// frames (160 ms) stay inside the 80–200 ms transition budget (AC-6.1).
const motionFrameInterval = 80 * time.Millisecond

// Per-moment frame budgets. Moments count down concurrently on one shared
// tick chain, so the worst case is two frames whatever combination fired.
const (
	motionDispatchFrames = 1
	motionPulseFrames    = 2
	motionDeliveryFrames = 2
)

// eventMsg carries one journaled event consumed live. Task 10's live pump
// sends these; until then tests feed them synthetically. Folding here is
// advisory and converges with the poll reload (task 10), which rebuilds the
// whole snapshot through viewmodel.Load.
type eventMsg struct {
	Event journal.Event
}

// motionFrameMsg is one scheduled motion frame. It is deliberately distinct
// from task 10's poll tick (a snapshot reload, design §13), so the
// no-permanent-loop assertion can count motion frames without tripping on
// the permanent poll.
type motionFrameMsg struct{}

// motionState holds the session-observed motion facts: whether the arrival
// reveal is still pending, whether the launch acknowledgement was consumed,
// the frames remaining on each active moment, and the previous run state
// for failure-transition detection.
type motionState struct {
	arrival        bool
	launched       bool
	dispatchFrames int
	pulseFrames    int
	deliveryFrames int
	prevRunState   string
}

// pending reports whether any moment still owns frames.
func (s motionState) pending() bool {
	return s.arrival || s.dispatchFrames > 0 || s.pulseFrames > 0 || s.deliveryFrames > 0
}

// scheduleMotion returns the next motion frame tick when full motion is on
// and a moment owns frames, else nil. Tests observe frame scheduling
// through this nil-ness without executing the tick.
func (m *Model) scheduleMotion() tea.Cmd {
	if m.cfg.Caps.Motion != caps.MotionFull || !m.motion.pending() {
		return nil
	}
	return tea.Tick(motionFrameInterval, func(time.Time) tea.Msg { return motionFrameMsg{} })
}

// consumeEvent folds one live event into the snapshot and arms its moment.
// Full motion arms settling frames; reduced/off motion applies the state
// change immediately with static emphasis and arms nothing (AC-4.2).
func (m *Model) consumeEvent(ev journal.Event) {
	if ev.RunSequence > m.snap.LastRunSeq {
		m.snap.LastRunSeq = ev.RunSequence
	}
	full := m.cfg.Caps.Motion == caps.MotionFull
	switch ev.Type {
	case "attempt.launched":
		// The launch acknowledgement: only the current attempt's ack (or
		// an unattributed one, as synthetic feeds send) launches the lane.
		if ev.AttemptID != "" && m.snap.Attempt != nil && ev.AttemptID != m.snap.Attempt.AttemptID {
			return
		}
		m.motion.launched = true
		if full {
			m.motion.dispatchFrames = motionDispatchFrames
		}
	case "attempt.progress":
		// Progress is advisory: a malformed payload is skipped with the
		// latest good value kept, mirroring viewmodel.Load. The full
		// counters land in the same update the event is consumed — the
		// pulse is emphasis, never a typewriter delay (moment 4).
		if p, err := decodeLiveProgress(ev); err == nil {
			m.snap.LatestProgress = p
		}
		if full {
			m.motion.pulseFrames = motionPulseFrames
		}
	case "attempt.native_result":
		// Malformed completion evidence cannot fail an Update, so it is
		// ignored here; the poll reload (task 10) re-applies Load's
		// strict decoding and surfaces the failure there.
		if n, err := decodeLiveNativeExit(ev); err == nil {
			m.snap.NativeExit = n
		}
		if full {
			m.motion.deliveryFrames = motionDeliveryFrames
		}
	case "verification.completed", "receipt.written":
		// The recorded outcome is already in the snapshot (or arrives
		// with the next reload); the reveal only emphasises it.
		if full {
			m.motion.deliveryFrames = motionDeliveryFrames
		}
	case "admission.decided":
		// The route card renders the recorded admission statically; a
		// malformed payload is ignored here and fails loudly at Load.
		if a, err := decodeLiveAdmission(ev); err == nil {
			m.snap.Admission = a
		}
	}
}

// decodeLiveProgress decodes an attempt.progress payload with viewmodel's
// field shape (the canonical decoder stays in the viewmodel package; the
// poll reload converges any drift).
func decodeLiveProgress(ev journal.Event) (*viewmodel.Progress, error) {
	var p struct {
		AssistantTurns int            `json:"assistant_turns"`
		ToolUses       map[string]int `json:"tool_uses"`
		Retries        int            `json:"retries"`
	}
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return nil, err
	}
	return &viewmodel.Progress{AssistantTurns: p.AssistantTurns, ToolUses: p.ToolUses, Retries: p.Retries, RunSeq: ev.RunSequence}, nil
}

// decodeLiveNativeExit decodes an attempt.native_result payload with
// viewmodel's field shape.
func decodeLiveNativeExit(ev journal.Event) (*viewmodel.NativeExit, error) {
	var p struct {
		ExitCode       *int    `json:"exit_code"`
		Signal         *string `json:"signal"`
		ResultObserved bool    `json:"result_observed"`
	}
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return nil, err
	}
	return &viewmodel.NativeExit{ExitCode: p.ExitCode, Signal: p.Signal, ResultObserved: p.ResultObserved, RunSeq: ev.RunSequence}, nil
}

// decodeLiveAdmission decodes an admission.decided payload with viewmodel's
// field shape.
func decodeLiveAdmission(ev journal.Event) (*viewmodel.Admission, error) {
	var rec admission.Record
	if err := json.Unmarshal(ev.Payload, &rec); err != nil {
		return nil, err
	}
	return &viewmodel.Admission{
		AdapterID: rec.Adapter.ID, AdapterVersion: rec.Adapter.Version, AdapterSurface: rec.Adapter.Surface,
		Qualified: rec.Billing.Qualified, PaidContinuation: rec.Billing.PaidContinuation, RunSeq: ev.RunSequence,
	}, nil
}

// seedLaunched infers a launch the snapshot already proves, so inspect-mode
// loads do not claim a proven launch is still waiting: native-reported
// progress, a native result, or a post-launch attempt state each prove the
// acknowledgement happened. A bare running projection without any of those
// still waits for the live ack (moment 3).
func (m *Model) seedLaunched() {
	if m.snap.LatestProgress != nil || m.snap.NativeExit != nil {
		m.motion.launched = true
		return
	}
	if m.snap.Attempt == nil {
		return
	}
	switch m.snap.Attempt.State {
	case "succeeded_native", "failed_native", "stop_requested", "stopped":
		m.motion.launched = true
	}
}

// dispatchWaiting reports whether the lane must withhold its running label:
// a running projection whose launch acknowledgement this session has not
// consumed (and no native-reported evidence proves). Every other state
// renders verbatim — only running can lie.
func (m *Model) dispatchWaiting() bool {
	if m.motion.launched || m.snap.Attempt == nil || m.snap.LatestProgress != nil {
		return false
	}
	return m.snap.Attempt.State == "running"
}

// dispatchWaitingText is the pre-running lane label: waiting, never running.
func dispatchWaitingText(c caps.Caps) string {
	return "dispatching " + dashGlyph(c) + " waiting for launch acknowledgement"
}

// isFailureState reports whether a run state needs the failure treatment
// (moment 10): an actionable error plus its preserved artefact.
func isFailureState(state string) bool {
	return state == "failed" || state == "blocked"
}

// noteRunState records the failure-transition edge: on the first load of a
// failed run, or on any transition into failure, focus moves to the pane
// holding the error plus the preserved artefact (moment 10). Steady-state
// reloads preserve focus like every other state.
func (m *Model) noteRunState(wasReady bool) {
	state := m.snap.Run.State
	firstFailed := !wasReady && isFailureState(state)
	transitioned := wasReady && isFailureState(state) && !isFailureState(m.motion.prevRunState)
	if firstFailed || transitioned {
		m.focusPane = paneTasks
		m.focusIndex = clampFocus(m.focusIndex, m.focusedRowCount())
	}
	m.motion.prevRunState = state
}

// routeLines is the moment 2 route card: the recorded adapter descriptor
// from Snapshot.Admission with the static pinned-route reason. There is no
// deliberation spinner and no rationale beyond what the snapshot records;
// without an admission the card reads unknown (I09).
func (m *Model) routeLines() []line {
	a := m.snap.Admission
	if a == nil {
		return []line{{text: "route: unknown"}}
	}
	return []line{
		{text: "route: " + cellOrUnknown(a.AdapterID) + " " + cellOrUnknown(a.AdapterVersion) +
			" (" + cellOrUnknown(a.AdapterSurface) + ")"},
		{text: "route reason: pinned by --adapter"},
	}
}

// waitingLine is the moment 6 waiting indicator for runs blocked on
// something else: a restrained static reason, never a countdown — Stage 1
// has no known resets or deadlines to count to.
func (m *Model) waitingLine() (line, bool) {
	var reason string
	switch m.snap.Run.State {
	case "created", "admission":
		reason = "admission in progress"
	case "stopping":
		reason = "stop requested, worker confirming"
	default:
		return line{}, false
	}
	return line{text: "waiting " + dashGlyph(m.cfg.Caps) + " " + reason, colour: m.cfg.Tokens.TextMuted}, true
}

// stagesLine is the moment 8 integration strip: frozen, checked and ready
// light strictly from recorded rows (candidate, verification, ready state).
// Stage names only — there is no denominator, so never a percentage.
func (m *Model) stagesLine() line {
	frozen, checked, ready := "[wait]", "[wait]", "[wait]"
	if m.snap.Candidate != nil {
		frozen = "[done]"
	}
	if m.snap.Verification != nil {
		checked = "[done]"
	}
	if m.snap.Run.State == "ready_for_review" {
		ready = "[done]"
	}
	return line{
		text:   "stages: " + frozen + " frozen  " + checked + " checked  " + ready + " ready",
		colour: m.cfg.Tokens.TextMuted,
	}
}

// deliveryOutcome names the recorded outcome the moment 9 reveal
// celebrates: exactly verified, failed or unverified — never generic
// success copy on a run that did not succeed.
func (m *Model) deliveryOutcome() string {
	if verified(m.snap) {
		return "verified " + shortRev(cell(m.snap.Candidate.Commit))
	}
	if isFailureState(m.snap.Run.State) {
		return "failed"
	}
	if m.snap.Verification != nil && m.snap.Verification.Result == "failed" {
		return "failed"
	}
	return "unverified"
}

// tasksRows renders the tasks pane with the motion moments spliced in: the
// waiting indicator for waiting states, the transient delivery outcome
// while the reveal runs, and the preserved artefact plus error emphasis
// for failed runs. Failure emphasis is a static error colour — never blink
// or reverse flash (moment 10).
func (m *Model) tasksRows(width int) []line {
	rows := m.tasksLines(width)
	if isFailureState(m.snap.Run.State) && len(rows) > 2 {
		rows[2].colour = m.cfg.Tokens.Err
	}
	if ln, ok := m.waitingLine(); ok {
		rows = append(rows, ln)
	}
	if m.motion.deliveryFrames > 0 {
		rows = append(rows, line{text: "outcome: " + m.deliveryOutcome(), colour: m.cfg.Tokens.Attention})
	}
	if isFailureState(m.snap.Run.State) && m.snap.Attempt != nil && m.snap.Attempt.WorkspacePath != "" {
		rows = append(rows, line{text: "preserved: " + cell(m.snap.Attempt.WorkspacePath)})
	}
	return rows
}

// agentsRows renders the agents pane with the motion moments spliced in:
// the dispatch gate withholds running until the launch acknowledgement,
// the pulse marks the activity row while its frames run, and the route
// card sits under the pane rule.
func (m *Model) agentsRows(width int) []line {
	rows := m.agentsLines(width)
	want := "  " + m.activityText()
	for i := range rows {
		if rows[i].text != want {
			continue
		}
		if m.dispatchWaiting() {
			rows[i].text = "  " + dispatchWaitingText(m.cfg.Caps)
		}
		if m.motion.pulseFrames > 0 {
			rows[i].text = "  " + dotGlyph(m.cfg.Caps) + " " + strings.TrimPrefix(rows[i].text, "  ")
		}
	}
	return spliceAfterRule(rows, m.routeLines())
}

// detailRows renders the selected-change pane with the integration stages
// appended after the verification block.
func (m *Model) detailRows(width int) []line {
	return append(m.detailLines(width), m.stagesLine())
}

// gateCompactActivity withholds the compact view's running label under the
// same dispatch rule as the agents pane (moment 3).
func (m *Model) gateCompactActivity(rows []line) []line {
	if !m.dispatchWaiting() {
		return rows
	}
	for i := range rows {
		if rows[i].text == "activity: state: running" {
			rows[i].text = "activity: " + dispatchWaitingText(m.cfg.Caps)
		}
	}
	return rows
}

// spliceAfterRule inserts extra rows under a pane's header-plus-rule rows;
// short row sets pass through untouched.
func spliceAfterRule(rows, extra []line) []line {
	if len(rows) < 2 {
		return rows
	}
	out := make([]line, 0, len(rows)+len(extra))
	out = append(out, rows[:2]...)
	out = append(out, extra...)
	return append(out, rows[2:]...)
}
