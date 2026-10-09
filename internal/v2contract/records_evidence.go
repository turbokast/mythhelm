//nolint:misspell // Artifact is the spec-mandated type name and wire key
package v2contract

import (
	"errors"
	"fmt"
	"math"
	"time"
)

func evidenceSchema(record string, got int) error {
	if got != SchemaVersion {
		return fmt.Errorf("v2contract: %s schema_version %d, want %d", record, got, SchemaVersion)
	}
	return nil
}

func evidenceNonEmpty(record, field, value string) error {
	if value == "" {
		return fmt.Errorf("v2contract: %s %s is empty", record, field)
	}
	return nil
}

func evidenceFinite(record, field string, v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("v2contract: %s %s is not finite", record, field)
	}
	return nil
}

func evidenceGitObject(record, field string, g GitObject) error {
	if g.Format == "" || g.Value == "" {
		return fmt.Errorf("v2contract: %s %s needs format and value", record, field)
	}
	return nil
}

// RoutingDecision records why a route was selected (v2 §4.1). Absent scores
// and probability stay absent: unknown is never zero (I09).
type RoutingDecision struct {
	SchemaVersion        int                `json:"schema_version" toml:"schema_version"`
	DecisionID           string             `json:"decision_id" toml:"decision_id"`
	RunID                string             `json:"run_id" toml:"run_id"`
	Eligible             []string           `json:"eligible" toml:"eligible"`
	Exclusions           map[string]string  `json:"exclusions" toml:"exclusions"` // route -> reason
	Features             map[string]string  `json:"features" toml:"features"`     // pre-assignment features
	SelectedRoute        string             `json:"selected_route" toml:"selected_route"`
	SelectedPolicy       string             `json:"selected_policy" toml:"selected_policy"`
	Rationale            string             `json:"rationale" toml:"rationale"`
	Scores               map[string]float64 `json:"scores,omitempty" toml:"scores,omitempty"`
	Uncertainty          string             `json:"uncertainty,omitempty" toml:"uncertainty,omitempty"`
	SelectionProbability *float64           `json:"selection_probability,omitempty" toml:"selection_probability,omitempty"`
	OverrideProvenance   string             `json:"override_provenance,omitempty" toml:"override_provenance,omitempty"`
}

// Validate requires the identity and selection fields and rejects NaN and
// ±Inf in Scores and SelectionProbability, which Digest cannot encode.
func (r RoutingDecision) Validate() error {
	const rec = "routing decision"
	if err := evidenceSchema(rec, r.SchemaVersion); err != nil {
		return err
	}
	for _, f := range []struct{ name, value string }{
		{"decision_id", r.DecisionID},
		{"run_id", r.RunID},
		{"selected_route", r.SelectedRoute},
		{"selected_policy", r.SelectedPolicy},
		{"rationale", r.Rationale},
	} {
		if err := evidenceNonEmpty(rec, f.name, f.value); err != nil {
			return err
		}
	}
	for route, score := range r.Scores {
		if err := evidenceFinite(rec, "scores["+route+"]", score); err != nil {
			return err
		}
	}
	if r.SelectionProbability != nil {
		return evidenceFinite(rec, "selection_probability", *r.SelectionProbability)
	}
	return nil
}

// DesignDisposition is proposed | accepted | superseded. It is never
// confused with route ranking (v2 §4.1).
type DesignDisposition string

// DesignDisposition values.
const (
	DesignProposed   DesignDisposition = "proposed"
	DesignAccepted   DesignDisposition = "accepted"
	DesignSuperseded DesignDisposition = "superseded"
)

// Valid reports whether d is a design disposition.
func (d DesignDisposition) Valid() bool {
	switch d {
	case DesignProposed, DesignAccepted, DesignSuperseded:
		return true
	}
	return false
}

// DesignDecision is a revisioned design choice; revisions are ordered through
// Supersedes (I20).
type DesignDecision struct {
	SchemaVersion int               `json:"schema_version" toml:"schema_version"`
	DecisionID    string            `json:"decision_id" toml:"decision_id"`
	Disposition   DesignDisposition `json:"disposition" toml:"disposition"`
	Rationale     string            `json:"rationale" toml:"rationale"`
	Alternatives  []string          `json:"alternatives" toml:"alternatives"`
	Owner         string            `json:"owner" toml:"owner"` // authorised decision owner
	Scope         string            `json:"scope" toml:"scope"`
	Sources       []string          `json:"sources" toml:"sources"`
	Revision      int               `json:"revision" toml:"revision"`
	Supersedes    *int              `json:"supersedes,omitempty" toml:"supersedes,omitempty"`
}

// Validate requires an in-scale disposition, an owner, a revision of at
// least 1, and a Supersedes that names an earlier revision.
func (d DesignDecision) Validate() error {
	const rec = "design decision"
	if err := evidenceSchema(rec, d.SchemaVersion); err != nil {
		return err
	}
	if err := evidenceNonEmpty(rec, "decision_id", d.DecisionID); err != nil {
		return err
	}
	if !d.Disposition.Valid() {
		return fmt.Errorf("v2contract: %s disposition %q is out of scale", rec, string(d.Disposition))
	}
	if err := evidenceNonEmpty(rec, "owner", d.Owner); err != nil {
		return err
	}
	if err := evidenceNonEmpty(rec, "rationale", d.Rationale); err != nil {
		return err
	}
	if d.Revision < 1 {
		return fmt.Errorf("v2contract: %s revision %d below 1", rec, d.Revision)
	}
	if d.Supersedes != nil && (*d.Supersedes < 1 || *d.Supersedes >= d.Revision) {
		return fmt.Errorf("v2contract: %s supersedes %d is not an earlier revision than %d", rec, *d.Supersedes, d.Revision)
	}
	return nil
}

// Artifact is a content-addressed stored object (v2 §4.1). SizeBytes is a
// pointer so a missing or null size is rejected while 0 stays a real size (I09).
type Artifact struct {
	SchemaVersion  int        `json:"schema_version" toml:"schema_version"`
	ArtifactID     string     `json:"artifact_id" toml:"artifact_id"`
	RepoID         string     `json:"repo_id" toml:"repo_id"`
	SHA256         string     `json:"sha256" toml:"sha256"`
	SizeBytes      *int64     `json:"size_bytes" toml:"size_bytes"`
	MediaType      string     `json:"media_type" toml:"media_type"`
	ProducerID     string     `json:"producer_id" toml:"producer_id"`
	Snapshot       GitObject  `json:"snapshot" toml:"snapshot"`
	Sensitivity    string     `json:"sensitivity" toml:"sensitivity"`
	Dependencies   []string   `json:"dependencies" toml:"dependencies"`
	ValidUntil     *time.Time `json:"valid_until,omitempty" toml:"valid_until,omitempty"`
	RetentionRoots []string   `json:"retention_roots" toml:"retention_roots"`
}

// Validate requires identities, a SHA256 of exactly 64 lowercase hex
// characters and a present, non-negative SizeBytes.
func (a Artifact) Validate() error {
	const rec = "artefact"
	if err := evidenceSchema(rec, a.SchemaVersion); err != nil {
		return err
	}
	for _, f := range []struct{ name, value string }{
		{"artifact_id", a.ArtifactID},
		{"repo_id", a.RepoID},
		{"producer_id", a.ProducerID},
	} {
		if err := evidenceNonEmpty(rec, f.name, f.value); err != nil {
			return err
		}
	}
	if !isSHA256Hex(a.SHA256) {
		return fmt.Errorf("v2contract: %s sha256 %q is not 64 lowercase hex characters", rec, a.SHA256)
	}
	if a.SizeBytes == nil {
		return errors.New("v2contract: artefact size_bytes is missing or null")
	}
	if *a.SizeBytes < 0 {
		return fmt.Errorf("v2contract: artefact size_bytes %d is negative", *a.SizeBytes)
	}
	return nil
}

func isSHA256Hex(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// ObservationKind is controller | tool | native. Agent prose is a claim,
// never an Observation with native authority (v2 §4.1).
type ObservationKind string

// ObservationKind values.
const (
	ObservationController ObservationKind = "controller"
	ObservationTool       ObservationKind = "tool"
	ObservationNative     ObservationKind = "native"
)

// Valid reports whether k is an observation kind.
func (k ObservationKind) Valid() bool {
	switch k {
	case ObservationController, ObservationTool, ObservationNative:
		return true
	}
	return false
}

// Observation is a fact observed by the controller, a tool or a native agent.
type Observation struct {
	SchemaVersion int             `json:"schema_version" toml:"schema_version"`
	ObservationID string          `json:"observation_id" toml:"observation_id"`
	Kind          ObservationKind `json:"kind" toml:"kind"`
	SourceID      string          `json:"source_id" toml:"source_id"`
	SourceVersion string          `json:"source_version" toml:"source_version"`
	ObservedAt    time.Time       `json:"observed_at" toml:"observed_at"`
	ValueKind     string          `json:"value_kind" toml:"value_kind"`
	Value         string          `json:"value" toml:"value"`
	Uncertainty   string          `json:"uncertainty" toml:"uncertainty"`
}

// Validate requires an in-scale kind, a source, a value kind and a non-zero
// ObservedAt.
func (o Observation) Validate() error {
	const rec = "observation"
	if err := evidenceSchema(rec, o.SchemaVersion); err != nil {
		return err
	}
	if err := evidenceNonEmpty(rec, "observation_id", o.ObservationID); err != nil {
		return err
	}
	if !o.Kind.Valid() {
		return fmt.Errorf("v2contract: %s kind %q is out of scale", rec, string(o.Kind))
	}
	if err := evidenceNonEmpty(rec, "source_id", o.SourceID); err != nil {
		return err
	}
	if err := evidenceNonEmpty(rec, "value_kind", o.ValueKind); err != nil {
		return err
	}
	if o.ObservedAt.IsZero() {
		return errors.New("v2contract: observation observed_at is zero")
	}
	return nil
}

// Verification is the sole attestation shape: a check result bound to an
// exact candidate (I07 lineage).
type Verification struct {
	SchemaVersion            int               `json:"schema_version" toml:"schema_version"`
	VerificationID           string            `json:"verification_id" toml:"verification_id"`
	Candidate                GitObject         `json:"candidate" toml:"candidate"` // exact candidate/tree
	Environment              string            `json:"environment" toml:"environment"`
	AcceptanceContractDigest string            `json:"acceptance_contract_digest" toml:"acceptance_contract_digest"`
	CheckDefinition          string            `json:"check_definition" toml:"check_definition"`
	CheckVersion             string            `json:"check_version" toml:"check_version"`
	VerifierID               string            `json:"verifier_id" toml:"verifier_id"`
	Results                  map[string]string `json:"results" toml:"results"`
	Logs                     []string          `json:"logs" toml:"logs"`
}

// Validate requires the attestation identity: verification, candidate,
// verifier and check definition.
func (v Verification) Validate() error {
	const rec = "verification"
	if err := evidenceSchema(rec, v.SchemaVersion); err != nil {
		return err
	}
	if err := evidenceNonEmpty(rec, "verification_id", v.VerificationID); err != nil {
		return err
	}
	if err := evidenceGitObject(rec, "candidate", v.Candidate); err != nil {
		return err
	}
	if err := evidenceNonEmpty(rec, "verifier_id", v.VerifierID); err != nil {
		return err
	}
	return evidenceNonEmpty(rec, "check_definition", v.CheckDefinition)
}
