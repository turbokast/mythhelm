package admission

import (
	"strings"

	"github.com/turbokast/mythhelm/internal/adapter"
)

// EntitlementStatement renders the verbatim entitlement sentence for a
// posture. Only a qualified posture whose G05 passed and whose source is not a
// user declaration reads as verified; a posture that fits no case is unknown,
// never the declared or verified sentence (I15).
func EntitlementStatement(b adapter.BillingPosture) string {
	declared := strings.HasPrefix(b.EntitlementSource, "user_declared")
	switch {
	case b.Mode == BillingLocalScripted:
		return "entitlement: not applicable (local-scripted: no inference, no network)"
	case !b.Qualified && declared:
		return "entitlement: user-declared, NOT verified by MYTHHELM; paid continuation: unknown (user declares disabled)"
	case b.Qualified && b.G05 == "passed" && !declared:
		return "entitlement: verified by MYTHHELM (" + b.EntitlementSource + "); paid continuation: " + b.PaidContinuation
	}
	return "entitlement: unknown"
}
