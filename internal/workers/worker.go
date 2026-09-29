// Package workers is the attempt worker (design §3, ADR 0004): a detached
// `mythhelm __worker` process that owns one native process, its pipes and
// its process group, spools the attempt's events for the supervisor, and
// runs the adapter's stop ladder until the group is confirmed gone.
package workers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/turbokast/mythhelm/adapters/fake"
	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/security"
)

// Command is the hidden mythhelm subcommand that runs a worker.
const Command = "__worker"

const (
	stopPollInterval  = 250 * time.Millisecond
	heartbeatInterval = 2 * time.Second
	progressInterval  = time.Second // at most one attempt.progress per second
	maxLaunchBytes    = 1 << 20
	stderrRingBytes   = 256 << 10 // NFR-1
	// waitDelay bounds how long Wait waits for the native's stderr to close
	// after it exits, in case a descendant still holds it.
	waitDelay       = 5 * time.Second
	identityVersion = 1
	attemptEnvVar   = "MYTHHELM_ATTEMPT_ID"
)

// Files in an attempt directory (design §5).
const (
	identityFile  = "worker.json"
	spoolFile     = "spool.jsonl"
	heartbeatFile = "heartbeat"
	stopFile      = "stop.request"
	stderrFile    = "stderr.log"
	workerLogFile = "worker.log"
)

// ErrNoProcess reports that no process has the given PID.
var ErrNoProcess = errors.New("no such process")

// adapters are the adapters a worker can start, by descriptor ID.
var adapters = map[string]func() adapter.Adapter{
	"builtin/fake": fake.New,
}

// Launch is the admitted native launch the supervisor hands a worker on its
// stdin. It never touches the disk, because Env may carry the AC-4.7 opt-in
// credential.
type Launch struct {
	LaunchToken string             `json:"launch_token"`
	TaskID      string             `json:"task_id"`
	AdapterID   string             `json:"adapter_id"`
	Path        string             `json:"path"`
	Args        []string           `json:"args"`
	Dir         string             `json:"dir"`
	Env         []string           `json:"env"`
	PromptPath  string             `json:"prompt_path"` // the task file, delivered on the native's stdin
	StopLadder  []adapter.StopStep `json:"stop_ladder"`
}

// Identity is worker.json: what the supervisor checks before it accepts a
// worker (§7.3). NativeLaunchIntent is written before the native process is
// created and NativePID after, so a crash between the two is visible.
type Identity struct {
	SchemaVersion      int        `json:"schema_version"`
	RunID              string     `json:"run_id"`
	AttemptID          string     `json:"attempt_id"`
	PID                int        `json:"pid"`
	StartTime          time.Time  `json:"start_time"`
	LaunchToken        string     `json:"launch_token"`
	NativeLaunchIntent *time.Time `json:"native_launch_intent"`
	NativePID          *int       `json:"native_pid"`
	NativePGID         *int       `json:"native_pgid"` // null where the platform has no owned process group
}

// AttemptDir is the attempt's directory in the state directory.
func AttemptDir(stateDir, runID, attemptID string) string {
	return filepath.Join(stateDir, "runs", runID, "attempts", attemptID)
}

// Spawn starts `exe __worker` for an attempt, detached from the caller's
// session (Unix) or console (Windows), hands it l on its stdin and returns
// without waiting for it. The worker's diagnostics go to worker.log.
func Spawn(exe, stateDir, runID, attemptID string, l Launch) (*os.Process, error) {
	dir := AttemptDir(stateDir, runID, attemptID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating the attempt directory: %w", err)
	}
	body, err := json.Marshal(l)
	if err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(filepath.Join(dir, workerLogFile), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600) //nolint:gosec // G304: inside the attempt directory
	if err != nil {
		return nil, err
	}
	defer func() { _ = logFile.Close() }()
	null, err := os.Open(os.DevNull)
	if err != nil {
		return nil, err
	}
	defer func() { _ = null.Close() }()
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer func() { _ = w.Close() }()
	// G702: exe is mythhelm itself and the argv is fixed; no shell is involved.
	proc, err := os.StartProcess(exe, []string{exe, Command, "--state", stateDir, "--run", runID, "--attempt", attemptID}, &os.ProcAttr{ //nolint:gosec // see above
		Dir:   dir,
		Env:   os.Environ(),
		Files: []*os.File{r, null, logFile},
		Sys:   detachedAttr(),
	})
	_ = r.Close()
	if err != nil {
		return nil, fmt.Errorf("starting the worker: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		_ = proc.Kill()
		return nil, fmt.Errorf("handing the launch to the worker: %w", err)
	}
	return proc, nil
}

// Main runs `mythhelm __worker --state <dir> --run <id> --attempt <id>` and
// returns its exit code: 0 once the attempt's terminal state is spooled, 2
// for an invalid invocation, 1 if the attempt could not be concluded.
func Main(args []string) int {
	log := slog.New(security.NewRedactingHandler(slog.NewTextHandler(os.Stderr, nil)))
	fl := flag.NewFlagSet(Command, flag.ContinueOnError)
	fl.SetOutput(os.Stderr)
	state := fl.String("state", "", "state directory")
	runID := fl.String("run", "", "run ID")
	attemptID := fl.String("attempt", "", "attempt ID")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if !filepath.IsAbs(*state) || !validID(*runID) || !validID(*attemptID) || fl.NArg() > 0 {
		log.Error("invalid worker invocation", "args", args)
		return 2
	}
	l, err := readLaunch(os.Stdin)
	if err != nil {
		log.Error("invalid launch", "err", err)
		return 2
	}
	log = log.With("run", *runID, "attempt", *attemptID)
	w := &worker{dir: AttemptDir(*state, *runID, *attemptID), runID: *runID, attemptID: *attemptID, launch: l, log: log}
	if err := w.run(context.Background()); err != nil {
		log.Error("worker failed", "err", err)
		return 1
	}
	return 0
}

// validID accepts the IDs MYTHHELM generates: letters, digits and '_'. An ID
// is a path component, so nothing else is allowed.
func validID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		switch {
		case r == '_', r >= '0' && r <= '9', r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
		default:
			return false
		}
	}
	return true
}

func readLaunch(r io.Reader) (Launch, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxLaunchBytes+1))
	if err != nil {
		return Launch{}, err
	}
	if len(b) > maxLaunchBytes {
		return Launch{}, fmt.Errorf("launch exceeds %d bytes", maxLaunchBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var l Launch
	if err := dec.Decode(&l); err != nil {
		return Launch{}, fmt.Errorf("decoding the launch: %w", err)
	}
	var problems []string
	if l.LaunchToken == "" {
		problems = append(problems, "no launch token")
	}
	if _, ok := adapters[l.AdapterID]; !ok {
		problems = append(problems, fmt.Sprintf("unknown adapter %q", l.AdapterID))
	}
	for name, p := range map[string]string{"path": l.Path, "dir": l.Dir, "prompt_path": l.PromptPath} {
		if !filepath.IsAbs(p) {
			problems = append(problems, name+" is not absolute")
		}
	}
	if len(l.StopLadder) == 0 {
		problems = append(problems, "empty stop ladder")
	}
	for _, step := range l.StopLadder {
		if step.Grace <= 0 {
			problems = append(problems, fmt.Sprintf("stop step %s has no grace period", step.Signal))
		}
	}
	if len(problems) > 0 {
		return Launch{}, fmt.Errorf("invalid launch: %s", strings.Join(problems, "; "))
	}
	return l, nil
}

// ReadIdentity reads worker.json from an attempt directory.
func ReadIdentity(dir string) (Identity, error) {
	b, err := os.ReadFile(filepath.Join(dir, identityFile)) //nolint:gosec // G304: inside the attempt directory
	if err != nil {
		return Identity{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var id Identity
	if err := dec.Decode(&id); err != nil {
		return Identity{}, fmt.Errorf("decoding %s: %w", identityFile, err)
	}
	if id.SchemaVersion != identityVersion {
		return Identity{}, fmt.Errorf("%s has schema_version %d, want %d", identityFile, id.SchemaVersion, identityVersion)
	}
	return id, nil
}

type stopRequest struct {
	RequestID   string    `json:"request_id"`
	RequestedAt time.Time `json:"requested_at"`
}

// RequestStop asks the attempt's worker to stop its native process. The
// worker records requestID as requested_by. The first request stands: a
// later one returns nil without replacing it. A request is not a confirmed
// stop; attempt.stopped is (I06).
func RequestStop(dir, requestID string) error {
	if requestID == "" {
		return errors.New("stop request needs an ID")
	}
	body, err := json.Marshal(stopRequest{RequestID: requestID, RequestedAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	tmp, err := writeTemp(dir, stopFile, body)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp) }()
	// A hard link creates the request whole, and only if none exists.
	if err := os.Link(tmp, filepath.Join(dir, stopFile)); err != nil && !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("requesting a stop: %w", err)
	}
	return nil
}

// readStopRequest reports a pending stop request. An unreadable request
// still stops the attempt; only its requester is unknown.
func readStopRequest(dir string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(dir, stopFile)) //nolint:gosec // G304: inside the attempt directory
	if err != nil {
		return "", false
	}
	var req stopRequest
	if json.Unmarshal(b, &req) != nil || req.RequestID == "" {
		return "unknown", true
	}
	return req.RequestID, true
}

// writeTemp writes body to a new synced 0600 file next to name.
func writeTemp(dir, name string, body []byte) (string, error) {
	f, err := os.CreateTemp(dir, "."+name+".*")
	if err != nil {
		return "", err
	}
	_, err = f.Write(body)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// writeFileAtomic replaces dir/name with body.
func writeFileAtomic(dir, name string, body []byte) error {
	tmp, err := writeTemp(dir, name, body)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func attemptMarker(attemptID string) string {
	return attemptEnvVar + "=" + attemptID
}

type worker struct {
	dir       string
	runID     string
	attemptID string
	launch    Launch
	log       *slog.Logger
	id        Identity
	beats     int64
	stderr    *ring
	// Test seams; nil means the real file operations.
	spoolSync func(*os.File) error
	writeFile func(dir, name string, body []byte) error
}

// outcome is what the worker learned about the native process.
type outcome struct {
	stopBy       string // requester of an active stop, or ""
	result       *adapter.Result
	nativeErr    string // class of the last NativeError observation
	exited       bool   // exit holds the native's observed exit
	exit         adapter.NativeExit
	report       adapter.InterruptReport
	pending      <-chan adapter.InterruptReport // an in-flight stop ladder's report
	launchFailed bool                           // no native process was created
	aborted      bool                           // the worker stopped the native because it could not record the attempt
}

func (w *worker) run(ctx context.Context) error {
	sp, err := createSpool(filepath.Join(w.dir, spoolFile), w.runID, w.launch.TaskID, w.attemptID)
	if err != nil {
		return err
	}
	defer func() { _ = sp.close() }()
	if w.spoolSync != nil {
		sp.sync = w.spoolSync
	}
	start, err := ProcessStartTime(os.Getpid())
	if err != nil {
		return fmt.Errorf("reading the worker's start time: %w", err)
	}
	w.id = Identity{SchemaVersion: identityVersion, RunID: w.runID, AttemptID: w.attemptID, PID: os.Getpid(), StartTime: start, LaunchToken: w.launch.LaunchToken}
	if err := w.writeIdentity(); err != nil {
		return err
	}
	if err := emitState(sp, "launching", ""); err != nil {
		return err
	}

	w.stderr = &ring{max: stderrRingBytes}
	l := &launcher{w: w}
	sess, err := w.start(ctx, l)
	if err != nil {
		w.log.Error("native launch failed", "err", err)
		// No native process was created, so there is nothing to stop.
		return w.conclude(ctx, sp, nil, outcome{launchFailed: true, report: adapter.InterruptReport{Confirmed: true}})
	}
	if l.identityErr != nil {
		return w.abort(ctx, sp, sess, outcome{}, l.identityErr)
	}
	if err := sp.emit(evLaunched, map[string]any{
		"worker_pid": w.id.PID, "worker_start_time": w.id.StartTime,
		"native_pid": w.id.NativePID, "native_pgid": w.id.NativePGID,
	}); err != nil {
		return w.abort(ctx, sp, sess, outcome{}, err)
	}
	if err := emitState(sp, "running", ""); err != nil {
		return w.abort(ctx, sp, sess, outcome{}, err)
	}

	out, err := w.supervise(ctx, sp, sess)
	if err != nil {
		return w.abort(ctx, sp, sess, out, err)
	}
	return w.conclude(ctx, sp, sess, out)
}

// start opens the prompt and launches the native process once through l.
func (w *worker) start(ctx context.Context, l *launcher) (adapter.Session, error) {
	prompt, err := os.Open(w.launch.PromptPath)
	if err != nil {
		return nil, fmt.Errorf("opening the prompt: %w", err)
	}
	defer func() { _ = prompt.Close() }()
	lp := adapter.LaunchProposal{
		Spec:       adapter.ProcSpec{Path: w.launch.Path, Args: w.launch.Args, Dir: w.launch.Dir, Env: w.launch.Env, Stdin: prompt},
		StopLadder: w.launch.StopLadder,
	}
	return adapters[w.launch.AdapterID]().Start(ctx, lp, l)
}

// abort stops the native process when the worker could not record what it
// does, since a native that cannot be observed must not keep running. It
// then records the attempt's end as far as the spool still accepts writes.
func (w *worker) abort(ctx context.Context, sp *spool, sess adapter.Session, out outcome, cause error) error {
	w.log.Error("aborting the attempt", "err", cause)
	// The session reaps the native only after its observations are taken.
	go func() {
		for range sess.Observations() {
		}
	}()
	out.aborted = true
	if out.pending != nil {
		// Join the ladder already running; never run two at once.
		out.report, out.pending = <-out.pending, nil
	} else {
		// No ladder is running. One that already finished confirms again
		// without sending anything; a recorded requester is kept.
		if out.stopBy == "" {
			out.stopBy = "worker"
		}
		out.report = sess.Interrupt(ctx)
	}
	if !out.exited {
		select {
		case out.exit = <-sess.Done():
			out.exited = true
		case <-time.After(waitDelay):
		}
	}
	if err := w.conclude(ctx, sp, sess, out); err != nil {
		w.log.Error("recording the aborted attempt", "err", err)
	}
	return cause
}

// supervise spools observations, heartbeats and polls for a stop request
// until the native process has exited and any stop ladder has finished.
func (w *worker) supervise(ctx context.Context, sp *spool, sess adapter.Session) (outcome, error) {
	var (
		out  outcome
		prog progress
		obs  = sess.Observations()
		done = sess.Done()
	)
	poll := time.NewTicker(stopPollInterval)
	defer poll.Stop()
	beat := time.NewTicker(heartbeatInterval)
	defer beat.Stop()
	w.heartbeat()

	for !out.exited || out.pending != nil {
		select {
		case ob, ok := <-obs:
			if !ok {
				obs = nil
				continue
			}
			if err := w.observe(sp, ob, &prog, &out); err != nil {
				return out, err
			}
		case exit := <-done:
			out.exit, out.exited, done = exit, true, nil
			// Observations is closed before Done fires; drain what is queued
			// (a nil channel means it was already drained).
			if obs != nil {
				for ob := range obs {
					if err := w.observe(sp, ob, &prog, &out); err != nil {
						return out, err
					}
				}
			}
			obs = nil
		case rep := <-out.pending:
			out.report, out.pending = rep, nil
		case <-beat.C:
			w.heartbeat()
		case <-poll.C:
			if err := prog.flush(sp, false); err != nil {
				return out, err
			}
			if out.exited || out.stopBy != "" {
				continue
			}
			by, ok := readStopRequest(w.dir)
			if !ok {
				continue
			}
			out.stopBy = by
			if err := sp.emit(evStopRequested, map[string]string{"requested_by": by}); err != nil {
				return out, err
			}
			if err := emitState(sp, "stop_requested", ""); err != nil {
				return out, err
			}
			w.log.Info("stop requested", "requested_by", by)
			pending := make(chan adapter.InterruptReport, 1)
			out.pending = pending
			go func() { pending <- sess.Interrupt(ctx) }()
		}
	}
	return out, prog.flush(sp, true)
}

// observe maps one observation to its event (design §5).
func (w *worker) observe(sp *spool, ob adapter.Observation, prog *progress, out *outcome) error {
	switch o := ob.(type) {
	case adapter.SessionStarted:
		mcp := o.MCPServers
		if mcp == nil {
			mcp = []adapter.MCPServer{}
		}
		return sp.emit(evNativeSession, map[string]any{
			"session_id": o.SessionID, "model": o.Model, "native_version": o.NativeVersion,
			"permission_mode": o.PermissionMode, "auth_source": o.APIKeySource,
			"tool_count": o.ToolCount, "mcp": mcp, "plugin_count": o.PluginCount,
		})
	case adapter.Progress:
		prog.add(o)
		return prog.flush(sp, false)
	case adapter.Retry:
		prog.retries++
		prog.dirty = true
		return prog.flush(sp, false)
	case adapter.PermissionDenied:
		return sp.emit(evPermissionDenied, map[string]string{"tool_name": o.ToolName})
	case adapter.NativeError:
		out.nativeErr = o.Class
	case adapter.Result:
		out.result = &o
	case adapter.ProtocolCounters:
		return sp.emit(evProtocolCounters, map[string]any{
			"malformed": o.Malformed, "oversized": o.Oversized, "invalid_utf8": o.InvalidUTF8,
			"depth_exceeded": o.DepthExceeded, "unknown_types": o.UnknownTypes, "progress_dropped": o.ProgressDropped,
		})
	}
	return nil
}

// conclude confirms the process group is gone, reports known descendants
// that escaped it, and spools the attempt's terminal state.
func (w *worker) conclude(ctx context.Context, sp *spool, sess adapter.Session, out outcome) error {
	if out.stopBy == "" && sess != nil {
		// The native exited by itself; anything left in its group is
		// stopped before anything else, even if the spool then fails.
		// Nothing is sent when the group is already gone.
		out.report = sess.Interrupt(ctx)
	}
	w.saveStderr()
	if out.exit.Err != nil {
		w.log.Warn("native exit not fully observed", "err", out.exit.Err)
	}
	if err := sp.emit(evNativeResult, nativeResult(out)); err != nil {
		return err
	}
	unresolved, scan := w.unresolvedDescendants()
	sent := out.report.Sent
	if sent == nil {
		sent = []adapter.StopSignal{}
	}
	if err := sp.emit(evStopped, map[string]any{
		"confirmed": out.report.Confirmed, "unresolved_pids": unresolved,
		"signals_sent": sent, "descendant_scan": scan,
	}); err != nil {
		return err
	}
	state, reason := classify(out, len(unresolved) > 0 || scan == scanFailed)
	w.log.Info("attempt concluded", "state", state, "reason", reason)
	return emitState(sp, state, reason)
}

const scanFailed = "failed"

// unresolvedDescendants lists processes outside the (gone) group that still
// carry the attempt's marker, and names how they were found.
func (w *worker) unresolvedDescendants() ([]int, string) {
	if descendantScan == "unavailable" {
		return []int{}, descendantScan
	}
	pids, err := markedPIDs(attemptMarker(w.attemptID))
	if err != nil {
		w.log.Error("descendant scan failed; ownership is unresolved", "err", err)
		return []int{}, scanFailed
	}
	if pids == nil {
		pids = []int{}
	}
	if len(pids) > 0 {
		w.log.Warn("descendants escaped the process group", "pids", pids)
	}
	return pids, descendantScan
}

// classify maps the native outcome to the attempt's terminal state (design
// §6.4 and AC-5.7). Ownership comes first: a group not confirmed gone, or a
// known escaped descendant, leaves the attempt interrupted, whatever the
// native reported.
func classify(out outcome, unresolved bool) (state, reason string) {
	res, exit := out.result, out.exit
	switch {
	case !out.report.Confirmed:
		return "interrupted", "stop_unconfirmed"
	case unresolved:
		return "interrupted", "unresolved_descendants"
	case out.launchFailed:
		return "failed_native", "launch_failed"
	case out.aborted:
		return "interrupted", "worker_persistence_failed"
	case out.stopBy != "":
		return "stopped", ""
	case out.nativeErr != "":
		return "failed_native", out.nativeErr
	case res != nil && (res.IsError || res.Subtype != "success"):
		return "failed_native", res.Subtype
	case res != nil && exit.Code == 0 && exit.Signal == "":
		return "succeeded_native", ""
	case res == nil && exit.Code == 0 && exit.Signal == "":
		return "failed_native", "result_unobserved"
	case exit.Signal != "":
		return "failed_native", "native_signal_" + strings.ReplaceAll(exit.Signal, " ", "_")
	default:
		return "failed_native", fmt.Sprintf("native_exit_%d", exit.Code)
	}
}

// nativeResult is attempt.native_result. Result fields are explicit nulls
// when no result frame was observed, and unreported values stay null, never
// zero (I09).
func nativeResult(out outcome) map[string]any {
	var exitCode, signal any
	switch {
	case !out.exited:
	case out.exit.Signal != "":
		signal = out.exit.Signal
	default:
		exitCode = out.exit.Code
	}
	p := map[string]any{
		"exit_code": exitCode, "signal": signal, "stop_requested": out.stopBy != "",
		"result_observed": out.result != nil,
		"subtype":         nil, "is_error": nil, "num_turns": nil, "duration_ms": nil, "stop_reason": nil,
		"usage_native_reported": nil, "retail_equivalent_estimate_usd": nil, "denials": nil,
	}
	if r := out.result; r != nil {
		p["subtype"], p["is_error"], p["num_turns"], p["duration_ms"] = r.Subtype, r.IsError, r.NumTurns, r.DurationMS
		p["stop_reason"], p["retail_equivalent_estimate_usd"] = orNull(r.StopReason), orNull(r.CostUSD)
		if r.Tokens != nil {
			p["usage_native_reported"] = r.Tokens
		}
		denials := r.PermissionDenials
		if denials == nil {
			denials = []string{}
		}
		p["denials"] = denials
	}
	return p
}

func orNull(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func emitState(sp *spool, state, reason string) error {
	return sp.emit(evStateChanged, map[string]any{"state": state, "reason": orNull(reason)})
}

// progress coalesces Progress and Retry observations into at most one
// attempt.progress per second.
type progress struct {
	turns    int
	tools    map[string]int
	retries  int
	dirty    bool
	lastEmit time.Time
}

func (p *progress) add(o adapter.Progress) {
	p.turns = max(p.turns, o.Turn)
	if o.Tool != "" {
		if p.tools == nil {
			p.tools = map[string]int{}
		}
		p.tools[o.Tool]++
	}
	p.dirty = true
}

// flush emits pending progress when a second has passed since the last, or
// always when final.
func (p *progress) flush(sp *spool, final bool) error {
	if !p.dirty || !final && time.Since(p.lastEmit) < progressInterval {
		return nil
	}
	tools := p.tools
	if tools == nil {
		tools = map[string]int{}
	}
	p.dirty, p.lastEmit = false, time.Now()
	return sp.emit(evProgress, map[string]any{"assistant_turns": p.turns, "tool_uses": tools, "retries": p.retries})
}

func (w *worker) writeIdentity() error {
	body, err := json.MarshalIndent(w.id, "", "  ")
	if err != nil {
		return err
	}
	write := w.writeFile
	if write == nil {
		write = writeFileAtomic
	}
	if err := write(w.dir, identityFile, append(body, '\n')); err != nil {
		return fmt.Errorf("writing %s: %w", identityFile, err)
	}
	return nil
}

// heartbeat says the worker is alive; it says nothing about the model's
// progress (§7.3).
func (w *worker) heartbeat() {
	w.beats++
	body := fmt.Sprintf("{\"beat\":%d,\"at\":%q}\n", w.beats, time.Now().UTC().Format(time.RFC3339Nano))
	if err := os.WriteFile(filepath.Join(w.dir, heartbeatFile), []byte(body), 0o600); err != nil {
		w.log.Warn("writing the heartbeat", "err", err)
	}
}

// saveStderr writes the redacted tail of the native's stderr.
func (w *worker) saveStderr() {
	body, dropped := w.stderr.contents()
	text := security.Redact(string(body))
	if dropped > 0 {
		text = fmt.Sprintf("[%d earlier bytes dropped]\n%s", dropped, text)
	}
	if err := os.WriteFile(filepath.Join(w.dir, stderrFile), []byte(text), 0o600); err != nil {
		w.log.Warn("writing the native stderr", "err", err)
	}
}

// launcher is the worker's adapter.Launcher: the only way a native process
// is created, at most once per worker.
type launcher struct {
	w    *worker
	proc *ownedProc
	// identityErr is set when the native started but its PID could not be
	// recorded. The process is still returned, so the session reaps it and
	// the worker stops it through the ladder.
	identityErr error
}

func (l *launcher) Launch(_ context.Context, spec adapter.ProcSpec) (adapter.OwnedProc, error) {
	if l.proc != nil {
		return nil, errors.New("the native process was already launched; a worker launches once (I12)")
	}
	if !filepath.IsAbs(spec.Path) {
		return nil, fmt.Errorf("native path %q is not absolute", spec.Path)
	}
	intent := time.Now().UTC()
	l.w.id.NativeLaunchIntent = &intent
	if err := l.w.writeIdentity(); err != nil {
		return nil, err
	}
	cmd := exec.Command(spec.Path, spec.Args...) //nolint:gosec // G204: the admitted argv, no shell
	// A nil Env would inherit the worker's environment; the child gets
	// exactly the admitted one.
	cmd.Dir, cmd.Env, cmd.Stdin, cmd.Stderr = spec.Dir, append([]string{}, spec.Env...), spec.Stdin, l.w.stderr
	cmd.SysProcAttr = nativeAttr()
	cmd.WaitDelay = waitDelay
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting the native process: %w", err)
	}
	p := &ownedProc{cmd: cmd, stdout: stdout, pgid: nativeGroup(cmd.Process.Pid)}
	l.proc = p
	pid := cmd.Process.Pid
	l.w.id.NativePID, l.w.id.NativePGID = &pid, p.pgid
	// A native the supervisor cannot identify must not run; the worker
	// aborts it once the session owns it.
	l.identityErr = l.w.writeIdentity()
	return p, nil
}

// ownedProc is a native process the worker owns. Signal and GroupGone are
// per platform.
type ownedProc struct {
	cmd    *exec.Cmd
	stdout io.Reader
	pgid   *int
	waited atomic.Bool
}

func (p *ownedProc) Stdout() io.Reader { return p.stdout }

func (p *ownedProc) Wait() adapter.NativeExit {
	err := p.cmd.Wait()
	p.waited.Store(true)
	exit := adapter.NativeExit{Code: -1}
	if st := p.cmd.ProcessState; st != nil {
		exit.Code = st.ExitCode()
		if sig, ok := strings.CutPrefix(st.String(), "signal: "); ok {
			exit.Signal = sig
		}
	}
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		exit.Err = err
	}
	return exit
}

// ring keeps the last max bytes written to it.
type ring struct {
	mu      sync.Mutex
	buf     []byte
	max     int
	dropped int64
}

func (r *ring) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, p...)
	if over := len(r.buf) - r.max; over > 0 {
		r.dropped += int64(over)
		r.buf = r.buf[:copy(r.buf, r.buf[over:])]
	}
	return len(p), nil
}

func (r *ring) contents() ([]byte, int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return bytes.Clone(r.buf), r.dropped
}
