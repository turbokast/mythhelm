package v2contract

import (
	"errors"
	"fmt"
	"strings"
)

// Code is a v2 §4.5 error code. Code — not message — drives transitions (G04).
type Code string

// The 24 required v2 §4.5 codes.
const (
	CodeInvalidContract         Code = "invalid_contract"
	CodeRevisionConflict        Code = "revision_conflict"
	CodeDependencyStale         Code = "dependency_stale"
	CodeAuthUnavailable         Code = "auth_unavailable"
	CodeEntitlementUnknown      Code = "entitlement_unknown"
	CodeEntitlementIneligible   Code = "entitlement_ineligible"
	CodeAllowanceExhausted      Code = "allowance_exhausted"
	CodeCapabilityUnsupported   Code = "capability_unsupported"
	CodePermissionDenied        Code = "permission_denied"
	CodeProviderThrottled       Code = "provider_throttled"
	CodeProtocolMismatch        Code = "protocol_mismatch"
	CodeProcessLost             Code = "process_lost"
	CodeOwnershipUnresolved     Code = "ownership_unresolved"
	CodeToolFailed              Code = "tool_failed"
	CodeCandidateRejected       Code = "candidate_rejected"
	CodeIntegrationConflict     Code = "integration_conflict"
	CodeVerificationFailed      Code = "verification_failed"
	CodeVerificationUnavailable Code = "verification_unavailable"
	CodePersistenceUnavailable  Code = "persistence_unavailable"
	CodeCancelIncomplete        Code = "cancel_incomplete"
	CodeExternalEffectUncertain Code = "external_effect_uncertain"
	CodeBudgetExhausted         Code = "budget_exhausted"
	CodePolicyIneligible        Code = "policy_ineligible"
	CodeSchemaTooNew            Code = "schema_too_new"
)

// Valid reports whether c is in the 24-code catalogue.
func (c Code) Valid() bool {
	return c.DefaultDisposition() != ""
}

// Disposition is a retry disposition. Ranks order retry permissiveness:
// never < after_user_action < after_reconciliation < after_cooldown <
// bounded_transient (D9).
type Disposition string

// Retry dispositions.
const (
	DispositionNever               Disposition = "never"
	DispositionAfterUserAction     Disposition = "after_user_action"
	DispositionAfterReconciliation Disposition = "after_reconciliation"
	DispositionAfterCooldown       Disposition = "after_cooldown"
	DispositionBoundedTransient    Disposition = "bounded_transient"
)

// DefaultDisposition returns the catalogue default disposition for c
// (design §5). It returns "" for codes outside the catalogue.
func (c Code) DefaultDisposition() Disposition {
	switch c {
	case CodeInvalidContract:
		return DispositionNever
	case CodeRevisionConflict:
		return DispositionAfterReconciliation
	case CodeDependencyStale:
		return DispositionAfterReconciliation
	case CodeAuthUnavailable:
		return DispositionAfterUserAction
	case CodeEntitlementUnknown:
		return DispositionAfterUserAction
	case CodeEntitlementIneligible:
		return DispositionNever
	case CodeAllowanceExhausted:
		return DispositionAfterUserAction
	case CodeCapabilityUnsupported:
		return DispositionNever
	case CodePermissionDenied:
		return DispositionAfterUserAction
	case CodeProviderThrottled:
		return DispositionAfterCooldown
	case CodeProtocolMismatch:
		return DispositionAfterUserAction
	case CodeProcessLost:
		return DispositionAfterReconciliation
	case CodeOwnershipUnresolved:
		return DispositionAfterReconciliation
	case CodeToolFailed:
		return DispositionBoundedTransient
	case CodeCandidateRejected:
		return DispositionNever
	case CodeIntegrationConflict:
		return DispositionAfterReconciliation
	case CodeVerificationFailed:
		return DispositionNever
	case CodeVerificationUnavailable:
		return DispositionBoundedTransient
	case CodePersistenceUnavailable:
		return DispositionBoundedTransient
	case CodeCancelIncomplete:
		return DispositionAfterReconciliation
	case CodeExternalEffectUncertain:
		return DispositionAfterReconciliation
	case CodeBudgetExhausted:
		return DispositionAfterUserAction
	case CodePolicyIneligible:
		return DispositionNever
	case CodeSchemaTooNew:
		return DispositionAfterUserAction
	}
	return ""
}

// dispositionRank orders retry permissiveness; unknown words rank -1.
func dispositionRank(d Disposition) int {
	switch d {
	case DispositionNever:
		return 0
	case DispositionAfterUserAction:
		return 1
	case DispositionAfterReconciliation:
		return 2
	case DispositionAfterCooldown:
		return 3
	case DispositionBoundedTransient:
		return 4
	}
	return -1
}

// ControlError is the v2 §4.5 error: code drives transitions, the message
// only explains. Revision is a pointer so absence omits it (I09).
type ControlError struct {
	Code        Code              `json:"code" toml:"code"`
	Owner       string            `json:"owner" toml:"owner"`
	OperationID string            `json:"operation_id" toml:"operation_id"`
	Object      string            `json:"object,omitempty" toml:"object,omitempty"`
	Revision    *int              `json:"revision,omitempty" toml:"revision,omitempty"`
	Disposition Disposition       `json:"disposition" toml:"disposition"`
	NextAction  string            `json:"next_action" toml:"next_action"`
	Evidence    []string          `json:"evidence_refs,omitempty" toml:"evidence_refs,omitempty"`
	Detail      map[string]string `json:"detail,omitempty" toml:"detail,omitempty"`
}

// Error returns "v2contract: <code>: <next_action>".
func (e *ControlError) Error() string {
	if e == nil {
		return "v2contract: <nil control error>"
	}
	return fmt.Sprintf("v2contract: %s: %s", e.Code, e.NextAction)
}

// Validate requires a catalogue code, non-empty owner, operation_id and
// next_action, a disposition ranked at or below the code default (never
// widening authority, I03), and namespaced detail keys.
func (e *ControlError) Validate() error {
	if e == nil {
		return errors.New("v2contract: control error is nil")
	}
	if !e.Code.Valid() {
		return fmt.Errorf("v2contract: control error code %q not in catalogue", string(e.Code))
	}
	if e.Owner == "" {
		return errors.New("v2contract: control error owner is empty")
	}
	if e.OperationID == "" {
		return errors.New("v2contract: control error operation_id is empty")
	}
	if e.NextAction == "" {
		return errors.New("v2contract: control error next_action is empty")
	}
	rank := dispositionRank(e.Disposition)
	if rank < 0 {
		return fmt.Errorf("v2contract: control error disposition %q is unknown", string(e.Disposition))
	}
	if def := e.Code.DefaultDisposition(); rank > dispositionRank(def) {
		return fmt.Errorf("v2contract: control error disposition %q widens default %q for code %q",
			string(e.Disposition), string(def), string(e.Code))
	}
	for k := range e.Detail {
		if !namespacedKey(k) {
			return fmt.Errorf("v2contract: control error detail key %q is not namespaced", k)
		}
	}
	return nil
}

// namespacedKey requires "<namespace>/<key>" with both parts non-empty.
func namespacedKey(k string) bool {
	i := strings.Index(k, "/")
	return i > 0 && i < len(k)-1
}

// MapAdapterFailure maps an adapter failure to a required code with its
// default (never-widening) disposition and namespaced detail. The detail
// key is namespace + "/cause" holding the cause text, or "unknown" when
// cause is nil (I09). NextAction points at that detail; callers replace it
// with code-specific guidance where they have it.
func MapAdapterFailure(code Code, owner, operationID, namespace string, cause error) *ControlError {
	detail := "unknown"
	if cause != nil {
		detail = cause.Error()
	}
	key := namespace + "/cause"
	return &ControlError{
		Code:        code,
		Owner:       owner,
		OperationID: operationID,
		Disposition: code.DefaultDisposition(),
		NextAction:  "see detail " + key,
		Detail:      map[string]string{key: detail},
	}
}
