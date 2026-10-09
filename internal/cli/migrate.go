package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // registers the "sqlite" driver for the read-only preview handle

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/migrate"
	"github.com/turbokast/mythhelm/internal/migrate/preview"
	"github.com/turbokast/mythhelm/internal/statedir"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// runMigrate implements `mythhelm migrate` (supervisor-migration design
// §6, D3): `migrate --preview` prints the hermetic plan and writes
// nothing; `migrate --apply` previews first, then runs Drain → Backup →
// Import → Adopt once `--yes` confirms, and prints a receipt. Bare
// `migrate` previews.
func runMigrate(args []string, stdio Stdio) error {
	fs := newFlagSet("migrate")
	showPreview := fs.Bool("preview", false, "print the migration plan and change nothing")
	apply := fs.Bool("apply", false, "preview, then migrate after --yes confirms")
	yes := fs.Bool("yes", false, "confirm the migration with --apply")
	format := fs.String("format", "plain", "output format: plain or jsonl")
	positional, err := parseFlags(fs, args, stdio)
	if errors.Is(err, flag.ErrHelp) {
		return err
	}
	if err != nil {
		return err
	}
	if len(positional) != 0 {
		return usageErrorf("migrate takes no arguments")
	}
	if *format != "plain" && *format != "jsonl" {
		return usageErrorf("--format must be plain or jsonl, got %q", *format)
	}
	if *showPreview && *apply {
		return usageErrorf("--preview and --apply are mutually exclusive")
	}
	if !*showPreview && !*apply {
		*showPreview = true
	}
	ctx := context.Background()
	dir, err := statedir.Resolve()
	if err != nil {
		return err
	}
	plan, err := previewState(ctx, dir)
	if err != nil {
		return err
	}
	if err := printPlan(stdio, *format, plan); err != nil {
		return err
	}
	if *showPreview || !*yes {
		if *apply && !*yes {
			_, _ = fmt.Fprintln(stdio.Err, "preview only: re-run with --apply --yes to migrate")
		}
		return nil
	}
	return applyState(ctx, stdio, *format, dir, plan)
}

// previewState opens the state database read-only — never creating,
// migrating or writing it — and computes the hermetic plan.
func previewState(ctx context.Context, dir string) (preview.Plan, error) {
	db, err := openStateReadOnly(dir)
	if err != nil {
		return preview.Plan{}, err
	}
	defer func() { _ = db.Close() }()
	plan, err := preview.PreviewPlan(ctx, db)
	if err != nil {
		return preview.Plan{}, mapControlError(err)
	}
	return plan, nil
}

// applyState migrates the schema, applies the previewed plan and prints
// the receipt. The plan printed above is the plan applied: Apply refuses
// when the ledger no longer covers it.
func applyState(ctx context.Context, stdio Stdio, format, dir string, plan preview.Plan) error {
	db, err := control.OpenLedger(ctx, dir)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	report, err := migrate.Apply(ctx, db, plan)
	if err != nil {
		return mapControlError(err)
	}
	receipt, err := readReceipt(ctx, db, report)
	if err != nil {
		return err
	}
	return printReceipt(stdio, format, receipt)
}

// openStateReadOnly opens dir/mythhelm.db read-only for the preview. It
// tolerates any schema version — the preview itself refuses newer ones
// with schema_too_new — and creates nothing: an absent database is
// ErrNoDatabase. Like any SQLite reader of a WAL database it may create
// the -shm index and an empty -wal beside the database; user bytes never
// change.
func openStateReadOnly(dir string) (*sql.DB, error) {
	path := filepath.Join(dir, journal.DBName)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("migrate: %s: %w", path, journal.ErrNoDatabase)
	} else if err != nil {
		return nil, fmt.Errorf("migrate: inspecting %s: %w", path, err)
	}
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	dsn := (&url.URL{Scheme: "file", Path: p,
		RawQuery: "mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("migrate: opening %s read-only: %w", path, err)
	}
	return db, nil
}

// receipt is the printed record of one apply: the durable phase, the
// backup the migration can restore from, and the drain outcomes.
type receipt struct {
	Phase       string   `json:"phase"`
	BackupPath  string   `json:"backup_path"`
	Drained     []string `json:"drained"`
	Adopted     []string `json:"adopted"`
	Quarantined []string `json:"quarantined"`
}

// readReceipt reads the durable phase and backup path after Apply and
// joins them with the drain report for the receipt.
func readReceipt(ctx context.Context, db *sql.DB, report migrate.DrainReport) (receipt, error) {
	r := receipt{Drained: report.Drained, Adopted: report.Adopted, Quarantined: report.Quarantined}
	err := db.QueryRowContext(ctx, `SELECT phase, backup_path FROM migration_state WHERE id = 1`).
		Scan(&r.Phase, &r.BackupPath)
	if err != nil {
		return receipt{}, fmt.Errorf("migrate: reading the migration receipt: %w", err)
	}
	return r, nil
}

// mapControlError maps a migration ControlError onto the CLI's exit
// codes: schema_too_new exits invalid (like journal.ErrSchemaTooNew) and
// ownership_unresolved exits 6 (like supervisor.ErrOwnership); anything
// else keeps the catalogue code in the message and exits internal.
func mapControlError(err error) error {
	var ce *v2contract.ControlError
	if !errors.As(err, &ce) {
		return err
	}
	switch ce.Code {
	case v2contract.CodeSchemaTooNew:
		return fmt.Errorf("%w: %w", err, journal.ErrSchemaTooNew)
	case v2contract.CodeOwnershipUnresolved:
		return fmt.Errorf("%w: %w", err, supervisor.ErrOwnership)
	default:
		return err
	}
}

// printPlan prints the preview: every run with its derived task and
// verbatim posture, the owner sites to drain, the pending schema steps
// and the backup target.
func printPlan(stdio Stdio, format string, plan preview.Plan) error {
	if format == "jsonl" {
		return json.NewEncoder(stdio.Out).Encode(struct {
			Type     string        `json:"type"`
			Runs     []preview.Run `json:"runs"`
			Owners   []string      `json:"owners"`
			Steps    []string      `json:"steps"`
			BackupTo string        `json:"backup_to"`
		}{"migrate.preview", plan.Runs, plan.Owners, plan.Steps, plan.BackupTo})
	}
	var b strings.Builder
	fmt.Fprintf(&b, "migration preview: %d runs, %d schema steps\n", len(plan.Runs), len(plan.Steps))
	for _, r := range plan.Runs {
		fmt.Fprintf(&b, "run %s -> task %s revision %d (posture %s)\n",
			r.RunID, r.TaskID, r.Revision, r.Posture)
	}
	b.WriteString("owner sites to drain:\n")
	for _, owner := range plan.Owners {
		fmt.Fprintf(&b, "  %s\n", owner)
	}
	b.WriteString("schema steps:\n")
	for _, step := range plan.Steps {
		fmt.Fprintf(&b, "  %s\n", step)
	}
	fmt.Fprintf(&b, "backup target: %s\n", plan.BackupTo)
	_, err := fmt.Fprint(stdio.Out, b.String())
	return err
}

// printReceipt prints the apply receipt: the durable phase, the backup
// path and the per-run drain outcomes.
func printReceipt(stdio Stdio, format string, r receipt) error {
	if format == "jsonl" {
		return json.NewEncoder(stdio.Out).Encode(struct {
			Type        string   `json:"type"`
			Phase       string   `json:"phase"`
			BackupPath  string   `json:"backup_path"`
			Drained     []string `json:"drained"`
			Adopted     []string `json:"adopted"`
			Quarantined []string `json:"quarantined"`
		}{"migrate.receipt", r.Phase, r.BackupPath, r.Drained, r.Adopted, r.Quarantined})
	}
	var b strings.Builder
	fmt.Fprintf(&b, "migration %s: drained %d, adopted %d, quarantined %d\n",
		r.Phase, len(r.Drained), len(r.Adopted), len(r.Quarantined))
	for _, id := range r.Drained {
		fmt.Fprintf(&b, "drained %s\n", id)
	}
	for _, id := range r.Adopted {
		fmt.Fprintf(&b, "adopted %s\n", id)
	}
	for _, id := range r.Quarantined {
		fmt.Fprintf(&b, "quarantined %s\n", id)
	}
	fmt.Fprintf(&b, "backup: %s\n", r.BackupPath)
	_, err := fmt.Fprint(stdio.Out, b.String())
	return err
}
