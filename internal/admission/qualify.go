package admission

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/turbokast/mythhelm/adapters/claudecode"
	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/qualify"
)

// EligibilityVerdict is the outcome of a qualification consult (v2 §4.2).
type EligibilityVerdict string

// Qualification consult verdicts.
const (
	Eligible    EligibilityVerdict = "eligible"
	Blocked     EligibilityVerdict = "blocked"
	Unsupported EligibilityVerdict = "unsupported"
)

// Eligibility is the consult outcome: the verdict, the reason behind a
// refusal (empty when eligible), and the record consulted (nil when no
// record was consulted).
type Eligibility struct {
	Verdict EligibilityVerdict
	Reason  string
	Record  *qualify.Record
}

// Consult refusal reasons. Drift refusals carry the CheckDrift reason text
// instead of one of these codes.
const (
	reasonNoRecord       = "no_qualification_record"
	reasonEntitlementUnp = "entitlement_not_proven"
	reasonAmbiguousMatch = "ambiguous_qualification_match"
	reasonStopUnproven   = "stop_at_exhaustion_unproven"
	reasonUnsupported    = "qualification_unsupported"
)

// stopAtExhaustion is the AC-3.3 capability: exhaustion reliably waits or
// stops rather than charges.
const stopAtExhaustion = "stop_at_exhaustion"

// knownHarnesses is the v2 §7.2 candidate set, tracking qualify.SeedV1.
var knownHarnesses = map[string]bool{
	"claude-code": true,
	"codex":       true,
	"opencode":    true,
	"muse":        true,
	"kimi":        true,
	"cursor":      true,
	"antigravity": true,
}

// ResolveQualification maps a probed native identity plus the registry to an
// eligibility verdict. A nil registry (missing dir or database, or an
// unreadable schema) counts as an absent record; evidence is the AuthStatus
// result, since the consult point follows it. profile is the consented
// execution profile name (d.Profile.Name); the resolver cannot see the owning
// decision, so the caller passes it.
//
// For subscription-only, an absent record blocks, an ambiguous stable match
// blocks, and a found record admits only when live-qualified with a proven
// entitlement (supported by unexpired evidence) and a supported
// stop-at-exhaustion marker; unknown quota alone never blocks (AC-3.3).
// Drifted records block with the drift reason. For subscription-declared and
// local-scripted, missing and non-live records admit with the record attached
// when one exists; only an unsupported surface refuses under every mode.
func ResolveQualification(ctx context.Context, reg *qualify.Registry, probe adapter.Probe, manifest adapter.ConfigManifest, evidence claudecode.AuthEvidence, billing string, profile string) (Eligibility, error) {
	switch billing {
	case BillingSubscriptionOnly, BillingSubscriptionDeclared, BillingLocalScripted:
	default:
		return Eligibility{}, fmt.Errorf("%w: unknown billing mode %q", ErrInvalid, billing)
	}
	observed := observedKey(probe, manifest, evidence, profile)
	if !knownHarnesses[observed.Harness] {
		return Eligibility{Verdict: Unsupported, Reason: reasonUnsupported}, nil
	}
	strict := billing == BillingSubscriptionOnly
	absent := Eligibility{Verdict: Eligible}
	if strict {
		absent = Eligibility{Verdict: Blocked, Reason: reasonNoRecord}
	}
	if reg == nil {
		return absent, nil
	}
	rec, drifted, reason, err := reg.Consult(ctx, observed)
	switch {
	case errors.Is(err, qualify.ErrNotFound):
		return absent, nil
	case err != nil && errors.Is(err, qualify.ErrAmbiguousMatch):
		if strict {
			return Eligibility{Verdict: Blocked, Reason: reasonAmbiguousMatch}, nil
		}
		return Eligibility{Verdict: Eligible}, nil
	case err != nil:
		return Eligibility{}, fmt.Errorf("qualify: registry unavailable: %w", err)
	}
	if rec.Progress == qualify.ProgressUnsupported || !knownHarnesses[rec.Key.Harness] {
		return Eligibility{Verdict: Unsupported, Reason: reasonUnsupported, Record: &rec}, nil
	}
	if drifted {
		if strict {
			return Eligibility{Verdict: Blocked, Reason: reason, Record: &rec}, nil
		}
		return Eligibility{Verdict: Eligible, Record: &rec}, nil
	}
	if !strict {
		return Eligibility{Verdict: Eligible, Record: &rec}, nil
	}
	now := time.Now()
	if rec.Progress != qualify.ProgressLiveQualified || !columnProven(rec.Entitlement, now) {
		return Eligibility{Verdict: Blocked, Reason: reasonEntitlementUnp, Record: &rec}, nil
	}
	if !stopSupported(rec.Capabilities, now) {
		return Eligibility{Verdict: Blocked, Reason: reasonStopUnproven, Record: &rec}, nil
	}
	return Eligibility{Verdict: Eligible, Record: &rec}, nil
}

// OpenQualificationRegistry opens the state dir's registry for an admission
// consult without migrating or seeding. A missing dir or database, or a
// schema this binary cannot read, maps to a nil registry, which
// ResolveQualification treats as an absent record. Any other open failure is
// returned, failing the admission instead of silently admitting it.
func OpenQualificationRegistry(ctx context.Context, dir string) (*qualify.Registry, error) {
	reg, err := qualify.OpenReadOnly(ctx, dir)
	if err != nil {
		if qualify.IsMissing(err) || qualify.IsSchemaMismatch(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("qualify: registry unavailable: %w", err)
	}
	return reg, nil
}

// observedKey builds the qualification key for the probed identity per the
// design §4 table: harness, surface and protocol from the adapter descriptor,
// digests from the probe and manifest, endpoint and auth class from the
// AuthStatus evidence.
func observedKey(probe adapter.Probe, manifest adapter.ConfigManifest, evidence claudecode.AuthEvidence, profile string) qualify.Key {
	desc := claudecode.New().Descriptor()
	endpoint := "unknown"
	if evidence.APIProvider == "firstParty" {
		endpoint = "first-party-subscription"
	}
	return qualify.Key{
		Harness:          desc.Harness,
		Surface:          desc.Surface,
		ExecutableDigest: "sha256:" + probe.SHA256,
		AdapterProtocol:  desc.ID + "+stream-json",
		OS:               probe.OS,
		Arch:             probe.Arch,
		ProviderEndpoint: endpoint,
		ModelSnapshot:    "unknown",
		EffortSettings:   "none",
		AuthCategory:     evidence.AuthMethod + "/" + evidence.SubscriptionType,
		ConfigDigest:     qualify.ConfigDigestOf(manifest.Digests),
		TrustProfile:     profile,
		WorkspaceClass:   "local-checkout",
		EntitlementClass: "included-plan",
	}
}

// columnProven reports whether the column counts as proven for the consult: a
// proven verdict with at least one unexpired supporting evidence entry.
// Evidence with past Expiry is ignored, treated as absent.
func columnProven(col qualify.Column, now time.Time) bool {
	if col.Verdict != qualify.Proven {
		return false
	}
	for _, e := range col.Evidence {
		if e.Expiry == nil || e.Expiry.After(now) {
			return true
		}
	}
	return false
}

// stopSupported reports whether the stop-at-exhaustion capability counts as
// supported for the consult. A missing entry, an expired entry, or any value
// but supported fails; expiry counts as unknown whatever Value claims.
func stopSupported(caps map[string]qualify.Capability, now time.Time) bool {
	capability, ok := caps[stopAtExhaustion]
	if !ok || capability.Value != "supported" {
		return false
	}
	return capability.Expiry == nil || capability.Expiry.After(now)
}
