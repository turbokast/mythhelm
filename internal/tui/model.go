package tui

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/tui/caps"
	"github.com/turbokast/mythhelm/internal/tui/theme"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
	"github.com/turbokast/mythhelm/internal/workspace"
)

// Focusable pane ids.
const (
	paneTasks    = "tasks"
	paneAgents   = "agent"
	paneEvidence = "evidence"
	paneDetail   = "diff"
)

// Config is the TUI program's inputs. Events feeds live journaled events and
// Notices carries unjournaled status in live mode (both from LiveFeed,
// live.go); Actions injects the supervisor calls the palette acts through
// (task 9).
type Config struct {
	RunID    string
	StateDir string
	Caps     caps.Caps
	Tokens   theme.Tokens
	// ThemeName names the built-in theme Tokens came from, so the first
	// toggle moves away from it; empty means "dark". Custom loaded tokens
	// keep no name: toggling from them lands on a built-in.
	ThemeName        string
	Events           <-chan journal.Event
	Notices          *NoticeBacklog
	Actions          Actions
	Mode             ClientMode
	RequestStop      func()
	Detach           func()
	Signals          <-chan ClientSignal
	OnExit           func(ExitIntent)
	OnHeadlessAction func(ActionCompletion)
}

// Model is the mission-view program: one snapshot rendered through the §15.4
// layout ladder. dialog holds a pending confirmation id ("" = none) with its
// transient state (dialogs.go); stopRequested marks a confirmed stop the
// worker has not journaled yet (actions.go). focusPrev restores the previous
// pane on Esc; filterOn/filterText hold the / filter; palette and helpOpen
// hold the : and ? overlays; themeName tracks the active built-in theme;
// selectedLane is the stable lane identity a reload keeps; motion holds the
// session-observed truthful-motion state (motion.go); notices holds the
// drained ui.notice events (live.go); loadSnapshot is the poll-tick reload
// seam; liveQuit/liveDone join the channel pump; recoverEvents and
// recoverNotices feed the in-flight recover action live; reload is the one
// owned in-flight snapshot load and reloadGen counts the action results a
// load must not predate (live.go).
type Model struct {
	viewDone            chan struct{}
	cfg                 Config
	action              *actionCall
	interrupts          uint64
	actionQuitArmed     bool
	exiting             bool
	exitIntent          ExitIntent
	terminalLoss        bool
	stopSent            bool
	detachSent          bool
	exitRow             int
	managedPoll         bool
	snap                viewmodel.Snapshot
	ready               bool
	loadErr             error
	empty               bool
	runCtx              context.Context
	actCancel           context.CancelFunc
	working             string
	diff                Diff
	diffErr             error
	diffRenderer        func(Diff, int) []string
	loadDiff            func(ctx context.Context, workspace, base, commit string) (Diff, error)
	loadSnapshot        func(ctx context.Context, dir, runID string) (viewmodel.Snapshot, error)
	width               int
	height              int
	focusPane           string
	focusPrev           string
	diffOrigin          string
	sidePane            string
	focusIndex          int
	selectedTask        string
	agentRaw            bool
	selectedLane        string
	dialog              string
	dialogField         int
	dialogFocus         int
	applyBranch         string
	acceptFlags         bool
	acceptUnverified    bool
	stopRequested       bool
	stopUnresolved      bool
	actualStopConfirmed bool
	stopConfirmed       bool
	dialogErr           string
	resultTitle         string
	resultBody          string
	filterOn            bool
	filterText          string
	palette             paletteState
	helpOpen            bool
	themeName           string
	motion              motionState
	notices             []journal.Event
	liveQuit            chan struct{}
	liveDone            chan struct{}
	liveMu              sync.Mutex
	recoverEvents       chan journal.Event
	recoverNotices      *NoticeBacklog
	reload              *reloadCall
	reloadGen           uint64
}

// newModel builds the program state with default dimensions; the first
// WindowSizeMsg replaces them with the terminal's real size.
func newModel(cfg Config) *Model {
	name := cfg.ThemeName
	if name == "" {
		name = "dark"
	}
	return &Model{
		cfg:          cfg,
		viewDone:     make(chan struct{}),
		loadDiff:     defaultLoadDiff,
		loadSnapshot: viewmodel.Load,
		width:        singleWidth,
		height:       24,
		focusPane:    paneTasks,
		focusPrev:    paneTasks,
		themeName:    name,
		// The arrival reveal is pending only under full motion; reduced
		// and off collapse it to the immediate paint (moment 1).
		motion: motionState{arrival: cfg.Caps.Motion == caps.MotionFull},
	}
}

// Run loads the run's snapshot and starts the mission-view program. A state
// dir with no database shows the empty state; any other load failure (an
// unknown run ID included) is returned before the program starts. In live
// mode the channel pump forwards Events until the program exits, and the
// 200 ms poll chain (live.go) reloads off the Update loop in both modes;
// quitting joins the pump and cancels and joins any in-flight reload, with
// no goroutine left behind.
func Run(ctx context.Context, cfg Config) error {
	m := newModel(cfg)
	m.runCtx = ctx
	load := m.loadSnapshot
	if load == nil {
		load = viewmodel.Load
	}
	snap, err := load(ctx, cfg.StateDir, cfg.RunID)
	if err != nil {
		if errors.Is(err, journal.ErrNoDatabase) {
			m.empty = true
		} else {
			return err
		}
	} else {
		m.setSnapshot(ctx, snap)
	}
	return runProgram(ctx, m, tea.WithAltScreen())
}

// setSnapshot installs a snapshot and (re)loads its candidate diff. The
// selected task keeps its identity across reloads; only a first load picks
// the default. It also seeds the launch the snapshot already proves and
// moves focus on the failure-transition edge (motion.go); it never arms
// animation frames, which only Update can schedule beside their tick.
func (m *Model) setSnapshot(ctx context.Context, snap viewmodel.Snapshot) {
	var diff Diff
	var diffErr error
	if snap.Attempt != nil && snap.Candidate != nil {
		diff, diffErr = m.loadDiff(ctx, snap.Attempt.WorkspacePath, snap.Candidate.BaseRev, snap.Candidate.Commit)
	}
	m.installSnapshot(snap, diff, diffErr)
}

// installSnapshot applies a snapshot and its already-loaded diff to the model.
func (m *Model) installSnapshot(snap viewmodel.Snapshot, diff Diff, diffErr error) {
	wasReady := m.ready
	prevAttempt, nextAttempt := "", ""
	if m.snap.Attempt != nil {
		prevAttempt = m.snap.Attempt.AttemptID
	}
	if snap.Attempt != nil {
		nextAttempt = snap.Attempt.AttemptID
	}
	if nextAttempt != prevAttempt {
		m.actualStopConfirmed = false
	}
	m.snap = snap
	if snap.Attempt != nil && snap.Attempt.State == "stopped" {
		m.actualStopConfirmed = true
		m.stopUnresolved = false
	}
	m.ready = true
	m.loadErr = nil
	if stopConverged(snap) {
		// The reload proves the request converged (the attempt stopped
		// or the run left the stopping states), so the pending label
		// retires and a late in-flight result cannot re-raise it; until
		// then it renders requested, never stopped (I06).
		m.stopRequested = false
		m.stopConfirmed = true
	}
	if nextAttempt != prevAttempt {
		// A new attempt waits for its own ack: the old launch (consumed
		// or seeded) must not carry over. seedLaunched re-seeds from the
		// new snapshot's scoped evidence below.
		m.motion.launched = false
		m.motion.launchedAttempt = ""
		m.motion.dispatchFrames = 0
	}
	m.seedLaunched()
	m.noteRunState(wasReady)
	if m.selectedTask == "" {
		m.selectedTask = snap.Run.RunID
		if snap.Attempt != nil && snap.Attempt.TaskID != "" {
			m.selectedTask = snap.Attempt.TaskID
		}
	}
	if m.selectedLane == "" {
		m.selectedLane = snap.Run.RunID
		if snap.Attempt != nil && snap.Attempt.AttemptID != "" {
			m.selectedLane = snap.Attempt.AttemptID
		}
	}
	m.diff, m.diffErr = diff, diffErr
}

// revisionRE accepts the 40–64 hex revisions the review diff uses.
var revisionRE = regexp.MustCompile(`^[0-9a-fA-F]{40,64}$`)

// defaultLoadDiff produces the candidate diff exactly as the review verb
// does: plain git diff of base to commit over the attempt workspace.
func defaultLoadDiff(ctx context.Context, ws, base, commit string) (Diff, error) {
	if !revisionRE.MatchString(base) || !revisionRE.MatchString(commit) {
		return Diff{}, errors.New("tui: invalid candidate revision")
	}
	out, err := workspace.Git(ctx, ws, false, "diff", "--no-color", "--no-ext-diff", "--no-textconv", base, commit) //nolint:misspell // Git's flag is --no-color.
	if err != nil {
		return Diff{}, err
	}
	return ParseDiff(out)
}

// Init implements tea.Model; its only start-up command is the arrival
// reveal's settling tick under full motion (nil otherwise).
func (m *Model) Init() tea.Cmd { return m.scheduleMotion() }

// Update implements tea.Model: resizes re-lay out around preserved identity,
// keys dispatch through handleKey (nav.go) and the lifecycle policy; live
// events fold into the snapshot and arm their moment (motion.go); motion
// frames count the active moments down; poll ticks drain notices and rebuild
// the snapshot once (live.go). Any key skips a pending arrival.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.exiting {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.applySize(msg)
		return m, nil
	case tea.KeyMsg:
		m.motion.arrival = false
		return m.handleKey(msg)
	case eventMsg:
		m.consumeEvent(msg.Event)
		m.noteStopConfirmation(msg.Event)
		return m, m.scheduleMotion()
	case ClientSignal:
		if msg == SignalTerminalLost {
			return m, m.terminalLost()
		}
		if msg == SignalInterrupt {
			return m, m.interrupt()
		}
		return m, nil
	case parentCancelledMsg:
		m.cancelAction()
		return m, m.exit(ExitUnresolvedAction)
	case pollTickMsg:
		reload := m.onPollTick()
		if m.managedPoll {
			return m, tea.Batch(reload, m.scheduleMotion())
		}
		return m, tea.Batch(m.pollCmd(), reload, m.scheduleMotion())
	case reloadResultMsg:
		m.applyReload(msg.call)
		return m, m.scheduleMotion()
	case actionResultMsg:
		// A load in flight may have read the journal before this action.
		m.reloadGen++
		m.applyActionResult(msg)
		return m, nil
	case motionFrameMsg:
		if m.cfg.Caps.Motion != caps.MotionFull {
			return m, nil
		}
		m.motion.arrival = false
		for _, frames := range []*int{&m.motion.dispatchFrames, &m.motion.pulseFrames, &m.motion.deliveryFrames} {
			if *frames > 0 {
				*frames--
			}
		}
		return m, m.scheduleMotion()
	default:
		return m, nil
	}
}

// View implements tea.Model, dispatching on the layout ladder. The dialog,
// palette and help overlays compose over the dimmed mission (overlays.go).
func (m *Model) View() string {
	if rows := m.overlayLines(); rows != nil {
		return m.overlayView(rows)
	}
	if m.loadErr != nil {
		return m.render([]line{{text: "error: " + cell(m.loadErr.Error())}})
	}
	if m.empty {
		return m.render([]line{
			{text: "no runs recorded in " + cellOrUnknown(m.cfg.StateDir)},
			{text: "start a run to populate this view"},
		})
	}
	if !m.ready {
		return m.render([]line{{text: "loading snapshot" + ellGlyph(m.cfg.Caps)}})
	}
	return m.observatoryView()
}

// render finalises single-column rows: each fits the width, then the view
// fits the height.
func (m *Model) render(rows []line) string {
	out := make([]string, 0, len(rows))
	for _, ln := range rows {
		out = append(out, renderLine(m.cfg.Caps, ln, m.width))
	}
	if m.height > 0 && len(out) > m.height {
		out = out[:m.height]
	}
	return strings.Join(out, "\n")
}

// finaliseRows fits and styles rows without capping their count.
func (m *Model) finaliseRows(rows []line, width int) []string {
	out := make([]string, 0, len(rows))
	for _, ln := range rows {
		out = append(out, renderLine(m.cfg.Caps, ln, width))
	}
	return out
}

// column is one pane's fitted rows plus its width.
type column struct {
	width int
	rows  []line
}

func (m *Model) focusedPaneLines(width int) []line { return m.paneLines(m.focusPane, width) }
func (m *Model) paneLines(id string, width int) []line {
	switch id {
	case paneAgents:
		return m.applyFilter(m.agentsRows(width))
	case paneEvidence:
		rows := []line{m.paneHeader(paneEvidence, "EVIDENCE"), {text: m.ruleLine(width), colour: m.cfg.Tokens.Border}}
		for _, row := range m.observatoryEvidence(width) {
			if strings.HasPrefix(row.text, "entitlement:") {
				for part := range strings.SplitSeq(lipgloss.NewStyle().Width(max(1, width)).Render(m.entitlementText()), "\n") {
					rows = append(rows, m.paneFact(strings.TrimRight(part, " "), width))
				}
			} else {
				rows = append(rows, row)
			}
			if strings.HasPrefix(row.text, "Billing:") {
				qualified, paid := "unknown", "unknown"
				if a := m.snap.Admission; a != nil {
					if a.Qualified {
						qualified = "true"
					} else {
						qualified = "false"
					}
					paid = observatoryValue(a.PaidContinuation)
				}
				rows = append(rows, m.paneFact("qualified: "+qualified, width), m.paneFact("paid continuation: "+paid, width))
			}
		}
		return rows
	case paneDetail:
		return m.detailLines(width)
	default:
		rows := m.applyFilter(m.tasksRows(width))
		var fitted []line
		for _, row := range rows {
			if len(row.spans) == 0 && lipgloss.Width(row.text) > width {
				for part := range strings.SplitSeq(lipgloss.NewStyle().Width(max(1, width)).Render(row.text), "\n") {
					fitted = append(fitted, line{text: part, colour: row.colour})
				}
			} else {
				fitted = append(fitted, row)
			}
		}
		return fitted
	}
}
func (m *Model) settledVerdict() bool {
	switch m.snap.Run.State {
	case "ready_for_review", "completed", "blocked", "failed", "cancelled":
		return true
	default:
		return false
	}
}

// missionNowNext retains the session's launch-acknowledgement guard when
// composing the previously independent NOW producer.
func (m *Model) missionNowNext(width int) []line {
	ack := m.dispatchWaiting() && m.snap.Run.State == "executing" && !m.inspectUnsupervised()
	cause := m.observatoryAttention() && m.snap.Run.Reason != ""
	if !ack && !cause {
		return m.nowNext(width)
	}
	current, next := m.observatoryNow(), m.observatoryNext()
	if ack {
		// Reuse the incumbent guard, so the new NOW cannot bypass the launch ack.
		guarded := m.gateCompactActivity([]line{{text: "activity: " + m.activityText()}})
		current = strings.TrimPrefix(guarded[0].text, "activity: ")
	}
	if cause {
		next = "reason: " + cell(m.snap.Run.Reason) + "; " + next
	}
	if width < 80 || m.height < 10 {
		return []line{m.observatoryLabel("now", current, width), m.observatoryLabel("next", next, width)}
	}
	leftWidth := (width - 2) / 2
	left := m.observatoryLabel("NOW", current, leftWidth)
	right := m.observatoryLabel("NEXT", next, width-leftWidth-2)
	left.spans = append(left.spans, span{text: strings.Repeat(" ", leftWidth-lipgloss.Width(left.text)+2), colour: m.cfg.Tokens.Text})
	return []line{lineFromSpans(append(left.spans, right.spans...)...)}
}

// observatoryView reserves mandatory chrome before optional content. Notices
// retain priority over filter/details, and no row follows the final key bar.
func (m *Model) observatoryView() string {
	l := resolveLayout(m.width, m.height)
	compact := l == layoutCompact
	keys := m.observatoryKeys(m.width, compact)
	key := keys[len(keys)-1]
	now := m.missionNowNext(m.width)
	if len(keys) > 1 && compact {
		relocated := keys[0].spans
		relocated[0].text = "next "
		budget := max(0, m.width-lipgloss.Width(lineFromSpans(relocated...).text)-1)
		relocated = append(relocated, span{text: " " + m.observatoryFit(strings.TrimPrefix(now[1].text, "next "), budget), colour: m.cfg.Tokens.Text})
		now[1] = lineFromSpans(relocated...)
	}
	notices := m.noticeRows(maxNoticeRows)
	filter, hasFilter := m.filterLine()
	var prefix, body, extras []line
	if compact {
		prefix = []line{m.observatoryHeader(m.width), now[0], now[1], m.compactUsage(m.width)}
		extras = append(extras, m.verificationLines()...)
		extras = append(extras, m.statusLines()[:2]...)
	} else {
		rule := line{text: m.ruleLine(m.width), colour: m.cfg.Tokens.Border}
		prefix = []line{m.observatoryHeader(m.width), rule, now[0]}
		if l == layoutSingle && (!m.settledVerdict() || m.focusPane == paneDetail) {
			prefix = append(prefix, m.tabsLine())
		}
		prefix = append(prefix, rule)
		extras = []line{rule}
	}
	// Reserve all available notice rows before optional input and body rows.
	spare := max(0, m.height-len(prefix)-1)
	if !compact {
		spare = max(0, spare-1)
	}
	if len(notices) > spare {
		notices = notices[len(notices)-spare:]
	}
	spare -= len(notices)
	var input []line
	if hasFilter && spare > 0 {
		input = []line{filter}
		spare--
	}
	if compact {
		if len(extras) > spare {
			extras = extras[:spare]
		}
		body = extras
	} else {
		var trunc []line
		if m.focusPane == paneDetail && m.diff.Truncated && m.snap.Attempt != nil && m.snap.Candidate != nil && spare >= 2 {
			trunc = m.truncationNotice()
			spare -= len(trunc)
		}
		body = m.observatoryBody(l, spare)
		body = append(body, trunc...)
	}
	rows := append([]line{}, prefix...)
	rows = append(rows, body...)
	rows = append(rows, notices...)
	rows = append(rows, input...)
	footerRows := 1
	if !compact {
		footerRows = 2
	}
	for len(rows) < m.height-footerRows {
		rows = append(rows, line{})
	}
	if !compact {
		rows = append(rows, line{text: m.ruleLine(m.width), colour: m.cfg.Tokens.Border})
	}
	if len(rows) > max(0, m.height-1) { // Extremely short compact terminals.
		if m.height >= 3 {
			rows = []line{m.observatoryHeader(m.width), now[len(now)-1]}
		} else {
			rows = rows[:max(0, m.height-1)]
		}
	}
	for len(rows) < m.height-1 {
		rows = append(rows, line{})
	}
	rows = append(rows, key)
	return m.render(rows)
}
func (m *Model) observatoryBody(l layout, height int) []line {
	if height <= 0 {
		return nil
	}
	if l != layoutThreePane && m.settledVerdict() && m.focusPane != paneDetail {
		return capLines(m.observatoryVerdict(m.width), height, m.cfg.Caps, m.cfg.Tokens.TextMuted)
	}
	if l == layoutSingle {
		if m.focusPane == paneDetail {
			return m.detailViewportLines(m.width, height)
		}
		return capLines(m.focusedPaneLines(m.width), height, m.cfg.Caps, m.cfg.Tokens.TextMuted)
	}
	ids := []string{paneTasks, paneAgents, paneEvidence}
	if l == layoutTwoPane {
		side := m.sidePane
		if m.focusPane != paneTasks {
			side = m.focusPane
		}
		if side == "" || side == paneTasks {
			side = paneAgents
		}
		ids = []string{paneTasks, side}
	} else if m.focusPane == paneDetail {
		origin := m.diffOrigin
		if origin == "" {
			origin = paneEvidence
		}
		for i, id := range ids {
			if id == origin {
				ids[i] = paneDetail
			}
		}
	}
	widths := columnWidths(m.width, len(ids))
	cols := make([]column, len(ids))
	for i, id := range ids {
		var rows []line
		if id == paneDetail {
			rows = m.detailViewportLines(widths[i], height)
		} else {
			rows = m.paneLines(id, widths[i])
		}
		if l == layoutTwoPane && id == paneTasks {
			stages := m.observatoryEvidence(widths[i])
			rows = append(rows, stages[:min(5, len(stages))]...)
		}
		cols[i] = column{width: widths[i], rows: capLines(rows, height, m.cfg.Caps, m.cfg.Tokens.TextMuted)}
	}
	sep := " " + styleText(m.cfg.Caps, vBarGlyph(m.cfg.Caps), m.cfg.Tokens.Border) + " "
	rendered := joinColumns(m.cfg.Caps, cols, sep)
	rows := make([]line, len(rendered))
	for i, text := range rendered {
		rows[i] = line{text: text}
	}
	return rows
}

// joinColumns pads each pane's rows to its width, styles them, and joins the
// rowtuple with separators. Short panes pad with blank rows.
func joinColumns(c caps.Caps, cols []column, sep string) []string {
	tallest := 0
	for _, col := range cols {
		if len(col.rows) > tallest {
			tallest = len(col.rows)
		}
	}
	out := make([]string, 0, tallest)
	for i := range tallest {
		parts := make([]string, 0, len(cols))
		for _, col := range cols {
			ln := line{}
			if i < len(col.rows) {
				ln = col.rows[i]
			}
			if len(ln.spans) > 0 {
				parts = append(parts, padCells(renderLine(c, ln, col.width), col.width))
			} else {
				parts = append(parts, styleText(c, padCells(fitLine(c, ln.text, col.width), col.width), ln.colour))
			}
		}
		out = append(out, strings.Join(parts, sep))
	}
	return out
}

// capLines caps rows at limit with a labelled marker row when rows are dropped.
func capLines(rows []line, limit int, c caps.Caps, colour string) []line {
	if limit < 1 {
		limit = 1
	}
	if len(rows) <= limit {
		return rows
	}
	marker := line{text: ellGlyph(c) + fmt.Sprintf(" and %d more lines", len(rows)-limit+1), colour: colour}
	return append(rows[:limit-1], marker)
}
