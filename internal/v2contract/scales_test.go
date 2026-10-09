package v2contract_test

import (
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/qualify"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

func TestScalesMatchMasterWords(t *testing.T) {
	t.Parallel()
	capability := []v2contract.Capability{v2contract.CapabilitySupported, v2contract.CapabilityUnsupported, v2contract.CapabilityUnknown}
	for i, w := range []string{"supported", "unsupported", "unknown"} {
		if string(capability[i]) != w || !capability[i].Valid() {
			t.Errorf("capability[%d] = %q valid=%v, want %q valid", i, capability[i], capability[i].Valid(), w)
		}
	}
	progress := []v2contract.Progress{
		v2contract.ProgressPlanned, v2contract.ProgressDocumentedCandidate, v2contract.ProgressFixtureTested,
		v2contract.ProgressLiveQualified, v2contract.ProgressExperimental, v2contract.ProgressBlocked,
		v2contract.ProgressUnsupported,
	}
	// D3 pin: v2contract duplicates the words; qualify's constants are the oracle.
	qualified := []qualify.Progress{
		qualify.ProgressPlanned, qualify.ProgressDocumentedCandidate, qualify.ProgressFixtureTested,
		qualify.ProgressLiveQualified, qualify.ProgressExperimental, qualify.ProgressBlocked,
		qualify.ProgressUnsupported,
	}
	for i, w := range []string{"planned", "documented-candidate", "fixture-tested", "live-qualified", "experimental", "blocked", "unsupported"} {
		if string(progress[i]) != w || !progress[i].Valid() {
			t.Errorf("progress[%d] = %q valid=%v, want %q valid", i, progress[i], progress[i].Valid(), w)
		}
		if string(progress[i]) != string(qualified[i]) {
			t.Errorf("progress[%d] = %q, qualify has %q", i, progress[i], qualified[i])
		}
	}
	eligibility := []v2contract.Eligibility{v2contract.EligibilityEligible, v2contract.EligibilityBlocked, v2contract.EligibilityUnsupported}
	for i, w := range []string{"eligible", "blocked", "unsupported"} {
		if string(eligibility[i]) != w || !eligibility[i].Valid() {
			t.Errorf("eligibility[%d] = %q valid=%v, want %q valid", i, eligibility[i], eligibility[i].Valid(), w)
		}
	}
	for _, bad := range []string{"", "Supported", "live_qualified"} {
		if v2contract.Capability(bad).Valid() || v2contract.Progress(bad).Valid() || v2contract.Eligibility(bad).Valid() {
			t.Errorf("out-of-scale %q reported valid", bad)
		}
	}
	// Capability words against qualify's validator (its predicate is unexported,
	// so drive it through CanonicalDigest on a defaulted record carrying the word).
	probe := func(word string) error {
		base, err := qualify.DecodeRecord([]byte(`{"schema_version":2,"progress":"planned"}`))
		if err != nil {
			t.Fatalf("DecodeRecord base: %v", err)
		}
		base.Capabilities = map[string]qualify.Capability{"c": {Value: word}}
		_, err = qualify.CanonicalDigest(base)
		return err
	}
	for _, w := range capability {
		if err := probe(string(w)); err != nil {
			t.Errorf("qualify rejects capability %q: %v", w, err)
		}
	}
	if err := probe("maybe"); err == nil || !strings.Contains(err.Error(), "Capabilities") {
		t.Errorf("qualify accepts out-of-scale capability: err = %v", err)
	}
}

func TestClaimRequiresEvidenceScopeExpiry(t *testing.T) {
	t.Parallel()
	exp := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	zero := time.Time{}
	valid := v2contract.Claim[v2contract.Capability]{Value: v2contract.CapabilitySupported, Evidence: "fixture", Scope: "linux", ExpiresAt: &exp}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid claim: %v", err)
	}
	tests := []struct {
		name    string
		mutate  func(c *v2contract.Claim[v2contract.Capability])
		wantErr string
	}{
		{"empty evidence", func(c *v2contract.Claim[v2contract.Capability]) { c.Evidence = "" }, "evidence"},
		{"empty scope", func(c *v2contract.Claim[v2contract.Capability]) { c.Scope = "" }, "scope"},
		{"out of scale value", func(c *v2contract.Claim[v2contract.Capability]) { c.Value = "maybe" }, `"maybe"`},
		{"nil expiry", func(c *v2contract.Claim[v2contract.Capability]) { c.ExpiresAt = nil }, "expires_at"},
		{"zero expiry", func(c *v2contract.Claim[v2contract.Capability]) { c.ExpiresAt = &zero }, "expires_at"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := valid
			tc.mutate(&c)
			if err := c.Validate(); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
	progress := v2contract.Claim[v2contract.Progress]{Value: "nonsense", Evidence: "e", Scope: "s", ExpiresAt: &exp}
	if err := progress.Validate(); err == nil || !strings.Contains(err.Error(), "nonsense") {
		t.Errorf("Progress claim with out-of-scale value: err = %v", err)
	}
}

func TestRevisionRefValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		ref     v2contract.RevisionRef
		wantErr string
	}{
		{"valid first kind", v2contract.RevisionRef{Kind: "artifact", ID: "art_demo_0001", Revision: 1}, ""}, //nolint:misspell // "artifact" is the wire value of the kind
		{"valid contract with digest", v2contract.RevisionRef{Kind: "contract", ID: "c", Revision: 3, Digest: "sha256:00"}, ""},
		{"kind other", v2contract.RevisionRef{Kind: "other", ID: "a", Revision: 1}, "kind"},
		{"revision zero", v2contract.RevisionRef{Kind: "contract", ID: "a", Revision: 0}, "revision"},
		{"empty id", v2contract.RevisionRef{Kind: "contract", Revision: 1}, "id"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.ref.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestRequestEnvelopeRequiresIdentity(t *testing.T) {
	t.Parallel()
	gen := int64(4)
	tests := []struct {
		name    string
		env     v2contract.RequestEnvelope
		wantErr string
	}{
		{"valid minimal", v2contract.RequestEnvelope{OperationID: "op_demo_0001"}, ""},
		{"valid full", v2contract.RequestEnvelope{OperationID: "op", Object: "run_demo_0001", ExpectedRevision: 7, Generation: &gen}, ""},
		{"empty operation id", v2contract.RequestEnvelope{ExpectedRevision: 1}, "operation_id"},
		{"negative expected revision", v2contract.RequestEnvelope{OperationID: "op", ExpectedRevision: -1}, "expected_revision"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.env.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestFrameLimitsExact(t *testing.T) {
	t.Parallel()
	if v2contract.SchemaVersion != 2 || v2contract.MaxFrameBytes != 1<<20 || v2contract.MaxNestingDepth != 64 || v2contract.MaxArtifactRefs != 128 {
		t.Fatalf("constants = %d %d %d %d", v2contract.SchemaVersion, v2contract.MaxFrameBytes, v2contract.MaxNestingDepth, v2contract.MaxArtifactRefs)
	}
	tests := []struct {
		name            string
		length, d, refs int
		wantErr         string
	}{
		{"all at boundary", 1 << 20, 64, 128, ""},
		{"all zero", 0, 0, 0, ""},
		{"bytes over", 1<<20 + 1, 64, 128, "frame bytes"},
		{"depth over", 1, 65, 0, "nesting depth"},
		{"refs over", 1, 1, 129, "artefact refs"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := v2contract.CheckFrameLimits(tc.length, tc.d, tc.refs)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("CheckFrameLimits = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("CheckFrameLimits error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestRequestEnvelopeDecodeKeepsAbsentGenerationNil(t *testing.T) {
	t.Parallel()
	got, err := v2contract.Decode[v2contract.RequestEnvelope]([]byte(`{"operation_id":"op","expected_revision":0}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Generation != nil {
		t.Errorf("absent generation decoded as %d, want nil (I09)", *got.Generation)
	}
}
