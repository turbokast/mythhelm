package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/turbokast/mythhelm/internal/statedir"
)

// SupervisorCommand is the hidden subcommand that runs the supervisor.
const SupervisorCommand = "__supervisor"

const (
	spawnWait    = 10 * time.Second
	spawnPoll    = 25 * time.Millisecond
	clientBudget = 30 * time.Second
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
	return dialOrSpawn(ctx, t, endpoint, spawn)
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

// spawnEnv is the environment a supervisor starts with: the variables that
// locate its state and its socket, nothing else.
func spawnEnv() []string {
	var env []string
	for _, k := range []string{"MYTHHELM_HOME", "XDG_RUNTIME_DIR", "XDG_STATE_HOME", "HOME", "TMPDIR"} {
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
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	var res Result
	if err := dec.Decode(&res); err != nil {
		return Result{}, fmt.Errorf("control: decoding the reply: %w", err)
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
