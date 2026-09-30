package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/security"
	"github.com/turbokast/mythhelm/internal/statedir"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workspace"
)

var revisionID = regexp.MustCompile(`^[0-9a-fA-F]{40,64}$`)

func runReview(args []string, stdio Stdio) error {
	fs := newFlagSet("review")
	format := fs.String("format", "plain", "output format: plain or jsonl")
	noDiff := fs.Bool("no-diff", false, "omit the candidate diff from plain output")
	positional, err := parseFlags(fs, args, stdio)
	if err != nil {
		return err
	}
	if len(positional) != 1 {
		return usageErrorf("expected one run ID")
	}
	if *format != "plain" && *format != "jsonl" {
		return usageErrorf("--format must be plain or jsonl, got %q", *format)
	}
	if !strings.HasPrefix(positional[0], "run_") || strings.ContainsAny(positional[0], `/\.`) {
		return usageErrorf("invalid run ID %q", positional[0])
	}
	ctx := context.Background()
	dir, err := statedir.Resolve()
	if err != nil {
		return err
	}
	j, err := journal.OpenReadOnly(ctx, dir)
	if errors.Is(err, journal.ErrNoDatabase) {
		return fmt.Errorf("run %s: %w", positional[0], journal.ErrNotFound)
	}
	if err != nil {
		return err
	}
	defer func() { _ = j.Close() }()
	run, err := j.Run(ctx, positional[0])
	if err != nil {
		return err
	}
	runDir := filepath.Join(dir, "runs", run.RunID)
	b, err := os.ReadFile(filepath.Join(runDir, "receipt.json")) // #nosec G304 -- run ID is validated and projection-bound
	if err != nil {
		return fmt.Errorf("reading receipt for %s: %w", run.RunID, err)
	}
	events, err := j.Events(ctx, run.RunID, 0)
	if err != nil {
		return err
	}
	var expected string
	for _, ev := range events {
		if ev.Type != "receipt.written" {
			continue
		}
		var m struct {
			SHA256 string `json:"sha256"`
		}
		if err := json.Unmarshal(ev.Payload, &m); err != nil {
			return err
		}
		expected = m.SHA256
	}
	sum := sha256.Sum256(b)
	if expected == "" || expected != hex.EncodeToString(sum[:]) {
		return fmt.Errorf("receipt for %s does not match its journaled SHA-256", run.RunID)
	}
	var r supervisor.Receipt
	if err := json.Unmarshal(b, &r); err != nil {
		return err
	}
	if r["run_id"] != run.RunID || r["state"] != run.State {
		return fmt.Errorf("receipt for %s does not match its run projection", run.RunID)
	}
	if *format == "jsonl" {
		return json.NewEncoder(stdio.Out).Encode(r)
	}
	return writeReviewPlain(ctx, stdio.Out, j, r, run.RunID, !*noDiff)
}

func reviewString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return "unknown"
	}
	return cell(fmt.Sprint(v))
}

func reviewMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func writeReviewPlain(ctx context.Context, w io.Writer, j *journal.Journal, r supervisor.Receipt, runID string, diff bool) error {
	requested := reviewMap(r["requested_outcome"])
	candidate := reviewMap(r["candidate"])
	verification := reviewMap(r["verification"])
	if _, err := fmt.Fprintf(w, "Run: %s\nState: %s\nTask: %s\nCandidate: %s\n\n",
		cell(runID), reviewString(r, "state"), reviewString(requested, "title"), reviewString(candidate, "commit")); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "Flags:\n"); err != nil {
		return err
	}
	flags, _ := candidate["flags"].([]any)
	if len(flags) == 0 {
		if _, err := io.WriteString(w, "  none\n"); err != nil {
			return err
		}
	}
	for _, flag := range flags {
		b, err := json.Marshal(flag)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "  %s\n", cell(string(b))); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "Checks:\n"); err != nil {
		return err
	}
	checks, _ := verification["checks"].([]any)
	if len(checks) == 0 {
		if _, err := io.WriteString(w, "  none\n"); err != nil {
			return err
		}
	}
	for _, raw := range checks {
		c := reviewMap(raw)
		if _, err := fmt.Fprintf(w, "  %s: %s (evidence: %s; sha256: %s)\n",
			reviewString(c, "name"), reviewString(c, "status"), reviewString(c, "evidence"), reviewString(c, "sha256")); err != nil {
			return err
		}
	}
	if !diff || reviewString(candidate, "commit") == "unknown" {
		return nil
	}
	base, commit := reviewString(candidate, "base_rev"), reviewString(candidate, "commit")
	if !revisionID.MatchString(base) || !revisionID.MatchString(commit) {
		return fmt.Errorf("receipt has an invalid candidate revision")
	}
	attempt, err := j.LatestAttempt(ctx, runID)
	if err != nil {
		return err
	}
	patch, err := workspace.Git(ctx, attempt.WorkspacePath, false, "diff", "--no-color", "--no-ext-diff", "--no-textconv", base, commit) //nolint:misspell // Git's flag is --no-color.
	if err != nil {
		return fmt.Errorf("reading candidate diff: %w", err)
	}
	if _, err := io.WriteString(w, "\nCandidate diff:\n"); err != nil {
		return err
	}
	_, err = io.WriteString(w, security.TermSafe(string(patch)))
	return err
}
