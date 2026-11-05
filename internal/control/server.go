package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"github.com/turbokast/mythhelm/internal/journal"
)

// Server owns the intent dispatch table as instance state, never a
// package-level map.
type Server struct {
	mu          sync.RWMutex
	methods     map[string]Handler
	long        map[string]bool
	attachments map[string]AttachmentHandler
	prepared    bool
	readOnly    bool
}

// NewServer builds a Server with the given method bindings.
func NewServer(handlers map[string]Handler) *Server {
	s := &Server{methods: make(map[string]Handler, len(handlers)), long: map[string]bool{}, attachments: map[string]AttachmentHandler{}}
	maps.Copy(s.methods, handlers)
	return s
}

// NewStatusServer serves only authenticated, validated read-only status.
// No operation or ledger transaction is created, including refusals.
func NewStatusServer(status Handler) *Server {
	handlers := map[string]Handler{}
	if status != nil {
		handlers["status"] = status
	}
	s := NewServer(handlers)
	s.readOnly = true
	return s
}

// Register binds method to h. A duplicate registration, or a transport-only
// attachment method, is an error naming the method.
func (s *Server) Register(method string, h Handler) error {
	if attachmentMethod(method) {
		return fmt.Errorf("control: method %q is served only by RegisterAttachment", method)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.methods[method]; dup {
		return fmt.Errorf("control: method %q is already registered", method)
	}
	s.methods[method] = h
	return nil
}

// RegisterLong binds method to h as a long handler: Dispatch runs it
// through ExecuteLong, so waits and post-commit launches hold no ledger
// transaction. A duplicate registration is an error naming the method.
func (s *Server) RegisterLong(method string, h Handler) error {
	if err := s.Register(method, h); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.long[method] = true
	return nil
}

// Dispatch runs the intent's method through Execute, or through
// ExecuteLong for a method bound with RegisterLong. An unregistered
// method, and a transport-only attachment method, returns
// capability_unsupported without touching the ledger.
func (s *Server) Dispatch(ctx context.Context, peer Peer, intent Intent) (Result, error) {
	if s.readOnly {
		if !peer.SameUser {
			return Result{}, newError(CodePermissionDenied, "peer is not the supervisor user")
		}
		if err := intent.Validate(); err != nil {
			return Result{}, newError(CodeInvalidContract, "invalid intent: %v", err)
		}
	}
	if attachmentMethod(intent.Method) {
		return Result{}, newError(CodeCapabilityUnsupported, "method %q is served only by the attachment transport", intent.Method)
	}
	s.mu.RLock()
	h, ok := s.methods[intent.Method]
	long := s.long[intent.Method]
	prepared := s.prepared && (intent.Method == "run.prepare" || intent.Method == "run.confirm")
	s.mu.RUnlock()
	if !ok {
		return Result{}, newError(CodeCapabilityUnsupported, "method %q is not supported", intent.Method)
	}
	if prepared {
		if !peer.SameUser || intent.CapabilityToken != "" {
			return Result{}, newError(CodePermissionDenied, "ordinary admission requires the standalone OS peer")
		}
		if err := intent.Validate(); err != nil {
			return Result{}, newError(CodeInvalidContract, "invalid admission intent")
		}
		return h(ctx, peer, intent)
	}
	if s.readOnly {
		return h(ctx, peer, intent)
	}
	if long {
		return ExecuteLong(ctx, h, peer, intent)
	}
	return Execute(ctx, h, peer, intent)
}

// EndpointPath is the supervisor's control socket: beside the instance lock,
// so it is per user and never under a state root.
func EndpointPath() (string, error) {
	lock, err := LockPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(lock), "control.sock"), nil
}

// StatusHandler answers the status intent from the instance lock metadata:
// the running pid, boot generation, state root and endpoint. A ledger that
// cannot be reached is persistence_unavailable.
func StatusHandler(db *sql.DB) Handler {
	return func(ctx context.Context, _ Peer, _ Intent) (Result, error) {
		if err := db.PingContext(ctx); err != nil {
			return Result{}, newError(CodePersistenceUnavailable, "ledger unreachable: %v", err)
		}
		lock, err := LockPath()
		if err != nil {
			return Result{}, err
		}
		meta, ok := readMetadata(lock)
		if !ok {
			return Result{}, newError(CodeProcessLost, "no supervisor instance is recorded")
		}
		endpoint, err := EndpointPath()
		if err != nil {
			return Result{}, err
		}
		body, err := json.Marshal(map[string]any{"pid": meta.PID, "generation": meta.Generation, "root": meta.Root,
			"endpoint": endpoint, "started_at": meta.StartedAt})
		if err != nil {
			return Result{}, err
		}
		return Result{Body: body}, nil
	}
}

const supervisorOwner = "supervisor"

// AssignHandler records ownership of the run named by the intent's Object
// under the supervisor. Assigning an unknown run is invalid_contract; a run
// already assigned is revision_conflict.
func AssignHandler(db *sql.DB) Handler {
	return func(ctx context.Context, _ Peer, in Intent) (Result, error) {
		if in.Object == "" {
			return Result{}, newError(CodeInvalidContract, "assign needs the run as the intent object")
		}
		err := Mutate(ctx, db, func(tx *sql.Tx) error {
			var one int
			err := tx.QueryRowContext(ctx, `SELECT 1 FROM runs WHERE run_id = ?`, in.Object).Scan(&one)
			if errors.Is(err, sql.ErrNoRows) {
				return newError(CodeInvalidContract, "run %q is unknown", in.Object)
			}
			if err != nil {
				return newError(CodePersistenceUnavailable, "reading run: %v", err)
			}
			err = tx.QueryRowContext(ctx, `SELECT 1 FROM run_assignments WHERE run_id = ?`, in.Object).Scan(&one)
			if err == nil {
				return newError(CodeRevisionConflict, "run %q is already assigned", in.Object)
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return newError(CodePersistenceUnavailable, "reading assignment: %v", err)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO run_assignments (run_id, owner, assigned_at, operation_id) VALUES (?,?,?,?)`,
				in.Object, supervisorOwner, now(), in.OperationID); err != nil {
				return newError(CodePersistenceUnavailable, "recording assignment: %v", err)
			}
			return nil
		})
		if err != nil {
			return Result{}, err
		}
		body, err := json.Marshal(map[string]string{"run_id": in.Object, "owner": supervisorOwner})
		if err != nil {
			return Result{}, err
		}
		return Result{Revision: in.ExpectedRevision + 1, Body: body}, nil
	}
}

// RegisterStop binds the stop method to s as a long handler: the spool
// wait runs outside any transaction. NewSupervisorServer does not call it:
// in S1 stop runs locally from the CLI.
func (s *Server) RegisterStop(d StopDeps) error {
	return s.RegisterLong("stop", StopHandler(d))
}

// RegisterRecover binds the recover method to s as a long handler: the
// continuation launch is a post-commit handoff. NewSupervisorServer does
// not call it: in S1 recover runs locally from the CLI.
func (s *Server) RegisterRecover(d RecoverDeps) error {
	return s.RegisterLong("recover", RecoverHandler(d))
}

// NewSupervisorServer binds every method the supervisor serves: the six
// short intents on db. Stop and recover are not served in S1 (Q-27): the
// handlers write attempt state with no journal event and no spool ingest,
// so a served request would leave the next owner ingesting a worker's
// stop_requested against a stopped attempt. They answer
// capability_unsupported and write nothing; the CLI runs both locally under
// run ownership.
func NewSupervisorServer(db *sql.DB) *Server {
	return NewServer(map[string]Handler{
		"status":    StatusHandler(db),
		"assign":    AssignHandler(db),
		"reserve":   ReserveHandler(db),
		"release":   ReleaseHandler(db),
		"heartbeat": HeartbeatHandler(db),
		"read":      ReadHandler(db),
	})
}

// OpenLedger migrates the state database in dir and returns the supervisor's
// handle on it, with the journal's pragmas: a busy timeout, foreign keys,
// WAL, full sync and write transactions that take the lock at BEGIN.
func OpenLedger(ctx context.Context, dir string) (*sql.DB, error) {
	j, err := journal.Open(ctx, dir)
	if err != nil {
		return nil, err
	}
	if err := j.Close(); err != nil {
		return nil, err
	}
	p := filepath.ToSlash(filepath.Join(dir, journal.DBName))
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	u := url.URL{Scheme: "file", Path: p, RawQuery: "_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_txlock=immediate"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("control: opening the ledger: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("control: opening the ledger: %w", err)
	}
	return db, nil
}

// Serve accepts connections on l until ctx ends or l fails, answering each
// request frame through srv with db as the ledger. It returns after every
// connection handler has finished.
func Serve(ctx context.Context, l Listener, srv *Server, db *sql.DB) error {
	var wg sync.WaitGroup
	defer wg.Wait()
	stop := context.AfterFunc(ctx, func() { _ = l.Close() })
	defer stop()
	for {
		conn, err := l.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("control: accepting: %w", err)
		}
		wg.Go(func() { serveConn(WithLedger(ctx, db), conn, srv) })
	}
}

// serveConn answers request frames until the peer leaves or a frame breaks a
// limit, which ends the integration (NFR-1). One owned writer drains a
// queue of one pending frame while it writes another, so a stalled Respond
// bounds what the connection holds. serveConn closes conn and returns only
// after the writer has returned; a Respond that never returns blocks it.
func serveConn(ctx context.Context, conn Conn, srv *Server) {
	ctx, cancel := context.WithCancel(ctx)
	queue := make(chan []byte, 1)
	var writer sync.WaitGroup
	writer.Go(func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case out := <-queue:
				if err := conn.Respond(out); err != nil || ctx.Err() != nil {
					return
				}
			}
		}
	})
	defer func() {
		cancel()
		_ = conn.Close()
		writer.Wait()
	}()
	for {
		frame, err := conn.Receive(ctx)
		if err != nil {
			return
		}
		in, err := Decode[Intent](frame)
		if err == nil && attachmentMethod(in.Method) {
			if !srv.answerAttachment(ctx, conn.Peer(), in, queue) {
				return
			}
			continue
		}
		res := answer(ctx, conn.Peer(), srv, in, err)
		out, err := Encode(res)
		if err != nil {
			return
		}
		select {
		case queue <- out:
		case <-ctx.Done():
			return
		}
	}
}

// answer dispatches one decoded request frame. Every failure is a Result
// carrying its Error, so the client sees one reply shape.
func answer(ctx context.Context, peer Peer, srv *Server, in Intent, decodeErr error) Result {
	if decodeErr != nil {
		return Result{Error: newError(CodeInvalidContract, "%v", decodeErr)}
	}
	res, err := srv.Dispatch(ctx, peer, in)
	res.OperationID = in.OperationID
	if err != nil {
		res.Error = replyError(err)
	}
	return res
}

// replyError keeps a control Error and hides any other failure behind one
// constant message, so no raw payload reaches the wire.
func replyError(err error) *Error {
	if ce, ok := errors.AsType[*Error](err); ok {
		return ce
	}
	return newError(CodePersistenceUnavailable, "the request could not be completed")
}
