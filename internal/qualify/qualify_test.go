// Package qualify_test pins the qualification-registry Task 1 contract: the
// v2 record model, honest-label scales and canonical digest/decode behaviour
// every later registry task builds against.
package qualify_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/qualify"
)

// fixedKeyJSON returns a fixed qualification key in canonical JSON form.
// Fixtures travel through JSON so the tests only name snake_case fields.
func fixedKeyJSON() []byte {
	return []byte(`{` +
		`"harness":"claude-code",` +
		`"surface":"native-cli-structured (print, stream-json)",` +
		`"executable_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",` +
		`"adapter_protocol":"builtin/claudecode+stream-json",` +
		`"os":"linux",` +
		`"arch":"amd64",` +
		`"provider_endpoint":"first-party-subscription",` +
		`"model_snapshot":"2026-09-01-claude-opus-4-6",` +
		`"effort_settings":"none",` +
		`"auth_category":"claude.ai/max",` +
		`"config_digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",` +
		`"trust_profile":"trusted-host",` +
		`"workspace_class":"local-checkout",` +
		`"entitlement_class":"included-plan"` +
		`}`)
}

// fixedKey decodes the fixed key fixture.
func fixedKey(t *testing.T) qualify.Key {
	t.Helper()

	var k qualify.Key
	if err := json.Unmarshal(fixedKeyJSON(), &k); err != nil {
		t.Fatalf("unmarshal fixed key: %v", err)
	}
	return k
}

// TestProgressScaleMatchesV2 pins the seven v2 §4.2 progress words in
// order (AC-2.2). Renaming any constant fails this test.
func TestProgressScaleMatchesV2(t *testing.T) {
	t.Parallel()

	got := []qualify.Progress{
		qualify.ProgressPlanned,
		qualify.ProgressDocumentedCandidate,
		qualify.ProgressFixtureTested,
		qualify.ProgressLiveQualified,
		qualify.ProgressExperimental,
		qualify.ProgressBlocked,
		qualify.ProgressUnsupported,
	}
	want := []string{
		"planned",
		"documented-candidate",
		"fixture-tested",
		"live-qualified",
		"experimental",
		"blocked",
		"unsupported",
	}
	if len(got) != len(want) {
		t.Fatalf("progress scale has %d values, want %d", len(got), len(want))
	}
	for i := range want {
		if string(got[i]) != want[i] {
			t.Errorf("progress[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestKeyHashDeterministic pins the canonical key hash: a fixed key maps
// to its golden hex, and changing one field changes the hash. Later store
// tasks derive every key hash through KeyHash (see the spec handoff).
func TestKeyHashDeterministic(t *testing.T) {
	t.Parallel()

	const golden = "4b8492e049a93bfcf4abcd9c2b6a8416f34cec19f1c4dc5e41d0e87ec855d72a"
	if got := qualify.KeyHash(fixedKey(t)); got != golden {
		t.Errorf("KeyHash(fixed key) = %q, want golden %q", got, golden)
	}

	mutated := fixedKey(t)
	mutated.Surface += " (other)"
	if qualify.KeyHash(mutated) == qualify.KeyHash(fixedKey(t)) {
		t.Error("KeyHash unchanged after mutating one key field, want a different hash")
	}
}

// fixedRecordJSON returns a fixed blocked record in canonical JSON form.
func fixedRecordJSON() []byte {
	return []byte(`{` +
		`"schema_version":2,` +
		`"revision":1,` +
		`"digest":"",` +
		`"key":` + string(fixedKeyJSON()) + `,` +
		`"progress":"blocked",` +
		`"fidelity":{"verdict":"unknown","evidence":[]},` +
		`"entitlement":{"verdict":"not-proven","evidence":[]},` +
		`"lifecycle":{"verdict":"unknown","evidence":[]},` +
		`"capabilities":{"stop_at_exhaustion":{"value":"unknown","evidence":"unknown","scope":"subscription spend","expiry":null}},` +
		`"quota":{"quantity":"unknown","label":"unknown","unit":"unknown","scope":"unknown","source":"unknown","at":"0001-01-01T00:00:00Z"},` +
		`"next_test":"run authorised fixture suite; authority: maintainer grant",` +
		`"superseded_at":null` +
		`}`)
}

// recordMap unmarshals record JSON into a generic map for fixture surgery.
func recordMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal record JSON: %v", err)
	}
	return m
}

// mustMarshal marshals v or fails the test.
func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()

	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

// asMap asserts v is a JSON object.
func asMap(t *testing.T, v any, what string) map[string]any {
	t.Helper()

	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s is %T, want object", what, v)
	}
	return m
}

// dropKeyField removes one field from the key object of record JSON.
func dropKeyField(t *testing.T, raw []byte, field string) []byte {
	t.Helper()

	m := recordMap(t, raw)
	delete(asMap(t, m["key"], "key"), field)
	return mustMarshal(t, m)
}

// setRecordField sets one top-level field of record JSON.
func setRecordField(t *testing.T, raw []byte, field string, value any) []byte {
	t.Helper()

	m := recordMap(t, raw)
	m[field] = value
	return mustMarshal(t, m)
}

// snakeCaseKey matches the canonical JSON key shape.
var snakeCaseKey = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// assertSnakeCaseKeys fails when any JSON object key is not snake_case.
func assertSnakeCaseKeys(t *testing.T, raw []byte) {
	t.Helper()

	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var walk func(path string, x any)
	walk = func(path string, x any) {
		switch x := x.(type) {
		case map[string]any:
			for k, item := range x {
				if !snakeCaseKey.MatchString(k) {
					t.Errorf("%s: key %q is not snake_case", path, k)
				}
				walk(path+"."+k, item)
			}
		case []any:
			for i, item := range x {
				walk(fmt.Sprintf("%s[%d]", path, i), item)
			}
		}
	}
	walk("$", v)
}

// TestDigestRecomputes pins the content digest: a fixed record maps to its
// golden sha256 digest, and flipping one byte changes the digest (NFR-2).
func TestDigestRecomputes(t *testing.T) {
	t.Parallel()

	rec, err := qualify.DecodeRecord(fixedRecordJSON())
	if err != nil {
		t.Fatalf("DecodeRecord(fixed record): %v", err)
	}
	const golden = "sha256:ee5372fd7e1761957b21ba52045508ac4bc23ed9adbe378aa3f668354a62408a"
	got, err := qualify.CanonicalDigest(rec)
	if err != nil {
		t.Fatalf("CanonicalDigest: %v", err)
	}
	if got != golden {
		t.Errorf("CanonicalDigest = %q, want golden %q", got, golden)
	}

	mutated := rec
	mutated.NextTest += " (retest)"
	mut, err := qualify.CanonicalDigest(mutated)
	if err != nil {
		t.Fatalf("CanonicalDigest(mutated): %v", err)
	}
	if mut == got {
		t.Error("digest unchanged after flipping one byte, want a different digest")
	}

	// The digest covers content, not the Digest/SupersededAt carriage.
	rec.Digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	again, err := qualify.CanonicalDigest(rec)
	if err != nil {
		t.Fatalf("CanonicalDigest: %v", err)
	}
	if again != got {
		t.Error("CanonicalDigest follows the Digest field, want content-only coverage")
	}
}

// TestOutOfScaleEnumRejected pins fail-closed validation: a record whose
// progress sits outside the v2 §4.2 scale errors naming the field, and the
// decoder never coerces it to a passing value.
func TestOutOfScaleEnumRejected(t *testing.T) {
	t.Parallel()

	rec, err := qualify.DecodeRecord(fixedRecordJSON())
	if err != nil {
		t.Fatalf("DecodeRecord(fixed record): %v", err)
	}
	rec.Progress = "approved"
	if got, err := qualify.CanonicalDigest(rec); err == nil {
		t.Errorf("CanonicalDigest(out-of-scale Progress) = %q, want error naming Progress", got)
	} else if !strings.Contains(err.Error(), "Progress") {
		t.Errorf("CanonicalDigest error = %q, want it to name Progress", err)
	}

	raw := mustMarshal(t, rec)
	got, err := qualify.DecodeRecord(raw)
	if err == nil {
		t.Fatal("DecodeRecord(out-of-scale Progress) succeeded, want error naming Progress")
	}
	if !strings.Contains(err.Error(), "Progress") {
		t.Errorf("DecodeRecord error = %q, want it to name Progress", err)
	}
	if !reflect.DeepEqual(got, qualify.Record{}) {
		t.Errorf("DecodeRecord returned %+v on error, want the zero record (fail closed)", got.Progress)
	}
}

// decodedKeyMap re-marshals a decoded key into a generic map so assertions
// name only snake_case fields.
func decodedKeyMap(t *testing.T, k qualify.Key) map[string]any {
	t.Helper()

	raw, err := json.Marshal(k)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

// TestCanonicalJSONUsesSnakeCase pins the canonical field shape: marshalled
// records carry snake_case keys, and JSON missing the snapshot field
// decodes to the unknown marker.
func TestCanonicalJSONUsesSnakeCase(t *testing.T) {
	t.Parallel()

	rec, err := qualify.DecodeRecord(fixedRecordJSON())
	if err != nil {
		t.Fatalf("DecodeRecord(fixed record): %v", err)
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"model_snapshot"`) {
		t.Errorf("canonical JSON lacks model_snapshot: %s", raw)
	}
	assertSnakeCaseKeys(t, raw)

	got, err := qualify.DecodeRecord(dropKeyField(t, raw, "model_snapshot"))
	if err != nil {
		t.Fatalf("DecodeRecord(missing snapshot): %v", err)
	}
	if decodedKeyMap(t, got.Key)["model_snapshot"] != "unknown" {
		t.Errorf("missing snapshot decodes to %v, want \"unknown\"",
			decodedKeyMap(t, got.Key)["model_snapshot"])
	}
}

// TestMissingReadsUnknown pins I09 at the decode boundary: absent values
// read unknown, truncated input names its offset, and out-of-scale enums
// name their field.
func TestMissingReadsUnknown(t *testing.T) {
	t.Parallel()

	got, err := qualify.DecodeRecord(dropKeyField(t, fixedRecordJSON(), "model_snapshot"))
	if err != nil {
		t.Fatalf("DecodeRecord(missing snapshot): %v", err)
	}
	keyMap := decodedKeyMap(t, got.Key)
	for field, v := range keyMap {
		s, ok := v.(string)
		if !ok || s == "" {
			t.Errorf("key field %q decoded to %#v, want a non-empty marker", field, v)
		}
	}
	if keyMap["model_snapshot"] != "unknown" {
		t.Errorf("missing snapshot = %#v, want \"unknown\"", keyMap["model_snapshot"])
	}

	if _, err := qualify.DecodeRecord([]byte(`{"schema_version":2,"key":{`)); err == nil {
		t.Error("DecodeRecord(truncated) succeeded, want error naming the offset")
	} else if !strings.Contains(err.Error(), "offset") {
		t.Errorf("DecodeRecord(truncated) error = %q, want it to name the offset", err)
	}

	bad := setRecordField(t, fixedRecordJSON(), "progress", "approved")
	if _, err := qualify.DecodeRecord(bad); err == nil {
		t.Error("DecodeRecord(out-of-scale progress) succeeded, want error")
	} else if !strings.Contains(err.Error(), "Progress") {
		t.Errorf("DecodeRecord error = %q, want it to name Progress", err)
	}
}

// TestDatumDefaults pins the unknown datum (AC-5.2): JSON missing quota
// and evidence label/source yields unknown labels and markers, and an
// out-of-scale datum label names its field.
func TestDatumDefaults(t *testing.T) {
	t.Parallel()

	m := recordMap(t, fixedRecordJSON())
	delete(m, "quota")
	fidelity := asMap(t, m["fidelity"], "fidelity")
	fidelity["evidence"] = []any{
		map[string]any{
			"id": "ev_probe1", "method": "offline-fixture", "suite": "at03-routes",
			"result": "pass", "uncertainty": "unknown", "at": "2026-10-01T00:00:00Z",
		},
	}
	got, err := qualify.DecodeRecord(mustMarshal(t, m))
	if err != nil {
		t.Fatalf("DecodeRecord: %v", err)
	}
	if got.Quota.Label != qualify.DatumUnknown {
		t.Errorf("Quota.Label = %q, want %q", got.Quota.Label, qualify.DatumUnknown)
	}
	for _, f := range []struct {
		name string
		got  string
	}{
		{"Quantity", got.Quota.Quantity},
		{"Unit", got.Quota.Unit},
		{"Scope", got.Quota.Scope},
		{"Source", got.Quota.Source},
	} {
		if f.got != "unknown" {
			t.Errorf("Quota.%s = %q, want \"unknown\"", f.name, f.got)
		}
	}
	if len(got.Fidelity.Evidence) != 1 {
		t.Fatalf("Fidelity.Evidence has %d entries, want 1", len(got.Fidelity.Evidence))
	}
	ev := got.Fidelity.Evidence[0]
	if ev.Label != qualify.DatumUnknown {
		t.Errorf("Evidence.Label = %q, want %q", ev.Label, qualify.DatumUnknown)
	}
	if ev.Source != "unknown" {
		t.Errorf("Evidence.Source = %q, want \"unknown\"", ev.Source)
	}

	bad := recordMap(t, fixedRecordJSON())
	asMap(t, bad["quota"], "quota")["label"] = "guessed"
	if _, err := qualify.DecodeRecord(mustMarshal(t, bad)); err == nil {
		t.Error("DecodeRecord(out-of-scale datum label) succeeded, want error")
	} else if !strings.Contains(err.Error(), "Quota.Label") {
		t.Errorf("DecodeRecord error = %q, want it to name Quota.Label", err)
	}
}

// TestColumnIndependence pins AC-1.2: mixed column verdicts survive a JSON
// round-trip with each column preserved and none changing another.
func TestColumnIndependence(t *testing.T) {
	t.Parallel()

	m := recordMap(t, fixedRecordJSON())
	m["fidelity"] = map[string]any{
		"verdict": "proven",
		"evidence": []any{map[string]any{
			"id": "ev_fixture1", "method": "offline-fixture", "suite": "at03-routes",
			"result": "pass", "uncertainty": "unknown", "at": "2026-10-01T00:00:00Z",
			"expiry": nil, "label": "observed", "source": "fixture harness",
		}},
	}
	m["entitlement"] = map[string]any{"verdict": "not-proven", "evidence": []any{}}
	m["lifecycle"] = map[string]any{"verdict": "unknown", "evidence": []any{}}
	got, err := qualify.DecodeRecord(mustMarshal(t, m))
	if err != nil {
		t.Fatalf("DecodeRecord: %v", err)
	}
	if got.Fidelity.Verdict != qualify.Proven {
		t.Errorf("Fidelity = %q, want %q", got.Fidelity.Verdict, qualify.Proven)
	}
	if got.Entitlement.Verdict != qualify.NotProven {
		t.Errorf("Entitlement = %q, want %q", got.Entitlement.Verdict, qualify.NotProven)
	}
	if got.Lifecycle.Verdict != qualify.Unknown {
		t.Errorf("Lifecycle = %q, want %q", got.Lifecycle.Verdict, qualify.Unknown)
	}

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	again, err := qualify.DecodeRecord(raw)
	if err != nil {
		t.Fatalf("DecodeRecord(round-trip): %v", err)
	}
	if again.Fidelity.Verdict != qualify.Proven ||
		again.Entitlement.Verdict != qualify.NotProven ||
		again.Lifecycle.Verdict != qualify.Unknown {
		t.Errorf("round-trip verdicts = %q/%q/%q, want proven/not-proven/unknown",
			again.Fidelity.Verdict, again.Entitlement.Verdict, again.Lifecycle.Verdict)
	}
}
