package codex

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/adapter"
)

func TestCompatibilityUntestedBlocksWithoutOverride(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		version      string
		allowUntried bool
		want         string
		wantErr      string
	}{
		{name: "the fixture version is fixture-tested", version: fixtureVersion, want: "fixture-tested on " + fixtureVersion},
		{name: "an untested version blocks without the override", version: "0.0.1", wantErr: "native_version_untested"},
		{name: "a newer major blocks without the override", version: "1.0.0", wantErr: "native_version_untested"},
		{name: "the override labels an untested version experimental", version: "0.0.1", allowUntried: true, want: "experimental: fixture-tested on " + fixtureVersion + "; running 0.0.1 untested"},
		{name: "an unrecognised version blocks even with the override", version: "0.0", allowUntried: true, wantErr: "native_version_unrecognised"},
		{name: "a non-numeric version blocks even with the override", version: "a.b.c", allowUntried: true, wantErr: "native_version_unrecognised"},
		{name: "a negative component blocks even with the override", version: "0.-1.0", allowUntried: true, wantErr: "native_version_unrecognised"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := compatibility(tt.version, tt.allowUntried)
			if tt.wantErr != "" {
				if !errors.Is(err, ErrCapability) || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("compatibility(%q) = %q, %v; want ErrCapability containing %q", tt.version, got, err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("compatibility(%q) = %q, %v; want %q", tt.version, got, err, tt.want)
			}
		})
	}
}

func TestCapabilitiesUnknownNeverDefaultSupported(t *testing.T) { // I09 (v2 §2)
	t.Parallel()
	named := []string{"StructuredEvents", "Resume", "LiveSteer", "ApprovalBridge", "UsageTokens", "Reconnect", "ModelMetadata"}
	for _, goos := range []string{"linux", "darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			t.Parallel()
			rec := New().Capabilities(adapter.Probe{OS: goos, Version: fixtureVersion, Compatibility: "fixture-tested on " + fixtureVersion})
			caps := reflect.ValueOf(rec.Capabilities)
			for _, name := range named {
				f := caps.FieldByName(name)
				if !f.IsValid() {
					t.Fatalf("adapter.Capabilities has no field %s", name)
				}
				if got := adapter.Tri(f.String()); got != adapter.Unknown {
					t.Errorf("Capabilities.%s = %q, want %q", name, got, adapter.Unknown)
				}
			}
			for i := range caps.NumField() {
				if got := adapter.Tri(caps.Field(i).String()); got != adapter.Unknown {
					t.Errorf("Capabilities.%s = %q, want %q: nothing is established offline", caps.Type().Field(i).Name, got, adapter.Unknown)
				}
			}
			if rec.Billing.IncludedOnlySupported != adapter.Unknown || rec.Billing.PaidOveragePrevention != adapter.Unknown {
				t.Errorf("billing = %+v, want included-only and paid-overage prevention unknown", rec.Billing)
			}
		})
	}
}

func TestCapabilitiesRecordIdentityAndPlatform(t *testing.T) {
	t.Parallel()
	tests := []struct {
		goos string
		want adapter.Tri
	}{
		{"linux", adapter.Supported},
		{"darwin", adapter.Supported},
		{"windows", adapter.Unknown},
		{"freebsd", adapter.Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			t.Parallel()
			rec := New().Capabilities(adapter.Probe{OS: tt.goos, Version: fixtureVersion, Compatibility: "fixture-tested on " + fixtureVersion})
			if rec.Platform.OS != tt.goos || rec.Platform.WorkerDetachment != tt.want || rec.Platform.ProcessTreeOwnership != tt.want {
				t.Fatalf("platform = %+v, want %s detachment and ownership on %s", rec.Platform, tt.want, tt.goos)
			}
			if rec.AdapterID != AdapterID || rec.HarnessID != "codex" || rec.RuntimeVersion != fixtureVersion || rec.ExecutionSurface != New().Descriptor().Surface {
				t.Fatalf("identity = %+v", rec)
			}
			if !strings.HasPrefix(rec.Qualification, "fixture-tested on "+fixtureVersion) || !strings.Contains(rec.Qualification, "planned") {
				t.Fatalf("qualification = %q, want the compatibility label and planned", rec.Qualification)
			}
		})
	}
}
