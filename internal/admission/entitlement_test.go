package admission_test

import (
	"testing"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/admission"
)

// I15 (v2 §2): a user declaration is never worded as verified entitlement.
func TestEntitlementStatementVerbatim(t *testing.T) {
	t.Parallel()
	declared, err := admission.ResolveBilling(t.Context(), admission.BillingSubscriptionDeclared,
		evidence("org", "dir"), declaration(evidence("org", "dir")), admission.Eligibility{})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		in   adapter.BillingPosture
		want string
	}{
		{
			name: "the declared Claude posture is user-declared and not verified",
			in:   declared,
			want: "entitlement: user-declared, NOT verified by MYTHHELM; paid continuation: unknown (user declares disabled)",
		},
		{
			name: "a strict qualified posture is verified with its paid continuation",
			in: adapter.BillingPosture{Mode: admission.BillingSubscriptionOnly, EntitlementSource: "registry:live-qualified",
				PaidContinuation: "prevented", Qualified: true, G05: "passed"},
			want: "entitlement: verified by MYTHHELM (registry:live-qualified); paid continuation: prevented",
		},
		{
			name: "the local-scripted posture is not applicable",
			in:   adapter.BillingPosture{Mode: admission.BillingLocalScripted, EntitlementSource: "not-applicable", G05: "not-applicable"},
			want: "entitlement: not applicable (local-scripted: no inference, no network)",
		},
		{
			name: "a qualified posture whose G05 has not passed is unknown",
			in: adapter.BillingPosture{Mode: admission.BillingSubscriptionOnly, EntitlementSource: "registry:live-qualified",
				PaidContinuation: "prevented", Qualified: true, G05: "not-passed"},
			want: "entitlement: unknown",
		},
		{
			name: "a declared source never reads as verified even when marked qualified",
			in: adapter.BillingPosture{Mode: admission.BillingSubscriptionDeclared, EntitlementSource: "user_declared+native_status(subscriptionType=max)",
				PaidContinuation: "unknown", Qualified: true, G05: "passed"},
			want: "entitlement: unknown",
		},
		{
			name: "an unadmitted posture is unknown",
			in:   adapter.BillingPosture{},
			want: "entitlement: unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := admission.EntitlementStatement(tt.in); got != tt.want {
				t.Errorf("EntitlementStatement = %q, want %q", got, tt.want)
			}
		})
	}
}
