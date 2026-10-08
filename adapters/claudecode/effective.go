package claudecode

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/turbokast/mythhelm/internal/adapter"
)

// EffectiveConfig is the established effective configuration for one probed
// route. Static inputs carry no funding, extra-usage or credit signal, so
// ExtraUsage and PurchasedCredits are "unknown" here; only the authorised
// live suite assesses them.
type EffectiveConfig struct {
	CredentialPrecedence []string          `json:"credential_precedence"`
	ManagedPolicy        PolicySummary     `json:"managed_policy"`
	ChildrenRoutes       []string          `json:"children_routes"`
	ExtraUsage           string            `json:"extra_usage"`
	PurchasedCredits     string            `json:"purchased_credits"`
	Sources              map[string]string `json:"sources"`
}

// PolicySummary is the effective managed policy relevant to billing. Gaps
// lists applicable sources the file inventory cannot certify (ADR-0002).
type PolicySummary struct {
	Digest  string   `json:"digest"`
	Sources []string `json:"sources"`
	Gaps    []string `json:"gaps"`
}

// UnresolvedSources lists the managed-policy sources no inventory covers.
// Removing one requires new inventory code or a scoping rationale amending
// ADR-0002.
var UnresolvedSources = []string{"managed-remote-cache", "macos-mdm-policy"}

// GapSources lists the unresolved sources that apply on this platform, in the
// order of the unresolved input. The remote cache location is unknown, so its
// absence is unprovable; MDM policy applies on darwin only.
func GapSources(unresolved []string) []string {
	gaps := []string{}
	for _, name := range unresolved {
		switch {
		case name == "managed-remote-cache", name == "macos-mdm-policy" && runtime.GOOS == "darwin":
			gaps = append(gaps, name)
		}
	}
	return gaps
}

// InventoryEffective builds the effective configuration from a probe,
// manifest and auth evidence. It makes no live call and reads no secret. A gap
// is returned as data, never as an error.
func InventoryEffective(p adapter.Probe, m Manifest, ev AuthEvidence) (EffectiveConfig, error) {
	if !filepath.IsAbs(p.Executable) {
		return EffectiveConfig{}, fmt.Errorf("%w: native_executable_not_absolute", ErrCapability)
	}
	if !ev.LoggedIn || ev.AuthMethod != "claude.ai" {
		return EffectiveConfig{}, fmt.Errorf("%w: credential_route_unrecognised", ErrCapability)
	}
	managed := map[string]string{}
	for name, digest := range m.Digests {
		if strings.HasPrefix(name, "managed") {
			managed[name] = digest
		}
	}
	names := make([]string, 0, len(managed))
	for name := range managed {
		names = append(names, name)
	}
	slices.Sort(names)
	encoded, err := json.Marshal(managed)
	if err != nil {
		return EffectiveConfig{}, fmt.Errorf("managed policy digest: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return EffectiveConfig{
		CredentialPrecedence: []string{"native-login"},
		ManagedPolicy:        PolicySummary{Digest: hex.EncodeToString(sum[:]), Sources: names, Gaps: GapSources(UnresolvedSources)},
		ChildrenRoutes:       []string{},
		ExtraUsage:           "unknown",
		PurchasedCredits:     "unknown",
		Sources: map[string]string{
			"credential_precedence": "auth-status",
			"managed_policy":        "settings-manifest",
			"children_routes":       "unknown",
			"extra_usage":           "unknown",
			"purchased_credits":     "unknown",
		},
	}, nil
}
