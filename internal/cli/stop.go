package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/statedir"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

func runStop(args []string, stdio Stdio) (retErr error) {
	fs := newFlagSet("stop")
	format := fs.String("format", "plain", "output format: plain or jsonl")
	fs.Bool("non-interactive", false, "never prompt (this command needs no decisions)")
	positional, err := parseFlags(fs, args, stdio)
	if errors.Is(err, flag.ErrHelp) {
		return err
	}
	runID, state := "", ""
	if len(positional) == 1 {
		runID = positional[0]
	}
	defer func() {
		if *format != "jsonl" {
			return
		}
		result := map[string]any{"type": "stop.result", "run_id": runID, "state": state,
			"exit_code": exitCode(retErr), "error_category": errorCategory(retErr)}
		if err := json.NewEncoder(stdio.Out).Encode(result); err != nil && retErr == nil {
			retErr = err
		}
	}()
	if err != nil {
		return err
	}
	if len(positional) != 1 || !strings.HasPrefix(runID, "run_") || strings.ContainsAny(runID, `/\.`) {
		return usageErrorf("expected one valid run ID")
	}
	if *format != "plain" && *format != "jsonl" {
		return usageErrorf("--format must be plain or jsonl")
	}
	ctx := context.Background()
	dir, err := statedir.Resolve()
	if err != nil {
		return err
	}
	j, err := journal.OpenReadOnly(ctx, dir)
	if err != nil {
		return err
	}
	defer func() { _ = j.Close() }()
	state, err = supervisor.Stop(ctx, j, runID)
	if err != nil {
		return err
	}
	if *format == "plain" {
		notice := "stop requested — waiting for worker confirmation"
		if state == "stopped" {
			notice = "stopped — worker confirmed no remaining processes"
		}
		_, err = fmt.Fprintf(stdio.Out, "%s: %s\n", cell(runID), notice)
	}
	return err
}
