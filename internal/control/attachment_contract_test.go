package control_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/control"
)

func attachmentFixture() control.OpenAttachment {
	return control.OpenAttachment{Grant: control.AttachmentGrant{AttachmentID: "attachment", ClientID: "client", RunID: "run", RepositoryID: "repository", Token: "synthetic-token", Generation: 7}, Projection: control.Projection{SchemaVersion: 2, AttachmentID: "attachment", Generation: 7, ProjectionRevision: 2, RunRevision: 3, RunID: "run", RunState: "executing", ReviewState: "unknown", Attention: "none"}}
}

func TestAttachmentContractBounds(t *testing.T) {
	t.Parallel()
	good := attachmentFixture()
	wireRoundTrip(t, good)
	empty := ""
	good.Projection.Label = &empty
	got := wireRoundTrip(t, good)
	if got.Projection.TaskID != nil || got.Projection.Label == nil || *got.Projection.Label != "" {
		t.Fatal("unknown and known empty projection fields collapsed")
	}
	raw, _ := json.Marshal(good)
	for _, tc := range []struct {
		name string
		data string
	}{
		{"unknown field", strings.Replace(string(raw), `"grant":{`, `"grant":{"callback":{},`, 1)},
		{"environment map", strings.Replace(string(raw), `"grant":{`, `"grant":{"environment":{"SYNTHETIC_SECRET":"never-wire"},`, 1)},
		{"duplicate nested key", strings.Replace(string(raw), `"run_id":"run"`, `"run_id":"foreign","run_id":"run"`, 1)},
		{"case alias", strings.Replace(string(raw), `"generation":7`, `"Generation":7`, 1)},
		{"wrong identity", strings.Replace(string(raw), `"attachment_id":"attachment"`, `"attachment_id":"other"`, 1)},
		{"null required field", strings.Replace(string(raw), `"run_id":"run"`, `"run_id":null`, 1)},
		{"oversized frame", strings.Replace(string(raw), `"synthetic-token"`, `"`+strings.Repeat("x", 64<<10)+`"`, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := control.Decode[control.OpenAttachment](prefixFrame([]byte(tc.data))); err == nil {
				t.Fatal("invalid attachment contract accepted")
			}
		})
	}
	for _, payload := range []string{
		`{"operation_id":"op","method":"attachment.open","params":{"run_id":"run","callback":{}}}`,
		`{"operation_id":"op","method":"attachment.open","params":{"run_id":"run","environment":{}}}`,
		`{"operation_id":"op","method":"attachment.open","params":{"run_id":"run","pad":"` + strings.Repeat("x", 64<<10) + `"}}`,
	} {
		if err := control.CheckIngress(prefixFrame([]byte(payload))); err == nil {
			t.Fatal("unsafe attachment ingress accepted")
		}
	}
}

// protocolConn is a synthetic same-user connection. The handler inspects actual
// serialised requests and supplies durable transport envelopes, without a DB.
type protocolConn struct {
	request func(context.Context, control.Intent) (any, error)
	peer    control.Peer
}

func (c *protocolConn) Peer() control.Peer { return c.peer }
func (c *protocolConn) Request(ctx context.Context, frame []byte) ([]byte, error) {
	in, err := control.Decode[control.Intent](frame)
	if err != nil {
		return nil, err
	}
	body, err := c.request(ctx, in)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return control.Encode(control.Result{OperationID: in.OperationID, Body: payload})
}
func (*protocolConn) Receive(context.Context) ([]byte, error) {
	return nil, errors.New("synthetic client only")
}
func (*protocolConn) Respond([]byte) error { return errors.New("synthetic client only") }
func (*protocolConn) Close() error         { return nil }

func TestAttachmentClientUsesOnlyScopedMethods(t *testing.T) {
	t.Parallel()
	good := attachmentFixture()
	methods := []string{}
	conn := &protocolConn{peer: control.Peer{SameUser: true}}
	conn.request = func(ctx context.Context, in control.Intent) (any, error) {
		if d, ok := ctx.Deadline(); !ok || time.Until(d) > 2*time.Second {
			t.Fatal("request lacks bounded deadline")
		}
		methods = append(methods, in.Method)
		if in.Method != "attachment.open" && (in.Generation == nil || *in.Generation != 7 || in.CapabilityToken != good.Grant.Token || in.Object != "run") {
			t.Fatalf("scope envelope = %+v", in)
		}
		switch in.Method {
		case "attachment.open":
			if string(in.Params) != `{"run_id":"run"}` {
				t.Fatalf("open body = %s", in.Params)
			}
			return good, nil
		case "attachment.read":
			return good.Projection, nil
		case "attachment.stop":
			if in.OperationID != "stop-op" || in.ExpectedRevision != 3 {
				t.Fatalf("stop envelope = %+v", in)
			}
			return control.StopAck{OperationID: "stop-op", Accepted: true, RunRevision: 4}, nil
		case "attachment.operation":
			return control.StopResult{OperationID: "stop-op", AttemptID: "attempt", State: "unconfirmed", Reason: "stop_unconfirmed", Ack: control.StopAck{OperationID: "stop-op", Accepted: true, RunRevision: 4}}, nil
		case "attachment.close":
			return struct{}{}, nil
		}
		return nil, errors.New("unexpected method")
	}
	client := control.NewAttachmentClient(conn)
	opened, err := client.Open(context.Background(), "run")
	if err != nil || opened.Grant.RunID != "run" {
		t.Fatalf("open = %+v, %v", opened, err)
	}
	projection, err := client.Read(context.Background(), good.Grant)
	if err != nil || projection.RunRevision != 3 {
		t.Fatalf("read = %+v, %v", projection, err)
	}
	ack, err := client.Stop(context.Background(), good.Grant, control.StopRequest{OperationID: "stop-op", RunID: "run", AttemptID: "attempt", ExpectedRevision: 3, Generation: 7})
	if err != nil || !ack.Accepted || ack.RunRevision != 4 {
		t.Fatalf("stop = %+v, %v", ack, err)
	}
	result, err := client.Operation(context.Background(), good.Grant, "stop-op")
	if err != nil || result.State != "unconfirmed" {
		t.Fatalf("operation = %+v, %v", result, err)
	}
	if err := client.Close(context.Background(), good.Grant); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(methods, []string{"attachment.open", "attachment.read", "attachment.stop", "attachment.operation", "attachment.close"}) {
		t.Fatalf("methods = %v", methods)
	}
	wrong := good.Grant
	wrong.Generation = 8
	if _, err := client.Stop(context.Background(), wrong, control.StopRequest{OperationID: "stop-op", RunID: "run", AttemptID: "attempt", ExpectedRevision: 3, Generation: 7}); err == nil {
		t.Fatal("mismatched generation reached transport")
	}
	if len(methods) != 5 {
		t.Fatal("invalid request had a side effect")
	}
}

func TestObserveSingleFlightAndCancellation(t *testing.T) {
	t.Parallel()
	entered := make(chan struct{})
	var once sync.Once
	conn := &protocolConn{peer: control.Peer{SameUser: true}, request: func(ctx context.Context, _ control.Intent) (any, error) {
		once.Do(func() { close(entered) })
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	client := control.NewRunClient(conn)
	request := control.ObserveRun{SchemaVersion: 2, RunID: "run", Generation: 7, Limit: 2}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := client.Observe(ctx, request); done <- err }()
	<-entered
	if _, err := client.Observe(context.Background(), request); !errors.Is(err, &control.Error{Code: control.CodeInvalidContract}) {
		t.Fatalf("concurrent observe = %v", err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}

func TestAttachmentDuplicateEnvelope(t *testing.T) {
	t.Parallel()
	for _, payload := range []string{
		`{"operation_id":"op","method":"attachment.open","method":"status","params":{"run_id":"run"}}`,
		`{"operation_id":"op","method":"attachment.open","params":{"run_id":"run","run_id":"other"}}`,
	} {
		if err := control.CheckIngress(prefixFrame([]byte(payload))); err == nil {
			t.Fatalf("duplicate request keys accepted: %s", payload)
		}
	}
}

func TestProjectionSafeVocabulary(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		change func(*control.Projection)
	}{
		{"unknown review vocabulary", func(p *control.Projection) { p.ReviewState = "finished" }},
		{"unknown attention vocabulary", func(p *control.Projection) { p.Attention = "urgent" }},
		{"oversized unicode label", func(p *control.Projection) { s := strings.Repeat("界", 161); p.Label = &s }},
		{"terminal escape label", func(p *control.Projection) { s := "synthetic\x1b]0;title\a"; p.Label = &s }},
		{"bidi label", func(p *control.Projection) { s := "synthetic\u202eunsafe"; p.Label = &s }},
		{"c1 label", func(p *control.Projection) { s := "synthetic\u009bunsafe"; p.Label = &s }},
		{"untrimmed label", func(p *control.Projection) { s := " synthetic "; p.Label = &s }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := attachmentFixture().Projection
			tc.change(&p)
			f, _ := control.Encode(p)
			if _, err := control.Decode[control.Projection](f); err == nil {
				t.Fatal("unsafe projection accepted")
			}
		})
	}
	p := attachmentFixture().Projection
	s := strings.Repeat("界", 160)
	p.Label = &s
	wireRoundTrip(t, p)
}

func TestStopResultReasonVocabulary(t *testing.T) {
	t.Parallel()
	for _, reason := range []string{"native_completed", "syntactically_valid_unknown", "native failed: synthetic detail"} {
		result := control.StopResult{OperationID: "op", AttemptID: "attempt", State: "unconfirmed", Reason: reason, Ack: control.StopAck{OperationID: "op", Accepted: true, RunRevision: 3}}
		f, _ := control.Encode(result)
		if _, err := control.Decode[control.StopResult](f); err == nil {
			t.Fatalf("unknown stop reason accepted: %q", reason)
		}
	}
	for _, reason := range []string{"", "stop_unconfirmed", "ownership_unresolved", "persistence_unavailable"} {
		wireRoundTrip(t, control.StopResult{OperationID: "op", AttemptID: "attempt", State: "unconfirmed", Reason: reason, Ack: control.StopAck{OperationID: "op", Accepted: true, RunRevision: 3}})
	}
}
