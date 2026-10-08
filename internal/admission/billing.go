package admission

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/turbokast/mythhelm/adapters/claudecode"
	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/qualify"
	"github.com/turbokast/mythhelm/internal/security"
)

// NativeConfigTrustKind is the trust-grant kind for native configuration digests.
const NativeConfigTrustKind = "native_config"

// AuthEvidence is the native login evidence admission evaluates.
type AuthEvidence = claudecode.AuthEvidence

// Declaration is an entitlement declaration row from the journal.
type Declaration = journal.DeclarationRow

// BillingPosture is the billing route admission recorded (§6.2, FR-4).
type BillingPosture = adapter.BillingPosture

// ResolveBilling never upgrades a user declaration into verified entitlement
// or overage prevention. Strict subscription-only admits only on an eligible
// consult of a live-qualified record with a proven entitlement (I15/G05); it
// ignores any declaration.
func ResolveBilling(ctx context.Context, mode string, evidence AuthEvidence, decl *Declaration, elig Eligibility) (BillingPosture, error) {
	if err := ctx.Err(); err != nil {
		return BillingPosture{}, err
	}
	if mode == BillingSubscriptionOnly {
		rec := elig.Record
		if elig.Verdict != Eligible || rec == nil || rec.Progress != qualify.ProgressLiveQualified || rec.Entitlement.Verdict != qualify.Proven {
			return BillingPosture{}, strictBillingBlock()
		}
		return BillingPosture{Mode: mode, CredentialProvenance: "native-login", EntitlementClass: "included-plan", EntitlementSource: "registry:live-qualified", PaidContinuation: "prevented", Qualified: true, G05: "passed"}, nil
	}
	if mode != BillingSubscriptionDeclared {
		return BillingPosture{}, fmt.Errorf("%w: Claude Code requires --billing subscription-declared or subscription-only", ErrInvalid)
	}
	if !evidence.LoggedIn || evidence.AuthMethod != "claude.ai" || evidence.APIProvider != "firstParty" || !slices.Contains([]string{"pro", "max", "team", "enterprise"}, evidence.SubscriptionType) || evidence.ConfigDirectory == "" || len(evidence.IdentityRef) != 64 {
		return BillingPosture{}, &BlockedError{Code: "needs_native_setup", Action: "run 'claude' and /login with your subscription"}
	}
	if decl == nil {
		return BillingPosture{}, &BlockedError{Code: "entitlement_declaration_required", Field: "--declare-entitlement", Action: "declare plan=<pro|max|team|enterprise>,extra-usage=disabled after checking your account"}
	}
	if decl.AdapterID != claudecode.AdapterID || decl.IdentityRef != evidence.IdentityRef {
		return BillingPosture{}, &BlockedError{Code: "declaration_identity_mismatch", Action: "account/config identity changed; make a fresh entitlement declaration"}
	}
	if !slices.Contains([]string{"pro", "max", "team", "enterprise"}, decl.PlanClass) || decl.ExtraUsage != "disabled" || decl.DeclaredAt.IsZero() {
		return BillingPosture{}, fmt.Errorf("%w: invalid entitlement declaration", ErrInvalid)
	}
	return BillingPosture{Mode: mode, CredentialProvenance: "native-login", EntitlementClass: "included-plan", EntitlementSource: "user_declared+native_status(subscriptionType=" + evidence.SubscriptionType + ")", PaidContinuation: "unknown", PaidContinuationUserDeclaration: "disabled", Qualified: false, G05: "not-passed"}, nil
}

func strictBillingBlock() error {
	return &BlockedError{Code: "entitlement_qualification_unavailable", Field: "--billing subscription-only", Action: "strict subscription-only admits only a live-qualified record with a proven entitlement for this exact route"}
}

// ResolveCredentialEnv previews child-only credential-route removals. It
// examines names, never credential values. oauthOptIn is a trusted user-level
// choice; project configuration must never set it (AC-4.7). Trusted
// passthrough names are applied without restoring denied credential routes.
func ResolveCredentialEnv(parent []string, strip, oauthOptIn bool, passthrough []string) ([]string, []adapter.ConfigDelta, error) {
	names := claudecode.CredentialOverrideNames()
	var found []string
	var oauth string // opaque complete env entry; never decoded or persisted
	for _, kv := range parent {
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if !slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(name, n) }) {
			continue
		}
		if name == "CLAUDE_CODE_OAUTH_TOKEN" && oauthOptIn {
			oauth = kv
			continue
		}
		found = append(found, strings.ToUpper(name))
	}
	slices.Sort(found)
	found = slices.Compact(found)
	if len(found) > 0 && !strip {
		return nil, nil, &BlockedError{Code: "credential_route_override", Field: strings.Join(found, ", "), Action: "remove these variables or pass --strip-credential-env to remove them from this child only"}
	}
	child, err := security.BuildEnv(parent, passthrough, nil)
	if err != nil {
		return nil, nil, err
	}
	if oauth != "" {
		child = append(child, oauth)
	}
	overrides := make([]adapter.ConfigDelta, 0, len(found))
	for _, name := range found {
		overrides = append(overrides, adapter.ConfigDelta{Kind: "env_remove", Name: name, Reason: "--strip-credential-env: remove credential-route override from native child only"})
	}
	return child, overrides, nil
}

// CheckNativeTrust checks a digest-bound native execution grant. True means
// the caller must persist the explicit grant atomically with its admission.
func CheckNativeTrust(ctx context.Context, j *journal.Journal, repoID string, manifest claudecode.Manifest, explicitDigest string) (bool, error) {
	if !manifest.RequiresTrust {
		if explicitDigest != "" {
			return false, fmt.Errorf("%w: --trust-native-config has no executable configuration to trust", ErrInvalid)
		}
		return false, nil
	}
	if len(manifest.Digest) != 64 || repoID == "" {
		return false, &BlockedError{Code: "native_config_unreadable", Action: "rebuild the native configuration inventory"}
	}
	if explicitDigest != "" {
		if explicitDigest != "sha256:"+manifest.Digest {
			return false, fmt.Errorf("%w: --trust-native-config must match sha256:%s", ErrInvalid, manifest.Digest)
		}
		return true, nil
	}
	trusted, err := HasTrust(ctx, j, NativeConfigTrustKind, repoID, manifest.Digest)
	if err != nil {
		return false, err
	}
	if trusted {
		return false, nil
	}
	return false, &BlockedError{Code: "untrusted_native_config", Field: "sha256:" + manifest.Digest, Action: "review hooks/MCP definitions and pass --trust-native-config sha256:" + manifest.Digest}
}

// NativeAdmissionError translates adapter refusals into the shared CLI error
// contract. A native capability refusal is exit 7; policy remains exit 3.
func NativeAdmissionError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, claudecode.ErrCapability) {
		return &BlockedError{Code: "native_capability_unavailable", Capability: true, Action: err.Error()}
	}
	if blocked, ok := errors.AsType[*adapter.BlockedError](err); ok {
		return &BlockedError{Code: blocked.Code, Field: blocked.Field, Action: blocked.Action}
	}
	return err
}
