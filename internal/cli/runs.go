package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/security"
	"github.com/turbokast/mythhelm/internal/statedir"
)

const runsUsage = "usage: mythhelm runs list [--format plain|jsonl] [--limit N]\n"

func runRuns(args []string, stdio Stdio) error {
	if len(args) == 0 {
		return usageErrorf("missing subcommand; %s", strings.TrimSuffix(runsUsage, "\n"))
	}
	switch args[0] {
	case "list":
		return runRunsList(args[1:], stdio)
	case "-h", "-help", "--help":
		_, err := io.WriteString(stdio.Out, runsUsage)
		return err
	}
	return usageErrorf("unknown subcommand %q; %s", args[0], strings.TrimSuffix(runsUsage, "\n"))
}

// runRunsList prints run projections, newest first. It opens the database
// read-only and never creates the state directory (AC-9.3).
func runRunsList(args []string, stdio Stdio) error {
	fs := newFlagSet("runs list")
	format := fs.String("format", "plain", "output format: plain or jsonl")
	limit := fs.Int("limit", 20, "maximum number of runs to list, newest first")
	positional, err := parseFlags(fs, args, stdio)
	if err != nil {
		return err
	}
	switch {
	case len(positional) > 0:
		return usageErrorf("unexpected argument %q", positional[0])
	case *format != "plain" && *format != "jsonl":
		return usageErrorf("--format must be plain or jsonl, got %q", *format)
	case *limit < 1:
		return usageErrorf("--limit must be at least 1, got %d", *limit)
	}

	rows, err := listRuns(context.Background(), *limit)
	if err != nil {
		return err
	}
	if *format == "jsonl" {
		return writeRunsJSONL(stdio.Out, rows)
	}
	return writeRunsPlain(stdio.Out, rows)
}

func listRuns(ctx context.Context, limit int) ([]journal.RunRow, error) {
	dir, err := statedir.Resolve()
	if err != nil {
		return nil, err
	}
	j, err := journal.OpenReadOnly(ctx, dir)
	if errors.Is(err, journal.ErrNoDatabase) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = j.Close() }()
	return j.ListRuns(ctx, limit)
}

// cellEscapes shows the tab and newline that security.TermSafe keeps, so a
// value cannot shift columns or forge a row.
var cellEscapes = strings.NewReplacer("\t", `\t`, "\n", `\n`)

// cell makes a stored value safe for one plain-output table cell (AC-8.2).
func cell(s string) string {
	return cellEscapes.Replace(security.TermSafe(s))
}

func writeRunsPlain(w io.Writer, rows []journal.RunRow) error {
	if len(rows) == 0 {
		_, err := io.WriteString(w, "no runs\n")
		return err
	}
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	_, _ = io.WriteString(tw, "RUN\tSTATE\tADAPTER\tUPDATED\tREPOSITORY\n")
	for _, r := range rows {
		state := r.State
		if r.Reason != "" {
			state += " (" + r.Reason + ")"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", cell(r.RunID), cell(state), cell(r.AdapterID),
			r.UpdatedAt.UTC().Format(time.RFC3339), cell(r.SourceRepo))
	}
	_ = tw.Flush() // writes to a strings.Builder cannot fail
	_, err := io.WriteString(w, b.String())
	return err
}

// runRowObject is the `runs list --format jsonl` schema (AC-1.3). Unset
// optional fields are explicit nulls.
type runRowObject struct {
	Type             string    `json:"type"`
	RunID            string    `json:"run_id"`
	State            string    `json:"state"`
	Reason           *string   `json:"reason"`
	AdapterID        string    `json:"adapter_id"`
	SourceRepo       string    `json:"source_repo"`
	SourceBranch     *string   `json:"source_branch"`
	BaseRev          *string   `json:"base_rev"`
	TaskSHA256       string    `json:"task_sha256"`
	BillingPosture   string    `json:"billing_posture"`
	ExecutionProfile string    `json:"execution_profile"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func writeRunsJSONL(w io.Writer, rows []journal.RunRow) error {
	enc := json.NewEncoder(w)
	for _, r := range rows {
		if err := enc.Encode(runRowObject{
			Type:             "run_row",
			RunID:            r.RunID,
			State:            r.State,
			Reason:           nullIfEmpty(r.Reason),
			AdapterID:        r.AdapterID,
			SourceRepo:       r.SourceRepo,
			SourceBranch:     nullIfEmpty(r.SourceBranch),
			BaseRev:          nullIfEmpty(r.BaseRev),
			TaskSHA256:       r.TaskSHA256,
			BillingPosture:   r.BillingPosture,
			ExecutionProfile: r.ExecutionProfile,
			CreatedAt:        r.CreatedAt.UTC(),
			UpdatedAt:        r.UpdatedAt.UTC(),
		}); err != nil {
			return err
		}
	}
	return nil
}
