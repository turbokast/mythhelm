package supervisor

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
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

func servicePlanFixture(t *testing.T) (*ServicePlan, ServiceInputs) {
	t.Helper()
	c, _, _ := serviceContextFixture(t)
	repo := t.TempDir()
	config := []byte("schema_version = 1\n[[checks]]\nname = \"synthetic\"\nargv = [\"git\", \"status\"]\ntimeout = \"5s\"\n")
	if err := os.WriteFile(filepath.Join(repo, "mythhelm.toml"), config, 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "--quiet", "--initial-branch=main"}, {"config", "user.name", "Synthetic Fixture"}, {"config", "user.email", "fixture@example.com"}, {"add", "mythhelm.toml"}, {"commit", "--quiet", "-m", "synthetic configuration"}} {
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixture repository: %v %s", err, out)
		}
	}
	taskFile := filepath.Join(t.TempDir(), "task.md")
	task := []byte("# Synthetic task\nCreate synthetic output.\n")
	if err := os.WriteFile(taskFile, task, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(task)
	m, err := c.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	in := ServiceInputs{RootID: m.RootID, PeerID: "peer_synthetic", ContextEpoch: m.ContextEpoch, PrincipalRef: m.PrincipalRef, PolicyRevision: m.PolicyRevision, Generation: m.Generation, Context: c, TaskArtifactRef: "pending/synthetic", Request: ab.RunSpecV2{SchemaVersion: 2, Repository: repo, TaskFile: taskFile, TaskSHA256: hex.EncodeToString(sum[:]), Adapter: "fake", Billing: "local-scripted", ExecutionProfile: "trusted-host", Ceilings: ab.OptionalCeilingsV1{Repairs: &zero}}}
	plan, err := PrepareService(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	return plan, in
}

func TestServiceChallengePlanExactPrivate(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*ab.BindingV1)
	}{
		{"missing required challenge", func(b *ab.BindingV1) { b.Challenges = b.Challenges[1:] }},
		{"extra required challenge", func(b *ab.BindingV1) {
			b.Challenges = append(b.Challenges, ab.ChallengeV1{ID: "challenge_extra", Kind: "project-config", Text: "Changed authority"})
		}},
		{"wrong kind", func(b *ab.BindingV1) { b.Challenges[0].Kind = "keep-going" }},
		{"changed text", func(b *ab.BindingV1) { b.Challenges[0].Text = "Changed authority" }},
		{"swapped IDs", func(b *ab.BindingV1) { b.Challenges[0].ID, b.Challenges[1].ID = b.Challenges[1].ID, b.Challenges[0].ID }},
		{"duplicate ID", func(b *ab.BindingV1) { b.Challenges[1].ID = b.Challenges[0].ID }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, _ := servicePlanFixture(t)
			s := plan.state
			raw, err := plan.CanonicalBinding()
			if err != nil {
				t.Fatal(err)
			}
			altered, err := ab.DecodeV1(raw)
			if err != nil {
				t.Fatal(err)
			}
			tc.change(&altered)
			// Keep the independent original authority metadata. Even coherent altered
			// bytes and their matching digest cannot replace that required plan.
			canonical, encodeErr := ab.EncodeV1(altered)
			if encodeErr != nil {
				canonical, err = json.Marshal(altered)
				if err != nil {
					t.Fatal(err)
				}
			}
			s.binding = altered
			s.canonical = canonical
			if sum, err := ab.DigestV1(altered); err == nil {
				s.metadata.BindingSHA256 = hex.EncodeToString(sum[:])
			}
			err = ValidateServicePlan(t.Context(), plan)
			if _, ok := errors.AsType[*v2contract.ControlError](err); !ok {
				t.Fatalf("altered authority plan accepted: %v", err)
			}
			if !s.invalid {
				t.Fatal("altered authority did not invalidate the original plan")
			}
		})
	}
	t.Run("unchanged actual producer", func(t *testing.T) {
		plan, _ := servicePlanFixture(t)
		if err := ValidateServicePlan(t.Context(), plan); err != nil {
			t.Fatal(err)
		}
		if _, err := plan.Metadata(); err != nil {
			t.Fatal(err)
		}
	})
}

// confirmedServiceFixture supplies synthetic contract authority through the
// actual closed storage producer. Registered OS-peer authority is exercised by
// the service-package tests; this helper makes no native qualification claim.
func confirmedServiceFixture(t *testing.T) (*ServicePlan, ServiceInputs, journal.ConfirmedPreparation) {
	t.Helper()
	initial, in := servicePlanFixture(t)
	raw, err := initial.CanonicalBinding()
	if err != nil {
		t.Fatal(err)
	}
	b, err := ab.DecodeV1(raw)
	if err != nil {
		t.Fatal(err)
	}
	content := in.Context.state.policy.Content
	fields, err := ab.DeriveRetentionFieldsV1(b, in.Context.state.policy)
	if err != nil {
		t.Fatal(err)
	}
	content.Rules = []ab.RetentionRuleV1{}
	for _, field := range fields {
		content.Rules = append(content.Rules, ab.RetentionRuleV1{FieldPath: field.FieldPath, Classification: ab.RetentionNonSecret, Provenance: ab.RetentionOperatorInput})
	}
	sum, err := ab.RetentionPolicyDigestV1(content)
	if err != nil {
		t.Fatal(err)
	}
	policy := ab.RetentionPolicyV1{Revision: "sha256:" + hex.EncodeToString(sum[:]), Content: content}
	old := in.Context
	fresh, err := NewExecutionContext(t.Context(), old.state.journal, old.state.lease, ExecutionContextOptions{PolicyRevision: policy.Revision, RetentionPolicy: policy})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.Close() })
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	m, err := fresh.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	in.Context = fresh
	in.ContextEpoch = m.ContextEpoch
	in.PolicyRevision = m.PolicyRevision
	plan, err := PrepareService(t.Context(), in)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = plan.CanonicalBinding()
	if err != nil {
		t.Fatal(err)
	}
	b, err = ab.DecodeV1(raw)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := plan.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	fields, err = ab.DeriveRetentionFieldsV1(b, policy)
	if err != nil {
		t.Fatal(err)
	}
	fieldSum, err := ab.RetentionFieldSetSHA256(fields)
	if err != nil {
		t.Fatal(err)
	}
	retention := b.Challenges[len(b.Challenges)-1]
	approval := ab.RetentionApprovalV1{SchemaVersion: 1, PreparationID: b.PreparationID, ChallengeID: retention.ID, BindingSHA256: metadata.BindingSHA256, FieldSetSHA256: hex.EncodeToString(fieldSum[:]), PolicyRevision: policy.Revision, PeerID: in.PeerID, RootID: in.RootID, ContextEpoch: in.ContextEpoch, Generation: in.Generation, ExpiresAt: b.ExpiresAt, ObservedAt: now.Format(time.RFC3339Nano), Decision: ab.RetentionApprove}
	if err := admission.CheckRetentionV1(b, policy, &approval, now); err != nil {
		t.Fatal(err)
	}
	lookup := journal.PreparationLookup{PreparationID: b.PreparationID, PeerID: in.PeerID, RootID: in.RootID, BindingSHA256: metadata.BindingSHA256, ContextEpoch: in.ContextEpoch, Generation: in.Generation, ObservedAt: now}
	j := fresh.state.journal
	state, err := j.StoreRetainedPreparation(t.Context(), journal.RetainedPreparationInput{Operation: journal.PreparationOperation{OperationID: "op_fixture_retention", ParamsSHA256: strings.Repeat("a", 64)}, CanonicalBinding: raw, Policy: policy, Retention: approval, Lookup: lookup})
	if err != nil {
		t.Fatal(err)
	}
	for i, ch := range b.Challenges {
		if ch.Kind == ab.RetentionChallengeKind {
			continue
		}
		lookup.ExpectedRevision = state.Revision
		lookup.ObservedAt = time.Now().UTC()
		state, err = j.ConfirmPreparation(t.Context(), lookup, journal.ConfirmationV1{Operation: journal.PreparationOperation{OperationID: fmt.Sprintf("op_fixture_answer_%d", i), ParamsSHA256: strings.Repeat("b", 64)}, ChallengeID: ch.ID, BindingSHA256: metadata.BindingSHA256, PeerID: in.PeerID, RootID: in.RootID, ContextEpoch: in.ContextEpoch, Generation: in.Generation, Decision: ab.RetentionApprove, ObservedAt: lookup.ObservedAt})
		if err != nil {
			t.Fatal(err)
		}
	}
	lookup.ExpectedRevision = state.Revision
	lookup.ObservedAt = time.Now().UTC()
	confirmed, err := j.ReadConfirmedPreparation(t.Context(), lookup)
	if err != nil {
		t.Fatal(err)
	}
	return plan, in, confirmed
}

func TestServicePermitPrivateCopy(t *testing.T) {
	plan, in, confirmed := confirmedServiceFixture(t)
	permit, err := RevalidateService(t.Context(), confirmed, in)
	if err != nil {
		t.Fatal(err)
	}
	task := string(permit.state.decision.Task.Content)
	argv := append([]string{}, permit.state.decision.Proposal.Spec.Args...)
	environment := append([]string{}, permit.state.decision.Proposal.Spec.Env...)
	check := permit.state.decision.ProjectConfig.Checks[0].Argv[0]
	repairs := permit.state.decision.EnvelopeFlags.Repairs
	plan.state.decision.Task.Content[0] = 'x'
	plan.state.decision.Proposal.Spec.Args[0] = "changed original argv"
	if len(plan.state.decision.Proposal.Spec.Env) > 0 {
		plan.state.decision.Proposal.Spec.Env[0] = "PATH=changed-original"
	}
	plan.state.decision.ProjectConfig.Checks[0].Argv[0] = "changed original check"
	plan.state.decision.Proposal.Manifest.Digests["changed"] = "changed original manifest"
	plan.state.decision.EnvelopeFlags.Repairs = 99
	*in.Request.Ceilings.Repairs = 88
	if string(permit.state.decision.Task.Content) != task || !slices.Equal(permit.state.decision.Proposal.Spec.Args, argv) || !slices.Equal(permit.state.decision.Proposal.Spec.Env, environment) || permit.state.decision.ProjectConfig.Checks[0].Argv[0] != check || permit.state.decision.EnvelopeFlags.Repairs != repairs || len(permit.state.decision.Proposal.Manifest.Digests) != 0 {
		t.Fatal("mutable source values changed the private permit")
	}
	metadata, err := permit.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	metadata.Plan.Challenges[0].Text = "changed copy"
	again, err := permit.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	if again.Plan.Challenges[0].Text == metadata.Plan.Challenges[0].Text {
		t.Fatal("permit metadata exposed private challenge state")
	}
	copyPermit := *permit
	permit.state.mu.Lock()
	permit.state.consumed = true
	permit.state.mu.Unlock()
	if _, err := copyPermit.Metadata(); err == nil {
		t.Fatal("copied permit did not share one-use state")
	}
}

func TestServiceZeroTotalRetentionRefuses(t *testing.T) {
	plan, _ := servicePlanFixture(t)
	plan.state.binding.Challenges = []ab.ChallengeV1{}
	err := ValidateServicePlan(t.Context(), plan)
	var ce *v2contract.ControlError
	if !errors.As(err, &ce) || ce.Code != v2contract.CodePolicyIneligible || ce.Detail["mythhelm.retention/reason"] != "missing_retention_authority" {
		t.Fatalf("zero-total plan=%v", err)
	}
}

func TestServiceFrozenTaskArtefactMismatch(t *testing.T) {
	plan, in, confirmed := confirmedServiceFixture(t)
	plan.state.decision.Task.Content[0] = 'x'
	if _, err := RevalidateService(t.Context(), confirmed, in); err == nil {
		t.Fatal("changed frozen task artefact minted a permit")
	}
}
