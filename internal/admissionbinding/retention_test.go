package admissionbinding_test

import (
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	ab "github.com/turbokast/mythhelm/internal/admissionbinding"
)

// Synthetic policy content; the digests below were computed independently
// with printf | sha256sum over the domain separator plus these exact bytes.
const (
	policyContentGolden = `{"SchemaVersion":1,"RootID":"root_1","Purpose":"local_run_binding","ExecutionPolicyRevision":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","Rules":[{"FieldPath":"Decision.GitEmail","Classification":"non_secret","Provenance":"operator_input"},{"FieldPath":"Decision.Proposal.Args.0","Classification":"forbidden","Provenance":"credential_environment"}]}`
	policyDigestGolden  = "c913d2f76a5ed151420e8151bf9e283cdfcaecfd6e1c6308f7000616efde8c76"
	policyGolden        = `{"Revision":"sha256:` + policyDigestGolden + `","Content":` + policyContentGolden + `}`
	emailValueGolden    = "be3f0eefe55eb09ca5826ae7d6e4c914491350ed0babca35eac01cd959e3c94b" // sha256 of "\"fixture@example.com\""
	fieldSetGolden      = "218c4624eb9db5968362bf8609fde990037b9eb76e10786ee187f01b50c652ec"
	approvalGolden      = `{"SchemaVersion":1,"PreparationID":"prep_1","ChallengeID":"challenge_4","BindingSHA256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","FieldSetSHA256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","PolicyRevision":"sha256:` + policyDigestGolden + `","PeerID":"peer_1","RootID":"root_1","ContextEpoch":"epoch_1","Generation":1,"ExpiresAt":"2026-01-01T00:05:00Z","ObservedAt":"2026-01-01T00:00:30Z","Decision":"approve"}`
)

func goldenPolicy() ab.RetentionPolicyV1 {
	return ab.RetentionPolicyV1{
		Revision: "sha256:" + policyDigestGolden,
		Content: ab.RetentionPolicyContentV1{SchemaVersion: 1, RootID: "root_1", Purpose: ab.RetentionPurposeV1, ExecutionPolicyRevision: "sha256:" + strings.Repeat("c", 64), Rules: []ab.RetentionRuleV1{
			{FieldPath: "Decision.GitEmail", Classification: ab.RetentionNonSecret, Provenance: ab.RetentionOperatorInput},
			{FieldPath: "Decision.Proposal.Args.0", Classification: ab.RetentionForbidden, Provenance: ab.RetentionCredentialEnv},
		}},
	}
}

func TestRetentionPolicyCanonicalCodec(t *testing.T) { // I02/I20 (v2 §§6/8)
	t.Parallel()
	p := goldenPolicy()
	sum, err := ab.RetentionPolicyDigestV1(p.Content)
	if err != nil || hex.EncodeToString(sum[:]) != policyDigestGolden {
		t.Fatalf("policy digest %x: %v", sum, err)
	}
	raw, err := ab.EncodeRetentionPolicyV1(p)
	if err != nil || string(raw) != policyGolden {
		t.Fatalf("policy bytes %s: %v", raw, err)
	}
	decoded, err := ab.DecodeRetentionPolicyV1([]byte(policyGolden))
	if err != nil || !reflect.DeepEqual(decoded, p) {
		t.Fatalf("policy decode %+v: %v", decoded, err)
	}
	refusals := map[string]string{
		"duplicate key":   strings.Replace(policyGolden, `"Revision"`, `"Revision":"x","Revision"`, 1),
		"unknown key":     strings.Replace(policyGolden, `"Revision"`, `"Extra":"x","Revision"`, 1),
		"missing key":     strings.Replace(policyGolden, `"Purpose":"local_run_binding",`, ``, 1),
		"folded case key": strings.Replace(policyGolden, `"Revision"`, `"revision"`, 1),
		"null rules":      strings.Replace(policyGolden, `"Rules":[`, `"Rules":null,"X":[`, 1),
		"whitespace":      strings.Replace(policyGolden, `"Content":`, `"Content": `, 1),
		"trailing input":  policyGolden + `{}`,
		"tampered rule":   strings.Replace(policyGolden, `"forbidden"`, `"non_secret"`, 1),
		"unsorted rules":  strings.Replace(policyGolden, `"Decision.GitEmail"`, `"Decision.Z"`, 1),
		"bad UTF-8":       strings.Replace(policyGolden, "root_1", "root_\xff", 1),
	}
	for name, input := range refusals {
		got, err := ab.DecodeRetentionPolicyV1([]byte(input))
		if !errors.Is(err, ab.ErrInvalidContract) || errors.Is(err, ab.ErrCapabilityUnsupported) {
			t.Errorf("%s: error %v", name, err)
		}
		if !reflect.DeepEqual(got, ab.RetentionPolicyV1{}) {
			t.Errorf("%s: failed decode is usable", name)
		}
	}
	if _, err := ab.DecodeRetentionPolicyV1(make([]byte, 1<<20+1)); !errors.Is(err, ab.ErrCapabilityUnsupported) {
		t.Fatalf("byte bound: %v", err)
	}
	// A label or the binding domain is never a policy content address.
	for _, revision := range []string{"policy_1", "sha256:" + strings.ToUpper(policyDigestGolden), policyDigestGolden} {
		p := goldenPolicy()
		p.Revision = revision
		if err := p.Validate(); !errors.Is(err, ab.ErrInvalidContract) {
			t.Fatalf("revision %q: %v", revision, err)
		}
	}
	p = goldenPolicy()
	p.Content.Purpose = "anything"
	if _, err := ab.RetentionPolicyDigestV1(p.Content); !errors.Is(err, ab.ErrInvalidContract) {
		t.Fatalf("purpose: %v", err)
	}
}

func TestRetentionApprovalCanonicalCodec(t *testing.T) {
	t.Parallel()
	a, err := ab.DecodeRetentionApprovalV1([]byte(approvalGolden))
	if err != nil || a.Decision != ab.RetentionApprove || a.Generation != 1 {
		t.Fatalf("approval decode %+v: %v", a, err)
	}
	raw, err := ab.EncodeRetentionApprovalV1(a)
	if err != nil || string(raw) != approvalGolden {
		t.Fatalf("approval bytes %s: %v", raw, err)
	}
	for name, tc := range map[string]struct {
		input string
		want  error
	}{
		"wildcard decision":  {strings.Replace(approvalGolden, `"approve"`, `"*"`, 1), ab.ErrInvalidContract},
		"boolean decision":   {strings.Replace(approvalGolden, `"approve"`, `true`, 1), ab.ErrInvalidContract},
		"prefixed field set": {strings.Replace(approvalGolden, `"FieldSetSHA256":"`, `"FieldSetSHA256":"sha256:`, 1), ab.ErrInvalidContract},
		"observed after":     {strings.Replace(approvalGolden, `"ObservedAt":"2026-01-01T00:00:30Z"`, `"ObservedAt":"2026-01-01T00:06:00Z"`, 1), ab.ErrInvalidContract},
		"zero generation":    {strings.Replace(approvalGolden, `"Generation":1`, `"Generation":0`, 1), ab.ErrInvalidContract},
		"float generation":   {strings.Replace(approvalGolden, `"Generation":1`, `"Generation":1.0`, 1), ab.ErrInvalidContract},
		"future schema":      {strings.Replace(approvalGolden, `"SchemaVersion":1`, `"SchemaVersion":2`, 1), ab.ErrCapabilityUnsupported},
	} {
		other := ab.ErrInvalidContract
		if tc.want == other {
			other = ab.ErrCapabilityUnsupported
		}
		got, err := ab.DecodeRetentionApprovalV1([]byte(tc.input))
		if !errors.Is(fmt.Errorf("caller: %w", err), tc.want) || errors.Is(err, other) || got != (ab.RetentionApprovalV1{}) {
			t.Errorf("%s: %+v, %v", name, got, err)
		}
	}
}

func TestRetentionFieldSetCanonicalDigest(t *testing.T) { // I20 (v2 §8)
	t.Parallel()
	b := fixture(t)
	p := goldenPolicy()
	b.PolicyRevision, b.Decision.Environment.PolicyRevision = p.Revision, p.Revision
	rows, err := ab.DeriveRetentionFieldsV1(b, p)
	if err != nil {
		t.Fatal(err)
	}
	var email, unclassified, args0 *ab.RetentionFieldV1
	for i := range rows {
		switch rows[i].FieldPath {
		case "Decision.GitEmail":
			email = &rows[i]
		case "Decision.Task.Title":
			unclassified = &rows[i]
		case "Decision.Proposal.Args.0":
			args0 = &rows[i]
		}
	}
	if email == nil || *email != (ab.RetentionFieldV1{FieldPath: "Decision.GitEmail", ValueSHA256: emailValueGolden, Classification: ab.RetentionNonSecret, Provenance: ab.RetentionOperatorInput}) {
		t.Fatalf("email row %+v", email)
	}
	if args0 == nil || args0.Classification != ab.RetentionForbidden || args0.Provenance != ab.RetentionCredentialEnv {
		t.Fatalf("args row %+v", args0)
	}
	if unclassified == nil || unclassified.Classification != "" || unclassified.Provenance != "" {
		t.Fatalf("unruled path must be reported unclassified: %+v", unclassified)
	}
	if _, err := ab.RetentionFieldSetSHA256(rows); !errors.Is(err, ab.ErrInvalidContract) {
		t.Fatalf("unclassified rows committed: %v", err)
	}
	sum, err := ab.RetentionFieldSetSHA256([]ab.RetentionFieldV1{*email})
	if err != nil || hex.EncodeToString(sum[:]) != fieldSetGolden {
		t.Fatalf("field-set digest %x: %v", sum, err)
	}
	unsorted := []ab.RetentionFieldV1{*args0, *email}
	if _, err := ab.RetentionFieldSetSHA256(unsorted); !errors.Is(err, ab.ErrInvalidContract) {
		t.Fatalf("unsorted rows committed: %v", err)
	}
	if _, err := ab.DeriveRetentionFieldsV1(b, ab.RetentionPolicyV1{Revision: p.Revision}); !errors.Is(err, ab.ErrInvalidContract) {
		t.Fatalf("hash-only policy derived rows: %v", err)
	}
}
