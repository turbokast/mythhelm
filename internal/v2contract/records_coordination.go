package v2contract

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// ContextManifest is the immutable handoff record (v2 §4.1).
type ContextManifest struct {
	SchemaVersion            int      `json:"schema_version" toml:"schema_version"`
	ManifestID               string   `json:"manifest_id" toml:"manifest_id"`
	Requirements             []string `json:"requirements" toml:"requirements"`
	DecisionIDs              []string `json:"decisions" toml:"decisions"`
	Inputs                   []string `json:"inputs" toml:"inputs"`
	Claims                   []string `json:"claims" toml:"claims"`
	Evidence                 []string `json:"evidence" toml:"evidence"`
	Unresolved               []string `json:"unresolved" toml:"unresolved"`
	Disclosures              []string `json:"disclosures" toml:"disclosures"`
	LastAcknowledgedRevision int      `json:"last_acknowledged_revision" toml:"last_acknowledged_revision"`
}

// Validate requires schema_version 2 and a non-empty manifest_id.
func (m ContextManifest) Validate() error {
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("v2contract: manifest schema_version %d is not %d", m.SchemaVersion, SchemaVersion)
	}
	if m.ManifestID == "" {
		return errors.New("v2contract: manifest manifest_id is empty")
	}
	return nil
}

// Handoff is the sending-use name of ContextManifest (v2 §4.1 lists one
// "Handoff / ContextManifest" row).
type Handoff = ContextManifest

// DeliveryDisposition is an open string (values grow with integrations; the
// v2 §4.3 example uses "queued_for_next_turn"), validated non-empty (D6).
type DeliveryDisposition string

// Message is a coordination message (v2 §4.1).
type Message struct {
	SchemaVersion int                 `json:"schema_version" toml:"schema_version"`
	MessageID     string              `json:"message_id" toml:"message_id"`
	Sender        string              `json:"sender" toml:"sender"`
	Recipient     string              `json:"recipient" toml:"recipient"`
	Kind          string              `json:"kind" toml:"kind"`
	CausedBy      string              `json:"caused_by,omitempty" toml:"caused_by,omitempty"`
	References    []string            `json:"references" toml:"references"`
	Sensitivity   string              `json:"sensitivity" toml:"sensitivity"`
	ExpiresAt     *time.Time          `json:"expires_at,omitempty" toml:"expires_at,omitempty"`
	Delivery      DeliveryDisposition `json:"delivery" toml:"delivery"`
}

// Validate requires schema_version 2 and non-empty identity, content and
// delivery fields.
func (m Message) Validate() error {
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("v2contract: message schema_version %d is not %d", m.SchemaVersion, SchemaVersion)
	}
	if m.MessageID == "" {
		return errors.New("v2contract: message message_id is empty")
	}
	if m.Sender == "" {
		return errors.New("v2contract: message sender is empty")
	}
	if m.Recipient == "" {
		return errors.New("v2contract: message recipient is empty")
	}
	if m.Kind == "" {
		return errors.New("v2contract: message kind is empty")
	}
	if m.Sensitivity == "" {
		return errors.New("v2contract: message sensitivity is empty")
	}
	if m.Delivery == "" {
		return errors.New("v2contract: message delivery is empty")
	}
	return nil
}

// Grant is user-issued standing or one-use authority (v2 §4.1). Unknown keys
// fail validation (v2 §4.2 authority contracts).
type Grant struct {
	SchemaVersion  int        `json:"schema_version" toml:"schema_version"`
	GrantID        string     `json:"grant_id" toml:"grant_id"`
	Use            string     `json:"use" toml:"use"` // "standing" | "one_use"
	Effect         string     `json:"effect" toml:"effect"`
	Target         string     `json:"target" toml:"target"`
	ArtifactID     string     `json:"artifact_id,omitempty" toml:"artifact_id,omitempty"` //nolint:misspell // "artifact_id" is the wire name
	ExpectedState  string     `json:"expected_state" toml:"expected_state"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty" toml:"expires_at,omitempty"`
	Revoked        bool       `json:"revoked" toml:"revoked"`
	OperationID    string     `json:"operation_id" toml:"operation_id"`
	Reconciliation string     `json:"reconciliation,omitempty" toml:"reconciliation,omitempty"`
}

// Accepted grant uses.
const (
	GrantStanding = "standing"
	GrantOneUse   = "one_use"
)

// Validate requires schema_version 2, a non-empty grant_id, a closed Use
// value, and non-empty effect, target, expected_state and operation_id.
func (g Grant) Validate() error {
	if g.SchemaVersion != SchemaVersion {
		return fmt.Errorf("v2contract: grant schema_version %d is not %d", g.SchemaVersion, SchemaVersion)
	}
	if g.GrantID == "" {
		return errors.New("v2contract: grant grant_id is empty")
	}
	if g.Use != GrantStanding && g.Use != GrantOneUse {
		return fmt.Errorf("v2contract: grant use %q is not standing or one_use", g.Use)
	}
	if g.Effect == "" {
		return errors.New("v2contract: grant effect is empty")
	}
	if g.Target == "" {
		return errors.New("v2contract: grant target is empty")
	}
	if g.ExpectedState == "" {
		return errors.New("v2contract: grant expected_state is empty")
	}
	if g.OperationID == "" {
		return errors.New("v2contract: grant operation_id is empty")
	}
	return nil
}

// EffectIntent is the single-effect-use name of Grant (v2 §4.1 lists one
// "Grant / EffectIntent" row).
type EffectIntent = Grant

// Reservation coordinates resource or billing buckets (v2 §4.1).
type Reservation struct {
	SchemaVersion   int        `json:"schema_version" toml:"schema_version"`
	ReservationID   string     `json:"reservation_id" toml:"reservation_id"`
	Bucket          string     `json:"bucket" toml:"bucket"`
	Scope           string     `json:"scope" toml:"scope"`
	Owner           string     `json:"owner" toml:"owner"`
	Generation      int64      `json:"generation" toml:"generation"`
	Quantity        string     `json:"quantity" toml:"quantity"` // quantity or "unknown", never 0 for unknown (I09)
	Status          string     `json:"status" toml:"status"`     // open string, validated non-empty (D6)
	ExpiresAt       *time.Time `json:"expires_at,omitempty" toml:"expires_at,omitempty"`
	HeartbeatAt     *time.Time `json:"heartbeat_at,omitempty" toml:"heartbeat_at,omitempty"`
	ReleaseEvidence string     `json:"release_evidence,omitempty" toml:"release_evidence,omitempty"`
}

// Validate requires schema_version 2 and non-empty identity, bucket, scope,
// owner, quantity and status.
func (r Reservation) Validate() error {
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("v2contract: reservation schema_version %d is not %d", r.SchemaVersion, SchemaVersion)
	}
	if r.ReservationID == "" {
		return errors.New("v2contract: reservation reservation_id is empty")
	}
	if r.Bucket == "" {
		return errors.New("v2contract: reservation bucket is empty")
	}
	if r.Scope == "" {
		return errors.New("v2contract: reservation scope is empty")
	}
	if r.Owner == "" {
		return errors.New("v2contract: reservation owner is empty")
	}
	if r.Quantity == "" {
		return errors.New("v2contract: reservation quantity is empty")
	}
	if r.Status == "" {
		return errors.New("v2contract: reservation status is empty")
	}
	return nil
}

// PolicyVersion is one immutable revision of a policy (v2 §4.1; I20).
type PolicyVersion struct {
	SchemaVersion   int               `json:"schema_version" toml:"schema_version"`
	PolicyID        string            `json:"policy_id" toml:"policy_id"`
	Version         int               `json:"version" toml:"version"` // immutable once referenced (I20)
	Parent          *int              `json:"parent,omitempty" toml:"parent,omitempty"`
	ActionFamily    string            `json:"action_family" toml:"action_family"`
	Prompts         map[string]string `json:"prompts" toml:"prompts"`
	Context         map[string]string `json:"context" toml:"context"`
	Review          map[string]string `json:"review" toml:"review"`
	Decomposition   map[string]string `json:"decomposition" toml:"decomposition"`
	EligibleCohorts []string          `json:"eligible_cohorts" toml:"eligible_cohorts"`
	Evidence        []string          `json:"evidence" toml:"evidence"`
	PromotedFrom    string            `json:"promoted_from,omitempty" toml:"promoted_from,omitempty"`
	RollbackTo      string            `json:"rollback_to,omitempty" toml:"rollback_to,omitempty"`
}

// Validate requires schema_version 2, a non-empty policy_id and action
// family, and Version >= 1 (a material change is a new revision, never an
// edit).
func (p PolicyVersion) Validate() error {
	if p.SchemaVersion != SchemaVersion {
		return fmt.Errorf("v2contract: policy schema_version %d is not %d", p.SchemaVersion, SchemaVersion)
	}
	if p.PolicyID == "" {
		return errors.New("v2contract: policy policy_id is empty")
	}
	if p.Version < 1 {
		return fmt.Errorf("v2contract: policy version %d below 1", p.Version)
	}
	if p.ActionFamily == "" {
		return errors.New("v2contract: policy action_family is empty")
	}
	return nil
}

// Experiment is a controlled comparison with traffic splits (v2 §4.1).
type Experiment struct {
	SchemaVersion int                `json:"schema_version" toml:"schema_version"`
	ExperimentID  string             `json:"experiment_id" toml:"experiment_id"`
	Hypothesis    string             `json:"hypothesis" toml:"hypothesis"`
	Population    string             `json:"population" toml:"population"`
	Unit          string             `json:"unit" toml:"unit"`
	Variants      []string           `json:"variants" toml:"variants"`
	Splits        map[string]float64 `json:"splits" toml:"splits"`
	Assignment    string             `json:"assignment" toml:"assignment"`
	Budgets       []Budget           `json:"budgets" toml:"budgets"`
	Metrics       []string           `json:"metrics" toml:"metrics"`
	Margins       map[string]string  `json:"margins" toml:"margins"`
	StoppingRule  string             `json:"stopping_rule" toml:"stopping_rule"`
	Evidence      []string           `json:"evidence" toml:"evidence"`
	Disposition   string             `json:"disposition" toml:"disposition"` // open string (D6)
}

// Validate requires schema_version 2, non-empty identity and content fields,
// and finite split values.
func (e Experiment) Validate() error {
	if e.SchemaVersion != SchemaVersion {
		return fmt.Errorf("v2contract: experiment schema_version %d is not %d", e.SchemaVersion, SchemaVersion)
	}
	if e.ExperimentID == "" {
		return errors.New("v2contract: experiment experiment_id is empty")
	}
	if e.Hypothesis == "" {
		return errors.New("v2contract: experiment hypothesis is empty")
	}
	if e.Population == "" {
		return errors.New("v2contract: experiment population is empty")
	}
	if e.Unit == "" {
		return errors.New("v2contract: experiment unit is empty")
	}
	for variant, split := range e.Splits {
		if math.IsNaN(split) || math.IsInf(split, 0) {
			return fmt.Errorf("v2contract: experiment splits[%q] is not finite", variant)
		}
	}
	if e.Assignment == "" {
		return errors.New("v2contract: experiment assignment is empty")
	}
	if e.StoppingRule == "" {
		return errors.New("v2contract: experiment stopping_rule is empty")
	}
	if e.Disposition == "" {
		return errors.New("v2contract: experiment disposition is empty")
	}
	return nil
}
