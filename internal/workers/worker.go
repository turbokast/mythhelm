// Package workers is the attempt worker (design §3, ADR 0004): a detached
// `mythhelm __worker` process that owns one native process, its pipes and
// its process group, spools the attempt's events for the supervisor, and
// runs the adapter's stop ladder until the group is confirmed gone.
package workers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	"github.com/turbokast/mythhelm/adapters/claudecode"
	"github.com/turbokast/mythhelm/adapters/fake"
	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/contain"
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
	"builtin/fake":       fake.New,
	"builtin/claudecode": claudecode.New,
}

// Launch is the admitted native launch the supervisor hands a worker on its
// stdin. It never touches the disk, because Env may carry the AC-4.7 opt-in
// credential.
type Launch struct {
	LaunchToken  string             `json:"launch_token"`
	TaskID       string             `json:"task_id"`
	AdapterID    string             `json:"adapter_id"`
	Path         string             `json:"path"`
	NativeSHA256 string             `json:"native_sha256"` // pinned at probe; re-verified before exec
	Args         []string           `json:"args"`
	Dir          string             `json:"dir"`
	Env          []string           `json:"env"`
	PromptPath   string             `json:"prompt_path"` // the task file, delivered on the native's stdin
	StopLadder   []adapter.StopStep `json:"stop_ladder"`
	// Containment, when set, runs the native inside the boundary: the worker
	// spawns `__contain` instead (design §2.2). ProxyAllow is the exact
	// host:port list its egress proxy forwards to; empty denies everything.
	Containment *contain.Policy `json:"containment,omitempty"`
	ProxyAllow  []string        `json:"proxy_allow,omitempty"`
	// UserConfigPaths maps each inventoried native-config source to the
	// absolute path the worker re-hashes before exec, and UserConfigDigests
	// carries the admitted hex digests (design §2.9). Empty digests verify
	// nothing; a digest without a mapping fails the launch.
	UserConfigPaths   map[string]string `json:"user_config_paths,omitempty"`
	UserConfigDigests map[string]string `json:"user_config_digests,omitempty"`
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
	// Nonce pins the worker to its admission. worker.json carries the raw
	// value echoed from Launch.Nonce (0600 file, the LaunchToken precedent);
	// the journaled expected record carries NonceDigest of it, never the raw
	// value (the launch_token_sha256 precedent). Empty in pre-change files,
	// which therefore never match.
	Nonce string `json:"nonce"`
}

// NonceDigest is the lowercase hex sha256 of a raw worker nonce. It lives in
// workers (not control) because MatchIdentity is the consumer and workers
// must not import control.
func NonceDigest(nonce string) string {
	sum := sha256.Sum256([]byte(nonce))
	return hex.EncodeToString(sum[:])
}

// MatchIdentity reports whether observed is the worker expected names:
// PID and start time must agree, and the nonce must agree. expected.Nonce
// is the journaled digest; observed.Nonce is the raw worker.json value;
// agreement is NonceDigest(observed.Nonce) == expected.Nonce. An empty
// nonce on either side never matches, so pre-change identity files fail
// closed.
func MatchIdentity(expected, observed Identity) bool {
	if expected.Nonce == "" || observed.Nonce == "" {
		return false
	}
	return expected.PID == observed.PID &&
		expected.StartTime.Equal(observed.StartTime) &&
		NonceDigest(observed.Nonce) == expected.Nonce
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
	if l.Containment != nil && !filepath.IsAbs(l.Containment.Workdir) {
		problems = append(problems, "containment workdir is not absolute")
	}
	for source, p := range l.UserConfigPaths {
		if !filepath.IsAbs(p) {
			problems = append(problems, fmt.Sprintf("user config %q path is not absolute", source))
		}
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

// writeFileAtomic replaces dir/name with body. On Windows the supervisor's
// identity poll can hold name open across the rename; that exact
// sharing/lock failure is retried under a bounded budget.
func writeFileAtomic(dir, name string, body []byte) error {
	tmp, err := writeTemp(dir, name, body)
	if err != nil {
		return err
	}
	dst := filepath.Join(dir, name)
	if err := renameWithRetry(func() error { return os.Rename(tmp, dst) }, retryIdentityWrite, maxIdentityWriteRetries, identityWriteRetryDelay); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func hashFile(path string) (string, error) {
	f, err := openHash(path) //nolint:gosec // The admitted launch names this path.
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	// The handle check, not the path, rejects a FIFO (or directory or
	// device) swapped in after admission: open-then-fstat on one handle
	// cannot hang and cannot be raced into hashing something else.
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file: %s", path)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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
	spoolSync   func(*os.File) error
	writeFile   func(dir, name string, body []byte) error
	pendingStop <-chan adapter.InterruptReport // owned ladder joined on panic
	observed    outcome                        // facts retained across a panic boundary
}

// outcome is what the worker learned about the native process.
type outcome struct {
	stopBy string // requester of an active stop, or ""
	// stopReason qualifies a stop: billing_route_mismatch whenever AC-4.4
	// fired, even under a user requester; a pure user stop carries no
	// reason. stopStarted records that the ladder ran or is
	// running, so a stop is never requested twice; stopRecorded records
	// that attempt.stop_requested reached the spool.
	stopReason   string
	stopStarted  bool
	stopRecorded bool
	result       *adapter.Result
	nativeErr    string // sticky poison: the first non-rate_limit class wins; rate_limit sticks only when alone
	exited       bool   // exit holds the native's observed exit
	exit         adapter.NativeExit
	report       adapter.InterruptReport
	pending      <-chan adapter.InterruptReport // an in-flight stop ladder's report
	launchFailed bool                           // no native process was created
	aborted      bool                           // the worker stopped the native because it could not record the attempt
	panicked     bool                           // raw panic values are never persisted
}

func (w *worker) run(ctx context.Context) (retErr error) {
	sp, err := createSpool(filepath.Join(w.dir, spoolFile), w.runID, w.launch.TaskID, w.attemptID)
	if err != nil {
		return err
	}
	defer func() { _ = sp.close() }()
	var sess adapter.Session
	l := &launcher{w: w}
	defer func() {
		if recover() == nil {
			return
		}
		// The panic may contain credentials or native content. Persist only
		// its classification, never its value or a raw stack trace.
		cause := errors.New("worker_panic")
		w.log.Error("worker panic; stopping owned execution")
		if w.stderr == nil {
			w.stderr = &ring{max: stderrRingBytes}
		}
		if sess != nil {
			out := w.observed
			out.panicked, out.pending = true, w.pendingStop
			retErr = w.abort(ctx, sp, sess, out, cause)
			return
		}
		out := outcome{panicked: true, report: adapter.InterruptReport{Confirmed: true}}
		if l.proc != nil {
			// Start panicked after creating the process but before returning
			// a session. This process is owned; stop it and reap it once.
			out.report.Confirmed = false
			if err := l.proc.Signal(adapter.StopKill); err == nil {
				out.report.Sent = []adapter.StopSignal{adapter.StopKill}
			} else {
				retErr = errors.Join(cause, err, w.conclude(ctx, sp, nil, out))
				return
			}
			// An escaped descendant can retain stdout indefinitely. Closing
			// our pipe lets Wait's bounded cleanup handle the owned process.
			if closer, ok := l.proc.Stdout().(io.Closer); ok {
				_ = closer.Close()
			}
			out.exit, out.exited = l.proc.Wait(), true
			out.report.Confirmed = l.proc.GroupGone()
		}
		retErr = errors.Join(cause, w.conclude(ctx, sp, nil, out))
	}()
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
	sess, err = w.start(ctx, l)
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

// start verifies the pinned native, opens the prompt and launches the
// native process once through l.
func (w *worker) start(ctx context.Context, l *launcher) (adapter.Session, error) {
	// The last hash check before exec: a binary swapped between admission
	// and spawn fails the launch instead of running unverified (I02).
	if w.launch.NativeSHA256 == "" {
		return nil, errors.New("launch carries no native digest")
	}
	digest, err := hashFile(w.launch.Path)
	if err != nil || digest != w.launch.NativeSHA256 {
		return nil, errors.New("native executable failed verification")
	}
	// The admission→exec TOCTOU close: mutable native config that changed
	// after admission fails the launch before any native process exists
	// (design §2.9, I20).
	if err := w.launch.verifyUserConfig(); err != nil {
		return nil, err
	}
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
	defer func() { w.observed = out }()
	w.log.Error("aborting the attempt", "err", cause)
	// The session reaps the native only after its observations are taken.
	go func() {
		for ob := range sess.Observations() {
			_ = ob
		}
	}()
	out.aborted = true
	if out.pending != nil {
		// Join the ladder already running; never run two at once.
		out.report, out.pending = <-out.pending, nil
		w.pendingStop = nil
	} else {
		// No ladder is running. One that already finished confirms again
		// without sending anything; a recorded requester is kept.
		if out.stopBy == "" {
			out.stopBy = "worker"
		}
		out.stopStarted = true
		out.report = sess.Interrupt(ctx)
	}
	if !out.exited {
		select {
		case exit, ok := <-sess.Done():
			if ok {
				out.exit, out.exited = exit, true
			}
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
func (w *worker) supervise(ctx context.Context, sp *spool, sess adapter.Session) (out outcome, err error) {
	defer func() { w.observed = out }()
	var (
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
			// A worker-initiated stop (AC-4.4) starts its ladder here;
			// after the native exited there is nothing left to stop, but
			// the requester and reason are still recorded.
			if out.stopBy != "" && !out.stopStarted && !out.exited {
				if err := w.requestStop(ctx, sp, sess, &out, out.stopBy, out.stopReason); err != nil {
					return out, err
				}
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
			w.pendingStop = nil
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
			if err := w.requestStop(ctx, sp, sess, &out, by, ""); err != nil {
				return out, err
			}
		}
	}
	return out, prog.flush(sp, true)
}

// requestStop records one stop request and runs the ladder for it. The first
// requester stands; later requests join the running ladder through pending.
func (w *worker) requestStop(ctx context.Context, sp *spool, sess adapter.Session, out *outcome, by, reason string) error {
	if out.stopStarted {
		return nil
	}
	out.stopStarted = true
	out.stopBy, out.stopReason = by, reason
	if err := sp.emit(evStopRequested, map[string]string{"requested_by": by}); err != nil {
		return err
	}
	if err := emitState(sp, "stop_requested", ""); err != nil {
		return err
	}
	out.stopRecorded = true
	w.log.Info("stop requested", "requested_by", by)
	pending := make(chan adapter.InterruptReport, 1)
	out.pending = pending
	w.pendingStop = pending
	go func() { pending <- sess.Interrupt(ctx) }()
	return nil
}

// observe maps one observation to its event (design §5).
func (w *worker) observe(sp *spool, ob adapter.Observation, prog *progress, out *outcome) error {
	switch o := ob.(type) {
	case adapter.SessionStarted:
		mcp := o.MCPServers
		if mcp == nil {
			mcp = []adapter.MCPServer{}
		}
		if err := sp.emit(evNativeSession, map[string]any{
			"session_id": o.SessionID, "model": o.Model, "native_version": o.NativeVersion,
			"permission_mode": o.PermissionMode, "auth_source": o.APIKeySource,
			"tool_count": o.ToolCount, "mcp": mcp, "plugin_count": o.PluginCount,
		}); err != nil {
			return err
		}
		// AC-4.4: a session that did not start on the admitted route is
		// interrupted immediately, and the run ends blocked whatever else
		// was in flight. The first requester stands, but the policy
		// reason always applies: a user stop that beat init must not
		// demote the route violation to a cancellation footnote.
		if o.APIKeySource != "none" {
			if out.stopBy == "" {
				out.stopBy = "worker"
			}
			out.stopReason = "billing_route_mismatch"
		}
		return nil
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
		// A native error is evidence, not just classification input: it is
		// journaled so a policy block (or a swallowed decoder failure)
		// survives whatever the attempt's terminal state says.
		if err := sp.emit(evNativeError, map[string]string{"class": o.Class}); err != nil {
			return err
		}
		// Poison is sticky: a later rate limit never washes an earlier
		// auth, billing, protocol or unrecognised class, while a later
		// auth class still overrides an earlier rate limit (design §6.4:
		// the auth row precedes the success row unconditionally).
		if o.Class != "rate_limit" || out.nativeErr == "" {
			out.nativeErr = o.Class
		}
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
	// A terminal line can become visible before its sync returns or panics.
	// It may already be ingested: never append a second terminal transition.
	// Panic cleanup still joins the stop ladder before reaching this point.
	if sp.terminalWritten {
		return nil
	}
	if out.stopBy != "" && !out.stopRecorded {
		// The stop was recorded only after the native exited (a
		// worker-initiated stop discovered while draining, or an abort).
		// The attempt still passes through stop_requested on its way to
		// its terminal state; stopped is unreachable from running.
		if err := sp.emit(evStopRequested, map[string]string{"requested_by": out.stopBy}); err != nil {
			return err
		}
		if err := emitState(sp, "stop_requested", ""); err != nil {
			return err
		}
		out.stopRecorded = true
	}
	if !out.stopStarted && sess != nil {
		// No ladder has run: the native exited by itself, or a stop was
		// recorded only after its exit. Anything left in its group is
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
	identities, identityErr := processIdentities(unresolved)
	if identityErr != nil {
		w.log.Error("descendant identities unavailable", "err", identityErr)
		scan = scanFailed
	}
	sent := out.report.Sent
	if sent == nil {
		sent = []adapter.StopSignal{}
	}
	if err := sp.emit(evStopped, map[string]any{
		"confirmed": out.report.Confirmed, "unresolved_pids": unresolved,
		"signals_sent": sent, "descendant_scan": scan,
		"unresolved_identities": identities,
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
	case out.panicked:
		return "interrupted", "worker_panic"
	case !out.report.Confirmed:
		return "interrupted", "stop_unconfirmed"
	case unresolved:
		return "interrupted", "unresolved_descendants"
	case out.launchFailed:
		return "failed_native", "launch_failed"
	case out.aborted:
		return "interrupted", "worker_persistence_failed"
	case out.stopBy != "":
		return "stopped", out.stopReason
	case out.nativeErr != "" && out.nativeErr != "rate_limit":
		// Auth and billing classes, protocol violations and unrecognised
		// errors fail the attempt whatever the result frame says.
		return "failed_native", out.nativeErr
	case res != nil && !res.IsError && res.Subtype == "success" && exit.Code == 0 && exit.Signal == "":
		// A successful result after a mere rate limit is still a success
		// (design §6.4: the success row wins over the rate-limit row).
		return "succeeded_native", ""
	case out.nativeErr != "":
		// Only rate_limit reaches here.
		return "failed_native", "provider_limit"
	case res != nil && (res.IsError || res.Subtype != "success"):
		return "failed_native", res.Subtype
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
		"error_count": nil, "startup_failure_reason": nil,
	}
	if r := out.result; r != nil {
		p["subtype"], p["is_error"], p["num_turns"], p["duration_ms"] = r.Subtype, r.IsError, r.NumTurns, r.DurationMS
		p["stop_reason"], p["retail_equivalent_estimate_usd"] = orNull(r.StopReason), orNull(r.CostUSD)
		p["error_count"], p["startup_failure_reason"] = r.ErrorCount, orNull(r.StartupFailureReason)
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
	cmd, h, err := l.w.launch.command(spec)
	if err != nil {
		return nil, err
	}
	cmd.Stderr = l.w.stderr
	cmd.WaitDelay = waitDelay
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		h.stop()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		h.stop()
		return nil, fmt.Errorf("starting the native process: %w", err)
	}
	h.started()
	p := &ownedProc{cmd: cmd, stdout: stdout, pgid: nativeGroup(cmd.Process.Pid), release: h.stop}
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
	// release frees what a contained spawn holds beyond the process: its
	// egress proxy and prompt copier. Nil for an uncontained native.
	release func()
}

func (p *ownedProc) Stdout() io.Reader { return p.stdout }

func (p *ownedProc) Wait() adapter.NativeExit {
	err := p.cmd.Wait()
	p.waited.Store(true)
	if p.release != nil {
		p.release()
	}
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
