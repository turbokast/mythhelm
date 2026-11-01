package journal_test

import (
	"database/sql"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

func quotaJournal(t *testing.T, bucket string, reset *string, used int64, exhausted string) *journal.Journal {
	t.Helper()
	j, err := journal.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
		at := time.Now().UTC()
		if err := journal.InsertRun(t.Context(), tx, journal.RunRow{RunID: "run_quota", State: "admission", AdapterID: "builtin/fake", SourceRepo: "/tmp/repo", TaskSHA256: "t", BillingPosture: "local-scripted", ExecutionProfile: "trusted-host", CreatedAt: at, UpdatedAt: at}); err != nil {
			return err
		}
		if exhausted != "" {
			if err := journal.SetBucketExhausted(t.Context(), tx, bucket, exhausted, reset); err != nil {
				return err
			}
			_, err := tx.ExecContext(t.Context(), `UPDATE bucket_state SET retries_used=? WHERE bucket=?`, used, bucket)
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return j
}

// Actual SQLite rows compare both production paths, including the reset
// override and full transaction rollback (I02/I09/I23, v2 §2).
func TestServiceQuotaMatchesLegacy(t *testing.T) {
	t.Parallel()
	at := time.Now().UTC()
	past, future := at.Add(-time.Hour).Format(time.RFC3339Nano), at.Add(time.Hour).Format(time.RFC3339Nano)
	for _, tc := range []struct {
		name                            string
		exhausted                       string
		reset                           *string
		used                            int64
		duplicate, failInsert, rollback bool
		wantError                       bool
		cleared                         bool
		wantUsed                        int64
	}{
		{name: "held"},
		{name: "duplicate", duplicate: true, wantError: true},
		{name: "due retry", exhausted: past, wantUsed: 1},
		{name: "future reset", exhausted: past, reset: &future, wantError: true},
		{name: "due reset", exhausted: past, reset: &past, cleared: true},
		{name: "due reset spent", exhausted: past, reset: &past, used: 3, cleared: true},
		{name: "spent", exhausted: past, used: 3, wantUsed: 3, wantError: true},
		{name: "failed insert rolls retry back", exhausted: past, failInsert: true, wantError: true},
		{name: "failed insert rolls clear back", exhausted: past, reset: &past, failInsert: true, wantError: true},
		{name: "later failure rolls retry back", exhausted: past, rollback: true, wantError: true},
		{name: "later failure rolls clear back", exhausted: past, reset: &past, rollback: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, fixed := range []bool{false, true} {
				t.Run(map[bool]string{false: "legacy", true: "fixed"}[fixed], func(t *testing.T) {
					j := quotaJournal(t, "bucket", tc.reset, tc.used, tc.exhausted)
					in := journal.QuotaReservationInput{RunID: "run_quota", Bucket: "bucket", ExecutionNanos: int64(10 * time.Minute), ObservedAt: at}
					reserve := func(tx *sql.Tx) (string, error) {
						if fixed {
							return journal.ReserveRunQuota(t.Context(), tx, in)
						}
						return admission.NewJournalReserver(billing.Ceilings{Execution: 10 * time.Minute}, supervisor.EvaluateBucket).Reserve(t.Context(), tx, in.RunID, in.Bucket, in.RunID)
					}
					var firstID string
					if tc.duplicate {
						if err := j.Transact(t.Context(), func(tx *sql.Tx) error { var err error; firstID, err = reserve(tx); return err }); err != nil {
							t.Fatal(err)
						}
					}
					if tc.failInsert {
						if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
							_, err := tx.ExecContext(t.Context(), `CREATE TRIGGER quota_fail BEFORE INSERT ON reservations BEGIN SELECT RAISE(ABORT, 'synthetic storage failure'); END`)
							return err
						}); err != nil {
							t.Fatal(err)
						}
					}
					var id string
					boom := errors.New("synthetic later admission failure")
					err := j.Transact(t.Context(), func(tx *sql.Tx) error {
						var err error
						id, err = reserve(tx)
						if err != nil {
							return err
						}
						if tc.rollback {
							return boom
						}
						return nil
					})
					if (err != nil) != tc.wantError {
						t.Fatalf("reserve = %q %v; want error %t", id, err, tc.wantError)
					}
					if tc.duplicate {
						if fixed && !errors.Is(err, journal.ErrDuplicateQuotaHold) {
							t.Fatalf("duplicate cause lost: %v", err)
						}
						if !fixed && !errors.Is(err, admission.ErrDuplicateHold) {
							t.Fatalf("legacy duplicate cause lost: %v", err)
						}
					}
					if tc.name == "spent" || tc.name == "future reset" {
						if !errors.Is(err, billing.ErrAllowanceExhausted) {
							t.Fatalf("refusal cause lost: %v", err)
						}
					}
					if tc.rollback && !errors.Is(err, boom) {
						t.Fatalf("rollback cause = %v", err)
					}
					var count int
					if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
						return tx.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM reservations`).Scan(&count)
					}); err != nil {
						t.Fatal(err)
					}
					wantCount := 0
					if !tc.wantError || tc.duplicate {
						wantCount = 1
					}
					if count != wantCount {
						t.Fatalf("holds = %d; want %d", count, wantCount)
					}
					if tc.duplicate {
						id = firstID
					}
					if wantCount == 1 {
						row, err := j.Reservation(t.Context(), id)
						if err != nil {
							t.Fatal(err)
						}
						if row.RunID != in.RunID || row.Owner != in.RunID || row.Scope != in.Bucket || row.Bucket != in.Bucket || row.Quantity != "unknown" || row.Status != "held" {
							t.Fatalf("hold = %+v", row)
						}
						created, e1 := time.Parse(time.RFC3339Nano, row.CreatedAt)
						expires, e2 := time.Parse(time.RFC3339Nano, row.ExpiresAt)
						if e1 != nil || e2 != nil || expires.Sub(created) != 70*time.Minute {
							t.Fatalf("hold times = %s %s", row.CreatedAt, row.ExpiresAt)
						}
						if fixed && !created.Equal(at) {
							t.Fatalf("observed time = %v; want %v", created, at)
						}
					}
					if tc.exhausted != "" {
						row, err := j.BucketState(t.Context(), "bucket")
						if tc.cleared {
							if !errors.Is(err, sql.ErrNoRows) {
								t.Fatalf("reset row = %+v %v", row, err)
							}
						} else {
							want := tc.wantUsed
							if tc.failInsert || tc.rollback {
								want = tc.used
							}
							if err != nil || row.RetriesUsed != want || row.ExhaustedAt != tc.exhausted || !reflect.DeepEqual(row.ResetAt, tc.reset) {
								t.Fatalf("bucket = %+v %v; want retries %d reset %v", row, err, want, tc.reset)
							}
						}
					}
				})
			}
		})
	}
}

func TestQuotaStorageIsNotAuthority(t *testing.T) {
	t.Parallel()
	// A closed input cannot carry a caller's evaluator, projection or success bit.
	typ := reflect.TypeFor[journal.QuotaReservationInput]()
	want := []string{"RunID", "Bucket", "ExecutionNanos", "ObservedAt"}
	if typ.NumField() != len(want) {
		t.Fatalf("quota input has %d fields", typ.NumField())
	}
	for i, name := range want {
		if typ.Field(i).Name != name {
			t.Fatalf("field %d = %s", i, typ.Field(i).Name)
		}
	}
	at := time.Now().UTC()
	future := at.Add(time.Hour).Format(time.RFC3339Nano)
	j := quotaJournal(t, "bucket", &future, 0, at.Add(-time.Hour).Format(time.RFC3339Nano))
	in := journal.QuotaReservationInput{RunID: "run_quota", Bucket: "bucket", ObservedAt: at, ExecutionNanos: int64(time.Minute)}
	err := j.Transact(t.Context(), func(tx *sql.Tx) error { _, err := journal.ReserveRunQuota(t.Context(), tx, in); return err })
	var refusal *journal.QuotaRefusal
	if !errors.As(err, &refusal) || !errors.Is(err, billing.ErrAllowanceExhausted) || refusal.Bucket != "bucket" || refusal.Wait != time.Hour || refusal.GiveUp {
		t.Fatalf("fixed refusal = %+v %v", refusal, err)
	}
	// The checked insert is deliberately storage-only for the legacy evaluator;
	// it still cannot accept a caller's claim that a duplicate is safe.
	if err := j.Transact(t.Context(), func(tx *sql.Tx) error { _, err := journal.InsertRunQuotaHold(t.Context(), tx, in); return err }); err != nil {
		t.Fatal(err)
	}
	in.Bucket = "another bucket"
	err = j.Transact(t.Context(), func(tx *sql.Tx) error { _, err := journal.InsertRunQuotaHold(t.Context(), tx, in); return err })
	if !errors.Is(err, journal.ErrDuplicateQuotaHold) {
		t.Fatalf("cross-bucket duplicate accepted: %v", err)
	}
}

func TestQuotaStorageRejectsMalformedInput(t *testing.T) {
	t.Parallel()
	at := time.Now().UTC()
	for _, tc := range []struct {
		name   string
		change func(*journal.QuotaReservationInput)
	}{
		{"missing run", func(in *journal.QuotaReservationInput) { in.RunID = "" }},
		{"missing bucket", func(in *journal.QuotaReservationInput) { in.Bucket = "" }},
		{"missing observed clock", func(in *journal.QuotaReservationInput) { in.ObservedAt = time.Time{} }},
		{"negative execution", func(in *journal.QuotaReservationInput) { in.ExecutionNanos = -1 }},
		{"overflow grace", func(in *journal.QuotaReservationInput) { in.ExecutionNanos = math.MaxInt64 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			j := quotaJournal(t, "bucket", nil, 0, "")
			in := journal.QuotaReservationInput{RunID: "run_quota", Bucket: "bucket", ExecutionNanos: int64(time.Minute), ObservedAt: at}
			tc.change(&in)
			for _, reserve := range []func(*sql.Tx) (string, error){
				func(tx *sql.Tx) (string, error) { return journal.ReserveRunQuota(t.Context(), tx, in) },
				func(tx *sql.Tx) (string, error) { return journal.InsertRunQuotaHold(t.Context(), tx, in) },
			} {
				err := j.Transact(t.Context(), func(tx *sql.Tx) error {
					id, err := reserve(tx)
					if err == nil || id != "" {
						t.Fatalf("malformed input returned %q %v", id, err)
					}
					return err
				})
				if err == nil {
					t.Fatal("malformed hold committed")
				}
			}
			if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
				var n int
				if err := tx.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM reservations`).Scan(&n); err != nil {
					return err
				}
				if n != 0 {
					t.Fatalf("malformed hold rows = %d", n)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
