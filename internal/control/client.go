package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/statedir"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// SupervisorCommand is the hidden subcommand that runs the supervisor.
const SupervisorCommand = "__supervisor"

const (
	spawnWait = 10 * time.Second
	spawnPoll = 25 * time.Millisecond
)

// ErrSpawnUnsupported reports a platform without a control transport yet.
var ErrSpawnUnsupported = errors.New("control: no control transport on this platform")

// Connect dials the supervisor, starting one first if none is running.
// Concurrent callers may each start one: the extra processes lose the
// instance lock and exit, and every caller ends up a client of the winner.
func Connect(ctx context.Context) (Conn, error) {
	t, err := DefaultTransport()
	if err != nil {
		return nil, err
	}
	endpoint, err := EndpointPath()
	if err != nil {
		return nil, err
	}
	dir, err := statedir.Resolve()
	if err != nil {
		return nil, err
	}
	conn, err := dialOrSpawn(ctx, t, endpoint, spawn)
	if err != nil {
		return nil, err
	}
	if err := requireRoot(ctx, conn, canonicalRoot(dir)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// requireRoot asks the connected supervisor which state root it serves and
// fails closed unless that is want. The socket is per user, not per root, so
// a supervisor of another MYTHHELM_HOME can answer here, whether it started
// first or won a startup race: attaching would act on its ledger, a second
// authority (AC-5.4).
func requireRoot(ctx context.Context, conn Conn, want string) error {
	res, err := Call(ctx, conn, Intent{OperationID: ids.New("op"), Method: "status"})
	if err != nil {
		return fmt.Errorf("control: checking the supervisor's state root: %w", err)
	}
	var st struct {
		Root string `json:"root"`
	}
	if err := json.Unmarshal(res.Body, &st); err != nil {
		return fmt.Errorf("control: decoding the supervisor's state root: %w", err)
	}
	if !sameRoot(canonicalRoot(st.Root), want) {
		return fmt.Errorf("%w: the supervisor serves %s, this command uses %s", ErrRootConflict, st.Root, want)
	}
	return nil
}

func dialOrSpawn(ctx context.Context, t Transport, endpoint string, start func() error) (Conn, error) {
	conn, err := t.Dial(endpoint)
	if err == nil || !errors.Is(err, ErrNoSupervisor) {
		return conn, err
	}
	if err := start(); err != nil {
		return nil, fmt.Errorf("control: starting the supervisor: %w", err)
	}
	deadline := time.After(spawnWait)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline:
			return nil, fmt.Errorf("control: the supervisor did not accept connections within %s: %w", spawnWait, ErrNoSupervisor)
		case <-time.After(spawnPoll):
		}
		conn, err = t.Dial(endpoint)
		if err == nil || !errors.Is(err, ErrNoSupervisor) {
			return conn, err
		}
	}
}

// spawnEnvKeys are the variables that locate the state root and the endpoint:
// on Unix the XDG and home variables, on Windows the temp directory (instance
// lock), the user name and %LocalAppData% (state root); SystemRoot keeps the
// Windows runtime working.
var spawnEnvKeys = []string{
	"MYTHHELM_HOME", "XDG_RUNTIME_DIR", "XDG_STATE_HOME", "HOME", "TMPDIR",
	"TEMP", "TMP", "USERNAME", "USERPROFILE", "LocalAppData", "SystemRoot",
}

// spawnEnv is the environment a supervisor starts with: the variables that
// locate its state and its socket, nothing else.
func spawnEnv() []string {
	var env []string
	for _, k := range spawnEnvKeys {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

func spawn() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, SupervisorCommand) //nolint:gosec // G204: our own binary, a fixed subcommand
	cmd.Env = spawnEnv()
	if err := detach(cmd); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// Call sends intent over conn and returns the reply. A reply carrying an
// Error returns it as the error too.
func Call(ctx context.Context, conn Conn, intent Intent) (Result, error) {
	frame, err := Encode(intent)
	if err != nil {
		return Result{}, err
	}
	reply, err := conn.Request(ctx, frame)
	if err != nil {
		return Result{}, err
	}
	payload, err := splitPrefix(reply)
	if err != nil {
		return Result{}, err
	}
	res, err := decodeWire[Result](payload)
	if err != nil {
		return Result{}, err
	}
	if res.Error != nil {
		return res, res.Error
	}
	return res, nil
}

// RunSupervisor serves the control protocol for the state root in dir until
// ctx ends. If another supervisor already serves this root it returns nil:
// the caller was only racing to start one.
func RunSupervisor(ctx context.Context, dir string) error {
	if err := statedir.Ensure(dir); err != nil {
		return err
	}
	release, err := AcquireInstance(dir)
	if errors.Is(err, ErrInstanceHeld) {
		return nil
	}
	if err != nil {
		return err
	}
	defer release()
	db, err := OpenLedger(ctx, dir)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	t, err := DefaultTransport()
	if err != nil {
		return err
	}
	endpoint, err := EndpointPath()
	if err != nil {
		return err
	}
	// Holding the instance lock means no other supervisor serves this
	// socket, so a file left at its path is stale.
	if err := os.Remove(endpoint); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("control: removing a stale socket: %w", err)
	}
	l, err := t.Listen(endpoint)
	if err != nil {
		return err
	}
	return Serve(ctx, l, NewSupervisorServer(db), db)
}

// DialExisting connects only to an existing authenticated endpoint. It never
// spawns a service, migrates a root, assigns work or invokes recovery.
func DialExisting(ctx context.Context) (Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	t, err := DefaultTransport()
	if err != nil {
		return nil, err
	}
	endpoint, err := EndpointPath()
	if err != nil {
		return nil, err
	}
	dir, err := statedir.Resolve()
	if err != nil {
		return nil, err
	}
	return dialExisting(ctx, t, endpoint, canonicalRoot(dir))
}
func dialExisting(ctx context.Context, t Transport, endpoint, want string) (Conn, error) {
	conn, err := t.DialContext(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	if !conn.Peer().SameUser {
		_ = conn.Close()
		return nil, newError(CodePermissionDenied, "unauthenticated endpoint")
	}
	if err := requireRoot(ctx, conn, want); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

// typedConnection serialises all requests made through one typed client, with
// a finite queue wait included in the end-to-end budget. A read cancellation
// affects this request only and never sends a cancellation mutation.
type typedConnection struct {
	conn  Conn
	slots chan struct{}
}

func newTypedConnection(conn Conn) *typedConnection {
	return &typedConnection{conn: conn, slots: make(chan struct{}, 1)}
}
func (c *typedConnection) enter(ctx context.Context) (context.Context, func(), error) {
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	select {
	case c.slots <- struct{}{}:
		return bounded, func() { <-c.slots; cancel() }, nil
	case <-bounded.Done():
		cancel()
		return nil, nil, bounded.Err()
	}
}
func (c *typedConnection) invoke(ctx context.Context, method, operationID, object string, rev uint64, gen *int64, token string, params any, limit int) (json.RawMessage, error) {
	if c.conn == nil || !c.conn.Peer().SameUser {
		return nil, newError(CodePermissionDenied, "authenticated same-user connection required")
	}
	if rev > math.MaxInt64 {
		return nil, contractError("revision exceeds ledger range")
	}
	if p, ok := params.(v2contract.Validator); ok {
		if err := p.Validate(); err != nil {
			return nil, err
		}
	}
	body, err := json.Marshal(params)
	if err != nil {
		return nil, contractError("invalid typed parameters")
	}
	intent := Intent{OperationID: operationID, Object: object, ExpectedRevision: int64(rev), Generation: gen, Method: method, Params: body, CapabilityToken: token}
	if err := intent.Validate(); err != nil {
		return nil, contractError("invalid request envelope")
	}
	frame, err := Encode(intent)
	if err != nil {
		return nil, err
	}
	if len(frame) > limit {
		return nil, contractError("request exceeds frame limit")
	}
	reply, err := c.conn.Request(ctx, frame)
	if err != nil {
		return nil, err
	}
	if len(reply) > limit {
		return nil, contractError("response exceeds frame limit")
	}
	payload, err := splitPrefix(reply)
	if err != nil {
		return nil, contractError("invalid response frame")
	}
	result, err := decodeWire[Result](payload)
	if err != nil {
		return nil, err
	}
	if result.OperationID != operationID || result.Revision < 0 {
		return nil, contractError("response operation mismatch")
	}
	if result.Error != nil {
		if !v2contract.Code(result.Error.Code).Valid() {
			return nil, contractError("unknown response error")
		}
		return nil, result.Error
	}
	return result.Body, nil
}
func typedResult[T v2contract.Validator](body []byte) (T, error) {
	var zero T
	out, err := decodeWire[T](body)
	if err != nil {
		return zero, err
	}
	if err := out.Validate(); err != nil {
		return zero, err
	}
	return out, nil
}

type serviceStatus struct {
	PID        int       `json:"pid"`
	Generation int64     `json:"generation"`
	Root       string    `json:"root"`
	Endpoint   string    `json:"endpoint"`
	StartedAt  time.Time `json:"started_at"`
}

type runClient struct {
	wire       *typedConnection
	generation int64
	observe    sync.Mutex
}

// NewRunClient creates a standalone typed client. The caller owns conn's lifetime.
func NewRunClient(conn Conn) RunClient { return &runClient{wire: newTypedConnection(conn)} }
func (c *runClient) serviceGeneration(ctx context.Context) (int64, error) {
	body, err := c.wire.invoke(ctx, "status", ids.New("op"), "", 0, nil, "", nil, v2contract.MaxFrameBytes)
	if err != nil {
		return 0, err
	}
	s, err := decodeWire[serviceStatus](body)
	if err != nil {
		return 0, err
	}
	if err := generation(s.Generation); err != nil {
		return 0, err
	}
	return s.Generation, nil
}
func (c *runClient) Prepare(ctx context.Context, spec RunSpec) (PreparedRun, error) {
	ctx, done, err := c.wire.enter(ctx)
	if err != nil {
		return PreparedRun{}, err
	}
	defer done()
	if err := spec.Validate(); err != nil {
		return PreparedRun{}, err
	}
	gen, err := c.serviceGeneration(ctx)
	if err != nil {
		return PreparedRun{}, err
	}
	body, err := c.wire.invoke(ctx, "run.prepare", ids.New("op"), "", 0, &gen, "", spec, v2contract.MaxFrameBytes)
	if err != nil {
		return PreparedRun{}, err
	}
	out, err := typedResult[PreparedRun](body)
	if err == nil && out.Generation != gen {
		return PreparedRun{}, contractError("preparation generation mismatch")
	}
	if err == nil {
		c.generation = gen
	}
	return out, err
}
func (c *runClient) preparedCall(ctx context.Context, method, operationID string, rev uint64, request any) (json.RawMessage, error) {
	if c.generation == 0 {
		gen, err := c.serviceGeneration(ctx)
		if err != nil {
			return nil, err
		}
		c.generation = gen
	}
	object := ""
	switch r := request.(type) {
	case RunConfirmation:
		object = r.PreparationID
	case StartRun:
		object = r.PreparationID
	}
	return c.wire.invoke(ctx, method, operationID, object, rev, &c.generation, "", request, v2contract.MaxFrameBytes)
}
func (c *runClient) Confirm(ctx context.Context, r RunConfirmation) (PreparedRun, error) {
	ctx, done, err := c.wire.enter(ctx)
	if err != nil {
		return PreparedRun{}, err
	}
	defer done()
	body, err := c.preparedCall(ctx, "run.confirm", ids.New("op"), 0, r)
	if err != nil {
		return PreparedRun{}, err
	}
	out, err := typedResult[PreparedRun](body)
	if err == nil && (out.Generation != c.generation || out.PreparationID != r.PreparationID || out.BindingSHA256 != r.BindingSHA256) {
		return PreparedRun{}, contractError("confirmation binding mismatch")
	}
	return out, err
}
func (c *runClient) Start(ctx context.Context, op string, rev uint64, r StartRun) (RunAccepted, error) {
	ctx, done, err := c.wire.enter(ctx)
	if err != nil {
		return RunAccepted{}, err
	}
	defer done()
	body, err := c.preparedCall(ctx, "run.start", op, rev, r)
	if err != nil {
		return RunAccepted{}, err
	}
	out, err := typedResult[RunAccepted](body)
	if err == nil && (out.OperationID != op || out.Generation != c.generation) {
		return RunAccepted{}, contractError("start acknowledgment mismatch")
	}
	return out, err
}
func (c *runClient) Recover(ctx context.Context, op string, r RecoverRun) (ActionAccepted, error) {
	return c.action(ctx, "run.recover", op, r.RunID, r.ExpectedRevision, r.Generation, r)
}
func (c *runClient) Apply(ctx context.Context, op string, r ApplyRun) (ActionAccepted, error) {
	return c.action(ctx, "run.apply", op, r.RunID, r.ExpectedRevision, r.Generation, r)
}
func (c *runClient) action(ctx context.Context, method, op, run string, rev uint64, gen int64, r any) (ActionAccepted, error) {
	ctx, done, err := c.wire.enter(ctx)
	if err != nil {
		return ActionAccepted{}, err
	}
	defer done()
	body, err := c.wire.invoke(ctx, method, op, run, rev, &gen, "", r, v2contract.MaxFrameBytes)
	if err != nil {
		return ActionAccepted{}, err
	}
	out, err := typedResult[ActionAccepted](body)
	if err == nil && (out.OperationID != op || out.RunID != run || out.Method != method || out.Generation != gen) {
		return ActionAccepted{}, contractError("action acknowledgment mismatch")
	}
	return out, err
}
func (c *runClient) Operation(ctx context.Context, r OperationQuery) (OperationStatus, error) {
	ctx, done, err := c.wire.enter(ctx)
	if err != nil {
		return OperationStatus{}, err
	}
	defer done()
	gen, err := c.serviceGeneration(ctx)
	if err != nil {
		return OperationStatus{}, err
	}
	body, err := c.wire.invoke(ctx, "run.operation", ids.New("op"), r.RunID, 0, &gen, "", r, v2contract.MaxFrameBytes)
	if err != nil {
		return OperationStatus{}, err
	}
	out, err := typedResult[OperationStatus](body)
	if err == nil && (out.OperationID != r.OperationID || out.RunID != r.RunID) {
		return OperationStatus{}, contractError("operation query identity mismatch")
	}
	return out, err
}
func (c *runClient) Observe(ctx context.Context, r ObserveRun) (RunObservation, error) {
	if !c.observe.TryLock() {
		return RunObservation{}, contractError("observation already outstanding")
	}
	defer c.observe.Unlock()
	ctx, done, err := c.wire.enter(ctx)
	if err != nil {
		return RunObservation{}, err
	}
	defer done()
	if r.Limit == 0 {
		r.Limit = 64
	}
	body, err := c.wire.invoke(ctx, "run.observe", ids.New("op"), r.RunID, 0, &r.Generation, "", r, v2contract.MaxFrameBytes)
	if err != nil {
		return RunObservation{}, err
	}
	out, err := typedResult[RunObservation](body)
	if err != nil {
		return RunObservation{}, err
	}
	if out.RunID != r.RunID || out.Generation != r.Generation || len(out.Events) > r.Limit || len(out.Events) == 0 && out.NextSequence != r.AfterSequence || len(out.Events) > 0 && out.Events[0].RunSequence != r.AfterSequence+1 {
		return RunObservation{}, contractError("observation response cursor or identity mismatch")
	}
	return out, nil
}

type attachmentClient struct{ wire *typedConnection }

// NewAttachmentClient creates a client without opening, migrating or owning a run.
func NewAttachmentClient(conn Conn) AttachmentClient {
	return &attachmentClient{wire: newTypedConnection(conn)}
}
func (c *attachmentClient) invoke(ctx context.Context, method, op, run string, rev uint64, grant *AttachmentGrant, params any) (json.RawMessage, error) {
	ctx, done, err := c.wire.enter(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	var gen *int64
	token := ""
	if grant != nil {
		gen = &grant.Generation
		token = grant.Token
	}
	if !strings.HasPrefix(method, "attachment.") {
		return nil, contractError("attachment method required")
	}
	return c.wire.invoke(ctx, method, op, run, rev, gen, token, params, AttachmentFrameBytes)
}
func (c *attachmentClient) Open(ctx context.Context, run string) (OpenAttachment, error) {
	body, err := c.invoke(ctx, "attachment.open", ids.New("op"), run, 0, nil, attachmentOpenParams{RunID: run})
	if err != nil {
		return OpenAttachment{}, err
	}
	out, err := typedResult[OpenAttachment](body)
	if err == nil && out.Grant.RunID != run {
		return OpenAttachment{}, contractError("opened another run")
	}
	return out, err
}
func (c *attachmentClient) Read(ctx context.Context, g AttachmentGrant) (Projection, error) {
	body, err := c.invoke(ctx, "attachment.read", ids.New("op"), g.RunID, 0, &g, attachmentGrantParams{Grant: g})
	if err != nil {
		return Projection{}, err
	}
	out, err := typedResult[Projection](body)
	if err == nil && (out.AttachmentID != g.AttachmentID || out.RunID != g.RunID || out.Generation != g.Generation) {
		return Projection{}, contractError("projection grant mismatch")
	}
	return out, err
}
func (c *attachmentClient) Stop(ctx context.Context, g AttachmentGrant, r StopRequest) (StopAck, error) {
	body, err := c.invoke(ctx, "attachment.stop", r.OperationID, g.RunID, r.ExpectedRevision, &g, attachmentStopParams{Grant: g, Request: r})
	if err != nil {
		return StopAck{}, err
	}
	out, err := typedResult[StopAck](body)
	if err == nil && out.OperationID != r.OperationID {
		return StopAck{}, contractError("stop acknowledgment mismatch")
	}
	return out, err
}
func (c *attachmentClient) Operation(ctx context.Context, g AttachmentGrant, op string) (StopResult, error) {
	body, err := c.invoke(ctx, "attachment.operation", ids.New("op"), g.RunID, 0, &g, attachmentOperationParams{Grant: g, OperationID: op})
	if err != nil {
		return StopResult{}, err
	}
	out, err := typedResult[StopResult](body)
	if err == nil && out.OperationID != op {
		return StopResult{}, contractError("stop query identity mismatch")
	}
	return out, err
}
func (c *attachmentClient) Close(ctx context.Context, g AttachmentGrant) error {
	body, err := c.invoke(ctx, "attachment.close", ids.New("op"), g.RunID, 0, &g, attachmentGrantParams{Grant: g})
	if err != nil {
		return err
	}
	_, err = decodeWire[struct{}](body)
	return err
}
