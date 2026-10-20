package control_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

func wireRoundTrip[T v2contract.Validator](t *testing.T, in T) T {
	t.Helper()
	f, err := control.Encode(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := control.Decode[T](f)
	if err != nil {
		t.Fatalf("decode %T: %v", in, err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("roundtrip %T differs: %#v != %#v", in, in, out)
	}
	return out
}

func runSpecFixture() control.RunSpec {
	root, _ := filepath.Abs(".")
	return control.RunSpec{SchemaVersion: 2, Repository: filepath.Join(root, "synthetic-repository"), TaskFile: filepath.Join(root, "synthetic-task.md"), TaskSHA256: "sha256:" + strings.Repeat("a", 64), Adapter: "fake", Billing: "subscription-only", Host: "standalone"}
}

func TestRunContractFlagCoverage(t *testing.T) {
	t.Parallel()
	// This inventory fails when admission gains a flag without a wire decision.
	//nolint:gosec // G101: field-name inventory, not credential values.
	pairs := map[string]string{"Repo": "Repository", "TaskFile": "TaskFile", "Adapter": "Adapter", "Billing": "Billing", "ExecutionProfile": "ExecutionProfile", "UseCommitted": "UseCommitted", "Rev": "Revision", "Host": "Host", "Scenario": "Scenario", "TrustProjectConfig": "TrustProjectConfig", "NoChecks": "NoChecks", "KeepGoing": "KeepGoing", "EnvelopeFlags": "Ceilings", "StripCredentialEnv": "StripCredentialEnv", "TrustNativeConfig": "TrustNativeConfig", "DeclareEntitlement": "DeclareEntitlement", "AllowUntestedNativeVersion": "AllowUntestedNativeVersion"}
	excluded := map[string]bool{"StateDir": true, "Env": true, "Confirm": true, "ProbeBoundary": true}
	req, wire := reflect.TypeFor[admission.Request](), reflect.TypeFor[control.RunSpec]()
	for field := range req.Fields() {
		if excluded[field.Name] {
			continue
		}
		name, ok := pairs[field.Name]
		if !ok {
			t.Fatalf("no wire decision for admission.%s", field.Name)
		}
		if _, ok := wire.FieldByName(name); !ok {
			t.Fatalf("missing RunSpec.%s", name)
		}
	}
	spec := runSpecFixture()
	spec.ExecutionProfile = "trusted-host"
	spec.Revision = "HEAD"
	spec.Scenario = "happy"
	spec.UseCommitted = true
	spec.NoChecks = true
	spec.KeepGoing = true
	spec.TrustProjectConfig = "sha256:" + strings.Repeat("b", 64)
	spec.TrustNativeConfig = "sha256:" + strings.Repeat("c", 64)
	spec.DeclareEntitlement = "plan=synthetic,extra-usage=disabled"
	spec.StripCredentialEnv = true
	spec.AllowUntestedNativeVersion = true
	got := wireRoundTrip(t, spec)
	if got.Ceilings.Repairs != nil {
		t.Fatal("unspecified repairs became zero")
	}
	zero := 0
	duration := int64(1)
	spec.Ceilings = control.RunCeilings{ExecutionMillis: &duration, Repairs: &zero, Replans: &zero, TransportRetries: &zero}
	got = wireRoundTrip(t, spec)
	if got.Ceilings.Repairs == nil || *got.Ceilings.Repairs != 0 {
		t.Fatal("explicit zero repairs lost")
	}
	for _, tc := range []struct {
		name   string
		change func(*control.RunSpec)
	}{
		{"missing task digest", func(s *control.RunSpec) { s.TaskSHA256 = "" }}, {"relative task", func(s *control.RunSpec) { s.TaskFile = "task.md" }}, {"host cannot launch", func(s *control.RunSpec) { s.Host = "herdr" }}, {"negative repair", func(s *control.RunSpec) { n := -1; s.Ceilings.Repairs = &n }}, {"zero duration", func(s *control.RunSpec) { n := int64(0); s.Ceilings.ExecutionMillis = &n }}, {"future schema", func(s *control.RunSpec) { s.SchemaVersion = 3 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := spec
			tc.change(&s)
			f, _ := control.Encode(s)
			if _, err := control.Decode[control.RunSpec](f); err == nil {
				t.Fatal("invalid run spec accepted")
			}
		})
	}
}

func observationFixture() control.RunObservation {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return control.RunObservation{SchemaVersion: 2, RunID: "run", Generation: 7, RunRevision: 3, Outcome: control.RunOutcome{RunID: "run", AttemptID: "attempt", State: "executing", AttemptState: "running"}, NextSequence: 2, Events: []journal.Event{
		{SchemaVersion: 1, EventID: "legacy", RunID: "run", ProducerID: "worker", ProducerSequence: 1, RunSequence: 1, ObservedAt: now, Type: "attempt.state_changed", Payload: json.RawMessage(`{"state":"running","legacy_only":true}`)},
		{SchemaVersion: 2, EventID: "canonical", RunID: "run", ProducerID: "service", ProducerSequence: 1, RunSequence: 2, Generation: 7, ObservedAt: now, Type: "service.adopted", Payload: json.RawMessage(`{"schema_version":2}`)},
	}}
}

func TestRunOperationObservationCodec(t *testing.T) {
	t.Parallel()
	start := control.RunAccepted{SchemaVersion: 2, OperationID: "op", RunID: "run", AttemptID: "attempt", Generation: 7, Revision: 1}
	out := control.RunOutcome{RunID: "run", AttemptID: "attempt", State: "completed", AttemptState: "succeeded_native", Settled: true}
	apply := control.ApplyOutcome{RunID: "run", Branch: "result", CandidateCommit: strings.Repeat("a", 40), State: "completed", Applied: true}
	stop := control.StopResult{OperationID: "op", AttemptID: "attempt", State: "confirmed", Ack: control.StopAck{OperationID: "op", Accepted: true, RunRevision: 2}}
	for _, method := range []string{"run.start", "run.recover", "run.apply", "stop"} {
		s := control.OperationStatus{SchemaVersion: 2, OperationID: "op", RunID: "run", Method: method, State: "completed"}
		switch method {
		case "run.start":
			s.Start = &start
			s.Outcome = &out
		case "run.recover":
			s.Action = &control.ActionAccepted{SchemaVersion: 2, OperationID: "op", RunID: "run", Method: method, Generation: 7, Revision: 2}
			s.Outcome = &out
		case "run.apply":
			s.Action = &control.ActionAccepted{SchemaVersion: 2, OperationID: "op", RunID: "run", Method: method, Generation: 7, Revision: 2}
			s.Apply = &apply
		case "stop":
			s.Stop = &stop
		}
		wireRoundTrip(t, s)
		s.Apply = &apply
		s.Outcome = &out
		f, _ := control.Encode(s)
		if _, err := control.Decode[control.OperationStatus](f); err == nil {
			t.Fatalf("incompatible %s union accepted", method)
		}
	}
	wireRoundTrip(t, observationFixture())
	for _, tc := range []struct {
		name   string
		change func(*control.RunObservation)
		code   control.Code
	}{
		{"gapped page", func(o *control.RunObservation) { o.Events[1].RunSequence = 3; o.NextSequence = 3 }, control.CodeInvalidContract},
		{"wrong run", func(o *control.RunObservation) { o.Events[1].RunID = "other" }, control.CodeInvalidContract},
		{"future critical schema", func(o *control.RunObservation) { o.Events[1].SchemaVersion = 3 }, control.CodeCapabilityUnsupported},
		{"malformed legacy", func(o *control.RunObservation) { o.Events[0].ObservedAt = time.Time{} }, control.CodeInvalidContract},
		{"bad cursor", func(o *control.RunObservation) { o.NextSequence = 1 }, control.CodeInvalidContract},
		{"event count", func(o *control.RunObservation) { o.Events = make([]journal.Event, 129) }, control.CodeInvalidContract},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := observationFixture()
			tc.change(&o)
			f, _ := control.Encode(o)
			_, err := control.Decode[control.RunObservation](f)
			if !errors.Is(err, &control.Error{Code: tc.code}) {
				t.Fatalf("error = %v, want %s", err, tc.code)
			}
		})
	}
	for _, r := range []control.ObserveRun{{SchemaVersion: 2, RunID: "run", Generation: 7, AfterSequence: -1}, {SchemaVersion: 2, RunID: "run", Generation: 7, Limit: 129}, {SchemaVersion: 2, RunID: "run", Generation: 7, WaitMillis: 1001}, {SchemaVersion: 2, RunID: "run", Generation: 7, AfterSequence: math.MaxInt64}} {
		f, _ := control.Encode(r)
		if _, err := control.Decode[control.ObserveRun](f); err == nil {
			t.Fatalf("invalid cursor/bounds accepted: %+v", r)
		}
	}
	wireRoundTrip(t, control.ObserveRun{SchemaVersion: 2, RunID: "run", Generation: 7, Limit: 128, WaitMillis: 1000})
}

func TestRunMethodBodiesRoundTrip(t *testing.T) {
	t.Parallel()
	binding := "sha256:" + strings.Repeat("d", 64)
	wireRoundTrip(t, control.PreparedRun{SchemaVersion: 2, PreparationID: "prep", BindingSHA256: binding, Generation: 7, Challenges: []control.AdmissionChallenge{{ID: "challenge", Kind: "native_trust", Text: "Synthetic qualification question", BindingSHA256: binding}}})
	wireRoundTrip(t, control.RunConfirmation{SchemaVersion: 2, PreparationID: "prep", ChallengeID: "challenge", BindingSHA256: binding, Approved: false})
	wireRoundTrip(t, control.StartRun{SchemaVersion: 2, PreparationID: "prep", BindingSHA256: binding})
	wireRoundTrip(t, control.RecoverRun{SchemaVersion: 2, RunID: "run", ExpectedRevision: 3, Generation: 7})
	wireRoundTrip(t, control.ApplyRun{SchemaVersion: 2, RunID: "run", CandidateSHA256: binding, Branch: "review/result", AcceptFlags: true, AcceptUnverified: true, ExpectedRevision: 3, Generation: 7})
	wireRoundTrip(t, control.OperationQuery{SchemaVersion: 2, RunID: "run", OperationID: "op"})
	raw, _ := json.Marshal(runSpecFixture())
	for _, data := range []string{
		strings.Replace(string(raw), `"schema_version":2`, `"schema_version":2,"environment":{}`, 1),
		strings.Replace(string(raw), `"schema_version":2`, `"schema_version":2,"confirm":{}`, 1),
		strings.Replace(string(raw), `"schema_version":2`, `"schema_version":2,"schema_version":2`, 1),
		strings.Replace(string(raw), `"ceilings":{`, `"ceilings":{"repairs":0,"repairs":1,`, 1),
	} {
		if _, err := control.Decode[control.RunSpec](prefixFrame([]byte(data))); err == nil {
			t.Fatal("undocumented request keys accepted")
		}
	}
}

func TestRunClientMethodEnvelopes(t *testing.T) {
	t.Parallel()
	binding := "sha256:" + strings.Repeat("d", 64)
	prepared := control.PreparedRun{SchemaVersion: 2, PreparationID: "prep", BindingSHA256: binding, Generation: 7, Challenges: []control.AdmissionChallenge{}, Ready: true}
	accepted := control.RunAccepted{SchemaVersion: 2, OperationID: "start-op", RunID: "run", AttemptID: "attempt", Generation: 7, Revision: 1}
	methods := []string{}
	conn := &protocolConn{peer: control.Peer{SameUser: true}, request: func(_ context.Context, in control.Intent) (any, error) {
		methods = append(methods, in.Method)
		if in.Method == "status" {
			return struct {
				Generation int64 `json:"generation"`
			}{7}, nil
		}
		if in.Generation == nil || *in.Generation != 7 || in.CapabilityToken != "" {
			t.Fatalf("standalone envelope = %+v", in)
		}
		switch in.Method {
		case "run.prepare":
			var s control.RunSpec
			if err := json.Unmarshal(in.Params, &s); err != nil || s.TaskSHA256 != runSpecFixture().TaskSHA256 {
				t.Fatalf("prepare = %s, %v", in.Params, err)
			}
			return prepared, nil
		case "run.confirm":
			if in.Object != "prep" {
				t.Fatalf("confirm object = %s", in.Object)
			}
			return prepared, nil
		case "run.start":
			if in.OperationID != "start-op" || in.ExpectedRevision != 0 || in.Object != "prep" {
				t.Fatalf("start = %+v", in)
			}
			return accepted, nil
		case "run.recover", "run.apply":
			if in.ExpectedRevision != 3 || in.Object != "run" {
				t.Fatalf("action = %+v", in)
			}
			return control.ActionAccepted{SchemaVersion: 2, OperationID: in.OperationID, RunID: "run", Method: in.Method, Generation: 7, Revision: 4}, nil
		case "run.operation":
			if in.OperationID == "start-op" {
				t.Fatal("query reused mutation operation ID")
			}
			return control.OperationStatus{SchemaVersion: 2, OperationID: "start-op", RunID: "run", Method: "run.start", State: "accepted", Start: &accepted}, nil
		case "run.observe":
			var r control.ObserveRun
			if err := json.Unmarshal(in.Params, &r); err != nil || r.Limit != 64 {
				t.Fatalf("observe default limit = %d, %v", r.Limit, err)
			}
			return observationFixture(), nil
		}
		return nil, errors.New("unexpected standalone method")
	}}
	client := control.NewRunClient(conn)
	p, err := client.Prepare(context.Background(), runSpecFixture())
	if err != nil || p.PreparationID != "prep" || !p.Ready {
		t.Fatalf("prepare = %+v, %v", p, err)
	}
	p, err = client.Confirm(context.Background(), control.RunConfirmation{SchemaVersion: 2, PreparationID: "prep", ChallengeID: "challenge", BindingSHA256: binding, Approved: true})
	if err != nil || !p.Ready {
		t.Fatalf("confirm = %+v, %v", p, err)
	}
	a, err := client.Start(context.Background(), "start-op", 0, control.StartRun{SchemaVersion: 2, PreparationID: "prep", BindingSHA256: binding})
	if err != nil || a.RunID != "run" {
		t.Fatalf("start = %+v, %v", a, err)
	}
	action, err := client.Recover(context.Background(), "recover-op", control.RecoverRun{SchemaVersion: 2, RunID: "run", ExpectedRevision: 3, Generation: 7})
	if err != nil || action.Method != "run.recover" {
		t.Fatalf("recover = %+v, %v", action, err)
	}
	action, err = client.Apply(context.Background(), "apply-op", control.ApplyRun{SchemaVersion: 2, RunID: "run", CandidateSHA256: binding, Branch: "result", ExpectedRevision: 3, Generation: 7})
	if err != nil || action.Method != "run.apply" {
		t.Fatalf("apply = %+v, %v", action, err)
	}
	status, err := client.Operation(context.Background(), control.OperationQuery{SchemaVersion: 2, RunID: "run", OperationID: "start-op"})
	if err != nil || status.State != "accepted" || status.Outcome != nil {
		t.Fatalf("operation = %+v, %v", status, err)
	}
	page, err := client.Observe(context.Background(), control.ObserveRun{SchemaVersion: 2, RunID: "run", Generation: 7})
	if err != nil || page.NextSequence != 2 {
		t.Fatalf("observe = %+v, %v", page, err)
	}
	want := []string{"status", "run.prepare", "run.confirm", "run.start", "run.recover", "run.apply", "status", "run.operation", "run.observe"}
	if !reflect.DeepEqual(methods, want) {
		t.Fatalf("methods = %v", methods)
	}
}

func TestObservationRejectsReplyBeforeCursorAdvance(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(*control.RunObservation)
	}{
		{"first event gap", func(o *control.RunObservation) {
			o.Events[0].RunSequence = 2
			o.Events[1].RunSequence = 3
			o.NextSequence = 3
		}},
		{"wrong generation", func(o *control.RunObservation) { o.Generation = 8 }},
		{"wrong run", func(o *control.RunObservation) {
			o.RunID = "other"
			o.Outcome.RunID = "other"
			for i := range o.Events {
				o.Events[i].RunID = "other"
			}
		}},
		{"empty page advanced", func(o *control.RunObservation) { o.Events = []journal.Event{}; o.NextSequence = 4 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := observationFixture()
			tc.change(&o)
			conn := &protocolConn{peer: control.Peer{SameUser: true}, request: func(context.Context, control.Intent) (any, error) { return o, nil }}
			got, err := control.NewRunClient(conn).Observe(context.Background(), control.ObserveRun{SchemaVersion: 2, RunID: "run", Generation: 7, Limit: 2})
			if err == nil || got.NextSequence != 0 || got.Events != nil {
				t.Fatalf("invalid page released: %+v, %v", got, err)
			}
		})
	}
}
