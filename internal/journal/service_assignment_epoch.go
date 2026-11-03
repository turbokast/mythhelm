package journal

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

// AssignmentEpoch links a run's adoption event and internal service.adopt
// operation to a control generation (design section 3.2). It is storage
// evidence only: it grants no held-owner authority and proves no OS owner.
type AssignmentEpoch struct {
	RunID, AttemptID, AdoptionEventID, OperationID string
	ControlGeneration                              int64
}

const (
	epochRecordOp = "assignment-epoch.record"
	epochReadOp   = "assignment-epoch.read"
	epochReason   = "mythhelm.assignment-epoch/reason"
)

// epochError builds a typed error whose public detail is one fixed reason;
// cause, if any, stays in the wrapped chain and never enters Detail.
func epochError(code v2contract.Code, op, runID, reason, next string, cause error) error {
	ce := &v2contract.ControlError{
		Code:        code,
		Owner:       "journal",
		OperationID: op,
		Object:      runID,
		Disposition: code.DefaultDisposition(),
		NextAction:  next,
		Detail:      map[string]string{epochReason: reason},
	}
	if cause == nil {
		return ce
	}
	return fmt.Errorf("%w: %w", ce, cause)
}

func epochStorage(op, runID string, err error) error {
	return epochError(v2contract.CodePersistenceUnavailable, op, runID, "storage_unavailable",
		"retry once the state database answers", err)
}

func runExists(ctx context.Context, tx *sql.Tx, runID string) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM runs WHERE run_id = ?`, runID).Scan(&n)
	return n == 1, err
}

func epochByIdentity(ctx context.Context, tx *sql.Tx, e AssignmentEpoch) ([]AssignmentEpoch, error) {
	rows, err := tx.QueryContext(ctx, `SELECT run_id, attempt_id, adoption_event_id, operation_id, control_generation
		FROM service_assignment_epochs
		WHERE (run_id = ? AND control_generation = ?) OR adoption_event_id = ? OR operation_id = ?`,
		e.RunID, e.ControlGeneration, e.AdoptionEventID, e.OperationID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []AssignmentEpoch
	for rows.Next() {
		var got AssignmentEpoch
		if err := rows.Scan(&got.RunID, &got.AttemptID, &got.AdoptionEventID, &got.OperationID, &got.ControlGeneration); err != nil {
			return nil, err
		}
		out = append(out, got)
	}
	return out, rows.Err()
}

// RecordAssignmentEpoch appends e inside the caller's transaction. The
// claimed service.adopt operation may be completed later in the same
// transaction, after reading the actual RunRevision. An exact replay of a
// committed epoch writes nothing; any other collision or a non-increasing
// generation refuses. The epoch trigger projects the current assignment.
func RecordAssignmentEpoch(ctx context.Context, tx *sql.Tx, e AssignmentEpoch) error {
	if e.RunID == "" || e.AttemptID == "" || e.AdoptionEventID == "" || e.OperationID == "" ||
		e.ControlGeneration <= 0 || e.OperationID == e.AdoptionEventID {
		return epochError(v2contract.CodeInvalidContract, epochRecordOp, e.RunID, "identity_mismatch",
			"supply a complete adoption identity", nil)
	}
	ok, err := runExists(ctx, tx, e.RunID)
	if err != nil {
		return epochStorage(epochRecordOp, e.RunID, err)
	}
	if !ok {
		return epochError(v2contract.CodeInvalidContract, epochRecordOp, e.RunID, "missing",
			"name an existing run", nil)
	}
	existing, err := epochByIdentity(ctx, tx, e)
	if err != nil {
		return epochStorage(epochRecordOp, e.RunID, err)
	}
	if len(existing) == 1 && existing[0] == e {
		return nil
	}
	if len(existing) > 0 {
		return epochError(v2contract.CodeRevisionConflict, epochRecordOp, e.RunID, "conflicting",
			"reconcile the recorded adoption before retrying", nil)
	}
	var latest sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(control_generation) FROM service_assignment_epochs WHERE run_id = ?`, e.RunID).Scan(&latest); err != nil {
		return epochStorage(epochRecordOp, e.RunID, err)
	}
	if latest.Valid && latest.Int64 >= e.ControlGeneration {
		return epochError(v2contract.CodeRevisionConflict, epochRecordOp, e.RunID, "stale",
			"adopt with a generation newer than the recorded epoch", nil)
	}
	var linked int
	err = tx.QueryRowContext(ctx, `SELECT
		EXISTS (SELECT 1 FROM attempts WHERE attempt_id = ? AND run_id = ?)
		AND EXISTS (SELECT 1 FROM operations WHERE operation_id = ? AND method = 'service.adopt' AND object = ? AND state = 'claimed')
		AND EXISTS (SELECT 1 FROM journal WHERE event_id = ? AND schema_version = 2 AND type = 'service.adopted'
			AND run_id = ? AND attempt_id = ? AND generation = ?
			AND json_type(payload, '$.control_generation') = 'integer' AND json_extract(payload, '$.control_generation') = ?
			AND json_type(payload, '$.operation_id') = 'text' AND json_extract(payload, '$.operation_id') = ?)`,
		e.AttemptID, e.RunID, e.OperationID, e.RunID, e.AdoptionEventID, e.RunID, e.AttemptID, e.ControlGeneration,
		e.ControlGeneration, e.OperationID).Scan(&linked)
	if err != nil {
		return epochStorage(epochRecordOp, e.RunID, err)
	}
	if linked != 1 {
		return epochError(v2contract.CodeInvalidContract, epochRecordOp, e.RunID, "identity_mismatch",
			"link the epoch to its run, attempt, claimed operation and adoption event", nil)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO service_assignment_epochs
		(run_id, control_generation, attempt_id, adoption_event_id, operation_id) VALUES (?, ?, ?, ?, ?)`,
		e.RunID, e.ControlGeneration, e.AttemptID, e.AdoptionEventID, e.OperationID); err != nil {
		return epochStorage(epochRecordOp, e.RunID, err)
	}
	return nil
}

// ReadAssignmentEpoch returns the greatest committed epoch for runID. false
// means no epoch evidence exists for a valid run: unknown, never generation 0
// or 1 and never ownership. A latest epoch whose operation is not completed
// is unavailable (ownership_unresolved); one that disagrees with the current
// assignment, its adoption event (including the envelope generation) or a
// completed result whose revision lies outside 1..current RunRevision is
// invalid_contract.
func ReadAssignmentEpoch(ctx context.Context, tx *sql.Tx, runID string) (AssignmentEpoch, bool, error) {
	ok, err := runExists(ctx, tx, runID)
	if err != nil {
		return AssignmentEpoch{}, false, epochStorage(epochReadOp, runID, err)
	}
	if !ok {
		return AssignmentEpoch{}, false, epochError(v2contract.CodeInvalidContract, epochReadOp, runID, "missing",
			"name an existing run", nil)
	}
	e := AssignmentEpoch{RunID: runID}
	var state string
	var coherent, resultBound int
	var opRevision int64
	var result sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT e.attempt_id, e.adoption_event_id, e.operation_id, e.control_generation, o.state,
		o.revision, o.result,
		EXISTS (SELECT 1 FROM attempts t WHERE t.attempt_id = e.attempt_id AND t.run_id = e.run_id)
		AND EXISTS (SELECT 1 FROM run_assignments a JOIN journal j ON j.event_id = e.adoption_event_id
			WHERE a.run_id = e.run_id AND a.owner = 'supervisor' AND a.operation_id = e.operation_id AND a.assigned_at = j.observed_at
			AND j.schema_version = 2 AND j.type = 'service.adopted' AND j.run_id = e.run_id AND j.attempt_id = e.attempt_id
			AND j.generation = e.control_generation
			AND json_extract(j.payload, '$.control_generation') = e.control_generation
			AND json_extract(j.payload, '$.operation_id') = e.operation_id)
		AND o.method = 'service.adopt' AND o.object = e.run_id,
		o.result IS NOT NULL AND o.finished_at IS NOT NULL AND o.revision >= 1
			AND o.revision <= (SELECT revision FROM run_control_revisions WHERE run_id = e.run_id)
		FROM service_assignment_epochs e JOIN operations o ON o.operation_id = e.operation_id
		WHERE e.run_id = ? ORDER BY e.control_generation DESC LIMIT 1`, runID).
		Scan(&e.AttemptID, &e.AdoptionEventID, &e.OperationID, &e.ControlGeneration, &state, &opRevision, &result, &coherent, &resultBound)
	if errors.Is(err, sql.ErrNoRows) {
		return AssignmentEpoch{}, false, nil
	}
	if err != nil {
		return AssignmentEpoch{}, false, epochStorage(epochReadOp, runID, err)
	}
	if coherent != 1 {
		return AssignmentEpoch{}, false, epochError(v2contract.CodeInvalidContract, epochReadOp, runID, "identity_mismatch",
			"reconcile the assignment with its latest epoch", nil)
	}
	if state != "done" {
		return AssignmentEpoch{}, false, epochError(v2contract.CodeOwnershipUnresolved, epochReadOp, runID, "missing",
			"complete or reconcile the adoption operation", nil)
	}
	if resultBound != 1 || !resultBindsOperation(result.String, e.OperationID, opRevision) {
		return AssignmentEpoch{}, false, epochError(v2contract.CodeInvalidContract, epochReadOp, runID, "identity_mismatch",
			"reconcile the adoption operation result with the run revision", nil)
	}
	return e, true, nil
}

// resultBindsOperation decodes a private projection of the existing
// control.Result wrapper (journal must not import control): a JSON object
// whose operation_id and revision match the stored operation and which
// carries no refusal error. Body stays opaque and optional.
func resultBindsOperation(raw, operationID string, revision int64) bool {
	var r struct {
		OperationID *string         `json:"operation_id"`
		Revision    *int64          `json:"revision"`
		Error       json.RawMessage `json:"error"`
	}
	if !strings.HasPrefix(strings.TrimSpace(raw), "{") || json.Unmarshal([]byte(raw), &r) != nil {
		return false
	}
	noError := len(r.Error) == 0 || string(r.Error) == "null"
	return r.OperationID != nil && *r.OperationID == operationID && r.Revision != nil && *r.Revision == revision && noError
}
