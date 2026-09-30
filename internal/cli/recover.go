package cli

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/signal"
	"strings"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/statedir"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

// runRecover takes over an abandoned run without launching a new native.
func runRecover(args []string, stdio Stdio) error {
	fs := newFlagSet("recover")
	format := fs.String("format", "plain", "output format: plain or jsonl")
	fs.Bool("non-interactive", false, "never prompt (recovery needs no new approval)")
	pos, err := parseFlags(fs, args, stdio)
	if errors.Is(err, flag.ErrHelp) {
		return err
	}
	r := newRenderer(*format, stdio)
	out := supervisor.Outcome{}
	if err == nil && (len(pos) != 1 || !strings.HasPrefix(pos[0], "run_") || strings.ContainsAny(pos[0], `/\.`)) {
		err = usageErrorf("expected one valid run ID")
	}
	if *format != "plain" && *format != "jsonl" {
		r = newRenderer("plain", stdio)
		err = usageErrorf("--format must be plain or jsonl")
	}
	if err != nil {
		return finish(r, out, err)
	}
	out.RunID = pos[0]
	ctx := context.Background()
	dir, err := statedir.Resolve()
	if err != nil {
		return finish(r, out, err)
	}
	j, err := journal.Open(ctx, dir)
	if err != nil {
		return finish(r, out, err)
	}
	defer func() { _ = j.Close() }()
	interrupts := make(chan os.Signal, 2)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	recovered, err := supervisor.RecoverWithHooks(ctx, j, pos[0], supervisor.Hooks{Event: r.event, Notice: r.notice, Interrupt: interrupts})
	out = recovered.Outcome
	if err == nil {
		if code, category := runExit(out); code != ExitOK {
			err = &outcomeError{code: code, category: category, err: errors.New(describe(out))}
		}
	}
	return finish(r, out, err)
}
