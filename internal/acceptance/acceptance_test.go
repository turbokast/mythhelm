package acceptance_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/acceptance"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

const (
	evalA = "sha256:aaaa"
	evalB = "sha256:bbbb"
)

type statusRow struct {
	name       string
	statuses   []string
	wantResult acceptance.Result
	wantReason string
}

// statusRows are the verdicts over check statuses alone, shared by Decide
// (with a matching evaluator) and DecideStatuses.
var statusRows = []statusRow{
	{"every check passed", []string{"passed", "passed"}, acceptance.ResultPassed, ""},
	{"a failed check with an unavailable one is failed", []string{"unavailable", "failed"}, acceptance.ResultFailed, "checks_failed"},
	{"a timed out check is failed", []string{"passed", "timed_out"}, acceptance.ResultFailed, "checks_failed"},
	{"a failed check with an unknown status is failed", []string{"weird", "failed"}, acceptance.ResultFailed, "checks_failed"},
	{"an unavailable check is unavailable", []string{"passed", "unavailable"}, acceptance.ResultUnavailable, "checks_unavailable"},
	{"a not run check is unavailable", []string{"passed", "not_run"}, acceptance.ResultUnavailable, "checks_unavailable"},
	{"an empty status list is unavailable", []string{}, acceptance.ResultUnavailable, "no_checks_ran"},
	{"a nil status list is unavailable", nil, acceptance.ResultUnavailable, "no_checks_ran"},
	{"an unknown status is unavailable", []string{"passed", "weird"}, acceptance.ResultUnavailable, "checks_unavailable"},
	{"an unknown status alone is unavailable", []string{"PASSED"}, acceptance.ResultUnavailable, "checks_unavailable"},
}

func TestDecideOrdering(t *testing.T) {
	t.Parallel()
	type row struct {
		name       string
		pin, ran   string
		statuses   []string
		wantResult acceptance.Result
		wantReason string
	}
	var rows []row
	for _, s := range statusRows {
		rows = append(rows, row{s.name, evalA, evalA, s.statuses, s.wantResult, s.wantReason})
	}
	rows = append(rows,
		row{"an empty pin evaluator is invalid", "", evalA, []string{"passed"}, acceptance.ResultInvalid, "evaluator_pin_missing"},
		row{"an empty pin and an empty ran digest is a missing pin", "", "", []string{"passed"}, acceptance.ResultInvalid, "evaluator_pin_missing"},
		row{"a differing ran digest is invalid even when every check passed", evalA, evalB, []string{"passed", "passed"}, acceptance.ResultInvalid, "evaluator_digest_mismatch"},
		row{"an empty ran digest is a mismatch", evalA, "", []string{"passed"}, acceptance.ResultInvalid, "evaluator_digest_mismatch"},
		row{"invalid outranks failed", evalA, evalB, []string{"failed"}, acceptance.ResultInvalid, "evaluator_digest_mismatch"},
	)
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, reason := acceptance.Decide(tc.pin, tc.ran, tc.statuses)
			if got != tc.wantResult || reason != tc.wantReason {
				t.Errorf("Decide(%q, %q, %v) = %q, %q; want %q, %q", tc.pin, tc.ran, tc.statuses, got, reason, tc.wantResult, tc.wantReason)
			}
		})
	}
}

func TestDecideStatusesIgnoresEvaluator(t *testing.T) {
	t.Parallel()
	for _, tc := range statusRows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, reason := acceptance.DecideStatuses(tc.statuses)
			if got != tc.wantResult || reason != tc.wantReason {
				t.Errorf("DecideStatuses(%v) = %q, %q; want %q, %q", tc.statuses, got, reason, tc.wantResult, tc.wantReason)
			}
			viaDecide, viaReason := acceptance.Decide(evalA, evalA, tc.statuses)
			if got != viaDecide || reason != viaReason {
				t.Errorf("DecideStatuses(%v) = %q, %q; Decide with a matching evaluator = %q, %q", tc.statuses, got, reason, viaDecide, viaReason)
			}
			if got == acceptance.ResultInvalid {
				t.Errorf("DecideStatuses(%v) = invalid; it has no evaluator to compare", tc.statuses)
			}
		})
	}
}

func TestDecideUnknownStatusIsUnavailable(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"", "weird", "Passed", "passed "} {
		got, _ := acceptance.Decide(evalA, evalA, []string{status})
		if got != acceptance.ResultUnavailable {
			t.Errorf("Decide over status %q = %q, want unavailable", status, got)
		}
		got, _ = acceptance.DecideStatuses([]string{"passed", status})
		if got != acceptance.ResultUnavailable {
			t.Errorf("DecideStatuses over passed and %q = %q, want unavailable", status, got)
		}
	}
}

func fullBinding() acceptance.Binding {
	return acceptance.Binding{
		Candidate:       v2contract.GitObject{Format: "sha1", Value: strings.Repeat("a", 40)},
		Contract:        "sha256:c1",
		CheckDefinition: "sha256:d1",
		Evaluator:       "sha256:e1",
		Environment:     "sha256:v1",
	}
}

func TestBindingDiffNamesDimensions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(got, want *acceptance.Binding)
		want   []string
	}{
		{"identical bindings do not differ", func(_, _ *acceptance.Binding) {}, nil},
		{"only the candidate", func(g, _ *acceptance.Binding) { g.Candidate.Value = strings.Repeat("b", 40) }, []string{"candidate"}},
		{"only the candidate format", func(g, _ *acceptance.Binding) { g.Candidate.Format = "sha256" }, []string{"candidate"}},
		{"only the contract", func(g, _ *acceptance.Binding) { g.Contract = "sha256:c2" }, []string{"contract"}},
		{"only the check definition", func(g, _ *acceptance.Binding) { g.CheckDefinition = "sha256:d2" }, []string{"check_definition"}},
		{"only the evaluator", func(g, _ *acceptance.Binding) { g.Evaluator = "sha256:e2" }, []string{"evaluator"}},
		{"only the environment", func(g, _ *acceptance.Binding) { g.Environment = "sha256:v2" }, []string{"environment"}},
		{"every dimension in the documented order", func(g, _ *acceptance.Binding) {
			g.Candidate.Value = strings.Repeat("b", 40)
			g.Contract, g.CheckDefinition, g.Evaluator, g.Environment = "x", "y", "z", "w"
		}, []string{"candidate", "contract", "check_definition", "evaluator", "environment"}},
		{"an empty candidate on the verdict side", func(g, _ *acceptance.Binding) { g.Candidate = v2contract.GitObject{} }, []string{"candidate"}},
		{"an empty candidate on both sides", func(g, w *acceptance.Binding) {
			g.Candidate, w.Candidate = v2contract.GitObject{}, v2contract.GitObject{}
		}, []string{"candidate"}},
		{"an empty contract on the verdict side", func(g, _ *acceptance.Binding) { g.Contract = "" }, []string{"contract"}},
		{"an empty contract on the wanted side", func(_, w *acceptance.Binding) { w.Contract = "" }, []string{"contract"}},
		{"an empty contract on both sides", func(g, w *acceptance.Binding) { g.Contract, w.Contract = "", "" }, []string{"contract"}},
		{"an empty check definition on both sides", func(g, w *acceptance.Binding) { g.CheckDefinition, w.CheckDefinition = "", "" }, []string{"check_definition"}},
		{"an empty evaluator on both sides", func(g, w *acceptance.Binding) { g.Evaluator, w.Evaluator = "", "" }, []string{"evaluator"}},
		{"an empty wanted environment is not compared", func(_, w *acceptance.Binding) { w.Environment = "" }, nil},
		{"an empty wanted environment is not compared even when the verdict has none", func(g, w *acceptance.Binding) { g.Environment, w.Environment = "", "" }, nil},
		{"a wanted environment the verdict lacks differs", func(g, _ *acceptance.Binding) { g.Environment = "" }, []string{"environment"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, want := fullBinding(), fullBinding()
			tc.mutate(&got, &want)
			if diff := got.Diff(want); !slices.Equal(diff, tc.want) {
				t.Errorf("Diff = %v, want %v", diff, tc.want)
			}
		})
	}
}

func TestEnvironmentIDStableAndValueFree(t *testing.T) {
	t.Parallel()
	base := acceptance.EnvironmentID("linux", "amd64", "host", []string{"HOME", "PATH", "LANG"})
	if !strings.HasPrefix(base, "sha256:") || len(base) != len("sha256:")+64 {
		t.Fatalf("EnvironmentID = %q, want sha256:<64 hex>", base)
	}
	names := []string{"PATH", "LANG", "HOME"}
	if got := acceptance.EnvironmentID("linux", "amd64", "host", names); got != base {
		t.Errorf("reordered names gave %q, want %q", got, base)
	}
	if !slices.Equal(names, []string{"PATH", "LANG", "HOME"}) {
		t.Errorf("EnvironmentID reordered its argument to %v", names)
	}
	changed := map[string]string{
		"goos":     acceptance.EnvironmentID("darwin", "amd64", "host", []string{"HOME", "PATH", "LANG"}),
		"goarch":   acceptance.EnvironmentID("linux", "arm64", "host", []string{"HOME", "PATH", "LANG"}),
		"verifier": acceptance.EnvironmentID("linux", "amd64", "boundary", []string{"HOME", "PATH", "LANG"}),
		"names":    acceptance.EnvironmentID("linux", "amd64", "host", []string{"HOME", "PATH"}),
	}
	for dim, got := range changed {
		if got == base {
			t.Errorf("changing %s left the environment ID unchanged", dim)
		}
	}
	if a, b := acceptance.EnvironmentID("a", "bc", "d", nil), acceptance.EnvironmentID("ab", "c", "d", nil); a == b {
		t.Errorf("field boundaries are ambiguous: %q == %q", a, b)
	}
}

func TestGitObjectOf(t *testing.T) {
	t.Parallel()
	sha1, sha256 := strings.Repeat("ab", 20), strings.Repeat("cd", 32)
	tests := []struct {
		name    string
		hex     string
		want    v2contract.GitObject
		wantErr bool
	}{
		{"40 hex is sha1", sha1, v2contract.GitObject{Format: "sha1", Value: sha1}, false},
		{"64 hex is sha256", sha256, v2contract.GitObject{Format: "sha256", Value: sha256}, false},
		{"41 characters is an error", sha1 + "a", v2contract.GitObject{}, true},
		{"a non-hex byte is an error", strings.Repeat("a", 39) + "g", v2contract.GitObject{}, true},
		{"empty is an error", "", v2contract.GitObject{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := acceptance.GitObjectOf(tc.hex)
			if (err != nil) != tc.wantErr {
				t.Fatalf("GitObjectOf(%q) error = %v, wantErr %v", tc.hex, err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("GitObjectOf(%q) = %+v, want %+v", tc.hex, got, tc.want)
			}
		})
	}
}

func TestAttestRoundTripValidates(t *testing.T) {
	t.Parallel()
	b := fullBinding()
	results := map[string]string{"unit": "passed", "lint": "passed"}
	logs := []string{"sha256:1111"}
	build := func() v2contract.Verification {
		return acceptance.Attest("ver_1", "host", b, results, logs)
	}
	v := build()
	if err := v.Validate(); err != nil {
		t.Fatalf("Attest does not validate: %v", err)
	}
	if got := acceptance.BindingOf(v); got != b {
		t.Errorf("BindingOf(Attest(b)) = %+v, want %+v", got, b)
	}
	if v.VerificationID != "ver_1" || v.VerifierID != "host" || v.SchemaVersion != v2contract.SchemaVersion {
		t.Errorf("identity = %q, %q, schema %d", v.VerificationID, v.VerifierID, v.SchemaVersion)
	}
	if v.AcceptanceContractDigest != b.Contract || v.CheckDefinition != b.CheckDefinition || v.CheckVersion != b.Evaluator || v.Environment != b.Environment {
		t.Errorf("attestation dimensions = %q %q %q %q, want the binding's", v.AcceptanceContractDigest, v.CheckDefinition, v.CheckVersion, v.Environment)
	}
	if !slices.Equal(v.Logs, logs) || v.Results["unit"] != "passed" || v.Results["lint"] != "passed" || len(v.Results) != 2 {
		t.Errorf("results = %v, logs = %v", v.Results, v.Logs)
	}
	d1, d2 := v2contract.Digest(v), v2contract.Digest(build())
	if d1 == "" || d1 != d2 {
		t.Errorf("digest not stable: %q vs %q", d1, d2)
	}
}

func TestPinDigestForms(t *testing.T) {
	t.Parallel()
	base := acceptance.NewPin("task-1", "abc123")
	if base.PolicyID != "P1" || base.PolicyVersion != 1 || base.TaskID != "task-1" || base.TaskRevision != 1 {
		t.Errorf("NewPin identity = %+v", base)
	}
	if base.PolicyDigest != v2contract.Digest(acceptance.P1()) {
		t.Errorf("PolicyDigest = %q, want the digest of P1", base.PolicyDigest)
	}
	if base.Contract != "sha256:abc123" {
		t.Errorf("Contract = %q, want sha256:abc123", base.Contract)
	}
	if base.CheckDefinition != "" || base.EvaluatorDigest != "" || base.EvaluatorName != "" || base.CheckTimeouts != nil {
		t.Errorf("a pin without WithChecks carries check fields: %+v", base)
	}
	timeouts := map[string]string{"unit": "5m"}
	pinned := base.WithChecks("cfg1", "host", "eval1", timeouts)
	if pinned.CheckDefinition != "sha256:cfg1" || pinned.EvaluatorDigest != "sha256:eval1" || pinned.EvaluatorName != "host" || pinned.Contract != "sha256:abc123" {
		t.Errorf("WithChecks = %+v", pinned)
	}
	if pinned.CheckTimeouts["unit"] != "5m" || len(pinned.CheckTimeouts) != 1 {
		t.Errorf("CheckTimeouts = %v", pinned.CheckTimeouts)
	}
	if base.CheckDefinition != "" || base.EvaluatorDigest != "" {
		t.Errorf("WithChecks changed its receiver: %+v", base)
	}
}

func TestP1DigestPinned(t *testing.T) {
	t.Parallel()
	const want = "sha256:46a35275a51a46891eb37bffc04c43901f23ba882eb33b451c39f338c5e0656b"
	if got := v2contract.Digest(acceptance.P1()); got != want {
		t.Errorf("Digest(P1()) = %q, want %q: P1 is immutable, a change is a new PolicyVersion", got, want)
	}
	if err := acceptance.P1().Validate(); err != nil {
		t.Errorf("P1 does not validate: %v", err)
	}
}

func TestDefaultTimeoutsValues(t *testing.T) {
	t.Parallel()
	want := acceptance.Timeouts{Bind: 10 * time.Second, Record: 30 * time.Second, Render: 30 * time.Second, Gate: 30 * time.Second}
	if got := acceptance.DefaultTimeouts(); got != want {
		t.Errorf("DefaultTimeouts() = %+v, want %+v", got, want)
	}
}

func TestResultLabelVerbatim(t *testing.T) {
	t.Parallel()
	tests := []struct {
		result         acceptance.Result
		label          string
		wantUnverified bool
	}{
		{acceptance.ResultPassed, "passed", false},
		{acceptance.ResultFailed, "failed", false},
		{acceptance.ResultNotRun, "NOT RUN", true},
		{acceptance.ResultUnavailable, "NOT RUN", true},
		{"NOT RUN", "NOT RUN", false},
		{acceptance.ResultInvalid, "INVALID", false},
		{"verified", "unknown", false},
		{"", "unknown", false},
	}
	for _, tc := range tests {
		t.Run(string(tc.result), func(t *testing.T) {
			t.Parallel()
			if got := tc.result.Label(); got != tc.label {
				t.Errorf("Result(%q).Label() = %q, want %q", tc.result, got, tc.label)
			}
			if got := tc.result.Unverified(); got != tc.wantUnverified {
				t.Errorf("Result(%q).Unverified() = %v, want %v", tc.result, got, tc.wantUnverified)
			}
		})
	}
}
