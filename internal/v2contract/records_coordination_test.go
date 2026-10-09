package v2contract_test

import (
	"bytes"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

// mustDecodeGoldens decodes each coordination golden into its record type.
func mustDecodeCoordination(t *testing.T) (v2contract.ContextManifest, v2contract.Message, v2contract.Grant, v2contract.Reservation, v2contract.PolicyVersion, v2contract.Experiment) {
	t.Helper()
	manifest, err := v2contract.Decode[v2contract.ContextManifest](loadGolden(t, "records/context_manifest.golden.json"))
	if err != nil {
		t.Fatalf("Decode manifest golden: %v", err)
	}
	message, err := v2contract.Decode[v2contract.Message](loadGolden(t, "records/message.golden.json"))
	if err != nil {
		t.Fatalf("Decode message golden: %v", err)
	}
	grant, err := v2contract.Decode[v2contract.Grant](loadGolden(t, "records/grant.golden.json"))
	if err != nil {
		t.Fatalf("Decode grant golden: %v", err)
	}
	reservation, err := v2contract.Decode[v2contract.Reservation](loadGolden(t, "records/reservation.golden.json"))
	if err != nil {
		t.Fatalf("Decode reservation golden: %v", err)
	}
	policy, err := v2contract.Decode[v2contract.PolicyVersion](loadGolden(t, "records/policy_version.golden.json"))
	if err != nil {
		t.Fatalf("Decode policy golden: %v", err)
	}
	experiment, err := v2contract.Decode[v2contract.Experiment](loadGolden(t, "records/experiment.golden.json"))
	if err != nil {
		t.Fatalf("Decode experiment golden: %v", err)
	}
	return manifest, message, grant, reservation, policy, experiment
}

func assertRoundTrip(t *testing.T, name string, golden []byte, v any) {
	t.Helper()
	re, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("%s: re-encode: %v", name, err)
	}
	if !bytes.Equal(bytes.TrimSpace(golden), re) {
		t.Errorf("%s: round-trip mismatch:\n got: %s\nwant: %s", name, re, bytes.TrimSpace(golden))
	}
}

// AC-1.1: the six coordination goldens decode, re-encode byte-identical, and
// carry schema_version 2.
func TestCoordinationGoldensRoundTrip(t *testing.T) {
	t.Parallel()
	manifest, message, grant, reservation, policy, experiment := mustDecodeCoordination(t)
	for _, v := range []struct {
		name string
		got  int
	}{
		{"manifest", manifest.SchemaVersion},
		{"message", message.SchemaVersion},
		{"grant", grant.SchemaVersion},
		{"reservation", reservation.SchemaVersion},
		{"policy", policy.SchemaVersion},
		{"experiment", experiment.SchemaVersion},
	} {
		if v.got != v2contract.SchemaVersion {
			t.Errorf("%s golden schema_version = %d, want %d", v.name, v.got, v2contract.SchemaVersion)
		}
	}
	assertRoundTrip(t, "manifest", loadGolden(t, "records/context_manifest.golden.json"), manifest)
	assertRoundTrip(t, "message", loadGolden(t, "records/message.golden.json"), message)
	assertRoundTrip(t, "grant", loadGolden(t, "records/grant.golden.json"), grant)
	assertRoundTrip(t, "reservation", loadGolden(t, "records/reservation.golden.json"), reservation)
	assertRoundTrip(t, "policy", loadGolden(t, "records/policy_version.golden.json"), policy)
	assertRoundTrip(t, "experiment", loadGolden(t, "records/experiment.golden.json"), experiment)

	// A golden at schema_version 1 is a different contract and must fail.
	stale := bytes.Replace(loadGolden(t, "records/context_manifest.golden.json"), []byte(`"schema_version":2`), []byte(`"schema_version":1`), 1)
	if _, err := v2contract.Decode[v2contract.ContextManifest](stale); err == nil || !strings.Contains(err.Error(), "schema_version") {
		t.Errorf("Decode(schema_version 1) error = %v, want naming schema_version", err)
	}
}

// Grant.Use is a closed enum: standing | one_use.
func TestGrantUseEnumClosed(t *testing.T) {
	t.Parallel()
	_, _, grant, _, _, _ := mustDecodeCoordination(t)
	grant.Use = "permanent"
	if err := grant.Validate(); err == nil || !strings.Contains(err.Error(), "use") {
		t.Errorf("Use permanent error = %v, want naming use", err)
	}
	for _, use := range []string{"standing", "one_use"} {
		grant.Use = use
		if err := grant.Validate(); err != nil {
			t.Errorf("Use %s error = %v, want nil", use, err)
		}
	}
}

// Handoff and EffectIntent are aliases: the same bytes decode under both
// names with equal digests.
func TestAliasesDecodeIdentically(t *testing.T) {
	t.Parallel()
	manifestBytes := loadGolden(t, "records/context_manifest.golden.json")
	manifest, err := v2contract.Decode[v2contract.ContextManifest](manifestBytes)
	if err != nil {
		t.Fatalf("Decode manifest: %v", err)
	}
	handoff, err := v2contract.Decode[v2contract.Handoff](manifestBytes)
	if err != nil {
		t.Fatalf("Decode handoff: %v", err)
	}
	if !reflect.DeepEqual(manifest, handoff) {
		t.Errorf("ContextManifest and Handoff decoded values differ")
	}
	if v2contract.Digest(manifest) != v2contract.Digest(handoff) {
		t.Errorf("ContextManifest and Handoff digests differ")
	}

	grantBytes := loadGolden(t, "records/grant.golden.json")
	grant, err := v2contract.Decode[v2contract.Grant](grantBytes)
	if err != nil {
		t.Fatalf("Decode grant: %v", err)
	}
	intent, err := v2contract.Decode[v2contract.EffectIntent](grantBytes)
	if err != nil {
		t.Fatalf("Decode effect intent: %v", err)
	}
	if !reflect.DeepEqual(grant, intent) {
		t.Errorf("Grant and EffectIntent decoded values differ")
	}
	if v2contract.Digest(grant) != v2contract.Digest(intent) {
		t.Errorf("Grant and EffectIntent digests differ")
	}
}

// I09 (v2 §2): the honest unknown quantity is the explicit string "unknown",
// never empty and never null-as-zero.
func TestReservationUnknownQuantity(t *testing.T) {
	t.Parallel()
	_, _, _, reservation, _, _ := mustDecodeCoordination(t)
	if reservation.Quantity != "unknown" {
		t.Fatalf("golden quantity = %q, want %q", reservation.Quantity, "unknown")
	}
	assertRoundTrip(t, "reservation", loadGolden(t, "records/reservation.golden.json"), reservation)

	reservation.Quantity = ""
	if err := reservation.Validate(); err == nil || !strings.Contains(err.Error(), "quantity") {
		t.Errorf("empty quantity error = %v, want naming quantity", err)
	}
	nulled := bytes.Replace(loadGolden(t, "records/reservation.golden.json"), []byte(`"quantity":"unknown"`), []byte(`"quantity":null`), 1)
	if _, err := v2contract.Decode[v2contract.Reservation](nulled); err == nil || !strings.Contains(err.Error(), "quantity") {
		t.Errorf("Decode(null quantity) error = %v, want naming quantity", err)
	}
}

// D6: DeliveryDisposition, Reservation.Status and Experiment.Disposition are
// open strings, validated non-empty.
func TestOpenStringsValidatedNonEmpty(t *testing.T) {
	t.Parallel()
	_, message, _, reservation, _, experiment := mustDecodeCoordination(t)
	message.Delivery = ""
	if err := message.Validate(); err == nil || !strings.Contains(err.Error(), "delivery") {
		t.Errorf("empty delivery error = %v, want naming delivery", err)
	}
	reservation.Status = ""
	if err := reservation.Validate(); err == nil || !strings.Contains(err.Error(), "status") {
		t.Errorf("empty status error = %v, want naming status", err)
	}
	experiment.Disposition = ""
	if err := experiment.Validate(); err == nil || !strings.Contains(err.Error(), "disposition") {
		t.Errorf("empty disposition error = %v, want naming disposition", err)
	}
}

// Non-finite splits cannot be marshalled, so they fail validation before
// Digest (design §2.2).
func TestExperimentRejectsNonFiniteSplits(t *testing.T) {
	t.Parallel()
	_, _, _, _, _, experiment := mustDecodeCoordination(t)
	for name, v := range map[string]float64{"NaN": math.NaN(), "+Inf": math.Inf(1), "-Inf": math.Inf(-1)} {
		experiment.Splits = map[string]float64{"a": v, "b": 0.5}
		if err := experiment.Validate(); err == nil || !strings.Contains(err.Error(), "splits") {
			t.Errorf("splits %s error = %v, want naming splits", name, err)
		}
	}
	experiment.Splits = map[string]float64{"a": 0.5, "b": 0.5}
	if err := experiment.Validate(); err != nil {
		t.Errorf("finite splits error = %v, want nil", err)
	}
}

// Identity and content rules: every record requires schema_version 2 and a
// non-empty ID; PolicyVersion requires Version >= 1; required content fields
// are never empty.
func TestCoordinationValidation(t *testing.T) {
	t.Parallel()
	manifest, message, grant, reservation, policy, experiment := mustDecodeCoordination(t)

	manifest.SchemaVersion = 0
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "schema_version") {
		t.Errorf("manifest schema_version 0 error = %v, want naming schema_version", err)
	}
	manifest.SchemaVersion = 2
	manifest.ManifestID = ""
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "manifest_id") {
		t.Errorf("empty manifest_id error = %v, want naming manifest_id", err)
	}

	message.MessageID = ""
	if err := message.Validate(); err == nil || !strings.Contains(err.Error(), "message_id") {
		t.Errorf("empty message_id error = %v, want naming message_id", err)
	}
	message.MessageID = "msg_demo_0001"
	for field, clear := range map[string]func(){
		"sender":      func() { message.Sender = "" },
		"recipient":   func() { message.Recipient = "" },
		"kind":        func() { message.Kind = "" },
		"sensitivity": func() { message.Sensitivity = "" },
	} {
		saved := message
		clear()
		if err := message.Validate(); err == nil || !strings.Contains(err.Error(), field) {
			t.Errorf("empty %s error = %v, want naming %s", field, err, field)
		}
		message = saved
	}

	grant.GrantID = ""
	if err := grant.Validate(); err == nil || !strings.Contains(err.Error(), "grant_id") {
		t.Errorf("empty grant_id error = %v, want naming grant_id", err)
	}
	grant.GrantID = "grt_demo_0001"
	for field, clear := range map[string]func(){
		"effect":         func() { grant.Effect = "" },
		"target":         func() { grant.Target = "" },
		"expected_state": func() { grant.ExpectedState = "" },
		"operation_id":   func() { grant.OperationID = "" },
	} {
		saved := grant
		clear()
		if err := grant.Validate(); err == nil || !strings.Contains(err.Error(), field) {
			t.Errorf("empty %s error = %v, want naming %s", field, err, field)
		}
		grant = saved
	}

	reservation.ReservationID = ""
	if err := reservation.Validate(); err == nil || !strings.Contains(err.Error(), "reservation_id") {
		t.Errorf("empty reservation_id error = %v, want naming reservation_id", err)
	}
	reservation.ReservationID = "res_demo_0001"
	for field, clear := range map[string]func(){
		"bucket": func() { reservation.Bucket = "" },
		"scope":  func() { reservation.Scope = "" },
		"owner":  func() { reservation.Owner = "" },
	} {
		saved := reservation
		clear()
		if err := reservation.Validate(); err == nil || !strings.Contains(err.Error(), field) {
			t.Errorf("empty %s error = %v, want naming %s", field, err, field)
		}
		reservation = saved
	}

	policy.PolicyID = ""
	if err := policy.Validate(); err == nil || !strings.Contains(err.Error(), "policy_id") {
		t.Errorf("empty policy_id error = %v, want naming policy_id", err)
	}
	policy.PolicyID = "pol_demo_0001"
	policy.Version = 0
	if err := policy.Validate(); err == nil || !strings.Contains(err.Error(), "version") {
		t.Errorf("version 0 error = %v, want naming version", err)
	}
	policy.Version = 3
	policy.ActionFamily = ""
	if err := policy.Validate(); err == nil || !strings.Contains(err.Error(), "action_family") {
		t.Errorf("empty action_family error = %v, want naming action_family", err)
	}

	experiment.ExperimentID = ""
	if err := experiment.Validate(); err == nil || !strings.Contains(err.Error(), "experiment_id") {
		t.Errorf("empty experiment_id error = %v, want naming experiment_id", err)
	}
	experiment.ExperimentID = "exp_demo_0001"
	for field, clear := range map[string]func(){
		"hypothesis":    func() { experiment.Hypothesis = "" },
		"population":    func() { experiment.Population = "" },
		"unit":          func() { experiment.Unit = "" },
		"assignment":    func() { experiment.Assignment = "" },
		"stopping_rule": func() { experiment.StoppingRule = "" },
	} {
		saved := experiment
		clear()
		if err := experiment.Validate(); err == nil || !strings.Contains(err.Error(), field) {
			t.Errorf("empty %s error = %v, want naming %s", field, err, field)
		}
		experiment = saved
	}
}
