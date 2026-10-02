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
// Notices carries unjournaled status in live mode (both wired in task 10);
// Actions injects the supervisor calls the palette acts through (task 9).
type Config struct {
	RunID    string
	StateDir string
	Caps     caps.Caps
	Tokens   theme.Tokens
	Events   <-chan journal.Event
	Notices  *NoticeBacklog
	Actions  Actions
}

// NoticeBacklog retains unjournaled notices in arrival order. Add never
// blocks and never drops; Drain takes every pending notice atomically. Task
// 10 moves this to live.go with LiveFeed and the poll-tick drain; it lives
// here until then so Config compiles.
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

// Model is the mission-view program: one snapshot rendered through the §15.4
// layout ladder. dialog holds a pending confirmation id ("" = none); task 9
// replaces it with rich confirmation prompts. focusPrev restores the previous
// pane on Esc; filterOn/filterText hold the / filter; palette and helpOpen
// hold the : and ? overlays; themeName tracks the active built-in theme;
// selectedLane is the stable lane identity a reload keeps.
type Model struct {
	cfg          Config
	snap         viewmodel.Snapshot
	ready        bool
	loadErr      error
	empty        bool
	diff         Diff
	diffErr      error
	loadDiff     func(ctx context.Context, workspace, base, commit string) (Diff, error)
	width        int
	height       int
	focusPane    string
	focusPrev    string
	focusIndex   int
	selectedTask string
	selectedLane string
	dialog       string
	filterOn     bool
	filterText   string
	palette      paletteState
	helpOpen     bool
	themeName    string
}

// newModel builds the program state with default dimensions; the first
// WindowSizeMsg replaces them with the terminal's real size.
func newModel(cfg Config) *Model {
	return &Model{
		cfg:       cfg,
		loadDiff:  defaultLoadDiff,
		width:     singleWidth,
		height:    24,
		focusPane: paneTasks,
		focusPrev: paneTasks,
		themeName: "dark",
	}
}

// Run loads the run's snapshot and starts the mission-view program. A state
// dir with no database shows the empty state; any other load failure (an
// unknown run ID included) is returned before the program starts.
func Run(ctx context.Context, cfg Config) error {
	m := newModel(cfg)
	snap, err := viewmodel.Load(ctx, cfg.StateDir, cfg.RunID)
	if err != nil {
		if errors.Is(err, journal.ErrNoDatabase) {
			m.empty = true
		} else {
			return err
		}
	} else {
		m.setSnapshot(ctx, snap)
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

// setSnapshot installs a snapshot and (re)loads its candidate diff. The
// selected task keeps its identity across reloads; only a first load picks
// the default.
func (m *Model) setSnapshot(ctx context.Context, snap viewmodel.Snapshot) {
	m.snap = snap
	m.ready = true
	m.loadErr = nil
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
		return Diff{}, fmt.Errorf("tui: invalid candidate revision")
	}
	out, err := workspace.Git(ctx, ws, false, "diff", "--no-color", "--no-ext-diff", "--no-textconv", base, commit) //nolint:misspell // Git's flag is --no-color.
	if err != nil {
		return Diff{}, err
	}
	return ParseDiff(out)
}

// Init implements tea.Model; the task 6 program has no start-up commands.
func (m *Model) Init() tea.Cmd { return nil }

// Update implements tea.Model: resizes re-lay out around preserved identity,
// keys dispatch through handleKey (nav.go), where Ctrl-C alone quits.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.applySize(msg)
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
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
		rows := m.compactLines()
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
// the truncation notice when the detail pane shows a truncated diff, and the
// persistent critical status.
func (m *Model) viewSingle() string {
	var rows []line
	rows = append(rows, m.tabsLine())
	bodyH := m.height - 3
	if m.showNotice(layoutSingle) {
		bodyH -= 2
	}
	rows = append(rows, capLines(m.focusedPaneLines(m.width), bodyH, m.cfg.Caps, m.cfg.Tokens.TextMuted)...)
	if m.showNotice(layoutSingle) {
		rows = append(rows, m.truncationNotice()...)
	}
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
	detail := column{width: widths[2], rows: m.detailLines(widths[2])}
	return m.viewColumns(layoutThreePane, []column{m.tasksColumn(widths[0]), m.agentsColumn(widths[1]), detail})
}

// column is one pane's fitted rows plus its width.
type column struct {
	width int
	rows  []line
}

func (m *Model) tasksColumn(width int) column {
	return column{width: width, rows: m.applyFilter(m.tasksLines(width))}
}

func (m *Model) agentsColumn(width int) column {
	return column{width: width, rows: m.applyFilter(m.agentsLines(width))}
}

// detailColumn is the switchable detail panel: the selected change when it
// holds focus, else the agents pane. Detail rows are built at the column
// width so the diff renderer fits before styling.
func (m *Model) detailColumn(width int) column {
	if m.focusPane == paneDetail {
		return column{width: width, rows: m.detailLines(width)}
	}
	return column{width: width, rows: m.agentsLines(width)}
}

// focusedPaneLines is the single rung's full-width pane. The filter applies
// to the tasks and agents panes only; the detail pane is never filtered.
func (m *Model) focusedPaneLines(width int) []line {
	switch m.focusPane {
	case paneAgents:
		return m.applyFilter(m.agentsLines(width))
	case paneDetail:
		return m.detailLines(width)
	default:
		return m.applyFilter(m.tasksLines(width))
	}
}

// viewColumns assembles the header, side-by-side panes, notice, status strip
// and footer for the two- and three-pane rungs.
func (m *Model) viewColumns(l layout, cols []column) string {
	widths := columnWidths(m.width, len(cols))
	sep := " " + vBarGlyph(m.cfg.Caps) + " "
	chrome := 2 + 2 + 1 // header, status strip, footer
	if m.showNotice(l) {
		chrome += 2
	}
	bodyH := m.height - chrome
	if bodyH < 1 {
		bodyH = 1
	}
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
	max := 0
	for _, col := range cols {
		if len(col.rows) > max {
			max = len(col.rows)
		}
	}
	out := make([]string, 0, max)
	for i := 0; i < max; i++ {
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

// capLines caps rows at max with a labelled marker row when rows are dropped.
func capLines(rows []line, max int, c caps.Caps, colour string) []line {
	if max < 1 {
		max = 1
	}
	if len(rows) <= max {
		return rows
	}
	marker := line{text: ellGlyph(c) + fmt.Sprintf(" and %d more lines", len(rows)-max+1), colour: colour}
	return append(rows[:max-1], marker)
}
