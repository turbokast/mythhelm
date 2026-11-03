package admission

import (
	"encoding/hex"
	"errors"
	"time"

	"github.com/turbokast/mythhelm/internal/admissionbinding"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// RetentionPolicyV1 aliases the closed leaf policy record.
type RetentionPolicyV1 = admissionbinding.RetentionPolicyV1

// RetentionApprovalV1 aliases the closed leaf approval record.
type RetentionApprovalV1 = admissionbinding.RetentionApprovalV1

// CheckRetentionV1 checks that approval is an exact affirmative record for this
// binding under this complete policy at now. It checks shape and consistency
// only: a caller-constructed record is never authority, and the check has no
// effect. Malformed or unsupported records map to invalid_contract or
// capability_unsupported; every structurally valid refusal is policy_ineligible.
// Callers replace OperationID with their authenticated operation identity.
func CheckRetentionV1(binding admissionbinding.BindingV1, policy RetentionPolicyV1, approval *RetentionApprovalV1, now time.Time) error {
	rows, err := admissionbinding.DeriveRetentionFieldsV1(binding, policy)
	if err != nil {
		return leafRetentionError(err)
	}
	if approval == nil {
		return retentionIneligible()
	}
	if err := approval.Validate(); err != nil {
		return leafRetentionError(err)
	}
	// I02/I19 (v2 §§6/8): every retained value needs an explicit non-secret
	// rule; native-auth, transcript and credential-environment provenance is
	// refused whatever the rule's classification or the approval says.
	derived := make(map[string]bool, len(rows))
	for _, r := range rows {
		derived[r.FieldPath] = true
		if r.Classification != admissionbinding.RetentionNonSecret || forbiddenProvenance(r.Provenance) {
			return retentionIneligible()
		}
	}
	if len(policy.Content.Rules) != len(rows) {
		return retentionIneligible()
	}
	for _, r := range policy.Content.Rules {
		if !derived[r.FieldPath] || forbiddenProvenance(r.Provenance) {
			return retentionIneligible()
		}
	}
	bindingSum, err := admissionbinding.DigestV1(binding)
	if err != nil {
		return leafRetentionError(err)
	}
	fieldSum, err := admissionbinding.RetentionFieldSetSHA256(rows)
	if err != nil {
		return leafRetentionError(err)
	}
	expires, err := time.Parse(time.RFC3339Nano, binding.ExpiresAt)
	if err != nil {
		return leafRetentionError(err)
	}
	observed, err := time.Parse(time.RFC3339Nano, approval.ObservedAt)
	if err != nil {
		return leafRetentionError(err)
	}
	a := approval
	if a.Decision != admissionbinding.RetentionApprove ||
		a.PreparationID != binding.PreparationID || a.ChallengeID != retentionChallengeID(binding) ||
		a.BindingSHA256 != hex.EncodeToString(bindingSum[:]) || a.FieldSetSHA256 != hex.EncodeToString(fieldSum[:]) ||
		a.PolicyRevision != policy.Revision || binding.PolicyRevision != policy.Revision ||
		a.PeerID != binding.PeerID || a.RootID != binding.RootID || policy.Content.RootID != binding.RootID ||
		a.ContextEpoch != binding.ContextEpoch || a.Generation != binding.Generation ||
		a.ExpiresAt != binding.ExpiresAt || observed.After(now) || !now.Before(expires) {
		return retentionIneligible()
	}
	return nil
}

// retentionChallengeID returns the ID of the single retention challenge, or ""
// when the binding has none or more than one.
func retentionChallengeID(b admissionbinding.BindingV1) string {
	id := ""
	for _, ch := range b.Challenges {
		if ch.Kind != admissionbinding.RetentionChallengeKind {
			continue
		}
		if id != "" {
			return ""
		}
		id = ch.ID
	}
	return id
}

func forbiddenProvenance(p admissionbinding.RetentionProvenanceV1) bool {
	switch p {
	case admissionbinding.RetentionNativeAuth, admissionbinding.RetentionTranscript, admissionbinding.RetentionCredentialEnv:
		return true
	}
	return false
}

// leafRetentionError maps leaf sentinels to safe constant control errors.
// Unexpected errors fail closed as invalid_contract; wrapped text never leaks.
func leafRetentionError(err error) error {
	if errors.Is(err, admissionbinding.ErrCapabilityUnsupported) && !errors.Is(err, admissionbinding.ErrInvalidContract) {
		return retentionControlError(v2contract.CodeCapabilityUnsupported, "mythhelm.admission_binding/reason", "capability_unsupported")
	}
	return retentionControlError(v2contract.CodeInvalidContract, "mythhelm.admission_binding/reason", "invalid_contract")
}

func retentionIneligible() error {
	return retentionControlError(v2contract.CodePolicyIneligible, "mythhelm.retention/reason", "policy_ineligible")
}

func retentionControlError(code v2contract.Code, key, reason string) error {
	return &v2contract.ControlError{
		Code:        code,
		Owner:       "admission",
		OperationID: "admission.retention_check",
		Disposition: code.DefaultDisposition(),
		NextAction:  "prepare the run again with an exact retention policy and approval",
		Detail:      map[string]string{key: reason},
	}
}
