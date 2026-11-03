package journal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"time"

	"github.com/turbokast/mythhelm/internal/acceptance"
	"github.com/turbokast/mythhelm/internal/adapter"
	ab "github.com/turbokast/mythhelm/internal/admissionbinding"
	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/contain"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// StoreRetainedPreparation is the first durable preparation transaction. It
// persists no rejected/unclassified binding and creates no execution effects.
func (j *Journal) StoreRetainedPreparation(ctx context.Context, in RetainedPreparationInput) (PreparationState, error) {
	var result PreparationState
	in.CanonicalBinding = bytes.Clone(in.CanonicalBinding)
	in.Policy.Content.Rules = slices.Clone(in.Policy.Content.Rules)
	if j == nil || j.db == nil {
		return result, admissionStorageError(v2contract.CodeInvalidContract)
	}
	b, err := ab.DecodeV1(in.CanonicalBinding)
	if err != nil {
		return result, admissionLeafError(err)
	}
	if err = checkLookupScope(in.Lookup, b); err != nil {
		return result, err
	}
	if err = checkPreparationOperation(in.Operation); err != nil {
		return result, err
	}
	policy, err := ab.EncodeRetentionPolicyV1(in.Policy)
	if err != nil {
		return result, admissionLeafError(err)
	}
	approval, err := ab.EncodeRetentionApprovalV1(in.Retention)
	if err != nil {
		return result, admissionLeafError(err)
	}
	answerAt, err := time.Parse(time.RFC3339Nano, in.Retention.ObservedAt)
	if err != nil {
		return result, admissionLeafError(err)
	}
	a := ConfirmationV1{Operation: in.Operation, ChallengeID: in.Retention.ChallengeID, BindingSHA256: in.Retention.BindingSHA256, PeerID: b.PeerID, RootID: b.RootID, ContextEpoch: b.ContextEpoch, Generation: b.Generation, Decision: ab.RetentionApprove, ObservedAt: answerAt}
	ready, err := preparationReady(b, []ConfirmationV1{a}, in.Lookup.ObservedAt)
	if err != nil {
		return result, err
	}
	err = j.Transact(ctx, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepared_runs WHERE preparation_id=?`, b.PreparationID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			replay, found, err := preparationHistoricalReplay(ctx, tx, in.Operation, in.Lookup)
			if err != nil {
				return err
			}
			if found {
				result = replay
				return nil
			}
			r, err := readPreparation(ctx, tx, in.Lookup)
			if err != nil {
				return err
			}
			if r.state.Consumed {
				return admissionStorageError(v2contract.CodeRevisionConflict)
			}
			if !bytes.Equal(r.canonical, in.CanonicalBinding) || !bytes.Equal(mustJSON(r.policy), mustJSON(in.Policy)) || !bytes.Equal(mustJSON(r.retention), mustJSON(in.Retention)) {
				return admissionStorageError(v2contract.CodeRevisionConflict)
			}
			return admissionStorageError(v2contract.CodeRevisionConflict)
		}
		if err := checkLookupBinding(in.Lookup, b); err != nil {
			return err
		}
		if err := retainedPolicyCheck(b, in.Policy, in.Retention, in.Lookup.ObservedAt); err != nil {
			return err
		}
		if in.Lookup.ExpectedRevision != 0 {
			return admissionStorageError(v2contract.CodeRevisionConflict)
		}
		if err := claimPreparationOperation(ctx, tx, in.Operation, b, in.Lookup.ObservedAt); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO prepared_runs(preparation_id,peer_id,generation,binding_sha256,revision,spec,challenges,confirmations,ready,consumed,expires_at) VALUES(?,?,?,?,1,?,?,?,?,0,?)`, b.PreparationID, b.PeerID, b.Generation, in.Lookup.BindingSHA256, string(mustJSON(b.Request)), string(mustJSON(b.Challenges)), string(mustJSON([]ConfirmationV1{a})), boolInt(ready), b.ExpiresAt)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO service_admission_bindings(preparation_id,codec_version,canonical_binding,binding_sha256,root_id,peer_id,generation,context_epoch,policy_revision,task_artifact_sha256,task_artifact_ref,expires_at,retention_policy,retention_policy_sha256,retention_approval,retention_approval_sha256) VALUES(?,1,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, b.PreparationID, string(in.CanonicalBinding), in.Lookup.BindingSHA256, b.RootID, b.PeerID, b.Generation, b.ContextEpoch, b.PolicyRevision, b.Decision.Task.SHA256, b.Decision.Task.ArtifactRef, b.ExpiresAt, string(policy), storageDigest(policy), string(approval), retentionApprovalRecordDigest(approval)) //nolint:misspell // Deployed SQL column names are immutable schema identities.
		if err != nil {
			return err
		}
		result = PreparationState{PreparationID: b.PreparationID, BindingSHA256: in.Lookup.BindingSHA256, Revision: 1, Ready: ready}
		return finishPreparationOperation(ctx, tx, in.Operation, b, result, in.Lookup.ObservedAt)
	})
	if err != nil {
		return PreparationState{}, admissionTransactionError(err)
	}
	return result, nil
}

// ConfirmPreparation records one affirmative bound answer and its immutable
// acknowledgement in the same current-row transaction.
func (j *Journal) ConfirmPreparation(ctx context.Context, l PreparationLookup, a ConfirmationV1) (PreparationState, error) {
	var result PreparationState
	if j == nil || j.db == nil {
		return result, admissionStorageError(v2contract.CodeInvalidContract)
	}
	if err := checkPreparationOperation(a.Operation); err != nil {
		return result, err
	}
	if !validStorageID(a.ChallengeID) || !storageSHA(a.BindingSHA256) || a.Generation <= 0 || a.ObservedAt.IsZero() || (a.Decision != ab.RetentionApprove && a.Decision != ab.RetentionReject) {
		return result, admissionStorageError(v2contract.CodeInvalidContract)
	}

	err := j.Transact(ctx, func(tx *sql.Tx) error {
		if a.PeerID != l.PeerID || a.RootID != l.RootID {
			return admissionStorageError(v2contract.CodePermissionDenied)
		}
		if a.Generation != l.Generation {
			return admissionStorageError(v2contract.CodeRevisionConflict)
		}
		if a.ContextEpoch != l.ContextEpoch {
			return admissionStorageError(v2contract.CodeDependencyStale)
		}
		if a.BindingSHA256 != l.BindingSHA256 {
			return admissionStorageError(v2contract.CodeInvalidContract)
		}
		replay, found, err := preparationHistoricalReplay(ctx, tx, a.Operation, l)
		if err != nil {
			return err
		}
		if found {
			result = replay
			return nil
		}
		r, err := readPreparation(ctx, tx, l)
		if err != nil {
			return err
		}
		if r.state.Consumed {
			return admissionStorageError(v2contract.CodeRevisionConflict)
		}
		if a.PeerID != l.PeerID || a.RootID != l.RootID {
			return admissionStorageError(v2contract.CodePermissionDenied)
		}
		if a.Generation != l.Generation {
			return admissionStorageError(v2contract.CodeRevisionConflict)
		}
		if a.ContextEpoch != l.ContextEpoch {
			return admissionStorageError(v2contract.CodeDependencyStale)
		}
		if a.BindingSHA256 != l.BindingSHA256 || a.ObservedAt.IsZero() || a.ObservedAt.After(l.ObservedAt) || (a.Decision != ab.RetentionApprove && a.Decision != ab.RetentionReject) {
			return admissionStorageError(v2contract.CodeInvalidContract)
		}
		if a.Decision == ab.RetentionReject {
			return retentionStorageError()
		}
		known := false
		for _, ch := range r.binding.Challenges {
			if ch.ID == a.ChallengeID {
				known = true
				if ch.Kind == ab.RetentionChallengeKind {
					return retentionStorageError()
				}
			}
		}
		if !known {
			return admissionStorageError(v2contract.CodeInvalidContract)
		}
		if l.ExpectedRevision != r.state.Revision {
			return admissionStorageError(v2contract.CodeRevisionConflict)
		}
		exists := false
		for _, old := range r.answers {
			if a.ObservedAt.Before(old.ObservedAt) {
				return admissionStorageError(v2contract.CodeDependencyStale)
			}
			if old.ChallengeID == a.ChallengeID {
				exists = true
			}
		}
		if err := claimPreparationOperation(ctx, tx, a.Operation, r.binding, l.ObservedAt); err != nil {
			return err
		}
		result = r.state
		if !exists {
			r.answers = append(r.answers, a)
			ready, err := preparationReady(r.binding, r.answers, l.ObservedAt)
			if err != nil {
				return err
			}
			res, err := tx.ExecContext(ctx, `UPDATE prepared_runs SET confirmations=?,ready=?,revision=revision+1 WHERE preparation_id=? AND revision=? AND consumed=0`, string(mustJSON(r.answers)), boolInt(ready), l.PreparationID, l.ExpectedRevision)
			if err != nil {
				return err
			}
			if err = requireOneRow(res, "preparation", l.PreparationID); err != nil {
				return admissionStorageError(v2contract.CodeRevisionConflict)
			}
			result.Revision++
			result.Ready = ready
		}
		return finishPreparationOperation(ctx, tx, a.Operation, r.binding, result, l.ObservedAt)
	})
	if err != nil {
		return PreparationState{}, admissionTransactionError(err)
	}
	return result, nil
}

// ReadConfirmedPreparation validates the complete current durable answer set.
// It grants no native/trust authority and remains subject to fresh revalidation.
func (j *Journal) ReadConfirmedPreparation(ctx context.Context, l PreparationLookup) (ConfirmedPreparation, error) {
	var result ConfirmedPreparation
	if j == nil || j.db == nil {
		return result, admissionStorageError(v2contract.CodeInvalidContract)
	}
	err := j.Transact(ctx, func(tx *sql.Tx) error {
		r, err := readPreparation(ctx, tx, l)
		if err != nil {
			return err
		}
		if l.ExpectedRevision != r.state.Revision || r.state.Consumed {
			return admissionStorageError(v2contract.CodeRevisionConflict)
		}
		if !r.state.Ready {
			return admissionStorageError(v2contract.CodeDependencyStale)
		}
		result = ConfirmedPreparation{&confirmedPreparationState{metadata: r.state, binding: bytes.Clone(r.canonical), lookup: l}}
		return nil
	})
	if err != nil {
		return ConfirmedPreparation{}, admissionTransactionError(err)
	}
	return result, nil
}

// CommitServiceAdmission consumes a confirmed preparation and records the
// fixed admitted effects once. Snapshotting and worker creation are excluded.
func (j *Journal) CommitServiceAdmission(ctx context.Context, c AdmissionCommit) (CommittedAdmission, error) {
	var result CommittedAdmission
	if j == nil || j.db == nil || c.state == nil {
		return result, admissionStorageError(v2contract.CodeInvalidContract)
	}
	in, err := c.Snapshot()
	if err != nil {
		return result, err
	}
	b, err := ab.DecodeV1(in.CanonicalBinding)
	if err != nil {
		return result, admissionLeafError(err)
	}
	err = j.Transact(ctx, func(tx *sql.Tx) error {
		// A live journal clock, not a caller timestamp, gates new admission and quota.
		now := time.Now().UTC()
		if now.Before(in.Lookup.ObservedAt) {
			return admissionStorageError(v2contract.CodeDependencyStale)
		}
		replay, found, err := admissionReplay(ctx, tx, in, b)
		if err != nil {
			return err
		}
		if found {
			result = replay
			return nil
		}
		l := in.Lookup
		l.ObservedAt = now
		r, err := readPreparation(ctx, tx, l)
		if err != nil {
			return err
		}
		if r.state.Consumed || r.state.Revision != l.ExpectedRevision {
			return admissionStorageError(v2contract.CodeRevisionConflict)
		}
		if !r.state.Ready || !bytes.Equal(r.canonical, in.CanonicalBinding) {
			return admissionStorageError(v2contract.CodeDependencyStale)
		}
		if r.policy.Content.ExecutionPolicyRevision != v2contract.Digest(in.Policy) {
			return admissionStorageError(v2contract.CodePolicyIneligible)
		}
		// Re-derive fixed pin and protected policy in this same current-row snapshot.
		checked, err := NewAdmissionCommit(in)
		if err != nil {
			return err
		}
		if checked.state == nil {
			return admissionStorageError(v2contract.CodeInvalidContract)
		}
		var active int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM runs WHERE state IN ('created','admission','executing','verifying','applying','stopping','interrupted','recovering')`).Scan(&active); err != nil {
			return err
		}
		if active != 0 {
			return admissionStorageError(v2contract.CodeOwnershipUnresolved)
		}
		var collisions int
		if err := tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM runs WHERE run_id=?)+(SELECT count(*) FROM operations WHERE operation_id=?)`, b.RunID, in.OperationID).Scan(&collisions); err != nil {
			return err
		}
		if collisions != 0 {
			return admissionStorageError(v2contract.CodeRevisionConflict)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO operations(operation_id,method,object,digest,state,claimed_at) VALUES(?,'run.start',?,?,'claimed',?)`, in.OperationID, b.RunID, startOperationDigest(in, b), formatTime(now)); err != nil {
			return err
		}
		d := b.Decision
		if err := InsertRun(ctx, tx, RunRow{RunID: b.RunID, State: "created", AdapterID: d.Adapter.ID, SourceRepo: d.Snapshot.SourceRepo, SourceBranch: d.Snapshot.Branch, BaseRev: d.Snapshot.BaseRev, TaskSHA256: d.Task.SHA256, BillingPosture: d.Proposal.Billing.Mode, ExecutionProfile: d.Profile.Name, CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		if err := appendAdmissionEvent(ctx, tx, b, "run.created", "", storageStatePayload{"created", ""}, now); err != nil {
			return err
		}
		if err := SetRunState(ctx, tx, b.RunID, "admission", "", now); err != nil {
			return err
		}
		if err := appendAdmissionEvent(ctx, tx, b, "run.state_changed", "", storageStatePayload{"admission", ""}, now); err != nil {
			return err
		}
		if err := EnsurePolicy(ctx, tx, in.Policy); err != nil {
			if errors.Is(err, ErrPolicyConflict) {
				return admissionStorageError(v2contract.CodePolicyIneligible)
			}
			return err
		}
		if d.Declaration != nil {
			decl := d.Declaration
			at, err := time.Parse(time.RFC3339Nano, decl.DeclaredAt)
			if err != nil {
				return admissionLeafError(err)
			}
			if err = InsertDeclaration(ctx, tx, DeclarationRow{AdapterID: decl.AdapterID, PlanClass: decl.PlanClass, ExtraUsage: decl.ExtraUsage, IdentityRef: decl.IdentityRef, DeclaredAt: at}); err != nil {
				return err
			}
		}
		if d.RecordNativeTrust {
			if err := InsertTrustGrant(ctx, tx, "native_config", d.RepoIdentity, d.NativeConfigDigest, now); err != nil {
				return err
			}
		}
		ceilings := d.ResolvedCeilings
		if err := UpsertRunEnvelope(ctx, tx, EnvelopeRow{RunID: b.RunID, ExecutionSeconds: ceilings.ExecutionNanos / int64(time.Second), Repairs: ceilings.Repairs, Replans: ceilings.Replans, TransportRetries: ceilings.TransportRetries, UpdatedAt: formatTime(now)}); err != nil {
			return err
		}
		if _, err := ReserveRunQuota(ctx, tx, QuotaReservationInput{RunID: b.RunID, Bucket: bindingQuotaBucket(b), ExecutionNanos: ceilings.ExecutionNanos, ObservedAt: now}); err != nil {
			if errors.Is(err, billing.ErrAllowanceExhausted) {
				return admissionStorageError(v2contract.CodeAllowanceExhausted)
			}
			return err
		}
		if err := appendAdmissionEvent(ctx, tx, b, "admission.decided", "", storageAdmissionPayload{bindingAdmissionRecord(b), in.AcceptancePin}, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO run_assignments(run_id,owner,assigned_at,operation_id) VALUES(?,'supervisor',?,?)`, b.RunID, formatTime(now), in.OperationID); err != nil {
			return err
		}
		if err := appendAdmissionEvent(ctx, tx, b, "service.run_assigned", "", storageAssignmentPayload{2, b.RunID, in.OperationID, b.AttemptID, b.Generation, "supervisor"}, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO service_admission_consumptions(preparation_id,operation_id,run_id,attempt_id,consumed_generation,binding_sha256) VALUES(?,?,?,?,?,?)`, b.PreparationID, in.OperationID, b.RunID, b.AttemptID, b.Generation, l.BindingSHA256); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO service_pending_dispatch(operation_id,preparation_id,run_id,attempt_id,binding_sha256,generation) VALUES(?,?,?,?,?,?)`, in.OperationID, b.PreparationID, b.RunID, b.AttemptID, l.BindingSHA256, b.Generation); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE prepared_runs SET consumed=1 WHERE preparation_id=? AND consumed=0 AND ready=1 AND revision=?`, b.PreparationID, l.ExpectedRevision)
		if err != nil {
			return err
		}
		if err = requireOneRow(res, "preparation consumption", b.PreparationID); err != nil {
			return admissionStorageError(v2contract.CodeRevisionConflict)
		}
		rev, err := RunRevision(ctx, tx, b.RunID)
		if err != nil {
			return err
		}
		m := admissionMetadata(in, b, rev)
		ack := admissionAcknowledgement(m)
		res, err = tx.ExecContext(ctx, `UPDATE operations SET state='done',revision=?,result=?,finished_at=? WHERE operation_id=? AND state='claimed'`, rev, string(ack), formatTime(now), in.OperationID)
		if err != nil {
			return err
		}
		if err = requireOneRow(res, "admission operation", in.OperationID); err != nil {
			return err
		}
		result = CommittedAdmission{&committedAdmissionState{metadata: m, binding: bytes.Clone(in.CanonicalBinding)}}
		return nil
	})
	if err != nil {
		return CommittedAdmission{}, admissionTransactionError(err)
	}
	return result, nil
}

// ArmServiceDispatch creates the first actual attempt only after canonical
// snapshot evidence, preserving the original admission acknowledgement.
func (j *Journal) ArmServiceDispatch(ctx context.Context, a DispatchArm) (CommittedDispatch, error) {
	var result CommittedDispatch
	if j == nil || j.db == nil || a.state == nil {
		return result, admissionStorageError(v2contract.CodeInvalidContract)
	}
	in, err := a.Snapshot()
	if err != nil {
		return result, err
	}
	m := in.Admission.state.metadata
	b, err := ab.DecodeV1(in.Admission.state.binding)
	if err != nil {
		return result, admissionLeafError(err)
	}
	err = j.Transact(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC()
		if now.Before(in.ObservedAt) || !now.Before(in.ExecutionDeadline) {
			return admissionStorageError(v2contract.CodeDependencyStale)
		}
		l := PreparationLookup{PreparationID: m.PreparationID, PeerID: m.PeerID, RootID: m.RootID, BindingSHA256: m.BindingSHA256, ContextEpoch: m.ContextEpoch, Generation: m.Generation, ObservedAt: now}
		r, err := readPreparation(ctx, tx, l)
		if err != nil {
			return err
		}
		if !r.state.Ready || !r.state.Consumed || !bytes.Equal(r.canonical, in.Admission.state.binding) {
			return admissionStorageError(v2contract.CodeDependencyStale)
		}
		var prep, run, attempt, binding, state, operationResult, owner, assignedOp string
		var generation int64
		var revision uint64
		err = tx.QueryRowContext(ctx, `SELECT p.preparation_id,p.run_id,p.attempt_id,p.binding_sha256,p.generation,p.state,p.revision,o.result,a.owner,a.operation_id FROM service_pending_dispatch p JOIN service_admission_consumptions c ON c.operation_id=p.operation_id JOIN operations o ON o.operation_id=p.operation_id JOIN run_assignments a ON a.run_id=p.run_id WHERE p.operation_id=? AND c.preparation_id=p.preparation_id AND c.run_id=p.run_id AND c.attempt_id=p.attempt_id AND c.binding_sha256=p.binding_sha256 AND c.consumed_generation=p.generation AND o.method='run.start' AND o.object=p.run_id AND o.state='done'`, m.OperationID).Scan(&prep, &run, &attempt, &binding, &generation, &state, &revision, &operationResult, &owner, &assignedOp)
		if errors.Is(err, sql.ErrNoRows) {
			return admissionStorageError(v2contract.CodeDependencyStale)
		}
		if err != nil {
			return err
		}
		if prep != m.PreparationID || run != m.RunID || attempt != m.AttemptID || binding != m.BindingSHA256 || generation != in.Generation || state != "pending" || revision != in.ExpectedPendingRevision {
			return admissionStorageError(v2contract.CodeRevisionConflict)
		}
		if owner != "supervisor" || assignedOp != m.OperationID {
			return admissionStorageError(v2contract.CodeOwnershipUnresolved)
		}
		if operationResult != string(admissionAcknowledgement(m)) {
			return admissionStorageError(v2contract.CodeInvalidContract)
		}
		if err := checkSnapshotForArm(ctx, tx, b, in); err != nil {
			return err
		}
		d := b.Decision
		c := d.ResolvedCeilings
		var execution, repairs, replans, retries, repairsUsed, replansUsed, retriesUsed int64
		var first sql.NullString
		var pauses string
		err = tx.QueryRowContext(ctx, `SELECT execution_seconds,repairs,replans,transport_retries,first_start_at,repairs_used,replans_used,transport_retries_seen,pause_spans FROM run_envelopes WHERE run_id=?`, b.RunID).Scan(&execution, &repairs, &replans, &retries, &first, &repairsUsed, &replansUsed, &retriesUsed, &pauses)
		if err != nil {
			return err
		}
		if first.Valid || execution != c.ExecutionNanos/int64(time.Second) || repairs != c.Repairs || replans != c.Replans || retries != c.TransportRetries || repairsUsed != 0 || replansUsed != 0 || retriesUsed != 0 || pauses != "[]" {
			return admissionStorageError(v2contract.CodeDependencyStale)
		}
		if !in.ExecutionDeadline.Equal(in.ObservedAt.Add(time.Duration(c.ExecutionNanos))) {
			return admissionStorageError(v2contract.CodeInvalidContract)
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM attempts WHERE run_id=?`, b.RunID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return admissionStorageError(v2contract.CodeRevisionConflict)
		}
		current, err := CurrentRunState(ctx, tx, b.RunID)
		if err != nil {
			return err
		}
		if current != "admission" && current != "executing" {
			return admissionStorageError(v2contract.CodeDependencyStale)
		}
		if current != "executing" {
			if err := SetRunState(ctx, tx, b.RunID, "executing", "", now); err != nil {
				return err
			}
			if err := appendAdmissionEvent(ctx, tx, b, "run.state_changed", "", storageStatePayload{"executing", ""}, now); err != nil {
				return err
			}
		}
		if err := InsertAttempt(ctx, tx, AttemptRow{AttemptID: b.AttemptID, RunID: b.RunID, TaskID: b.TaskID, AttemptNumber: 1, State: "launch_intent_recorded", LaunchTokenSHA256: in.LaunchTokenSHA256, WorkspacePath: d.Workdir}); err != nil {
			return err
		}
		if err := appendAdmissionEvent(ctx, tx, b, "attempt.launch_intent_recorded", b.AttemptID, storageLaunchIntent{in.LaunchTokenSHA256}, in.ObservedAt); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE run_envelopes SET first_start_at=?,updated_at=? WHERE run_id=? AND first_start_at IS NULL AND repairs_used=0 AND replans_used=0 AND transport_retries_seen=0`, formatTime(in.ObservedAt), formatTime(now), b.RunID)
		if err != nil {
			return err
		}
		if err = requireOneRow(res, "first launch accounting", b.RunID); err != nil {
			return admissionStorageError(v2contract.CodeRevisionConflict)
		}
		provenance := storageLaunchProvenance{1, b.RunID, b.AttemptID, in.LaunchTokenSHA256, in.NonceSHA256, in.LadderVersion, &in.Generation, in.ExecutionDeadline}
		if err := appendAdmissionEvent(ctx, tx, b, "attempt.admission_pinned", b.AttemptID, storageAdmissionPin{in.LadderVersion, in.NonceSHA256, provenance}, in.ObservedAt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO dispatch_outbox(operation_id,preparation_id,peer_id,generation,binding_sha256,admitted_evidence_sha256,run_id,attempt_id,launch_token_sha256) VALUES(?,?,?,?,?,?,?,?,?)`, m.OperationID, m.PreparationID, m.PeerID, m.Generation, m.BindingSHA256, storageDigest(in.Admission.state.binding), m.RunID, m.AttemptID, in.LaunchTokenSHA256); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO service_dispatch_claims(operation_id,claim_generation,snapshot_event_id,launch_token_sha256,nonce_sha256,ladder_version,execution_deadline) VALUES(?,?,?,?,?,?,?)`, m.OperationID, in.Generation, in.SnapshotEventID, in.LaunchTokenSHA256, in.NonceSHA256, in.LadderVersion, formatTime(in.ExecutionDeadline)); err != nil {
			return err
		}
		res, err = tx.ExecContext(ctx, `UPDATE service_pending_dispatch SET state='armed',revision=revision+1 WHERE operation_id=? AND state='pending' AND revision=?`, m.OperationID, in.ExpectedPendingRevision)
		if err != nil {
			return err
		}
		if err = requireOneRow(res, "pending dispatch", m.OperationID); err != nil {
			return admissionStorageError(v2contract.CodeRevisionConflict)
		}
		rev, err := RunRevision(ctx, tx, b.RunID)
		if err != nil {
			return err
		}
		result = CommittedDispatch{&committedDispatchState{DispatchMetadata{Admission: m, SnapshotEventID: in.SnapshotEventID, LaunchTokenSHA256: in.LaunchTokenSHA256, NonceSHA256: in.NonceSHA256, LadderVersion: in.LadderVersion, ExecutionDeadline: in.ExecutionDeadline, Revision: rev}}}
		return nil
	})
	if err != nil {
		return CommittedDispatch{}, admissionTransactionError(err)
	}
	return result, nil
}

func retainedPolicyCheck(b ab.BindingV1, p ab.RetentionPolicyV1, a ab.RetentionApprovalV1, now time.Time) error {
	rows, err := ab.DeriveRetentionFieldsV1(b, p)
	if err != nil {
		return admissionLeafError(err)
	}
	if err := a.Validate(); err != nil {
		return admissionLeafError(err)
	}
	if len(rows) != len(p.Content.Rules) {
		return retentionStorageError()
	}
	for _, r := range rows {
		if r.Classification != ab.RetentionNonSecret || r.Provenance == ab.RetentionNativeAuth || r.Provenance == ab.RetentionTranscript || r.Provenance == ab.RetentionCredentialEnv {
			return retentionStorageError()
		}
	}
	sum, err := ab.DigestV1(b)
	if err != nil {
		return admissionLeafError(err)
	}
	fields, err := ab.RetentionFieldSetSHA256(rows)
	if err != nil {
		return admissionLeafError(err)
	}
	challenge := ""
	count := 0
	for _, ch := range b.Challenges {
		if ch.Kind == ab.RetentionChallengeKind {
			challenge = ch.ID
			count++
		}
	}
	observed, err := time.Parse(time.RFC3339Nano, a.ObservedAt)
	if err != nil {
		return admissionLeafError(err)
	}
	expires, err := time.Parse(time.RFC3339Nano, b.ExpiresAt)
	if err != nil {
		return admissionLeafError(err)
	}
	if count != 1 || a.ChallengeID != challenge || a.Decision != ab.RetentionApprove || a.PreparationID != b.PreparationID || a.BindingSHA256 != hex.EncodeToString(sum[:]) || a.FieldSetSHA256 != hex.EncodeToString(fields[:]) || a.PolicyRevision != p.Revision || b.PolicyRevision != p.Revision || p.Content.RootID != b.RootID || a.PeerID != b.PeerID || a.RootID != b.RootID || a.ContextEpoch != b.ContextEpoch || a.Generation != b.Generation || a.ExpiresAt != b.ExpiresAt || observed.After(now) || !now.Before(expires) {
		return retentionStorageError()
	}
	return nil
}

type preparationRow struct {
	binding   ab.BindingV1
	canonical []byte
	policy    ab.RetentionPolicyV1
	retention ab.RetentionApprovalV1
	state     PreparationState
	answers   []ConfirmationV1
}

// prepared_runs.spec is a schema11 internal cache: codec1 BindingV1.Request
// (RunSpecV2, schema2), encoded in leaf declaration order by encoding/json.
// It preserves nanoseconds, explicit zero and null optionals. This is neither
// control.RunSpec wire JSON nor a source of authority. Every read rederives the
// exact projection from the immutable binding and rejects any other format;
// historical schema9 preparations without that binding remain unavailable.
func readPreparation(ctx context.Context, tx *sql.Tx, l PreparationLookup) (preparationRow, error) {
	var r preparationRow
	var raw, policy, approval, answers string
	var storedBindingSHA, policySHA, approvalSHA, spec, challenges string
	var ready, consumed int
	err := tx.QueryRowContext(ctx, `SELECT b.canonical_binding,b.retention_policy,b.retention_approval,p.revision,p.ready,p.consumed,p.confirmations,b.binding_sha256,b.retention_policy_sha256,b.retention_approval_sha256,p.spec,p.challenges FROM prepared_runs p JOIN service_admission_bindings b USING(preparation_id) WHERE p.preparation_id=?`, l.PreparationID).Scan(&raw, &policy, &approval, &r.state.Revision, &ready, &consumed, &answers, &storedBindingSHA, &policySHA, &approvalSHA, &spec, &challenges)
	if errors.Is(err, sql.ErrNoRows) {
		return r, admissionStorageError(v2contract.CodeDependencyStale)
	}
	if err != nil {
		return r, admissionStorageError(v2contract.CodePersistenceUnavailable)
	}
	r.canonical = []byte(raw)
	r.binding, err = ab.DecodeV1(r.canonical)
	if err != nil {
		return r, admissionLeafError(err)
	}
	if err = checkLookupBinding(l, r.binding); err != nil {
		return r, err
	}
	if storedBindingSHA != l.BindingSHA256 || policySHA != storageDigest([]byte(policy)) || approvalSHA != retentionApprovalRecordDigest([]byte(approval)) || spec != string(mustJSON(r.binding.Request)) || challenges != string(mustJSON(r.binding.Challenges)) {
		return r, admissionStorageError(v2contract.CodeInvalidContract)
	}
	r.policy, err = ab.DecodeRetentionPolicyV1([]byte(policy))
	if err != nil {
		return r, admissionLeafError(err)
	}
	r.retention, err = ab.DecodeRetentionApprovalV1([]byte(approval))
	if err != nil {
		return r, admissionLeafError(err)
	}
	if err = retainedPolicyCheck(r.binding, r.policy, r.retention, l.ObservedAt); err != nil {
		return r, err
	}
	if !decodeStorageJSON([]byte(answers), &r.answers) {
		return r, admissionStorageError(v2contract.CodeInvalidContract)
	}
	computed, err := preparationReady(r.binding, r.answers, l.ObservedAt)
	if err != nil {
		return r, err
	}
	if ready != boolInt(computed) {
		return r, admissionStorageError(v2contract.CodeInvalidContract)
	}
	for _, answer := range r.answers {
		if err := checkPreparationOperation(answer.Operation); err != nil {
			return r, err
		}
		replay, found, err := preparationReplay(ctx, tx, answer.Operation, r.binding, l)
		if err != nil {
			return r, err
		}
		if !found || replay.Revision > r.state.Revision {
			return r, admissionStorageError(v2contract.CodeInvalidContract)
		}
		if answer.ChallengeID == r.retention.ChallengeID {
			at, err := time.Parse(time.RFC3339Nano, r.retention.ObservedAt)
			if err != nil {
				return r, admissionLeafError(err)
			}
			if !answer.ObservedAt.Equal(at) || answer.BindingSHA256 != r.retention.BindingSHA256 || answer.Decision != r.retention.Decision {
				return r, admissionStorageError(v2contract.CodeInvalidContract)
			}
		}
	}
	r.state.PreparationID = l.PreparationID
	r.state.BindingSHA256 = l.BindingSHA256
	r.state.Ready = computed
	r.state.Consumed = consumed != 0
	return r, nil
}
func preparationReady(b ab.BindingV1, answers []ConfirmationV1, now time.Time) (bool, error) {
	required := make(map[string]bool, len(b.Challenges))
	retention := false
	for _, ch := range b.Challenges {
		required[ch.ID] = false
		retention = retention || ch.Kind == ab.RetentionChallengeKind
	}
	if !retention || len(required) == 0 {
		return false, retentionStorageError()
	}
	for _, a := range answers {
		answered, known := required[a.ChallengeID]
		if !known || answered || a.Decision != ab.RetentionApprove || a.BindingSHA256 == "" || a.PeerID != b.PeerID || a.RootID != b.RootID || a.ContextEpoch != b.ContextEpoch || a.Generation != b.Generation || a.ObservedAt.IsZero() || a.ObservedAt.After(now) {
			return false, admissionStorageError(v2contract.CodeInvalidContract)
		}
		sum, err := ab.DigestV1(b)
		if err != nil {
			return false, admissionLeafError(err)
		}
		if a.BindingSHA256 != hex.EncodeToString(sum[:]) {
			return false, admissionStorageError(v2contract.CodeInvalidContract)
		}
		required[a.ChallengeID] = true
	}
	for _, yes := range required {
		if !yes {
			return false, nil
		}
	}
	return true, nil
}
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func checkPreparationOperation(op PreparationOperation) error {
	if !validStorageID(op.OperationID) || !storageSHA(op.ParamsSHA256) {
		return admissionStorageError(v2contract.CodeInvalidContract)
	}
	return nil
}
func admissionTransactionError(err error) error {
	if ce, ok := errors.AsType[*v2contract.ControlError](err); ok {
		if ce.Code == v2contract.CodePolicyIneligible && ce.Detail["mythhelm.retention/reason"] == "policy_ineligible" {
			return retentionStorageError()
		}
		switch ce.Code {
		case v2contract.CodeInvalidContract, v2contract.CodeRevisionConflict, v2contract.CodeDependencyStale, v2contract.CodePermissionDenied, v2contract.CodePolicyIneligible, v2contract.CodeCapabilityUnsupported, v2contract.CodePersistenceUnavailable, v2contract.CodeAllowanceExhausted, v2contract.CodeOwnershipUnresolved:
			return admissionStorageError(ce.Code)
		default:
			return admissionStorageError(v2contract.CodeInvalidContract)
		}
	}
	return admissionStorageError(v2contract.CodePersistenceUnavailable)
}

// These wire-only mirrors keep journal independent from control. Producer
// goldens compare complete persisted bytes against the actual control codec.
type preparationChallengeAck struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Text          string `json:"text"`
	BindingSHA256 string `json:"binding_sha256"`
}
type preparationBodyAck struct {
	SchemaVersion int                       `json:"schema_version"`
	PreparationID string                    `json:"preparation_id"`
	BindingSHA256 string                    `json:"binding_sha256"`
	Generation    int64                     `json:"generation"`
	Challenges    []preparationChallengeAck `json:"challenges"`
	Ready         bool                      `json:"ready"`
}
type preparationResultAck struct {
	OperationID string          `json:"operation_id"`
	Revision    int64           `json:"revision"`
	Body        json.RawMessage `json:"body,omitempty"`
}

func preparationAcknowledgement(op PreparationOperation, b ab.BindingV1, state PreparationState) []byte {
	challenges := make([]preparationChallengeAck, len(b.Challenges))
	for i, ch := range b.Challenges {
		challenges[i] = preparationChallengeAck{ch.ID, ch.Kind, ch.Text, state.BindingSHA256}
	}
	return mustJSON(preparationResultAck{op.OperationID, int64(state.Revision), mustJSON(preparationBodyAck{2, b.PreparationID, state.BindingSHA256, b.Generation, challenges, state.Ready})}) // #nosec G115 -- successful durable preparation revisions come from nonnegative SQLite INTEGER rows.
}
func claimPreparationOperation(ctx context.Context, tx *sql.Tx, op PreparationOperation, b ab.BindingV1, now time.Time) error {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM operations WHERE operation_id=?`, op.OperationID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return admissionStorageError(v2contract.CodeRevisionConflict)
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO operations(operation_id,method,object,digest,state,claimed_at) VALUES(?,'run.confirm',?,?,'claimed',?)`, op.OperationID, b.PreparationID, op.ParamsSHA256, formatTime(now))
	return err
}
func finishPreparationOperation(ctx context.Context, tx *sql.Tx, op PreparationOperation, b ab.BindingV1, state PreparationState, now time.Time) error {
	res, err := tx.ExecContext(ctx, `UPDATE operations SET state='done',revision=?,result=?,finished_at=? WHERE operation_id=? AND state='claimed'`, state.Revision, string(preparationAcknowledgement(op, b, state)), formatTime(now), op.OperationID)
	if err != nil {
		return err
	}
	return requireOneRow(res, "preparation operation", op.OperationID)
}
func preparationReplay(ctx context.Context, tx *sql.Tx, op PreparationOperation, b ab.BindingV1, l PreparationLookup) (PreparationState, bool, error) {
	var method, object, digest, state string
	var raw sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT method,object,digest,state,result FROM operations WHERE operation_id=?`, op.OperationID).Scan(&method, &object, &digest, &state, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return PreparationState{}, false, nil
	}
	if err != nil {
		return PreparationState{}, false, err
	}
	if method != "run.confirm" || object != b.PreparationID || digest != op.ParamsSHA256 || state != "done" || !raw.Valid {
		return PreparationState{}, false, admissionStorageError(v2contract.CodeRevisionConflict)
	}
	var ack preparationResultAck
	if !decodeStorageJSON([]byte(raw.String), &ack) {
		return PreparationState{}, false, admissionStorageError(v2contract.CodeInvalidContract)
	}
	var body preparationBodyAck
	if !decodeStorageJSON(ack.Body, &body) || body.PreparationID != b.PreparationID || body.BindingSHA256 != l.BindingSHA256 || body.Generation != l.Generation || ack.OperationID != op.OperationID || ack.Revision <= 0 {
		return PreparationState{}, false, admissionStorageError(v2contract.CodeInvalidContract)
	}
	result := PreparationState{PreparationID: body.PreparationID, BindingSHA256: body.BindingSHA256, Revision: uint64(ack.Revision), Ready: body.Ready}
	if !bytes.Equal([]byte(raw.String), preparationAcknowledgement(op, b, result)) {
		return PreparationState{}, false, admissionStorageError(v2contract.CodeInvalidContract)
	}
	return result, true, nil
}

// storageAdmissionRecord mirrors the existing admission.decided payload.
// This fixed mapping preserves observations (including unknown billing) rather
// than interpreting the stored record as a fresh admission decision.
type storageAdmissionRecord struct {
	Adapter              adapter.Descriptor       `json:"adapter"`
	Native               adapter.Probe            `json:"native"`
	NativeAuth           *storageAuth             `json:"native_auth,omitempty"`
	Snapshot             storageSnapshot          `json:"snapshot"`
	Task                 storageTask              `json:"task"`
	DataDestinations     []string                 `json:"data_destinations"`
	ExecutionProfile     storageProfile           `json:"execution_profile"`
	Billing              adapter.BillingPosture   `json:"billing"`
	RequiredCapabilities map[string]adapter.Tri   `json:"required_capabilities"`
	Capabilities         adapter.CapabilityRecord `json:"capabilities"`
	Overrides            []adapter.ConfigDelta    `json:"overrides"`
	ConfigManifest       adapter.ConfigManifest   `json:"config_manifest"`
	Host                 string                   `json:"host"`
	Scenario             string                   `json:"scenario,omitempty"`
	GitIdentity          storageGitIdentity       `json:"git_identity"`
	ProjectConfigDigest  string                   `json:"project_config_digest,omitempty"`
	NoChecks             bool                     `json:"no_checks"`
	KeepGoing            bool                     `json:"keep_going"`
	AllowedTools         []string                 `json:"allowed_tools,omitempty"`
	NativeConfigDigest   string                   `json:"native_config_digest,omitempty"`
	NativeHooks          int64                    `json:"native_hooks,omitempty"`
	NativeTrustGrant     string                   `json:"native_trust_grant,omitempty"`
}
type storageAuth struct {
	LoggedIn         bool   `json:"logged_in"`
	AuthMethod       string `json:"auth_method"`
	APIProvider      string `json:"api_provider"`
	SubscriptionType string `json:"subscription_type"`
	IdentityRef      string `json:"identity_ref"`
}
type storageSnapshot struct {
	SourceRepo string `json:"source_repo"`
	Branch     string `json:"branch"`
	BaseRev    string `json:"base_rev"`
	Dirty      bool   `json:"dirty_at_admission"`
	Source     string `json:"selected_by"`
}
type storageTask struct {
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}
type storageProfile struct {
	Name       string            `json:"name"`
	Contained  bool              `json:"contained"`
	Consent    string            `json:"consent"`
	Disclosure string            `json:"disclosure"`
	Boundary   *contain.Evidence `json:"boundary,omitempty"`
}
type storageGitIdentity struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}
type storageAdmissionPayload struct {
	storageAdmissionRecord
	AcceptancePin acceptance.Pin `json:"acceptance_pin"`
}

func bindingAdmissionRecord(b ab.BindingV1) storageAdmissionRecord {
	d := b.Decision
	destinations := []string{"local state directory"}
	switch d.Adapter.ID {
	case "builtin/claudecode", "builtin/codex":
		destinations = append(destinations, "first-party native API over the network")
	case "builtin/fake":
		destinations = append(destinations, "no network (scripted fake agent)")
	}
	rec := storageAdmissionRecord{Adapter: adapter.Descriptor(d.Adapter), Native: adapter.Probe(d.Probe), Snapshot: storageSnapshot(d.Snapshot), Task: storageTask{d.Task.SHA256, d.Task.Bytes}, DataDestinations: destinations, ExecutionProfile: storageProfile{Name: d.Profile.Name, Contained: d.Profile.Contained, Consent: d.Profile.Consent, Disclosure: d.Profile.Disclosure}, Billing: adapter.BillingPosture(d.Proposal.Billing), RequiredCapabilities: map[string]adapter.Tri{"structured_events": adapter.Tri(d.Proposal.Capabilities.Capabilities.StructuredEvents)}, Capabilities: bindingCapabilities(d.Proposal.Capabilities), Overrides: []adapter.ConfigDelta{}, ConfigManifest: adapter.ConfigManifest{Digests: map[string]string{}}, Host: d.Host, Scenario: d.Scenario, GitIdentity: storageGitIdentity{d.GitName, d.GitEmail}, ProjectConfigDigest: d.ConfigDigest, NoChecks: d.NoChecks, KeepGoing: d.KeepGoing, AllowedTools: d.ProjectConfig.AllowedTools, NativeConfigDigest: d.NativeConfigDigest, NativeTrustGrant: d.NativeTrustGrant}
	if d.NativeHooks != nil {
		rec.NativeHooks = *d.NativeHooks
	}
	if d.NativeAuth != nil {
		a := d.NativeAuth
		rec.NativeAuth = &storageAuth{a.LoggedIn, a.AuthMethod, a.APIProvider, a.SubscriptionType, a.IdentityRef}
	}
	if d.Profile.Boundary != nil {
		x := d.Profile.Boundary
		rec.ExecutionProfile.Boundary = &contain.Evidence{Profile: x.Profile, OS: x.OS, Route: x.Route, Boundary: x.Boundary, Version: x.Version, Owner: x.Owner, Coverage: contain.Coverage{Filesystem: contain.Claim(x.Coverage.Filesystem), Process: contain.Claim(x.Coverage.Process), Network: contain.Claim(x.Coverage.Network), Credential: contain.Claim(x.Coverage.Credential)}}
	}
	for _, v := range d.Proposal.Overrides {
		rec.Overrides = append(rec.Overrides, adapter.ConfigDelta(v))
	}
	for _, v := range d.Proposal.Manifest {
		rec.ConfigManifest.Digests[v.Source] = v.Digest
	}
	return rec
}
func bindingCapabilities(c ab.CapabilityV1) adapter.CapabilityRecord {
	o := c.Capabilities
	result := adapter.CapabilityRecord{SchemaVersion: int(c.SchemaVersion), AdapterID: c.AdapterID, AdapterVersion: c.AdapterVersion, RuntimeVersion: c.RuntimeVersion, Mode: c.Mode, ExecutionSurface: c.ExecutionSurface, HarnessID: c.HarnessID, FidelityQualification: c.FidelityQualification, Billing: adapter.BillingCapabilities{Entitlement: c.Billing.Entitlement, IncludedOnlySupported: adapter.Tri(c.Billing.IncludedOnlySupported), PaidOveragePrevention: adapter.Tri(c.Billing.PaidOveragePrevention), EvidenceID: c.Billing.EvidenceID}, HostIntegration: adapter.HostIntegration{HerdrEmbedded: adapter.Tri(c.HostIntegration.HerdrEmbedded), HerdrStateBridge: adapter.Tri(c.HostIntegration.HerdrStateBridge), NativeInteractiveAttach: adapter.Tri(c.HostIntegration.NativeInteractiveAttach)}, Capabilities: adapter.Capabilities{StructuredEvents: adapter.Tri(o.StructuredEvents), Resume: adapter.Tri(o.Resume), LiveSteer: adapter.Tri(o.LiveSteer), ApprovalBridge: adapter.Tri(o.ApprovalBridge), UsageTokens: adapter.Tri(o.UsageTokens), QuotaRemaining: adapter.Tri(o.QuotaRemaining), HardMonetaryLimit: adapter.Tri(o.HardMonetaryLimit), NativeSubagents: adapter.Tri(o.NativeSubagents)}, Sandbox: adapter.Sandbox{Status: adapter.Tri(c.Sandbox.Status), Scope: c.Sandbox.Scope}, Platform: adapter.Platform{OS: c.Platform.OS, WorkerDetachment: adapter.Tri(c.Platform.WorkerDetachment), ProcessTreeOwnership: adapter.Tri(c.Platform.ProcessTreeOwnership)}, Qualification: c.Qualification}
	if o.Reconnect != nil {
		result.Capabilities.Reconnect = adapter.Tri(*o.Reconnect)
	}
	if o.ModelMetadata != nil {
		result.Capabilities.ModelMetadata = adapter.Tri(*o.ModelMetadata)
	}
	return result
}
func bindingQuotaBucket(b ab.BindingV1) string {
	d := b.Decision
	identity := ""
	if d.NativeAuth != nil {
		identity = d.NativeAuth.IdentityRef
	}
	parts := []string{d.Adapter.Harness, d.Adapter.Surface, d.Proposal.Billing.EntitlementClass, identity}
	for i, v := range parts {
		if v == "" {
			parts[i] = "unknown"
		}
	}
	return string(mustJSON(parts))
}

// appendAdmissionEvent is a fixed internal writer, not an injected projection.
// appendTx owns canonical sequence and migration checks already shipped on main.
func appendAdmissionEvent(ctx context.Context, tx *sql.Tx, b ab.BindingV1, typ, attemptID string, payload any, now time.Time) error {
	producer := "service-admission/" + b.RunID
	var seq int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT last_sequence FROM producers WHERE producer_id=?),0)+1`, producer).Scan(&seq); err != nil {
		return err
	}
	ev := Event{SchemaVersion: 2, EventID: ids.New("evt"), RunID: b.RunID, TaskID: b.TaskID, AttemptID: attemptID, ProducerID: producer, ProducerSequence: seq, Generation: b.Generation, ObservedAt: now, Type: typ, Payload: mustJSON(payload)}
	return appendTx(ctx, tx, ev, nil)
}

type storageStatePayload struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}
type storageAssignmentPayload struct {
	SchemaVersion int    `json:"schema_version"`
	RunID         string `json:"run_id"`
	OperationID   string `json:"operation_id"`
	AttemptID     string `json:"attempt_id"`
	Generation    int64  `json:"generation"`
	Owner         string `json:"owner"`
}
type storageAcceptedBody struct {
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	RunID         string `json:"run_id"`
	AttemptID     string `json:"attempt_id"`
	Generation    int64  `json:"generation"`
	Revision      uint64 `json:"revision"`
}

func admissionMetadata(in AdmissionCommitInput, b ab.BindingV1, revision uint64) AdmissionMetadata {
	return AdmissionMetadata{OperationID: in.OperationID, PreparationID: b.PreparationID, RootID: b.RootID, PeerID: b.PeerID, ContextEpoch: b.ContextEpoch, BindingSHA256: in.Lookup.BindingSHA256, RunID: b.RunID, TaskID: b.TaskID, AttemptID: b.AttemptID, Generation: b.Generation, Revision: revision}
}
func admissionAcknowledgement(m AdmissionMetadata) []byte {
	return mustJSON(preparationResultAck{m.OperationID, int64(m.Revision), mustJSON(storageAcceptedBody{2, m.OperationID, m.RunID, m.AttemptID, m.Generation, m.Revision})}) // #nosec G115 -- metadata is privately minted from the final durable signed SQLite RunRevision.
}
func startOperationDigest(in AdmissionCommitInput, b ab.BindingV1) string {
	params := mustJSON(struct {
		SchemaVersion int    `json:"schema_version"`
		PreparationID string `json:"preparation_id"`
		BindingSHA256 string `json:"binding_sha256"`
	}{2, b.PreparationID, in.Lookup.BindingSHA256})
	h := sha256.New()
	for _, part := range []string{"run.start", b.PreparationID, string(params), strconv.FormatUint(in.Lookup.ExpectedRevision, 10), strconv.FormatInt(b.Generation, 10), storageDigest(nil)} {
		_, _ = fmt.Fprintf(h, "%d:%s;", len(part), part)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func admissionReplay(ctx context.Context, tx *sql.Tx, in AdmissionCommitInput, b ab.BindingV1) (CommittedAdmission, bool, error) {
	var prep, run, attempt, binding string
	var generation int64
	err := tx.QueryRowContext(ctx, `SELECT preparation_id,run_id,attempt_id,consumed_generation,binding_sha256 FROM service_admission_consumptions WHERE operation_id=?`, in.OperationID).Scan(&prep, &run, &attempt, &generation, &binding)
	if errors.Is(err, sql.ErrNoRows) {
		return CommittedAdmission{}, false, nil
	}
	if err != nil {
		return CommittedAdmission{}, false, err
	}
	if prep != b.PreparationID || run != b.RunID || attempt != b.AttemptID || generation != b.Generation || binding != in.Lookup.BindingSHA256 {
		return CommittedAdmission{}, false, admissionStorageError(v2contract.CodeRevisionConflict)
	}
	// Recheck immutable scope from actual storage before releasing an old result.
	var canonical, root, peer, epoch string
	if err := tx.QueryRowContext(ctx, `SELECT canonical_binding,root_id,peer_id,context_epoch FROM service_admission_bindings WHERE preparation_id=?`, prep).Scan(&canonical, &root, &peer, &epoch); err != nil {
		return CommittedAdmission{}, false, err
	}
	if root != in.Lookup.RootID || peer != in.Lookup.PeerID {
		return CommittedAdmission{}, false, admissionStorageError(v2contract.CodePermissionDenied)
	}
	if epoch != in.Lookup.ContextEpoch || canonical != string(in.CanonicalBinding) {
		return CommittedAdmission{}, false, admissionStorageError(v2contract.CodeDependencyStale)
	}
	var method, object, digest, state, raw string
	var revision uint64
	if err := tx.QueryRowContext(ctx, `SELECT method,object,digest,state,result,revision FROM operations WHERE operation_id=?`, in.OperationID).Scan(&method, &object, &digest, &state, &raw, &revision); err != nil {
		return CommittedAdmission{}, false, err
	}
	m := admissionMetadata(in, b, revision)
	if method != "run.start" || object != b.RunID || digest != startOperationDigest(in, b) || state != "done" || revision == 0 || raw != string(admissionAcknowledgement(m)) {
		return CommittedAdmission{}, false, admissionStorageError(v2contract.CodeRevisionConflict)
	}
	return CommittedAdmission{&committedAdmissionState{metadata: m, binding: bytes.Clone(in.CanonicalBinding)}}, true, nil
}

type storageLaunchIntent struct {
	LaunchTokenSHA256 string `json:"launch_token_sha256"`
}
type storageLaunchProvenance struct {
	SchemaVersion     int       `json:"schema_version"`
	RunID             string    `json:"run_id"`
	AttemptID         string    `json:"attempt_id"`
	LaunchTokenSHA256 string    `json:"launch_token_sha256"`
	NonceSHA256       string    `json:"nonce_sha256"`
	LadderVersion     string    `json:"ladder_version"`
	Generation        *int64    `json:"generation"`
	ExecutionDeadline time.Time `json:"execution_deadline"`
}
type storageAdmissionPin struct {
	LadderVersion    string                  `json:"ladder_version"`
	NonceSHA256      string                  `json:"nonce_sha256"`
	LaunchProvenance storageLaunchProvenance `json:"launch_provenance"`
}
type storageSnapshotPayload struct {
	BaseRev                   string  `json:"base_rev"`
	Branch                    *string `json:"branch"`
	ClonePath                 string  `json:"clone_path"`
	ProjectConfigTrustGranted bool    `json:"project_config_trust_granted"`
}

func checkSnapshotForArm(ctx context.Context, tx *sql.Tx, b ab.BindingV1, in DispatchArmInput) error {
	var run, task, typ, raw, observed string
	var generation, seq, admissionSeq int64
	err := tx.QueryRowContext(ctx, `SELECT run_id,COALESCE(task_id,''),type,payload,generation,seq,observed_at FROM journal WHERE event_id=?`, in.SnapshotEventID).Scan(&run, &task, &typ, &raw, &generation, &seq, &observed)
	if errors.Is(err, sql.ErrNoRows) {
		return admissionStorageError(v2contract.CodeDependencyStale)
	}
	if err != nil {
		return err
	}
	snapshotAt, parseErr := time.Parse(time.RFC3339Nano, observed)
	if parseErr != nil {
		return admissionStorageError(v2contract.CodeInvalidContract)
	}
	if in.ObservedAt.Before(snapshotAt) {
		return admissionStorageError(v2contract.CodeDependencyStale)
	}
	if run != b.RunID || task != b.TaskID || typ != "workspace.snapshot_created" || generation != b.Generation {
		return admissionStorageError(v2contract.CodeDependencyStale)
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0) FROM journal WHERE run_id=? AND type='admission.decided'`, b.RunID).Scan(&admissionSeq); err != nil {
		return err
	}
	if admissionSeq == 0 || seq <= admissionSeq {
		return admissionStorageError(v2contract.CodeDependencyStale)
	}
	var payload storageSnapshotPayload
	if !decodeStorageJSON([]byte(raw), &payload) {
		return admissionStorageError(v2contract.CodeInvalidContract)
	}
	branch := ""
	if payload.Branch != nil {
		branch = *payload.Branch
	}
	d := b.Decision
	if payload.BaseRev != d.Snapshot.BaseRev || payload.ClonePath != d.Workdir || branch != d.Snapshot.Branch || payload.ProjectConfigTrustGranted != d.RecordTrust {
		return admissionStorageError(v2contract.CodeDependencyStale)
	}
	if d.RecordTrust {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM trust_grants WHERE kind='project_config' AND repo_identity=? AND digest=?`, d.RepoIdentity, d.ConfigDigest).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return admissionStorageError(v2contract.CodeDependencyStale)
		}
	}
	return nil
}

// decodeStorageJSON rejects duplicate/unknown keys and bounds depth/bytes.
// Records are fixed DTOs; no extension object can become execution authority.
func decodeStorageJSON(raw []byte, out any) bool {
	if len(raw) == 0 || len(raw) > 1048576 {
		return false
	}
	scan := json.NewDecoder(bytes.NewReader(raw))
	var value func(int) bool
	value = func(depth int) bool {
		if depth > 64 {
			return false
		}
		token, err := scan.Token()
		if err != nil {
			return false
		}
		delimiter, compound := token.(json.Delim)
		if !compound {
			return true
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for scan.More() {
				key, err := scan.Token()
				if err != nil {
					return false
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return false
				}
				seen[name] = true
				if !value(depth + 1) {
					return false
				}
			}
			end, err := scan.Token()
			return err == nil && end == json.Delim('}')
		case '[':
			for scan.More() {
				if !value(depth + 1) {
					return false
				}
			}
			end, err := scan.Token()
			return err == nil && end == json.Delim(']')
		}
		return false
	}
	if !value(0) {
		return false
	}
	if _, err := scan.Token(); !errors.Is(err, io.EOF) {
		return false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(out) == nil
}

// A historical acknowledgement is immutable evidence, never a fresh ready or
// consent assertion. Authenticate scope before replay, then apply freshness and
// consumed/revision checks only for a new operation.
func preparationHistoricalReplay(ctx context.Context, tx *sql.Tx, op PreparationOperation, l PreparationLookup) (PreparationState, bool, error) {
	var raw string
	err := tx.QueryRowContext(ctx, `SELECT canonical_binding FROM service_admission_bindings WHERE preparation_id=?`, l.PreparationID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return PreparationState{}, false, nil
	}
	if err != nil {
		return PreparationState{}, false, err
	}
	b, err := ab.DecodeV1([]byte(raw))
	if err != nil {
		return PreparationState{}, false, admissionLeafError(err)
	}
	if err := checkLookupScope(l, b); err != nil {
		return PreparationState{}, false, err
	}
	return preparationReplay(ctx, tx, op, b, l)
}

// retentionApprovalRecordDigest is the version-1 approval-record digest.
// Policy/full-binding integrity columns use plain SHA256 of encoded bytes;
// this approval column uses its specified domain. Neither form proves consent.
func retentionApprovalRecordDigest(raw []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte("MYTHHELM:retention-approval:v1\n"))
	_, _ = h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}
