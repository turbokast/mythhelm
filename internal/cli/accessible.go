// Linear screen-reader renderer: the --accessible output mode.
//
// RunAccessible streams one view-model snapshot's state labels followed by
// the run's journaled events in run_sequence order, then follows live until
// its context ends. It never emits cursor movement, alternate-screen codes
// or per-tick chatter: a poll that finds no new events writes nothing. Long
// runs page through repeated invocations: each invocation streams at most
// accessiblePageSize history events after After, and its last line is always
// the next-after trailer naming the cursor the next invocation resumes from.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
)

// AccessibleConfig configures one accessible-stream invocation.
type AccessibleConfig struct {
	RunID    string
	StateDir string
	Out      io.Writer
	// Poll is the live-follow re-read cadence; non-positive means 500ms.
	Poll time.Duration
	// After is the resume offset: only events with run_sequence > After
	// stream (0 starts from the beginning).
	After int64
}

// accessiblePageSize bounds one invocation's initial history page; the
// trailer cursor lets the next invocation resume where this one stopped.
const accessiblePageSize = 1000

// accessibleMaxWidth bounds every emitted line in terminal cells. Content
// past the cap truncates at a cell boundary with a labelled marker, so a
// hostile value can never push the adjacent status line out of shape.
const accessibleMaxWidth = 200

// defaultAccessiblePoll is the live-follow cadence when Poll is unset.
const defaultAccessiblePoll = 500 * time.Millisecond

// RunAccessible writes the accessible label stream for cfg.RunID to cfg.Out.
// Fresh invocations that reach the present (After == 0 with at most one page
// of history) start with the current-state summary; resume and paging
// invocations are pure stream continuations, so labels never repeat across
// pages. New events stream as they journal; polls with no new events write
// nothing. On context end it writes the next-after trailer as its last line
// and returns nil. Load, stream and write failures return an error with no
// trailer.
func RunAccessible(ctx context.Context, cfg AccessibleConfig) error {
	if cfg.Out == nil {
		return fmt.Errorf("accessible: no output writer")
	}
	poll := cfg.Poll
	if poll <= 0 {
		poll = defaultAccessiblePoll
	}
	snap, err := viewmodel.Load(ctx, cfg.StateDir, cfg.RunID)
	if err != nil {
		return err
	}
	history, err := viewmodel.EventsSince(ctx, cfg.StateDir, cfg.RunID, cfg.After)
	if err != nil {
		return err
	}
	w := &accessibleWriter{out: cfg.Out}
	last := cfg.After
	page := history
	if len(page) > accessiblePageSize {
		page = page[:accessiblePageSize]
	}
	if accessibleShowsSummary(cfg.After, len(history)) {
		for _, line := range accessibleSummary(snap) {
			w.line(line)
		}
	}
	for _, ev := range page {
		w.line(accessibleEventLine(ev))
		last = ev.RunSequence
	}
	if err := w.err; err != nil {
		return err
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.line(fmt.Sprintf("next-after: %d", last))
			return w.err
		case <-ticker.C:
			events, err := viewmodel.EventsSince(ctx, cfg.StateDir, cfg.RunID, last)
			if err != nil {
				if ctx.Err() != nil {
					// Lost the race with cancellation: shut down clean.
					w.line(fmt.Sprintf("next-after: %d", last))
					return w.err
				}
				return err
			}
			for _, ev := range events {
				w.line(accessibleEventLine(ev))
				last = ev.RunSequence
			}
			if err := w.err; err != nil {
				return err
			}
		}
	}
}

// accessibleShowsSummary reports whether this invocation opens with the
// current-state summary: fresh invocations whose whole history fits one
// page. EventsSince is uncapped, so a history of exactly accessiblePageSize
// is complete, not truncated. Resume and paging invocations stay pure
// stream continuations, so labels never repeat across pages.
func accessibleShowsSummary(after int64, historyLen int) bool {
	return after == 0 && historyLen <= accessiblePageSize
}

// cellOrUnknown sanitises a stored value for a state label; an empty value
// is absent, so it renders unknown, never blank (I09).
func cellOrUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return cell(s)
}

// accessibleWriter keeps the first write error; every line passes through
// fitAccessibleLine so output is single-line, bidi-safe and width-capped.
type accessibleWriter struct {
	out io.Writer
	err error
}

func (w *accessibleWriter) line(s string) {
	if w.err != nil {
		return
	}
	_, w.err = io.WriteString(w.out, fitAccessibleLine(s)+"\n")
}

// accessibleSummary renders the snapshot's current-state labels, one per
// line. Absent values render unknown, never zero or blank (I09); every
// stored value is terminal-sanitised before it interpolates.
func accessibleSummary(snap viewmodel.Snapshot) []string {
	lines := []string{
		"goal: " + cell(snap.Goal),
		"run state: " + cellOrUnknown(snap.Run.State) + " (reason: " + reasonOrNone(snap.Run.Reason) + ")",
		accessibleAttemptLine(snap),
		accessibleCandidateLine(snap),
		accessibleVerificationLine(snap),
		accessibleProgressLine(snap),
		accessibleAdmissionLine(snap),
	}
	if snap.NativeExit != nil {
		lines = append(lines, accessibleNativeExitLine(snap))
	}
	return append(lines,
		"next action: "+accessibleNextAction(snap.Run.State),
		"actions: "+accessibleActions(snap.Run.State),
	)
}

func reasonOrNone(reason string) string {
	if reason == "" {
		return "none"
	}
	return cell(reason)
}

func accessibleAttemptLine(snap viewmodel.Snapshot) string {
	if snap.Attempt == nil {
		return "attempt: none recorded"
	}
	line := fmt.Sprintf("attempt %d: %s", snap.Attempt.AttemptNumber, cellOrUnknown(snap.Attempt.State))
	if snap.Attempt.Reason != "" {
		line += " (" + cell(snap.Attempt.Reason) + ")"
	}
	return line
}

// accessibleCandidateLine names the frozen revision with its changed files,
// fitting as many whole filenames as the width cap allows and labelling the
// remainder, so no filename is ever half-rendered or silently dropped.
func accessibleCandidateLine(snap viewmodel.Snapshot) string {
	if snap.Candidate == nil {
		return "candidate: none recorded"
	}
	var paths []string
	if err := json.Unmarshal(snap.Candidate.ChangedPaths, &paths); err != nil {
		return "candidate: " + cellOrUnknown(snap.Candidate.Commit) + " (changed files unknown)"
	}
	head := fmt.Sprintf("candidate: %s (%d files changed", cellOrUnknown(snap.Candidate.Commit), len(paths))
	if len(paths) == 0 {
		return head + ")"
	}
	head += ": "
	// Reserve the longest possible remainder suffix so the line fits even
	// when most filenames spill; the writer's final fit is the backstop.
	suffix := fmt.Sprintf("... and %d more)", len(paths))
	budget := accessibleMaxWidth - accessibleCells(head) - accessibleCells(suffix)
	var names []string
	rest := 0
	for i, name := range paths {
		shown := cell(name)
		extra := accessibleCells(shown)
		if i > 0 {
			extra += len(", ")
		}
		if extra > budget {
			rest = len(paths) - i
			break
		}
		budget -= extra
		names = append(names, shown)
	}
	if rest > 0 {
		return fmt.Sprintf("%s%s... and %d more)", head, strings.Join(names, ", "), rest)
	}
	return head + strings.Join(names, ", ") + ")"
}

func accessibleVerificationLine(snap viewmodel.Snapshot) string {
	if snap.Verification == nil {
		return "verification: waiting"
	}
	result := snap.Verification.Result
	if result == "" {
		result = "unknown"
	}
	if result == "NOT RUN" {
		return "verification: NOT RUN (waived by --no-checks)"
	}
	return fmt.Sprintf("verification: %s (candidate %s)", cell(result), cellOrUnknown(snap.Verification.CandidateCommit))
}

func accessibleProgressLine(snap viewmodel.Snapshot) string {
	if snap.LatestProgress == nil {
		return "progress: unknown"
	}
	line := fmt.Sprintf("progress: %d assistant turns", snap.LatestProgress.AssistantTurns)
	if snap.LatestProgress.Retries > 0 {
		line += fmt.Sprintf(", %d retries", snap.LatestProgress.Retries)
	}
	return line + " (native-reported)"
}

func accessibleAdmissionLine(snap viewmodel.Snapshot) string {
	if snap.Admission == nil {
		return "admission: unknown"
	}
	return fmt.Sprintf("admission: adapter %s %s (%s); qualified: %t; paid continuation: %s",
		cellOrUnknown(snap.Admission.AdapterID), cellOrUnknown(snap.Admission.AdapterVersion),
		cellOrUnknown(snap.Admission.AdapterSurface), snap.Admission.Qualified,
		cellOrUnknown(snap.Admission.PaidContinuation))
}

func accessibleNativeExitLine(snap viewmodel.Snapshot) string {
	native := snap.NativeExit
	exit := "exit code unknown"
	switch {
	case native.ExitCode != nil:
		exit = fmt.Sprintf("exit code %d", *native.ExitCode)
	case native.Signal != nil:
		exit = "signal " + cellOrUnknown(*native.Signal)
	}
	observed := "no result frame observed"
	if native.ResultObserved {
		observed = "result observed"
	}
	return "native exit: " + exit + "; " + observed
}

// accessibleNextAction maps every run state to its required next step,
// keeping design §7's requested-vs-confirmed distinctions in linearised
// copy: requested states never read confirmed.
func accessibleNextAction(state string) string {
	switch state {
	case "created", "admission":
		return "wait for admission to decide"
	case "executing":
		return "wait for worker to finish"
	case "verifying":
		return "wait for verification to finish"
	case "ready_for_review":
		return "review the candidate, then apply to accept it"
	case "applying":
		return "wait for apply to finish"
	case "stopping":
		return "waiting for worker confirmation"
	case "recovering":
		return "wait for recovery to finish"
	case "interrupted":
		return "recover the run to continue"
	case "completed":
		return "none: run completed"
	case "cancelled":
		return "none: run cancelled"
	case "failed", "blocked":
		return "inspect the run; no automatic next step"
	default:
		return "unknown"
	}
}

// accessibleActions names the actions valid in this state by their full
// names, so the stream never depends on single-key hints a screen reader
// cannot see.
func accessibleActions(state string) string {
	switch state {
	case "executing", "verifying":
		return "stop"
	case "interrupted":
		return "recover"
	case "ready_for_review":
		return "apply"
	default:
		return "none available"
	}
}

// accessibleEventLine renders one journaled event as a single ordered stream
// line, reusing the linear renderer's labels so the two cannot drift.
func accessibleEventLine(ev journal.Event) string {
	return fmt.Sprintf("event %d: %s", ev.RunSequence, oneAccessibleLine(plainEvent(ev)))
}

// oneAccessibleLine flattens a rendered event to one line: blank segments go
// and the rest join with "; ", keeping the run_sequence prefix meaningful.
func oneAccessibleLine(s string) string {
	parts := strings.Split(s, "\n")
	kept := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "; ")
}

// fitAccessibleLine makes a composed line safe to emit: bidi controls become
// U+FFFD so hostile text cannot visually reorder the adjacent status line,
// and over-wide lines truncate at a cell boundary with a labelled marker.
func fitAccessibleLine(s string) string {
	s = neutraliseBidi(s)
	if accessibleCells(s) <= accessibleMaxWidth {
		return s
	}
	const marker = "... (truncated)"
	budget := accessibleMaxWidth - len(marker)
	var b strings.Builder
	b.Grow(len(s))
	used := 0
	for _, r := range s {
		w := accessibleRuneWidth(r)
		if used+w > budget {
			break
		}
		used += w
		b.WriteRune(r)
	}
	return b.String() + marker
}

// neutraliseBidi replaces Unicode bidirectional controls with U+FFFD: the
// replacement is visible and width-1, so the text stays honest without being
// able to displace neighbouring lines on a terminal.
func neutraliseBidi(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == 0x061C || r == 0x200E || r == 0x200F:
			return unicode.ReplacementChar
		case r >= 0x202A && r <= 0x202E:
			return unicode.ReplacementChar
		case r >= 0x2066 && r <= 0x2069:
			return unicode.ReplacementChar
		default:
			return r
		}
	}, s)
}

// accessibleCells counts s in terminal cells: combining marks and zero-width
// format runes take none, wide and emoji runes take two, the rest take one.
func accessibleCells(s string) int {
	total := 0
	for _, r := range s {
		total += accessibleRuneWidth(r)
	}
	return total
}

func accessibleRuneWidth(r rune) int {
	switch {
	case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r):
		return 0
	case r == 0x200B || r == 0x200C || r == 0x200D || r == 0xFEFF:
		return 0
	case r >= 0x2060 && r <= 0x2064:
		return 0
	case isAccessibleWide(r):
		return 2
	default:
		return 1
	}
}

// isAccessibleWide reports the East Asian wide/fullwidth intervals plus the
// emoji ranges terminals render double-width (stdlib only: this package must
// not gain a width dependency for one predicate).
func isAccessibleWide(r rune) bool {
	for _, span := range [][2]rune{
		{0x1100, 0x115F},
		{0x231A, 0x231B}, {0x2329, 0x232A},
		{0x23E9, 0x23EC}, {0x23F0, 0x23F0}, {0x23F3, 0x23F3},
		{0x25FD, 0x25FE},
		{0x2614, 0x2615}, {0x2648, 0x2653},
		{0x267F, 0x267F}, {0x2693, 0x2693}, {0x26A1, 0x26A1},
		{0x26AA, 0x26AB}, {0x26BD, 0x26BE}, {0x26C4, 0x26C5},
		{0x26CE, 0x26CE}, {0x26D4, 0x26D4}, {0x26EA, 0x26EA},
		{0x26F2, 0x26F3}, {0x26F5, 0x26F5}, {0x26FA, 0x26FA}, {0x26FD, 0x26FD},
		{0x2705, 0x2705}, {0x270A, 0x270B}, {0x2728, 0x2728},
		{0x274C, 0x274C}, {0x274E, 0x274E},
		{0x2753, 0x2755}, {0x2757, 0x2757},
		{0x2795, 0x2797}, {0x27B0, 0x27B0}, {0x27BF, 0x27BF},
		{0x2B1B, 0x2B1C}, {0x2B50, 0x2B50}, {0x2B55, 0x2B55},
		{0x2E80, 0x303E}, {0x3041, 0x33FF}, {0x3400, 0x4DBF},
		{0x4E00, 0xA4CF}, {0xA960, 0xA97C}, {0xAC00, 0xD7A3},
		{0xF900, 0xFAFF}, {0xFE10, 0xFE19}, {0xFE30, 0xFE6F},
		{0xFF00, 0xFF60}, {0xFFE0, 0xFFE6},
		{0x1F000, 0x1F02F}, {0x1F0A0, 0x1F0FF},
		{0x1F300, 0x1F6FF}, {0x1F900, 0x1F9FF}, {0x1FA70, 0x1FAFF},
		{0x20000, 0x2FFFD}, {0x30000, 0x3FFFD},
	} {
		if r >= span[0] && r <= span[1] {
			return true
		}
	}
	return false
}
