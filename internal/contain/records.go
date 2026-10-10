package contain

const (
	boundaryV1   = "mythhelm-restricted/1"
	recordsOwner = "mythhelm/contained-execution-profiles"
)

// SeedV1 returns the v1 boundary evidence, keyed "profile/os/route". Only the
// combinations recorded here are known; any other key is unknown and callers
// refuse it (I02). Linux has the namespace boundary; darwin and windows are
// recorded as unqualified, so a refusal can name each missing dimension.
//
// The v1 records are frozen (design §2.10):
// tests/e2e/contain_refusal_test.go pins every version, owner and method,
// where method is the Boundary name plus the four claim Details. Changing a
// record means updating the freeze test deliberately, never drifting it.
func SeedV1() map[string]Evidence {
	seed := map[string]Evidence{}
	for _, profile := range []string{"restricted", "inspect"} {
		for _, route := range []string{"builtin/fake", "builtin/claudecode"} {
			for _, goos := range []string{"linux", "darwin", "windows"} {
				ev := Evidence{Profile: profile, OS: goos, Route: route, Boundary: "none", Version: "1", Owner: recordsOwner, Coverage: unqualified(goos)}
				if goos == "linux" {
					ev.Boundary, ev.Coverage = boundaryV1, linuxCoverage(route)
				}
				seed[profile+"/"+goos+"/"+route] = ev
			}
		}
	}
	return seed
}

func linuxCoverage(route string) Coverage {
	c := Coverage{
		Filesystem: Claim{Name: "mount-namespace-readonly-root", Version: "1", Enforced: true,
			Detail: "recursive read-only root; read-write or read-only workdir bind; tmpfs /tmp and HOME"},
		Process: Claim{Name: "user-namespace", Version: "1", Enforced: true,
			Detail: "no capabilities and NO_NEW_PRIVS; signalling same-UID host processes is not blocked (no PID namespace)"},
		Network: Claim{Name: "filtering-connect-proxy", Version: "1", Enforced: true,
			Detail: "proxy-routed (direct egress not blocked)"},
		Credential: Claim{Name: "scratch-home-auth-binds", Version: "1", Enforced: true,
			Detail: "ambient credential files are unreachable; only admitted single-file auth binds exist"},
	}
	if route == "builtin/claudecode" {
		c.Credential = Claim{Name: "scratch-home-auth-binds", Version: "1", Enforced: false,
			Detail: "native auth is not yet bound into the boundary for this route"}
	}
	return c
}

func unqualified(goos string) Coverage {
	c := Claim{Name: "unqualified", Version: "1", Enforced: false, Detail: "no native boundary is qualified for " + goos + " in v1"}
	return Coverage{Filesystem: c, Process: c, Network: c, Credential: c}
}
