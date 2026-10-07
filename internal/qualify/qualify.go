// Package qualify is MYTHHELM's versioned qualification record model
// (v2 §§4.2, 7.1, 7.3): the record types, honest-label scales and canonical
// digest/decode contract every registry task builds against.
package qualify

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Progress is the v2 §4.2 qualification scale (AC-2.2).
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

// Verdict is one column's value (AC-1.2).
type Verdict string

// Column verdicts.
const (
	Proven    Verdict = "proven"
	NotProven Verdict = "not-proven"
	Unknown   Verdict = "unknown"
)

// DatumLabel is the v2 §7.3 data provenance label (AC-5.2).
type DatumLabel string

// Datum provenance labels.
const (
	Reported     DatumLabel = "reported"
	Observed     DatumLabel = "observed"
	Estimated    DatumLabel = "estimated"
	UserDeclared DatumLabel = "user-declared"
	DatumUnknown DatumLabel = "unknown"
)

// unknownString marks unestablished string data (I09).
const unknownString = "unknown"

// Key is the v2 §7.1 qualification key.
type Key struct {
	Harness          string `json:"harness"`
	Surface          string `json:"surface"`
	ExecutableDigest string `json:"executable_digest"`
	AdapterProtocol  string `json:"adapter_protocol"`
	OS               string `json:"os"`
	Arch             string `json:"arch"`
	ProviderEndpoint string `json:"provider_endpoint"`
	ModelSnapshot    string `json:"model_snapshot"`
	EffortSettings   string `json:"effort_settings"`
	AuthCategory     string `json:"auth_category"`
	ConfigDigest     string `json:"config_digest"`
	TrustProfile     string `json:"trust_profile"`
	WorkspaceClass   string `json:"workspace_class"`
	EntitlementClass string `json:"entitlement_class"`
}

// Evidence is one versioned support for a column verdict (v2 §13).
type Evidence struct {
	ID          string     `json:"id"`
	Method      string     `json:"method"`
	Suite       string     `json:"suite"`
	Result      string     `json:"result"`
	Uncertainty string     `json:"uncertainty"`
	At          time.Time  `json:"at"`
	Expiry      *time.Time `json:"expiry"`
	Label       DatumLabel `json:"label"`
	Source      string     `json:"source"`
}

// Datum is one labelled quantitative datum (AC-5.2, v2 §7.3): every
// quantity carries its label, unit, scope, source and timestamp.
type Datum struct {
	Quantity string     `json:"quantity"`
	Label    DatumLabel `json:"label"`
	Unit     string     `json:"unit"`
	Scope    string     `json:"scope"`
	Source   string     `json:"source"`
	At       time.Time  `json:"at"`
}

// Column is one independently-recorded qualification column (AC-1.2).
type Column struct {
	Verdict  Verdict    `json:"verdict"`
	Evidence []Evidence `json:"evidence"`
}

// Record is one immutable revision of a qualification record (AC-1.3).
type Record struct {
	SchemaVersion int                   `json:"schema_version"`
	Revision      int                   `json:"revision"`
	Digest        string                `json:"digest"`
	Key           Key                   `json:"key"`
	Progress      Progress              `json:"progress"`
	Fidelity      Column                `json:"fidelity"`
	Entitlement   Column                `json:"entitlement"`
	Lifecycle     Column                `json:"lifecycle"`
	Capabilities  map[string]Capability `json:"capabilities"`
	Quota         Datum                 `json:"quota"`
	NextTest      string                `json:"next_test"`
	SupersededAt  *time.Time            `json:"superseded_at"`
}

// Capability is one v2 §4.2 capability value with its support.
type Capability struct {
	Value    string     `json:"value"`
	Evidence string     `json:"evidence"`
	Scope    string     `json:"scope"`
	Expiry   *time.Time `json:"expiry"`
}

// KeyHash returns the hex SHA-256 over the canonical JSON of k. Every
// store caller derives key hashes through this function. Empty fields
// hash as "unknown", exactly as DecodeRecord would default them, so a
// hand-built key and its decoded form hash identically.
func KeyHash(k Key) string {
	k = normalizeKeyForHash(k)
	raw, err := json.Marshal(k)
	if err != nil {
		// Unreachable: Key holds only strings, which always marshal.
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// CanonicalDigest recomputes the record's content digest (NFR-2):
// "sha256:<hex>" over the canonical JSON of r minus Digest and
// SupersededAt (the two carriage fields are absent from the hashed
// bytes, not zeroed). An out-of-scale enum value returns an error
// naming the offending field. Nil collections and empty free-text
// fields hash as their decoded forms, so equivalent records agree.
func CanonicalDigest(r Record) (string, error) {
	if err := validateRecord(r); err != nil {
		return "", err
	}
	r = normalizeForHash(r)
	raw, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("qualify: canonical record JSON: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", fmt.Errorf("qualify: canonical record JSON: %w", err)
	}
	delete(m, "digest")
	delete(m, "superseded_at")
	raw, err = json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("qualify: canonical record JSON: %w", err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// DecodeRecord decodes canonical record JSON, applying missing→"unknown"
// defaults recursively (Key fields, Datum fields, Evidence Label/Source and
// Quota). Progress has no unknown value in its scale, so a missing or
// out-of-scale Progress fails validation. Syntax errors name the offset;
// out-of-scale enum values name the offending field. Any error returns the
// zero Record: unknown input never coerces to a passing value.
func DecodeRecord(data []byte) (Record, error) {
	var r Record
	if err := json.Unmarshal(data, &r); err != nil {
		if syntaxErr, ok := errors.AsType[*json.SyntaxError](err); ok {
			return Record{}, fmt.Errorf("qualify: invalid record JSON at offset %d: %w", syntaxErr.Offset, err)
		}
		if typeErr, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
			return Record{}, fmt.Errorf("qualify: invalid record JSON at offset %d: %w", typeErr.Offset, err)
		}
		return Record{}, fmt.Errorf("qualify: invalid record JSON: %w", err)
	}
	applyDefaults(&r)
	if err := validateRecord(r); err != nil {
		return Record{}, err
	}
	return r, nil
}

// validateRecord rejects any enum value outside its honest-label scale,
// naming the offending field.
func validateRecord(r Record) error {
	if !validProgress(r.Progress) {
		return fmt.Errorf("qualify: out-of-scale Progress %q", r.Progress)
	}
	for _, col := range []struct {
		name string
		col  Column
	}{
		{"Fidelity", r.Fidelity},
		{"Entitlement", r.Entitlement},
		{"Lifecycle", r.Lifecycle},
	} {
		if !validVerdict(col.col.Verdict) {
			return fmt.Errorf("qualify: out-of-scale %s.Verdict %q", col.name, col.col.Verdict)
		}
		for i := range col.col.Evidence {
			if !validDatumLabel(col.col.Evidence[i].Label) {
				return fmt.Errorf("qualify: out-of-scale %s.Evidence[%d].Label %q",
					col.name, i, col.col.Evidence[i].Label)
			}
		}
	}
	if !validDatumLabel(r.Quota.Label) {
		return fmt.Errorf("qualify: out-of-scale Quota.Label %q", r.Quota.Label)
	}
	for name, c := range r.Capabilities {
		if !validCapabilityValue(c.Value) {
			return fmt.Errorf("qualify: out-of-scale Capabilities[%q].Value %q", name, c.Value)
		}
	}
	return nil
}

// validProgress reports whether p is in the v2 §4.2 progress scale.
func validProgress(p Progress) bool {
	switch p {
	case ProgressPlanned, ProgressDocumentedCandidate, ProgressFixtureTested,
		ProgressLiveQualified, ProgressExperimental, ProgressBlocked,
		ProgressUnsupported:
		return true
	}
	return false
}

// validVerdict reports whether v is a column verdict (AC-1.2).
func validVerdict(v Verdict) bool {
	switch v {
	case Proven, NotProven, Unknown:
		return true
	}
	return false
}

// validDatumLabel reports whether l is a v2 §7.3 provenance label.
func validDatumLabel(l DatumLabel) bool {
	switch l {
	case Reported, Observed, Estimated, UserDeclared, DatumUnknown:
		return true
	}
	return false
}

// validCapabilityValue reports whether v is a v2 §4.2 capability value.
func validCapabilityValue(v string) bool {
	switch v {
	case "supported", "unsupported", "unknown":
		return true
	}
	return false
}

// applyDefaults fills missing values with the unknown marker (I09).
func applyDefaults(r *Record) {
	k := &r.Key
	setUnknown(&k.Harness)
	setUnknown(&k.Surface)
	setUnknown(&k.ExecutableDigest)
	setUnknown(&k.AdapterProtocol)
	setUnknown(&k.OS)
	setUnknown(&k.Arch)
	setUnknown(&k.ProviderEndpoint)
	setUnknown(&k.ModelSnapshot)
	setUnknown(&k.EffortSettings)
	setUnknown(&k.AuthCategory)
	setUnknown(&k.ConfigDigest)
	setUnknown(&k.TrustProfile)
	setUnknown(&k.WorkspaceClass)
	setUnknown(&k.EntitlementClass)

	defaultColumn(&r.Fidelity)
	defaultColumn(&r.Entitlement)
	defaultColumn(&r.Lifecycle)
	defaultDatum(&r.Quota)
	for name, c := range r.Capabilities {
		setUnknown(&c.Value)
		setUnknown(&c.Evidence)
		setUnknown(&c.Scope)
		r.Capabilities[name] = c
	}
}

// setUnknown replaces an empty marker with the unknown value.
func setUnknown(s *string) {
	if *s == "" {
		*s = unknownString
	}
}

// normalizeKeyForHash returns k with every empty field set to "unknown",
// matching what DecodeRecord would default. KeyHash hashes this form so
// hand-built and decoded keys agree.
func normalizeKeyForHash(k Key) Key {
	setUnknown(&k.Harness)
	setUnknown(&k.Surface)
	setUnknown(&k.ExecutableDigest)
	setUnknown(&k.AdapterProtocol)
	setUnknown(&k.OS)
	setUnknown(&k.Arch)
	setUnknown(&k.ProviderEndpoint)
	setUnknown(&k.ModelSnapshot)
	setUnknown(&k.EffortSettings)
	setUnknown(&k.AuthCategory)
	setUnknown(&k.ConfigDigest)
	setUnknown(&k.TrustProfile)
	setUnknown(&k.WorkspaceClass)
	setUnknown(&k.EntitlementClass)
	return k
}

// normalizeForHash returns r with the non-enum canonicalization
// DecodeRecord applies: nil Evidence slices and a nil Capabilities map
// become empty, and empty free-text fields become "unknown". Enum
// validation runs before this (CanonicalDigest validates first), so
// out-of-scale values still error instead of hashing silently.
func normalizeForHash(r Record) Record {
	r.Key = normalizeKeyForHash(r.Key)
	for _, c := range []*Column{&r.Fidelity, &r.Entitlement, &r.Lifecycle} {
		if c.Evidence == nil {
			c.Evidence = []Evidence{}
		}
		for i := range c.Evidence {
			e := &c.Evidence[i]
			setUnknown(&e.Source)
			setUnknown(&e.Suite)
			setUnknown(&e.Uncertainty)
		}
	}
	if r.Capabilities == nil {
		r.Capabilities = map[string]Capability{}
	}
	for name, c := range r.Capabilities {
		setUnknown(&c.Evidence)
		setUnknown(&c.Scope)
		r.Capabilities[name] = c
	}
	setUnknown(&r.Quota.Quantity)
	setUnknown(&r.Quota.Unit)
	setUnknown(&r.Quota.Scope)
	setUnknown(&r.Quota.Source)
	return r
}

// defaultColumn defaults an empty verdict and its evidence entries.
func defaultColumn(c *Column) {
	if c.Verdict == "" {
		c.Verdict = Unknown
	}
	for i := range c.Evidence {
		defaultEvidence(&c.Evidence[i])
	}
}

// defaultEvidence fills unassessed evidence fields with unknown markers.
func defaultEvidence(e *Evidence) {
	if e.Label == "" {
		e.Label = DatumUnknown
	}
	setUnknown(&e.Source)
	setUnknown(&e.Suite)
	setUnknown(&e.Uncertainty)
}

// defaultDatum fills an unestablished datum with unknown markers.
func defaultDatum(d *Datum) {
	if d.Label == "" {
		d.Label = DatumUnknown
	}
	setUnknown(&d.Quantity)
	setUnknown(&d.Unit)
	setUnknown(&d.Scope)
	setUnknown(&d.Source)
}
