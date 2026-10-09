package contain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"slices"
	"strconv"
)

// The boundary an evaluator names: the Linux namespace boundary, version 1.
const (
	BoundaryName    = "mythhelm-restricted"
	BoundaryVersion = "1"
)

// Evaluator identifies what ran a verification: Name is the boundary (or
// "host"), Digest hashes everything that decides what the checks could do.
type Evaluator struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

// EvaluatorDigest hashes the boundary name and version, the canonical check
// policy and the sorted check-definition digests. The policy contributes only
// stable semantic fields: its profile, read-only flag and sorted auth-bind
// targets. Workdir, ProxyAddr and bind sources are runtime-specific and are
// excluded, so equivalent evaluations hash equal across runs.
func EvaluatorDigest(boundary, version string, policy Policy, checkDigests []string) Evaluator {
	targets := make([]string, len(policy.AuthBinds))
	for i, b := range policy.AuthBinds {
		targets[i] = b.Target
	}
	slices.Sort(targets)
	checks := slices.Clone(checkDigests)
	slices.Sort(checks)

	h := sha256.New()
	field(h, boundary)
	field(h, version)
	field(h, policy.Profile)
	field(h, strconv.FormatBool(policy.ReadOnly))
	for _, list := range [][]string{targets, checks} {
		field(h, strconv.Itoa(len(list)))
		for _, s := range list {
			field(h, s)
		}
	}
	return Evaluator{Name: boundary, Digest: hex.EncodeToString(h.Sum(nil))}
}

// field writes s length-prefixed, so adjacent fields cannot run together.
func field(h hash.Hash, s string) {
	_, _ = fmt.Fprintf(h, "%d:%s;", len(s), s)
}
