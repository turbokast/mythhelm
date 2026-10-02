package tui

import (
	"fmt"
	"strings"

	"github.com/turbokast/mythhelm/internal/tui/caps"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
)

// verified reports whether the snapshot's candidate is verified: a passed
// verification row that checked exactly the frozen candidate commit (I07).
func verified(snap viewmodel.Snapshot) bool {
	if snap.Verification == nil || snap.Verification.Result != "passed" {
		return false
	}
	return snap.Candidate == nil || snap.Verification.CandidateCommit == snap.Candidate.Commit
}

// nextAction maps the run state to its design §7 next-action string. Stopping
// stays "waiting for worker confirmation" — requested is never labelled
// stopped (I06).
func nextAction(snap viewmodel.Snapshot, c caps.Caps) string {
	d := " " + dashGlyph(c) + " "
	switch snap.Run.State {
	case "created", "admission":
		return "admitting" + d + "no action"
	case "executing", "verifying":
		return "none" + d + "agent working (stop available)"
	case "stopping":
		return "waiting for worker confirmation" + d + "stop requested, not confirmed"
	case "ready_for_review":
		if verified(snap) {
			return "review the candidate, then apply or close"
		}
		return "verification unavailable" + d + "apply needs explicit unverified acceptance"
	case "blocked", "failed":
		return "read the reason; recover or start a new run"
	case "interrupted":
		return "recover the run"
	case "recovering":
		return "recovery running" + d + "no action"
	case "applying", "completed", "cancelled":
		return "none" + d + "run is terminal"
	default:
		return "unknown"
	}
}

// stateChip maps a run state to its §15.3 text chip.
func stateChip(state string) string {
	switch state {
	case "executing", "verifying", "recovering", "applying":
		return "[run ]"
	case "created", "admission", "interrupted":
		return "[wait]"
	case "ready_for_review", "stopping":
		return "[req ]"
	case "blocked", "failed":
		return "[fail]"
	case "completed":
		return "[done]"
	case "cancelled":
		return "[stop]"
	default:
		return "[????]"
	}
}

// shortRev abbreviates a revision for pairing labels; short inputs pass
// through rather than panicking.
func shortRev(rev string) string {
	if len(rev) <= 7 {
		return rev
	}
	return rev[:7]
}

// laneLetter is the stable local lane identifier for attempt n (A, B, ...).
func laneLetter(n int64) string {
	if n < 1 {
		return "?"
	}
	return string(rune('A' + (n-1)%26))
}

// laneAdapter resolves the lane's adapter id: the recorded admission first,
// then the run projection, never blank (I09).
func laneAdapter(snap viewmodel.Snapshot) string {
	if snap.Admission != nil && snap.Admission.AdapterID != "" {
		return cell(snap.Admission.AdapterID)
	}
	return cellOrUnknown(snap.Run.AdapterID)
}

// headerLines renders the mission header: run, lane label, state and goal.
func (m *Model) headerLines() []line {
	snap := m.snap
	c := m.cfg.Caps
	title := strings.Join([]string{
		"MYTHHELM",
		cellOrUnknown(snap.Run.RunID),
		laneAdapter(snap) + " / " + cellOrUnknown(snap.Run.ExecutionProfile),
		dotGlyph(c) + " " + cellOrUnknown(snap.Run.State),
	}, "  ")
	return []line{
		{text: title, colour: m.cfg.Tokens.Text},
		{text: "Goal: " + cellOrUnknown(snap.Goal), colour: m.cfg.Tokens.Text},
	}
}

// paneHeader renders one pane title with its focus marker.
func (m *Model) paneHeader(id, title string) line {
	c := m.cfg.Caps
	marker := "  "
	colour := m.cfg.Tokens.TextMuted
	if m.focusPane == id {
		marker = focusGlyph(c) + " "
		colour = m.cfg.Tokens.Focus
	}
	return line{text: marker + title, colour: colour}
}

// tasksLines renders the tasks pane: the run's task row with its state chip.
func (m *Model) tasksLines(width int) []line {
	c := m.cfg.Caps
	row := focusGlyph(c) + " " + stateChip(m.snap.Run.State) + " " +
		cellOrUnknown(m.snap.Goal) + " (" + cellOrUnknown(m.snap.Run.State) + ")"
	if m.snap.Run.Reason != "" {
		row += " " + dashGlyph(c) + " " + cell(m.snap.Run.Reason)
	}
	return []line{
		m.paneHeader(paneTasks, "TASKS"),
		{text: m.ruleLine(width)},
		{text: row, colour: m.cfg.Tokens.Text},
	}
}

// agentsLines renders the agents pane: one lane per attempt with its runtime
// label, activity and workspace — never a vendor logo (AC-1.3).
func (m *Model) agentsLines(width int) []line {
	out := []line{m.paneHeader(paneAgents, "AGENTS"), {text: m.ruleLine(width)}}
	if m.snap.Attempt == nil {
		return append(out, line{text: "no attempts yet"})
	}
	c := m.cfg.Caps
	n := m.snap.Attempt.AttemptNumber
	head := laneLetter(n) + " " + laneDotGlyph(c) + " attempt " + fmt.Sprint(n) + "  " +
		laneAdapter(m.snap) + " / " + cellOrUnknown(m.snap.Run.ExecutionProfile)
	out = append(out, line{text: head, colour: m.cfg.Tokens.Text})
	out = append(out, line{text: "  " + m.activityText()})
	out = append(out, line{text: "  workspace: " + cellOrUnknown(m.snap.Attempt.WorkspacePath)})
	return out
}

// activityText is the lane's current activity: the latest native-reported
// progress counters, else the attempt state.
func (m *Model) activityText() string {
	if p := m.snap.LatestProgress; p != nil {
		s := fmt.Sprintf("%d assistant turns (native-reported)", p.AssistantTurns)
		if p.Retries > 0 {
			s += fmt.Sprintf(", retries: %d", p.Retries)
		}
		return s
	}
	if m.snap.Attempt != nil {
		return "state: " + cellOrUnknown(m.snap.Attempt.State)
	}
	return "unknown"
}

// detailLines renders the selected-change pane: the candidate diff or its
// labelled absence, plus verification against its checked revision (I07).
func (m *Model) detailLines(width int) []line {
	out := []line{m.paneHeader(paneDetail, "SELECTED CHANGE"), {text: m.ruleLine(width)}}
	if m.snap.Candidate == nil {
		out = append(out, line{text: "no candidate frozen yet"})
	} else if m.diffErr != nil {
		out = append(out, line{text: "diff unavailable: " + cell(m.diffErr.Error())})
	} else {
		rows := m.diff.Render(width, m.cfg.Caps, m.cfg.Tokens)
		if m.diff.Truncated && len(rows) > 0 {
			// The in-pane counts footer is replaced by the full-width
			// provenance notice below the columns, which also names the
			// exact fallback command the bare diff cannot know.
			rows = rows[:len(rows)-1]
		}
		for _, r := range rows {
			out = append(out, line{text: r})
		}
	}
	return append(out, m.verificationLines()...)
}

// verificationLines pairs the verification result with its checked revision:
// a mismatch reads "checks ran against A — candidate is B" and never
// "verified" (AC-7.2); only a passed check of the frozen commit verifies.
func (m *Model) verificationLines() []line {
	v := m.snap.Verification
	if v == nil {
		return []line{{text: "verification: waiting"}}
	}
	t := m.cfg.Tokens
	checks := v.CandidateCommit
	if m.snap.Candidate != nil && v.CandidateCommit != m.snap.Candidate.Commit {
		return []line{{
			text:   "checks ran against " + shortRev(cell(checks)) + " " + dashGlyph(m.cfg.Caps) + " candidate is " + shortRev(cell(m.snap.Candidate.Commit)),
			colour: t.Warning,
		}}
	}
	if m.snap.Candidate == nil {
		return []line{{text: "verification: " + cellOrUnknown(v.Result) + " (checked " + shortRev(cellOrUnknown(checks)) + ")"}}
	}
	short := shortRev(cell(v.CandidateCommit))
	switch v.Result {
	case "passed":
		out := []line{{text: "verified " + short, colour: t.OK}}
		return append(out, m.checkLines()...)
	case "failed":
		out := []line{{text: "verification: failed (" + short + ")", colour: t.Err}}
		return append(out, m.checkLines()...)
	case "not_run", "NOT RUN":
		return []line{{text: "verification: NOT RUN (waived by --no-checks)"}}
	case "":
		return []line{{text: "verification: unknown (" + short + ")"}}
	default:
		return []line{{text: "verification: " + cell(v.Result) + " (" + short + ")"}}
	}
}

// checkLines renders one row per recorded check with its verbatim status.
func (m *Model) checkLines() []line {
	var out []line
	for _, c := range m.snap.Verification.Checks {
		out = append(out, line{text: "  " + cellOrUnknown(c.Name) + ": " + cellOrUnknown(c.Status)})
	}
	return out
}

// statusLines renders the status strip: billing posture, kind-labelled
// admission figures and the next required action (AC-7.3).
func (m *Model) statusLines() []line {
	c := m.cfg.Caps
	sep := "  " + vBarGlyph(c) + " "
	qualified, paid := "unknown", "unknown"
	if a := m.snap.Admission; a != nil {
		if a.Qualified {
			qualified = "true"
		} else {
			qualified = "false"
		}
		if a.PaidContinuation != "" {
			paid = cell(a.PaidContinuation)
		}
	}
	billing := "Billing: " + cellOrUnknown(m.snap.Run.BillingPosture) + sep +
		"qualified: " + qualified + sep +
		"paid continuation: " + paid + sep +
		"native exit: " + m.nativeExitText()
	return []line{
		{text: billing, colour: m.cfg.Tokens.TextMuted},
		{text: "Next action: " + nextAction(m.snap, c), colour: m.cfg.Tokens.Text},
	}
}

// nativeExitText renders the agent's completion report as a native exit, never
// as verification (I07).
func (m *Model) nativeExitText() string {
	n := m.snap.NativeExit
	if n == nil {
		return "unknown"
	}
	if n.ExitCode != nil {
		return fmt.Sprint(*n.ExitCode)
	}
	if n.Signal != nil && *n.Signal != "" {
		return "signal " + cell(*n.Signal)
	}
	return "unknown"
}

// footerLine is the §15.3 key line.
func (m *Model) footerLine() line {
	sep := " " + sepDotGlyph(m.cfg.Caps) + " "
	return line{
		text:   strings.Join([]string{"Tab focus", "/ filter", ": commands", "? help", "Enter inspect", "q exit options"}, sep),
		colour: m.cfg.Tokens.TextMuted,
	}
}

// truncationNotice is the full-width provenance notice for a truncated diff:
// retained over total line counts, the cap that fired, and the exact fallback
// command with base and commit from the candidate row. It spans the full
// width because the command never fits a pane column.
func (m *Model) truncationNotice() []line {
	retained := len(m.diff.Lines)
	label := "50,000-line limit"
	if m.diff.TruncateCap == truncateBytes {
		label = "4 MiB byte limit"
	}
	first := fmt.Sprintf("showing %d of %d lines (%s) %s full diff via:",
		retained, m.diff.TotalLines(), label, dashGlyph(m.cfg.Caps))
	cmd := "git -C \"" + cell(m.snap.Attempt.WorkspacePath) + "\" diff " +
		cell(m.snap.Candidate.BaseRev) + " " + cell(m.snap.Candidate.Commit)
	return []line{
		{text: first, colour: m.cfg.Tokens.Warning},
		{text: "`" + cmd + "`", colour: m.cfg.Tokens.Warning},
	}
}

// showNotice reports whether the truncation notice applies to the current
// view: a truncated diff behind a visible selected-change pane.
func (m *Model) showNotice(l layout) bool {
	if !m.diff.Truncated || m.snap.Attempt == nil || m.snap.Candidate == nil {
		return false
	}
	switch l {
	case layoutThreePane:
		return true
	case layoutTwoPane, layoutSingle:
		return m.focusPane == paneDetail
	default:
		return false
	}
}

// compactLines renders the compact task/status view with its linear-mode
// offer (AC-2.4). The first five rows carry the offer, so even a 5-row
// terminal sees it; later rows add activity, verification and billing detail
// when the height allows.
func (m *Model) compactLines() []line {
	c := m.cfg.Caps
	snap := m.snap
	lines := []line{
		{text: "MYTHHELM  " + cellOrUnknown(snap.Run.RunID) + "  " + cellOrUnknown(snap.Run.State), colour: m.cfg.Tokens.Text},
		{text: "Goal: " + cellOrUnknown(snap.Goal), colour: m.cfg.Tokens.Text},
		{text: "state: " + stateChip(snap.Run.State) + " " + cellOrUnknown(snap.Run.State)},
		{text: "next action: " + nextAction(snap, c)},
		{text: "compact view " + dashGlyph(c) + " small terminal (--accessible for linear)"},
	}
	lines = append(lines, line{text: "activity: " + m.activityText()})
	if v := m.verificationLines(); len(v) > 0 {
		lines = append(lines, v[0])
	}
	lines = append(lines, m.statusLines()[0])
	return lines
}

// tabsLine renders the single-pane tab bar with the focused tab bracketed.
func (m *Model) tabsLine() line {
	tabs := []struct {
		id    string
		title string
	}{
		{paneTasks, "Tasks"},
		{paneAgents, "Agents"},
		{paneDetail, "Selected change"},
	}
	parts := make([]string, 0, len(tabs))
	for _, t := range tabs {
		if m.focusPane == t.id {
			parts = append(parts, "["+t.title+"]")
		} else {
			parts = append(parts, t.title)
		}
	}
	return line{text: strings.Join(parts, "  "), colour: m.cfg.Tokens.Text}
}

// ruleLine is the horizontal rule under a pane title, width cells wide.
func (m *Model) ruleLine(width int) string {
	if width < 1 {
		width = 1
	}
	return strings.Repeat(ruleGlyph(m.cfg.Caps), width)
}

// criticalStatusLine is the single pane's persistent critical status.
func (m *Model) criticalStatusLine() line {
	return line{
		text:   cellOrUnknown(m.snap.Run.State) + " " + dashGlyph(m.cfg.Caps) + " " + nextAction(m.snap, m.cfg.Caps),
		colour: m.cfg.Tokens.Attention,
	}
}
