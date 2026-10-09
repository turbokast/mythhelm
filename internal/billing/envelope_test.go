package billing

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestResolveCeilingsPrecedence(t *testing.T) {
	unset := Ceilings{Execution: -1, Repairs: -1, Replans: -1, TransportRetries: -1}
	built := BuiltInCeilings()
	cases := []struct {
		name        string
		flags, file *Ceilings
		want        Ceilings
	}{
		{"both nil", nil, nil, built},
		{"both fully unset", &unset, &unset, built},
		{"file over built-ins", nil, &Ceilings{Execution: time.Hour, Repairs: 7, Replans: 6, TransportRetries: 9},
			Ceilings{Execution: time.Hour, Repairs: 7, Replans: 6, TransportRetries: 9}},
		{"flags over file", &Ceilings{Execution: time.Minute, Repairs: 1, Replans: 1, TransportRetries: 1},
			&Ceilings{Execution: time.Hour, Repairs: 7, Replans: 6, TransportRetries: 9},
			Ceilings{Execution: time.Minute, Repairs: 1, Replans: 1, TransportRetries: 1}},
		{"per field mix", &Ceilings{Execution: -1, Repairs: 1, Replans: -1, TransportRetries: -1},
			&Ceilings{Execution: time.Hour, Repairs: 7, Replans: -1, TransportRetries: 9},
			Ceilings{Execution: time.Hour, Repairs: 1, Replans: built.Replans, TransportRetries: 9}},
		{"zero is a set value", &Ceilings{Execution: -1, Repairs: 0, Replans: -1, TransportRetries: -1}, nil,
			Ceilings{Execution: built.Execution, Repairs: 0, Replans: built.Replans, TransportRetries: built.TransportRetries}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveCeilings(tc.flags, tc.file); got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestBuiltInCeilingsAreS1Values(t *testing.T) {
	want := Ceilings{Execution: 30 * time.Minute, Repairs: 3, Replans: 2, TransportRetries: 5}
	if got := BuiltInCeilings(); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestDeadlineExcludesOnlyQuiescentPause(t *testing.T) {
	start := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	c := Ceilings{Execution: 30 * time.Minute}
	at := func(m int) time.Time { return start.Add(time.Duration(m) * time.Minute) }
	base := at(30)
	cases := []struct {
		name        string
		pauses      []PauseSpan
		now         time.Time
		wantDead    time.Time
		wantExpired bool
	}{
		{"no pauses, before", nil, at(29), base, false},
		{"no pauses, at deadline", nil, at(30), base, true},
		{"quiesced span extends by its length", []PauseSpan{{RequestedAt: at(5), QuiescedAt: at(6), ResumedAt: at(16)}}, at(35), at(40), false},
		{"still-running turn extends by zero", []PauseSpan{{RequestedAt: at(5), ResumedAt: at(16)}}, at(35), base, true},
		{"requested but never quiesced and unresumed extends by zero", []PauseSpan{{RequestedAt: at(5)}}, at(35), base, true},
		{"open quiesced span extends to now", []PauseSpan{{RequestedAt: at(5), QuiescedAt: at(6)}}, at(50), at(74), false},
		{"two spans add", []PauseSpan{
			{RequestedAt: at(1), QuiescedAt: at(2), ResumedAt: at(4)},
			{RequestedAt: at(10), QuiescedAt: at(11), ResumedAt: at(14)},
		}, at(36), at(35), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dead, expired := Deadline(start, c, tc.pauses, tc.now)
			if !dead.Equal(tc.wantDead) || expired != tc.wantExpired {
				t.Fatalf("got (%v, %v), want (%v, %v)", dead, expired, tc.wantDead, tc.wantExpired)
			}
		})
	}
}

func TestCheckNamesExhaustedKind(t *testing.T) {
	c := Ceilings{Execution: time.Minute, Repairs: 3, Replans: 2, TransportRetries: 5}
	cases := []struct {
		name     string
		counts   AttemptCounts
		wantKind string
	}{
		{"at ceiling passes", AttemptCounts{Repairs: 3, Replans: 2, TransportRetries: 5}, ""},
		{"zero passes", AttemptCounts{}, ""},
		{"repairs over", AttemptCounts{Repairs: 4}, "repairs"},
		{"replans over", AttemptCounts{Replans: 3}, "replans"},
		{"transport retries over", AttemptCounts{TransportRetries: 6}, "transport_retries"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := c.Check(tc.counts)
			if tc.wantKind == "" {
				if err != nil {
					t.Fatalf("unexpected error %v", err)
				}
				return
			}
			if !errors.Is(err, ErrBudgetExhausted) {
				t.Fatalf("got %v, want ErrBudgetExhausted", err)
			}
			if !strings.Contains(err.Error(), tc.wantKind) {
				t.Fatalf("message %q does not name kind %q", err.Error(), tc.wantKind)
			}
		})
	}
}
