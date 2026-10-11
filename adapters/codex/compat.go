package codex

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/turbokast/mythhelm/internal/adapter"
)

// ErrCapability is a version/platform refusal, mapped to CLI exit 7.
var ErrCapability = errors.New("native capability unavailable")

// fixtureVersion is the synthetic version the decoder fixtures stand for.
// It is not a released Codex version: the real baseline waits on OQ1.
const fixtureVersion = "0.0.0"

func compatibility(version string, allowUntested bool) (string, error) {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("%w: native_version_unrecognised", ErrCapability)
	}
	for _, s := range parts {
		if n, err := strconv.Atoi(s); err != nil || n < 0 {
			return "", fmt.Errorf("%w: native_version_unrecognised", ErrCapability)
		}
	}
	if version == fixtureVersion {
		return "fixture-tested on " + fixtureVersion, nil
	}
	if allowUntested {
		return "experimental: fixture-tested on " + fixtureVersion + "; running " + version + " untested", nil
	}
	return "", fmt.Errorf("%w: native_version_untested (%s): use --allow-untested-native-version only after reviewing compatibility", ErrCapability, version)
}

// Capabilities reports nothing as supported: a fixture proves only
// MYTHHELM's own parsing, so every native fact stays unknown (I09).
func (*codexAdapter) Capabilities(p adapter.Probe) adapter.CapabilityRecord {
	platform := adapter.Platform{OS: p.OS, WorkerDetachment: adapter.Unknown, ProcessTreeOwnership: adapter.Unknown}
	switch p.OS {
	case "linux", "darwin":
		platform.WorkerDetachment, platform.ProcessTreeOwnership = adapter.Supported, adapter.Supported
	}
	d := descriptor()
	return adapter.CapabilityRecord{
		SchemaVersion: 1, AdapterID: AdapterID, AdapterVersion: d.Version, RuntimeVersion: p.Version, Mode: "headless", ExecutionSurface: d.Surface, HarnessID: d.Harness,
		FidelityQualification: "native executable; decoder validated against synthetic fixtures only",
		Billing:               adapter.BillingCapabilities{Entitlement: "not established", IncludedOnlySupported: adapter.Unknown, PaidOveragePrevention: adapter.Unknown},
		HostIntegration:       adapter.HostIntegration{HerdrEmbedded: adapter.Unsupported, HerdrStateBridge: adapter.Unsupported, NativeInteractiveAttach: adapter.Unsupported},
		Capabilities: adapter.Capabilities{
			StructuredEvents: adapter.Unknown, Resume: adapter.Unknown, LiveSteer: adapter.Unknown, ApprovalBridge: adapter.Unknown, UsageTokens: adapter.Unknown,
			QuotaRemaining: adapter.Unknown, HardMonetaryLimit: adapter.Unknown, NativeSubagents: adapter.Unknown, Reconnect: adapter.Unknown, ModelMetadata: adapter.Unknown,
		},
		Sandbox: adapter.Sandbox{Status: adapter.Unsupported, Scope: "trusted-host: not contained"}, Platform: platform, Qualification: p.Compatibility + "; planned",
	}
}
