package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/turbokast/mythhelm/internal/tui/caps"
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
	return nextAction(m.snap, m.cfg.Caps)
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
	snap, t, c := m.snap, m.cfg.Tokens, m.cfg.Caps
	spans := []span{
		{text: missionGlyph(c), colour: t.Focus},
		{text: " MYTHHELM", colour: t.Text, bold: true},
		{text: "  " + cellOrUnknown(snap.Run.RunID) + "  " + laneAdapter(snap) + " / " + cellOrUnknown(snap.Run.ExecutionProfile) + "  ", colour: t.Text},
	}
	spans = append(spans, m.stateSpans()...)
	return []line{lineFromSpans(spans...), lineFromSpans(span{text: "Goal: " + cellOrUnknown(snap.Goal), colour: t.Text})}
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
	spans := []span{{text: focusGlyph(c), colour: t.Focus}, {text: " ", colour: t.Text}}
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
	return []line{m.paneHeader(paneTasks, "TASKS"), {text: m.ruleLine(width)}, lineFromSpans(spans...)}
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
	head := laneLetter(n) + " " + laneDotGlyph(c) + " attempt " + strconv.FormatInt(n, 10) + "  " +
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
	switch {
	case m.snap.Candidate == nil:
		out = append(out, line{text: "no candidate frozen yet"})
	case m.diffErr != nil:
		out = append(out, line{text: "diff unavailable: " + cell(m.diffErr.Error())})
	default:
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
			text:   "mismatch: checks ran against " + shortRev(cell(checks)) + " " + dashGlyph(m.cfg.Caps) + " candidate is " + shortRev(cell(m.snap.Candidate.Commit)),
			colour: t.Err,
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
		{text: first, colour: m.cfg.Tokens.TextMuted},
		{text: "`" + cmd + "`", colour: m.cfg.Tokens.TextMuted},
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

// compactOfferRows is the leading run of compactLines rows the five-row
// offer occupies; notice reservation splits the body here.
const compactOfferRows = 5

// compactLines renders the compact task/status view with its linear-mode
// offer (AC-2.4). The first five rows carry the offer, so even a 5-row
// terminal sees it; later rows add activity, verification and billing detail
// when the height allows.
func (m *Model) compactLines() []line {
	c := m.cfg.Caps
	lines := append(m.headerLines(), lineFromSpans(append([]span{{text: "state: ", colour: m.cfg.Tokens.Text}}, m.stateSpans()...)...))
	lines = append(lines, []line{
		{text: "next action: " + m.nextActionText()},
		{text: "compact view " + dashGlyph(c) + " small terminal (--accessible for linear)"},
	}...)
	lines = append(lines, line{text: "activity: " + m.activityText()})
	if v := m.verificationLines(); len(v) > 0 {
		lines = append(lines, v[0])
	}
	return append(lines, m.statusLines()[:2]...)
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
		text:   cellOrUnknown(m.snap.Run.State) + " " + dashGlyph(m.cfg.Caps) + " " + m.nextActionText(),
		colour: m.cfg.Tokens.Attention,
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
