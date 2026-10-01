package journal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"
)

// DeclarationRow contains only the user assertion and hashed native identity.
// Native status does not verify the assertion or extra-usage prevention.
type DeclarationRow struct {
	AdapterID   string    `json:"adapter_id"`
	PlanClass   string    `json:"plan_class"`
	ExtraUsage  string    `json:"extra_usage"`
	IdentityRef string    `json:"identity_ref"`
	DeclaredAt  time.Time `json:"declared_at"`
}

// CurrentDeclaration returns the unsuperseded declaration for the exact
// adapter/account/config identity. It never accepts another account's row.
func (j *Journal) CurrentDeclaration(ctx context.Context, adapterID, identityRef string) (DeclarationRow, error) {
	var d DeclarationRow
	var declaredAt string
	err := j.db.QueryRowContext(ctx, `SELECT adapter_id,plan_class,extra_usage,identity_ref,declared_at FROM declarations WHERE adapter_id=? AND identity_ref=? AND superseded_at IS NULL`, adapterID, identityRef).Scan(&d.AdapterID, &d.PlanClass, &d.ExtraUsage, &d.IdentityRef, &declaredAt)
	if errors.Is(err, sql.ErrNoRows) {
		return DeclarationRow{}, fmt.Errorf("journal: entitlement declaration: %w", ErrNotFound)
	}
	if err != nil {
		return DeclarationRow{}, fmt.Errorf("journal: reading entitlement declaration: %w", err)
	}
	d.DeclaredAt, err = time.Parse(time.RFC3339Nano, declaredAt)
	if err != nil {
		return DeclarationRow{}, fmt.Errorf("journal: declaration timestamp: %w", err)
	}
	return d, nil
}

// InsertDeclaration supersedes the previous row and inserts its replacement
// inside Append's projection transaction. The unique index remains the guard
// against direct competing current inserts and concurrent declarations.
func InsertDeclaration(ctx context.Context, tx *sql.Tx, d DeclarationRow) error {
	if d.AdapterID == "" || len(d.IdentityRef) != 64 || !slices.Contains([]string{"pro", "max", "team", "enterprise"}, d.PlanClass) || d.ExtraUsage != "disabled" || d.DeclaredAt.IsZero() {
		return fmt.Errorf("journal: invalid entitlement declaration")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE declarations SET superseded_at=? WHERE adapter_id=? AND identity_ref=? AND superseded_at IS NULL`, formatTime(d.DeclaredAt), d.AdapterID, d.IdentityRef); err != nil {
		return fmt.Errorf("journal: superseding entitlement declaration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO declarations (adapter_id,plan_class,extra_usage,identity_ref,declared_at) VALUES (?,?,?,?,?)`, d.AdapterID, d.PlanClass, d.ExtraUsage, d.IdentityRef, formatTime(d.DeclaredAt)); err != nil {
		return fmt.Errorf("journal: declaring entitlement: %w", err)
	}
	return nil
}
