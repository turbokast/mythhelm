package v2contract_test

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

func evidenceRoundTrip[T v2contract.Validator](t *testing.T, rel string) T {
	t.Helper()
	golden := bytes.TrimSpace(loadGolden(t, rel))
	got, err := v2contract.Decode[T](golden)
	if err != nil {
		t.Fatalf("Decode %s: %v", rel, err)
	}
	enc, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal %s: %v", rel, err)
	}
	if !bytes.Equal(enc, golden) {
		t.Errorf("%s re-encoded differently:\n got %s\nwant %s", rel, enc, golden)
	}
	if !strings.Contains(string(enc), `"schema_version":2`) {
		t.Errorf("%s does not carry schema_version 2: %s", rel, enc)
	}
	return got
}

func TestEvidenceGoldensRoundTrip(t *testing.T) {
	t.Parallel()
	evidenceRoundTrip[v2contract.RoutingDecision](t, "records/routing_decision.golden.json")
	evidenceRoundTrip[v2contract.DesignDecision](t, "records/design_decision.golden.json")
	evidenceRoundTrip[v2contract.Artifact](t, "records/artifact.golden.json")
	evidenceRoundTrip[v2contract.Observation](t, "records/observation.golden.json")
	evidenceRoundTrip[v2contract.Verification](t, "records/verification.golden.json")

	_, err := v2contract.Decode[v2contract.Observation]([]byte(strings.Replace(
		string(bytes.TrimSpace(loadGolden(t, "records/observation.golden.json"))),
		`"schema_version":2`, `"schema_version":1`, 1)))
	if err == nil || !strings.Contains(err.Error(), "schema_version") {
		t.Errorf("schema_version 1 error = %v, want naming schema_version", err)
	}
}

// I09 (v2 §2): an enum outside its scale is rejected naming the field, never coerced.
func TestOutOfScaleEnumsRejected(t *testing.T) {
	t.Parallel()
	dd := evidenceRoundTrip[v2contract.DesignDecision](t, "records/design_decision.golden.json")
	dd.Disposition = "approved"
	if err := dd.Validate(); err == nil || !strings.Contains(err.Error(), "disposition") || !strings.Contains(err.Error(), "approved") {
		t.Errorf("DesignDecision disposition error = %v, want naming disposition and value", err)
	}
	for _, d := range []v2contract.DesignDisposition{"proposed", "accepted", "superseded"} {
		dd.Disposition = d
		if err := dd.Validate(); err != nil {
			t.Errorf("DesignDecision disposition %q: %v", d, err)
		}
	}

	ob := evidenceRoundTrip[v2contract.Observation](t, "records/observation.golden.json")
	ob.Kind = "agent"
	if err := ob.Validate(); err == nil || !strings.Contains(err.Error(), "kind") || !strings.Contains(err.Error(), "agent") {
		t.Errorf("Observation kind error = %v, want naming kind and value", err)
	}
	for _, k := range []v2contract.ObservationKind{"controller", "tool", "native"} {
		ob.Kind = k
		if err := ob.Validate(); err != nil {
			t.Errorf("Observation kind %q: %v", k, err)
		}
	}
}

// I20 (v2 §2): design decision revisions are ordered through Supersedes.
func TestDesignDecisionRevisionOrder(t *testing.T) {
	t.Parallel()
	good := evidenceRoundTrip[v2contract.DesignDecision](t, "records/design_decision.golden.json")
	zero, same, later := 0, 2, 3
	tests := []struct {
		name       string
		revision   int
		supersedes *int
		wantErr    string
	}{
		{"revision zero", 0, nil, "revision"},
		{"supersedes zero", 2, &zero, "supersedes"},
		{"supersedes equal", 2, &same, "supersedes"},
		{"supersedes later", 2, &later, "supersedes"},
		{"first revision without supersedes", 1, nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dd := good
			dd.Revision, dd.Supersedes = tc.revision, tc.supersedes
			err := dd.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

// I09 (v2 §2): absent scores stay absent and uncertainty survives; no zero is invented.
func TestRoutingDecisionPreservesUncertainty(t *testing.T) {
	t.Parallel()
	got := evidenceRoundTrip[v2contract.RoutingDecision](t, "records/routing_decision.golden.json")
	if got.Uncertainty != "no scores available" {
		t.Errorf("Uncertainty = %q, want preserved", got.Uncertainty)
	}
	if got.Scores != nil {
		t.Errorf("Scores = %v, want nil (no zero scores invented)", got.Scores)
	}
	if got.SelectionProbability != nil {
		t.Errorf("SelectionProbability = %v, want nil", *got.SelectionProbability)
	}
	enc, _ := json.Marshal(got)
	for _, key := range []string{`"scores"`, `"selection_probability"`} {
		if strings.Contains(string(enc), key) {
			t.Errorf("encoding contains %s: %s", key, enc)
		}
	}
}

func TestRoutingDecisionRejectsNonFiniteFloats(t *testing.T) {
	t.Parallel()
	good := evidenceRoundTrip[v2contract.RoutingDecision](t, "records/routing_decision.golden.json")
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		rd := good
		rd.Scores = map[string]float64{"claude-code": bad}
		if err := rd.Validate(); err == nil || !strings.Contains(err.Error(), "scores") {
			t.Errorf("Scores %v error = %v, want naming scores", bad, err)
		}
		rd = good
		rd.SelectionProbability = &bad
		if err := rd.Validate(); err == nil || !strings.Contains(err.Error(), "selection_probability") {
			t.Errorf("SelectionProbability %v error = %v, want naming selection_probability", bad, err)
		}
	}
	finite := 0.25
	good.SelectionProbability = &finite
	if err := good.Validate(); err != nil {
		t.Errorf("finite values: %v", err)
	}
	if v2contract.Digest(good) == "" {
		t.Error("Digest of a validated record is empty")
	}
}

func TestEvidenceRequiredIdentity(t *testing.T) {
	t.Parallel()
	rd := evidenceRoundTrip[v2contract.RoutingDecision](t, "records/routing_decision.golden.json")
	dd := evidenceRoundTrip[v2contract.DesignDecision](t, "records/design_decision.golden.json")
	ar := evidenceRoundTrip[v2contract.Artifact](t, "records/artifact.golden.json")
	ob := evidenceRoundTrip[v2contract.Observation](t, "records/observation.golden.json")
	ve := evidenceRoundTrip[v2contract.Verification](t, "records/verification.golden.json")
	tests := []struct {
		name    string
		mutate  func() error
		wantErr string
	}{
		{"routing decision_id", func() error { r := rd; r.DecisionID = ""; return r.Validate() }, "decision_id"},
		{"routing run_id", func() error { r := rd; r.RunID = ""; return r.Validate() }, "run_id"},
		{"routing selected_route", func() error { r := rd; r.SelectedRoute = ""; return r.Validate() }, "selected_route"},
		{"design decision_id", func() error { r := dd; r.DecisionID = ""; return r.Validate() }, "decision_id"},
		{"design owner", func() error { r := dd; r.Owner = ""; return r.Validate() }, "owner"},
		{"artifact artifact_id", func() error { r := ar; r.ArtifactID = ""; return r.Validate() }, "artifact_id"},
		{"artifact repo_id", func() error { r := ar; r.RepoID = ""; return r.Validate() }, "repo_id"},
		{"observation observation_id", func() error { r := ob; r.ObservationID = ""; return r.Validate() }, "observation_id"},
		{"observation source_id", func() error { r := ob; r.SourceID = ""; return r.Validate() }, "source_id"},
		{"observation observed_at zero", func() error { r := ob; r.ObservedAt = time.Time{}; return r.Validate() }, "observed_at"},
		{"verification verification_id", func() error { r := ve; r.VerificationID = ""; return r.Validate() }, "verification_id"},
		{"verification candidate", func() error { r := ve; r.Candidate = v2contract.GitObject{}; return r.Validate() }, "candidate"},
		{"verification verifier_id", func() error { r := ve; r.VerifierID = ""; return r.Validate() }, "verifier_id"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.mutate(); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

// I20 (v2 §2): artifacts are content-addressed; identity is exactly 64 lowercase hex.
func TestArtifactRequiresContentIdentity(t *testing.T) {
	t.Parallel()
	good := evidenceRoundTrip[v2contract.Artifact](t, "records/artifact.golden.json")
	tests := []struct {
		name    string
		sha     string
		wantErr bool
	}{
		{"empty", "", true},
		{"non-hex", strings.Repeat("g", 64), true},
		{"uppercase hex", strings.Repeat("A", 64), true},
		{"too short", strings.Repeat("a", 63), true},
		{"too long", strings.Repeat("a", 65), true},
		{"64 lowercase hex", strings.Repeat("a", 64), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := good
			a.SHA256 = tc.sha
			err := a.Validate()
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "sha256") {
				t.Fatalf("Validate error = %v, want naming sha256", err)
			}
		})
	}
	enc, _ := json.Marshal(good)
	if strings.Contains(string(enc), "valid_until") {
		t.Errorf("absent ValidUntil encoded: %s", enc)
	}
}

// I09 (v2 §2): null or missing size_bytes is unknown, never 0; 0 is a real size.
func TestArtifactSizeBytesPresence(t *testing.T) {
	t.Parallel()
	golden := string(bytes.TrimSpace(loadGolden(t, "records/artifact.golden.json")))
	const withZero = `"size_bytes":0,`
	if !strings.Contains(golden, withZero) {
		t.Fatalf("golden lacks %s", withZero)
	}
	for name, in := range map[string]string{
		"null":    strings.Replace(golden, withZero, `"size_bytes":null,`, 1),
		"missing": strings.Replace(golden, withZero, "", 1),
	} {
		_, err := v2contract.Decode[v2contract.Artifact]([]byte(in))
		if err == nil || !strings.Contains(err.Error(), "size_bytes") {
			t.Errorf("%s: error = %v, want naming size_bytes", name, err)
		}
	}
	got := evidenceRoundTrip[v2contract.Artifact](t, "records/artifact.golden.json")
	if got.SizeBytes == nil || *got.SizeBytes != 0 {
		t.Errorf("SizeBytes = %v, want pointer to 0", got.SizeBytes)
	}
	neg := int64(-1)
	got.SizeBytes = &neg
	if err := got.Validate(); err == nil || !strings.Contains(err.Error(), "size_bytes") {
		t.Errorf("negative size error = %v, want naming size_bytes", err)
	}
}
