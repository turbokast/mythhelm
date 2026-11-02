package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
)

// verified reports whether the snapshot's candidate is verified: a passed
// verification row that checked exactly the frozen candidate commit (I07).
// Without a frozen candidate there is nothing the check could verify, so a
// passed row alone never verifies (matching verificationLines, which also
// withholds the label when the candidate is nil).
func verified(snap viewmodel.Snapshot) bool {
	if snap.Verification == nil || snap.Verification.Result != "passed" {
		return false
	}
	return snap.Candidate != nil && snap.Candidate.Commit != "" && snap.Verification.CandidateCommit == snap.Candidate.Commit
}

// stopPending reports whether a confirmed stop still awaits worker
// confirmation: the flag is set and the run is still in a state a stop
// request can be outstanding against (I06).
func (m *Model) stopPending() bool {
	if !m.stopRequested {
		return false
	}
	switch m.snap.Run.State {
	case "executing", "verifying", "stopping":
		return true
	default:
		return false
	}
}

// stopConverged reports whether a reloaded snapshot retires a pending stop
// request: the attempt reached stopped, or the run left the states a stop
// request can be outstanding against.
func stopConverged(snap viewmodel.Snapshot) bool {
	if snap.Attempt != nil && snap.Attempt.State == "stopped" {
		return true
	}
	switch snap.Run.State {
	case "executing", "verifying", "stopping":
		return false
	default:
		return true
	}
}

// nextActionText is the next-action row with the pending-stop override: a
// confirmed stop renders stop requested until the worker confirms it, never
// stopped (I06).
func (m *Model) nextActionText() string {
	if m.stopPending() {
		return "stop requested " + dashGlyph(m.cfg.Caps) + " waiting for worker confirmation"
	}
	return m.observatoryNext()
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
	return []line{m.observatoryHeader(m.width)}
}

// paneHeader keeps focus on one marker cell; its label is bold Text with an
// inherited-text-colour underline, never a longer focus-coloured run.
func (m *Model) paneHeader(id, title string) line {
	if m.focusPane != id {
		return line{text: "  " + title, colour: m.cfg.Tokens.TextMuted}
	}
	return lineFromSpans(span{text: focusGlyph(m.cfg.Caps), colour: m.cfg.Tokens.Focus}, span{text: " " + title, colour: m.cfg.Tokens.Text, bold: true, underline: true})
}

// tasksLines preserves the plain text mirror used by filtering. Failed task
// rows retain the static Err emphasis also applied by the motion wrapper.
func (m *Model) tasksLines(width int) []line {
	t, c := m.cfg.Tokens, m.cfg.Caps
	spans := []span{{text: focusGlyph(c), colour: t.Focus}, {text: " " + cellOrUnknown(m.snap.Run.RunID) + " ", colour: t.Text}}
	spans = append(spans, m.stateSpans()...)
	tail := " " + cellOrUnknown(m.snap.Goal)
	if m.snap.Run.Reason != "" {
		tail += " " + dashGlyph(c) + " " + cell(m.snap.Run.Reason)
	}
	spans = append(spans, span{text: tail, colour: t.Text})
	if isFailureState(m.snap.Run.State) {
		for i := range spans {
			spans[i].colour = t.Err
		}
	}
	rows := []line{m.paneHeader(paneTasks, "TASKS"), {text: m.ruleLine(width), colour: t.Border}, lineFromSpans(spans...)}
	if m.snap.Run.Reason != "" {
		rows = append(rows, line{text: "reason: " + cell(m.snap.Run.Reason), colour: t.Text})
	}
	return rows
}

// agentsLines renders the agents pane: one lane per attempt with its runtime
// label, activity and workspace — never a vendor logo (AC-1.3).
func (m *Model) agentsLines(width int) []line {
	out := []line{m.paneHeader(paneAgents, "AGENT"), {text: m.ruleLine(width), colour: m.cfg.Tokens.Border}}
	if m.motion.pulseFrames > 0 || m.dispatchWaiting() {
		out = append(out, line{text: "  " + m.activityText()})
	}
	return append(out, m.observatoryAgent(width)...)
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
	out := []line{m.paneHeader(paneDetail, "DIFF"), {text: m.ruleLine(width)}}
	switch {
	case m.snap.Candidate == nil:
		out = append(out, line{text: "no candidate frozen yet"})
	case m.diffErr != nil:
		out = append(out, line{text: "diff unavailable: " + cell(m.diffErr.Error())})
	default:
		rows := m.renderDiff(m.diff, width)
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

// detailViewportLines bounds formatter input before rendering. The original
// diff remains the export source; viewport omission is not physical truncation.
func (m *Model) detailViewportLines(width, height int) []line {
	if m.snap.Candidate == nil || m.diffErr != nil {
		return capLines(m.detailLines(width), height, m.cfg.Caps, m.cfg.Tokens.TextMuted)
	}
	proof := m.verificationLines()
	diffRows := len(m.diff.Lines)
	if diffRows == 0 && !m.diff.Truncated {
		diffRows = 1 // the formatter's labelled empty diff
	}
	total := 2 + diffRows + len(proof)
	if total <= height {
		return m.detailLines(width)
	}
	out := []line{m.paneHeader(paneDetail, "DIFF"), {text: m.ruleLine(width)}}
	// Keep one row for the exact same omission marker as render-then-cap.
	prefix := max(0, height-1)
	if prefix < len(out) {
		out = out[:prefix]
	} else {
		visible := min(diffRows, prefix-len(out))
		if visible > 0 {
			diff := m.diff
			diff.Lines = diff.Lines[:min(visible, len(diff.Lines))]
			rows := m.renderDiff(diff, width)
			if diff.Truncated {
				rows = rows[:len(rows)-1]
			}
			for _, row := range rows {
				out = append(out, line{text: row})
			}
		}
		out = append(out, proof[:min(len(proof), prefix-len(out))]...)
	}
	return append(out, line{
		text:   ellGlyph(m.cfg.Caps) + fmt.Sprintf(" and %d more lines", total-prefix),
		colour: m.cfg.Tokens.TextMuted,
	})
}

// renderDiff keeps the formatter local to a model, including instrumentation.
func (m *Model) renderDiff(diff Diff, width int) []string {
	if m.diffRenderer != nil {
		return m.diffRenderer(diff, width)
	}
	return diff.Render(width, m.cfg.Caps, m.cfg.Tokens)
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
			text:   "mismatch: checks ran against " + shortRev(cellOrUnknown(checks)) + " " + dashGlyph(m.cfg.Caps) + " candidate is " + shortRev(cellOrUnknown(m.snap.Candidate.Commit)),
			colour: t.Err,
		}}
	}
	if m.snap.Candidate == nil || (v.Result == "passed" && !verified(m.snap)) {
		return []line{{text: "verification: " + cellOrUnknown(v.Result) + " (checked " + shortRev(cellOrUnknown(checks)) + ")"}}
	}
	short := shortRev(cellOrUnknown(v.CandidateCommit))
	switch v.Result {
	case "passed":
		out := []line{{text: "verified " + short, colour: t.OK}}
		return append(out, m.checkLines()...)
	case "failed":
		out := []line{{text: "verification: failed (" + short + ")", colour: t.Err}}
		return append(out, m.checkLines()...)
	case "not_run", "NOT RUN", "unavailable":
		return []line{{text: "verification: NOT RUN (" + reasonOrUnknown(v.Reason) + ")"}}
	case "invalid":
		return []line{{text: "verification: INVALID (" + reasonOrUnknown(v.Reason) + ")", colour: t.Err}}
	case "":
		return []line{{text: "verification: unknown (" + short + ")"}}
	default:
		return []line{{text: "verification: " + cell(v.Result) + " (" + short + ")"}}
	}
}

// reasonOrUnknown is a recorded verification reason, or "reason unknown" for
// a run recorded before reasons existed: never blank, never invented (I09).
func reasonOrUnknown(reason string) string {
	if reason == "" {
		return "reason unknown"
	}
	return cell(reason)
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
		{text: m.entitlementText(), colour: m.cfg.Tokens.TextMuted},
		{text: "Next action: " + m.nextActionText(), colour: m.cfg.Tokens.Text},
	}
}

// entitlementText is the recorded entitlement sentence verbatim, or
// "entitlement: unknown" when no admission (or no statement) was recorded: a
// declared entitlement is never worded as verified (I15).
func (m *Model) entitlementText() string {
	if a := m.snap.Admission; a != nil && a.EntitlementStatement != "" {
		return cell(a.EntitlementStatement)
	}
	return "entitlement: unknown"
}

// nativeExitText renders the agent's completion report as a native exit, never
// as verification (I07).
func (m *Model) nativeExitText() string {
	n := m.snap.NativeExit
	if n == nil {
		return "unknown"
	}
	if n.ExitCode != nil {
		return strconv.Itoa(*n.ExitCode)
	}
	if n.Signal != nil && *n.Signal != "" {
		return "signal " + cell(*n.Signal)
	}
	return "unknown"
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
		{text: first, colour: m.cfg.Tokens.TextMuted},
		{text: "`" + cmd + "`", colour: m.cfg.Tokens.TextMuted},
	}
}

// tabsLine renders the single-pane tab bar with the focused tab bracketed.
func (m *Model) tabsLine() line {
	t := m.cfg.Tokens
	var spans []span
	for i, tab := range []struct{ id, title string }{{paneTasks, "Tasks"}, {paneAgents, "Agent"}, {paneEvidence, "Evidence"}, {paneDetail, "Diff"}} {
		if i > 0 {
			spans = append(spans, span{text: "  ", colour: t.TextMuted})
		}
		if tab.id == m.focusPane {
			spans = append(spans, span{text: focusGlyph(m.cfg.Caps), colour: t.Focus}, span{text: " " + tab.title, colour: t.Text, bold: true})
		} else {
			spans = append(spans, span{text: tab.title, colour: t.TextMuted})
		}
	}
	return lineFromSpans(spans...)
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
		text:   cellOrUnknown(m.snap.Run.State) + " " + dashGlyph(m.cfg.Caps) + " " + m.nextActionText(),
		colour: m.cfg.Tokens.Text,
	}
}

// stateSpans is the closed Observatory vocabulary. Native success alone is
// never verification: ready_for_review gets an OK marker only for a matching
// passed check of a nonempty frozen candidate commit.
func (m *Model) stateSpans() []span {
	c, t := m.cfg.Caps, m.cfg.Tokens
	marker, word, colour := unknownGlyph(c), "unknown", t.TextMuted
	switch m.snap.Run.State {
	case "created":
		marker, word = pendingGlyph(c), "starting"
	case "admission":
		marker, word = pendingGlyph(c), "admitting"
	case "executing":
		marker, word, colour = dotGlyph(c), "working", t.Focus
	case "verifying":
		marker, word, colour = dotGlyph(c), "checking", t.Focus
	case "ready_for_review":
		word = "ready, unverified"
		if verified(m.snap) {
			marker, word, colour = dotGlyph(c), "ready for review", t.OK
		}
	case "applying":
		marker, word, colour = dotGlyph(c), "applying", t.Focus
	case "completed":
		marker, word = checkGlyph(c), "done"
	case "stopping":
		marker, word = pendingGlyph(c), "stop requested"
	case "cancelled":
		marker, word = checkGlyph(c), "stopped"
	case "interrupted":
		word = "interrupted"
	case "recovering":
		marker, word, colour = dotGlyph(c), "recovering", t.Focus
	case "blocked":
		marker, word, colour = "[fail]", "blocked", t.Err
	case "failed":
		marker, word, colour = "[fail]", "failed", t.Err
	}
	return []span{{text: marker, colour: colour}, {text: " " + word, colour: t.Text}}
}

func (m *Model) compactUsage(width int) line {
	var values []string
	for _, u := range m.snap.Usage {
		text := observatoryValue(u.Scope) + "/" + observatoryValue(u.Unit) + " " + observatoryValue(u.Quantity) + " (" + observatoryValue(u.Label) + ")"
		if u.Scope == "retail-equivalent" {
			text += " estimate, not a charge"
		}
		values = append(values, text)
	}
	text := "unknown (no usage recorded)"
	if len(values) > 0 {
		text = strings.Join(values, "; ")
	}
	return m.observatoryLabel("use", text, width)
}
