package admission_test

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/contain"
)

var allCoverage = []string{"filesystem", "process", "network", "credential"}

func TestRefusalNamesEachMissingCoverage(t *testing.T) {
	for _, tc := range []struct{ name, os, route string }{
		{"windows is not qualified", "windows", "builtin/fake"},
		{"darwin is not qualified", "darwin", "builtin/fake"},
		{"an unknown route is not assumed", "linux", "builtin/unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := admission.BoundaryConsult("restricted", tc.os, tc.route)
			var blocked *admission.BlockedError
			if !errors.As(err, &blocked) || !blocked.Capability || blocked.Code != "execution_profile_unavailable" {
				t.Fatalf("err = %v, want a capability (exit 7) execution_profile_unavailable refusal", err)
			}
			for _, dim := range allCoverage {
				if !strings.Contains(err.Error(), dim) {
					t.Errorf("refusal %q does not name %s", err, dim)
				}
			}
		})
	}

	// A combination the registry has no record for is also ErrMissingCoverage,
	// never a zero-value supported record (I02).
	if _, err := admission.BoundaryConsult("restricted", "plan9", "builtin/fake"); !errors.Is(err, contain.ErrMissingCoverage) {
		t.Errorf("unknown combination: err = %v, want ErrMissingCoverage", err)
	}
}

func TestBoundaryConsultNamesOnlyMissingDimensions(t *testing.T) {
	// The Claude Code route has no native auth inside the boundary yet.
	_, err := admission.BoundaryConsult("restricted", "linux", "builtin/claudecode")
	if _, ok := errors.AsType[*admission.BlockedError](err); !ok {
		t.Fatalf("err = %v, want a refusal", err)
	}
	if !strings.Contains(err.Error(), "credential") {
		t.Errorf("refusal %q does not name credential", err)
	}
	for _, dim := range []string{"filesystem", "process", "network"} {
		if strings.Contains(err.Error(), dim) {
			t.Errorf("refusal %q names enforced dimension %s", err, dim)
		}
	}
}

func TestSeededEvidenceIsVersionedAndOwned(t *testing.T) {
	seed := contain.SeedV1()
	if len(seed) == 0 {
		t.Fatal("no seeded evidence")
	}
	for key, ev := range seed {
		if want := ev.Profile + "/" + ev.OS + "/" + ev.Route; key != want {
			t.Errorf("record keyed %q describes %q", key, want)
		}
		if ev.Boundary == "" || ev.Version == "" || ev.Owner == "" {
			t.Errorf("%s: boundary %q, version %q, owner %q must all be set (I14)", key, ev.Boundary, ev.Version, ev.Owner)
		}
		for name, c := range map[string]contain.Claim{
			"filesystem": ev.Coverage.Filesystem, "process": ev.Coverage.Process,
			"network": ev.Coverage.Network, "credential": ev.Coverage.Credential,
		} {
			if c.Name == "" || c.Version == "" {
				t.Errorf("%s: %s claim lacks name or version: %+v", key, name, c)
			}
		}
	}
	ev, err := admission.BoundaryConsult("restricted", "linux", "builtin/fake")
	if err != nil {
		t.Fatalf("linux fake route: %v", err)
	}
	if missing := ev.Coverage.Missing([]contain.Dimension{contain.DimFilesystem, contain.DimProcess, contain.DimNetwork, contain.DimCredential}); missing != nil {
		t.Errorf("supported record misses %v", missing)
	}
	if !strings.Contains(ev.Coverage.Network.Detail, "direct egress not blocked") {
		t.Errorf("network claim %+v does not disclose the direct-egress residual (I09)", ev.Coverage.Network)
	}
}

func decideProfile(t *testing.T, profile string, confirm func(string) (bool, error)) (admission.Decision, error) {
	t.Helper()
	return decideProfileProbed(t, profile, confirm, availableProbe)
}

const probedVersion = "mythhelm-restricted/1 linux 6.8.0-synthetic"

func availableProbe() contain.Availability {
	return contain.Availability{Supported: true, Version: probedVersion}
}

func decideProfileProbed(t *testing.T, profile string, confirm func(string) (bool, error), probe func() contain.Availability) (admission.Decision, error) {
	t.Helper()
	repo, task := envelopeRepo(t, "")
	_, digest, err := admission.ParseProjectConfig(envelopesConfig(""))
	if err != nil {
		t.Fatal(err)
	}
	return admission.Decide(t.Context(), admission.Request{
		StateDir: t.TempDir(), Repo: repo, TaskFile: task, Adapter: admission.AdapterFake,
		Billing: admission.BillingLocalScripted, ExecutionProfile: profile,
		TrustProjectConfig: "sha256:" + digest, Env: os.Environ(), Confirm: confirm,
		ProbeBoundary: probe,
	})
}

// A contained profile is admitted only when the boundary can be entered on
// this host now: a host without usable unprivileged user namespaces refuses
// with exit 7, naming the probe's reason, before anything is admitted
// (AC-1.2, I02).
func TestContainedProfileRefusedWhenBoundaryProbeFails(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the restricted boundary is qualified on Linux only")
	}
	const reason = "user namespaces unavailable: operation not permitted"
	for _, profile := range []string{"", admission.ProfileRestricted, admission.ProfileInspect} {
		t.Run("profile="+profile, func(t *testing.T) {
			d, err := decideProfileProbed(t, profile, nil, func() contain.Availability {
				return contain.Availability{Reason: reason}
			})
			var blocked *admission.BlockedError
			if !errors.As(err, &blocked) || !blocked.Capability || blocked.Code != "execution_profile_unavailable" {
				t.Fatalf("err = %v, want a capability (exit 7) execution_profile_unavailable refusal", err)
			}
			if !strings.Contains(blocked.Field, reason) {
				t.Errorf("refusal %q does not name the probe's reason %q", blocked.Field, reason)
			}
			if d.RunID != "" || d.Profile.Contained || d.Profile.Boundary != nil {
				t.Errorf("a refused run admitted %+v", d.Profile)
			}
		})
	}
}

// The probed version replaces the registry record's version in the admitted
// evidence, so the receipt reports the boundary that was actually entered
// (AC-3.2, I09, I14).
func TestContainedProfileBindsProbedVersion(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the restricted boundary is qualified on Linux only")
	}
	for _, profile := range []string{admission.ProfileRestricted, admission.ProfileInspect} {
		d, err := decideProfileProbed(t, profile, nil, availableProbe)
		if err != nil {
			t.Fatalf("%s: %v", profile, err)
		}
		if d.Profile.Boundary == nil || d.Profile.Boundary.Version != probedVersion {
			t.Errorf("%s: boundary = %+v, want version %q from the probe", profile, d.Profile.Boundary, probedVersion)
		}
	}
}

// trusted-host enters no boundary, so it never probes.
func TestTrustedHostDoesNotProbeBoundary(t *testing.T) {
	_, err := decideProfileProbed(t, admission.ProfileTrustedHost, nil, func() contain.Availability {
		t.Error("trusted-host probed the boundary")
		return contain.Availability{}
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRestrictedDefaultNoFlag(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the restricted boundary is qualified on Linux only")
	}
	d, err := decideProfile(t, "", nil)
	if err != nil {
		t.Fatalf("empty flag with no Confirm: %v; want restricted admitted without a consent question", err)
	}
	if d.Profile.Name != admission.ProfileRestricted || !d.Profile.Contained {
		t.Errorf("profile = %+v, want restricted and contained", d.Profile)
	}
}

func TestRestrictedRefusedWhereUnqualified(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("covered by TestRestrictedDefaultNoFlag on Linux")
	}
	_, err := decideProfile(t, "", nil)
	var blocked *admission.BlockedError
	if !errors.As(err, &blocked) || !blocked.Capability {
		t.Fatalf("err = %v, want an exit-7 refusal, never host authority", err)
	}
}

func TestTrustedHostStillNeedsConsent(t *testing.T) {
	// An explicit trusted-host with no Confirm is the flag consent.
	d, err := decideProfile(t, "trusted-host", nil)
	if err != nil {
		t.Fatal(err)
	}
	if d.Profile.Name != admission.ProfileTrustedHost || d.Profile.Contained || d.Profile.Consent != "--execution-profile" {
		t.Errorf("profile = %+v, want trusted-host, uncontained, consent via the flag", d.Profile)
	}

	// A refusing Confirm no longer blocks an empty flag, and never widens it
	// to trusted-host.
	if runtime.GOOS == "linux" {
		asked := false
		d, err = decideProfile(t, "", func(string) (bool, error) { asked = true; return false, nil })
		if err != nil {
			t.Fatal(err)
		}
		if asked || d.Profile.Name != admission.ProfileRestricted {
			t.Errorf("asked = %v, profile = %+v, want restricted with no question", asked, d.Profile)
		}
	}
}

// TestRestrictedAndInspectRefuseCodex pins N7: no boundary evidence exists for
// the Codex route, so both contained profiles are refused through the
// registry's unknown-key path on every OS (I02).
func TestRestrictedAndInspectRefuseCodex(t *testing.T) {
	t.Parallel()
	for key := range contain.SeedV1() {
		if strings.HasSuffix(key, "/builtin/codex") {
			t.Errorf("SeedV1 holds %q: a Codex record needs a boundary qualification, not a seed edit", key)
		}
	}
	for _, profile := range []string{"restricted", "inspect"} {
		for _, goos := range []string{"linux", "darwin", "windows"} {
			t.Run(profile+"/"+goos, func(t *testing.T) {
				t.Parallel()
				_, err := admission.BoundaryConsult(profile, goos, "builtin/codex")
				blocked, ok := errors.AsType[*admission.BlockedError](err)
				if !ok || !blocked.Capability || blocked.Code != "execution_profile_unavailable" {
					t.Fatalf("err = %v, want a capability (exit 7) execution_profile_unavailable refusal", err)
				}
				if !errors.Is(err, contain.ErrMissingCoverage) {
					t.Errorf("err = %v, want the unknown-key path (contain.ErrMissingCoverage)", err)
				}
				if want := profile + " on " + goos + "/builtin/codex: missing coverage: filesystem, process, network, credential"; !strings.Contains(blocked.Field, want) {
					t.Errorf("Field = %q, want it to contain %q", blocked.Field, want)
				}
			})
		}
	}
}
