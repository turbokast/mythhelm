package qualify

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// DriftInput is the pinned-vs-observed comparison for one record: the
// record's pinned digests against the digests observed now (AC-4.1).
type DriftInput struct {
	Record           Record
	ExecutableDigest string // observed now
	ConfigDigest     string // observed now
}

// CheckDrift reports whether the record's pinned binary digest or relevant
// configuration drifted from the observed values. It writes nothing (D4).
// An unestablished ("unknown") pinned value never drifts — there is no
// pinned value to drift from (I09). An unobserved current value fails
// closed against an established pin. The reason names the drifted field.
func CheckDrift(in DriftInput) (drifted bool, reason string) {
	pinnedExec, observedExec := normDigest(in.Record.Key.ExecutableDigest), normDigest(in.ExecutableDigest)
	if pinnedExec != unknownString && observedExec != pinnedExec {
		return true, driftReason("executable_digest", pinnedExec, observedExec)
	}
	pinnedCfg, observedCfg := normDigest(in.Record.Key.ConfigDigest), normDigest(in.ConfigDigest)
	if pinnedCfg != unknownString && observedCfg != pinnedCfg {
		return true, driftReason("config_digest", pinnedCfg, observedCfg)
	}
	return false, ""
}

// driftReason names the drifted field and both sides of the comparison: a
// changed value reports pinned-vs-observed, an unobserved value reports the
// pin it can no longer confirm.
func driftReason(field, pinned, observed string) string {
	if observed == unknownString {
		return fmt.Sprintf("%s unobserved (pinned %s)", field, pinned)
	}
	return fmt.Sprintf("%s drifted: pinned %s, observed %s", field, pinned, observed)
}

// normDigest maps an empty digest to the unknown marker, matching the
// DecodeRecord default, so unset and explicitly-unknown digests compare
// identically.
func normDigest(d string) string {
	if d == "" {
		return unknownString
	}
	return d
}

// ConfigDigestOf derives the comparable config digest from a manifest's
// digests map: "sha256:" + hex(sha256(json.Marshal(digests))).
// Encoding/json sorts map keys, so insertion order never affects the
// digest; this mirrors the existing sha256-of-marshalled-map construction
// at adapters/claudecode/settings.go.
func ConfigDigestOf(digests map[string]string) string {
	raw, err := json.Marshal(digests)
	if err != nil {
		// Unreachable: map[string]string always marshals.
		return unknownString
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
