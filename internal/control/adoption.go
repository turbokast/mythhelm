package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// ServiceAdoption is checked reconciliation evidence, never an owner capability.
type ServiceAdoption struct {
	SchemaVersion         int       `json:"schema_version"`
	RunID                 string    `json:"run_id"`
	AttemptID             string    `json:"attempt_id"`
	LaunchSHA256          string    `json:"launch_sha256"`
	OperationID           string    `json:"operation_id"`
	WorkerPID             int       `json:"worker_pid"`
	WorkerStartedAt       time.Time `json:"worker_started_at"`
	LaunchGeneration      int64     `json:"launch_generation"`
	ControlGeneration     int64     `json:"control_generation"`
	CommittedSpoolOffset  int64     `json:"committed_spool_offset"`
	ReconciliationEventID string    `json:"reconciliation_event_id"`
}

func adoptionError(code v2contract.Code) error {
	return &v2contract.ControlError{Code: code, Owner: "control", OperationID: "service.adopt", Disposition: code.DefaultDisposition(), NextAction: "reconcile the retained launch"}
}

func validateAdoption(a ServiceAdoption) error {
	if a.SchemaVersion > 2 {
		return adoptionError(v2contract.CodeCapabilityUnsupported)
	}
	sum, err := hex.DecodeString(a.LaunchSHA256)
	if a.SchemaVersion != 2 || a.RunID == "" || a.AttemptID == "" || a.OperationID == "" || a.ReconciliationEventID == "" || a.OperationID == a.ReconciliationEventID || a.WorkerPID <= 0 || a.WorkerStartedAt.IsZero() || a.LaunchGeneration < 0 || a.ControlGeneration <= 0 || a.CommittedSpoolOffset < 0 || err != nil || len(sum) != 32 || hex.EncodeToString(sum) != a.LaunchSHA256 {
		return adoptionError(v2contract.CodeInvalidContract)
	}
	return nil
}

// DecodeServiceAdoption strictly decodes schema2 evidence without interpreting
// historical schema1 events as an adoption or accepting future critical data.
func DecodeServiceAdoption(event journal.Event) (ServiceAdoption, error) {
	var a ServiceAdoption
	if event.SchemaVersion > 2 {
		return a, adoptionError(v2contract.CodeCapabilityUnsupported)
	}
	if event.SchemaVersion != 2 || event.Type != "service.adopted" {
		return a, adoptionError(v2contract.CodeInvalidContract)
	}
	if err := v2contract.Envelope(event).Validate(); err != nil {
		return a, adoptionError(v2contract.CodeInvalidContract)
	}
	fields, ok := uniqueObject(event.Payload)
	if !ok || len(fields) != 11 {
		return a, adoptionError(v2contract.CodeInvalidContract)
	}
	for _, name := range []string{"schema_version", "run_id", "attempt_id", "launch_sha256", "operation_id", "worker_pid", "worker_started_at", "launch_generation", "control_generation", "committed_spool_offset", "reconciliation_event_id"} {
		if v, ok := fields[name]; !ok || isNull(v) {
			return a, adoptionError(v2contract.CodeInvalidContract)
		}
	}
	// All fields above are mandatory; unknown fields fail independently of count.
	dec := json.NewDecoder(bytes.NewReader(event.Payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return a, adoptionError(v2contract.CodeInvalidContract)
	}
	if err := validateAdoption(a); err != nil {
		return a, err
	}
	if event.RunID != a.RunID || event.AttemptID != a.AttemptID || event.Generation != a.ControlGeneration || event.EventID != a.ReconciliationEventID {
		return a, adoptionError(v2contract.CodeInvalidContract)
	}
	return a, nil
}

// RecordServiceAdoption records a trusted held-owner reconciliation atomically.
// Callers verify OS/launch/spool evidence before entering this transaction.
func RecordServiceAdoption(ctx context.Context, tx *sql.Tx, a ServiceAdoption) (err error) {
	defer func() {
		if err == nil {
			return
		}
		var ce *v2contract.ControlError
		if errors.As(err, &ce) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		err = fmt.Errorf("%w: %w", adoptionError(v2contract.CodePersistenceUnavailable), err)
	}()
	if tx == nil {
		return adoptionError(v2contract.CodeInvalidContract)
	}
	if err := validateAdoption(a); err != nil {
		return err
	}
	raw, err := json.Marshal(a)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	var method, object, storedDigest, state string
	var result sql.NullString
	var revision int64
	err = tx.QueryRowContext(ctx, `SELECT method,object,digest,state,result,revision FROM operations WHERE operation_id=?`, a.OperationID).Scan(&method, &object, &storedDigest, &state, &result, &revision)
	switch {
	case err == nil:
		if method != "service.adopt" || object != a.RunID || storedDigest != digest {
			return adoptionError(v2contract.CodeRevisionConflict)
		}
		if state != "done" || !result.Valid {
			return adoptionError(v2contract.CodeOwnershipUnresolved)
		}
		var r Result
		if json.Unmarshal([]byte(result.String), &r) != nil || r.OperationID != a.OperationID || r.Revision != revision || revision <= 0 || r.Error != nil || !bytes.Equal(r.Body, raw) {
			return adoptionError(v2contract.CodeInvalidContract)
		}
		var linked int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM service_assignment_epochs WHERE run_id=? AND attempt_id=? AND operation_id=? AND adoption_event_id=? AND control_generation=?`, a.RunID, a.AttemptID, a.OperationID, a.ReconciliationEventID, a.ControlGeneration).Scan(&linked); err != nil {
			return err
		}
		if linked != 1 {
			return adoptionError(v2contract.CodeInvalidContract)
		}
		return nil
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("adoption operation: %w", err)
	}
	var collision int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM journal WHERE event_id=?`, a.ReconciliationEventID).Scan(&collision); err != nil {
		return err
	}
	if collision != 0 {
		return adoptionError(v2contract.CodeRevisionConflict)
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO operations(operation_id,method,object,digest,state,claimed_at) VALUES (?,'service.adopt',?,?,'claimed',?)`, a.OperationID, a.RunID, digest, now.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	producer := "service.adopt." + strconv.FormatInt(a.ControlGeneration, 10)
	var sequence int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT last_sequence FROM producers WHERE producer_id=?),0)+1`, producer).Scan(&sequence); err != nil {
		return err
	}
	event := v2contract.Envelope{SchemaVersion: 2, EventID: a.ReconciliationEventID, RunID: a.RunID, AttemptID: a.AttemptID, ProducerID: producer, ProducerSequence: sequence, Generation: a.ControlGeneration, ObservedAt: now, Type: "service.adopted", Payload: raw}
	if err := journal.AppendCanonicalTx(ctx, tx, event, func(tx *sql.Tx) error {
		return journal.RecordAssignmentEpoch(ctx, tx, journal.AssignmentEpoch{RunID: a.RunID, AttemptID: a.AttemptID, AdoptionEventID: a.ReconciliationEventID, OperationID: a.OperationID, ControlGeneration: a.ControlGeneration})
	}); err != nil {
		return err
	}
	current, err := journal.RunRevision(ctx, tx, a.RunID)
	if err != nil {
		return err
	}
	if current > uint64(^uint64(0)>>1) {
		return adoptionError(v2contract.CodeInvalidContract)
	}
	completed, err := json.Marshal(Result{OperationID: a.OperationID, Revision: int64(current), Body: raw})
	if err != nil {
		return err
	}
	resultWrite, err := tx.ExecContext(ctx, `UPDATE operations SET state='done',revision=?,result=?,finished_at=? WHERE operation_id=? AND state='claimed'`, current, string(completed), now.Format(time.RFC3339Nano), a.OperationID)
	if err != nil {
		return err
	}
	affected, err := resultWrite.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return adoptionError(v2contract.CodeOwnershipUnresolved)
	}
	return nil
}
