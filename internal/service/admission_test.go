package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	ab "github.com/turbokast/mythhelm/internal/admissionbinding"
	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

func admissionIntent(t *testing.T, r *runtime, method, operation, object string, revision int64, body any) control.Intent {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	generation := r.lease.Generation()
	return control.Intent{OperationID: operation, Object: object, ExpectedRevision: revision, Generation: &generation, Method: method, Params: raw}
}
func admissionRowCounts(t *testing.T, r *runtime) string {
	t.Helper()
	var counts []int
	err := r.journal.Transact(t.Context(), func(tx *sql.Tx) error {
		for _, table := range []string{"operations", "prepared_runs", "service_admission_bindings", "trust_grants", "runs", "attempts"} {
			var count int
			if err := tx.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil {
				return err
			}
			counts = append(counts, count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprint(counts)
}
func requireAdmissionCode(t *testing.T, err error, code control.Code) {
	t.Helper()
	var ce *control.Error
	if !errors.As(err, &ce) || ce.Code != code {
		t.Fatalf("admission error = %v, want %s", err, code)
	}
}
func requestFixture(t *testing.T) control.RunSpec {
	t.Helper()
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "--quiet", "--initial-branch=main"}, {"config", "user.name", "Synthetic Fixture"}, {"config", "user.email", "fixture@example.com"}, {"commit", "--allow-empty", "--quiet", "-m", "synthetic fixture"}} {
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixture git: %v: %s", err, out)
		}
	}
	task := filepath.Join(t.TempDir(), "task.md")
	content := []byte("# Synthetic task\nCreate synthetic evidence.\n")
	if err := os.WriteFile(task, content, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	return control.RunSpec{SchemaVersion: 2, Repository: repo, TaskFile: task, TaskSHA256: hex.EncodeToString(sum[:]), Adapter: "fake", Billing: "local-scripted", ExecutionProfile: "trusted-host", NoChecks: true, Scenario: "happy"}
}

func TestServiceAdmissionAuthority(t *testing.T) {
	r := testRuntime(t)
	before := admissionRowCounts(t, r)
	spec := control.RunSpec{SchemaVersion: 2, Repository: filepath.Join(t.TempDir(), "repo"), TaskFile: filepath.Join(t.TempDir(), "task.md"), TaskSHA256: strings.Repeat("a", 64), Adapter: "fake", Billing: "local-scripted", ExecutionProfile: "trusted-host", NoChecks: true}
	original := admissionIntent(t, r, "run.prepare", "op_authority", "", 0, spec)
	for _, tc := range []struct {
		name   string
		peer   control.Peer
		change func(*control.Intent)
		code   control.Code
	}{
		{"attachment token", control.Peer{SameUser: true}, func(in *control.Intent) { in.CapabilityToken = "synthetic-attachment-token" }, control.CodePermissionDenied},
		{"foreign OS peer", control.Peer{}, func(*control.Intent) {}, control.CodePermissionDenied},
		{"changed generation", control.Peer{SameUser: true}, func(in *control.Intent) { v := *in.Generation + 1; in.Generation = &v }, control.CodeRevisionConflict},
		{"missing generation", control.Peer{SameUser: true}, func(in *control.Intent) { in.Generation = nil }, control.CodeRevisionConflict},
		{"caller environment", control.Peer{SameUser: true}, func(in *control.Intent) {
			in.Params = append(in.Params[:len(in.Params)-1], []byte(`,"env":{"PATH":"caller-sentinel"}}`)...)
		}, control.CodeInvalidContract},
		{"host cannot prepare", control.Peer{SameUser: true}, func(in *control.Intent) { bad := spec; bad.Host = "herdr"; in.Params, _ = json.Marshal(bad) }, control.CodeInvalidContract},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := original
			in.Params = append(json.RawMessage{}, original.Params...)
			tc.change(&in)
			result, err := r.server.Dispatch(t.Context(), tc.peer, in)
			requireAdmissionCode(t, err, tc.code)
			if len(result.Body) != 0 {
				t.Fatal("refusal disclosed a preparation")
			}
			if got := admissionRowCounts(t, r); got != before {
				t.Fatalf("authority refusal changed rows: %s -> %s", before, got)
			}
		})
	}
	// The production registration exists even without an actual configured policy.
	result, err := r.server.Dispatch(t.Context(), control.Peer{SameUser: true}, original)
	requireAdmissionCode(t, err, control.CodeCapabilityUnsupported)
	if len(result.Body) != 0 || admissionRowCounts(t, r) != before {
		t.Fatal("unconfigured policy retained a preparation")
	}
}

func configureSyntheticAdmission(t *testing.T, r *runtime, spec control.RunSpec) {
	t.Helper()
	sum := sha256.Sum256([]byte(r.lease.Root()))
	content := ab.RetentionPolicyContentV1{SchemaVersion: 1, RootID: "root_" + hex.EncodeToString(sum[:]), Purpose: ab.RetentionPurposeV1, ExecutionPolicyRevision: "sha256:" + strings.Repeat("a", 64), Rules: []ab.RetentionRuleV1{}}
	seal := func(content ab.RetentionPolicyContentV1) ab.RetentionPolicyV1 {
		t.Helper()
		sum, err := ab.RetentionPolicyDigestV1(content)
		if err != nil {
			t.Fatal(err)
		}
		return ab.RetentionPolicyV1{Revision: "sha256:" + hex.EncodeToString(sum[:]), Content: content}
	}
	policy := seal(content)
	build := func(policy ab.RetentionPolicyV1) *supervisor.ExecutionContext {
		t.Helper()
		c, err := supervisor.NewExecutionContext(t.Context(), r.journal, r.lease, supervisor.ExecutionContextOptions{PolicyRevision: policy.Revision, RetentionPolicy: policy})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	c := build(policy)
	metadata, err := c.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	request, err := admissionRequest(spec)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := supervisor.PrepareService(t.Context(), supervisor.ServiceInputs{RootID: metadata.RootID, PeerID: "peer_synthetic", ContextEpoch: metadata.ContextEpoch, PrincipalRef: metadata.PrincipalRef, PolicyRevision: metadata.PolicyRevision, Generation: metadata.Generation, Request: request, TaskArtifactRef: "pending/synthetic", Context: c})
	if err != nil {
		_ = c.Close()
		t.Fatalf("actual staged producer must create a coherent binding before synthetic policy classification: %v", err)
	}
	raw, err := plan.CanonicalBinding()
	if err != nil {
		t.Fatal(err)
	}
	binding, err := ab.DecodeV1(raw)
	if err != nil {
		t.Fatal(err)
	}
	fields, err := ab.DeriveRetentionFieldsV1(binding, policy)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range fields {
		content.Rules = append(content.Rules, ab.RetentionRuleV1{FieldPath: field.FieldPath, Classification: ab.RetentionNonSecret, Provenance: ab.RetentionOperatorInput})
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	policy = seal(content)
	r.preparations.context = build(policy)
	r.preparations.policy = policy
}

// admissionFramedConn runs the actual registered server through the same
// frame/result codecs as the revision-aware client; it supplies no handler fake.
type admissionFramedConn struct {
	runtime *runtime
	peer    control.Peer
}

func (c *admissionFramedConn) Peer() control.Peer { return c.peer }
func (c *admissionFramedConn) Request(ctx context.Context, frame []byte) ([]byte, error) {
	in, err := control.Decode[control.Intent](frame)
	if err != nil {
		return nil, err
	}
	result, err := c.runtime.server.Dispatch(ctx, c.peer, in)
	result.OperationID = in.OperationID
	if err != nil {
		var ce *control.Error
		if !errors.As(err, &ce) {
			return nil, err
		}
		result.Error = ce
	}
	return control.Encode(result)
}
func (c *admissionFramedConn) Receive(context.Context) ([]byte, error) {
	return nil, errors.New("fixture has no receive loop")
}
func (c *admissionFramedConn) Respond([]byte) error { return errors.New("fixture has no respond loop") }
func (c *admissionFramedConn) Close() error         { return nil }

func TestServicePreparedOuterRevisions(t *testing.T) {
	r := testRuntime(t)
	spec := retentionOnlyRequestFixture(t)
	configureSyntheticAdmission(t, r, spec)
	client := control.NewPreparedRunClient(&admissionFramedConn{runtime: r, peer: control.Peer{SameUser: true, PID: 77}})
	before := admissionRowCounts(t, r)
	prepared, err := client.PrepareRevisioned(t.Context(), "op_prepared", spec)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Revision != 0 || prepared.Run.Ready || len(prepared.Run.Challenges) != 1 {
		t.Fatalf("initial preparation=%+v", prepared)
	}
	if got := admissionRowCounts(t, r); got != before {
		t.Fatalf("unapproved prepare wrote rows: %s -> %s", before, got)
	}
	challenge := prepared.Run.Challenges[0]
	answer := control.RunConfirmation{SchemaVersion: 2, PreparationID: prepared.Run.PreparationID, ChallengeID: challenge.ID, BindingSHA256: prepared.Run.BindingSHA256, Approved: true}
	confirmed, err := client.ConfirmRevisioned(t.Context(), "op_retention", 0, answer)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Revision != 1 || !confirmed.Run.Ready {
		t.Fatalf("retention-only readiness=%+v", confirmed)
	}
	replay, err := client.ConfirmRevisioned(t.Context(), "op_retention", 0, answer)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Revision != confirmed.Revision || replay.Run.Ready != confirmed.Run.Ready {
		t.Fatalf("historical replay changed: %+v", replay)
	}
	if _, err := client.Start(t.Context(), "op_start", confirmed.Revision, control.StartRun{SchemaVersion: 2, PreparationID: prepared.Run.PreparationID, BindingSHA256: prepared.Run.BindingSHA256}); err == nil {
		t.Fatal("start became available before its dispatch producer")
	}
}

func TestServicePlanMetadataCopies(t *testing.T) {
	r := testRuntime(t)
	spec := requestFixture(t)
	configureSyntheticAdmission(t, r, spec)
	request := admissionIntent(t, r, "run.prepare", "op_metadata", "", 0, spec)
	result, err := r.server.Dispatch(t.Context(), control.Peer{SameUser: true}, request)
	if err != nil {
		t.Fatal(err)
	}
	var reply control.PreparedRun
	if err := json.Unmarshal(result.Body, &reply); err != nil {
		t.Fatal(err)
	}
	plan := r.preparations.plans[reply.PreparationID].plan
	metadata, err := plan.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := plan.CanonicalBinding()
	if err != nil {
		t.Fatal(err)
	}
	metadata.Challenges[0].Text = "changed caller copy"
	canonical[0] = 'x'
	again, err := plan.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := plan.CanonicalBinding()
	if err != nil {
		t.Fatal(err)
	}
	if again.Challenges[0].Text == metadata.Challenges[0].Text || bytes[0] != '{' {
		t.Fatal("copy getters exposed private state")
	}
	var nilPlan *supervisor.ServicePlan
	if _, err := nilPlan.Metadata(); err == nil {
		t.Fatal("nil plan returned metadata")
	}
	if err := r.preparations.context.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := plan.CanonicalBinding(); err == nil {
		t.Fatal("closed context left a usable plan")
	}
}

func TestServiceAdmissionBinding(t *testing.T) {
	r := testRuntime(t)
	spec := requestFixture(t)
	configureSyntheticAdmission(t, r, spec)
	in := admissionIntent(t, r, "run.prepare", "op_binding", "", 0, spec)
	result, err := r.server.Dispatch(t.Context(), control.Peer{SameUser: true}, in)
	if err != nil {
		t.Fatal(err)
	}
	var prepared control.PreparedRun
	if err := json.Unmarshal(result.Body, &prepared); err != nil {
		t.Fatal(err)
	}
	before := admissionRowCounts(t, r)
	if err := os.WriteFile(spec.TaskFile, []byte("# Changed task\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ch := prepared.Challenges[0]
	answer := control.RunConfirmation{SchemaVersion: 2, PreparationID: prepared.PreparationID, ChallengeID: ch.ID, BindingSHA256: prepared.BindingSHA256, Approved: true}
	confirm := admissionIntent(t, r, "run.confirm", "op_changed_task", prepared.PreparationID, 0, answer)
	result, err = r.server.Dispatch(t.Context(), control.Peer{SameUser: true}, confirm)
	requireAdmissionCode(t, err, control.Code("dependency_stale"))
	if len(result.Body) != 0 || admissionRowCounts(t, r) != before {
		t.Fatal("changed task granted or persisted admission")
	}
}

func TestServiceRetentionConfirmationOrder(t *testing.T) {
	r := testRuntime(t)
	spec := projectRequestFixture(t)
	configureSyntheticAdmission(t, r, spec)
	client := control.NewPreparedRunClient(&admissionFramedConn{runtime: r, peer: control.Peer{SameUser: true, PID: 77}})
	prepared, err := client.PrepareRevisioned(t.Context(), "op_order_prepare", spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Run.Challenges) != 2 {
		t.Fatalf("required challenges=%+v", prepared.Run.Challenges)
	}
	byKind := map[string]control.AdmissionChallenge{}
	for _, ch := range prepared.Run.Challenges {
		byKind[ch.Kind] = ch
	}
	answer := func(ch control.AdmissionChallenge) control.RunConfirmation {
		return control.RunConfirmation{SchemaVersion: 2, PreparationID: prepared.Run.PreparationID, ChallengeID: ch.ID, BindingSHA256: prepared.Run.BindingSHA256, Approved: true}
	}
	before := admissionRowCounts(t, r)
	_, err = client.ConfirmRevisioned(t.Context(), "op_wrong_order", 0, answer(byKind["project-config"]))
	requireAdmissionCode(t, err, control.Code("policy_ineligible"))
	if admissionRowCounts(t, r) != before {
		t.Fatal("wrong-order confirmation persisted state")
	}
	retained, err := client.ConfirmRevisioned(t.Context(), "op_order_retention", 0, answer(byKind[ab.RetentionChallengeKind]))
	if err != nil {
		t.Fatal(err)
	}
	if retained.Revision != 1 || retained.Run.Ready {
		t.Fatalf("retention prematurely ready=%+v", retained)
	}
	final, err := client.ConfirmRevisioned(t.Context(), "op_order_project", retained.Revision, answer(byKind["project-config"]))
	if err != nil {
		t.Fatal(err)
	}
	if final.Revision != 2 || !final.Run.Ready {
		t.Fatalf("final confirmation not ready=%+v", final)
	}
	entry := r.preparations.plans[prepared.Run.PreparationID]
	metadata, err := entry.plan.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	lookup := journal.PreparationLookup{PreparationID: metadata.PreparationID, PeerID: metadata.PeerID, RootID: metadata.RootID, BindingSHA256: metadata.BindingSHA256, ContextEpoch: metadata.ContextEpoch, Generation: metadata.Generation, ExpectedRevision: final.Revision, ObservedAt: time.Now().UTC()}
	confirmed, err := r.journal.ReadConfirmedPreparation(t.Context(), lookup)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := supervisor.RevalidateService(t.Context(), confirmed, entry.inputs)
	if err != nil {
		t.Fatal(err)
	}
	pm, err := permit.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	if pm.Plan.PreparationID != prepared.Run.PreparationID || pm.PermitNonceSHA256 == "" {
		t.Fatalf("permit identity=%+v", pm)
	}
	var launches int
	if err := r.journal.Transact(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRowContext(t.Context(), `SELECT count(*) FROM attempts`).Scan(&launches)
	}); err != nil {
		t.Fatal(err)
	}
	if launches != 0 {
		t.Fatal("preparation launched an attempt")
	}
}

func TestServiceAdmissionChallengePlanExact(t *testing.T) {
	r := testRuntime(t)
	spec := retentionOnlyRequestFixture(t)
	configureSyntheticAdmission(t, r, spec)
	peer := control.Peer{SameUser: true, PID: 77}
	result, err := r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.prepare", "op_exact_prepare", "", 0, spec))
	if err != nil {
		t.Fatal(err)
	}
	var prepared control.PreparedRun
	if err := json.Unmarshal(result.Body, &prepared); err != nil {
		t.Fatal(err)
	}
	before := admissionRowCounts(t, r)
	base := control.RunConfirmation{SchemaVersion: 2, PreparationID: prepared.PreparationID, ChallengeID: prepared.Challenges[0].ID, BindingSHA256: prepared.BindingSHA256, Approved: true}
	for _, tc := range []struct {
		name   string
		change func(*control.RunConfirmation)
	}{
		{"foreign challenge", func(a *control.RunConfirmation) { a.ChallengeID = "challenge_foreign" }},
		{"changed binding with coherent digest shape", func(a *control.RunConfirmation) { a.BindingSHA256 = strings.Repeat("b", 64) }},
		{"foreign preparation", func(a *control.RunConfirmation) { a.PreparationID = "prep_foreign" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			answer := base
			tc.change(&answer)
			out, err := r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.confirm", "op_exact_"+strings.ReplaceAll(tc.name, " ", "_"), answer.PreparationID, 0, answer))
			if err == nil || len(out.Body) != 0 {
				t.Fatalf("altered authority acknowledged: %s %v", out.Body, err)
			}
			if admissionRowCounts(t, r) != before {
				t.Fatal("altered authority wrote a durable record")
			}
		})
	}
	// Only the unchanged exact answer may cross the approved-retention boundary.
	out, err := r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.confirm", "op_exact_control", base.PreparationID, 0, base))
	if err != nil {
		t.Fatal(err)
	}
	var controlReply control.PreparedRun
	if err := json.Unmarshal(out.Body, &controlReply); err != nil {
		t.Fatal(err)
	}
	if out.Revision != 1 || !controlReply.Ready {
		t.Fatalf("exact authority failed readiness=%s", out.Body)
	}
}

func TestServicePreparationCapacity(t *testing.T) {
	r := testRuntime(t)
	spec := requestFixture(t)
	configureSyntheticAdmission(t, r, spec)
	before := admissionRowCounts(t, r)
	for peerIndex := range 4 {
		peer := control.Peer{SameUser: true, PID: int32(100 + peerIndex)}
		for i := range 8 {
			_, err := r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.prepare", fmt.Sprintf("op_capacity_%d_%d", peerIndex, i), "", 0, spec))
			if err != nil {
				t.Fatalf("bounded preparation %d/%d: %v", peerIndex, i, err)
			}
		}
		_, err := r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.prepare", fmt.Sprintf("op_peer_overflow_%d", peerIndex), "", 0, spec))
		requireAdmissionCode(t, err, control.CodeAllowanceExhausted)
	}
	_, err := r.server.Dispatch(t.Context(), control.Peer{SameUser: true, PID: 999}, admissionIntent(t, r, "run.prepare", "op_total_overflow", "", 0, spec))
	requireAdmissionCode(t, err, control.CodeAllowanceExhausted)
	if len(r.preparations.plans) != 32 || len(r.preparations.operations) != 32 {
		t.Fatalf("unbounded retained RAM plans=%d operations=%d", len(r.preparations.plans), len(r.preparations.operations))
	}
	if admissionRowCounts(t, r) != before {
		t.Fatal("unapproved capacity fixture persisted or launched")
	}
}

func TestPreparedAdmissionVolatileDispatch(t *testing.T) {
	r := testRuntime(t)
	before := admissionRowCounts(t, r)
	spec := requestFixture(t)
	client := control.NewPreparedRunClient(&admissionFramedConn{runtime: r, peer: control.Peer{SameUser: true, PID: 77}})
	prepared, err := client.PrepareRevisioned(t.Context(), "op_no_policy", spec)
	requireAdmissionCode(t, err, control.CodeCapabilityUnsupported)
	if prepared.Run.PreparationID != "" || admissionRowCounts(t, r) != before {
		t.Fatal("unapproved framed preparation persisted state or disclosed a plan")
	}
	// A closed standalone client cannot use its historical nonrevisioned API.
	_, err = client.Prepare(t.Context(), spec)
	requireAdmissionCode(t, err, control.CodeCapabilityUnsupported)
	if len(r.preparations.plans) != 0 || len(r.preparations.operations) != 0 {
		t.Fatal("unconfigured policy retained volatile preparation bytes")
	}
}

func TestPreparedClientActualMigrationRefusal(t *testing.T) {
	root := isolatedRoot(t)
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	j, err := journal.Open(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, journal.DBName)
	before, err := os.ReadFile(dbPath) // #nosec G304 -- Fixed database name within this isolated fixture root.
	if err != nil {
		t.Fatal(err)
	}
	r, err := openRuntime(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := r.shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if !r.migrationRequired || r.journal != nil {
		t.Fatal("actual status fixture unexpectedly acquired a writer")
	}
	spec := requestFixture(t)
	client := control.NewPreparedRunClient(&admissionFramedConn{runtime: r, peer: control.Peer{SameUser: true, PID: 77}})
	result, err := client.PrepareRevisioned(t.Context(), "op_migration_blocked", spec)
	var ce *control.Error
	if !errors.As(err, &ce) || ce.Code != control.CodeCapabilityUnsupported || ce.Message != "migration_required" {
		t.Fatalf("actual migration status refusal=%v", err)
	}
	after, err := os.ReadFile(dbPath) // #nosec G304 -- Same isolated fixture database, checked for refusal effects.
	if err != nil {
		t.Fatal(err)
	}
	if result.Run.PreparationID != "" || !bytes.Equal(before, after) || len(r.preparations.plans) != 0 {
		t.Fatal("migration refusal retained or changed preparation state")
	}
}

func prepareFixtureRun(t *testing.T, r *runtime, spec control.RunSpec, operation string) (control.PreparedRun, control.Peer) {
	t.Helper()
	peer := control.Peer{SameUser: true, PID: 77}
	result, err := r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.prepare", operation, "", 0, spec))
	if err != nil {
		t.Fatal(err)
	}
	var prepared control.PreparedRun
	if err := json.Unmarshal(result.Body, &prepared); err != nil {
		t.Fatal(err)
	}
	return prepared, peer
}
func retentionAnswer(p control.PreparedRun) control.RunConfirmation {
	for _, ch := range p.Challenges {
		if ch.Kind == ab.RetentionChallengeKind {
			return control.RunConfirmation{SchemaVersion: 2, PreparationID: p.PreparationID, ChallengeID: ch.ID, BindingSHA256: p.BindingSHA256, Approved: true}
		}
	}
	return control.RunConfirmation{}
}

func TestServiceAdmissionBindingAllInputs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*testing.T, *preparationEntry, control.RunSpec)
	}{
		{"repository", func(t *testing.T, e *preparationEntry, _ control.RunSpec) { e.inputs.Request.Repository = t.TempDir() }},
		{"revision", func(_ *testing.T, e *preparationEntry, _ control.RunSpec) { e.inputs.Request.Revision = "HEAD~1" }},
		{"task digest", func(_ *testing.T, e *preparationEntry, _ control.RunSpec) {
			e.inputs.Request.TaskSHA256 = strings.Repeat("b", 64)
		}},
		{"task bytes", func(t *testing.T, _ *preparationEntry, s control.RunSpec) {
			if err := os.WriteFile(s.TaskFile, []byte("# Changed task\n"), 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"no checks", func(_ *testing.T, e *preparationEntry, _ control.RunSpec) { e.inputs.Request.NoChecks = false }},
		{"project trust", func(_ *testing.T, e *preparationEntry, _ control.RunSpec) {
			e.inputs.Request.TrustProjectConfig = "sha256:" + strings.Repeat("c", 64)
		}},
		{"native trust", func(_ *testing.T, e *preparationEntry, _ control.RunSpec) {
			e.inputs.Request.TrustNativeConfig = "sha256:" + strings.Repeat("c", 64)
		}},
		{"route", func(_ *testing.T, e *preparationEntry, _ control.RunSpec) { e.inputs.Request.Adapter = "claudecode" }},
		{"profile", func(_ *testing.T, e *preparationEntry, _ control.RunSpec) {
			e.inputs.Request.ExecutionProfile = "restricted"
		}},
		{"billing", func(_ *testing.T, e *preparationEntry, _ control.RunSpec) {
			e.inputs.Request.Billing = "subscription-only"
		}},
		{"envelope", func(_ *testing.T, e *preparationEntry, _ control.RunSpec) {
			v := int64(0)
			e.inputs.Request.Ceilings.Repairs = &v
		}},
		{"task artefact", func(_ *testing.T, e *preparationEntry, _ control.RunSpec) {
			e.inputs.TaskArtifactRef = "pending/changed"
		}},
		{"repository evidence", func(t *testing.T, _ *preparationEntry, s control.RunSpec) {
			cmd := exec.CommandContext(t.Context(), "git", "commit", "--allow-empty", "--quiet", "-m", "changed source")
			cmd.Dir = s.Repository
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("source change: %v %s", err, out)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := testRuntime(t)
			spec := requestFixture(t)
			configureSyntheticAdmission(t, r, spec)
			prepared, peer := prepareFixtureRun(t, r, spec, "op_all_inputs")
			before := admissionRowCounts(t, r)
			tc.change(t, r.preparations.plans[prepared.PreparationID], spec)
			result, err := r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.confirm", "op_changed_inputs", prepared.PreparationID, 0, retentionAnswer(prepared)))
			requireAdmissionCode(t, err, control.Code("dependency_stale"))
			if len(result.Body) != 0 || admissionRowCounts(t, r) != before {
				t.Fatal("changed preparation input produced effects")
			}
		})
	}
}

func TestServiceAdmissionEarliestEffectBarriers(t *testing.T) {
	for _, stage := range []string{"prepare acknowledgement", "retention persistence"} {
		t.Run(stage, func(t *testing.T) {
			r := testRuntime(t)
			spec := requestFixture(t)
			configureSyntheticAdmission(t, r, spec)
			var prepared control.PreparedRun
			peer := control.Peer{SameUser: true, PID: 77}
			if stage == "retention persistence" {
				prepared, peer = prepareFixtureRun(t, r, spec, "op_barrier_prepare")
			}
			before := admissionRowCounts(t, r)
			barriers := 0
			r.preparations.beforeEffect = func() {
				barriers++
				if err := os.WriteFile(spec.TaskFile, []byte("# Changed at the earliest effect\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			intent := admissionIntent(t, r, "run.prepare", "op_barrier_prepare", "", 0, spec)
			if stage == "retention persistence" {
				intent = admissionIntent(t, r, "run.confirm", "op_barrier_confirm", prepared.PreparationID, 0, retentionAnswer(prepared))
			}
			result, err := r.server.Dispatch(t.Context(), peer, intent)
			requireAdmissionCode(t, err, control.Code("dependency_stale"))
			if barriers != 1 || len(result.Body) != 0 || admissionRowCounts(t, r) != before {
				t.Fatalf("earliest effect was not fenced: barriers=%d body=%s", barriers, result.Body)
			}
		})
	}
}

func TestServiceRetentionOnlyReadiness(t *testing.T) {
	r := testRuntime(t)
	spec := retentionOnlyRequestFixture(t)
	configureSyntheticAdmission(t, r, spec)
	prepared, peer := prepareFixtureRun(t, r, spec, "op_ready_prepare")
	answer := retentionAnswer(prepared)
	firstIntent := admissionIntent(t, r, "run.confirm", "op_ready_retention", prepared.PreparationID, 0, answer)
	first, err := r.server.Dispatch(t.Context(), peer, firstIntent)
	if err != nil {
		t.Fatal(err)
	}
	var body control.PreparedRun
	if err := json.Unmarshal(first.Body, &body); err != nil {
		t.Fatal(err)
	}
	if first.Revision != 1 || !body.Ready {
		t.Fatalf("retention-only readiness=%s revision=%d", first.Body, first.Revision)
	}
	before := admissionRowCounts(t, r)
	// A completed operation remains an immutable historical acknowledgement
	// after source drift; a new operation cannot reuse it as fresh authority.
	if err := os.WriteFile(spec.TaskFile, []byte("# Changed after committed confirmation\n"), 0600); err != nil {
		t.Fatal(err)
	}
	replay, err := r.server.Dispatch(t.Context(), peer, firstIntent)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Revision != first.Revision || !bytes.Equal(replay.Body, first.Body) {
		t.Fatal("historical readiness acknowledgement changed")
	}
	_, err = r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.confirm", "op_ready_new", prepared.PreparationID, 1, answer))
	requireAdmissionCode(t, err, control.Code("dependency_stale"))
	if admissionRowCounts(t, r) != before {
		t.Fatal("source drift produced fresh confirmation effects")
	}
}

func TestRetentionBeforePersistence(t *testing.T) {
	for _, stage := range []string{"preparation row", "binding row", "operation acknowledgement"} {
		t.Run(stage, func(t *testing.T) {
			r := testRuntime(t)
			spec := retentionOnlyRequestFixture(t)
			configureSyntheticAdmission(t, r, spec)
			prepared, peer := prepareFixtureRun(t, r, spec, "op_retention_atomic_prepare")
			before := admissionRowCounts(t, r)
			statement := map[string]string{
				"preparation row":           `CREATE TRIGGER deny_retention BEFORE INSERT ON prepared_runs BEGIN SELECT RAISE(ABORT,'synthetic preparation failure'); END`,
				"binding row":               `CREATE TRIGGER deny_retention BEFORE INSERT ON service_admission_bindings BEGIN SELECT RAISE(ABORT,'synthetic binding failure'); END`,
				"operation acknowledgement": `CREATE TRIGGER deny_retention BEFORE UPDATE ON operations WHEN NEW.state='done' BEGIN SELECT RAISE(ABORT,'synthetic acknowledgement failure'); END`,
			}[stage]
			if err := r.journal.Transact(t.Context(), func(tx *sql.Tx) error { _, err := tx.ExecContext(t.Context(), statement); return err }); err != nil {
				t.Fatal(err)
			}
			in := admissionIntent(t, r, "run.confirm", "op_retention_atomic", prepared.PreparationID, 0, retentionAnswer(prepared))
			result, err := r.server.Dispatch(t.Context(), peer, in)
			if err == nil || len(result.Body) != 0 || admissionRowCounts(t, r) != before || len(r.preparations.confirmations) != 0 || r.preparations.plans[prepared.PreparationID].retained {
				t.Fatalf("failed retention transaction leaked effects: %v", err)
			}
			if err := r.journal.Transact(t.Context(), func(tx *sql.Tx) error {
				_, err := tx.ExecContext(t.Context(), `DROP TRIGGER deny_retention`)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			result, err = r.server.Dispatch(t.Context(), peer, in)
			if err != nil {
				t.Fatal(err)
			}
			var ready control.PreparedRun
			if err := json.Unmarshal(result.Body, &ready); err != nil {
				t.Fatal(err)
			}
			if result.Revision != 1 || !ready.Ready {
				t.Fatalf("restored retention producer not ready=%s", result.Body)
			}
		})
	}
}

func projectRequestFixture(t *testing.T) control.RunSpec {
	t.Helper()
	spec := requestFixture(t)
	config := []byte("schema_version = 1\n[[checks]]\nname = \"synthetic\"\nargv = [\"git\", \"status\", \"--porcelain\"]\ntimeout = \"5s\"\n")
	if err := os.WriteFile(filepath.Join(spec.Repository, "mythhelm.toml"), config, 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "mythhelm.toml"}, {"commit", "--quiet", "-m", "synthetic checks"}} {
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = spec.Repository
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixture config commit: %v %s", err, out)
		}
	}
	spec.NoChecks = false
	return spec
}
func preparationImage(t *testing.T, r *runtime, id string) string {
	t.Helper()
	var revision int64
	var ready int
	var answers string
	if err := r.journal.Transact(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRowContext(t.Context(), `SELECT revision,ready,confirmations FROM prepared_runs WHERE preparation_id=?`, id).Scan(&revision, &ready, &answers)
	}); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%d/%d/%s", revision, ready, answers)
}

func TestServiceRemainingConfirmationAtomic(t *testing.T) {
	for _, stage := range []string{"answer row", "operation acknowledgement", "source barrier"} {
		t.Run(stage, func(t *testing.T) {
			r := testRuntime(t)
			spec := projectRequestFixture(t)
			configureSyntheticAdmission(t, r, spec)
			prepared, peer := prepareFixtureRun(t, r, spec, "op_extra_atomic_prepare")
			retained, err := r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.confirm", "op_extra_atomic_retain", prepared.PreparationID, 0, retentionAnswer(prepared)))
			if err != nil {
				t.Fatal(err)
			}
			if retained.Revision != 1 {
				t.Fatal("retention did not establish revision one")
			}
			var project control.AdmissionChallenge
			for _, ch := range prepared.Challenges {
				if ch.Kind == "project-config" {
					project = ch
				}
			}
			answer := control.RunConfirmation{SchemaVersion: 2, PreparationID: prepared.PreparationID, ChallengeID: project.ID, BindingSHA256: prepared.BindingSHA256, Approved: true}
			in := admissionIntent(t, r, "run.confirm", "op_extra_atomic_answer", prepared.PreparationID, 1, answer)
			before, image := admissionRowCounts(t, r), preparationImage(t, r, prepared.PreparationID)
			if stage == "source barrier" {
				r.preparations.beforeEffect = func() {
					if err := os.WriteFile(filepath.Join(spec.Repository, "mythhelm.toml"), []byte("# changed at final authority barrier\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				statement := map[string]string{"answer row": `CREATE TRIGGER deny_answer BEFORE UPDATE ON prepared_runs BEGIN SELECT RAISE(ABORT,'synthetic answer failure'); END`, "operation acknowledgement": `CREATE TRIGGER deny_answer BEFORE UPDATE ON operations WHEN NEW.state='done' BEGIN SELECT RAISE(ABORT,'synthetic answer acknowledgement failure'); END`}[stage]
				if err := r.journal.Transact(t.Context(), func(tx *sql.Tx) error { _, err := tx.ExecContext(t.Context(), statement); return err }); err != nil {
					t.Fatal(err)
				}
			}
			result, err := r.server.Dispatch(t.Context(), peer, in)
			if err == nil || len(result.Body) != 0 || admissionRowCounts(t, r) != before || preparationImage(t, r, prepared.PreparationID) != image || len(r.preparations.confirmations) != 1 {
				t.Fatalf("failed answer leaked row/readiness/ack effects: %v", err)
			}
			if stage == "source barrier" {
				requireAdmissionCode(t, err, control.Code("dependency_stale"))
				return
			}
			if err := r.journal.Transact(t.Context(), func(tx *sql.Tx) error { _, err := tx.ExecContext(t.Context(), `DROP TRIGGER deny_answer`); return err }); err != nil {
				t.Fatal(err)
			}
			result, err = r.server.Dispatch(t.Context(), peer, in)
			if err != nil {
				t.Fatal(err)
			}
			var final control.PreparedRun
			if err := json.Unmarshal(result.Body, &final); err != nil {
				t.Fatal(err)
			}
			if result.Revision != 2 || !final.Ready {
				t.Fatalf("restored answer not ready=%s", result.Body)
			}
		})
	}
}

func retentionOnlyRequestFixture(t *testing.T) control.RunSpec {
	t.Helper()
	spec := projectRequestFixture(t)
	config, err := os.ReadFile(filepath.Join(spec.Repository, admission.ProjectConfigFile)) // #nosec G304 -- Fixed committed configuration within this fixture repository.
	if err != nil {
		t.Fatal(err)
	}
	_, digest, err := admission.ParseProjectConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	spec.TrustProjectConfig = "sha256:" + digest
	return spec
}

func TestServiceNoChecksRequiresExactAnswer(t *testing.T) {
	r := testRuntime(t)
	spec := requestFixture(t)
	configureSyntheticAdmission(t, r, spec)
	prepared, peer := prepareFixtureRun(t, r, spec, "op_no_checks_prepare")
	var noChecks control.AdmissionChallenge
	for _, ch := range prepared.Challenges {
		if ch.Kind == "no-checks" {
			noChecks = ch
		}
	}
	if noChecks.ID == "" || len(prepared.Challenges) != 2 {
		t.Fatalf("explicit no-checks challenge missing: %+v", prepared.Challenges)
	}
	retained, err := r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.confirm", "op_no_checks_retain", prepared.PreparationID, 0, retentionAnswer(prepared)))
	if err != nil {
		t.Fatal(err)
	}
	var intermediate control.PreparedRun
	if err := json.Unmarshal(retained.Body, &intermediate); err != nil {
		t.Fatal(err)
	}
	if intermediate.Ready || retained.Revision != 1 {
		t.Fatal("retention alone approved no-checks authority")
	}
	answer := control.RunConfirmation{SchemaVersion: 2, PreparationID: prepared.PreparationID, ChallengeID: noChecks.ID, BindingSHA256: prepared.BindingSHA256, Approved: true}
	final, err := r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.confirm", "op_no_checks_answer", prepared.PreparationID, 1, answer))
	if err != nil {
		t.Fatal(err)
	}
	var complete control.PreparedRun
	if err := json.Unmarshal(final.Body, &complete); err != nil {
		t.Fatal(err)
	}
	if !complete.Ready || final.Revision != 2 {
		t.Fatal("exact no-checks answer did not finish readiness")
	}
}

func TestServiceRetentionPolicyEligibility(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		provenance ab.RetentionProvenanceV1
	}{
		{"email classification", "Decision.GitEmail", ab.RetentionOperatorInput},
		{"path classification", "Request.TaskFile", ab.RetentionOperatorInput},
		{"argv classification", "Decision.Proposal.Args.0", ab.RetentionOperatorInput},
		{"challenge classification", "Challenges.0.Text", ab.RetentionOperatorInput},
		{"native authentication provenance", "Request.TaskFile", ab.RetentionNativeAuth},
		{"full environment provenance", "Request.TaskFile", ab.RetentionCredentialEnv},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := testRuntime(t)
			spec := retentionOnlyRequestFixture(t)
			configureSyntheticAdmission(t, r, spec)
			old := r.preparations.context
			content := r.preparations.policy.Content
			content.Rules = append([]ab.RetentionRuleV1{}, content.Rules...)
			found := false
			for i := range content.Rules {
				if content.Rules[i].FieldPath == tc.path {
					found = true
					content.Rules[i].Provenance = tc.provenance
					if tc.provenance == ab.RetentionOperatorInput {
						content.Rules[i].Classification = ab.RetentionForbidden
					}
				}
			}
			if !found {
				t.Fatalf("missing classification fixture %s", tc.path)
			}
			sum, err := ab.RetentionPolicyDigestV1(content)
			if err != nil {
				t.Fatal(err)
			}
			policy := ab.RetentionPolicyV1{Revision: "sha256:" + hex.EncodeToString(sum[:]), Content: content}
			fresh, err := supervisor.NewExecutionContext(t.Context(), r.journal, r.lease, supervisor.ExecutionContextOptions{PolicyRevision: policy.Revision, RetentionPolicy: policy})
			if err != nil {
				t.Fatal(err)
			}
			if err := old.Close(); err != nil {
				t.Fatal(err)
			}
			r.preparations.context = fresh
			r.preparations.policy = policy
			prepared, peer := prepareFixtureRun(t, r, spec, "op_policy_prepare")
			before := admissionRowCounts(t, r)
			result, err := r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.confirm", "op_policy_refusal", prepared.PreparationID, 0, retentionAnswer(prepared)))
			requireAdmissionCode(t, err, control.Code("policy_ineligible"))
			if len(result.Body) != 0 || admissionRowCounts(t, r) != before || len(r.preparations.confirmations) != 0 {
				t.Fatal("forbidden retained value or provenance produced authority")
			}
		})
	}
}

func TestServicePreparedConcurrentRevisions(t *testing.T) {
	r := testRuntime(t)
	spec := projectRequestFixture(t)
	configureSyntheticAdmission(t, r, spec)
	prepared, peer := prepareFixtureRun(t, r, spec, "op_concurrent_prepare")
	if _, err := r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.confirm", "op_concurrent_retention", prepared.PreparationID, 0, retentionAnswer(prepared))); err != nil {
		t.Fatal(err)
	}
	var project control.AdmissionChallenge
	for _, challenge := range prepared.Challenges {
		if challenge.Kind == "project-config" {
			project = challenge
		}
	}
	answer := control.RunConfirmation{SchemaVersion: 2, PreparationID: prepared.PreparationID, ChallengeID: project.ID, BindingSHA256: prepared.BindingSHA256, Approved: true}
	type reply struct {
		prepared control.PreparedReply
		err      error
	}
	results := make(chan reply, 2)
	start := make(chan struct{})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for i := range 2 {
		go func() {
			<-start
			client := control.NewPreparedRunClient(&admissionFramedConn{runtime: r, peer: peer})
			result, err := client.ConfirmRevisioned(ctx, fmt.Sprintf("op_concurrent_answer_%d", i), 1, answer)
			results <- reply{result, err}
		}()
	}
	close(start)
	accepted, refused := 0, 0
	for range 2 {
		select {
		case result := <-results:
			if result.err == nil {
				accepted++
				if result.prepared.Revision != 2 || !result.prepared.Run.Ready {
					t.Fatalf("winning revision=%+v", result.prepared)
				}
			} else {
				refused++
				requireAdmissionCode(t, result.err, control.CodeRevisionConflict)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if accepted != 1 || refused != 1 || len(r.preparations.confirmations) != 2 {
		t.Fatalf("concurrent stale effects: accepted=%d refused=%d hints=%d", accepted, refused, len(r.preparations.confirmations))
	}
	image := preparationImage(t, r, prepared.PreparationID)
	if !strings.HasPrefix(image, "2/1/") {
		t.Fatalf("row/outer revision mismatch=%s", image)
	}
}

func TestServiceRejectionRevokesPlanAndPermit(t *testing.T) {
	r := testRuntime(t)
	spec := retentionOnlyRequestFixture(t)
	configureSyntheticAdmission(t, r, spec)
	prepared, peer := prepareFixtureRun(t, r, spec, "op_revoke_prepare")
	answer := retentionAnswer(prepared)
	retainedIntent := admissionIntent(t, r, "run.confirm", "op_revoke_retention", prepared.PreparationID, 0, answer)
	first, err := r.server.Dispatch(t.Context(), peer, retainedIntent)
	if err != nil {
		t.Fatal(err)
	}
	entry := r.preparations.plans[prepared.PreparationID]
	metadata, err := entry.plan.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	lookup := journal.PreparationLookup{PreparationID: metadata.PreparationID, PeerID: metadata.PeerID, RootID: metadata.RootID, BindingSHA256: metadata.BindingSHA256, ContextEpoch: metadata.ContextEpoch, Generation: metadata.Generation, ExpectedRevision: 1, ObservedAt: time.Now().UTC()}
	confirmed, err := r.journal.ReadConfirmedPreparation(t.Context(), lookup)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := supervisor.RevalidateService(t.Context(), confirmed, entry.inputs)
	if err != nil {
		t.Fatal(err)
	}
	before := admissionRowCounts(t, r)
	answer.Approved = false
	attackPeer := peer
	attackPeer.PID++
	_, err = r.server.Dispatch(t.Context(), attackPeer, admissionIntent(t, r, "run.confirm", "op_foreign_rejection", prepared.PreparationID, 1, answer))
	requireAdmissionCode(t, err, control.CodePermissionDenied)
	if _, err := permit.Metadata(); err != nil {
		t.Fatalf("wrong peer revoked a legitimate permit: %v", err)
	}
	_, err = r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.confirm", "op_exact_rejection", prepared.PreparationID, 1, answer))
	requireAdmissionCode(t, err, control.Code("policy_ineligible"))
	if _, err := permit.Metadata(); err == nil {
		t.Fatal("outstanding permit survived an exact authenticated rejection")
	}
	if _, err := supervisor.RevalidateService(t.Context(), confirmed, entry.inputs); err == nil {
		t.Fatal("rejected plan minted another permit")
	}
	historical, err := r.server.Dispatch(t.Context(), peer, retainedIntent)
	if err != nil {
		t.Fatal(err)
	}
	if historical.Revision != first.Revision || !bytes.Equal(historical.Body, first.Body) {
		t.Fatal("rejection rewrote the immutable historical acknowledgement")
	}
	if admissionRowCounts(t, r) != before {
		t.Fatal("plan-specific revocation changed durable rows or granted effects")
	}
}
