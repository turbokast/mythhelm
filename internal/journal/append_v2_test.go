// Append's v2 acceptance (supervisor-migration Task 2, design §7): v2
// envelopes append once the durable migration phase allows them —
// migration-owned envelopes at drained, every v2 envelope at imported or
// later — through the stream-1 validators in check order, while v1 decodes
// byte-identically forever (AC-7.5).
//
// External test package: the golden fixture names admission.Decision and the
// supervisor state words, and both packages import journal.
package journal_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/v2contract"
	_ "modernc.org/sqlite" // registers the "sqlite" driver for phase moves
)

func openV2Temp(t *testing.T) (*journal.Journal, string) {
	t.Helper()
	dir := t.TempDir()
	j, err := journal.Open(t.Context(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := j.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return j, dir
}

// openWritable opens dbPath for the test's own writes (phase moves). It
// mirrors journal's DSN construction without reaching into unexported
// helpers, following openStored's read-only pattern.
func openWritable(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	p := filepath.ToSlash(dbPath)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	uri := (&url.URL{Scheme: "file", Path: p, RawQuery: "_pragma=busy_timeout(5000)"}).String()
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// setPhase moves the database's durable migration phase, as the migrate
// Apply step would between Drain, Import and Adopt.
func setPhase(t *testing.T, dir, phase string) {
	t.Helper()
	db := openWritable(t, filepath.Join(dir, journal.DBName))
	if _, err := db.ExecContext(t.Context(), `UPDATE migration_state SET phase = ? WHERE id = 1`, phase); err != nil {
		t.Fatalf("setting phase %s: %v", phase, err)
	}
}

var v2FixedTime = time.Date(2026, time.October, 8, 12, 0, 0, 0, time.UTC)

// validV2 returns a valid stream-1 envelope. Every accepted input in these
// tests starts life as one, so Validate passes before Append runs and the
// tests exercise the phase rule and the ledger checks, not the vocabulary.
func validV2(eventID, runID, producer string, seq, gen int64, typ string) v2contract.Envelope {
	return v2contract.Envelope{
		SchemaVersion:    v2contract.SchemaVersion,
		EventID:          eventID,
		RunID:            runID,
		ProducerID:       producer,
		ProducerSequence: seq,
		Generation:       gen,
		ObservedAt:       v2FixedTime,
		Type:             typ,
		Payload:          json.RawMessage(`{"state":"candidate"}`),
	}
}

// asEvent converts a stream-1 envelope to the journal envelope, which
// mirrors it field-for-field.
func asEvent(e v2contract.Envelope) journal.Event {
	return journal.Event{
		SchemaVersion:    e.SchemaVersion,
		EventID:          e.EventID,
		RunID:            e.RunID,
		TaskID:           e.TaskID,
		AttemptID:        e.AttemptID,
		ProducerID:       e.ProducerID,
		ProducerSequence: e.ProducerSequence,
		RunSequence:      e.RunSequence,
		Generation:       e.Generation,
		CausedBy:         e.CausedBy,
		ObservedAt:       e.ObservedAt,
		Type:             e.Type,
		Payload:          e.Payload,
	}
}

// controlCode extracts the catalogue code from an Append-stage v2 failure,
// which must be a valid ControlError.
func controlCode(t *testing.T, err error) v2contract.Code {
	t.Helper()
	var ce *v2contract.ControlError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want a *v2contract.ControlError in the chain", err)
	}
	if err := ce.Validate(); err != nil {
		t.Fatalf("ControlError %+v invalid: %v", ce, err)
	}
	return ce.Code
}

func mustV2Append(t *testing.T, j *journal.Journal, env v2contract.Envelope) {
	t.Helper()
	if err := env.Validate(); err != nil {
		t.Fatalf("fixture envelope invalid: %v", err)
	}
	if err := j.Append(t.Context(), asEvent(env), nil); err != nil {
		t.Fatalf("Append v2 %s: %v", env.EventID, err)
	}
}

func TestAppendAcceptsV2PostMigration(t *testing.T) {
	for _, phase := range []string{"imported", "adopted"} {
		t.Run("phase_"+phase, func(t *testing.T) {
			ctx := t.Context()
			j, dir := openV2Temp(t)
			setPhase(t, dir, phase)

			mustV2Append(t, j, validV2("evt_v2_1", "run_v2", "sup_v2", 1, 1, "task.transitioned"))
			mustV2Append(t, j, validV2("evt_v2_2", "run_v2", "sup_v2", 2, 1, "task.transitioned"))
			evs, err := j.Events(ctx, "run_v2", 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(evs) != 2 || evs[0].RunSequence != 1 || evs[1].RunSequence != 2 {
				t.Fatalf("run_sequence = %v, want supervisor-allocated 1, 2", evs)
			}

			// Duplicates ack with nil error by event_id, appending nothing.
			if err := j.Append(ctx, asEvent(validV2("evt_v2_1", "run_v2", "sup_v2", 1, 1, "task.transitioned")), nil); err != nil {
				t.Fatalf("duplicate append: %v, want nil", err)
			}
			if evs, err := j.Events(ctx, "run_v2", 0); err != nil || len(evs) != 2 {
				t.Fatalf("events after duplicate = %d, %v; want 2, nil", len(evs), err)
			}
		})
	}

	t.Run("gap_and_stale_and_invalid", func(t *testing.T) {
		ctx := t.Context()
		j, dir := openV2Temp(t)
		setPhase(t, dir, "imported")
		mustV2Append(t, j, validV2("evt_v2_base", "run_v2_checks", "sup_v2_checks", 1, 1, "task.transitioned"))

		if err := j.Append(ctx, asEvent(validV2("evt_v2_gap", "run_v2_checks", "sup_v2_checks", 3, 1, "task.transitioned")), nil); !errors.Is(err, v2contract.ErrSequenceGap) {
			t.Errorf("gap err = %v, want v2contract.ErrSequenceGap", err)
		} else if code := controlCode(t, err); code != v2contract.CodeInvalidContract {
			t.Errorf("gap code = %s, want invalid_contract", code)
		}

		if err := j.Append(ctx, asEvent(validV2("evt_v2_stale", "run_v2_checks", "sup_v2_checks", 2, 0, "task.transitioned")), nil); !errors.Is(err, v2contract.ErrStaleGeneration) {
			t.Errorf("stale err = %v, want v2contract.ErrStaleGeneration", err)
		} else if code := controlCode(t, err); code != v2contract.CodeOwnershipUnresolved {
			t.Errorf("stale code = %s, want ownership_unresolved", code)
		}

		bad := validV2("evt_v2_bad", "run_v2_checks", "sup_v2_checks", 2, 1, "task.transitioned")
		bad.Payload = nil
		if err := j.Append(ctx, asEvent(bad), nil); err == nil {
			t.Error("invalid envelope appended, want rejection")
		} else if code := controlCode(t, err); code != v2contract.CodeInvalidContract {
			t.Errorf("invalid code = %s, want invalid_contract", code)
		}

		// An empty event_id still rejects with a valid ControlError: the
		// operation ID falls back to "unknown" instead of failing Validate.
		noID := validV2("", "run_v2_checks", "sup_v2_checks", 2, 1, "task.transitioned")
		if err := j.Append(ctx, asEvent(noID), nil); err == nil {
			t.Error("empty event_id appended, want rejection")
		} else if code := controlCode(t, err); code != v2contract.CodeInvalidContract {
			t.Errorf("empty event_id code = %s, want invalid_contract", code)
		}

		if evs, err := j.Events(ctx, "run_v2_checks", 0); err != nil || len(evs) != 1 {
			t.Fatalf("events after rejects = %d, %v; want 1, nil", len(evs), err)
		}
	})
}

// TestAppendV2TxBoundaryErrors covers Append's transaction boundaries for
// v2: a BeginTx failure (closed database) and a Commit failure (the
// projection rolls the transaction back) both map to
// persistence_unavailable, while v1 keeps its raw journal errors.
func TestAppendV2TxBoundaryErrors(t *testing.T) {
	ctx := t.Context()

	beginFails := func(t *testing.T, ev journal.Event) error {
		t.Helper()
		dir := t.TempDir()
		j, err := journal.Open(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := j.Close(); err != nil {
			t.Fatal(err)
		}
		return j.Append(ctx, ev, nil)
	}

	err := beginFails(t, asEvent(validV2("evt_v2_nobegin", "run_v2_tx", "sup_v2_tx", 1, 1, "task.transitioned")))
	if err == nil {
		t.Fatal("Append on a closed database returned nil, want an error")
	}
	if code := controlCode(t, err); code != v2contract.CodePersistenceUnavailable {
		t.Errorf("v2 BeginTx code = %s, want persistence_unavailable", code)
	}

	err = beginFails(t, journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: "evt_v1_nobegin", RunID: "run_v1_tx",
		ProducerID: "sup_v1_tx", ProducerSequence: 1, Generation: 1,
		ObservedAt: v2FixedTime, Type: "run.state_changed",
		Payload: json.RawMessage(`{"state":"created","reason":null}`),
	})
	if err == nil {
		t.Fatal("v1 Append on a closed database returned nil, want an error")
	}
	var ce *v2contract.ControlError
	if errors.As(err, &ce) {
		t.Errorf("v1 BeginTx err = %v, want the raw journal error, not a ControlError", err)
	}
	if !strings.Contains(err.Error(), "journal: starting append") {
		t.Errorf("v1 BeginTx err = %v, want the unchanged message", err)
	}

	commitFails := func(t *testing.T, j *journal.Journal, ev journal.Event) error {
		t.Helper()
		return j.Append(ctx, ev, func(tx *sql.Tx) error { return tx.Rollback() })
	}

	j, dir := openV2Temp(t)
	setPhase(t, dir, "imported")
	err = commitFails(t, j, asEvent(validV2("evt_v2_nocommit", "run_v2_tx", "sup_v2_commit", 1, 1, "task.transitioned")))
	if !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("v2 commit err = %v, want sql.ErrTxDone in the chain", err)
	}
	if code := controlCode(t, err); code != v2contract.CodePersistenceUnavailable {
		t.Errorf("v2 commit code = %s, want persistence_unavailable", code)
	}

	j1, _ := openV2Temp(t)
	err = commitFails(t, j1, journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: "evt_v1_nocommit", RunID: "run_v1_tx",
		ProducerID: "sup_v1_commit", ProducerSequence: 1, Generation: 1,
		ObservedAt: v2FixedTime, Type: "run.state_changed",
		Payload: json.RawMessage(`{"state":"created","reason":null}`),
	})
	if !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("v1 commit err = %v, want sql.ErrTxDone in the chain", err)
	}
	if errors.As(err, &ce) {
		t.Errorf("v1 commit err = %v, want the raw journal error, not a ControlError", err)
	}
}

func TestAppendAcceptsMigrationOwnedAtDrained(t *testing.T) {
	ctx := t.Context()
	j, dir := openV2Temp(t)
	setPhase(t, dir, "drained")
	for i, typ := range []string{"migration.quarantined", "migration.imported"} {
		env := validV2(fmt.Sprintf("evt_mig_%d", i), "run_mig", "sup_mig", int64(i+1), 1, typ)
		mustV2Append(t, j, env)
	}
	if evs, err := j.Events(ctx, "run_mig", 0); err != nil || len(evs) != 2 {
		t.Fatalf("events = %d, %v; want 2, nil", len(evs), err)
	}
}

func TestAppendRejectsOrdinaryV2AtDrained(t *testing.T) {
	ctx := t.Context()
	j, dir := openV2Temp(t)
	setPhase(t, dir, "drained")
	err := j.Append(ctx, asEvent(validV2("evt_v2_ord", "run_ord", "sup_ord", 1, 1, "task.transitioned")), nil)
	if err == nil {
		t.Fatal("ordinary v2 envelope appended at drained, want rejection")
	}
	if code := controlCode(t, err); code != v2contract.CodeInvalidContract {
		t.Errorf("code = %s, want invalid_contract", code)
	}
	if !errors.Is(err, journal.ErrInvalidEvent) {
		t.Errorf("err = %v, want journal.ErrInvalidEvent in the chain", err)
	}
	if evs, err := j.Events(ctx, "run_ord", 0); err != nil || len(evs) != 0 {
		t.Fatalf("events = %d, %v; want 0, nil", len(evs), err)
	}
}

func TestAppendRejectsV2PreMigration(t *testing.T) {
	for _, phase := range []string{"not_started", "previewed"} {
		for _, typ := range []string{"task.transitioned", "migration.imported"} {
			t.Run(phase+"/"+typ, func(t *testing.T) {
				ctx := t.Context()
				j, dir := openV2Temp(t)
				setPhase(t, dir, phase)
				err := j.Append(ctx, asEvent(validV2("evt_v2_pre", "run_pre", "sup_pre", 1, 1, typ)), nil)
				if err == nil {
					t.Fatalf("%s appended pre-drained (%s), want rejection", typ, phase)
				}
				if code := controlCode(t, err); code != v2contract.CodeInvalidContract {
					t.Errorf("code = %s, want invalid_contract", code)
				}
				if !errors.Is(err, journal.ErrInvalidEvent) {
					t.Errorf("err = %v, want journal.ErrInvalidEvent in the chain", err)
				}
				if evs, err := j.Events(ctx, "run_pre", 0); err != nil || len(evs) != 0 {
					t.Fatalf("events = %d, %v; want 0, nil", len(evs), err)
				}
			})
		}
	}
}

// Every v1 run status word: the design §4 happy path, then the stop,
// loss, recovery and terminal exits. The count guard in the test fails if
// the machine gains a word without updating this fixture and its golden.
var goldenRunStates = []supervisor.RunState{
	supervisor.RunCreated,
	supervisor.RunAdmission,
	supervisor.RunExecuting,
	supervisor.RunVerifying,
	supervisor.RunReadyForReview,
	supervisor.RunApplying,
	supervisor.RunCompleted,
	supervisor.RunStopping,
	supervisor.RunCancelled,
	supervisor.RunInterrupted,
	supervisor.RunRecovering,
	supervisor.RunBlocked,
	supervisor.RunFailed,
}

// Every v1 attempt status word along the launch chain, then the stop and
// loss exits. Count-guarded like the run states.
var goldenAttemptStates = []supervisor.AttemptState{
	supervisor.AttemptLaunchIntentRecorded,
	supervisor.AttemptLaunching,
	supervisor.AttemptRunning,
	supervisor.AttemptSucceededNative,
	supervisor.AttemptFailedNative,
	supervisor.AttemptStopRequested,
	supervisor.AttemptStopped,
	supervisor.AttemptInterrupted,
	supervisor.AttemptQuarantined,
}

// goldenReason keeps the fixture's reasons legal under checkReason: a state
// recording failure or an unresolved condition says why, and
// ready_for_review is either verified or unverified.
func goldenReason(state string) string {
	switch state {
	case string(supervisor.RunReadyForReview):
		return "unverified"
	case string(supervisor.RunBlocked), string(supervisor.RunFailed),
		string(supervisor.RunInterrupted), string(supervisor.AttemptFailedNative):
		return "golden"
	default:
		return ""
	}
}

func TestV1DecodeByteIdentical(t *testing.T) {
	if n := len(goldenRunStates); n != 13 {
		t.Fatalf("golden run states = %d, want 13: update the fixture and golden", n)
	}
	if n := len(goldenAttemptStates); n != 9 {
		t.Fatalf("golden attempt states = %d, want 9: update the fixture and golden", n)
	}
	ctx := t.Context()
	j, _ := openV2Temp(t)
	base := time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC)
	const runID, attemptID = "run_golden", "att_golden"
	var seq int64
	v1 := func(eventID, typ, payload string) journal.Event {
		seq++
		return journal.Event{
			SchemaVersion:    journal.EnvelopeVersion,
			EventID:          eventID,
			RunID:            runID,
			ProducerID:       "sup_golden",
			ProducerSequence: seq,
			Generation:       1,
			ObservedAt:       base.Add(time.Duration(seq) * time.Second),
			Type:             typ,
			Payload:          json.RawMessage(payload),
		}
	}
	mustAppend := func(ev journal.Event, project func(*sql.Tx) error) {
		t.Helper()
		if err := j.Append(ctx, ev, project); err != nil {
			t.Fatalf("Append %s: %v", ev.EventID, err)
		}
	}

	created := base
	mustAppend(v1("evt_golden_created", "run.created", `{"state":"created","reason":null}`),
		func(tx *sql.Tx) error {
			return journal.InsertRun(ctx, tx, journal.RunRow{
				RunID: runID, State: string(supervisor.RunCreated),
				AdapterID: "builtin/fake", SourceRepo: "example.com/golden",
				TaskSHA256: "sha256:golden", BillingPosture: "subscription-declared",
				ExecutionProfile: "lab", CreatedAt: created, UpdatedAt: created,
			})
		})

	decision := admission.Decision{
		StateDir: "testdata/golden-state", RunID: runID, TaskID: "task_golden",
		AttemptID: attemptID, RunDir: "testdata/golden-state/runs/" + runID,
		Workdir: "testdata/golden-state/runs/" + runID + "/workspace",
		Host:    "golden-host", Scenario: "golden", GitName: "Golden", GitEmail: "golden@example.com",
		ConfigDigest: "sha256:config", RepoIdentity: "example.com/golden",
		NativeHooks: 2, NativeTrustGrant: "grant_golden",
	}
	decisionJSON, err := json.Marshal(decision)
	if err != nil {
		t.Fatalf("marshalling Decision: %v", err)
	}
	mustAppend(v1("evt_golden_admitted", "run.admitted", string(decisionJSON)), nil)

	for i, state := range goldenRunStates {
		reason := goldenReason(string(state))
		var payload string
		if reason == "" {
			payload = fmt.Sprintf(`{"state":%q,"reason":null}`, string(state))
		} else {
			payload = fmt.Sprintf(`{"state":%q,"reason":%q}`, string(state), reason)
		}
		mustAppend(v1(fmt.Sprintf("evt_golden_run_%02d", i), "run.state_changed", payload),
			func(tx *sql.Tx) error {
				return journal.SetRunState(ctx, tx, runID, string(state), reason, base.Add(time.Duration(seq)*time.Second))
			})
	}

	mustAppend(v1("evt_golden_intent", "attempt.launch_intent_recorded", `{"launch_token_sha256":"sha256:launch"}`),
		func(tx *sql.Tx) error {
			return journal.InsertAttempt(ctx, tx, journal.AttemptRow{
				AttemptID: attemptID, RunID: runID, TaskID: "task_golden",
				AttemptNumber: 1, State: string(supervisor.AttemptLaunchIntentRecorded),
				LaunchTokenSHA256: "sha256:launch", WorkspacePath: "/tmp/golden-workspace",
			})
		})
	for i, state := range goldenAttemptStates[1:] {
		reason := goldenReason(string(state))
		var payload string
		if reason == "" {
			payload = fmt.Sprintf(`{"state":%q,"reason":null}`, string(state))
		} else {
			payload = fmt.Sprintf(`{"state":%q,"reason":%q}`, string(state), reason)
		}
		mustAppend(v1(fmt.Sprintf("evt_golden_attempt_%02d", i), "attempt.state_changed", payload),
			func(tx *sql.Tx) error {
				return journal.SetAttemptState(ctx, tx, attemptID, string(state), reason)
			})
	}

	evs, err := j.Events(ctx, runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	run, err := j.Run(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	att, err := j.Attempt(ctx, attemptID)
	if err != nil {
		t.Fatal(err)
	}
	dump, err := json.MarshalIndent(struct {
		Events  []journal.Event
		Run     journal.RunRow
		Attempt journal.AttemptRow
	}{evs, run, att}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	// GOLDEN_OUT captures the dump for golden authoring (used once, from
	// the pre-change base); normal runs compare against the constant.
	if out := os.Getenv("GOLDEN_OUT"); out != "" {
		//nolint:gosec // G703: the golden-capture path is author tooling, never user input
		if err := os.WriteFile(out, append(dump, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	if string(dump) != goldenV1 {
		t.Fatalf("v1 projections differ from the golden journal (%d events); AC-7.5", len(evs))
	}
}

// goldenV1 is the canonical dump of the fixture above, captured from the
// pre-change base (origin/main 148084b) via GOLDEN_OUT: v1 must decode
// under v1 forever (AC-7.5), so any drift fails the test.
const goldenV1 = `{
  "Events": [
    {
      "schema_version": 1,
      "event_id": "evt_golden_created",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 1,
      "run_sequence": 1,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:01Z",
      "type": "run.created",
      "payload": {
        "state": "created",
        "reason": null
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_admitted",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 2,
      "run_sequence": 2,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:02Z",
      "type": "run.admitted",
      "payload": {
        "StateDir": "testdata/golden-state",
        "RunID": "run_golden",
        "TaskID": "task_golden",
        "AttemptID": "att_golden",
        "RunDir": "testdata/golden-state/runs/run_golden",
        "Workdir": "testdata/golden-state/runs/run_golden/workspace",
        "Host": "golden-host",
        "Scenario": "golden",
        "GitName": "Golden",
        "GitEmail": "golden@example.com",
        "ProjectConfig": {
          "SchemaVersion": 0,
          "Environment": {
            "Passthrough": null
          },
          "Adapters": {
            "ClaudeCode": {
              "AllowedTools": null
            }
          },
          "Checks": null,
          "Envelopes": {
            "Execution": "",
            "Repairs": null,
            "Replans": null,
            "TransportRetries": null
          }
        },
        "ConfigDigest": "sha256:config",
        "RepoIdentity": "example.com/golden",
        "RecordTrust": false,
        "NoChecks": false,
        "KeepGoing": false,
        "EnvelopeFlags": null,
        "Declaration": null,
        "RecordNativeTrust": false,
        "NativeConfigDigest": "",
        "NativeHooks": 2,
        "NativeTrustGrant": "grant_golden",
        "NativeAuth": null,
        "Task": {
          "Content": null,
          "SHA256": "",
          "Title": ""
        },
        "Snapshot": {
          "source_repo": "",
          "branch": "",
          "base_rev": "",
          "dirty_at_admission": false,
          "selected_by": ""
        },
        "Profile": {
          "name": "",
          "contained": false,
          "consent": "",
          "disclosure": ""
        },
        "Adapter": {
          "id": "",
          "version": "",
          "harness": "",
          "surface": ""
        },
        "Probe": {
          "executable": "",
          "version": "",
          "sha256": "",
          "os": "",
          "arch": "",
          "compatibility": ""
        },
        "Proposal": {
          "Spec": {
            "Path": "",
            "Args": null,
            "Dir": "",
            "Env": null,
            "Stdin": null
          },
          "StopLadder": null,
          "Overrides": null,
          "Billing": {
            "mode": "",
            "credential_provenance": "",
            "entitlement_class": "",
            "entitlement_source": "",
            "paid_continuation": "",
            "paid_continuation_user_declaration": "",
            "qualified": false,
            "g05": ""
          },
          "Manifest": {
            "digests": null
          },
          "Capabilities": {
            "schema_version": 0,
            "adapter_id": "",
            "adapter_version": "",
            "runtime_version": "",
            "mode": "",
            "execution_surface": "",
            "harness_id": "",
            "fidelity_qualification": "",
            "billing": {
              "entitlement": "",
              "included_only_supported": "",
              "paid_overage_prevention": "",
              "evidence_id": null
            },
            "host_integration": {
              "herdr_embedded": "",
              "herdr_state_bridge": "",
              "native_interactive_attach": ""
            },
            "capabilities": {
              "structured_events": "",
              "resume": "",
              "live_steer": "",
              "approval_bridge": "",
              "usage_tokens": "",
              "quota_remaining": "",
              "hard_monetary_limit": "",
              "native_subagents": ""
            },
            "sandbox": {
              "status": "",
              "scope": ""
            },
            "platform": {
              "os": "",
              "worker_detachment": "",
              "process_tree_ownership": ""
            },
            "qualification": ""
          }
        }
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_run_00",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 3,
      "run_sequence": 3,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:03Z",
      "type": "run.state_changed",
      "payload": {
        "state": "created",
        "reason": null
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_run_01",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 4,
      "run_sequence": 4,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:04Z",
      "type": "run.state_changed",
      "payload": {
        "state": "admission",
        "reason": null
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_run_02",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 5,
      "run_sequence": 5,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:05Z",
      "type": "run.state_changed",
      "payload": {
        "state": "executing",
        "reason": null
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_run_03",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 6,
      "run_sequence": 6,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:06Z",
      "type": "run.state_changed",
      "payload": {
        "state": "verifying",
        "reason": null
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_run_04",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 7,
      "run_sequence": 7,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:07Z",
      "type": "run.state_changed",
      "payload": {
        "state": "ready_for_review",
        "reason": "unverified"
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_run_05",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 8,
      "run_sequence": 8,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:08Z",
      "type": "run.state_changed",
      "payload": {
        "state": "applying",
        "reason": null
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_run_06",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 9,
      "run_sequence": 9,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:09Z",
      "type": "run.state_changed",
      "payload": {
        "state": "completed",
        "reason": null
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_run_07",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 10,
      "run_sequence": 10,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:10Z",
      "type": "run.state_changed",
      "payload": {
        "state": "stopping",
        "reason": null
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_run_08",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 11,
      "run_sequence": 11,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:11Z",
      "type": "run.state_changed",
      "payload": {
        "state": "cancelled",
        "reason": null
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_run_09",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 12,
      "run_sequence": 12,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:12Z",
      "type": "run.state_changed",
      "payload": {
        "state": "interrupted",
        "reason": "golden"
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_run_10",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 13,
      "run_sequence": 13,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:13Z",
      "type": "run.state_changed",
      "payload": {
        "state": "recovering",
        "reason": null
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_run_11",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 14,
      "run_sequence": 14,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:14Z",
      "type": "run.state_changed",
      "payload": {
        "state": "blocked",
        "reason": "golden"
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_run_12",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 15,
      "run_sequence": 15,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:15Z",
      "type": "run.state_changed",
      "payload": {
        "state": "failed",
        "reason": "golden"
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_intent",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 16,
      "run_sequence": 16,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:16Z",
      "type": "attempt.launch_intent_recorded",
      "payload": {
        "launch_token_sha256": "sha256:launch"
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_attempt_00",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 17,
      "run_sequence": 17,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:17Z",
      "type": "attempt.state_changed",
      "payload": {
        "state": "launching",
        "reason": null
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_attempt_01",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 18,
      "run_sequence": 18,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:18Z",
      "type": "attempt.state_changed",
      "payload": {
        "state": "running",
        "reason": null
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_attempt_02",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 19,
      "run_sequence": 19,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:19Z",
      "type": "attempt.state_changed",
      "payload": {
        "state": "succeeded_native",
        "reason": null
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_attempt_03",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 20,
      "run_sequence": 20,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:20Z",
      "type": "attempt.state_changed",
      "payload": {
        "state": "failed_native",
        "reason": "golden"
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_attempt_04",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 21,
      "run_sequence": 21,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:21Z",
      "type": "attempt.state_changed",
      "payload": {
        "state": "stop_requested",
        "reason": null
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_attempt_05",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 22,
      "run_sequence": 22,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:22Z",
      "type": "attempt.state_changed",
      "payload": {
        "state": "stopped",
        "reason": null
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_attempt_06",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 23,
      "run_sequence": 23,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:23Z",
      "type": "attempt.state_changed",
      "payload": {
        "state": "interrupted",
        "reason": "golden"
      }
    },
    {
      "schema_version": 1,
      "event_id": "evt_golden_attempt_07",
      "run_id": "run_golden",
      "producer_id": "sup_golden",
      "producer_sequence": 24,
      "run_sequence": 24,
      "generation": 1,
      "observed_at": "2026-10-01T12:00:24Z",
      "type": "attempt.state_changed",
      "payload": {
        "state": "quarantined",
        "reason": null
      }
    }
  ],
  "Run": {
    "RunID": "run_golden",
    "State": "failed",
    "Reason": "golden",
    "AdapterID": "builtin/fake",
    "SourceRepo": "example.com/golden",
    "SourceBranch": "",
    "BaseRev": "",
    "TaskSHA256": "sha256:golden",
    "BillingPosture": "subscription-declared",
    "ExecutionProfile": "lab",
    "CreatedAt": "2026-10-01T12:00:00Z",
    "UpdatedAt": "2026-10-01T12:00:15Z"
  },
  "Attempt": {
    "AttemptID": "att_golden",
    "RunID": "run_golden",
    "TaskID": "task_golden",
    "AttemptNumber": 1,
    "State": "quarantined",
    "Reason": "",
    "LaunchTokenSHA256": "sha256:launch",
    "WorkspacePath": "/tmp/golden-workspace",
    "SpoolOffset": 0
  }
}`
