package contain

import (
	"errors"
	"slices"
	"testing"
)

var allDimensions = []Dimension{DimFilesystem, DimProcess, DimNetwork, DimCredential}

func enforced(name string) Claim {
	return Claim{Name: name, Version: "1", Enforced: true}
}

func TestCoverageMissingAggregates(t *testing.T) {
	t.Parallel()

	full := Coverage{
		Filesystem: enforced("fs"),
		Process:    enforced("proc"),
		Network:    enforced("net"),
		Credential: enforced("cred"),
	}
	partial := full
	partial.Network = Claim{Name: "net", Version: "1", Enforced: false, Detail: "direct egress open"}
	partial.Credential = Claim{}

	tests := []struct {
		name     string
		coverage Coverage
		required []Dimension
		want     []Dimension
	}{
		{"unenforced network and unknown credential are both reported", partial, allDimensions, []Dimension{DimNetwork, DimCredential}},
		{"all enforced reports none", full, allDimensions, nil},
		{"only required dimensions are reported", partial, []Dimension{DimFilesystem, DimNetwork}, []Dimension{DimNetwork}},
		{"required order is preserved", partial, []Dimension{DimCredential, DimNetwork}, []Dimension{DimCredential, DimNetwork}},
		{"zero coverage misses every required dimension", Coverage{}, allDimensions, allDimensions},
		{"unknown dimension is missing, never assumed enforced", full, []Dimension{"memory"}, []Dimension{"memory"}},
		{"nothing required misses nothing", Coverage{}, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.coverage.Missing(tt.required)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("Missing(%v) = %v, want %v", tt.required, got, tt.want)
			}
		})
	}
}

type mapRegistry map[[3]string]Evidence

func (r mapRegistry) Lookup(profile, os, route string) (Evidence, bool) {
	e, ok := r[[3]string{profile, os, route}]
	return e, ok
}

func TestRegistryUnknownRefuses(t *testing.T) {
	t.Parallel()

	known := Evidence{
		Profile:  "restricted",
		OS:       "linux",
		Route:    "fake",
		Boundary: "mythhelm-restricted/1",
		Version:  "1",
		Owner:    "mythhelm",
		Coverage: Coverage{Filesystem: enforced("fs")},
	}
	var reg Registry = mapRegistry{{"restricted", "linux", "fake"}: known}

	got, ok := reg.Lookup("restricted", "linux", "fake")
	if !ok || got.Boundary != known.Boundary {
		t.Fatalf("Lookup(known) = %+v, %v; want the recorded evidence (control: the double must be able to answer ok)", got, ok)
	}

	got, ok = reg.Lookup("restricted", "darwin", "fake")
	if ok {
		t.Fatalf("Lookup(restricted/darwin/fake) ok = true, want false")
	}
	if got.Boundary != "" || got.Coverage.Filesystem.Enforced {
		t.Fatalf("unknown lookup returned a non-zero record %+v; it must not look supported", got)
	}
}

func TestSentinelErrorsAreDistinct(t *testing.T) {
	t.Parallel()

	if errors.Is(ErrUnsupported, ErrMissingCoverage) || errors.Is(ErrMissingCoverage, ErrUnsupported) {
		t.Fatal("ErrUnsupported and ErrMissingCoverage must be distinguishable with errors.Is")
	}
}
