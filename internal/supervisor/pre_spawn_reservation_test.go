package supervisor_test

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workers"
)

func TestPreSpawnTerminationReleasesOnlyItsQuotaHold(t *testing.T) {
	// The instance-lock helper sets XDG_RUNTIME_DIR; these cases are serial.
	for _, tc := range []struct {
		name, stage, fault, reason                string
		state                                     supervisor.RunState
		intent, rollback, keepHeld, identityDrift bool
	}{
		{name: "cancel before intent", stage: "executing", fault: "interrupt", state: supervisor.RunCancelled},
		{name: "cancel after intent", stage: "intent", fault: "interrupt", state: supervisor.RunCancelled, intent: true},
		{name: "snapshot failure", stage: "admission", fault: "snapshot", state: supervisor.RunFailed, reason: "preflight_snapshot_failed"},
		{name: "launch gate refusal", stage: "executing", fault: "gate", state: supervisor.RunBlocked, reason: "envelope_deadline_exceeded"},
		{name: "config binding failure", stage: "intent", fault: "config", state: supervisor.RunFailed, reason: "native_config_unbound", intent: true},
		{name: "admission pin failure", stage: "admission", fault: "pin", state: supervisor.RunFailed, reason: "admission_pin_failed", intent: true},
		{name: "excluded spawn failure", stage: "intent", fault: "spawn", state: supervisor.RunFailed, reason: "worker_spawn_failed", intent: true, keepHeld: true},
		{name: "quota host changed before stop", stage: "intent", fault: "interrupt", state: supervisor.RunStopping, intent: true, keepHeld: true, identityDrift: true},
		{name: "cancel release rollback", stage: "intent", fault: "interrupt", state: supervisor.RunStopping, intent: true, rollback: true},
		{name: "snapshot release rollback", stage: "admission", fault: "snapshot", state: supervisor.RunAdmission, reason: "", rollback: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			holdInstanceLock(t, f.state)
			d, err := admission.Decide(t.Context(), admission.Request{StateDir: f.state, Repo: f.repo, TaskFile: f.task, Adapter: admission.AdapterFake, Billing: admission.BillingLocalScripted, ExecutionProfile: admission.ProfileTrustedHost, Env: os.Environ(), NoChecks: true})
			if err != nil {
				t.Fatal(err)
			}
			if tc.fault == "config" {
				d.Proposal.Manifest.Digests = map[string]string{"unknown_source": strings.Repeat("a", 64)}
			}
			j := f.journal(t)
			interrupts := make(chan os.Signal, 1)
			var quota journal.ReservationRow
			var controls []journal.ReservationRow
			seenAdmission, triggered := false, false
			out, runErr := supervisor.Run(t.Context(), d, supervisor.Hooks{Interrupt: interrupts, Event: func(ev journal.Event) {
				if ev.Type == "admission.decided" {
					seenAdmission = true
					db := applyDB(t, f.state)
					var id string
					if err := db.QueryRowContext(t.Context(), `SELECT reservation_id FROM reservations WHERE run_id=?`, d.RunID).Scan(&id); err != nil {
						t.Fatal(err)
					}
					quota, err = j.Reservation(t.Context(), id)
					if err != nil {
						t.Fatal(err)
					}
					if quota.Status != "held" {
						t.Fatalf("admission quota status=%s", quota.Status)
					}
					// These deliberately overlapping rows are not the quota claim returned
					// by this admission. Releasing all claims of the run would corrupt them.
					for _, kind := range []string{"service", "other owner", "other bucket", "other quota ID", "orphaned"} {
						row := quota
						row.ReservationID = ids.New("rsv")
						switch kind {
						case "service": // The host belongs to the service extension, below.
						case "other owner":
							row.Owner = "other-owner"
						case "other bucket":
							row.Bucket = "other-bucket"
							row.Scope = row.Bucket
						case "orphaned":
							row.Status = "orphaned"
						}
						if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
							if err := journal.InsertReservation(t.Context(), tx, row); err != nil {
								return err
							}
							if kind == "service" {
								_, err := tx.ExecContext(t.Context(), `UPDATE reservations SET execution_host='test-host' WHERE reservation_id=?`, row.ReservationID)
								return err
							}
							return nil
						}); err != nil {
							t.Fatal(err)
						}
						controls = append(controls, row)
					}
					if tc.identityDrift {
						if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
							_, err := tx.ExecContext(t.Context(), `UPDATE reservations SET execution_host='reassigned-host' WHERE reservation_id=?`, quota.ReservationID)
							return err
						}); err != nil {
							t.Fatal(err)
						}
					}
					if tc.rollback {
						if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
							_, err := tx.ExecContext(t.Context(), `CREATE TRIGGER fail_quota_release BEFORE UPDATE OF status ON reservations WHEN NEW.status='released' BEGIN SELECT RAISE(ABORT, 'synthetic release failure'); END`)
							return err
						}); err != nil {
							t.Fatal(err)
						}
					}
				}
				stage := ""
				switch {
				case ev.Type == "admission.decided":
					stage = "admission"
				case ev.Type == "attempt.launch_intent_recorded":
					stage = "intent"
				case ev.Type == "run.state_changed" && payloadOf(t, ev)["state"] == "executing":
					stage = "executing"
				}
				if stage != tc.stage || triggered {
					return
				}
				triggered = true
				switch tc.fault {
				case "interrupt":
					interrupts <- os.Interrupt
				case "snapshot":
					if err := os.MkdirAll(filepath.Dir(d.Workdir), 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(d.Workdir, []byte("synthetic snapshot obstruction"), 0600); err != nil {
						t.Fatal(err)
					}
				case "gate":
					if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
						_, err := tx.ExecContext(t.Context(), `UPDATE run_envelopes SET first_start_at=? WHERE run_id=?`, time.Now().Add(-24*time.Hour).UTC().Format(time.RFC3339Nano), d.RunID)
						return err
					}); err != nil {
						t.Fatal(err)
					}
				case "spawn":
					path := workers.AttemptDir(f.state, d.RunID, d.AttemptID)
					if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte("synthetic worker directory obstruction"), 0600); err != nil {
						t.Fatal(err)
					}
				case "pin":
					if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
						_, err := tx.ExecContext(t.Context(), `CREATE TRIGGER fail_admission_pin BEFORE INSERT ON journal WHEN NEW.type='attempt.admission_pinned' BEGIN SELECT RAISE(ABORT, 'synthetic pin failure'); END`)
						return err
					}); err != nil {
						t.Fatal(err)
					}
				}
			}})
			if !seenAdmission || !triggered {
				t.Fatalf("fault not reached: admission=%t triggered=%t", seenAdmission, triggered)
			}
			switch {
			case tc.identityDrift:
				if !errors.Is(runErr, journal.ErrNotFound) {
					t.Fatalf("changed quota identity error=%v, want ErrNotFound", runErr)
				}
			case tc.rollback:
				if runErr == nil || !strings.Contains(runErr.Error(), "synthetic release failure") {
					t.Fatalf("release rollback error=%v", runErr)
				}
			case tc.fault == "interrupt":
				if runErr != nil {
					t.Fatal(runErr)
				}
			case runErr == nil:
				t.Fatal("early fault was hidden")
			}
			row, err := j.Run(t.Context(), d.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if row.State != string(tc.state) || row.Reason != tc.reason || out.State != tc.state {
				t.Fatalf("run=%s/%s outcome=%s, want %s/%s", row.State, row.Reason, out.State, tc.state, tc.reason)
			}
			released, err := j.Reservation(t.Context(), quota.ReservationID)
			if err != nil {
				t.Fatal(err)
			}
			if tc.rollback || tc.keepHeld {
				if released.Status != "held" || released.ReleaseEvidence != "" {
					t.Fatalf("release escaped rollback: %+v", released)
				}
			} else {
				why := tc.reason
				if why == "" {
					why = "cancelled_before_spawn"
				}
				// I09/I24 (v2 §2): release has a witnessed no-spawn basis, not a
				// fabricated stopped-attempt claim, and never touches another owner.
				want := "worker not spawned: " + why
				if released.Status != "released" || released.ReleaseEvidence != want {
					t.Fatalf("reservation=%s evidence=%q, want released/%q", released.Status, released.ReleaseEvidence, want)
				}
			}
			for _, before := range controls {
				after, err := j.Reservation(t.Context(), before.ReservationID)
				if err != nil {
					t.Fatal(err)
				}
				if after.Status != before.Status || after.ReleaseEvidence != "" {
					t.Fatalf("unrelated hold changed: before=%+v after=%+v", before, after)
				}
			}
			events := f.events(t, d.RunID)
			if (typeIndex(events, "attempt.launch_intent_recorded") >= 0) != tc.intent {
				t.Fatalf("intent presence differs from %t", tc.intent)
			}
			for _, typ := range []string{"attempt.launched", "attempt.stopped"} {
				if typeIndex(events, typ) >= 0 {
					t.Fatalf("no-spawn exit invented %s", typ)
				}
			}
			if tc.fault == "spawn" {
				info, err := os.Stat(workers.AttemptDir(f.state, d.RunID, d.AttemptID))
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("spawn obstruction not retained: %v", err)
				}
			} else if _, err := os.Stat(filepath.Join(workers.AttemptDir(f.state, d.RunID, d.AttemptID), "worker.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("worker exists: %v", err)
			}
			if tc.rollback || tc.identityDrift {
				for _, ev := range events {
					if ev.Type == "run.state_changed" {
						state := payloadOf(t, ev)["state"]
						if state == "failed" || state == "blocked" || state == "cancelled" {
							t.Fatalf("terminal event escaped release rollback: %v", state)
						}
					}
				}
			}
		})
	}
}
