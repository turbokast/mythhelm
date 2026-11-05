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
	// Design §3 requires physical canonical request paths; temporary roots may
	// contain OS aliases (for example a symlink or Windows short name).
	repo, err := admission.RepoIdentity(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
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
		result control.Result
		err    error
	}
	results := make(chan reply, 2)
	start := make(chan struct{})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	// Exercise the service revision race with its own bounded context. The typed
	// client has a separate two-second total budget, including queued validation.
	// TestReviewStaleConfirmationHasCurrent covers its actual framed metadata.
	for i := range 2 {
		intent := admissionIntent(t, r, "run.confirm", fmt.Sprintf("op_concurrent_answer_%d", i), prepared.PreparationID, 1, answer)
		go func() {
			<-start
			result, err := r.server.Dispatch(ctx, peer, intent)
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
				var current control.PreparedRun
				if err := json.Unmarshal(result.result.Body, &current); err != nil {
					t.Fatal(err)
				}
				if result.result.Revision != 2 || !current.Ready {
					t.Fatalf("winning revision=%+v body=%+v", result.result, current)
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

// admissionDeadlineConn records the confirmation's request context before the
// typed client cancels it on return. Status discovery cannot supply this evidence.
type admissionDeadlineConn struct {
	*admissionFramedConn
	confirmed              bool
	confirmationContextErr error
}

func (c *admissionDeadlineConn) Request(ctx context.Context, frame []byte) ([]byte, error) {
	in, err := control.Decode[control.Intent](frame)
	if err != nil {
		return nil, err
	}
	response, err := c.admissionFramedConn.Request(ctx, frame)
	if in.Method == "run.confirm" {
		c.confirmed = true
		c.confirmationContextErr = ctx.Err()
	}
	return response, err
}

func TestServicePreparedConcurrentStaleClients(t *testing.T) {
	r := testRuntime(t)
	spec := projectRequestFixture(t)
	configureSyntheticAdmission(t, r, spec)
	prepared, peer := prepareFixtureRun(t, r, spec, "op_stale_clients_prepare")
	if _, err := r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.confirm", "op_stale_clients_retention", prepared.PreparationID, 0, retentionAnswer(prepared))); err != nil {
		t.Fatal(err)
	}
	var project control.AdmissionChallenge
	for _, challenge := range prepared.Challenges {
		if challenge.Kind == "project-config" {
			project = challenge
		}
	}
	answer := control.RunConfirmation{SchemaVersion: 2, PreparationID: prepared.PreparationID, ChallengeID: project.ID, BindingSHA256: prepared.BindingSHA256, Approved: true}
	before := preparationImage(t, r, prepared.PreparationID)
	if !strings.HasPrefix(before, "1/0/") {
		t.Fatalf("unexpected initial revision/readiness: %s", before)
	}
	rowsBefore := admissionRowCounts(t, r)
	hintsBefore := len(r.preparations.confirmations)
	type reply struct {
		err        error
		confirmed  bool
		contextErr error
	}
	results := make(chan reply, 2)
	start := make(chan struct{})
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for i := range 2 {
		go func() {
			<-start
			conn := &admissionDeadlineConn{admissionFramedConn: &admissionFramedConn{runtime: r, peer: peer}}
			client := control.NewPreparedRunClient(conn)
			_, err := client.ConfirmRevisioned(ctx, fmt.Sprintf("op_stale_clients_answer_%d", i), 0, answer)
			results <- reply{err, conn.confirmed, conn.confirmationContextErr}
		}()
	}
	close(start)
	for range 2 {
		select {
		case got := <-results:
			if !got.confirmed {
				t.Fatalf("confirmation did not reach registered server: %v", got.err)
			}
			var ce *control.Error
			if !errors.As(got.err, &ce) {
				t.Fatalf("stale client must receive typed refusal: %v", got.err)
			}
			if ce.Code == control.Code("dependency_stale") {
				if !errors.Is(got.contextErr, context.DeadlineExceeded) {
					t.Fatalf("dependency stale without observed request deadline: err=%v context=%v", got.err, got.contextErr)
				}
				// Deadline exhaustion proves refusal only, not revision-conflict timing.
				continue
			}
			requireAdmissionCode(t, got.err, control.CodeRevisionConflict)
			var conflict *control.PreparationConflictError
			if !errors.As(got.err, &conflict) {
				t.Fatalf("missing typed current metadata: %v", got.err)
			}
			current, ok := conflict.Current()
			if !ok || current.Revision != 1 || current.Run.Ready || current.Run.PreparationID != prepared.PreparationID || current.Run.BindingSHA256 != prepared.BindingSHA256 || current.Run.Generation != prepared.Generation {
				t.Fatalf("incoherent current metadata: %+v present=%v", current, ok)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if after := preparationImage(t, r, prepared.PreparationID); after != before {
		t.Fatalf("stale clients changed durable state: before=%s after=%s", before, after)
	}
	if rowsAfter := admissionRowCounts(t, r); rowsAfter != rowsBefore {
		t.Fatalf("stale clients changed durable row counts: before=%s after=%s", rowsBefore, rowsAfter)
	}
	if hintsAfter := len(r.preparations.confirmations); hintsAfter != hintsBefore {
		t.Fatalf("stale clients changed confirmation hints: before=%d after=%d", hintsBefore, hintsAfter)
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

func TestReviewStaleRejectionDoesNotRevoke(t *testing.T) {
	r := testRuntime(t)
	spec := retentionOnlyRequestFixture(t)
	configureSyntheticAdmission(t, r, spec)
	prepared, peer := prepareFixtureRun(t, r, spec, "op_review_prepare")
	answer := retentionAnswer(prepared)
	if _, err := r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.confirm", "op_review_retention", prepared.PreparationID, 0, answer)); err != nil {
		t.Fatal(err)
	}
	entry := r.preparations.plans[prepared.PreparationID]
	m, err := entry.plan.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := r.journal.ReadConfirmedPreparation(t.Context(), journal.PreparationLookup{PreparationID: m.PreparationID, PeerID: m.PeerID, RootID: m.RootID, BindingSHA256: m.BindingSHA256, ContextEpoch: m.ContextEpoch, Generation: m.Generation, ExpectedRevision: 1, ObservedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	permit, err := supervisor.RevalidateService(t.Context(), confirmed, entry.inputs)
	if err != nil {
		t.Fatal(err)
	}
	answer.Approved = false
	out, err := r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.confirm", "op_review_stale_reject", prepared.PreparationID, 0, answer))
	var ce *control.Error
	if !errors.As(err, &ce) || ce.Code != control.CodeRevisionConflict {
		t.Errorf("stale rejection error = %v, expected revision_conflict; reply=%s", err, out.Body)
	}
	if _, err := permit.Metadata(); err != nil {
		t.Errorf("stale rejection revoked current permit: %v", err)
	}
}

func TestReviewStaleConfirmationHasCurrent(t *testing.T) {
	r := testRuntime(t)
	spec := projectRequestFixture(t)
	configureSyntheticAdmission(t, r, spec)
	client := control.NewPreparedRunClient(&admissionFramedConn{runtime: r, peer: control.Peer{SameUser: true, PID: 77}})
	p, err := client.PrepareRevisioned(t.Context(), "op_review_current_prepare", spec)
	if err != nil {
		t.Fatal(err)
	}
	retention := retentionAnswer(p.Run)
	retained, err := client.ConfirmRevisioned(t.Context(), "op_review_current_retain", 0, retention)
	if err != nil {
		t.Fatal(err)
	}
	if p.Revision != 0 || p.Run.Ready || retained.Revision != 1 || retained.Run.Ready {
		t.Fatalf("unexpected prepare/retention revisions: prepared=%+v retained=%+v", p, retained)
	}
	var answer control.RunConfirmation
	for _, ch := range p.Run.Challenges {
		if ch.Kind == "project-config" {
			answer = control.RunConfirmation{SchemaVersion: 2, PreparationID: p.Run.PreparationID, ChallengeID: ch.ID, BindingSHA256: p.Run.BindingSHA256, Approved: true}
		}
	}
	final, err := client.ConfirmRevisioned(t.Context(), "op_review_current_answer", retained.Revision, answer)
	if err != nil {
		t.Fatal(err)
	}
	if final.Revision != 2 || !final.Run.Ready {
		t.Fatalf("unexpected final confirmation: %+v", final)
	}
	before := preparationImage(t, r, p.Run.PreparationID)
	if !strings.HasPrefix(before, "2/1/") {
		t.Fatalf("row/outer revision mismatch=%s", before)
	}
	_, err = client.ConfirmRevisioned(t.Context(), "op_review_current_stale", retained.Revision, answer)
	var conflict *control.PreparationConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("stale confirmation error=%T %v; expected PreparationConflictError with current revision %d", err, err, final.Revision)
	}
	current, ok := conflict.Current()
	requireAdmissionCode(t, err, control.CodeRevisionConflict)
	if !ok || current.Revision != final.Revision || !current.Run.Ready || current.Run.PreparationID != p.Run.PreparationID || current.Run.Generation != p.Run.Generation || current.Run.BindingSHA256 != p.Run.BindingSHA256 {
		t.Fatalf("incoherent current=%+v present=%v", current, ok)
	}
	if after := preparationImage(t, r, p.Run.PreparationID); after != before {
		t.Fatalf("stale confirmation changed durable state: before=%s after=%s", before, after)
	}
}

func admissionEffectSnapshot(t *testing.T, r *runtime) string {
	t.Helper()
	rows := []string{}
	if err := r.journal.Transact(t.Context(), func(tx *sql.Tx) error {
		query, err := tx.QueryContext(t.Context(), `SELECT preparation_id,revision,ready,confirmations FROM prepared_runs ORDER BY preparation_id`)
		if err != nil {
			return err
		}
		defer func() { _ = query.Close() }()
		for query.Next() {
			var id, answers string
			var revision, ready int
			if err := query.Scan(&id, &revision, &ready, &answers); err != nil {
				return err
			}
			rows = append(rows, fmt.Sprintf("%s/%d/%d/%s", id, revision, ready, answers))
		}
		return query.Err()
	}); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%s/%v/active=%d/ops=%d/hints=%d", admissionRowCounts(t, r), rows, len(r.preparations.plans), len(r.preparations.operations), len(r.preparations.confirmations))
}

func TestServiceClosedPlanEarliestEffectMatrix(t *testing.T) {
	for _, stage := range []string{"prepare", "retention", "remaining answer"} {
		for _, change := range []string{"missing", "extra", "wrong kind", "changed text", "swapped IDs", "duplicate ID", "authority"} {
			t.Run(stage+"/"+change, func(t *testing.T) {
				r := testRuntime(t)
				spec := projectRequestFixture(t)
				configureSyntheticAdmission(t, r, spec)
				peer := control.Peer{SameUser: true, PID: 77}
				var prepared control.PreparedRun
				if stage != "prepare" {
					prepared, peer = prepareFixtureRun(t, r, spec, "op_matrix_prepare")
				}
				if stage == "remaining answer" {
					if _, err := r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.confirm", "op_matrix_retain", prepared.PreparationID, 0, retentionAnswer(prepared))); err != nil {
						t.Fatal(err)
					}
				}
				before := admissionEffectSnapshot(t, r)
				barriers := 0
				r.preparations.beforeEffect = func() {
					barriers++
					pending := r.preparations.pending
					if pending == nil {
						t.Fatal("fresh transition did not expose its closed view")
					}
					b, err := ab.DecodeV1(pending.canonical)
					if err != nil {
						t.Fatal(err)
					}
					switch change {
					case "missing":
						b.Challenges = b.Challenges[1:]
					case "extra":
						b.Challenges = append(b.Challenges, ab.ChallengeV1{ID: "challenge_extra", Kind: "project-config", Text: "Different authority"})
					case "wrong kind":
						b.Challenges[0].Kind = "no-checks"
					case "changed text":
						b.Challenges[0].Text = "Different authority"
					case "swapped IDs":
						b.Challenges[0].ID, b.Challenges[1].ID = b.Challenges[1].ID, b.Challenges[0].ID
					case "duplicate ID":
						b.Challenges[1].ID = b.Challenges[0].ID
					case "authority":
						b.Request.KeepGoing = true
						b.Decision.KeepGoing = true
						pending.inputs.Request.KeepGoing = true
					}
					pending.metadata.Challenges = append([]ab.ChallengeV1{}, b.Challenges...)
					raw, err := ab.EncodeV1(b)
					if err != nil {
						if change != "duplicate ID" {
							t.Fatal(err)
						}
						raw, err = json.Marshal(b)
						if err != nil {
							t.Fatal(err)
						}
					}
					pending.canonical = raw
					if digest, err := ab.DigestV1(b); err == nil {
						pending.metadata.BindingSHA256 = hex.EncodeToString(digest[:])
					}
				}
				in := admissionIntent(t, r, "run.prepare", "op_matrix_prepare", "", 0, spec)
				if stage == "retention" {
					in = admissionIntent(t, r, "run.confirm", "op_matrix_retain", prepared.PreparationID, 0, retentionAnswer(prepared))
				}
				if stage == "remaining answer" {
					var answer control.RunConfirmation
					for _, ch := range prepared.Challenges {
						if ch.Kind == "project-config" {
							answer = control.RunConfirmation{SchemaVersion: 2, PreparationID: prepared.PreparationID, ChallengeID: ch.ID, BindingSHA256: prepared.BindingSHA256, Approved: true}
						}
					}
					in = admissionIntent(t, r, "run.confirm", "op_matrix_answer", prepared.PreparationID, 1, answer)
				}
				result, err := r.server.Dispatch(t.Context(), peer, in)
				if err == nil || len(result.Body) != 0 || barriers != 1 || r.preparations.pending != nil || admissionEffectSnapshot(t, r) != before {
					t.Fatalf("altered closed authority crossed %s boundary: error=%v body=%s barriers=%d", stage, err, result.Body, barriers)
				}
			})
		}
	}
}

func TestServicePreparationCapacityRecovery(t *testing.T) {
	for _, mode := range []string{"expire", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			r := testRuntime(t)
			spec := requestFixture(t)
			configureSyntheticAdmission(t, r, spec)
			before := admissionRowCounts(t, r)
			prepared := []control.PreparedRun{}
			peers := []control.Peer{}
			for i := range 32 {
				peer := control.Peer{SameUser: true, PID: int32(100 + i/8)}
				p, _ := prepareFixtureRunPeer(t, r, spec, fmt.Sprintf("op_recovery_%d", i), peer)
				prepared = append(prepared, p)
				peers = append(peers, peer)
			}
			for _, index := range []int{0, 8} {
				p := prepared[index]
				if mode == "expire" {
					r.preparations.plans[p.PreparationID].expires = time.Now().Add(-time.Second)
				} else {
					answer := retentionAnswer(p)
					answer.Approved = false
					_, err := r.server.Dispatch(t.Context(), peers[index], admissionIntent(t, r, "run.confirm", fmt.Sprintf("op_cancel_%d", index), p.PreparationID, 0, answer))
					requireAdmissionCode(t, err, control.Code("policy_ineligible"))
					old := r.preparations.operations[fmt.Sprintf("op_recovery_%d", index)]
					if len(old.result.Body) != 0 {
						t.Fatal("cancellation kept the full unapproved acknowledgement")
					}
				}
			}
			prepareFixtureRunPeer(t, r, spec, "op_recovered_peer", peers[0])
			prepareFixtureRunPeer(t, r, spec, "op_recovered_global", control.Peer{SameUser: true, PID: 999})
			if len(r.preparations.plans) != 32 {
				t.Fatalf("recovered active slots=%d", len(r.preparations.plans))
			}
			if _, ok := r.preparations.plans[prepared[0].PreparationID]; ok {
				t.Fatal("expired/cancelled plan remained active")
			}
			if _, ok := r.preparations.plans[prepared[1].PreparationID]; !ok {
				t.Fatal("live unexpired preparation was evicted")
			}
			if admissionRowCounts(t, r) != before {
				t.Fatal("volatile capacity recovery changed durable authority")
			}
			_, err := r.server.Dispatch(t.Context(), peers[0], admissionIntent(t, r, "run.prepare", "op_recovery_0", "", 0, spec))
			if mode == "cancel" {
				requireAdmissionCode(t, err, control.Code("dependency_stale"))
			}
		})
	}
}
func prepareFixtureRunPeer(t *testing.T, r *runtime, spec control.RunSpec, operation string, peer control.Peer) (control.PreparedRun, control.Result) {
	t.Helper()
	result, err := r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.prepare", operation, "", 0, spec))
	if err != nil {
		t.Fatal(err)
	}
	var prepared control.PreparedRun
	if err := json.Unmarshal(result.Body, &prepared); err != nil {
		t.Fatal(err)
	}
	return prepared, result
}

func TestServiceTaskByteCap(t *testing.T) {
	r := testRuntime(t)
	spec := requestFixture(t)
	configureSyntheticAdmission(t, r, spec)
	body := append([]byte("# Bounded task\n"), bytes.Repeat([]byte("x"), admission.MaxTaskBytes)...)
	if err := os.WriteFile(spec.TaskFile, body, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	spec.TaskSHA256 = hex.EncodeToString(sum[:])
	before := admissionEffectSnapshot(t, r)
	result, err := r.server.Dispatch(t.Context(), control.Peer{SameUser: true, PID: 77}, admissionIntent(t, r, "run.prepare", "op_oversize_task", "", 0, spec))
	if err == nil || len(result.Body) != 0 || admissionEffectSnapshot(t, r) != before {
		t.Fatal("oversized task crossed the volatile preparation boundary")
	}
}

func TestServicePreparedRestartRefuses(t *testing.T) {
	r := testRuntime(t)
	spec := requestFixture(t)
	configureSyntheticAdmission(t, r, spec)
	prepared, peer := prepareFixtureRun(t, r, spec, "op_restart_prepare")
	root := r.lease.Root()
	if err := r.shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	fresh, err := openRuntime(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.shutdown(context.Background()) })
	configureSyntheticAdmission(t, fresh, spec)
	before := admissionEffectSnapshot(t, fresh)
	result, err := fresh.server.Dispatch(t.Context(), peer, admissionIntent(t, fresh, "run.confirm", "op_restart_confirm", prepared.PreparationID, 0, retentionAnswer(prepared)))
	requireAdmissionCode(t, err, control.Code("dependency_stale"))
	if len(result.Body) != 0 || admissionEffectSnapshot(t, fresh) != before {
		t.Fatal("restart recreated authority from a forgotten preparation")
	}
}

func TestServiceBindingByteCap(t *testing.T) {
	r := testRuntime(t)
	spec := retentionOnlyRequestFixture(t)
	configureSyntheticAdmission(t, r, spec)
	arg := "'" + strings.Repeat("\"", 2500) + "'"
	config := []byte("schema_version = 1\n[[checks]]\nname = \"bounded\"\nargv = ['git'," + strings.Repeat(arg+",", 254) + arg + "]\ntimeout = \"5s\"\n")
	_, digest, err := admission.ParseProjectConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(spec.Repository, admission.ProjectConfigFile), config, 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", admission.ProjectConfigFile}, {"commit", "--quiet", "-m", "bounded binding fixture"}} {
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = spec.Repository
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("binding fixture: %v %s", err, out)
		}
	}
	spec.TrustProjectConfig = "sha256:" + digest
	before := admissionEffectSnapshot(t, r)
	result, err := r.server.Dispatch(t.Context(), control.Peer{SameUser: true, PID: 77}, admissionIntent(t, r, "run.prepare", "op_oversize_binding", "", 0, spec))
	if err == nil || len(result.Body) != 0 || admissionEffectSnapshot(t, r) != before {
		t.Fatal("oversized canonical binding produced effects")
	}
}

func TestServicePreparedRootReplacementRefuses(t *testing.T) {
	r := testRuntime(t)
	spec := requestFixture(t)
	configureSyntheticAdmission(t, r, spec)
	prepared, peer := prepareFixtureRun(t, r, spec, "op_replaced_prepare")
	root := r.lease.Root()
	if err := r.journal.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root, root+"-original"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	replacement, err := journal.Open(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if err := replacement.Close(); err != nil {
		t.Fatal(err)
	}
	database := filepath.Join(root, journal.DBName)
	before, err := os.ReadFile(database) // #nosec G304 -- Fixed replacement database within this isolated root.
	if err != nil {
		t.Fatal(err)
	} // #nosec G304 -- Fixed replacement database within this isolated root.
	result, err := r.server.Dispatch(t.Context(), peer, admissionIntent(t, r, "run.confirm", "op_replaced_confirm", prepared.PreparationID, 0, retentionAnswer(prepared)))
	requireAdmissionCode(t, err, control.Code("dependency_stale"))
	after, err := os.ReadFile(database) // #nosec G304 -- Same deliberate replacement database; verifies refusal effects.
	if err != nil {
		t.Fatal(err)
	} // #nosec G304 -- Same deliberate replacement database; verifies refusal effects.
	if len(result.Body) != 0 || !bytes.Equal(before, after) || r.preparations.pending != nil {
		t.Fatal("physical root replacement disclosed or changed preparation authority")
	}
}

// This fixture changes process-wide temporary-directory inputs and cannot run in parallel.
func TestServiceCanonicalRequestFixtureAliases(t *testing.T) {
	// The child TempDir includes its test name. Repeating the parent test name
	// in this root exceeds Windows Git's path limit for loose object files.
	base, err := os.MkdirTemp("", "mha-") //nolint:usetesting // t.TempDir repeats the test name and exceeds Windows Git path limits; cleanup below owns this short root.
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(base); err != nil {
			t.Errorf("remove owned alias fixture root: %v", err)
		}
	})
	alias := filepath.Join(base, "alias")
	target := filepath.Join(base, "physical")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, alias); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	for _, name := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(name, alias)
	}
	t.Run("physical path contract", func(t *testing.T) {
		r := testRuntime(t)
		spec := requestFixture(t)
		physical, err := admission.RepoIdentity(spec.Repository)
		if err != nil {
			t.Fatal(err)
		}
		if spec.Repository != physical {
			t.Fatalf("fixture repository=%q, physical=%q", spec.Repository, physical)
		}
		configureSyntheticAdmission(t, r, spec)
		prepared, _ := prepareFixtureRun(t, r, spec, "op_canonical_fixture")
		raw, err := r.preparations.plans[prepared.PreparationID].plan.CanonicalBinding()
		if err != nil {
			t.Fatal(err)
		}
		binding, err := ab.DecodeV1(raw)
		if err != nil {
			t.Fatal(err)
		}
		if binding.Request.Repository != physical || binding.Decision.RepoIdentity != physical || binding.Decision.Snapshot.SourceRepo != physical {
			t.Fatal("actual producer changed physical request provenance")
		}
	})
}
