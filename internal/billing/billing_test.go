package billing

import (
	"errors"
	"testing"
)

// I09 (v2 §2): the exhaustion codes are exact strings MH-21 adopts.
func TestCodesPinV245Strings(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"allowance code", string(CodeAllowanceExhausted), "allowance_exhausted"},
		{"budget code", string(CodeBudgetExhausted), "budget_exhausted"},
		{"allowance sentinel", ErrAllowanceExhausted.Error(), "allowance_exhausted"},
		{"budget sentinel", ErrBudgetExhausted.Error(), "budget_exhausted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
	if errors.Is(ErrAllowanceExhausted, ErrBudgetExhausted) {
		t.Fatal("the two sentinels must stay distinct")
	}
}
