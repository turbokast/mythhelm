package tui

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/tui/caps"
	"github.com/turbokast/mythhelm/internal/tui/theme"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
	"github.com/turbokast/mythhelm/internal/workspace"
)

// Focusable pane ids.
const (
	paneTasks  = "tasks"
	paneAgents = "agents"
	paneDetail = "detail"
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
	ThemeName string
	Events    <-chan journal.Event
	Notices   *NoticeBacklog
	Actions   Actions
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
// recoverNotices feed the in-flight recover action live (live.go).
type Model struct {
	cfg              Config
	snap             viewmodel.Snapshot
	ready            bool
	loadErr          error
	empty            bool
	runCtx           context.Context
	actCancel        context.CancelFunc
	working          string
	diff             Diff
	diffErr          error
	loadDiff         func(ctx context.Context, workspace, base, commit string) (Diff, error)
	loadSnapshot     func(ctx context.Context, dir, runID string) (viewmodel.Snapshot, error)
	width            int
	height           int
	focusPane        string
	focusPrev        string
	focusIndex       int
	selectedTask     string
	selectedLane     string
	dialog           string
	dialogField      int
	dialogFocus      int
	applyBranch      string
	acceptFlags      bool
	acceptUnverified bool
	stopRequested    bool
	stopConfirmed    bool
	dialogErr        string
	resultTitle      string
	resultBody       string
	filterOn         bool
	filterText       string
	palette          paletteState
	helpOpen         bool
	themeName        string
	motion           motionState
	notices          []journal.Event
	liveQuit         chan struct{}
	liveDone         chan struct{}
	liveMu           sync.Mutex
	recoverEvents    chan journal.Event
	recoverNotices   *NoticeBacklog
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
// 200 ms poll chain (live.go) reloads in both modes; quitting joins the
// pump with no goroutine left behind.
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
	p := tea.NewProgram(m, tea.WithAltScreen())
	if cfg.Events != nil {
		m.startLivePump(func(ev journal.Event) { p.Send(eventMsg{Event: ev}) })
	}
	// Bootstrap the poll chain with one immediate tick: Init stays the
	// motion tick alone, so the motion tests keep asserting Init nil-ness
	// for "no motion frames" while live and inspect modes still reload.
	go func() { p.Send(pollTickMsg{}) }()
	_, err = p.Run()
	m.stopLivePump()
	m.cancelAction()
	return err
}

// setSnapshot installs a snapshot and (re)loads its candidate diff. The
// selected task keeps its identity across reloads; only a first load picks
// the default. It also seeds the launch the snapshot already proves and
// moves focus on the failure-transition edge (motion.go); it never arms
// animation frames, which only Update can schedule beside their tick.
func (m *Model) setSnapshot(ctx context.Context, snap viewmodel.Snapshot) {
	wasReady := m.ready
	prevAttempt, nextAttempt := "", ""
	if m.snap.Attempt != nil {
		prevAttempt = m.snap.Attempt.AttemptID
	}
	if snap.Attempt != nil {
		nextAttempt = snap.Attempt.AttemptID
	}
	m.snap = snap
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
	m.diff, m.diffErr = Diff{}, nil
	if snap.Attempt != nil && snap.Candidate != nil {
		d, err := m.loadDiff(ctx, snap.Attempt.WorkspacePath, snap.Candidate.BaseRev, snap.Candidate.Commit)
		if err != nil {
			m.diffErr = err
		} else {
			m.diff = d
		}
	}
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
// keys dispatch through handleKey (nav.go), where Ctrl-C alone quits; live
// events fold into the snapshot and arm their moment (motion.go); motion
// frames count the active moments down; poll ticks drain notices and rebuild
// the snapshot once (live.go). Any key skips a pending arrival.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.applySize(msg)
		return m, nil
	case tea.KeyMsg:
		m.motion.arrival = false
		return m.handleKey(msg)
	case eventMsg:
		m.consumeEvent(msg.Event)
		return m, m.scheduleMotion()
	case pollTickMsg:
		m.onPollTick()
		return m, tea.Batch(m.pollCmd(), m.scheduleMotion())
	case actionResultMsg:
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
// palette and help overlays take over the whole view while open.
func (m *Model) View() string {
	if m.dialog != "" {
		return m.render(m.dialogLines())
	}
	if m.palette.open {
		return m.render(m.paletteLines())
	}
	if m.helpOpen {
		return m.render(m.helpLines())
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
	switch resolveLayout(m.width, m.height) {
	case layoutCompact:
		body := m.gateCompactActivity(m.compactLines())
		// Notices are must-see status, so they sit between the five-row
		// offer (which a 5-row terminal always sees whole) and the
		// detail rows: render's height truncation then drops detail
		// before notices, never the reverse.
		offer, detail := body, []line(nil)
		if len(body) > compactOfferRows {
			offer, detail = body[:compactOfferRows], body[compactOfferRows:]
		}
		rows := append(append(append([]line{}, offer...), m.noticeRows(maxNoticeRows)...), detail...)
		if ln, ok := m.filterLine(); ok {
			rows = append(rows, ln)
		}
		return m.render(rows)
	case layoutSingle:
		return m.viewSingle()
	case layoutTwoPane:
		return m.viewTwoPane()
	default:
		return m.viewThreePane()
	}
}

// render finalises single-column rows: each fits the width, then the view
// fits the height.
func (m *Model) render(rows []line) string {
	out := make([]string, 0, len(rows))
	for _, ln := range rows {
		out = append(out, styleText(m.cfg.Caps, fitLine(m.cfg.Caps, ln.text, m.width), ln.colour))
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
		out = append(out, styleText(m.cfg.Caps, fitLine(m.cfg.Caps, ln.text, width), ln.colour))
	}
	return out
}

// viewSingle renders the 80–99 column rung: labelled tabs, the focused pane,
// the truncation notice when the detail pane shows a truncated diff, drained
// notices, and the persistent critical status.
func (m *Model) viewSingle() string {
	var rows []line
	rows = append(rows, m.tabsLine())
	notices := m.noticeRows(maxNoticeRows)
	bodyH := m.height - 3 - len(notices)
	if m.showNotice(layoutSingle) {
		bodyH -= 2
	}
	rows = append(rows, capLines(m.focusedPaneLines(m.width), bodyH, m.cfg.Caps, m.cfg.Tokens.TextMuted)...)
	if m.showNotice(layoutSingle) {
		rows = append(rows, m.truncationNotice()...)
	}
	rows = append(rows, notices...)
	rows = append(rows, m.criticalStatusLine(), m.footerLine())
	if ln, ok := m.filterLine(); ok {
		rows = append(rows, ln)
	}
	return m.render(rows)
}

// viewTwoPane renders the 100–139 column rung: tasks beside the switchable
// detail panel (agents, or the selected change when it holds focus).
func (m *Model) viewTwoPane() string {
	widths := columnWidths(m.width, 2)
	return m.viewColumns(layoutTwoPane, []column{m.tasksColumn(widths[0]), m.detailColumn(widths[1])})
}

// viewThreePane renders the 140+ column rung: tasks, agents and selected
// detail side by side.
func (m *Model) viewThreePane() string {
	widths := columnWidths(m.width, 3)
	detail := column{width: widths[2], rows: m.detailRows(widths[2])}
	return m.viewColumns(layoutThreePane, []column{m.tasksColumn(widths[0]), m.agentsColumn(widths[1]), detail})
}

// column is one pane's fitted rows plus its width.
type column struct {
	width int
	rows  []line
}

func (m *Model) tasksColumn(width int) column {
	return column{width: width, rows: m.applyFilter(m.tasksRows(width))}
}

func (m *Model) agentsColumn(width int) column {
	return column{width: width, rows: m.applyFilter(m.agentsRows(width))}
}

// detailColumn is the switchable detail panel: the selected change when it
// holds focus, else the agents pane. Detail rows are built at the column
// width so the diff renderer fits before styling.
func (m *Model) detailColumn(width int) column {
	if m.focusPane == paneDetail {
		return column{width: width, rows: m.detailRows(width)}
	}
	return column{width: width, rows: m.applyFilter(m.agentsRows(width))}
}

// focusedPaneLines is the single rung's full-width pane. The filter applies
// to the tasks and agents panes only; the detail pane is never filtered.
func (m *Model) focusedPaneLines(width int) []line {
	switch m.focusPane {
	case paneAgents:
		return m.applyFilter(m.agentsRows(width))
	case paneDetail:
		return m.detailRows(width)
	default:
		return m.applyFilter(m.tasksRows(width))
	}
}

// viewColumns assembles the header, side-by-side panes, truncation notice,
// drained notices, status strip and footer for the two- and three-pane rungs.
// Panes virtualise to the body height and notices to their tail cap, so the
// truncation notice keeps its rows whenever a truncated diff shows.
func (m *Model) viewColumns(l layout, cols []column) string {
	widths := columnWidths(m.width, len(cols))
	sep := " " + vBarGlyph(m.cfg.Caps) + " "
	notices := m.noticeRows(maxNoticeRows)
	chrome := 2 + 2 + 1 + len(notices) // header, status strip, footer, notices
	if m.showNotice(l) {
		chrome += 2
	}
	bodyH := max(m.height-chrome, 1)
	for i := range cols {
		if cols[i].width == 0 {
			cols[i].width = widths[i]
		}
		cols[i].rows = capLines(cols[i].rows, bodyH, m.cfg.Caps, m.cfg.Tokens.TextMuted)
	}
	var out []string
	out = append(out, m.finaliseRows(m.headerLines(), m.width)...)
	out = append(out, joinColumns(m.cfg.Caps, cols, sep)...)
	if m.showNotice(l) {
		out = append(out, m.finaliseRows(m.truncationNotice(), m.width)...)
	}
	out = append(out, m.finaliseRows(notices, m.width)...)
	out = append(out, m.finaliseRows(m.statusLines(), m.width)...)
	out = append(out, m.finaliseRows([]line{m.footerLine()}, m.width)...)
	if ln, ok := m.filterLine(); ok {
		out = append(out, m.finaliseRows([]line{ln}, m.width)...)
	}
	if m.height > 0 && len(out) > m.height {
		out = out[:m.height]
	}
	return strings.Join(out, "\n")
}

// columnWidths splits the width across n panes with one separator each.
func columnWidths(width, n int) []int {
	seps := (n - 1) * 3
	rest := width - seps
	if n == 2 {
		left := rest * 2 / 5
		return []int{left, rest - left}
	}
	tasks := rest / 4
	agents := rest * 3 / 10
	return []int{tasks, agents, rest - tasks - agents}
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
			parts = append(parts, styleText(c, padCells(fitLine(c, ln.text, col.width), col.width), ln.colour))
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
