package journal

import (
	"errors"
	"fmt"
	ab "github.com/turbokast/mythhelm/internal/admissionbinding"
	"github.com/turbokast/mythhelm/internal/v2contract"
	"reflect"
	"testing"
)

func TestAdmissionTypedFailureMapping(t *testing.T) { // I02/I09 (v2 §§6/8): leaf failures never become authority or raw diagnostics.
	t.Parallel()
	for _, tc := range []struct {
		name string
		err  error
		code v2contract.Code
	}{
		{"invalid", ab.ErrInvalidContract, v2contract.CodeInvalidContract},
		{"future", ab.ErrCapabilityUnsupported, v2contract.CodeCapabilityUnsupported},
		{"wrapped invalid", fmt.Errorf("private decoder detail: %w", ab.ErrInvalidContract), v2contract.CodeInvalidContract},
		{"twice wrapped invalid", fmt.Errorf("private outer: %w", fmt.Errorf("private inner: %w", ab.ErrInvalidContract)), v2contract.CodeInvalidContract},
		{"wrapped future", fmt.Errorf("private decoder detail: %w", ab.ErrCapabilityUnsupported), v2contract.CodeCapabilityUnsupported},
		{"twice wrapped future", fmt.Errorf("private outer: %w", fmt.Errorf("private inner: %w", ab.ErrCapabilityUnsupported)), v2contract.CodeCapabilityUnsupported},
		{"unexpected", errors.New("unsupported decoder secret path"), v2contract.CodeInvalidContract},
		{"both sentinels", errors.Join(ab.ErrInvalidContract, ab.ErrCapabilityUnsupported), v2contract.CodeInvalidContract},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := admissionLeafError(tc.err)
			var ce *v2contract.ControlError
			if !errors.As(err, &ce) || ce.Code != tc.code {
				t.Fatalf("typed mapping=%v want %s", err, tc.code)
			}
			if e := ce.Validate(); e != nil {
				t.Fatal(e)
			}
			want := map[string]string{"mythhelm.admission_binding/reason": string(tc.code)}
			if !reflect.DeepEqual(ce.Detail, want) {
				t.Fatalf("unsafe detail: %#v", ce.Detail)
			}
		})
	}
	for _, code := range []v2contract.Code{v2contract.CodeInvalidContract, v2contract.CodePermissionDenied, v2contract.CodeRevisionConflict, v2contract.CodeDependencyStale, v2contract.CodePolicyIneligible, v2contract.CodeCapabilityUnsupported, v2contract.CodePersistenceUnavailable, v2contract.CodeAllowanceExhausted, v2contract.CodeOwnershipUnresolved} {
		t.Run(string(code), func(t *testing.T) {
			t.Parallel()
			var ce *v2contract.ControlError
			if !errors.As(admissionStorageError(code), &ce) || ce.Code != code || ce.Validate() != nil {
				t.Fatalf("invalid bounded storage error: %v", ce)
			}
		})
	}
	var retention *v2contract.ControlError
	if !errors.As(retentionStorageError(), &retention) || retention.Code != v2contract.CodePolicyIneligible || !reflect.DeepEqual(retention.Detail, map[string]string{"mythhelm.retention/reason": "policy_ineligible"}) {
		t.Fatal("retention refusal lost its separate policy classification")
	}
}

func TestAdmissionOperationBoundaryDiagnostics(t *testing.T) { // I09 (v2 §8): normalized operation errors disclose no nested input/error text.
	t.Parallel()
	raw := &v2contract.ControlError{Code: v2contract.CodePersistenceUnavailable, Owner: "private owner", OperationID: "private operation", NextAction: "private raw text", Disposition: v2contract.DispositionBoundedTransient, Detail: map[string]string{"private/key": "private value"}}
	err := admissionTransactionError(fmt.Errorf("private wrapper: %w", raw))
	var ce *v2contract.ControlError
	if !errors.As(err, &ce) || ce.Validate() != nil || ce.Owner != "journal" || ce.OperationID != "service-admission.storage" || !reflect.DeepEqual(ce.Detail, map[string]string{"mythhelm.admission_binding/reason": "persistence_unavailable"}) {
		t.Fatalf("unsafe operation diagnostic: %#v", ce)
	}
}
