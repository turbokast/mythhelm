package journal_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/acceptance"
	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/admission"
	ab "github.com/turbokast/mythhelm/internal/admissionbinding"
	"github.com/turbokast/mythhelm/internal/contain"
	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/qualify"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// This literal synthetic fixture is independent of the storage mapper.
const storageBindingGolden = `{"SchemaVersion":1,"PreparationID":"prep_1","PeerID":"peer_1","RootID":"root_1","Generation":1,"ContextEpoch":"epoch_1","PolicyRevision":"policy_1","ExpiresAt":"2026-01-01T00:05:00Z","RunID":"run_1","TaskID":"task_1","AttemptID":"attempt_1","Request":{"SchemaVersion":2,"Repository":"/tmp/repo","TaskFile":"/tmp/task.md","TaskSHA256":"sha256:ABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABABAB","Adapter":"claudecode","Billing":"subscription-declared","ExecutionProfile":"restricted","Revision":"main","Host":"standalone","Scenario":"","UseCommitted":false,"NoChecks":false,"KeepGoing":true,"TrustProjectConfig":"sha256:abababababababababababababababababababababababababababababababab","TrustNativeConfig":"sha256:abababababababababababababababababababababababababababababababab","DeclareEntitlement":"plan=pro,extra-usage=disabled","StripCredentialEnv":true,"AllowUntestedNativeVersion":true,"Ceilings":{"ExecutionNanos":60000000000,"Repairs":0,"Replans":null,"TransportRetries":2}},"Decision":{"StateRootID":"root_1","RunID":"run_1","TaskID":"task_1","AttemptID":"attempt_1","RunDir":"/tmp/state/runs/run_1","Workdir":"/tmp/state/runs/run_1/workspace","Host":"standalone","Scenario":"","GitName":"Synthetic Fixture","GitEmail":"fixture@example.com","ProjectConfig":{"SchemaVersion":1,"Passthrough":["ZED","ALPHA"],"AllowedTools":["Read","Bash(go test:*)"],"Checks":[{"Name":"vet","Argv":["go","vet","./..."],"FailOnOutput":true,"Timeout":"10s"},{"Name":"test","Argv":["go","test","./..."],"FailOnOutput":false,"Timeout":"1m"}],"Envelopes":{"Execution":"2m","Repairs":3,"Replans":1,"TransportRetries":5}},"ConfigDigest":"abababababababababababababababababababababababababababababababab","RepoIdentity":"/tmp/repo","RecordTrust":true,"NoChecks":false,"KeepGoing":true,"EnvelopeFlags":{"ExecutionNanos":60000000000,"Repairs":0,"Replans":null,"TransportRetries":2},"ResolvedCeilings":{"ExecutionNanos":60000000000,"Repairs":0,"Replans":1,"TransportRetries":2},"Declaration":{"AdapterID":"builtin/claudecode","PlanClass":"pro","ExtraUsage":"disabled","IdentityRef":"abababababababababababababababababababababababababababababababab","DeclaredAt":"2026-01-01T00:00:00Z"},"RecordNativeTrust":true,"NativeConfigDigest":"abababababababababababababababababababababababababababababababab","NativeHooks":0,"NativeTrustGrant":"native_config:sha256:abababababababababababababababababababababababababababababababab","NativeAuth":{"LoggedIn":true,"AuthMethod":"claude.ai","APIProvider":"firstParty","SubscriptionType":"pro","IdentityRef":"abababababababababababababababababababababababababababababababab","ConfigIdentity":"config_1"},"Task":{"SHA256":"abababababababababababababababababababababababababababababababab","Title":"Synthetic <&> task","ArtifactRef":"owned/task_1","Bytes":12},"Snapshot":{"SourceRepo":"/tmp/repo","Branch":"main","BaseRev":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Dirty":false,"Source":"--rev"},"Profile":{"Name":"restricted","Contained":true,"Consent":"--execution-profile","Disclosure":"fixture","Boundary":{"Profile":"restricted","OS":"linux","Route":"builtin/claudecode","Boundary":"fixture","Version":"1","Owner":"fixture","Coverage":{"Filesystem":{"Name":"synthetic boundary","Version":"1","Enforced":true,"Detail":"fixture <&>"},"Process":{"Name":"synthetic boundary","Version":"1","Enforced":true,"Detail":"fixture <&>"},"Network":{"Name":"synthetic boundary","Version":"1","Enforced":true,"Detail":"fixture <&>"},"Credential":{"Name":"synthetic boundary","Version":"1","Enforced":true,"Detail":"fixture <&>"}}}},"Adapter":{"ID":"builtin/claudecode","Version":"1","Harness":"claude-code","Surface":"native-cli-structured"},"Probe":{"Executable":"/tmp/native","Version":"2.1.284","SHA256":"abababababababababababababababababababababababababababababababab","OS":"linux","Arch":"amd64","Compatibility":"fixture-tested"},"Proposal":{"Path":"/tmp/native","Args":["--print","second","first"],"Dir":"/tmp/state/runs/run_1/workspace","StopLadder":[{"Signal":"interrupt","GraceNanos":0},{"Signal":"kill","GraceNanos":1}],"Overrides":[{"Kind":"env","Name":"FIXTURE","Value":"yes","Reason":"synthetic"}],"Billing":{"Mode":"subscription-declared","CredentialProvenance":"native-login","EntitlementClass":"included-plan","EntitlementSource":"user_declared","PaidContinuation":"unknown","PaidContinuationUserDeclaration":"disabled","Qualified":false,"G05":"not-passed"},"Manifest":[{"Source":"project","Digest":"abababababababababababababababababababababababababababababababab"},{"Source":"user","Digest":"abababababababababababababababababababababababababababababababab"}],"Capabilities":{"SchemaVersion":1,"AdapterID":"builtin/claudecode","AdapterVersion":"1","RuntimeVersion":"2.1.284","Mode":"headless","ExecutionSurface":"native-cli-structured","HarnessID":"claude-code","FidelityQualification":"fixture-tested","Billing":{"Entitlement":"subscription","IncludedOnlySupported":"unknown","PaidOveragePrevention":"unsupported","EvidenceID":"evidence_1"},"HostIntegration":{"HerdrEmbedded":"unsupported","HerdrStateBridge":"unknown","NativeInteractiveAttach":"unsupported"},"Capabilities":{"StructuredEvents":"supported","Resume":"unsupported","LiveSteer":"unknown","ApprovalBridge":"unsupported","UsageTokens":"supported","QuotaRemaining":"unknown","HardMonetaryLimit":"unsupported","NativeSubagents":"unsupported","Reconnect":"unknown","ModelMetadata":null},"Sandbox":{"Status":"supported","Scope":"fixture"},"Platform":{"OS":"linux","WorkerDetachment":"supported","ProcessTreeOwnership":"supported"},"Qualification":"synthetic fixture only"}},"Environment":{"PolicyRevision":"policy_1","ContextEpoch":"epoch_1","PrincipalRef":"principal_1","NativeConfigIdentity":"config_1","RequiredNames":["ALPHA","ZED"],"Destinations":["first-party"]}},"Challenges":[{"ID":"challenge_1","Kind":"project-config","Text":"Trust exact project configuration"},{"ID":"challenge_2","Kind":"native-config","Text":"Trust exact native configuration"},{"ID":"challenge_3","Kind":"keep-going","Text":"Continue within finite envelope"}]}`

type preparationFixture struct {
	b     ab.BindingV1
	input journal.RetainedPreparationInput
	now   time.Time
}

func preparationFixtureFor(t *testing.T, extra bool) preparationFixture {
	t.Helper()
	var b ab.BindingV1
	if err := json.Unmarshal([]byte(storageBindingGolden), &b); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	b.ExpiresAt = now.Add(4 * time.Minute).Format(time.RFC3339Nano)
	b.Challenges = []ab.ChallengeV1{{ID: "retention_1", Kind: ab.RetentionChallengeKind, Text: "Retain synthetic local binding"}}
	if extra {
		b.Challenges = append(b.Challenges, ab.ChallengeV1{ID: "native_1", Kind: "native-config", Text: "Trust exact synthetic native configuration"})
	}
	// All path-shaped fields remain native absolute paths in the wire request.
	b.Request.Repository = filepath.Join(t.TempDir(), "repository")
	b.Request.TaskFile = filepath.Join(t.TempDir(), "task.md")
	b.Decision.RepoIdentity = b.Request.Repository
	b.Decision.Snapshot.SourceRepo = b.Request.Repository
	b.Decision.RunDir = filepath.Join(t.TempDir(), "runs", b.RunID)
	b.Decision.Workdir = filepath.Join(b.Decision.RunDir, "workspace")
	b.Decision.Probe.Executable = filepath.Join(t.TempDir(), "native")
	b.Decision.Probe.OS = runtime.GOOS
	b.Decision.Proposal.Capabilities.Platform.OS = runtime.GOOS
	b.Decision.Profile.Boundary.OS = runtime.GOOS
	b.Decision.Proposal.Path = b.Decision.Probe.Executable
	b.Decision.Proposal.Dir = b.Decision.Workdir
	return sealPreparationFixture(t, b, now)
}
func sealPreparationFixture(t *testing.T, b ab.BindingV1, now time.Time) preparationFixture {
	t.Helper()
	content := ab.RetentionPolicyContentV1{SchemaVersion: 1, RootID: b.RootID, Purpose: ab.RetentionPurposeV1, ExecutionPolicyRevision: v2contract.Digest(acceptance.P1()), Rules: []ab.RetentionRuleV1{}}
	sum, err := ab.RetentionPolicyDigestV1(content)
	if err != nil {
		t.Fatal(err)
	}
	policy := ab.RetentionPolicyV1{Revision: "sha256:" + hex.EncodeToString(sum[:]), Content: content}
	rows, err := ab.DeriveRetentionFieldsV1(b, policy)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		content.Rules = append(content.Rules, ab.RetentionRuleV1{FieldPath: r.FieldPath, Classification: ab.RetentionNonSecret, Provenance: ab.RetentionOperatorInput})
	}
	sum, err = ab.RetentionPolicyDigestV1(content)
	if err != nil {
		t.Fatal(err)
	}
	policy = ab.RetentionPolicyV1{Revision: "sha256:" + hex.EncodeToString(sum[:]), Content: content}
	b.PolicyRevision = policy.Revision
	b.Decision.Environment.PolicyRevision = policy.Revision
	raw, err := ab.EncodeV1(b)
	if err != nil {
		t.Fatal(err)
	}
	sum, err = ab.DigestV1(b)
	if err != nil {
		t.Fatal(err)
	}
	digest := hex.EncodeToString(sum[:])
	rows, err = ab.DeriveRetentionFieldsV1(b, policy)
	if err != nil {
		t.Fatal(err)
	}
	fieldSum, err := ab.RetentionFieldSetSHA256(rows)
	if err != nil {
		t.Fatal(err)
	}
	a := ab.RetentionApprovalV1{SchemaVersion: 1, PreparationID: b.PreparationID, ChallengeID: "retention_1", BindingSHA256: digest, FieldSetSHA256: hex.EncodeToString(fieldSum[:]), PolicyRevision: policy.Revision, PeerID: b.PeerID, RootID: b.RootID, ContextEpoch: b.ContextEpoch, Generation: b.Generation, ExpiresAt: b.ExpiresAt, ObservedAt: now.Format(time.RFC3339Nano), Decision: ab.RetentionApprove}
	return preparationFixture{b: b, now: now, input: journal.RetainedPreparationInput{Operation: journal.PreparationOperation{OperationID: "retention_op", ParamsSHA256: strings.Repeat("a", 64)}, CanonicalBinding: raw, Policy: policy, Retention: a, Lookup: journal.PreparationLookup{PreparationID: b.PreparationID, PeerID: b.PeerID, RootID: b.RootID, BindingSHA256: digest, ContextEpoch: b.ContextEpoch, Generation: b.Generation, ObservedAt: now}}}
}
func storageJournal(t *testing.T) *journal.Journal {
	t.Helper()
	j, err := journal.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return j
}
func TestStoredRetentionOnlyReadiness(t *testing.T) { // I02/I03/I20 (v2 §§6/8): retention is required, not launch authority.
	t.Parallel()
	for _, extra := range []bool{false, true} {
		t.Run(map[bool]string{false: "retention only", true: "additional answer"}[extra], func(t *testing.T) {
			t.Parallel()
			f := preparationFixtureFor(t, extra)
			j := storageJournal(t)
			state, err := j.StoreRetainedPreparation(t.Context(), f.input)
			if err != nil {
				t.Fatal(err)
			}
			if state.Revision != 1 || state.Ready == extra || state.Consumed || state.PreparationID != f.b.PreparationID {
				t.Fatalf("unexpected first durable state: %+v", state)
			}
			replay, err := j.StoreRetainedPreparation(t.Context(), f.input)
			if err != nil || replay != state {
				t.Fatalf("immutable replay=%+v %v want %+v", replay, err, state)
			}
			if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
				var raw string
				if err := tx.QueryRowContext(t.Context(), `SELECT result FROM operations WHERE operation_id=?`, f.input.Operation.OperationID).Scan(&raw); err != nil {
					return err
				}
				var got control.Result
				if err := json.Unmarshal([]byte(raw), &got); err != nil {
					return err
				}
				challenges := make([]control.AdmissionChallenge, len(f.b.Challenges))
				for i, ch := range f.b.Challenges {
					challenges[i] = control.AdmissionChallenge{ID: ch.ID, Kind: ch.Kind, Text: ch.Text, BindingSHA256: f.input.Lookup.BindingSHA256}
				}
				body, err := json.Marshal(control.PreparedRun{SchemaVersion: 2, PreparationID: f.b.PreparationID, BindingSHA256: f.input.Lookup.BindingSHA256, Generation: f.b.Generation, Challenges: challenges, Ready: !extra})
				if err != nil {
					return err
				}
				want, err := json.Marshal(control.Result{OperationID: f.input.Operation.OperationID, Revision: 1, Body: body})
				if err != nil {
					return err
				}
				if raw != string(want) {
					t.Errorf("durable acknowledgement differs from actual control codec: got %s want %s", raw, want)
				}
				var attempts, runs int
				if err := tx.QueryRowContext(t.Context(), `SELECT count(*) FROM attempts`).Scan(&attempts); err != nil {
					return err
				}
				if err := tx.QueryRowContext(t.Context(), `SELECT count(*) FROM runs`).Scan(&runs); err != nil {
					return err
				}
				if attempts != 0 || runs != 0 {
					t.Errorf("retention granted execution effects: runs %d attempts %d", runs, attempts)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			lookup := f.input.Lookup
			lookup.ExpectedRevision = 1
			p, err := j.ReadConfirmedPreparation(t.Context(), lookup)
			if extra {
				if err == nil {
					t.Fatal("missing answer produced confirmed handle")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			raw, err := p.CanonicalBinding()
			if err != nil || string(raw) != string(f.input.CanonicalBinding) {
				t.Fatal("confirmed binding differs")
			}
		})
	}
}

func preparedCommit(t *testing.T, j *journal.Journal, f preparationFixture) journal.AdmissionCommit {
	t.Helper()
	if _, err := j.StoreRetainedPreparation(t.Context(), f.input); err != nil {
		t.Fatal(err)
	}
	l := f.input.Lookup
	l.ExpectedRevision = 1
	p, err := j.ReadConfirmedPreparation(t.Context(), l)
	if err != nil {
		t.Fatal(err)
	}
	c, err := journal.NewAdmissionCommit(journal.AdmissionCommitInput{Preparation: p, Lookup: l, OperationID: "start_op", CanonicalBinding: f.input.CanonicalBinding, AcceptancePin: legacyAcceptancePin(t, f), Policy: acceptance.P1()})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func adoptStorageFixture(t *testing.T, j *journal.Journal) {
	t.Helper()
	// Synthetic storage fixtures select an explicit supported migration phase;
	// production root adoption is owned by the separately admitted service.
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), `UPDATE migration_state SET phase='adopted'`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
func TestServiceAdmissionAtomicStorage(t *testing.T) { // I02/I12/I20/I23 (v2 §§5/6/8).
	t.Parallel()
	t.Run("concurrent exact start", testConcurrentAdmission)
	t.Run("current quota refusal", testCurrentQuotaRefusal)
	t.Run("actual admission producer golden", testAdmissionRecordProducerGolden)
	t.Run("actual start digest and ack", testActualStartIntentAndAcknowledgement)
	t.Run("active run conflict", testActiveRunConflict)
	t.Run("admission statement rollback", testAdmissionStatementRollback)
	t.Run("arming statement rollback", testArmingStatementRollback)

	f := preparationFixtureFor(t, false)
	j := storageJournal(t)
	adoptStorageFixture(t, j)
	c := preparedCommit(t, j, f)
	admitted, err := j.CommitServiceAdmission(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	m, err := admitted.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	if m.RunID != f.b.RunID || m.AttemptID != f.b.AttemptID || m.BindingSHA256 != f.input.Lookup.BindingSHA256 || m.Revision != storageRunRevision(t, j, f.b.RunID) {
		t.Fatalf("bad committed metadata: %+v", m)
	}
	again, err := j.CommitServiceAdmission(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := again.Metadata()
	if err != nil || replay != m {
		t.Fatalf("admission replay changed: %+v %v", replay, err)
	}
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		for _, table := range []string{"runs", "service_admission_consumptions", "service_pending_dispatch", "run_assignments", "reservations"} {
			var n int
			if err := tx.QueryRowContext(t.Context(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
				return err
			}
			if n != 1 {
				t.Errorf("%s count=%d want1", table, n)
			}
		}
		var n int
		if err := tx.QueryRowContext(t.Context(), `SELECT count(*) FROM attempts`).Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Error("admission created an attempt before snapshot")
		}
		var first sql.NullString
		if err := tx.QueryRowContext(t.Context(), `SELECT first_start_at FROM run_envelopes`).Scan(&first); err != nil {
			return err
		}
		if first.Valid {
			t.Error("admission started execution clock")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func snapshotForArm(t *testing.T, j *journal.Journal, f preparationFixture) string {
	t.Helper()
	id := "snapshot_event"
	payload, err := json.Marshal(struct {
		BaseRev   string `json:"base_rev"`
		Branch    string `json:"branch"`
		ClonePath string `json:"clone_path"`
		Trust     bool   `json:"project_config_trust_granted"`
	}{f.b.Decision.Snapshot.BaseRev, f.b.Decision.Snapshot.Branch, f.b.Decision.Workdir, f.b.Decision.RecordTrust})
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Append(t.Context(), journal.Event{SchemaVersion: 2, EventID: id, RunID: f.b.RunID, TaskID: f.b.TaskID, ProducerID: "snapshot_fixture", ProducerSequence: 1, Generation: f.b.Generation, ObservedAt: time.Now().UTC(), Type: "workspace.snapshot_created", Payload: payload}, func(tx *sql.Tx) error {
		if f.b.Decision.RecordTrust {
			return journal.InsertTrustGrant(t.Context(), tx, "project_config", f.b.Decision.RepoIdentity, f.b.Decision.ConfigDigest, time.Now().UTC())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return id
}
func TestServiceArmingStorage(t *testing.T) { // I12/I20/I21 (v2 §§5/6): durable single-use launch intent.
	t.Parallel()
	f := preparationFixtureFor(t, false)
	j := storageJournal(t)
	adoptStorageFixture(t, j)
	c := preparedCommit(t, j, f)
	admitted, err := j.CommitServiceAdmission(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := snapshotForArm(t, j, f)
	now := time.Now().UTC()
	in := journal.DispatchArmInput{Admission: admitted, ExpectedPendingRevision: 1, SnapshotEventID: snapshot, BindingSHA256: f.input.Lookup.BindingSHA256, LaunchTokenSHA256: strings.Repeat("c", 64), NonceSHA256: strings.Repeat("d", 64), LadderVersion: "synthetic-ladder-v1", PolicyRevision: f.b.PolicyRevision, Generation: f.b.Generation, ObservedAt: now, ExecutionDeadline: now.Add(time.Duration(f.b.Decision.ResolvedCeilings.ExecutionNanos))}
	arm, err := journal.NewDispatchArm(in)
	if err != nil {
		t.Fatal(err)
	}
	dispatch, err := j.ArmServiceDispatch(t.Context(), arm)
	if err != nil {
		t.Fatal(err)
	}
	m, err := dispatch.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	if m.SnapshotEventID != snapshot || m.LaunchTokenSHA256 != in.LaunchTokenSHA256 || m.Revision != storageRunRevision(t, j, f.b.RunID) || !m.ExecutionDeadline.Equal(in.ExecutionDeadline) {
		t.Fatalf("bad dispatch: %+v", m)
	}
	_, err = j.ArmServiceDispatch(t.Context(), arm)
	assertStorageCode(t, err, v2contract.CodeRevisionConflict)
	provenance, err := control.ReadLaunchProvenance(t.Context(), j, f.b.RunID, f.b.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	if provenance.Generation == nil || *provenance.Generation != f.b.Generation || provenance.LaunchTokenSHA256 != in.LaunchTokenSHA256 || provenance.NonceSHA256 != in.NonceSHA256 || !provenance.ExecutionDeadline.Equal(in.ExecutionDeadline) {
		t.Fatalf("dispatch pin differs from actual provenance reader: %+v", provenance)
	}
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		for _, table := range []string{"attempts", "dispatch_outbox", "service_dispatch_claims"} {
			var n int
			if err := tx.QueryRowContext(t.Context(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
				return err
			}
			if n != 1 {
				t.Errorf("%s count=%d want1", table, n)
			}
		}
		var first string
		if err := tx.QueryRowContext(t.Context(), `SELECT first_start_at FROM run_envelopes`).Scan(&first); err != nil {
			return err
		}
		if first != now.Format(time.RFC3339Nano) {
			t.Errorf("first start changed: %s", first)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func tableImages(t *testing.T, j *journal.Journal) map[string]string {
	t.Helper()
	images := map[string]string{}
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		for _, table := range []string{"runs", "attempts", "journal", "producers", "run_control_revisions", "policies", "declarations", "trust_grants", "run_envelopes", "reservations", "run_assignments", "service_admission_bindings", "service_admission_consumptions", "service_pending_dispatch", "service_dispatch_claims", "dispatch_outbox", "prepared_runs", "operations", "bucket_state"} {
			rows, err := tx.QueryContext(t.Context(), `SELECT * FROM `+table+` ORDER BY rowid`) // #nosec G202 -- table comes only from the fixed literal fixture relation list above.
			if err != nil {
				return err
			}
			cols, err := rows.Columns()
			if err != nil {
				_ = rows.Close()
				return err
			}
			var image [][]any
			for rows.Next() {
				values := make([]any, len(cols))
				pointers := make([]any, len(cols))
				for i := range values {
					pointers[i] = &values[i]
				}
				if err := rows.Scan(pointers...); err != nil {
					_ = rows.Close()
					return err
				}
				image = append(image, values)
			}
			err = rows.Err()
			_ = rows.Close()
			if err != nil {
				return err
			}
			raw, err := json.Marshal(image)
			if err != nil {
				return err
			}
			images[table] = string(raw)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return images
}
func assertUnchangedTables(t *testing.T, before, after map[string]string) {
	t.Helper()
	for table, old := range before {
		if after[table] != old {
			t.Errorf("failed transaction changed %s", table)
		}
	}
}
func installStorageFailure(t *testing.T, j *journal.Journal, table, action, condition string) {
	t.Helper()
	when := ""
	if condition != "" {
		when = " WHEN " + condition
	}
	query := `CREATE TRIGGER injected_failure BEFORE ` + action + ` ON ` + table + when + ` BEGIN SELECT RAISE(ABORT,'synthetic storage barrier'); END` // #nosec G202 -- trusted test-only literal fixture identifiers/actions/conditions, never external input.
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error { _, err := tx.ExecContext(t.Context(), query); return err }); err != nil {
		t.Fatal(err)
	}
}
func testAdmissionStatementRollback(t *testing.T) {
	t.Helper()
	barriers := []struct{ name, table, action, condition string }{
		{"claim", "operations", "INSERT", ""}, {"run", "runs", "INSERT", ""}, {"created event", "journal", "INSERT", "NEW.type='run.created'"}, {"run state", "runs", "UPDATE", ""}, {"state event", "journal", "INSERT", "NEW.type='run.state_changed'"}, {"policy", "policies", "INSERT", ""}, {"declaration", "declarations", "INSERT", ""}, {"native trust", "trust_grants", "INSERT", ""}, {"envelope", "run_envelopes", "INSERT", ""}, {"quota hold", "reservations", "INSERT", ""}, {"admission event", "journal", "INSERT", "NEW.type='admission.decided'"}, {"assignment", "run_assignments", "INSERT", ""}, {"assignment event", "journal", "INSERT", "NEW.type='service.run_assigned'"}, {"consumption", "service_admission_consumptions", "INSERT", ""}, {"pending intent", "service_pending_dispatch", "INSERT", ""}, {"consume CAS", "prepared_runs", "UPDATE", "NEW.consumed=1"}, {"ack", "operations", "UPDATE", "NEW.state='done'"}, {"producer projection", "producers", "INSERT", ""}, {"revision projection", "run_control_revisions", "UPDATE", ""},
	}
	for _, barrier := range barriers {
		t.Run(barrier.name, func(t *testing.T) {
			t.Parallel()
			j := storageJournal(t)
			adoptStorageFixture(t, j)
			f := preparationFixtureFor(t, false)
			c := preparedCommit(t, j, f)
			before := tableImages(t, j)
			installStorageFailure(t, j, barrier.table, barrier.action, barrier.condition)
			_, err := j.CommitServiceAdmission(t.Context(), c)
			assertStorageCode(t, err, v2contract.CodePersistenceUnavailable)
			assertUnchangedTables(t, before, tableImages(t, j))
		})
	}
}
func testArmingStatementRollback(t *testing.T) {
	t.Helper()
	barriers := []struct{ name, table, action, condition string }{
		{"run state", "runs", "UPDATE", ""}, {"state event", "journal", "INSERT", "NEW.type='run.state_changed'"}, {"attempt", "attempts", "INSERT", ""}, {"launch intent", "journal", "INSERT", "NEW.type='attempt.launch_intent_recorded'"}, {"accounting", "run_envelopes", "UPDATE", ""}, {"admission pin", "journal", "INSERT", "NEW.type='attempt.admission_pinned'"}, {"outbox", "dispatch_outbox", "INSERT", ""}, {"claim", "service_dispatch_claims", "INSERT", ""}, {"pending CAS", "service_pending_dispatch", "UPDATE", ""}, {"producer projection", "producers", "UPDATE", ""}, {"revision projection", "run_control_revisions", "UPDATE", ""},
	}
	for _, barrier := range barriers {
		t.Run(barrier.name, func(t *testing.T) {
			t.Parallel()
			j := storageJournal(t)
			adoptStorageFixture(t, j)
			f := preparationFixtureFor(t, false)
			c := preparedCommit(t, j, f)
			a, err := j.CommitServiceAdmission(t.Context(), c)
			if err != nil {
				t.Fatal(err)
			}
			snapshot := snapshotForArm(t, j, f)
			now := time.Now().UTC()
			arm, err := journal.NewDispatchArm(journal.DispatchArmInput{Admission: a, ExpectedPendingRevision: 1, SnapshotEventID: snapshot, BindingSHA256: f.input.Lookup.BindingSHA256, LaunchTokenSHA256: strings.Repeat("c", 64), NonceSHA256: strings.Repeat("d", 64), LadderVersion: "synthetic-ladder-v1", PolicyRevision: f.b.PolicyRevision, Generation: f.b.Generation, ObservedAt: now, ExecutionDeadline: now.Add(time.Duration(f.b.Decision.ResolvedCeilings.ExecutionNanos))})
			if err != nil {
				t.Fatal(err)
			}
			before := tableImages(t, j)
			installStorageFailure(t, j, barrier.table, barrier.action, barrier.condition)
			_, err = j.ArmServiceDispatch(t.Context(), arm)
			assertStorageCode(t, err, v2contract.CodePersistenceUnavailable)
			assertUnchangedTables(t, before, tableImages(t, j))
		})
	}
}

func confirmationFor(f preparationFixture, op, id string) journal.ConfirmationV1 {
	return journal.ConfirmationV1{Operation: journal.PreparationOperation{OperationID: op, ParamsSHA256: strings.Repeat("b", 64)}, ChallengeID: id, BindingSHA256: f.input.Lookup.BindingSHA256, PeerID: f.b.PeerID, RootID: f.b.RootID, ContextEpoch: f.b.ContextEpoch, Generation: f.b.Generation, Decision: ab.RetentionApprove, ObservedAt: f.now}
}
func TestRetainedPreparationStages(t *testing.T) { // I02/I03/I20 (v2 §§6/8): only the complete bound answer set is ready.
	t.Parallel()
	t.Run("complete answers and historical replay", func(t *testing.T) {
		t.Parallel()
		f := preparationFixtureFor(t, true)
		j := storageJournal(t)
		first, err := j.StoreRetainedPreparation(t.Context(), f.input)
		if err != nil {
			t.Fatal(err)
		}
		l := f.input.Lookup
		l.ExpectedRevision = first.Revision
		answer := confirmationFor(f, "native_op", "native_1")
		ready, err := j.ConfirmPreparation(t.Context(), l, answer)
		if err != nil {
			t.Fatal(err)
		}
		if !ready.Ready || ready.Revision != 2 {
			t.Fatalf("last answer state=%+v", ready)
		}
		replay, err := j.ConfirmPreparation(t.Context(), l, answer)
		if err != nil || replay != ready {
			t.Fatalf("lost acknowledgement replay changed: %+v %v", replay, err)
		}
		historical, err := j.StoreRetainedPreparation(t.Context(), f.input)
		if err != nil || historical != first {
			t.Fatalf("initial ack was rewritten: %+v %v", historical, err)
		}
		l.ExpectedRevision = 2
		answer.Operation.OperationID = "duplicate_answer_op"
		duplicate, err := j.ConfirmPreparation(t.Context(), l, answer)
		if err != nil || duplicate != ready {
			t.Fatalf("duplicate answer changed revision: %+v %v", duplicate, err)
		}
		p, err := j.ReadConfirmedPreparation(t.Context(), l)
		if err != nil {
			t.Fatal(err)
		}
		m, err := p.Metadata()
		if err != nil || m != ready {
			t.Fatalf("confirmed metadata=%+v %v", m, err)
		}
	})
	cases := []struct {
		name   string
		change func(*journal.PreparationLookup, *journal.ConfirmationV1)
		code   v2contract.Code
	}{
		{"stale revision", func(l *journal.PreparationLookup, _ *journal.ConfirmationV1) { l.ExpectedRevision = 0 }, v2contract.CodeRevisionConflict},
		{"foreign peer", func(l *journal.PreparationLookup, _ *journal.ConfirmationV1) { l.PeerID = "foreign" }, v2contract.CodePermissionDenied},
		{"foreign root", func(l *journal.PreparationLookup, _ *journal.ConfirmationV1) { l.RootID = "foreign" }, v2contract.CodePermissionDenied},
		{"stale generation", func(l *journal.PreparationLookup, _ *journal.ConfirmationV1) { l.Generation++ }, v2contract.CodeRevisionConflict},
		{"stale context", func(l *journal.PreparationLookup, _ *journal.ConfirmationV1) { l.ContextEpoch = "changed" }, v2contract.CodeDependencyStale},
		{"expiry", func(l *journal.PreparationLookup, _ *journal.ConfirmationV1) {
			l.ObservedAt = l.ObservedAt.Add(5 * time.Minute)
		}, v2contract.CodeDependencyStale},
		{"unknown challenge", func(_ *journal.PreparationLookup, a *journal.ConfirmationV1) { a.ChallengeID = "unknown" }, v2contract.CodeInvalidContract},
		{"rejected", func(_ *journal.PreparationLookup, a *journal.ConfirmationV1) { a.Decision = ab.RetentionReject }, v2contract.CodePolicyIneligible},
		{"altered answer binding", func(_ *journal.PreparationLookup, a *journal.ConfirmationV1) {
			a.BindingSHA256 = strings.Repeat("e", 64)
		}, v2contract.CodeInvalidContract},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := preparationFixtureFor(t, true)
			j := storageJournal(t)
			if _, err := j.StoreRetainedPreparation(t.Context(), f.input); err != nil {
				t.Fatal(err)
			}
			l := f.input.Lookup
			l.ExpectedRevision = 1
			a := confirmationFor(f, "native_op", "native_1")
			tc.change(&l, &a)
			before := tableImages(t, j)
			_, err := j.ConfirmPreparation(t.Context(), l, a)
			assertStorageCode(t, err, tc.code)
			assertUnchangedTables(t, before, tableImages(t, j))
		})
	}
	for _, barrier := range []struct{ name, table, action string }{{"preparation", "prepared_runs", "INSERT"}, {"binding", "service_admission_bindings", "INSERT"}, {"ack", "operations", "UPDATE"}} {
		t.Run("retention rollback "+barrier.name, func(t *testing.T) {
			t.Parallel()
			f := preparationFixtureFor(t, false)
			j := storageJournal(t)
			before := tableImages(t, j)
			installStorageFailure(t, j, barrier.table, barrier.action, "")
			_, err := j.StoreRetainedPreparation(t.Context(), f.input)
			assertStorageCode(t, err, v2contract.CodePersistenceUnavailable)
			assertUnchangedTables(t, before, tableImages(t, j))
		})
	}
	for _, barrier := range []struct{ name, table, action string }{{"claim", "operations", "INSERT"}, {"answer", "prepared_runs", "UPDATE"}, {"ack", "operations", "UPDATE"}} {
		t.Run("answer rollback "+barrier.name, func(t *testing.T) {
			t.Parallel()
			f := preparationFixtureFor(t, true)
			j := storageJournal(t)
			if _, err := j.StoreRetainedPreparation(t.Context(), f.input); err != nil {
				t.Fatal(err)
			}
			l := f.input.Lookup
			l.ExpectedRevision = 1
			before := tableImages(t, j)
			installStorageFailure(t, j, barrier.table, barrier.action, "")
			_, err := j.ConfirmPreparation(t.Context(), l, confirmationFor(f, "native_op", "native_1"))
			assertStorageCode(t, err, v2contract.CodePersistenceUnavailable)
			assertUnchangedTables(t, before, tableImages(t, j))
		})
	}
}

// legacyAcceptancePin invokes the actual admission pin producer; the journal
// implementation cannot import it because supervisor reaches back into journal.
func legacyAcceptancePin(t *testing.T, f preparationFixture) acceptance.Pin {
	t.Helper()
	d := admission.Decision{TaskID: f.b.TaskID, ConfigDigest: f.b.Decision.ConfigDigest, NoChecks: f.b.Decision.NoChecks, KeepGoing: f.b.Decision.KeepGoing}
	d.Task.SHA256 = f.b.Decision.Task.SHA256
	d.Profile.Name = f.b.Decision.Profile.Name
	d.Profile.Contained = f.b.Decision.Profile.Contained
	for _, ch := range f.b.Decision.ProjectConfig.Checks {
		d.ProjectConfig.Checks = append(d.ProjectConfig.Checks, admission.CheckConfig{Name: ch.Name, Argv: ch.Argv, FailOnOutput: ch.FailOnOutput, Timeout: ch.Timeout})
	}
	payload, err := supervisor.NewAdmissionPayload(d)
	if err != nil {
		t.Fatal(err)
	}
	return payload.AcceptancePin
}
func TestClosedAdmissionConstructorsAndGetters(t *testing.T) { // I02/I20 (v2 §§6/8).
	t.Parallel()
	t.Run("constructor mutation matrix", testClosedConstructorMutationMatrix)
	f := preparationFixtureFor(t, false)
	j := storageJournal(t)
	if _, err := j.StoreRetainedPreparation(t.Context(), f.input); err != nil {
		t.Fatal(err)
	}
	l := f.input.Lookup
	l.ExpectedRevision = 1
	p, err := j.ReadConfirmedPreparation(t.Context(), l)
	if err != nil {
		t.Fatal(err)
	}
	pin := legacyAcceptancePin(t, f)
	in := journal.AdmissionCommitInput{Preparation: p, Lookup: l, OperationID: "start_op", CanonicalBinding: f.input.CanonicalBinding, AcceptancePin: pin, Policy: acceptance.P1()}
	c, err := journal.NewAdmissionCommit(in)
	if err != nil {
		t.Fatal(err)
	}
	in.CanonicalBinding[0] = '!'
	in.AcceptancePin.CheckTimeouts["vet"] = "999h"
	in.Policy.Prompts["mode"] = "mutated"
	s, err := c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if s.CanonicalBinding[0] != '{' || s.AcceptancePin.CheckTimeouts["vet"] != "10s" || s.Policy.Prompts["mode"] != "native-session" {
		t.Fatal("constructor retained mutable input")
	}
	s.CanonicalBinding[0] = '!'
	s.AcceptancePin.CheckTimeouts["vet"] = "999h"
	s.Policy.Prompts["mode"] = "mutated"
	s, err = c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if s.CanonicalBinding[0] != '{' || s.AcceptancePin.CheckTimeouts["vet"] != "10s" || s.Policy.Prompts["mode"] != "native-session" {
		t.Fatal("snapshot leaked mutable state")
	}
	raw, err := p.CanonicalBinding()
	if err != nil {
		t.Fatal(err)
	}
	raw[0] = '!'
	raw, err = p.CanonicalBinding()
	if err != nil || raw[0] != '{' {
		t.Fatal("confirmed getter leaked mutable state")
	}
	if _, err = (journal.ConfirmedPreparation{}).Metadata(); err == nil {
		t.Fatal("zero confirmed handle accepted")
	}
	if _, err = (journal.CommittedAdmission{}).Metadata(); err == nil {
		t.Fatal("zero admitted handle accepted")
	}
	if _, err = (journal.CommittedDispatch{}).Metadata(); err == nil {
		t.Fatal("zero dispatch handle accepted")
	}
	if _, err = (journal.AdmissionCommit{}).Snapshot(); err == nil {
		t.Fatal("zero commit accepted")
	}
	if _, err = (journal.DispatchArm{}).Snapshot(); err == nil {
		t.Fatal("zero arm accepted")
	}
}
func assertStorageCode(t *testing.T, err error, want v2contract.Code) {
	t.Helper()
	var ce *v2contract.ControlError
	if !errors.As(err, &ce) || ce.Code != want {
		t.Fatalf("error=%v want typed %s", err, want)
	}
	if e := ce.Validate(); e != nil {
		t.Fatal(e)
	}
}

func TestPreparationRevisionAndPolicyCommitment(t *testing.T) { // I02/I20 (v2 §§6/8).
	t.Parallel()
	t.Run("literal approval domain and tamper", testRetentionDigestColumns)
	t.Run("no mandatory retention authority", func(t *testing.T) {
		t.Parallel()
		f := preparationFixtureFor(t, false)
		b := f.b
		b.Challenges = []ab.ChallengeV1{}
		b.Decision.KeepGoing = false
		b.Request.KeepGoing = false
		b.Decision.RecordTrust = false
		b.Decision.RecordNativeTrust = false
		b.Request.TrustProjectConfig = ""
		b.Request.TrustNativeConfig = ""
		f = sealPreparationFixture(t, b, f.now)
		j := storageJournal(t)
		before := tableImages(t, j)
		_, err := j.StoreRetainedPreparation(t.Context(), f.input)
		assertStorageCode(t, err, v2contract.CodePolicyIneligible)
		assertUnchangedTables(t, before, tableImages(t, j))
	})
	for _, tc := range []struct {
		name   string
		mutate func(*journal.RetainedPreparationInput)
		code   v2contract.Code
	}{
		{"rejected retention", func(in *journal.RetainedPreparationInput) { in.Retention.Decision = ab.RetentionReject }, v2contract.CodePolicyIneligible},
		{"forged field set", func(in *journal.RetainedPreparationInput) { in.Retention.FieldSetSHA256 = strings.Repeat("e", 64) }, v2contract.CodePolicyIneligible},
		{"changed full policy", func(in *journal.RetainedPreparationInput) {
			in.Policy.Content.Rules[0].Classification = ab.RetentionForbidden
		}, v2contract.CodeInvalidContract},
		{"future policy", func(in *journal.RetainedPreparationInput) { in.Policy.Content.SchemaVersion = 2 }, v2contract.CodeCapabilityUnsupported},
		{"changed operation digest", func(in *journal.RetainedPreparationInput) { in.Operation.ParamsSHA256 = "not-a-digest" }, v2contract.CodeInvalidContract},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := preparationFixtureFor(t, false)
			j := storageJournal(t)
			tc.mutate(&f.input)
			before := tableImages(t, j)
			_, err := j.StoreRetainedPreparation(t.Context(), f.input)
			assertStorageCode(t, err, tc.code)
			assertUnchangedTables(t, before, tableImages(t, j))
		})
	}
	t.Run("forged ready", func(t *testing.T) {
		t.Parallel()
		f := preparationFixtureFor(t, true)
		j := storageJournal(t)
		if _, err := j.StoreRetainedPreparation(t.Context(), f.input); err != nil {
			t.Fatal(err)
		}
		if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
			_, err := tx.ExecContext(t.Context(), `UPDATE prepared_runs SET ready=1`)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		l := f.input.Lookup
		l.ExpectedRevision = 1
		before := tableImages(t, j)
		_, err := j.ReadConfirmedPreparation(t.Context(), l)
		assertStorageCode(t, err, v2contract.CodeInvalidContract)
		assertUnchangedTables(t, before, tableImages(t, j))
	})
	t.Run("old unbound preparation", func(t *testing.T) {
		t.Parallel()
		f := preparationFixtureFor(t, false)
		j := storageJournal(t)
		if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
			_, err := tx.ExecContext(t.Context(), `INSERT INTO prepared_runs(preparation_id,peer_id,generation,binding_sha256,revision,spec,challenges,confirmations,ready,consumed,expires_at) VALUES(?,?,?, ?,1,'{}','[]','[]',1,0,?)`, f.b.PreparationID, f.b.PeerID, f.b.Generation, f.input.Lookup.BindingSHA256, f.b.ExpiresAt)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		l := f.input.Lookup
		l.ExpectedRevision = 1
		_, err := j.ReadConfirmedPreparation(t.Context(), l)
		assertStorageCode(t, err, v2contract.CodeDependencyStale)
	})
	t.Run("protected execution policy content", func(t *testing.T) {
		t.Parallel()
		f := preparationFixtureFor(t, false)
		p := f.input.Policy
		p.Content.ExecutionPolicyRevision = "sha256:" + strings.Repeat("c", 64)
		sum, err := ab.RetentionPolicyDigestV1(p.Content)
		if err != nil {
			t.Fatal(err)
		}
		p.Revision = "sha256:" + hex.EncodeToString(sum[:])
		f.b.PolicyRevision = p.Revision
		f.b.Decision.Environment.PolicyRevision = p.Revision
		raw, err := ab.EncodeV1(f.b)
		if err != nil {
			t.Fatal(err)
		}
		digest, err := ab.DigestV1(f.b)
		if err != nil {
			t.Fatal(err)
		}
		f.input.CanonicalBinding = raw
		f.input.Policy = p
		f.input.Lookup.BindingSHA256 = hex.EncodeToString(digest[:])
		f.input.Retention.PolicyRevision = p.Revision
		f.input.Retention.BindingSHA256 = f.input.Lookup.BindingSHA256
		rows, err := ab.DeriveRetentionFieldsV1(f.b, p)
		if err != nil {
			t.Fatal(err)
		}
		fields, err := ab.RetentionFieldSetSHA256(rows)
		if err != nil {
			t.Fatal(err)
		}
		f.input.Retention.FieldSetSHA256 = hex.EncodeToString(fields[:])
		j := storageJournal(t)
		adoptStorageFixture(t, j)
		c := preparedCommit(t, j, f)
		before := tableImages(t, j)
		_, err = j.CommitServiceAdmission(t.Context(), c)
		assertStorageCode(t, err, v2contract.CodePolicyIneligible)
		assertUnchangedTables(t, before, tableImages(t, j))
	})
}

func testConcurrentAdmission(t *testing.T) {
	t.Helper()
	f := preparationFixtureFor(t, false)
	j := storageJournal(t)
	adoptStorageFixture(t, j)
	c := preparedCommit(t, j, f)
	start := make(chan struct{})
	results := make(chan journal.CommittedAdmission, 2)
	failures := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			<-start
			r, err := j.CommitServiceAdmission(t.Context(), c)
			results <- r
			failures <- err
		})
	}
	close(start)
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first journal.AdmissionMetadata
	for r := range results {
		m, err := r.Metadata()
		if err != nil {
			t.Fatal(err)
		}
		if first.OperationID == "" {
			first = m
		} else if m != first {
			t.Fatalf("duplicate concurrent admission changed immutable result: %+v != %+v", m, first)
		}
	}
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(t.Context(), `SELECT count(*) FROM runs`).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("concurrent start created %d runs", n)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func testCurrentQuotaRefusal(t *testing.T) {
	t.Helper()
	f := preparationFixtureFor(t, false)
	j := storageJournal(t)
	adoptStorageFixture(t, j)
	c := preparedCommit(t, j, f)
	d := f.b.Decision
	identity := ""
	if d.NativeAuth != nil {
		identity = d.NativeAuth.IdentityRef
	}
	bucket := admission.QuotaBucket(qualify.Record{Key: qualify.Key{Harness: d.Adapter.Harness, Surface: d.Adapter.Surface, EntitlementClass: d.Proposal.Billing.EntitlementClass}}, identity)
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.SetBucketExhausted(t.Context(), tx, bucket, time.Now().UTC().Format(time.RFC3339Nano), nil)
	}); err != nil {
		t.Fatal(err)
	}
	before := tableImages(t, j)
	_, err := j.CommitServiceAdmission(t.Context(), c)
	assertStorageCode(t, err, v2contract.CodeAllowanceExhausted)
	assertUnchangedTables(t, before, tableImages(t, j))
}
func testActiveRunConflict(t *testing.T) {
	t.Helper()
	f := preparationFixtureFor(t, false)
	j := storageJournal(t)
	adoptStorageFixture(t, j)
	c := preparedCommit(t, j, f)
	if err := supervisor.CreateRun(t.Context(), j, journal.RunRow{RunID: "active_other", AdapterID: "builtin/fake", SourceRepo: filepath.Join(t.TempDir(), "repo"), TaskSHA256: strings.Repeat("a", 64), BillingPosture: "local-scripted", ExecutionProfile: "trusted-host"}, supervisor.NewProducer("existing_fixture", 1)); err != nil {
		t.Fatal(err)
	}
	before := tableImages(t, j)
	_, err := j.CommitServiceAdmission(t.Context(), c)
	assertStorageCode(t, err, v2contract.CodeOwnershipUnresolved)
	assertUnchangedTables(t, before, tableImages(t, j))
}

// legacyFullDecision deliberately spells the actual legacy producer's source
// values independently of the journal's closed DTO-to-payload mapping.
func legacyFullDecision(t *testing.T, f preparationFixture) admission.Decision {
	t.Helper()
	d := admission.Decision{RunID: "run_1", TaskID: "task_1", AttemptID: "attempt_1", Host: "standalone", GitName: "Synthetic Fixture", GitEmail: "fixture@example.com", ConfigDigest: strings.Repeat("ab", 32), RepoIdentity: f.b.Decision.RepoIdentity, RecordTrust: true, NoChecks: false, KeepGoing: true, RecordNativeTrust: true, NativeConfigDigest: strings.Repeat("ab", 32), NativeTrustGrant: "native_config:sha256:" + strings.Repeat("ab", 32)}
	d.Task = admission.Task{SHA256: strings.Repeat("ab", 32), Title: "Synthetic <&> task", Content: []byte("twelve bytes")}
	d.Adapter = adapter.Descriptor{ID: "builtin/claudecode", Version: "1", Harness: "claude-code", Surface: "native-cli-structured"}
	d.Probe = adapter.Probe{Executable: f.b.Decision.Probe.Executable, Version: "2.1.284", SHA256: strings.Repeat("ab", 32), OS: runtime.GOOS, Arch: "amd64", Compatibility: "fixture-tested"}
	d.NativeAuth = &admission.AuthEvidence{LoggedIn: true, AuthMethod: "claude.ai", APIProvider: "firstParty", SubscriptionType: "pro", IdentityRef: strings.Repeat("ab", 32)}
	d.Snapshot = admission.Snapshot{SourceRepo: f.b.Decision.Snapshot.SourceRepo, Branch: "main", BaseRev: strings.Repeat("a", 40), Source: "--rev"}
	claim := contain.Claim{Name: "synthetic boundary", Version: "1", Enforced: true, Detail: "fixture <&>"}
	d.Profile = admission.Profile{Name: "restricted", Contained: true, Consent: "--execution-profile", Disclosure: "fixture", Boundary: &contain.Evidence{Profile: "restricted", OS: runtime.GOOS, Route: "builtin/claudecode", Boundary: "fixture", Version: "1", Owner: "fixture", Coverage: contain.Coverage{Filesystem: claim, Process: claim, Network: claim, Credential: claim}}}
	d.ProjectConfig.Checks = []admission.CheckConfig{{Name: "vet", Argv: []string{"go", "vet", "./..."}, FailOnOutput: true, Timeout: "10s"}, {Name: "test", Argv: []string{"go", "test", "./..."}, Timeout: "1m"}}
	d.ProjectConfig.Adapters.ClaudeCode.AllowedTools = []string{"Read", "Bash(go test:*)"}
	d.Proposal.Billing = adapter.BillingPosture{Mode: "subscription-declared", CredentialProvenance: "native-login", EntitlementClass: "included-plan", EntitlementSource: "user_declared", PaidContinuation: "unknown", PaidContinuationUserDeclaration: "disabled", Qualified: false, G05: "not-passed"}
	d.Proposal.Overrides = []adapter.ConfigDelta{{Kind: "env", Name: "FIXTURE", Value: "yes", Reason: "synthetic"}}
	d.Proposal.Manifest = adapter.ConfigManifest{Digests: map[string]string{"project": strings.Repeat("ab", 32), "user": strings.Repeat("ab", 32)}}
	evidence := "evidence_1"
	d.Proposal.Capabilities = adapter.CapabilityRecord{SchemaVersion: 1, AdapterID: "builtin/claudecode", AdapterVersion: "1", RuntimeVersion: "2.1.284", Mode: "headless", ExecutionSurface: "native-cli-structured", HarnessID: "claude-code", FidelityQualification: "fixture-tested", Billing: adapter.BillingCapabilities{Entitlement: "subscription", IncludedOnlySupported: adapter.Unknown, PaidOveragePrevention: adapter.Unsupported, EvidenceID: &evidence}, HostIntegration: adapter.HostIntegration{HerdrEmbedded: adapter.Unsupported, HerdrStateBridge: adapter.Unknown, NativeInteractiveAttach: adapter.Unsupported}, Capabilities: adapter.Capabilities{StructuredEvents: adapter.Supported, Resume: adapter.Unsupported, LiveSteer: adapter.Unknown, ApprovalBridge: adapter.Unsupported, UsageTokens: adapter.Supported, QuotaRemaining: adapter.Unknown, HardMonetaryLimit: adapter.Unsupported, NativeSubagents: adapter.Unsupported, Reconnect: adapter.Unknown}, Sandbox: adapter.Sandbox{Status: adapter.Supported, Scope: "fixture"}, Platform: adapter.Platform{OS: runtime.GOOS, WorkerDetachment: adapter.Supported, ProcessTreeOwnership: adapter.Supported}, Qualification: "synthetic fixture only"}
	return d
}
func testAdmissionRecordProducerGolden(t *testing.T) {
	t.Helper()
	f := preparationFixtureFor(t, false)
	j := storageJournal(t)
	adoptStorageFixture(t, j)
	c := preparedCommit(t, j, f)
	if _, err := j.CommitServiceAdmission(t.Context(), c); err != nil {
		t.Fatal(err)
	}
	expected, err := supervisor.NewAdmissionPayload(legacyFullDecision(t, f))
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	events, err := j.Events(t.Context(), f.b.RunID, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ev := range events {
		if ev.Type == "admission.decided" {
			found = true
			if string(ev.Payload) != string(want) {
				t.Fatalf("admission payload differs from actual legacy producer\ngot %s\nwant%s", ev.Payload, want)
			}
		}
	}
	if !found {
		t.Fatal("no canonical admission payload")
	}
}

func testRetentionDigestColumns(t *testing.T) {
	t.Helper()
	f := preparationFixtureFor(t, false)
	j := storageJournal(t)
	if _, err := j.StoreRetainedPreparation(t.Context(), f.input); err != nil {
		t.Fatal(err)
	}
	var approval, approvalDigest, policy, policyDigest string
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRowContext(t.Context(), `SELECT retention_approval,retention_approval_sha256,retention_policy,retention_policy_sha256 FROM service_admission_bindings`).Scan(&approval, &approvalDigest, &policy, &policyDigest)
	}); err != nil {
		t.Fatal(err)
	}
	// Literal prefix and independent hash construction are part of the v1 vector.
	expected := sha256.Sum256(append([]byte("MYTHHELM:retention-approval:v1\n"), []byte(approval)...))
	plain := sha256.Sum256([]byte(approval))
	policySum := sha256.Sum256([]byte(policy))
	if approvalDigest != hex.EncodeToString(expected[:]) || approvalDigest == hex.EncodeToString(plain[:]) || policyDigest != hex.EncodeToString(policySum[:]) {
		t.Fatal("approval domain or policy byte integrity digest changed")
	}
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(t.Context(), `DROP TRIGGER service_admission_bindings_immutable`); err != nil {
			return err
		}
		_, err := tx.ExecContext(t.Context(), `UPDATE service_admission_bindings SET retention_approval=json_set(retention_approval,'$.FieldSetSHA256',?)`, strings.Repeat("e", 64))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	l := f.input.Lookup
	l.ExpectedRevision = 1
	before := tableImages(t, j)
	_, err := j.ReadConfirmedPreparation(t.Context(), l)
	assertStorageCode(t, err, v2contract.CodeInvalidContract)
	assertUnchangedTables(t, before, tableImages(t, j))
}

func testActualStartIntentAndAcknowledgement(t *testing.T) {
	t.Helper()
	f := preparationFixtureFor(t, false)
	j := storageJournal(t)
	adoptStorageFixture(t, j)
	c := preparedCommit(t, j, f)
	admitted, err := j.CommitServiceAdmission(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	m, err := admitted.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(j.StateDir(), journal.DBName))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	body, err := json.Marshal(control.StartRun{SchemaVersion: 2, PreparationID: f.b.PreparationID, BindingSHA256: f.input.Lookup.BindingSHA256})
	if err != nil {
		t.Fatal(err)
	}
	in := control.Intent{OperationID: "actual_wire_start", Object: f.b.PreparationID, ExpectedRevision: 1, Generation: &f.b.Generation, Method: "run.start", Params: body}
	// The actual generic digest producer is exercised on an isolated synthetic
	// operation; its handler does no native work or service authority grant.
	if _, err := control.Execute(control.WithLedger(t.Context(), db), func(_ context.Context, _ control.Peer, _ control.Intent) (control.Result, error) {
		return control.Result{Revision: 1}, nil
	}, control.Peer{SameUser: true}, in); err != nil {
		t.Fatal(err)
	}
	var actualDigest, serviceDigest, storedAck string
	if err := db.QueryRowContext(t.Context(), `SELECT digest FROM operations WHERE operation_id='actual_wire_start'`).Scan(&actualDigest); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT digest,result FROM operations WHERE operation_id='start_op'`).Scan(&serviceDigest, &storedAck); err != nil {
		t.Fatal(err)
	}
	if actualDigest != serviceDigest {
		t.Fatalf("service start digest differs from actual control intent producer: %s != %s", serviceDigest, actualDigest)
	}
	ackBody, err := json.Marshal(control.RunAccepted{SchemaVersion: 2, OperationID: m.OperationID, RunID: m.RunID, AttemptID: m.AttemptID, Generation: m.Generation, Revision: m.Revision})
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(control.Result{OperationID: m.OperationID, Revision: int64(m.Revision), Body: ackBody}) // #nosec G115 -- m is minted from this fixture database final signed RunRevision.
	if err != nil {
		t.Fatal(err)
	}
	if storedAck != string(want) {
		t.Fatalf("service acceptance ack differs from actual wire codec: %s != %s", storedAck, want)
	}
}

func TestStorageFollowupPreparationMatrix(t *testing.T) { // I02/I12/I20 (v2 §§5/6/8).
	t.Parallel()
	t.Run("multiple additional answers and historical expiry", func(t *testing.T) {
		t.Parallel()
		f := preparationFixtureFor(t, true)
		b := f.b
		b.Challenges = append(b.Challenges, ab.ChallengeV1{ID: "project_1", Kind: "project-config", Text: "Trust the second exact synthetic configuration"})
		f = sealPreparationFixture(t, b, f.now)
		j := storageJournal(t)
		initial, err := j.StoreRetainedPreparation(t.Context(), f.input)
		if err != nil {
			t.Fatal(err)
		}
		l := f.input.Lookup
		l.ExpectedRevision = 1
		a := confirmationFor(f, "first_extra", "native_1")
		middle, err := j.ConfirmPreparation(t.Context(), l, a)
		if err != nil {
			t.Fatal(err)
		}
		if middle.Ready || middle.Revision != 2 {
			t.Fatalf("middle=%+v", middle)
		}
		next := l
		next.ExpectedRevision = 2
		last, err := j.ConfirmPreparation(t.Context(), next, confirmationFor(f, "last_extra", "project_1"))
		if err != nil {
			t.Fatal(err)
		}
		if !last.Ready || last.Revision != 3 {
			t.Fatalf("final=%+v", last)
		}
		expired := l
		expired.ObservedAt = expired.ObservedAt.Add(5 * time.Minute)
		before := tableImages(t, j)
		historical, err := j.ConfirmPreparation(t.Context(), expired, a)
		if err != nil || historical != middle {
			t.Fatalf("historical middle acknowledgement=%+v %v", historical, err)
		}
		original := f.input
		original.Lookup.ObservedAt = expired.ObservedAt
		retained, err := j.StoreRetainedPreparation(t.Context(), original)
		if err != nil || retained != initial {
			t.Fatalf("historical initial acknowledgement=%+v %v", retained, err)
		}
		assertUnchangedTables(t, before, tableImages(t, j))
		fresh := a
		fresh.Operation.OperationID = "new_after_expiry"
		_, err = j.ConfirmPreparation(t.Context(), expired, fresh)
		assertStorageCode(t, err, v2contract.CodeDependencyStale)
		assertUnchangedTables(t, before, tableImages(t, j))
		expired.PeerID = "foreign"
		_, err = j.ConfirmPreparation(t.Context(), expired, a)
		assertStorageCode(t, err, v2contract.CodePermissionDenied)
		assertUnchangedTables(t, before, tableImages(t, j))
	})
	t.Run("consumed historical acknowledgement is not fresh consent", func(t *testing.T) {
		t.Parallel()
		f := preparationFixtureFor(t, true)
		j := storageJournal(t)
		adoptStorageFixture(t, j)
		first, err := j.StoreRetainedPreparation(t.Context(), f.input)
		if err != nil {
			t.Fatal(err)
		}
		l := f.input.Lookup
		l.ExpectedRevision = 1
		a := confirmationFor(f, "accepted_extra", "native_1")
		ready, err := j.ConfirmPreparation(t.Context(), l, a)
		if err != nil {
			t.Fatal(err)
		}
		current := l
		current.ExpectedRevision = 2
		p, err := j.ReadConfirmedPreparation(t.Context(), current)
		if err != nil {
			t.Fatal(err)
		}
		c, err := journal.NewAdmissionCommit(journal.AdmissionCommitInput{Preparation: p, Lookup: current, OperationID: "consume_op", CanonicalBinding: f.input.CanonicalBinding, AcceptancePin: legacyAcceptancePin(t, f), Policy: acceptance.P1()})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := j.CommitServiceAdmission(t.Context(), c); err != nil {
			t.Fatal(err)
		}
		before := tableImages(t, j)
		old, err := j.ConfirmPreparation(t.Context(), l, a)
		if err != nil || old != ready {
			t.Fatalf("consumed historical acknowledgement=%+v %v", old, err)
		}
		old, err = j.StoreRetainedPreparation(t.Context(), f.input)
		if err != nil || old != first {
			t.Fatalf("consumed retention acknowledgement=%+v %v", old, err)
		}
		fresh := a
		fresh.Operation.OperationID = "new_after_consumption"
		_, err = j.ConfirmPreparation(t.Context(), current, fresh)
		assertStorageCode(t, err, v2contract.CodeRevisionConflict)
		assertUnchangedTables(t, before, tableImages(t, j))
		_, err = j.ReadConfirmedPreparation(t.Context(), current)
		assertStorageCode(t, err, v2contract.CodeRevisionConflict)
		assertUnchangedTables(t, before, tableImages(t, j))
	})
}
func TestStorageConcurrentArming(t *testing.T) { // I12/I21 (v2 §6): one durable first-attempt claim, no second clock.
	t.Parallel()
	f := preparationFixtureFor(t, false)
	j := storageJournal(t)
	adoptStorageFixture(t, j)
	c := preparedCommit(t, j, f)
	admitted, err := j.CommitServiceAdmission(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := snapshotForArm(t, j, f)
	now := time.Now().UTC()
	arm, err := journal.NewDispatchArm(journal.DispatchArmInput{Admission: admitted, ExpectedPendingRevision: 1, SnapshotEventID: snapshot, BindingSHA256: f.input.Lookup.BindingSHA256, LaunchTokenSHA256: strings.Repeat("c", 64), NonceSHA256: strings.Repeat("d", 64), LadderVersion: "synthetic-ladder-v1", PolicyRevision: f.b.PolicyRevision, Generation: f.b.Generation, ObservedAt: now, ExecutionDeadline: now.Add(time.Duration(f.b.Decision.ResolvedCeilings.ExecutionNanos))})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { <-start; _, err := j.ArmServiceDispatch(t.Context(), arm); results <- err })
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			assertStorageCode(t, err, v2contract.CodeRevisionConflict)
		}
	}
	if success != 1 {
		t.Fatalf("successful arming transactions=%d want1", success)
	}
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		for _, table := range []string{"attempts", "dispatch_outbox", "service_dispatch_claims"} {
			var n int
			if err := tx.QueryRowContext(t.Context(), `SELECT count(*) FROM `+table).Scan(&n); err != nil {
				return err
			}
			if n != 1 {
				t.Errorf("%s count=%d want1", table, n)
			}
		}
		var first string
		if err := tx.QueryRowContext(t.Context(), `SELECT first_start_at FROM run_envelopes`).Scan(&first); err != nil {
			return err
		}
		if first != now.Format(time.RFC3339Nano) {
			t.Error("concurrent arming changed first launch clock")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func TestStoredRunSpecCacheFormat(t *testing.T) { // I09/I20 (v2 §§5/6): internal nanosecond cache is not a wire request.
	t.Parallel()
	f := preparationFixtureFor(t, false)
	j := storageJournal(t)
	if _, err := j.StoreRetainedPreparation(t.Context(), f.input); err != nil {
		t.Fatal(err)
	}
	var cache string
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		return tx.QueryRowContext(t.Context(), `SELECT spec FROM prepared_runs`).Scan(&cache)
	}); err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(f.b.Request)
	if err != nil {
		t.Fatal(err)
	}
	if cache != string(want) || !strings.HasPrefix(cache, `{"SchemaVersion":2,"Repository":`) || !strings.Contains(cache, `"ExecutionNanos":60000000000,"Repairs":0,"Replans":null`) {
		t.Fatal("internal leaf cache lost its literal version/nanosecond/null representation")
	}
	if _, err := v2contract.Decode[control.RunSpec]([]byte(cache)); err == nil {
		t.Fatal("internal cache was accepted by actual control wire decoder")
	}
	r := f.b.Request
	millis := int64(60000)
	repairs, retries := 0, 2
	wire := control.RunSpec{SchemaVersion: 2, Repository: r.Repository, TaskFile: r.TaskFile, TaskSHA256: r.TaskSHA256, Adapter: r.Adapter, Billing: r.Billing, ExecutionProfile: r.ExecutionProfile, Revision: r.Revision, Host: r.Host, Scenario: r.Scenario, UseCommitted: r.UseCommitted, NoChecks: r.NoChecks, KeepGoing: r.KeepGoing, TrustProjectConfig: r.TrustProjectConfig, TrustNativeConfig: r.TrustNativeConfig, DeclareEntitlement: r.DeclareEntitlement, StripCredentialEnv: r.StripCredentialEnv, AllowUntestedNativeVersion: r.AllowUntestedNativeVersion, Ceilings: control.RunCeilings{ExecutionMillis: &millis, Repairs: &repairs, TransportRetries: &retries}}
	encoded, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v2contract.Decode[control.RunSpec](encoded); err != nil {
		t.Fatal(err)
	}
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(t.Context(), `DROP TRIGGER prepared_runs_binding_identity`); err != nil {
			return err
		}
		_, err := tx.ExecContext(t.Context(), `UPDATE prepared_runs SET spec=?`, string(encoded))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	l := f.input.Lookup
	l.ExpectedRevision = 1
	before := tableImages(t, j)
	_, err = j.ReadConfirmedPreparation(t.Context(), l)
	assertStorageCode(t, err, v2contract.CodeInvalidContract)
	assertUnchangedTables(t, before, tableImages(t, j))
}

func testClosedConstructorMutationMatrix(t *testing.T) {
	t.Helper()
	f := preparationFixtureFor(t, false)
	j := storageJournal(t)
	adoptStorageFixture(t, j)
	c := preparedCommit(t, j, f)
	cases := []struct {
		name   string
		mutate func(*journal.AdmissionCommitInput)
		code   v2contract.Code
	}{
		{"zero preparation", func(in *journal.AdmissionCommitInput) { in.Preparation = journal.ConfirmedPreparation{} }, v2contract.CodeInvalidContract},
		{"empty operation", func(in *journal.AdmissionCommitInput) { in.OperationID = "" }, v2contract.CodeInvalidContract},
		{"foreign root", func(in *journal.AdmissionCommitInput) { in.Lookup.RootID = "foreign" }, v2contract.CodePermissionDenied},
		{"foreign peer", func(in *journal.AdmissionCommitInput) { in.Lookup.PeerID = "foreign" }, v2contract.CodePermissionDenied},
		{"changed epoch", func(in *journal.AdmissionCommitInput) { in.Lookup.ContextEpoch = "changed" }, v2contract.CodeDependencyStale},
		{"changed generation", func(in *journal.AdmissionCommitInput) { in.Lookup.Generation++ }, v2contract.CodeRevisionConflict},
		{"changed binding hash", func(in *journal.AdmissionCommitInput) { in.Lookup.BindingSHA256 = strings.Repeat("e", 64) }, v2contract.CodeRevisionConflict},
		{"stale revision", func(in *journal.AdmissionCommitInput) { in.Lookup.ExpectedRevision++ }, v2contract.CodeInvalidContract},
		{"malformed binding", func(in *journal.AdmissionCommitInput) { in.CanonicalBinding = []byte(`{}`) }, v2contract.CodeInvalidContract},
		{"changed pin", func(in *journal.AdmissionCommitInput) {
			in.AcceptancePin.Contract = "sha256:" + strings.Repeat("e", 64)
		}, v2contract.CodeInvalidContract},
		{"changed evaluator", func(in *journal.AdmissionCommitInput) {
			in.AcceptancePin.EvaluatorDigest = "sha256:" + strings.Repeat("e", 64)
		}, v2contract.CodeInvalidContract},
		{"changed timeout", func(in *journal.AdmissionCommitInput) { in.AcceptancePin.CheckTimeouts["vet"] = "20s" }, v2contract.CodeInvalidContract},
		{"changed protected policy", func(in *journal.AdmissionCommitInput) { in.Policy.Prompts["mode"] = "changed" }, v2contract.CodeInvalidContract},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in, err := c.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(&in)
			before := tableImages(t, j)
			_, err = journal.NewAdmissionCommit(in)
			assertStorageCode(t, err, tc.code)
			assertUnchangedTables(t, before, tableImages(t, j))
		})
	}
	mutate := func(in *journal.AdmissionCommitInput) {
		in.CanonicalBinding[0] = '!'
		for k := range in.AcceptancePin.CheckTimeouts {
			in.AcceptancePin.CheckTimeouts[k] = "changed"
		}
		for _, m := range []map[string]string{in.Policy.Prompts, in.Policy.Context, in.Policy.Review, in.Policy.Decomposition} {
			for k := range m {
				m[k] = "changed"
			}
		}
		for i := range in.Policy.Evidence {
			in.Policy.Evidence[i] = "changed"
		}
		in.Policy.EligibleCohorts = append(in.Policy.EligibleCohorts, "changed")
		in.Lookup.PeerID = "changed"
		in.OperationID = "changed"
	}
	in, err := c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	closed, err := journal.NewAdmissionCommit(in)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&in)
	for range 2 {
		out, err := closed.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		policy, err := json.Marshal(out.Policy)
		if err != nil {
			t.Fatal(err)
		}
		expected, err := json.Marshal(acceptance.P1())
		if err != nil {
			t.Fatal(err)
		}
		if string(out.CanonicalBinding) != string(f.input.CanonicalBinding) || string(policy) != string(expected) || out.AcceptancePin.CheckTimeouts["vet"] != "10s" || out.AcceptancePin.CheckTimeouts["test"] != "1m" || out.Lookup.PeerID != f.b.PeerID || out.OperationID != "start_op" {
			t.Fatal("constructor/getter retained a mutable byte, map, slice or value")
		}
		mutate(&out)
	}
	admitted, err := j.CommitServiceAdmission(t.Context(), closed)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := snapshotForArm(t, j, f)
	now := time.Now().UTC()
	base := journal.DispatchArmInput{Admission: admitted, ExpectedPendingRevision: 1, SnapshotEventID: snapshot, BindingSHA256: f.input.Lookup.BindingSHA256, LaunchTokenSHA256: strings.Repeat("c", 64), NonceSHA256: strings.Repeat("d", 64), LadderVersion: "synthetic-ladder-v1", PolicyRevision: f.b.PolicyRevision, Generation: f.b.Generation, ObservedAt: now, ExecutionDeadline: now.Add(time.Duration(f.b.Decision.ResolvedCeilings.ExecutionNanos))}
	for _, tc := range []struct {
		name   string
		mutate func(*journal.DispatchArmInput)
	}{
		{"zero admission", func(in *journal.DispatchArmInput) { in.Admission = journal.CommittedAdmission{} }},
		{"zero pending revision", func(in *journal.DispatchArmInput) { in.ExpectedPendingRevision = 0 }},
		{"missing snapshot", func(in *journal.DispatchArmInput) { in.SnapshotEventID = "" }},
		{"bad launch hash", func(in *journal.DispatchArmInput) { in.LaunchTokenSHA256 = "bad" }},
		{"bad nonce hash", func(in *journal.DispatchArmInput) { in.NonceSHA256 = "bad" }},
		{"missing ladder", func(in *journal.DispatchArmInput) { in.LadderVersion = "" }},
		{"changed generation", func(in *journal.DispatchArmInput) { in.Generation++ }},
		{"changed policy", func(in *journal.DispatchArmInput) { in.PolicyRevision = "changed" }},
		{"changed binding", func(in *journal.DispatchArmInput) { in.BindingSHA256 = strings.Repeat("e", 64) }},
		{"zero observed clock", func(in *journal.DispatchArmInput) { in.ObservedAt = time.Time{} }},
		{"invalid deadline", func(in *journal.DispatchArmInput) { in.ExecutionDeadline = in.ObservedAt }},
	} {
		t.Run("arm "+tc.name, func(t *testing.T) {
			in := base
			tc.mutate(&in)
			before := tableImages(t, j)
			_, err := journal.NewDispatchArm(in)
			assertStorageCode(t, err, v2contract.CodeInvalidContract)
			assertUnchangedTables(t, before, tableImages(t, j))
		})
	}
	arm, err := journal.NewDispatchArm(base)
	if err != nil {
		t.Fatal(err)
	}
	base.Admission = journal.CommittedAdmission{}
	base.SnapshotEventID = "changed"
	copied, err := arm.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	copied.Admission = journal.CommittedAdmission{}
	copied.SnapshotEventID = "changed"
	copied, err = arm.Snapshot()
	if err != nil || copied.SnapshotEventID != snapshot {
		t.Fatal("dispatch-arm snapshot leaked mutable values")
	}
	dispatch, err := j.ArmServiceDispatch(t.Context(), arm)
	if err != nil {
		t.Fatal(err)
	}
	m, err := dispatch.Metadata()
	if err != nil {
		t.Fatal(err)
	}
	m.Admission.RunID = "changed"
	again, err := dispatch.Metadata()
	if err != nil || again.Admission.RunID != f.b.RunID {
		t.Fatal("dispatch metadata leaked mutable values")
	}
}

// storageRunRevision reads the final durable revision after the API returns,
// independently of the returned acknowledgement and its cached handle.
func storageRunRevision(t *testing.T, j *journal.Journal, run string) uint64 {
	t.Helper()
	var revision uint64
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		var err error
		revision, err = journal.RunRevision(t.Context(), tx, run)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return revision
}

func TestStorageDistinctOperationsConsumePreparationOnce(t *testing.T) { // I12/I20 (v2 §6): distinct operations compete for one current preparation.
	t.Parallel()
	f := preparationFixtureFor(t, false)
	j := storageJournal(t)
	adoptStorageFixture(t, j)
	first := preparedCommit(t, j, f)
	secondInput, err := first.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	secondInput.OperationID = "competing_start"
	second, err := journal.NewAdmissionCommit(secondInput)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		admission journal.CommittedAdmission
		err       error
	}
	start, results := make(chan struct{}), make(chan result, 2)
	var wg sync.WaitGroup
	for _, commit := range []journal.AdmissionCommit{first, second} {
		wg.Add(1)
		go func(c journal.AdmissionCommit) {
			defer wg.Done()
			<-start
			a, e := j.CommitServiceAdmission(t.Context(), c)
			results <- result{a, e}
		}(commit)
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	winner := ""
	for r := range results {
		if r.err != nil {
			assertStorageCode(t, r.err, v2contract.CodeRevisionConflict)
			continue
		}
		successes++
		m, e := r.admission.Metadata()
		if e != nil {
			t.Fatal(e)
		}
		winner = m.OperationID
		if m.Revision != storageRunRevision(t, j, f.b.RunID) {
			t.Fatal("winner revision is not final durable revision")
		}
	}
	if successes != 1 {
		t.Fatalf("distinct operation successes=%d want1", successes)
	}
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		for _, table := range []string{"runs", "service_admission_consumptions", "service_pending_dispatch", "reservations", "run_assignments"} {
			var count int
			if err := tx.QueryRowContext(t.Context(), `SELECT count(*) FROM `+table).Scan(&count); err != nil {
				return err
			}
			if count != 1 {
				t.Errorf("%s count=%d want1", table, count)
			}
		}
		var starts int
		if err := tx.QueryRowContext(t.Context(), `SELECT count(*) FROM operations WHERE method='run.start' AND operation_id=?`, winner).Scan(&starts); err != nil {
			return err
		}
		if starts != 1 {
			t.Errorf("winner run.start operations=%d want1", starts)
		}
		var allStarts int
		if err := tx.QueryRowContext(t.Context(), `SELECT count(*) FROM operations WHERE method='run.start'`).Scan(&allStarts); err != nil {
			return err
		}
		if allStarts != 1 {
			t.Errorf("run.start operations=%d want1", allStarts)
		}
		for _, table := range []string{"service_admission_consumptions", "service_pending_dispatch", "run_assignments"} {
			var op string
			if err := tx.QueryRowContext(t.Context(), `SELECT operation_id FROM `+table).Scan(&op); err != nil {
				return err
			}
			if op != winner {
				t.Errorf("%s operation=%q want winner %q", table, op, winner)
			}
		}
		var consumed int
		if err := tx.QueryRowContext(t.Context(), `SELECT consumed FROM prepared_runs`).Scan(&consumed); err != nil {
			return err
		}
		if consumed != 1 {
			t.Errorf("preparation consumed=%d want1", consumed)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestStorageArmingRechecksDurableRows(t *testing.T) { // I02/I12/I20/I21 (v2 §§5/6/8): current authority and finite envelope, never cached authority.
	t.Parallel()
	cases := []struct {
		name, statement, snapshotChange, armChange string
		code                                       v2contract.Code
	}{
		{name: "owner reassigned", statement: `UPDATE run_assignments SET owner='other'`, code: v2contract.CodeOwnershipUnresolved},
		{name: "assignment operation changed", statement: `UPDATE run_assignments SET operation_id='retention_op'`, code: v2contract.CodeOwnershipUnresolved},
		{name: "pending settled", statement: `UPDATE service_pending_dispatch SET state='settled',revision=2`, code: v2contract.CodeRevisionConflict},
		{name: "pending expected revision mismatch", armChange: "pending revision", code: v2contract.CodeRevisionConflict},
		{name: "snapshot run mismatch", snapshotChange: "run", code: v2contract.CodeDependencyStale},
		{name: "snapshot wrong event type", snapshotChange: "type", code: v2contract.CodeDependencyStale},
		{name: "snapshot task mismatch", snapshotChange: "task", code: v2contract.CodeDependencyStale},
		{name: "snapshot generation mismatch", snapshotChange: "generation", code: v2contract.CodeDependencyStale},
		{name: "snapshot wrong base", snapshotChange: "base", code: v2contract.CodeDependencyStale},
		{name: "snapshot wrong branch", snapshotChange: "branch", code: v2contract.CodeDependencyStale},
		{name: "snapshot wrong workspace", snapshotChange: "workspace", code: v2contract.CodeDependencyStale},
		{name: "snapshot trust differs", snapshotChange: "trust", code: v2contract.CodeDependencyStale},
		{name: "snapshot precedes latest admission", snapshotChange: "order", code: v2contract.CodeDependencyStale},
		{name: "snapshot observed after arming observation", armChange: "observation", code: v2contract.CodeDependencyStale},
		{name: "project trust revoked", statement: `DELETE FROM trust_grants WHERE kind='project_config'`, code: v2contract.CodeDependencyStale},
		{name: "execution allowance changed", statement: `UPDATE run_envelopes SET execution_seconds=execution_seconds+1`, code: v2contract.CodeDependencyStale},
		{name: "repair allowance changed", statement: `UPDATE run_envelopes SET repairs=repairs+1`, code: v2contract.CodeDependencyStale},
		{name: "replan allowance changed", statement: `UPDATE run_envelopes SET replans=replans+1`, code: v2contract.CodeDependencyStale},
		{name: "transport allowance changed", statement: `UPDATE run_envelopes SET transport_retries=transport_retries+1`, code: v2contract.CodeDependencyStale},
		{name: "repair already used", statement: `UPDATE run_envelopes SET repairs_used=1`, code: v2contract.CodeDependencyStale},
		{name: "replan already used", statement: `UPDATE run_envelopes SET replans_used=1`, code: v2contract.CodeDependencyStale},
		{name: "transport already used", statement: `UPDATE run_envelopes SET transport_retries_seen=1`, code: v2contract.CodeDependencyStale},
		{name: "execution clock already started", statement: `UPDATE run_envelopes SET first_start_at='2026-01-01T00:00:00Z'`, code: v2contract.CodeDependencyStale},
		{name: "pauses already recorded", statement: `UPDATE run_envelopes SET pause_spans='[{"start":"2026-01-01T00:00:00Z"}]'`, code: v2contract.CodeDependencyStale},
		{name: "deadline differs from exact ceiling", armChange: "deadline", code: v2contract.CodeInvalidContract},
		{name: "run already stopped", statement: `UPDATE runs SET state='stopped'`, code: v2contract.CodeDependencyStale},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := preparationFixtureFor(t, false)
			j := storageJournal(t)
			adoptStorageFixture(t, j)
			c := preparedCommit(t, j, f)
			admitted, err := j.CommitServiceAdmission(t.Context(), c)
			if err != nil {
				t.Fatal(err)
			}
			snapshotFixture := f
			switch tc.snapshotChange {
			case "run":
				if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
					return journal.InsertRun(t.Context(), tx, journal.RunRow{RunID: "other_run", State: "created", AdapterID: "builtin/fake", SourceRepo: f.b.Decision.Snapshot.SourceRepo, TaskSHA256: strings.Repeat("a", 64), BillingPosture: "local-scripted", ExecutionProfile: "trusted-host", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
				}); err != nil {
					t.Fatal(err)
				}
				snapshotFixture.b.RunID = "other_run"
			case "task":
				snapshotFixture.b.TaskID = "other_task"
			case "generation":
				snapshotFixture.b.Generation++
			case "base":
				snapshotFixture.b.Decision.Snapshot.BaseRev = strings.Repeat("b", 40)
			case "branch":
				snapshotFixture.b.Decision.Snapshot.Branch = "other"
			case "workspace":
				snapshotFixture.b.Decision.Workdir = filepath.Join(t.TempDir(), "other")
			case "trust":
				snapshotFixture.b.Decision.RecordTrust = false
			}
			snapshot := snapshotForArm(t, j, snapshotFixture)
			if tc.snapshotChange == "type" {
				events, err := j.Events(t.Context(), f.b.RunID, 0)
				if err != nil {
					t.Fatal(err)
				}
				snapshot = ""
				for _, event := range events {
					if event.Type == "admission.decided" {
						snapshot = event.EventID
						break
					}
				}
				if snapshot == "" {
					t.Fatal("missing real admission event")
				}
			}
			if tc.snapshotChange == "order" {
				if err := j.Append(t.Context(), journal.Event{SchemaVersion: 2, EventID: "later_admission", RunID: f.b.RunID, TaskID: f.b.TaskID, ProducerID: "later_admission_fixture", ProducerSequence: 1, Generation: f.b.Generation, ObservedAt: time.Now().UTC(), Type: "admission.decided", Payload: json.RawMessage(`{}`)}, nil); err != nil {
					t.Fatal(err)
				}
			}
			now := time.Now().UTC()
			in := journal.DispatchArmInput{Admission: admitted, ExpectedPendingRevision: 1, SnapshotEventID: snapshot, BindingSHA256: f.input.Lookup.BindingSHA256, LaunchTokenSHA256: strings.Repeat("c", 64), NonceSHA256: strings.Repeat("d", 64), LadderVersion: "synthetic-ladder-v1", PolicyRevision: f.b.PolicyRevision, Generation: f.b.Generation, ObservedAt: now, ExecutionDeadline: now.Add(time.Duration(f.b.Decision.ResolvedCeilings.ExecutionNanos))}
			if tc.name == "pending settled" {
				in.ExpectedPendingRevision = 2
			}
			switch tc.armChange {
			case "pending revision":
				in.ExpectedPendingRevision = 2
			case "deadline":
				in.ExecutionDeadline = in.ExecutionDeadline.Add(time.Second)
			case "observation":
				in.ObservedAt = now.Add(-time.Second)
				in.ExecutionDeadline = in.ObservedAt.Add(time.Duration(f.b.Decision.ResolvedCeilings.ExecutionNanos))
			}
			arm, err := journal.NewDispatchArm(in)
			if err != nil {
				t.Fatalf("fixture did not reach durable-row consumer: %v", err)
			}
			if tc.statement != "" {
				if err := j.Transact(t.Context(), func(tx *sql.Tx) error { _, e := tx.ExecContext(t.Context(), tc.statement); return e }); err != nil {
					t.Fatalf("current-row mutation: %v", err)
				}
			}
			before := tableImages(t, j)
			_, err = j.ArmServiceDispatch(t.Context(), arm)
			assertStorageCode(t, err, tc.code)
			assertUnchangedTables(t, before, tableImages(t, j))
		})
	}
}

func TestReadPreparationStateCheckedCurrent(t *testing.T) {
	t.Parallel()
	for _, extra := range []bool{false, true} {
		t.Run(fmt.Sprintf("remaining-%t", extra), func(t *testing.T) {
			t.Parallel()
			f := preparationFixtureFor(t, extra)
			j := storageJournal(t)
			expected, err := j.StoreRetainedPreparation(t.Context(), f.input)
			if err != nil {
				t.Fatal(err)
			}
			lookup := f.input.Lookup
			lookup.ExpectedRevision = 0
			var before int
			if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
				return tx.QueryRowContext(t.Context(), `SELECT count(*) FROM operations`).Scan(&before)
			}); err != nil {
				t.Fatal(err)
			}
			current, err := j.ReadPreparationState(t.Context(), lookup)
			if err != nil || current != expected || current.Ready == extra {
				t.Fatalf("checked current=%+v,%v expected=%+v", current, err, expected)
			}
			for _, tc := range []struct {
				name   string
				change func(*journal.PreparationLookup)
			}{
				{"foreign peer", func(l *journal.PreparationLookup) { l.PeerID = "peer_foreign" }},
				{"foreign root", func(l *journal.PreparationLookup) { l.RootID = "root_foreign" }},
				{"stale context", func(l *journal.PreparationLookup) { l.ContextEpoch = "epoch_foreign" }},
				{"changed binding", func(l *journal.PreparationLookup) { l.BindingSHA256 = strings.Repeat("c", 64) }},
				{"expired", func(l *journal.PreparationLookup) { l.ObservedAt = f.now.Add(time.Hour) }},
			} {
				t.Run(tc.name, func(t *testing.T) {
					bad := lookup
					tc.change(&bad)
					got, err := j.ReadPreparationState(t.Context(), bad)
					if err == nil || got.PreparationID != "" || got.Revision != 0 {
						t.Fatalf("unproven metadata escaped: %+v %v", got, err)
					}
				})
			}
			var after int
			if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
				return tx.QueryRowContext(t.Context(), `SELECT count(*) FROM operations`).Scan(&after)
			}); err != nil {
				t.Fatal(err)
			}
			if before != after {
				t.Fatal("read-only metadata created an operation acknowledgement")
			}
		})
	}
}
