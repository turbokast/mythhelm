package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/statedir"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workspace"
)

func runApply(args []string, stdio Stdio) (retErr error) {
	fs := newFlagSet("apply")
	branch := fs.String("to-branch", "", "new local branch in the admitted source repository")
	acceptFlags := fs.Bool("accept-flags", false, "accept candidate validation flags")
	acceptUnverified := fs.Bool("accept-unverified", false, "accept a candidate without checks")
	format := fs.String("format", "plain", "output format: plain or jsonl")
	positional, err := parseFlags(fs, args, stdio)
	if errors.Is(err, flag.ErrHelp) {
		return err
	}
	runID := ""
	if len(positional) == 1 {
		runID = positional[0]
	}
	result := map[string]any{"type": "apply.result", "run_id": runID, "branch": *branch}
	defer func() {
		if *format != "jsonl" {
			return
		}
		result["exit_code"], result["error_category"] = exitCode(retErr), errorCategory(retErr)
		if err := json.NewEncoder(stdio.Out).Encode(result); err != nil && retErr == nil {
			retErr = err
		}
	}()
	if err != nil {
		return err
	}
	if len(positional) != 1 || *branch == "" {
		return usageErrorf("expected one run ID and --to-branch")
	}
	if *format != "plain" && *format != "jsonl" {
		return usageErrorf("--format must be plain or jsonl, got %q", *format)
	}
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
	// Interactive writes drain first: apply is refused while migration
	// owns the state directory or the ledger has moved past previewed.
	if err := checkMigrationClear(ctx, j, "apply the run"); err != nil {
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
			return &outcomeError{code: ExitBlocked, category: categories[ExitBlocked], err: err}
		}
		return err
	}
	if *format == "jsonl" {
		result["state"] = r["state"]
		result["candidate_commit"] = reviewMap(r["candidate"])["commit"]
		return nil
	}
	_, err = fmt.Fprintf(stdio.Out, "Applied run %s to branch %s at %s\n", cell(runID), cell(*branch),
		cell(reviewString(reviewMap(r["candidate"]), "commit")))
	return err
}

// checkMigrationClear refuses an interactive write while migration owns the
// state directory (the migration lock is held) or the ledger has moved past
// the previewed phase (I05, I18). It mirrors the supervisor's admission
// guard: the guard stays unexported in each package rather than growing a
// shared cross-package API for two call sites each. The refusal names the
// v2 ownership_unresolved code and wraps supervisor.ErrOwnership, which
// exitCode maps to exit 6.
func checkMigrationClear(ctx context.Context, j *journal.Journal, action string) error {
	var phase string
	err := j.Transact(ctx, func(tx *sql.Tx) error {
		var name string
		tbl := tx.QueryRowContext(ctx, `SELECT name FROM sqlite_master
			WHERE type = 'table' AND name = 'migration_state'`)
		if err := tbl.Scan(&name); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("cli: checking migration_state: %w", err)
		}
		if err := tx.QueryRowContext(ctx,
			`SELECT phase FROM migration_state WHERE id = 1`).Scan(&phase); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("cli: reading migration phase: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	switch phase {
	case "drained", "imported", "adopted":
		return fmt.Errorf("cannot %s: ownership_unresolved: migration phase is %q: %w",
			action, phase, supervisor.ErrOwnership)
	}
	release, err := supervisor.AcquireOwner(j.StateDir())
	if err != nil {
		if errors.Is(err, supervisor.ErrOwnerHeld) {
			return fmt.Errorf("cannot %s: ownership_unresolved: migration holds the state directory: %w",
				action, supervisor.ErrOwnership)
		}
		return err
	}
	release()
	return nil
}
