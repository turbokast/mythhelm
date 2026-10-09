package v2contract_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

// catalogue pins the v2 §4.5 list: each exported constant, its wire word and
// its design §5 default disposition, in design order.
var catalogue = []struct {
	code v2contract.Code
	word string
	def  v2contract.Disposition
}{
	{v2contract.CodeInvalidContract, "invalid_contract", v2contract.DispositionNever},
	{v2contract.CodeRevisionConflict, "revision_conflict", v2contract.DispositionAfterReconciliation},
	{v2contract.CodeDependencyStale, "dependency_stale", v2contract.DispositionAfterReconciliation},
	{v2contract.CodeAuthUnavailable, "auth_unavailable", v2contract.DispositionAfterUserAction},
	{v2contract.CodeEntitlementUnknown, "entitlement_unknown", v2contract.DispositionAfterUserAction},
	{v2contract.CodeEntitlementIneligible, "entitlement_ineligible", v2contract.DispositionNever},
	{v2contract.CodeAllowanceExhausted, "allowance_exhausted", v2contract.DispositionAfterUserAction},
	{v2contract.CodeCapabilityUnsupported, "capability_unsupported", v2contract.DispositionNever},
	{v2contract.CodePermissionDenied, "permission_denied", v2contract.DispositionAfterUserAction},
	{v2contract.CodeProviderThrottled, "provider_throttled", v2contract.DispositionAfterCooldown},
	{v2contract.CodeProtocolMismatch, "protocol_mismatch", v2contract.DispositionAfterUserAction},
	{v2contract.CodeProcessLost, "process_lost", v2contract.DispositionAfterReconciliation},
	{v2contract.CodeOwnershipUnresolved, "ownership_unresolved", v2contract.DispositionAfterReconciliation},
	{v2contract.CodeToolFailed, "tool_failed", v2contract.DispositionBoundedTransient},
	{v2contract.CodeCandidateRejected, "candidate_rejected", v2contract.DispositionNever},
	{v2contract.CodeIntegrationConflict, "integration_conflict", v2contract.DispositionAfterReconciliation},
	{v2contract.CodeVerificationFailed, "verification_failed", v2contract.DispositionNever},
	{v2contract.CodeVerificationUnavailable, "verification_unavailable", v2contract.DispositionBoundedTransient},
	{v2contract.CodePersistenceUnavailable, "persistence_unavailable", v2contract.DispositionBoundedTransient},
	{v2contract.CodeCancelIncomplete, "cancel_incomplete", v2contract.DispositionAfterReconciliation},
	{v2contract.CodeExternalEffectUncertain, "external_effect_uncertain", v2contract.DispositionAfterReconciliation},
	{v2contract.CodeBudgetExhausted, "budget_exhausted", v2contract.DispositionAfterUserAction},
	{v2contract.CodePolicyIneligible, "policy_ineligible", v2contract.DispositionNever},
	{v2contract.CodeSchemaTooNew, "schema_too_new", v2contract.DispositionAfterUserAction},
}

// G04 (v2 §18.3): the constant set equals the §4.5 list exactly.
func TestCatalogueHas24Codes(t *testing.T) {
	t.Parallel()
	if len(catalogue) != 24 {
		t.Fatalf("catalogue table has %d rows, want 24", len(catalogue))
	}
	for i, row := range catalogue {
		if string(row.code) != row.word {
			t.Errorf("catalogue[%d] constant = %q, want word %q", i, row.code, row.word)
		}
		if !row.code.Valid() {
			t.Errorf("catalogue[%d] code %q not Valid", i, row.word)
		}
		if !v2contract.Code(row.word).Valid() {
			t.Errorf("catalogue[%d] word %q not Valid", i, row.word)
		}
	}
	for _, bad := range []string{"", "TOOL_FAILED", "tool_failed ", "tool-failed", "unknown_code", "release_blocked"} {
		if v2contract.Code(bad).Valid() {
			t.Errorf("code %q reported valid", bad)
		}
		if got := v2contract.Code(bad).DefaultDisposition(); got != "" {
			t.Errorf("code %q default disposition = %q, want empty", bad, got)
		}
	}
	// A 25th Code constant cannot be caught by referencing the 24 known ones,
	// so count the declarations at the source: constants are not
	// runtime-enumerable.
	sourced := codeConstantsFromSource(t)
	if len(sourced) != 24 {
		t.Fatalf("errors.go declares %d Code constants, want 24", len(sourced))
	}
	want := map[string]bool{}
	for _, row := range catalogue {
		want[row.word] = true
	}
	for name, word := range sourced {
		if !want[word] {
			t.Errorf("errors.go constant %s = %q outside the §4.5 list", name, word)
		}
	}
}

// codeConstantsFromSource collects the Name -> wire value of every constant
// whose identifier starts with "Code" in errors.go.
func codeConstantsFromSource(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "errors.go", nil, 0)
	if err != nil {
		t.Fatalf("parse errors.go: %v", err)
	}
	out := map[string]string{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				id := name.Name
				if id == "Code" || !strings.HasPrefix(id, "Code") {
					continue
				}
				var v ast.Expr
				switch len(vs.Values) {
				case 0:
					t.Fatalf("constant %s has no value literal", id)
				case 1:
					v = vs.Values[0]
				default:
					if i >= len(vs.Values) {
						t.Fatalf("constant %s misaligned values", id)
					}
					v = vs.Values[i]
				}
				lit, ok := v.(*ast.BasicLit)
				if !ok {
					t.Fatalf("constant %s value is not a literal", id)
				}
				s, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquote %s: %v", id, err)
				}
				out[id] = s
			}
		}
	}
	return out
}

// G04 (v2 §18.3): all 24 code→disposition mappings equal design §5's table.
func TestDefaultDispositions(t *testing.T) {
	t.Parallel()
	known := map[v2contract.Disposition]bool{
		v2contract.DispositionNever:               true,
		v2contract.DispositionAfterUserAction:     true,
		v2contract.DispositionAfterReconciliation: true,
		v2contract.DispositionAfterCooldown:       true,
		v2contract.DispositionBoundedTransient:    true,
	}
	if len(known) != 5 {
		t.Fatalf("disposition set has %d words, want 5", len(known))
	}
	for _, row := range catalogue {
		if got := row.code.DefaultDisposition(); got != row.def {
			t.Errorf("%s default = %q, want %q", row.word, got, row.def)
		}
		if got := v2contract.Code(row.word).DefaultDisposition(); got != row.def {
			t.Errorf("Code(%q) default = %q, want %q", row.word, got, row.def)
		}
		if !known[row.def] {
			t.Errorf("%s default %q outside the 5 dispositions", row.word, row.def)
		}
	}
}

// I03 (v2 §2): dispositions never widen above the code default (D9 rank order).
func TestValidateRejectsWidenedDisposition(t *testing.T) {
	t.Parallel()
	valid := func() *v2contract.ControlError {
		return &v2contract.ControlError{
			Code:        v2contract.CodeEntitlementIneligible,
			Owner:       "supervisor",
			OperationID: "op_demo_0001",
			Disposition: v2contract.DispositionNever,
			NextAction:  "do not retry",
		}
	}
	if err := valid().Validate(); err != nil {
		t.Fatalf("default disposition: %v", err)
	}
	tests := []struct {
		name    string
		mutate  func(e *v2contract.ControlError)
		wantErr string
	}{
		{"widened never to bounded_transient", func(e *v2contract.ControlError) {
			e.Disposition = v2contract.DispositionBoundedTransient
		}, "disposition"},
		{"widened never to after_user_action", func(e *v2contract.ControlError) {
			e.Disposition = v2contract.DispositionAfterUserAction
		}, "disposition"},
		{"widened reconciliation to cooldown", func(e *v2contract.ControlError) {
			e.Code = v2contract.CodeRevisionConflict
			e.Disposition = v2contract.DispositionAfterCooldown
		}, "disposition"},
		{"unknown disposition", func(e *v2contract.ControlError) {
			e.Disposition = "someday"
		}, "disposition"},
		{"unknown code", func(e *v2contract.ControlError) {
			e.Code = "bogus"
		}, "code"},
		{"empty owner", func(e *v2contract.ControlError) { e.Owner = "" }, "owner"},
		{"empty operation_id", func(e *v2contract.ControlError) { e.OperationID = "" }, "operation_id"},
		{"empty next_action", func(e *v2contract.ControlError) { e.NextAction = "" }, "next_action"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := valid()
			tc.mutate(e)
			err := e.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
	// Lower ranks narrow authority and pass: never for a bounded_transient
	// default, and reconciliation/user narrowing for a cooldown default.
	narrowed := valid()
	narrowed.Code = v2contract.CodeToolFailed
	narrowed.Disposition = v2contract.DispositionNever
	if err := narrowed.Validate(); err != nil {
		t.Errorf("narrowed tool_failed/never: %v", err)
	}
	narrowed.Disposition = v2contract.DispositionBoundedTransient
	if err := narrowed.Validate(); err != nil {
		t.Errorf("default tool_failed/bounded_transient: %v", err)
	}
	throttled := valid()
	throttled.Code = v2contract.CodeProviderThrottled
	for _, d := range []v2contract.Disposition{
		v2contract.DispositionNever,
		v2contract.DispositionAfterUserAction,
		v2contract.DispositionAfterReconciliation,
		v2contract.DispositionAfterCooldown,
	} {
		throttled.Disposition = d
		if err := throttled.Validate(); err != nil {
			t.Errorf("provider_throttled/%s: %v", d, err)
		}
	}
	var nilErr *v2contract.ControlError
	if err := nilErr.Validate(); err == nil {
		t.Errorf("nil ControlError Validate = nil, want error")
	}
}

// I03 (v2 §2): adapter detail is namespaced and grants no authority.
func TestValidateRejectsUnnamespacedDetail(t *testing.T) {
	t.Parallel()
	valid := func() *v2contract.ControlError {
		return &v2contract.ControlError{
			Code:        v2contract.CodeToolFailed,
			Owner:       "supervisor",
			OperationID: "op_demo_0001",
			Disposition: v2contract.DispositionBoundedTransient,
			NextAction:  "retry with backoff",
			Detail:      map[string]string{"adapter/claudecode/cause": "exit 1"},
		}
	}
	if err := valid().Validate(); err != nil {
		t.Fatalf("namespaced detail: %v", err)
	}
	for _, detail := range []map[string]string{nil, {}} {
		e := valid()
		e.Detail = detail
		if err := e.Validate(); err != nil {
			t.Errorf("absent detail: %v", err)
		}
	}
	tests := []struct {
		name    string
		detail  map[string]string
		wantKey string
	}{
		{"flat key", map[string]string{"cause": "boom"}, "cause"},
		{"one flat among namespaced", map[string]string{"adapter/claudecode/cause": "ok", "flat": "bad"}, "flat"},
		{"empty namespace", map[string]string{"/cause": "boom"}, "/cause"},
		{"empty key", map[string]string{"adapter/": "boom"}, "adapter/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := valid()
			e.Detail = tc.detail
			err := e.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.wantKey) {
				t.Fatalf("Validate error = %v, want naming %q", err, tc.wantKey)
			}
		})
	}
}

func mustMap(t *testing.T, code v2contract.Code, owner, op, ns string, cause error) *v2contract.ControlError {
	t.Helper()
	e, err := v2contract.MapAdapterFailure(code, owner, op, ns, cause)
	if err != nil {
		t.Fatalf("MapAdapterFailure(%q, %q): %v", code, ns, err)
	}
	return e
}

// AC-3.2: adapter failures map to a required code with its default,
// never-widening disposition and namespaced detail.
func TestMapAdapterFailure(t *testing.T) {
	t.Parallel()
	got := mustMap(t,
		v2contract.CodeToolFailed, "owner1", "op1", "adapter/claudecode", errors.New("boom"))
	if got.Code != v2contract.CodeToolFailed {
		t.Errorf("Code = %q, want tool_failed", got.Code)
	}
	if want := v2contract.CodeToolFailed.DefaultDisposition(); got.Disposition != want {
		t.Errorf("Disposition = %q, want default %q", got.Disposition, want)
	}
	if got.Detail["adapter/claudecode/cause"] != "boom" {
		t.Errorf("Detail = %v, want adapter/claudecode/cause=boom", got.Detail)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("mapped error Validate: %v", err)
	}
	wantErr := "v2contract: tool_failed: " + got.NextAction
	if got.Error() != wantErr {
		t.Errorf("Error() = %q, want %q", got.Error(), wantErr)
	}
	// Code — not message — drives transitions: the same code with a
	// different cause takes the same branch.
	other := mustMap(t,
		v2contract.CodeToolFailed, "owner2", "op2", "adapter/other", errors.New("different"))
	if other.Code != got.Code {
		t.Errorf("same-code errors differ: %q vs %q", other.Code, got.Code)
	}
	if other.Error() == got.Error() {
		t.Errorf("distinct failures share Error() text %q", got.Error())
	}
	ineligible := mustMap(t,
		v2contract.CodeEntitlementIneligible, "owner1", "op1", "adapter/native", errors.New("nope"))
	if ineligible.Disposition != v2contract.DispositionNever {
		t.Errorf("ineligible Disposition = %q, want never", ineligible.Disposition)
	}
	if err := ineligible.Validate(); err != nil {
		t.Errorf("ineligible mapped error Validate: %v", err)
	}
	var nilCause error
	unknown := mustMap(t,
		v2contract.CodeProcessLost, "owner1", "op1", "adapter/native", nilCause)
	if unknown.Detail["adapter/native/cause"] != "unknown" {
		t.Errorf("nil-cause Detail = %v, want unknown stated explicitly", unknown.Detail)
	}
	if err := unknown.Validate(); err != nil {
		t.Errorf("nil-cause mapped error Validate: %v", err)
	}
}

// I03 (v2 §2): the mapper refuses input it would turn into an invalid or
// authority-less error instead of returning one.
func TestMapAdapterFailureRejectsBadInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		code      v2contract.Code
		namespace string
		wantErr   string
	}{
		{"empty namespace", v2contract.CodeToolFailed, "", "namespace"},
		{"unknown code", v2contract.Code("made_up"), "adapter/native", `"made_up"`},
		{"empty code", v2contract.Code(""), "adapter/native", "code"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := v2contract.MapAdapterFailure(tc.code, "owner1", "op1", tc.namespace, errors.New("x"))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
			if got != nil {
				t.Errorf("got %+v alongside error, want nil", got)
			}
		})
	}
}

// AC-9.1: the golden decodes strictly, re-encodes byte-identical, and an
// absent revision omits (I09: missing is never zero).
func TestErrorGoldenRoundTrip(t *testing.T) {
	t.Parallel()
	raw := loadGolden(t, "error.golden.json")
	got, err := v2contract.Decode[*v2contract.ControlError](raw)
	if err != nil {
		t.Fatalf("Decode golden: %v", err)
	}
	if got.Revision == nil || *got.Revision != 3 {
		t.Fatalf("golden revision = %v, want 3 (all fields set)", got.Revision)
	}
	re, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("re-encode golden: %v", err)
	}
	if want := bytes.TrimSpace(raw); !bytes.Equal(re, want) {
		t.Fatalf("re-encoded golden differs:\n got: %s\nwant: %s", re, want)
	}
	got.Revision = nil
	omitted, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("re-encode without revision: %v", err)
	}
	if strings.Contains(string(omitted), "revision") {
		t.Errorf("absent revision encoded as %s, want omitted", omitted)
	}
	if _, err := v2contract.Decode[*v2contract.ControlError](omitted); err != nil {
		t.Errorf("Decode without revision: %v", err)
	}
}
