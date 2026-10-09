package v2contract

import (
	"errors"
	"fmt"
	"time"
)

// Capability is the v2 §4.2 capability scale. Unknown stays distinct from
// unsupported (I09).
type Capability string

// Capability values.
const (
	CapabilitySupported   Capability = "supported"
	CapabilityUnsupported Capability = "unsupported"
	CapabilityUnknown     Capability = "unknown"
)

// Valid reports whether c is on the capability scale.
func (c Capability) Valid() bool {
	switch c {
	case CapabilitySupported, CapabilityUnsupported, CapabilityUnknown:
		return true
	}
	return false
}

// Progress is the v2 §4.2 qualification progress scale (I14).
type Progress string

// Progress values in v2 §4.2 scale order.
const (
	ProgressPlanned             Progress = "planned"
	ProgressDocumentedCandidate Progress = "documented-candidate"
	ProgressFixtureTested       Progress = "fixture-tested"
	ProgressLiveQualified       Progress = "live-qualified"
	ProgressExperimental        Progress = "experimental"
	ProgressBlocked             Progress = "blocked"
	ProgressUnsupported         Progress = "unsupported"
)

// Valid reports whether p is on the progress scale.
func (p Progress) Valid() bool {
	switch p {
	case ProgressPlanned, ProgressDocumentedCandidate, ProgressFixtureTested,
		ProgressLiveQualified, ProgressExperimental, ProgressBlocked,
		ProgressUnsupported:
		return true
	}
	return false
}

// Eligibility is the v2 §4.2 eligibility scale.
type Eligibility string

// Eligibility values.
const (
	EligibilityEligible    Eligibility = "eligible"
	EligibilityBlocked     Eligibility = "blocked"
	EligibilityUnsupported Eligibility = "unsupported"
)

// Valid reports whether e is on the eligibility scale.
func (e Eligibility) Valid() bool {
	switch e {
	case EligibilityEligible, EligibilityBlocked, EligibilityUnsupported:
		return true
	}
	return false
}

// ScaleValue is a vocabulary word that can validate itself.
type ScaleValue interface {
	~string
	Valid() bool
}

// Claim binds a vocabulary value to its evidence, scope and expiry (v2 §4.2).
type Claim[T ScaleValue] struct {
	Value     T          `json:"value" toml:"value"`
	Evidence  string     `json:"evidence" toml:"evidence"`
	Scope     string     `json:"scope" toml:"scope"`
	ExpiresAt *time.Time `json:"expires_at,omitempty" toml:"expires_at,omitempty"`
}

// Validate requires an in-scale value (naming it), non-empty evidence and
// scope, and a non-zero expiry.
func (c Claim[T]) Validate() error {
	if !c.Value.Valid() {
		return fmt.Errorf("v2contract: claim value %q is out of scale", string(c.Value))
	}
	if c.Evidence == "" {
		return errors.New("v2contract: claim evidence is empty")
	}
	if c.Scope == "" {
		return errors.New("v2contract: claim scope is empty")
	}
	if c.ExpiresAt == nil || c.ExpiresAt.IsZero() {
		return errors.New("v2contract: claim expires_at is required")
	}
	return nil
}
