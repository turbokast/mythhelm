// Precise-refusal tests and the frozen v1 evidence pins (design §2.10,
// NFR-1, AC-1.2). These run on every OS: the refusal legs assert exit 7 with
// each missing dimension named, and the freeze pins every record's version,
// owner and method, where method is the Boundary name plus the four claim
// Details. The Linux enforcement legs that admit records live in
// contain_linux_test.go.
package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/contain"
)

// TestUnsupportedRefusalPrecise pins the refusal half of NFR-1: restricted
// and inspect where the provider cannot cover them exit 7, name every
// missing dimension, and record nothing (no drop to host authority). On
// Linux the fake route is admitted (the enforcement tests cover it), so the
// Linux legs refuse the claudecode route, whose credential claim is
// unenforced; off Linux every contained combination refuses. The Codex
// route has no record at all, so it refuses all four dimensions on every OS
// (N7 of codex-native-adapter): a CLI that accepts --adapter codex needs this
// refusal test.
func TestUnsupportedRefusalPrecise(t *testing.T) {
	type refusalCase struct {
		profile string
		adapter string
		missing string
	}
	four := "missing coverage: filesystem, process, network, credential"
	cases := []refusalCase{
		{"restricted", "codex", four},
		{"inspect", "codex", four},
	}
	if runtime.GOOS == "linux" {
		credential := "missing coverage: credential"
		cases = append(cases,
			refusalCase{"restricted", "claudecode", credential},
			refusalCase{"inspect", "claudecode", credential})
	} else {
		cases = append(cases,
			refusalCase{"restricted", "fake", four},
			refusalCase{"inspect", "fake", four},
			refusalCase{"restricted", "claudecode", four},
			refusalCase{"inspect", "claudecode", four})
	}
	for _, c := range cases {
		t.Run(c.profile+"/"+c.adapter, func(t *testing.T) {
			e := newEnv(t)
			repo, digest := mkRepo(t, e, t.TempDir(), "pass")
			code, _, stderr := run(t, e, repo, "run", "--task-file", "task.md", "--adapter", c.adapter,
				"--billing", "local-scripted", "--execution-profile", c.profile,
				"--non-interactive", "--trust-project-config", "sha256:"+digest)
			if code != 7 {
				t.Fatalf("exit %d, want 7 (refusal without host fallback)\nstderr:\n%s", code, stderr)
			}
			if !strings.Contains(stderr, "execution_profile_unavailable") {
				t.Errorf("stderr lacks execution_profile_unavailable:\n%s", stderr)
			}
			if !strings.Contains(stderr, c.missing) {
				t.Errorf("stderr lacks %q:\n%s", c.missing, stderr)
			}
			if route := "builtin/" + c.adapter; !strings.Contains(stderr, route) {
				t.Errorf("stderr does not name the route %s:\n%s", route, stderr)
			}
			if entries, err := os.ReadDir(filepath.Join(e.state, "runs")); err == nil && len(entries) != 0 {
				t.Errorf("a refused run recorded %d runs", len(entries))
			}
		})
	}
}

// TestEvidenceRecordsVersioned freezes the v1 boundary evidence (design
// §2.10, NFR-1): every SeedV1 record carries its exact version, owner and
// method, and every profile × OS × route key the CLI accepts is either a
// record with an enforcement test (admitted on Linux) or a record with a
// precise refusal (the consult the refusal tests exercise). An unrecorded,
// untested combination fails here: the key set is asserted exactly, so a
// newly accepted combination trips the count until the suite covers it.
// Trusted-host claims no boundary (the receipt renders it unknown), so it
// is pinned as unrecorded rather than refused.
func TestEvidenceRecordsVersioned(t *testing.T) {
	seed := contain.SeedV1()
	profiles := []string{"restricted", "inspect"}
	oses := []string{"linux", "darwin", "windows"}
	routes := []string{"builtin/fake", "builtin/claudecode"}
	if len(seed) != len(profiles)*len(oses)*len(routes) {
		keys := make([]string, 0, len(seed))
		for k := range seed {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		t.Fatalf("SeedV1 holds %d records, want %d: %v",
			len(seed), len(profiles)*len(oses)*len(routes), keys)
	}
	all := []contain.Dimension{contain.DimFilesystem, contain.DimProcess, contain.DimNetwork, contain.DimCredential}
	for _, profile := range profiles {
		for _, goos := range oses {
			for _, route := range routes {
				key := profile + "/" + goos + "/" + route
				ev, ok := seed[key]
				if !ok {
					t.Errorf("%s has no record and no refusal test", key)
					continue
				}
				checkFrozenRecord(t, ev, profile, goos, route)
				missing := ev.Coverage.Missing(all)
				if len(missing) == 0 {
					// Admitted records need enforcement tests on their OS:
					// in v1 only the Linux fake route is admitted, covered
					// by the escape fixtures (restricted) and the
					// inspect-write test in contain_linux_test.go.
					if goos != "linux" || route != "builtin/fake" {
						t.Errorf("%s is admitted without an enforcement test", key)
					}
					continue
				}
				_, err := admission.BoundaryConsult(profile, goos, route)
				if err == nil {
					t.Errorf("%s is missing %v but consults clean", key, missing)
					continue
				}
				if !strings.Contains(err.Error(), "execution_profile_unavailable") {
					t.Errorf("%s refusal lacks execution_profile_unavailable: %v", key, err)
				}
				for _, d := range missing {
					if !strings.Contains(err.Error(), string(d)) {
						t.Errorf("%s refusal does not name %s: %v", key, d, err)
					}
				}
			}
		}
	}
	for _, goos := range oses {
		for _, route := range routes {
			if _, ok := seed["trusted-host/"+goos+"/"+route]; ok {
				t.Errorf("trusted-host/%s/%s has a record; host authority claims no boundary", goos, route)
			}
		}
	}
	_, err := admission.BoundaryConsult("restricted", "plan9", "builtin/fake")
	if !errors.Is(err, contain.ErrMissingCoverage) {
		t.Fatalf("unknown OS consult = %v, want %v", err, contain.ErrMissingCoverage)
	}
	for _, d := range all {
		if !strings.Contains(err.Error(), string(d)) {
			t.Errorf("unknown OS refusal does not name %s: %v", d, err)
		}
	}
}

// checkFrozenRecord pins one record's exact version, owner and method.
func checkFrozenRecord(t *testing.T, ev contain.Evidence, profile, goos, route string) {
	t.Helper()
	key := profile + "/" + goos + "/" + route
	if ev.Profile != profile || ev.OS != goos || ev.Route != route {
		t.Errorf("%s: identity = %s/%s/%s", key, ev.Profile, ev.OS, ev.Route)
	}
	if ev.Version != "1" {
		t.Errorf("%s: version = %q, want %q", key, ev.Version, "1")
	}
	if ev.Owner != "mythhelm/contained-execution-profiles" {
		t.Errorf("%s: owner = %q", key, ev.Owner)
	}
	wantBoundary := "none"
	if goos == "linux" {
		wantBoundary = "mythhelm-restricted/1"
	}
	if ev.Boundary != wantBoundary {
		t.Errorf("%s: boundary = %q, want %q", key, ev.Boundary, wantBoundary)
	}
	got := map[contain.Dimension]contain.Claim{
		contain.DimFilesystem: ev.Coverage.Filesystem,
		contain.DimProcess:    ev.Coverage.Process,
		contain.DimNetwork:    ev.Coverage.Network,
		contain.DimCredential: ev.Coverage.Credential,
	}
	for dim, claim := range got {
		if want := wantFrozenClaim(goos, route, dim); claim != want {
			t.Errorf("%s: %s claim = %+v, want %+v", key, dim, claim, want)
		}
	}
}

// wantFrozenClaim is the exact v1 claim for one dimension: the Linux
// namespace boundary where qualified (the claudecode route still lacks its
// credential claim), unqualified elsewhere.
func wantFrozenClaim(goos, route string, dim contain.Dimension) contain.Claim {
	if goos != "linux" {
		return contain.Claim{Name: "unqualified", Version: "1",
			Detail: "no native boundary is qualified for " + goos + " in v1"}
	}
	switch dim {
	case contain.DimFilesystem:
		return contain.Claim{Name: "mount-namespace-readonly-root", Version: "1", Enforced: true,
			Detail: "recursive read-only root; read-write or read-only workdir bind; tmpfs /tmp and HOME"}
	case contain.DimProcess:
		return contain.Claim{Name: "user-namespace", Version: "1", Enforced: true,
			Detail: "no capabilities and NO_NEW_PRIVS; signalling same-UID host processes is not blocked (no PID namespace)"}
	case contain.DimNetwork:
		return contain.Claim{Name: "filtering-connect-proxy", Version: "1", Enforced: true,
			Detail: "proxy-routed (direct egress not blocked)"}
	case contain.DimCredential:
		if route == "builtin/claudecode" {
			return contain.Claim{Name: "scratch-home-auth-binds", Version: "1",
				Detail: "native auth is not yet bound into the boundary for this route"}
		}
		return contain.Claim{Name: "scratch-home-auth-binds", Version: "1", Enforced: true,
			Detail: "ambient credential files are unreachable; only admitted single-file auth binds exist"}
	}
	return contain.Claim{}
}
