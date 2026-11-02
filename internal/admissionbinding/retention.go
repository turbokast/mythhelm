package admissionbinding

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// RetentionClassV1 classifies one retained binding value.
type RetentionClassV1 string

// RetentionProvenanceV1 records where one retained binding value came from.
type RetentionProvenanceV1 string

// RetentionDecisionV1 records the closed answer to the retention challenge.
type RetentionDecisionV1 string

// Retention classifications, provenances and decisions are closed sets.
const (
	RetentionNonSecret     RetentionClassV1      = "non_secret"
	RetentionForbidden     RetentionClassV1      = "forbidden"
	RetentionOperatorInput RetentionProvenanceV1 = "operator_input"
	RetentionServiceConfig RetentionProvenanceV1 = "service_config"
	RetentionNativeStatus  RetentionProvenanceV1 = "native_status_nonsecret"
	RetentionNativeAuth    RetentionProvenanceV1 = "native_auth_store"
	RetentionTranscript    RetentionProvenanceV1 = "native_transcript"
	RetentionCredentialEnv RetentionProvenanceV1 = "credential_environment"
	RetentionApprove       RetentionDecisionV1   = "approve"
	RetentionReject        RetentionDecisionV1   = "reject"
)

// RetentionPurposeV1 is the only purpose a version-1 policy may serve.
const RetentionPurposeV1 = "local_run_binding"

// RetentionChallengeKind identifies the mandatory retention challenge.
const RetentionChallengeKind = "retain-local-binding"

const (
	retentionPolicyDomain = "MYTHHELM:retention-policy:v1\n"
	retentionFieldsDomain = "MYTHHELM:retention-fields:v1\n"
	maxRetentionRows      = 256
	maxRetentionString    = 4096
)

// RetentionFieldV1 is one derived value row. It is never part of BindingV1 or
// the policy content.
type RetentionFieldV1 struct {
	FieldPath, ValueSHA256 string
	Classification         RetentionClassV1
	Provenance             RetentionProvenanceV1
}

// RetentionRuleV1 classifies one exact binding field path; it holds no value.
type RetentionRuleV1 struct {
	FieldPath      string
	Classification RetentionClassV1
	Provenance     RetentionProvenanceV1
}

// RetentionPolicyContentV1 is the static content-addressed policy artefact.
type RetentionPolicyContentV1 struct {
	SchemaVersion                            int64 // exactly 1
	RootID, Purpose, ExecutionPolicyRevision string
	Rules                                    []RetentionRuleV1
}

// RetentionPolicyV1 pairs policy content with its verified content address.
type RetentionPolicyV1 struct {
	Revision string // sha256:<lowercase digest of Content>
	Content  RetentionPolicyContentV1
}

// RetentionApprovalV1 is a structural record of one retention answer. A
// constructed record is shape evidence, never authority.
type RetentionApprovalV1 struct {
	SchemaVersion                                  int64 // exactly 1
	PreparationID, ChallengeID, BindingSHA256      string
	FieldSetSHA256, PolicyRevision, PeerID, RootID string
	ContextEpoch                                   string
	Generation                                     int64
	ExpiresAt, ObservedAt                          string
	Decision                                       RetentionDecisionV1
}

// retainedPaths lists the finite typed binding fields whose values need an
// explicit retention classification; "*" expands to every decimal index.
// This is a typed-field control, not universal secret recognition.
func retainedPaths() []string {
	return []string{
		"Request.Repository",
		"Request.TaskFile",
		"Decision.RunDir",
		"Decision.Workdir",
		"Decision.GitName",
		"Decision.GitEmail",
		"Decision.ProjectConfig.Passthrough.*",
		"Decision.ProjectConfig.AllowedTools.*",
		"Decision.ProjectConfig.Checks.*.Argv.*",
		"Decision.RepoIdentity",
		"Decision.NativeAuth.IdentityRef",
		"Decision.NativeAuth.ConfigIdentity",
		"Decision.Task.Title",
		"Decision.Task.ArtifactRef",
		"Decision.Snapshot.SourceRepo",
		"Decision.Probe.Executable",
		"Decision.Proposal.Path",
		"Decision.Proposal.Args.*",
		"Decision.Proposal.Dir",
		"Decision.Proposal.Overrides.*.Kind",
		"Decision.Proposal.Overrides.*.Name",
		"Decision.Proposal.Overrides.*.Reason",
		"Decision.Proposal.Overrides.*.Value",
		"Decision.Proposal.Manifest.*.Source",
		"Challenges.*.Text",
	}
}

var fieldPathPattern = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*(\.([A-Z][A-Za-z0-9]*|0|[1-9][0-9]{0,8}))*$`)

// Validate checks the policy content and that Revision is its exact digest.
func (p RetentionPolicyV1) Validate() error {
	if err := validatePolicyContent(p.Content); err != nil {
		return err
	}
	sum, err := RetentionPolicyDigestV1(p.Content)
	if err != nil {
		return err
	}
	if p.Revision != "sha256:"+hex.EncodeToString(sum[:]) {
		return fmt.Errorf("%w: policy revision", ErrInvalidContract)
	}
	return nil
}

// Validate checks approval shape only; it never establishes consent.
func (a RetentionApprovalV1) Validate() error {
	v := validation{}
	v.schema(a.SchemaVersion, 1)
	for _, s := range []string{a.PreparationID, a.ChallengeID, a.BindingSHA256, a.FieldSetSHA256, a.PolicyRevision, a.PeerID, a.RootID, a.ContextEpoch, a.ExpiresAt, a.ObservedAt, string(a.Decision)} {
		retentionString(&v, s)
	}
	v.require(validID(a.PreparationID) && validID(a.ChallengeID))
	v.require(lowerHash(a.BindingSHA256) && lowerHash(a.FieldSetSHA256) && prefixedHash(a.PolicyRevision))
	v.require(a.PeerID != "" && a.RootID != "" && a.ContextEpoch != "" && a.Generation > 0)
	v.require(timestamp(a.ExpiresAt) && timestamp(a.ObservedAt))
	if expires, err := time.Parse(time.RFC3339Nano, a.ExpiresAt); err == nil {
		observed, err := time.Parse(time.RFC3339Nano, a.ObservedAt)
		v.require(err == nil && !observed.After(expires))
	}
	v.require(a.Decision == RetentionApprove || a.Decision == RetentionReject)
	return v.err("approval")
}

// RetentionPolicyDigestV1 is SHA-256 over the policy domain and canonical content.
func RetentionPolicyDigestV1(c RetentionPolicyContentV1) ([32]byte, error) {
	if err := validatePolicyContent(c); err != nil {
		return [32]byte{}, err
	}
	raw, err := canonicalPolicyJSON(c)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(append([]byte(retentionPolicyDomain), raw...)), nil
}

// DeriveRetentionFieldsV1 resolves every retained binding value in exact path
// order. A retained path without a rule yields a row with empty classification
// and provenance (unclassified) for the policy checker to refuse; structural
// leaf errors are reserved for malformed inputs. Rules naming no retained
// path are reported by comparing Rules with the returned rows.
func DeriveRetentionFieldsV1(b BindingV1, p RetentionPolicyV1) ([]RetentionFieldV1, error) {
	if err := ValidateV1(b); err != nil {
		return nil, err
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	rules := make(map[string]RetentionRuleV1, len(p.Content.Rules))
	for _, r := range p.Content.Rules {
		rules[r.FieldPath] = r
	}
	var rows []RetentionFieldV1
	for _, pattern := range retainedPaths() {
		var err error
		resolve(reflect.ValueOf(b), strings.Split(pattern, "."), "", func(fieldPath string, value reflect.Value) {
			raw, encErr := canonicalJSON(value.Interface())
			if encErr != nil {
				err = encErr
				return
			}
			sum := sha256.Sum256(raw)
			r := rules[fieldPath]
			rows = append(rows, RetentionFieldV1{FieldPath: fieldPath, ValueSHA256: hex.EncodeToString(sum[:]), Classification: r.Classification, Provenance: r.Provenance})
		})
		if err != nil {
			return nil, err
		}
	}
	if len(rows) > maxRetentionRows {
		return nil, fmt.Errorf("%w: retained field count", ErrCapabilityUnsupported)
	}
	slices.SortFunc(rows, func(x, y RetentionFieldV1) int { return strings.Compare(x.FieldPath, y.FieldPath) })
	return rows, nil
}

// RetentionFieldSetSHA256 commits fully classified rows under their own domain.
func RetentionFieldSetSHA256(rows []RetentionFieldV1) ([32]byte, error) {
	v := validation{}
	if len(rows) > maxRetentionRows {
		v.unsupported = true
	}
	for i, r := range rows {
		retentionString(&v, r.FieldPath)
		v.require(fieldPathPattern.MatchString(r.FieldPath) && (i == 0 || rows[i-1].FieldPath < r.FieldPath))
		v.require(lowerHash(r.ValueSHA256) && knownClass(r.Classification) && knownProvenance(r.Provenance))
	}
	if err := v.err("field set"); err != nil {
		return [32]byte{}, err
	}
	raw, err := canonicalJSON(rows)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(append([]byte(retentionFieldsDomain), raw...)), nil
}

// EncodeRetentionPolicyV1 validates and returns canonical policy bytes.
func EncodeRetentionPolicyV1(p RetentionPolicyV1) ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return canonicalPolicyJSON(p)
}

// canonicalPolicyJSON applies the decoder's byte bound so no encoding or
// digest exists for policy bytes DecodeRetentionPolicyV1 refuses.
func canonicalPolicyJSON(x any) ([]byte, error) {
	raw, err := canonicalJSON(x)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxBytes {
		return nil, fmt.Errorf("%w: policy byte bound", ErrCapabilityUnsupported)
	}
	return raw, nil
}

// DecodeRetentionPolicyV1 strictly decodes and verifies canonical policy bytes.
func DecodeRetentionPolicyV1(raw []byte) (RetentionPolicyV1, error) {
	var p RetentionPolicyV1
	if err := strictDecode(raw, &p); err != nil {
		return RetentionPolicyV1{}, err
	}
	if err := p.Validate(); err != nil {
		return RetentionPolicyV1{}, err
	}
	return p, nil
}

// EncodeRetentionApprovalV1 validates and returns canonical approval bytes.
func EncodeRetentionApprovalV1(a RetentionApprovalV1) ([]byte, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}
	return canonicalJSON(a)
}

// DecodeRetentionApprovalV1 strictly decodes canonical approval bytes.
func DecodeRetentionApprovalV1(raw []byte) (RetentionApprovalV1, error) {
	var a RetentionApprovalV1
	if err := strictDecode(raw, &a); err != nil {
		return RetentionApprovalV1{}, err
	}
	if err := a.Validate(); err != nil {
		return RetentionApprovalV1{}, err
	}
	return a, nil
}

func validatePolicyContent(c RetentionPolicyContentV1) error {
	v := validation{}
	v.schema(c.SchemaVersion, 1)
	for _, s := range []string{c.RootID, c.Purpose, c.ExecutionPolicyRevision} {
		retentionString(&v, s)
	}
	v.require(c.RootID != "" && c.Purpose == RetentionPurposeV1 && prefixedHash(c.ExecutionPolicyRevision))
	if len(c.Rules) > maxRetentionRows {
		v.unsupported = true
	}
	for i, r := range c.Rules {
		retentionString(&v, r.FieldPath)
		v.require(fieldPathPattern.MatchString(r.FieldPath) && (i == 0 || c.Rules[i-1].FieldPath < r.FieldPath))
		v.require(knownClass(r.Classification) && knownProvenance(r.Provenance))
	}
	return v.err("policy")
}

func (v *validation) err(what string) error {
	if v.invalid {
		return fmt.Errorf("%w: %s", ErrInvalidContract, what)
	}
	if v.unsupported {
		return fmt.Errorf("%w: %s bound", ErrCapabilityUnsupported, what)
	}
	return nil
}

func retentionString(v *validation, s string) {
	v.require(utf8.ValidString(s) && !strings.ContainsRune(s, 0))
	if len(s) > maxRetentionString {
		v.unsupported = true
	}
}

func knownClass(c RetentionClassV1) bool {
	return c == RetentionNonSecret || c == RetentionForbidden
}

func knownProvenance(p RetentionProvenanceV1) bool {
	switch p {
	case RetentionOperatorInput, RetentionServiceConfig, RetentionNativeStatus, RetentionNativeAuth, RetentionTranscript, RetentionCredentialEnv:
		return true
	}
	return false
}

func prefixedHash(s string) bool {
	rest, ok := strings.CutPrefix(s, "sha256:")
	return ok && lowerHash(rest)
}

func resolve(x reflect.Value, segments []string, prefix string, emit func(string, reflect.Value)) {
	if x.Kind() == reflect.Pointer {
		if x.IsNil() {
			return
		}
		x = x.Elem()
	}
	if len(segments) == 0 {
		emit(prefix, x)
		return
	}
	join := func(s string) string {
		if prefix == "" {
			return s
		}
		return prefix + "." + s
	}
	if segments[0] == "*" {
		for i := 0; i < x.Len(); i++ {
			resolve(x.Index(i), segments[1:], join(fmt.Sprint(i)), emit)
		}
		return
	}
	resolve(x.FieldByName(segments[0]), segments[1:], join(segments[0]), emit)
}

// strictDecode applies the binding codec's duplicate/unknown/missing/null and
// canonical re-encoding refusals to a retention record.
func strictDecode(raw []byte, out any) error {
	if len(raw) > maxBytes {
		return fmt.Errorf("%w: byte bound", ErrCapabilityUnsupported)
	}
	if !utf8.Valid(raw) {
		return fmt.Errorf("%w: UTF-8", ErrInvalidContract)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := shape(dec, reflect.TypeOf(out).Elem(), 0); err != nil {
		return fmt.Errorf("%w: shape", ErrInvalidContract)
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("%w: trailing input", ErrInvalidContract)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%w: value", ErrInvalidContract)
	}
	encoded, err := canonicalJSON(reflect.ValueOf(out).Elem().Interface())
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, encoded) {
		return fmt.Errorf("%w: noncanonical", ErrInvalidContract)
	}
	return nil
}

// canonicalJSON encodes any closed DTO value with the binding codec's
// canonical rules: copied collections, nil slices as [], no HTML escaping and
// literal U+2028/U+2029.
func canonicalJSON(x any) ([]byte, error) {
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(collections(reflect.ValueOf(x)).Interface()); err != nil {
		return nil, fmt.Errorf("%w: encode", ErrInvalidContract)
	}
	raw := bytes.TrimSuffix(out.Bytes(), []byte{'\n'})
	var utf8JSON []byte
	for i := 0; i < len(raw); {
		if raw[i] == '\\' && i+1 < len(raw) {
			if i+6 <= len(raw) && (string(raw[i:i+6]) == `\u2028` || string(raw[i:i+6]) == `\u2029`) {
				if raw[i+5] == '8' {
					utf8JSON = append(utf8JSON, 0xe2, 0x80, 0xa8)
				} else {
					utf8JSON = append(utf8JSON, 0xe2, 0x80, 0xa9)
				}
				i += 6
				continue
			}
			utf8JSON = append(utf8JSON, raw[i:i+2]...)
			i += 2
			continue
		}
		utf8JSON = append(utf8JSON, raw[i])
		i++
	}
	return utf8JSON, nil
}
