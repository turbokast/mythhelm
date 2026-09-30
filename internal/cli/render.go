package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
)

// runResult is the end of a run command: the run.result event (design §5).
type runResult struct {
	RunID         string
	State         string
	Reason        string
	AttemptReason string
	Code          ExitCode
	Category      string
}

// renderer shows a run as it happens. Write errors are kept, and the first
// one is returned by err.
type renderer interface {
	event(journal.Event)
	notice(string)
	result(runResult)
	err() error
}

func newRenderer(format string, stdio Stdio) renderer {
	if format == "jsonl" {
		return &jsonlRenderer{out: json.NewEncoder(stdio.Out), diag: stdio.Err}
	}
	return &plainRenderer{out: stdio.Out}
}

// writeErr keeps the first write error.
type writeErr struct{ first error }

func (w *writeErr) keep(err error) {
	if w.first == nil {
		w.first = err
	}
}

func (w *writeErr) err() error { return w.first }

// jsonlRenderer writes only envelope events to stdout, one per line, ending
// with run.result; diagnostics go to stderr (AC-1.3).
type jsonlRenderer struct {
	writeErr
	out  *json.Encoder
	diag io.Writer
}

func (r *jsonlRenderer) event(ev journal.Event) { r.keep(r.out.Encode(ev)) }

func (r *jsonlRenderer) notice(msg string) {
	_, err := fmt.Fprintf(r.diag, "mythhelm run: %s\n", msg)
	r.keep(err)
}

// result writes run.result. It is not journaled: it comes from its own
// one-event producer, and its run_sequence is 0.
func (r *jsonlRenderer) result(res runResult) {
	payload, err := json.Marshal(struct {
		State         *string `json:"state"`
		Reason        *string `json:"reason"`
		AttemptReason *string `json:"attempt_reason"`
		ExitCode      int     `json:"exit_code"`
		ErrorCategory *string `json:"error_category"`
	}{orNull(res.State), orNull(res.Reason), orNull(res.AttemptReason), int(res.Code), orNull(res.Category)})
	if err != nil {
		r.keep(err)
		return
	}
	r.event(journal.Event{
		SchemaVersion:    journal.EnvelopeVersion,
		EventID:          ids.New("evt"),
		RunID:            res.RunID,
		ProducerID:       ids.New("cli"),
		ProducerSequence: 1,
		Generation:       1,
		ObservedAt:       time.Now().UTC(),
		Type:             "run.result",
		Payload:          payload,
	})
}

func orNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// plainRenderer writes linear text with no cursor movement or colour
// (AC-1.5). Every value from an event is terminal-sanitised.
type plainRenderer struct {
	writeErr
	out io.Writer
}

func (r *plainRenderer) line(s string) {
	_, err := io.WriteString(r.out, s+"\n")
	r.keep(err)
}

func (r *plainRenderer) event(ev journal.Event) {
	if s := plainEvent(ev); s != "" {
		r.line(s)
	}
}

func (r *plainRenderer) notice(msg string) { r.line(cell(msg)) }

func (r *plainRenderer) result(res runResult) {
	state := res.State
	if state == "" {
		state = "not admitted"
	}
	if res.Reason != "" {
		state += " (" + res.Reason + ")"
	}
	r.line(fmt.Sprintf("result: %s; exit %d %s", cell(state), res.Code, cell(res.Category)))
}

// payload is a decoded event payload. Journaled payloads are JSON objects.
type payload map[string]any

// get returns the value at the key path, or nil.
func (p payload) get(keys ...string) any {
	var v any = map[string]any(p)
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

// text formats the value at the key path for a terminal; an absent value is
// "unknown", never zero (I09).
func (p payload) text(keys ...string) string {
	switch v := p.get(keys...).(type) {
	case nil:
		return "unknown"
	case string:
		return cell(v)
	case float64:
		return fmt.Sprint(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "unknown"
		}
		return cell(string(b))
	}
}

func (p payload) state() string {
	s := p.text("state")
	if p.get("reason") != nil {
		s += " (" + p.text("reason") + ")"
	}
	return s
}

func plainEvent(ev journal.Event) string {
	var p payload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return cell(ev.Type)
	}
	switch ev.Type {
	case "run.created":
		return "run " + cell(ev.RunID) + ": created"
	case "run.state_changed":
		return "run: " + p.state()
	case "admission.decided":
		return admissionLines(p)
	case "workspace.snapshot_created":
		return fmt.Sprintf("snapshot: %s cloned to %s", p.text("base_rev"), p.text("clone_path"))
	case "attempt.launch_intent_recorded":
		return "attempt " + cell(ev.AttemptID) + ": launch intent recorded"
	case "attempt.state_changed":
		return "attempt: " + p.state()
	case "attempt.launched":
		return fmt.Sprintf("attempt: worker pid %s, native pid %s", p.text("worker_pid"), p.text("native_pid"))
	case "attempt.native_session":
		return fmt.Sprintf("native session %s, model %s", p.text("session_id"), p.text("model"))
	case "attempt.progress":
		return fmt.Sprintf("progress: %s assistant turns, tool uses %s (native-reported)", p.text("assistant_turns"), p.text("tool_uses"))
	case "attempt.permission_denied":
		return "native permission denied: " + p.text("tool_name")
	case "attempt.native_result":
		exit := "exit code " + p.text("exit_code")
		if p.get("signal") != nil {
			exit = "signal " + p.text("signal")
		}
		result := "no result frame observed"
		if p.get("result_observed") == true {
			result = "result " + p.text("subtype")
		}
		return fmt.Sprintf("native exit: %s; %s", exit, result)
	case "attempt.stop_requested":
		return "stop requested by " + p.text("requested_by")
	case "attempt.stopped":
		confirmed := "process group gone"
		if p.get("confirmed") != true {
			confirmed = "process group NOT confirmed gone"
		}
		return fmt.Sprintf("attempt: %s; unresolved descendants %s", confirmed, p.text("unresolved_pids"))
	case "attempt.protocol_counters":
		return fmt.Sprintf("protocol counters: malformed %s, oversized %s, invalid utf-8 %s, depth exceeded %s",
			p.text("malformed"), p.text("oversized"), p.text("invalid_utf8"), p.text("depth_exceeded"))
	case "verification.started":
		return "verification: started"
	case "check.completed":
		return fmt.Sprintf("check %s: %s", p.text("name"), p.text("status"))
	case "verification.completed":
		if p.get("result") == "NOT RUN" {
			return "verification: NOT RUN (waived by --no-checks)"
		}
		return "verification: " + p.text("result")
	}
	return cell(ev.Type)
}

func admissionLines(p payload) string {
	lines := []string{
		fmt.Sprintf("admission decided: adapter %s %s (%s)", p.text("adapter", "id"), p.text("adapter", "version"), p.text("adapter", "surface")),
	}
	if p.get("adapter", "harness") == "fake" {
		lines = append(lines, "  SCRIPTED: fake adapter, no real agent, no inference, no network")
	}
	branch := p.text("snapshot", "branch")
	lines = append(lines,
		fmt.Sprintf("  snapshot: %s (branch %s, selected by %s)", p.text("snapshot", "base_rev"), branch, p.text("snapshot", "selected_by")),
		fmt.Sprintf("  execution profile: %s, %s", p.text("execution_profile", "name"), p.text("execution_profile", "disclosure")),
		fmt.Sprintf("  billing: %s (qualified: %s)", p.text("billing", "mode"), p.text("billing", "qualified")),
	)
	return strings.Join(lines, "\n")
}
