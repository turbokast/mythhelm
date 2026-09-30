package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/statedir"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workspace"
)

func runApply(args []string, stdio Stdio) error {
	fs := newFlagSet("apply")
	branch := fs.String("to-branch", "", "new local branch in the admitted source repository")
	acceptFlags := fs.Bool("accept-flags", false, "accept candidate validation flags")
	acceptUnverified := fs.Bool("accept-unverified", false, "accept a candidate without checks")
	format := fs.String("format", "plain", "output format: plain or jsonl")
	positional, err := parseFlags(fs, args, stdio)
	if err != nil {
		return err
	}
	if len(positional) != 1 || *branch == "" {
		return usageErrorf("expected one run ID and --to-branch")
	}
	if *format != "plain" && *format != "jsonl" {
		return usageErrorf("--format must be plain or jsonl, got %q", *format)
	}
	runID := positional[0]
	if !strings.HasPrefix(runID, "run_") || strings.ContainsAny(runID, `/\\.`) {
		return usageErrorf("invalid run ID %q", runID)
	}
	ctx := context.Background()
	dir, err := statedir.Resolve()
	if err != nil {
		return err
	}
	j, err := journal.Open(ctx, dir)
	if err != nil {
		return err
	}
	defer func() { _ = j.Close() }()
	if _, err := j.Run(ctx, runID); err != nil {
		return err
	}
	release, err := supervisor.AcquireOwner(filepath.Join(dir, "runs", runID))
	if err != nil {
		return err
	}
	defer release()
	r, err := supervisor.ApplyRun(ctx, j, runID, *branch, *acceptFlags, *acceptUnverified)
	if err != nil {
		if errors.Is(err, workspace.ErrInvalidBranch) {
			return usageError{err}
		}
		if errors.Is(err, supervisor.ErrApplyBlocked) || errors.Is(err, workspace.ErrBranchExists) {
			return &outcomeError{code: ExitBlocked, category: "apply_blocked", err: err}
		}
		return err
	}
	if *format == "jsonl" {
		return json.NewEncoder(stdio.Out).Encode(r)
	}
	_, err = fmt.Fprintf(stdio.Out, "Applied run %s to branch %s at %s\n", cell(runID), cell(*branch),
		cell(reviewString(reviewMap(r["candidate"]), "commit")))
	return err
}
