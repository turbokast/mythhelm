package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/statedir"
)

// SupervisorCommand is the hidden subcommand that runs the supervisor
// process; main routes it to SupervisorMain.
const SupervisorCommand = control.SupervisorCommand

// statusTimeout bounds one `supervisor status`, including a lazy start.
const statusTimeout = 30 * time.Second

func runSupervisor(args []string, stdio Stdio) error {
	if len(args) == 0 || args[0] != "status" {
		if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "-help") {
			_, err := io.WriteString(stdio.Out, "usage: mythhelm supervisor status [--format plain|jsonl]\n")
			return err
		}
		return usageErrorf("expected the subcommand: status")
	}
	return runSupervisorStatus(args[1:], stdio)
}

func runSupervisorStatus(args []string, stdio Stdio) error {
	fs := newFlagSet("supervisor status")
	format := fs.String("format", "plain", "output format: plain or jsonl")
	positional, err := parseFlags(fs, args, stdio)
	if errors.Is(err, flag.ErrHelp) {
		return err
	}
	if err != nil {
		return err
	}
	if len(positional) != 0 {
		return usageErrorf("supervisor status takes no arguments")
	}
	if *format != "plain" && *format != "jsonl" {
		return usageErrorf("--format must be plain or jsonl")
	}

	ctx, cancel := context.WithTimeout(context.Background(), statusTimeout)
	defer cancel()
	conn, err := control.Connect(ctx)
	if errors.Is(err, control.ErrSpawnUnsupported) {
		return &outcomeError{code: ExitCapability, category: "capability_unsupported", err: err}
	}
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	res, err := control.Call(ctx, conn, control.Intent{OperationID: ids.New("op"), Method: "status"})
	if err != nil {
		return err
	}
	var st struct {
		PID        int    `json:"pid"`
		Generation uint64 `json:"generation"`
		Root       string `json:"root"`
		Endpoint   string `json:"endpoint"`
	}
	if err := json.Unmarshal(res.Body, &st); err != nil {
		return fmt.Errorf("decoding the supervisor status: %w", err)
	}
	if *format == "jsonl" {
		return json.NewEncoder(stdio.Out).Encode(map[string]any{"type": "supervisor.status", "pid": st.PID,
			"generation": st.Generation, "root": st.Root, "endpoint": st.Endpoint})
	}
	_, err = fmt.Fprintf(stdio.Out, "pid:        %d\ngeneration: %d\nroot:       %s\nendpoint:   %s\n", st.PID, st.Generation, st.Root, st.Endpoint)
	return err
}

// SupervisorMain runs the supervisor for the resolved state root until it
// is interrupted, and returns the process exit code.
func SupervisorMain(stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dir, err := statedir.Resolve()
	if err == nil {
		err = control.RunSupervisor(ctx, dir)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "mythhelm supervisor: %v\n", err)
		return int(ExitInternal)
	}
	return int(ExitOK)
}
