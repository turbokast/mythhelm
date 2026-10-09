package billing

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestEstimateQuantifiesVerifyPass(t *testing.T) {
	t.Parallel()
	checks := []CheckBound{{"vet", 30 * time.Second}, {"test", 5 * time.Minute}, {"lint", 90 * time.Second}}
	got := EstimateReserve(checks, Ceilings{Execution: time.Hour, Repairs: 3, Replans: 2, TransportRetries: 5})
	want := ReserveEstimate{VerifyPassChecks: 3, VerifyTimeoutSum: 7 * time.Minute, RepairCeiling: 3,
		Note: "estimate, not a reserve of provider quota"}
	if got != want {
		t.Errorf("EstimateReserve = %+v, want %+v", got, want)
	}
	empty := EstimateReserve(nil, Ceilings{Repairs: 0})
	if empty.VerifyPassChecks != 0 || empty.VerifyTimeoutSum != 0 || empty.RepairCeiling != 0 || empty.Note != want.Note {
		t.Errorf("EstimateReserve(no checks, 0 repairs) = %+v, want zero quantities with the note", empty)
	}
}

func TestRemainderCoversReserveBoundary(t *testing.T) {
	t.Parallel()
	est := ReserveEstimate{VerifyPassChecks: 2, VerifyTimeoutSum: 90 * time.Second, RepairCeiling: 3}
	tests := []struct {
		name string
		est  ReserveEstimate
		left time.Duration
		want bool
	}{
		{"exactly at the boundary covers", est, 90 * time.Second, true},
		{"one nanosecond short does not", est, 90*time.Second - time.Nanosecond, false},
		{"plenty covers", est, time.Hour, true},
		{"repair ceiling plays no part", ReserveEstimate{VerifyTimeoutSum: 90 * time.Second, RepairCeiling: 1000}, 90 * time.Second, true},
		{"check count plays no part", ReserveEstimate{VerifyPassChecks: 1000, VerifyTimeoutSum: 90 * time.Second}, 90 * time.Second, true},
		{"empty pass is covered by nothing left", ReserveEstimate{}, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := RemainderCoversReserve(ExecutionRemainder{TimeLeft: tc.left}, tc.est); got != tc.want {
				t.Errorf("RemainderCoversReserve(left %v, %+v) = %t, want %t", tc.left, tc.est, got, tc.want)
			}
		})
	}
}

// I10 (v2 §2): the reserve is an estimate, never a hard token claim.
func TestReserveNeverHardTokenClaim(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeFor[ReserveEstimate]()
	for f := range typ.Fields() {
		if strings.Contains(strings.ToLower(f.Name), "token") || strings.Contains(strings.ToLower(f.Name), "quota") {
			t.Errorf("ReserveEstimate field %s names a quantity of provider quota", f.Name)
		}
	}
	est := EstimateReserve([]CheckBound{{"a", time.Second}}, BuiltInCeilings())
	if !strings.Contains(est.Note, "estimate") || !strings.Contains(est.Note, "not a reserve of provider quota") {
		t.Errorf("Note = %q, want it to pin estimate-only", est.Note)
	}
	if est.Note != ReserveNote {
		t.Errorf("Note %q differs from ReserveNote %q", est.Note, ReserveNote)
	}
}
