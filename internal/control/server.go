package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"sync"
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
