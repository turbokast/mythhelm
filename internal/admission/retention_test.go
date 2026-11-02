package admission

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	ab "github.com/turbokast/mythhelm/internal/admissionbinding"
	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

var retentionNow = time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)

// sha256 of the canonical JSON string "Synthetic <&> task", computed
// independently with sha256sum.
const titleValueSHA256 = "e7f5a420d102e99ebe9c8ff16ed9796034167e082d4781768b6250f4f601527e"

// retentionPaths is the independently written retained-path inventory of the
// synthetic binding (decimal indices, byte order).
var retentionPaths = []string{
	"Challenges.0.Text", "Challenges.1.Text", "Challenges.2.Text", "Challenges.3.Text",
	"Decision.GitEmail", "Decision.GitName",
	"Decision.NativeAuth.ConfigIdentity", "Decision.NativeAuth.IdentityRef",
	"Decision.Probe.Executable",
	"Decision.ProjectConfig.AllowedTools.0", "Decision.ProjectConfig.AllowedTools.1",
	"Decision.ProjectConfig.Checks.0.Argv.0", "Decision.ProjectConfig.Checks.0.Argv.1", "Decision.ProjectConfig.Checks.0.Argv.2",
	"Decision.ProjectConfig.Checks.1.Argv.0", "Decision.ProjectConfig.Checks.1.Argv.1", "Decision.ProjectConfig.Checks.1.Argv.2",
	"Decision.ProjectConfig.Passthrough.0", "Decision.ProjectConfig.Passthrough.1",
	"Decision.Proposal.Args.0", "Decision.Proposal.Args.1", "Decision.Proposal.Args.2",
	"Decision.Proposal.Dir",
	"Decision.Proposal.Manifest.0.Source", "Decision.Proposal.Manifest.1.Source",
	"Decision.Proposal.Overrides.0.Kind", "Decision.Proposal.Overrides.0.Name", "Decision.Proposal.Overrides.0.Reason", "Decision.Proposal.Overrides.0.Value",
	"Decision.Proposal.Path",
	"Decision.RepoIdentity", "Decision.RunDir",
	"Decision.Snapshot.SourceRepo",
	"Decision.Task.ArtifactRef", "Decision.Task.Title",
	"Decision.Workdir",
	"Request.Repository", "Request.TaskFile",
}

// syntheticBinding loads the single independent Task23 synthetic golden
// rather than duplicating it; no real account or credential is involved.
func syntheticBinding(t *testing.T) ab.BindingV1 {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "../admissionbinding/codec_test.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			if vs.Names[0].Name != "golden" {
				continue
			}
			raw, err := strconv.Unquote(vs.Values[0].(*ast.BasicLit).Value)
			if err != nil {
				t.Fatal(err)
			}
			var b ab.BindingV1
			if err := json.Unmarshal([]byte(raw), &b); err != nil {
				t.Fatal(err)
			}
			return b
		}
	}
	t.Fatal("synthetic golden not found")
	return ab.BindingV1{}
}

type retentionCase struct {
	binding  ab.BindingV1
	policy   RetentionPolicyV1
	approval RetentionApprovalV1
}

// newRetentionCase builds an exact synthetic approval in dependency order:
// policy content, then the binding pinned to its revision, then the field set.
func newRetentionCase(t *testing.T) retentionCase {
	t.Helper()
	b := syntheticBinding(t)
	b.Challenges = append(b.Challenges, ab.ChallengeV1{ID: "challenge_4", Kind: ab.RetentionChallengeKind, Text: "Retain local binding for root_1 (local_run_binding): git identity, task title, paths, argv, tools, manifest, overrides"})
	content := ab.RetentionPolicyContentV1{SchemaVersion: 1, RootID: "root_1", Purpose: ab.RetentionPurposeV1, ExecutionPolicyRevision: "sha256:" + strings.Repeat("c", 64)}
	for _, p := range retentionPaths {
		content.Rules = append(content.Rules, ab.RetentionRuleV1{FieldPath: p, Classification: ab.RetentionNonSecret, Provenance: ab.RetentionOperatorInput})
	}
	c := retentionCase{binding: b}
	c.approval = RetentionApprovalV1{SchemaVersion: 1, PreparationID: "prep_1", ChallengeID: "challenge_4", PeerID: "peer_1", RootID: "root_1", ContextEpoch: "epoch_1", Generation: 1, ExpiresAt: "2026-01-01T00:05:00Z", ObservedAt: "2026-01-01T00:00:30Z", Decision: ab.RetentionApprove}
	c.setPolicy(t, content)
	c.reseal(t)
	return c
}

// setPolicy installs content under its verified revision and repins the binding.
func (c *retentionCase) setPolicy(t *testing.T, content ab.RetentionPolicyContentV1) {
	t.Helper()
	sum, err := ab.RetentionPolicyDigestV1(content)
	if err != nil {
		t.Fatal(err)
	}
	c.policy = RetentionPolicyV1{Revision: "sha256:" + hex.EncodeToString(sum[:]), Content: content}
	c.binding.PolicyRevision = c.policy.Revision
	c.binding.Decision.Environment.PolicyRevision = c.policy.Revision
	c.approval.PolicyRevision = c.policy.Revision
}

// reseal recomputes the approval's binding and field-set digests so a refusal
// is caused by the mutated pin, not by a stale digest. Unclassified rows have
// no committable field set, so that digest is left unchanged.
func (c *retentionCase) reseal(t *testing.T) {
	t.Helper()
	sum, err := ab.DigestV1(c.binding)
	if err != nil {
		t.Fatal(err)
	}
	c.approval.BindingSHA256 = hex.EncodeToString(sum[:])
	rows, err := ab.DeriveRetentionFieldsV1(c.binding, c.policy)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(rows, func(r ab.RetentionFieldV1) bool { return r.Classification == "" }) {
		return
	}
	set, err := ab.RetentionFieldSetSHA256(rows)
	if err != nil {
		t.Fatal(err)
	}
	c.approval.FieldSetSHA256 = hex.EncodeToString(set[:])
}

func (c retentionCase) check() error {
	a := c.approval
	return CheckRetentionV1(c.binding, c.policy, &a, retentionNow)
}

func (c *retentionCase) setRule(t *testing.T, path string, class ab.RetentionClassV1, prov ab.RetentionProvenanceV1) {
	t.Helper()
	content := c.policy.Content
	content.Rules = slices.Clone(content.Rules)
	i := slices.IndexFunc(content.Rules, func(r ab.RetentionRuleV1) bool { return r.FieldPath == path })
	if i < 0 {
		t.Fatalf("no rule %s", path)
	}
	content.Rules[i].Classification, content.Rules[i].Provenance = class, prov
	c.setPolicy(t, content)
	c.reseal(t)
}

func (c *retentionCase) dropRule(t *testing.T, path string) {
	t.Helper()
	content := c.policy.Content
	content.Rules = slices.DeleteFunc(slices.Clone(content.Rules), func(r ab.RetentionRuleV1) bool { return r.FieldPath == path })
	if len(content.Rules) != len(c.policy.Content.Rules)-1 {
		t.Fatalf("no rule %s", path)
	}
	c.setPolicy(t, content)
	c.reseal(t)
}

func wantRetentionCode(t *testing.T, err error, code v2contract.Code) {
	t.Helper()
	var ce *v2contract.ControlError
	if !errors.As(err, &ce) {
		t.Fatalf("error %v is not a ControlError", err)
	}
	if ce.Code != code {
		t.Fatalf("code %s, want %s", ce.Code, code)
	}
	want := map[string]string{"mythhelm.admission_binding/reason": string(code)}
	if code == v2contract.CodePolicyIneligible {
		want = map[string]string{"mythhelm.retention/reason": "policy_ineligible"}
	}
	if !maps.Equal(ce.Detail, want) {
		t.Fatalf("detail %v, want %v", ce.Detail, want)
	}
	if err := ce.Validate(); err != nil {
		t.Fatalf("control error invalid: %v", err)
	}
	if ce.Disposition != code.DefaultDisposition() {
		t.Fatalf("disposition %s", ce.Disposition)
	}
	if errors.Is(err, ab.ErrInvalidContract) || errors.Is(err, ab.ErrCapabilityUnsupported) || strings.Contains(err.Error(), "admissionbinding") {
		t.Fatalf("leaf sentinel or wrapped text leaked: %q", err.Error())
	}
}

func TestRetentionUnclassifiedRefuses(t *testing.T) { // I02/I20 (v2 §§6/8)
	t.Parallel()
	c := newRetentionCase(t)
	rows, err := ab.DeriveRetentionFieldsV1(c.binding, c.policy)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, r := range rows {
		paths = append(paths, r.FieldPath)
	}
	if !slices.Equal(paths, retentionPaths) {
		t.Fatalf("derived paths %v", paths)
	}
	if err := c.check(); err != nil {
		t.Fatalf("exact synthetic approval: %v", err)
	}
	classes := map[string]string{
		"git email": "Decision.GitEmail", "git name": "Decision.GitName",
		"task title": "Decision.Task.Title", "task artifact": "Decision.Task.ArtifactRef",
		"challenge": "Challenges.3.Text", "repository path": "Request.Repository",
		"config path": "Decision.Probe.Executable", "check argv": "Decision.ProjectConfig.Checks.0.Argv.1",
		"proposal argv": "Decision.Proposal.Args.0", "tools": "Decision.ProjectConfig.AllowedTools.1",
		"manifest": "Decision.Proposal.Manifest.0.Source", "override": "Decision.Proposal.Overrides.0.Value",
	}
	for name, path := range classes {
		t.Run("unclassified "+name, func(t *testing.T) {
			t.Parallel()
			c := newRetentionCase(t)
			c.dropRule(t, path)
			wantRetentionCode(t, c.check(), v2contract.CodePolicyIneligible)
		})
		t.Run("forbidden "+name, func(t *testing.T) {
			t.Parallel()
			c := newRetentionCase(t)
			c.setRule(t, path, ab.RetentionForbidden, ab.RetentionOperatorInput)
			wantRetentionCode(t, c.check(), v2contract.CodePolicyIneligible)
		})
	}
	mismatches := map[string]func(*testing.T, *retentionCase){
		"extra rule": func(t *testing.T, c *retentionCase) {
			content := c.policy.Content
			content.Rules = append(slices.Clone(content.Rules), ab.RetentionRuleV1{FieldPath: "Request.Revision", Classification: ab.RetentionNonSecret, Provenance: ab.RetentionOperatorInput})
			slices.SortFunc(content.Rules, func(x, y ab.RetentionRuleV1) int { return strings.Compare(x.FieldPath, y.FieldPath) })
			c.setPolicy(t, content)
			c.reseal(t)
		},
		"approval policy revision": func(_ *testing.T, c *retentionCase) { c.approval.PolicyRevision = "sha256:" + strings.Repeat("d", 64) },
		"policy root": func(t *testing.T, c *retentionCase) {
			content := c.policy.Content
			content.RootID = "root_2"
			c.setPolicy(t, content)
			c.reseal(t)
		},
		"binding pinned elsewhere": func(t *testing.T, c *retentionCase) {
			c.binding.PolicyRevision = "sha256:" + strings.Repeat("d", 64)
			c.binding.Decision.Environment.PolicyRevision = c.binding.PolicyRevision
			c.reseal(t)
		},
		"approval root":    func(_ *testing.T, c *retentionCase) { c.approval.RootID = "root_2" },
		"peer":             func(_ *testing.T, c *retentionCase) { c.approval.PeerID = "peer_2" },
		"context":          func(_ *testing.T, c *retentionCase) { c.approval.ContextEpoch = "epoch_2" },
		"generation":       func(_ *testing.T, c *retentionCase) { c.approval.Generation = 2 },
		"preparation":      func(_ *testing.T, c *retentionCase) { c.approval.PreparationID = "prep_2" },
		"challenge":        func(_ *testing.T, c *retentionCase) { c.approval.ChallengeID = "challenge_1" },
		"binding digest":   func(_ *testing.T, c *retentionCase) { c.approval.BindingSHA256 = strings.Repeat("e", 64) },
		"field-set digest": func(_ *testing.T, c *retentionCase) { c.approval.FieldSetSHA256 = strings.Repeat("e", 64) },
		"expiry pin":       func(_ *testing.T, c *retentionCase) { c.approval.ExpiresAt = "2026-01-01T00:04:00Z" },
		"observed later":   func(_ *testing.T, c *retentionCase) { c.approval.ObservedAt = "2026-01-01T00:02:00Z" },
		"rejected":         func(_ *testing.T, c *retentionCase) { c.approval.Decision = ab.RetentionReject },
		"no retention challenge": func(t *testing.T, c *retentionCase) {
			c.binding.Challenges[3].Kind = "project-config"
			c.reseal(t)
		},
		"two retention challenges": func(t *testing.T, c *retentionCase) {
			c.binding.Challenges[2].Kind = ab.RetentionChallengeKind
			c.reseal(t)
		},
	}
	for name, mutate := range mismatches {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newRetentionCase(t)
			mutate(t, &c)
			wantRetentionCode(t, c.check(), v2contract.CodePolicyIneligible)
		})
	}
	t.Run("expired", func(t *testing.T) {
		t.Parallel()
		c := newRetentionCase(t)
		a := c.approval
		wantRetentionCode(t, CheckRetentionV1(c.binding, c.policy, &a, time.Date(2026, 1, 1, 0, 5, 0, 0, time.UTC)), v2contract.CodePolicyIneligible)
	})
	t.Run("missing approval", func(t *testing.T) {
		t.Parallel()
		c := newRetentionCase(t)
		wantRetentionCode(t, CheckRetentionV1(c.binding, c.policy, nil, retentionNow), v2contract.CodePolicyIneligible)
	})
	t.Run("unknown classification", func(t *testing.T) {
		t.Parallel()
		c := newRetentionCase(t)
		c.policy.Content.Rules = slices.Clone(c.policy.Content.Rules)
		c.policy.Content.Rules[0].Classification = "public"
		wantRetentionCode(t, c.check(), v2contract.CodeInvalidContract)
	})
}

func TestRetentionCannotApproveNativeSecrets(t *testing.T) { // I19 (v2 §8: no copied subscription secrets)
	t.Parallel()
	for _, prov := range []ab.RetentionProvenanceV1{ab.RetentionNativeAuth, ab.RetentionTranscript, ab.RetentionCredentialEnv} {
		for _, path := range []string{"Decision.NativeAuth.IdentityRef", "Decision.Proposal.Args.1", "Decision.Proposal.Overrides.0.Value"} {
			t.Run(string(prov)+" "+path, func(t *testing.T) {
				t.Parallel()
				c := newRetentionCase(t)
				c.setRule(t, path, ab.RetentionNonSecret, prov)
				// The approval is affirmative and freshly sealed over the
				// exact rows, so only the provenance refusal can reject it.
				if c.approval.Decision != ab.RetentionApprove || c.approval.FieldSetSHA256 == "" {
					t.Fatal("approval is not an affirmative sealed record")
				}
				wantRetentionCode(t, c.check(), v2contract.CodePolicyIneligible)
			})
		}
	}
	for _, prov := range []ab.RetentionProvenanceV1{ab.RetentionOperatorInput, ab.RetentionServiceConfig, ab.RetentionNativeStatus} {
		t.Run("permitted "+string(prov), func(t *testing.T) {
			t.Parallel()
			c := newRetentionCase(t)
			c.setRule(t, "Decision.NativeAuth.IdentityRef", ab.RetentionNonSecret, prov)
			if err := c.check(); err != nil {
				t.Fatalf("non-secret provenance refused: %v", err)
			}
		})
	}
}

func TestRetentionLeafAndAdmissionErrorMapping(t *testing.T) {
	t.Parallel()
	validate := func(c retentionCase) error { return c.policy.Validate() }
	approval := func(c retentionCase) error { return c.approval.Validate() }
	binding := func(c retentionCase) error { return ab.ValidateV1(c.binding) }
	cases := []struct {
		name   string
		mutate func(*retentionCase)
		leaf   func(retentionCase) error
		want   v2contract.Code
	}{
		{"malformed policy revision", func(c *retentionCase) { c.policy.Revision = "sha256:" + strings.Repeat("A", 64) }, validate, v2contract.CodeInvalidContract},
		{"future policy schema", func(c *retentionCase) { c.policy.Content.SchemaVersion = 2 }, validate, v2contract.CodeCapabilityUnsupported},
		{"oversize policy rules", func(c *retentionCase) {
			for i := range 257 {
				c.policy.Content.Rules = append(c.policy.Content.Rules, ab.RetentionRuleV1{FieldPath: fmt.Sprintf("Z.%d", 100+i), Classification: ab.RetentionNonSecret, Provenance: ab.RetentionOperatorInput})
			}
		}, validate, v2contract.CodeCapabilityUnsupported},
		{"future approval schema", func(c *retentionCase) { c.approval.SchemaVersion = 2 }, approval, v2contract.CodeCapabilityUnsupported},
		{"malformed approval hash", func(c *retentionCase) { c.approval.BindingSHA256 = "sha256:" + c.approval.BindingSHA256 }, approval, v2contract.CodeInvalidContract},
		{"oversize approval string", func(c *retentionCase) { c.approval.PeerID = strings.Repeat("secretpeer", 410) }, approval, v2contract.CodeCapabilityUnsupported},
		{"malformed binding", func(c *retentionCase) { c.binding.RunID = "different" }, binding, v2contract.CodeInvalidContract},
		{"future binding schema", func(c *retentionCase) { c.binding.SchemaVersion = 2 }, binding, v2contract.CodeCapabilityUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := newRetentionCase(t)
			tc.mutate(&c)
			leafWant, other := ab.ErrInvalidContract, ab.ErrCapabilityUnsupported
			if tc.want == v2contract.CodeCapabilityUnsupported {
				leafWant, other = other, leafWant
			}
			err := tc.leaf(c)
			for range 3 {
				if !errors.Is(err, leafWant) || errors.Is(err, other) {
					t.Fatalf("leaf error %v, want only %v", err, leafWant)
				}
				err = fmt.Errorf("caller: %w", err)
			}
			got := c.check()
			wantRetentionCode(t, got, tc.want)
			if strings.Contains(got.Error(), "secretpeer") {
				t.Fatalf("input text leaked: %q", got.Error())
			}
		})
	}
	t.Run("policy refusals stay policy_ineligible", func(t *testing.T) {
		t.Parallel()
		forbidden := newRetentionCase(t)
		forbidden.setRule(t, "Decision.GitEmail", ab.RetentionForbidden, ab.RetentionOperatorInput)
		unclassified := newRetentionCase(t)
		unclassified.dropRule(t, "Decision.GitEmail")
		missing := newRetentionCase(t)
		for _, err := range []error{forbidden.check(), unclassified.check(), CheckRetentionV1(missing.binding, missing.policy, nil, retentionNow)} {
			wantRetentionCode(t, err, v2contract.CodePolicyIneligible)
		}
	})
}

// The shared mapper classifies by errors.Is identity only: a fresh error that
// merely repeats a sentinel's text, or any unexpected error, is invalid_contract.
func TestRetentionLeafErrorMapperUsesIdentityNotText(t *testing.T) { // I02 (v2 §6)
	t.Parallel()
	cases := []struct {
		name   string
		err    error
		code   v2contract.Code
		reason string
	}{
		{"wrapped invalid sentinel", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", ab.ErrInvalidContract)), v2contract.CodeInvalidContract, "invalid_contract"},
		{"wrapped unsupported sentinel", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", ab.ErrCapabilityUnsupported)), v2contract.CodeCapabilityUnsupported, "capability_unsupported"},
		{"fresh error with unsupported text", errors.New(ab.ErrCapabilityUnsupported.Error()), v2contract.CodeInvalidContract, "invalid_contract"},
		{"fresh error prefixed by unsupported text", errors.New(ab.ErrCapabilityUnsupported.Error() + ": secretpeer"), v2contract.CodeInvalidContract, "invalid_contract"},
		{"unexpected error", errors.New("secretpeer unexpected"), v2contract.CodeInvalidContract, "invalid_contract"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := leafRetentionError(tc.err)
			var ce *v2contract.ControlError
			if !errors.As(got, &ce) {
				t.Fatalf("mapped %T %v, want *v2contract.ControlError", got, got)
			}
			if ce.Code != tc.code {
				t.Fatalf("code %s, want %s", ce.Code, tc.code)
			}
			if want := map[string]string{"mythhelm.admission_binding/reason": tc.reason}; !maps.Equal(ce.Detail, want) {
				t.Fatalf("detail %v, want %v", ce.Detail, want)
			}
			if err := ce.Validate(); err != nil {
				t.Fatal(err)
			}
			if errors.Is(got, ab.ErrInvalidContract) || errors.Is(got, ab.ErrCapabilityUnsupported) {
				t.Fatalf("mapped error still wraps the leaf: %v", got)
			}
			if strings.Contains(got.Error(), "secretpeer") || strings.Contains(got.Error(), "inner") {
				t.Fatalf("input text leaked: %q", got.Error())
			}
		})
	}
}

func TestRetentionExactValueBinding(t *testing.T) { // I02/I20 (v2 §§6/8)
	t.Parallel()
	changes := map[string]func(*ab.BindingV1){
		"git email":      func(b *ab.BindingV1) { b.Decision.GitEmail = "other@example.com" },
		"redacted title": func(b *ab.BindingV1) { b.Decision.Task.Title = "Synthetic [redacted] task" },
		"proposal argv":  func(b *ab.BindingV1) { b.Decision.Proposal.Args[1] = "third" },
		"check argv":     func(b *ab.BindingV1) { b.Decision.ProjectConfig.Checks[0].Argv[2] = "./internal/..." },
		"override value": func(b *ab.BindingV1) { b.Decision.Proposal.Overrides[0].Value = "no" },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newRetentionCase(t)
			change(&c.binding)
			wantRetentionCode(t, c.check(), v2contract.CodePolicyIneligible)
			// A copied field-set digest under a recomputed binding digest
			// still refuses: the changed value is never redacted away.
			fieldSet := c.approval.FieldSetSHA256
			c.reseal(t)
			if c.approval.FieldSetSHA256 == fieldSet {
				t.Fatal("changed value left the field set unchanged")
			}
			c.approval.FieldSetSHA256 = fieldSet
			wantRetentionCode(t, c.check(), v2contract.CodePolicyIneligible)
		})
	}
	t.Run("title row is deterministic and version-bound", func(t *testing.T) {
		t.Parallel()
		c := newRetentionCase(t)
		first, err := ab.DeriveRetentionFieldsV1(c.binding, c.policy)
		if err != nil {
			t.Fatal(err)
		}
		second, err := ab.DeriveRetentionFieldsV1(c.binding, c.policy)
		if err != nil || !reflect.DeepEqual(first, second) {
			t.Fatalf("field rows are not deterministic: %v", err)
		}
		i := slices.IndexFunc(first, func(r ab.RetentionFieldV1) bool { return r.FieldPath == "Decision.Task.Title" })
		if i < 0 || first[i].ValueSHA256 != titleValueSHA256 {
			t.Fatalf("title row %+v", first)
		}
	})
}

func TestRetentionRecordsGrantNoAuthority(t *testing.T) { // I03 (v2 §6: records are not authority)
	t.Parallel()
	c := newRetentionCase(t)
	before := newRetentionCase(t)
	if err := c.check(); err != nil {
		t.Fatalf("structurally valid synthetic approval: %v", err)
	}
	if !reflect.DeepEqual(c, before) {
		t.Fatal("check mutated its inputs")
	}
	// The checker's imports are pure; an os, exec, journal or service import
	// would let it write or launch.
	f, err := parser.ParseFile(token.NewFileSet(), "retention.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	var imports []string
	for _, im := range f.Imports {
		p, err := strconv.Unquote(im.Path.Value)
		if err != nil {
			t.Fatal(err)
		}
		imports = append(imports, p)
	}
	want := []string{"encoding/hex", "errors", "time", "github.com/turbokast/mythhelm/internal/admissionbinding", "github.com/turbokast/mythhelm/internal/v2contract"}
	if !slices.Equal(imports, want) {
		t.Fatalf("retention.go imports %v, want %v", imports, want)
	}
	policyRaw, err := ab.EncodeRetentionPolicyV1(c.policy)
	if err != nil {
		t.Fatal(err)
	}
	approvalRaw, err := ab.EncodeRetentionApprovalV1(c.approval)
	if err != nil {
		t.Fatal(err)
	}
	spec := control.RunSpec{SchemaVersion: 2, Repository: "/tmp/repo", TaskFile: "/tmp/task.md", TaskSHA256: strings.Repeat("a", 64), Adapter: "fake", Billing: "api"}
	conf := control.RunConfirmation{SchemaVersion: 2, PreparationID: "prep_1", ChallengeID: "challenge_4", BindingSHA256: strings.Repeat("a", 64), Approved: true}
	if _, err := control.DecodeParams[control.RunSpec](withFields(t, spec, nil)); err != nil {
		t.Fatalf("control RunSpec refused: %v", err)
	}
	if _, err := control.DecodeParams[control.RunConfirmation](withFields(t, conf, nil)); err != nil {
		t.Fatalf("control RunConfirmation refused: %v", err)
	}
	for name, extra := range map[string]map[string]json.RawMessage{
		"policy":    {"retention_policy": policyRaw},
		"approval":  {"retention_approval": approvalRaw},
		"field set": {"field_set_sha256": json.RawMessage(strconv.Quote(c.approval.FieldSetSHA256))},
	} {
		if _, err := control.DecodeParams[control.RunSpec](withFields(t, spec, extra)); err == nil {
			t.Fatalf("RunSpec accepted an extra %s wire field", name)
		}
		if _, err := control.DecodeParams[control.RunConfirmation](withFields(t, conf, extra)); err == nil {
			t.Fatalf("RunConfirmation accepted an extra %s wire field", name)
		}
	}
}

func withFields(t *testing.T, v any, extra map[string]json.RawMessage) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	maps.Copy(m, extra)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRetentionFieldSetIsInternalBindingCommitment(t *testing.T) { // I02/I20 (v2 §§6/8)
	t.Parallel()
	refusals := map[string]struct {
		mutate func(*testing.T, *retentionCase)
		want   v2contract.Code
	}{
		"tampered policy content": {func(_ *testing.T, c *retentionCase) {
			c.policy.Content.Rules = slices.Clone(c.policy.Content.Rules)
			c.policy.Content.Rules[0].Provenance = ab.RetentionServiceConfig
		}, v2contract.CodeInvalidContract},
		"missing policy content": {func(_ *testing.T, c *retentionCase) { c.policy.Content = ab.RetentionPolicyContentV1{} }, v2contract.CodeInvalidContract},
		"hash-only consent": {func(_ *testing.T, c *retentionCase) {
			c.policy = RetentionPolicyV1{Revision: c.policy.Revision}
		}, v2contract.CodeInvalidContract},
		"rule provenance change": {func(t *testing.T, c *retentionCase) {
			fieldSet := c.approval.FieldSetSHA256
			c.setRule(t, "Decision.GitName", ab.RetentionNonSecret, ab.RetentionServiceConfig)
			c.approval.FieldSetSHA256 = fieldSet
		}, v2contract.CodePolicyIneligible},
		"scope root change": {func(t *testing.T, c *retentionCase) {
			fieldSet := c.approval.FieldSetSHA256
			content := c.policy.Content
			content.RootID = "root_2"
			c.binding.RootID, c.binding.Decision.StateRootID, c.approval.RootID = "root_2", "root_2", "root_2"
			c.setPolicy(t, content)
			c.reseal(t)
			if c.approval.FieldSetSHA256 != fieldSet {
				t.Fatal("root is not a field-set row; expected an unchanged field set")
			}
			c.approval.PolicyRevision = "sha256:" + strings.Repeat("d", 64)
		}, v2contract.CodePolicyIneligible},
		"challenge text change": {func(t *testing.T, c *retentionCase) {
			fieldSet := c.approval.FieldSetSHA256
			c.binding.Challenges[3].Text = "Retain everything"
			c.reseal(t)
			c.approval.FieldSetSHA256 = fieldSet
		}, v2contract.CodePolicyIneligible},
		"copied approval": {func(t *testing.T, c *retentionCase) {
			other := newRetentionCase(t)
			other.binding.PreparationID = "prep_2"
			c.binding = other.binding
		}, v2contract.CodePolicyIneligible},
	}
	for name, tc := range refusals {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := newRetentionCase(t)
			tc.mutate(t, &c)
			wantRetentionCode(t, c.check(), tc.want)
		})
	}
	t.Run("policy then binding then field set", func(t *testing.T) {
		t.Parallel()
		c := newRetentionCase(t)
		policySum, err := ab.RetentionPolicyDigestV1(c.policy.Content)
		if err != nil {
			t.Fatal(err)
		}
		if c.binding.PolicyRevision != "sha256:"+hex.EncodeToString(policySum[:]) {
			t.Fatal("binding does not pin the policy content address")
		}
		bindingSum, err := ab.DigestV1(c.binding)
		if err != nil {
			t.Fatal(err)
		}
		if c.approval.BindingSHA256 != hex.EncodeToString(bindingSum[:]) {
			t.Fatal("approval does not commit the binding digest")
		}
		for _, f := range reflect.VisibleFields(reflect.TypeFor[ab.RetentionPolicyContentV1]()) {
			if strings.Contains(f.Name, "SHA256") || f.Name == "Challenges" {
				t.Fatalf("policy content carries later commitment %s", f.Name)
			}
		}
		for _, f := range reflect.VisibleFields(reflect.TypeFor[ab.BindingV1]()) {
			if strings.Contains(f.Name, "FieldSet") || strings.Contains(f.Name, "Approval") {
				t.Fatalf("binding carries later commitment %s", f.Name)
			}
		}
	})
}

func TestRetentionUnclassifiedOverrideKindAndReasonRefused(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"Decision.Proposal.Overrides.0.Kind", "Decision.Proposal.Overrides.0.Reason"} {
		c := newRetentionCase(t)
		c.dropRule(t, path)
		var ce *v2contract.ControlError
		if err := c.check(); !errors.As(err, &ce) || ce.Code != v2contract.CodePolicyIneligible {
			t.Fatalf("%s unclassified approved: %v", path, err)
		}
	}
}

func TestRetentionFractionalExpiryValid(t *testing.T) {
	t.Parallel()
	c := newRetentionCase(t)
	c.binding.ExpiresAt = "2026-01-01T00:01:00.1Z"
	c.approval.ExpiresAt = c.binding.ExpiresAt
	c.approval.ObservedAt = "2026-01-01T00:01:00Z"
	c.reseal(t)
	if err := c.check(); err != nil {
		t.Fatalf("chronologically valid observed<=now<expiry refused: %v", err)
	}
}

func TestRetentionPolicyUnicodeSeparatorsLiteral(t *testing.T) {
	t.Parallel()
	c := newRetentionCase(t)
	content := c.policy.Content
	content.RootID = "root_\u2028_\u2029"
	c.setPolicy(t, content)
	raw, err := ab.EncodeRetentionPolicyV1(c.policy)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(`\u2028`)) || bytes.Contains(raw, []byte(`\u2029`)) || !bytes.Contains(raw, []byte("root_\u2028_\u2029")) {
		t.Fatalf("policy canonical JSON does not carry literal separators: %q", raw)
	}
}

func TestRetentionPolicyByteBound(t *testing.T) {
	t.Parallel()
	c := newRetentionCase(t)
	content := c.policy.Content
	content.Rules = nil
	for i := 0; i < 256; i++ {
		content.Rules = append(content.Rules, ab.RetentionRuleV1{FieldPath: "A" + strings.Repeat("x", 4092) + fmt.Sprintf("%03d", i), Classification: ab.RetentionNonSecret, Provenance: ab.RetentionOperatorInput})
	}
	if _, err := ab.RetentionPolicyDigestV1(content); !errors.Is(err, ab.ErrCapabilityUnsupported) {
		t.Fatalf("oversize policy digest: %v", err)
	}
	p := ab.RetentionPolicyV1{Revision: "sha256:" + strings.Repeat("d", 64), Content: content}
	if raw, err := ab.EncodeRetentionPolicyV1(p); !errors.Is(err, ab.ErrCapabilityUnsupported) || raw != nil {
		t.Fatalf("oversize policy produced usable %d bytes: %v", len(raw), err)
	}
}
