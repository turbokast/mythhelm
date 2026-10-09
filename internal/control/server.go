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
	mu      sync.RWMutex
	methods map[string]Handler
}

// NewServer builds a Server with the given method bindings.
func NewServer(handlers map[string]Handler) *Server {
	s := &Server{methods: make(map[string]Handler, len(handlers))}
	maps.Copy(s.methods, handlers)
	return s
}

// Register binds method to h. A duplicate registration is an error naming
// the method.
func (s *Server) Register(method string, h Handler) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.methods[method]; dup {
		return fmt.Errorf("control: method %q is already registered", method)
	}
	s.methods[method] = h
	return nil
}

// Dispatch runs the intent's method through Execute. An unregistered method
// returns capability_unsupported without touching the ledger.
func (s *Server) Dispatch(ctx context.Context, peer Peer, intent Intent) (Result, error) {
	s.mu.RLock()
	h, ok := s.methods[intent.Method]
	s.mu.RUnlock()
	if !ok {
		return Result{}, newError(CodeCapabilityUnsupported, "method %q is not supported", intent.Method)
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
			return Result{}, newError(CodeCapabilityUnsupported, "no supervisor instance is recorded")
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

// RegisterStop binds the stop method to s. It stays out of
// NewSupervisorServer until Task 4 folds the stop and recover methods into
// the served set with their support-matrix rows, so the matrix test keeps
// passing without a stop row meanwhile.
func (s *Server) RegisterStop(d StopDeps) error {
	return s.Register("stop", StopHandler(d))
}

// NewSupervisorServer binds every method the supervisor serves to db.
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
		wg.Go(func() {
			defer func() { _ = conn.Close() }()
			serveConn(WithLedger(ctx, db), conn, srv)
		})
	}
}

// serveConn answers request frames until the peer leaves or a frame breaks a
// limit, which ends the integration (NFR-1).
func serveConn(ctx context.Context, conn Conn, srv *Server) {
	for {
		frame, err := conn.Receive(ctx)
		if err != nil {
			return
		}
		res := answer(ctx, conn.Peer(), srv, frame)
		out, err := Encode(res)
		if err != nil {
			return
		}
		if err := conn.Respond(out); err != nil {
			return
		}
	}
}

// answer decodes one request frame and dispatches it. Every failure is a
// Result carrying its Error, so the client sees one reply shape.
func answer(ctx context.Context, peer Peer, srv *Server, frame []byte) Result {
	in, err := Decode[Intent](frame)
	if err != nil {
		return Result{Error: newError(CodeInvalidContract, "%v", err)}
	}
	res, err := srv.Dispatch(ctx, peer, in)
	res.OperationID = in.OperationID
	var ce *Error
	switch {
	case err == nil:
	case errors.As(err, &ce):
		res.Error = ce
	default:
		res.Error = newError(CodePersistenceUnavailable, "the request could not be completed")
	}
	return res
}
