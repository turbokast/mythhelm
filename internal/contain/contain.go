// Package contain defines the containment contract: what a boundary claims to
// enforce, the evidence record per profile, OS and route, and the policy a
// worker enters. It has no SQLite or process dependencies.
package contain

import "errors"

// Dimension is one axis of containment a profile can require.
type Dimension string

// The four dimensions a boundary can enforce.
const (
	DimFilesystem Dimension = "filesystem"
	DimProcess    Dimension = "process"
	DimNetwork    Dimension = "network"
	DimCredential Dimension = "credential"
)

// ErrUnsupported marks a boundary this OS or route cannot provide;
// ErrMissingCoverage marks one that omits a required dimension.
var (
	ErrUnsupported     = errors.New("boundary unsupported")
	ErrMissingCoverage = errors.New("boundary coverage missing")
)

// Claim names one enforcement mechanism and says whether it is enforced.
type Claim struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Enforced bool   `json:"enforced"`
	Detail   string `json:"detail"`
}

// Coverage holds one Claim per dimension. The zero Claim is not enforced.
type Coverage struct {
	Filesystem Claim `json:"filesystem"`
	Process    Claim `json:"process"`
	Network    Claim `json:"network"`
	Credential Claim `json:"credential"`
}

// Missing returns, in the order given, each required dimension whose claim is
// not enforced. A dimension this package does not know counts as missing.
func (c Coverage) Missing(required []Dimension) []Dimension {
	var missing []Dimension
	for _, d := range required {
		if !c.enforces(d) {
			missing = append(missing, d)
		}
	}
	return missing
}

func (c Coverage) enforces(d Dimension) bool {
	switch d {
	case DimFilesystem:
		return c.Filesystem.Enforced
	case DimProcess:
		return c.Process.Enforced
	case DimNetwork:
		return c.Network.Enforced
	case DimCredential:
		return c.Credential.Enforced
	}
	return false
}

// Evidence is the versioned record for one profile, OS and route; Version and
// Owner stand behind its nested claims.
type Evidence struct {
	Profile  string   `json:"profile"`
	OS       string   `json:"os"`
	Route    string   `json:"route"`
	Boundary string   `json:"boundary"`
	Version  string   `json:"version"`
	Owner    string   `json:"owner"`
	Coverage Coverage `json:"coverage"`
}

// Registry answers with ok == false for every profile, OS and route it has no
// record for; callers never treat the zero Evidence as supported.
type Registry interface {
	Lookup(profile, os, route string) (Evidence, bool)
}

// AuthBind exposes one native-auth path inside the boundary.
type AuthBind struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// Policy is everything a worker needs to enter its boundary.
type Policy struct {
	Profile   string     `json:"profile"`
	Workdir   string     `json:"workdir"`
	ReadOnly  bool       `json:"readonly"`
	AuthBinds []AuthBind `json:"auth_binds"`
	ProxyAddr string     `json:"proxy_addr"`
}

// Availability is a runtime capability probe: a version when supported, a
// precise reason when not.
type Availability struct {
	Supported bool   `json:"supported"`
	Version   string `json:"version"`
	Reason    string `json:"reason"`
}
