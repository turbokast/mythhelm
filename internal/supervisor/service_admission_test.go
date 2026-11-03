package supervisor

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	ab "github.com/turbokast/mythhelm/internal/admissionbinding"
	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

func serviceContextFixture(t *testing.T) (*ExecutionContext, *journal.Journal, *control.InstanceLease) {
	t.Helper()
	// The instance lock is process-wide, so these fixtures cannot run in parallel.
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	root := t.TempDir()

	lease, err := control.AcquireInstanceLease(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lease.Close() })
	j, err := journal.Open(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), `UPDATE migration_state SET phase='adopted' WHERE id=1`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(lease.Root()))
	content := ab.RetentionPolicyContentV1{SchemaVersion: 1, RootID: "root_" + hex.EncodeToString(sum[:]), Purpose: ab.RetentionPurposeV1, ExecutionPolicyRevision: "sha256:" + strings.Repeat("a", 64), Rules: []ab.RetentionRuleV1{}}
	sum, err = ab.RetentionPolicyDigestV1(content)
	if err != nil {
		t.Fatal(err)
	}
	policy := ab.RetentionPolicyV1{Revision: "sha256:" + hex.EncodeToString(sum[:]), Content: content}
	c, err := NewExecutionContext(t.Context(), j, lease, ExecutionContextOptions{PolicyRevision: policy.Revision, RetentionPolicy: policy})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, j, lease
}

func TestServiceExecutionContextLifetime(t *testing.T) {
	c, _, lease := serviceContextFixture(t)
	m, err := c.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	if m.RootID == "" || m.ContextEpoch == "" || m.PrincipalRef == "" || m.Generation != lease.Generation() {
		t.Fatalf("incomplete context: %+v", m)
	}
	copied := *c
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = copied.Metadata()
	var ce *v2contract.ControlError
	if !errors.As(err, &ce) || ce.Code != v2contract.CodeInvalidContract {
		t.Fatalf("closed copy error = %v", err)
	}
}

func TestServiceExecutionContextAuthority(t *testing.T) {
	c, j, lease := serviceContextFixture(t)
	options := ExecutionContextOptions{PolicyRevision: c.state.policy.Revision, RetentionPolicy: c.state.policy}
	for _, tc := range []struct {
		name    string
		journal *journal.Journal
		lease   *control.InstanceLease
		options ExecutionContextOptions
	}{
		{"missing journal", nil, lease, options},
		{"missing lease", j, nil, options},
		{"forged lease", j, &control.InstanceLease{}, options},
		{"missing actual policy", j, lease, ExecutionContextOptions{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewExecutionContext(t.Context(), tc.journal, tc.lease, tc.options)
			if err == nil || got != nil {
				t.Fatalf("unproven context = %v, %v", got, err)
			}
		})
	}
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), `UPDATE migration_state SET phase='not_started' WHERE id=1`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got, err := NewExecutionContext(t.Context(), j, lease, options)
	var ce *v2contract.ControlError
	if got != nil || !errors.As(err, &ce) || ce.Code != v2contract.CodeOwnershipUnresolved {
		t.Fatalf("unadopted context=%v, %v", got, err)
	}
}

func TestServiceContextPolicyPrivateCopy(t *testing.T) {
	c, j, lease := serviceContextFixture(t)
	content := c.state.policy.Content
	content.Rules = []ab.RetentionRuleV1{{FieldPath: "Request.Repository", Classification: ab.RetentionNonSecret, Provenance: ab.RetentionOperatorInput}}
	sum, err := ab.RetentionPolicyDigestV1(content)
	if err != nil {
		t.Fatal(err)
	}
	policy := ab.RetentionPolicyV1{Revision: "sha256:" + hex.EncodeToString(sum[:]), Content: content}
	copyContext, err := NewExecutionContext(t.Context(), j, lease, ExecutionContextOptions{PolicyRevision: policy.Revision, RetentionPolicy: policy})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = copyContext.Close() })
	policy.Content.Rules[0].Classification = ab.RetentionForbidden
	if copyContext.state.policy.Content.Rules[0].Classification != ab.RetentionNonSecret {
		t.Fatal("source policy mutated the private context")
	}
}
