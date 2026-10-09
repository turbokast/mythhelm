// Package v2contract is the canonical v2 contract vocabulary (master spec
// v2 §4): record types, lifecycle tables, the error catalogue and the
// event-v2 envelope. Pure data and pure validators; no I/O, no time source,
// no global state.
package v2contract

import (
	"errors"
	"fmt"
)

// SchemaVersion is the schema_version every canonical v2 payload carries.
const SchemaVersion = 2

// Frame limits (v2 §4.3; NFR-1). Enforcement points live in later specs,
// which reference these values.
const (
	MaxFrameBytes   = 1 << 20 // 1 MiB encoded frame
	MaxNestingDepth = 64
	MaxArtifactRefs = 128 // bounded artefact references per frame (D2)
)

// CheckFrameLimits rejects a frame whose encoded size, nesting depth or
// artefact reference count exceeds its limit, naming the dimension.
func CheckFrameLimits(encodedLen, depth, refs int) error {
	switch {
	case encodedLen > MaxFrameBytes:
		return fmt.Errorf("v2contract: frame bytes %d exceed limit %d", encodedLen, MaxFrameBytes)
	case depth > MaxNestingDepth:
		return fmt.Errorf("v2contract: nesting depth %d exceeds limit %d", depth, MaxNestingDepth)
	case refs > MaxArtifactRefs:
		return fmt.Errorf("v2contract: artefact refs %d exceed limit %d", refs, MaxArtifactRefs)
	}
	return nil
}

// GitObject carries a Git object ID with its format and full value (v2 §4.2:
// never assume SHA-1 length).
type GitObject struct {
	Format string `json:"format" toml:"format"` // e.g. "sha1", "sha256"
	Value  string `json:"value" toml:"value"`   // full hex value
}

// Budget is one finite envelope (v2 §4.1 Run/Experiment; I21). Limit "unknown"
// means unmeasured, never zero (I09).
type Budget struct {
	Name  string `json:"name" toml:"name"`
	Limit string `json:"limit" toml:"limit"`
	Unit  string `json:"unit" toml:"unit"`
}

// RevisionRef pins a dependency to an exact artefact or contract revision
// (v2 §4.1 TaskRevision).
type RevisionRef struct {
	Kind     string `json:"kind" toml:"kind"` // one of two accepted kinds
	ID       string `json:"id" toml:"id"`
	Revision int    `json:"revision" toml:"revision"` // >= 1
	Digest   string `json:"digest,omitempty" toml:"digest,omitempty"`
}

// Validate rejects a kind outside the two accepted kinds, an empty id and a
// revision below 1, naming the field.
func (r RevisionRef) Validate() error {
	if r.Kind != "artifact" && r.Kind != "contract" { //nolint:misspell // "artifact" is the wire value of the kind
		return fmt.Errorf("v2contract: revision ref kind %q not an accepted kind", r.Kind)
	}
	if r.ID == "" {
		return errors.New("v2contract: revision ref id is empty")
	}
	if r.Revision < 1 {
		return fmt.Errorf("v2contract: revision ref revision %d below 1", r.Revision)
	}
	return nil
}

// RequestEnvelope is the v2 §4.2 envelope every mutating control request
// carries: the client-chosen idempotency key, the target object identity, the
// ledger revision the client read, and the controller generation where
// applicable (absent omits it, I09).
type RequestEnvelope struct {
	OperationID      string `json:"operation_id" toml:"operation_id"`
	Object           string `json:"object,omitempty" toml:"object,omitempty"`
	ExpectedRevision int64  `json:"expected_revision" toml:"expected_revision"`
	Generation       *int64 `json:"generation,omitempty" toml:"generation,omitempty"`
}

// Validate requires a non-empty OperationID and a non-negative
// ExpectedRevision. Object and Generation are carried opaquely: the owning
// method defines where they are required.
func (r RequestEnvelope) Validate() error {
	if r.OperationID == "" {
		return errors.New("v2contract: request envelope operation_id is empty")
	}
	if r.ExpectedRevision < 0 {
		return fmt.Errorf("v2contract: request envelope expected_revision %d is negative", r.ExpectedRevision)
	}
	return nil
}
