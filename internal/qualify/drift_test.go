package qualify

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// Pinned digests for the drift fixtures: realistic sha256 shapes, fixed so
// the reasons are stable.
const (
	driftExecPinned = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	driftExecOther  = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	driftCfgPinned  = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	driftCfgOther   = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
)

// TestDriftDetectsBinaryAndConfigChange pins AC-4.1 at the pure check: a
// changed binary or config digest reports drifted with a reason naming the
// field, identical digests report clean. An unestablished ("unknown") pinned
// value cannot drift — there is nothing to drift from (I09) — while an
// unobserved current value fails closed against an established pin.
func TestDriftDetectsBinaryAndConfigChange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pinnedEx  string
		pinnedCfg string
		exec      string
		cfg       string
		drifted   bool
		reasonHas string
	}{
		{name: "binary change drifts", pinnedEx: driftExecPinned, pinnedCfg: driftCfgPinned, exec: driftExecOther, cfg: driftCfgPinned, drifted: true, reasonHas: "executable_digest"},
		{name: "config change drifts", pinnedEx: driftExecPinned, pinnedCfg: driftCfgPinned, exec: driftExecPinned, cfg: driftCfgOther, drifted: true, reasonHas: "config_digest"},
		{name: "identical digests clean", pinnedEx: driftExecPinned, pinnedCfg: driftCfgPinned, exec: driftExecPinned, cfg: driftCfgPinned, drifted: false},
		{name: "both changed names binary first", pinnedEx: driftExecPinned, pinnedCfg: driftCfgPinned, exec: driftExecOther, cfg: driftCfgOther, drifted: true, reasonHas: "executable_digest"},
		{name: "unknown pinned binary never drifts", pinnedEx: "unknown", pinnedCfg: driftCfgPinned, exec: driftExecOther, cfg: driftCfgPinned, drifted: false},
		{name: "unknown pinned config never drifts", pinnedEx: driftExecPinned, pinnedCfg: "unknown", exec: driftExecPinned, cfg: driftCfgOther, drifted: false},
		{name: "unobserved binary fails closed", pinnedEx: driftExecPinned, pinnedCfg: driftCfgPinned, exec: "unknown", cfg: driftCfgPinned, drifted: true, reasonHas: "executable_digest"},
		{name: "unobserved config fails closed", pinnedEx: driftExecPinned, pinnedCfg: driftCfgPinned, exec: driftExecPinned, cfg: "", drifted: true, reasonHas: "config_digest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := Record{Key: Key{ExecutableDigest: tt.pinnedEx, ConfigDigest: tt.pinnedCfg}}
			drifted, reason := CheckDrift(DriftInput{Record: rec, ExecutableDigest: tt.exec, ConfigDigest: tt.cfg})
			if drifted != tt.drifted {
				t.Fatalf("CheckDrift drifted = %v, want %v (reason %q)", drifted, tt.drifted, reason)
			}
			if tt.drifted {
				if !strings.Contains(reason, tt.reasonHas) {
					t.Errorf("reason %q does not name %q", reason, tt.reasonHas)
				}
			} else if reason != "" {
				t.Errorf("clean check carries reason %q, want empty", reason)
			}
		})
	}
}

// TestConfigDigestOfDeterministic pins the comparable config digest: the
// same entries in different insertion orders yield the identical
// "sha256:<hex>", and changing one value changes it.
func TestConfigDigestOfDeterministic(t *testing.T) {
	t.Parallel()

	entries := [][2]string{{"hooks", "aaa"}, {"mcp", "bbb"}, {"plugins", "ccc"}, {"skills", "ddd"}}
	forward := map[string]string{}
	for _, e := range entries {
		forward[e[0]] = e[1]
	}
	reversed := map[string]string{}
	for _, e := range slices.Backward(entries) {
		reversed[e[0]] = e[1]
	}
	a, b := ConfigDigestOf(forward), ConfigDigestOf(reversed)
	if a != b {
		t.Fatalf("ConfigDigestOf insertion orders differ: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, "sha256:") || len(a) != len("sha256:")+64 {
		t.Fatalf("ConfigDigestOf = %q, want sha256:<64 hex>", a)
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(a, "sha256:")); err != nil {
		t.Fatalf("ConfigDigestOf hex part does not decode: %v", err)
	}

	// The construction mirrors the sha256-of-marshalled-map in
	// adapters/claudecode/settings.go: encoding/json sorts map keys, so
	// insertion order never affects the digest.
	raw, err := json.Marshal(forward)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if want := "sha256:" + hex.EncodeToString(sum[:]); a != want {
		t.Errorf("ConfigDigestOf = %q, want the sha256-of-marshalled-map %q", a, want)
	}

	changed := map[string]string{"hooks": "aaa", "mcp": "bbb", "plugins": "ccc", "skills": "zzz"}
	if got := ConfigDigestOf(changed); got == a {
		t.Error("ConfigDigestOf unchanged after changing one value, want a different digest")
	}
}
