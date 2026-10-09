package workers

import (
	"testing"

	"github.com/turbokast/mythhelm/internal/adapter"
)

func TestExhaustionWinsOverRateLimit(t *testing.T) {
	w, sp := stubWorker(t)
	res := adapter.Result{Subtype: "error_during_execution", IsError: true}
	for _, c := range []struct {
		name    string
		classes []string
	}{
		{"exhaustion then a throttle", []string{"allowance_exhausted", "rate_limit"}},
		{"throttle then exhaustion", []string{"rate_limit", "allowance_exhausted"}},
		{"throttles around exhaustion", []string{"rate_limit", "allowance_exhausted", "rate_limit"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var prog progress
			var out outcome
			for _, class := range c.classes {
				if err := w.observe(sp, adapter.NativeError{Class: class}, &prog, &out); err != nil {
					t.Fatal(err)
				}
			}
			out.result, out.exit, out.report = &res, adapter.NativeExit{Code: 1}, adapter.InterruptReport{Confirmed: true}
			state, reason := classify(out, false)
			if state != "failed_native" || reason != "allowance_exhausted" {
				t.Fatalf("classify = %s/%s, want failed_native/allowance_exhausted (not provider_limit)", state, reason)
			}
		})
	}
	// A lone throttle is still the transient class (design D10).
	var prog progress
	var out outcome
	if err := w.observe(sp, adapter.NativeError{Class: "rate_limit"}, &prog, &out); err != nil {
		t.Fatal(err)
	}
	out.result, out.exit, out.report = &res, adapter.NativeExit{Code: 1}, adapter.InterruptReport{Confirmed: true}
	if _, reason := classify(out, false); reason != "provider_limit" {
		t.Fatalf("lone rate_limit reason = %q, want provider_limit", reason)
	}
}
