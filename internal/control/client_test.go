package control_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/control"
)

func TestPreparedClientLegacyPreparationRefusesWithoutRequest(t *testing.T) {
	calls := 0
	conn := &protocolConn{peer: control.Peer{SameUser: true}, request: func(context.Context, control.Intent) (any, error) {
		calls++
		return nil, errors.New("unexpected request")
	}}
	for _, client := range []control.RunClient{control.NewRunClient(conn), control.NewPreparedRunClient(conn)} {
		_, err := client.Prepare(context.Background(), runSpecFixture())
		var ce *control.Error
		if !errors.As(err, &ce) || ce.Code != control.CodeCapabilityUnsupported || ce.Message != "revisioned_preparation_required" {
			t.Fatalf("legacy Prepare error=%v", err)
		}
		_, err = client.Confirm(context.Background(), control.RunConfirmation{})
		if !errors.As(err, &ce) || ce.Code != control.CodeCapabilityUnsupported || ce.Message != "revisioned_preparation_required" {
			t.Fatalf("legacy Confirm error=%v", err)
		}
	}
	if calls != 0 {
		t.Fatalf("legacy preparation sent %d requests", calls)
	}
}

func TestPreparedClientRevisionedRoundTrip(t *testing.T) {
	binding := "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	conn := &protocolConn{peer: control.Peer{SameUser: true}, request: func(_ context.Context, in control.Intent) (any, error) {
		if in.Method == "status" {
			return struct {
				Generation int64 `json:"generation"`
			}{7}, nil
		}
		if in.OperationID != "prepare-intent" {
			t.Fatalf("operation=%q", in.OperationID)
		}
		return control.PreparedRun{SchemaVersion: 2, PreparationID: "prep", BindingSHA256: binding, Generation: 7, Challenges: []control.AdmissionChallenge{}, Ready: false}, nil
	}}
	reply, err := control.NewPreparedRunClient(conn).PrepareRevisioned(context.Background(), "prepare-intent", runSpecFixture())
	if err != nil || reply.Revision != 0 || reply.Run.PreparationID != "prep" {
		t.Fatalf("prepare=%+v,%v", reply, err)
	}
}

// revisionConn returns independent framed outer envelopes, not a body-only
// convenience shim, so missing metadata reaches the real typed decoder.
type revisionConn struct {
	protocolConn
	handle func(control.Intent) ([]byte, error)
}

func (c *revisionConn) Request(_ context.Context, frame []byte) ([]byte, error) {
	in, err := control.Decode[control.Intent](frame)
	if err != nil {
		return nil, err
	}
	return c.handle(in)
}
func preparationFixture() control.PreparedRun {
	return control.PreparedRun{SchemaVersion: 2, PreparationID: "prep", BindingSHA256: "sha256:" + strings.Repeat("d", 64), Generation: 7, Challenges: []control.AdmissionChallenge{{ID: "retention", Kind: "synthetic", Text: "synthetic approval", BindingSHA256: "sha256:" + strings.Repeat("d", 64)}}, Ready: true}
}
func revisionFrame(in control.Intent, rev int64, body any, ce *control.Error) []byte {
	raw, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	frame, err := control.Encode(control.Result{OperationID: in.OperationID, Revision: rev, Body: raw, Error: ce})
	if err != nil {
		panic(err)
	}
	return frame
}
func newRevisionConn(handle func(control.Intent) ([]byte, error)) *revisionConn {
	c := &revisionConn{handle: handle}
	c.peer = control.Peer{SameUser: true}
	return c
}
func statusFrame(in control.Intent, gen int64) []byte {
	return revisionFrame(in, 0, struct {
		Generation int64 `json:"generation"`
	}{gen}, nil)
}
func confirmationFixture() control.RunConfirmation {
	p := preparationFixture()
	return control.RunConfirmation{SchemaVersion: 2, PreparationID: p.PreparationID, BindingSHA256: p.BindingSHA256, ChallengeID: "retention", Approved: true}
}

func TestPreparedClientServerRevisionsDriveConfirmAndStart(t *testing.T) {
	p := preparationFixture()
	confirm := 0
	conn := newRevisionConn(func(in control.Intent) ([]byte, error) {
		switch in.Method {
		case "status":
			return statusFrame(in, 7), nil
		case "run.prepare":
			if in.OperationID != "prepare" || in.ExpectedRevision != 0 {
				t.Fatalf("prepare envelope=%+v", in)
			}
			return revisionFrame(in, 0, p, nil), nil
		case "run.confirm":
			want := []int64{0, 1, 7}
			returns := []int64{1, 7, 11}
			if confirm >= len(want) || in.ExpectedRevision != want[confirm] || in.OperationID != fmt.Sprintf("confirm%d", confirm) {
				t.Fatalf("confirm envelope=%+v", in)
			}
			rev := returns[confirm]
			confirm++
			return revisionFrame(in, rev, p, nil), nil
		case "run.start":
			if in.ExpectedRevision != 11 || in.OperationID != "start" {
				t.Fatalf("start guessed revision: %+v", in)
			}
			return revisionFrame(in, 1, control.RunAccepted{SchemaVersion: 2, OperationID: "start", RunID: "run", AttemptID: "attempt", Generation: 7, Revision: 1}, nil), nil
		default:
			t.Fatalf("method=%s", in.Method)
			return nil, nil
		}
	})
	client := control.NewPreparedRunClient(conn)
	reply, err := client.PrepareRevisioned(context.Background(), "prepare", runSpecFixture())
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		reply, err = client.ConfirmRevisioned(context.Background(), fmt.Sprintf("confirm%d", i), reply.Revision, confirmationFixture())
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = client.Start(context.Background(), "start", reply.Revision, control.StartRun{SchemaVersion: 2, PreparationID: p.PreparationID, BindingSHA256: p.BindingSHA256}); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedReplyMetadataValidation(t *testing.T) {
	p := preparationFixture()
	raw, _ := json.Marshal(p)
	for _, tc := range []struct{ name, outer string }{
		{"missing", `{"operation_id":"prepare","body":%s}`},
		{"null", `{"operation_id":"prepare","revision":null,"body":%s}`},
		{"negative", `{"operation_id":"prepare","revision":-1,"body":%s}`},
		{"noninitial", `{"operation_id":"prepare","revision":4,"body":%s}`},
		{"operation", `{"operation_id":"foreign","revision":0,"body":%s}`},
		{"duplicate", `{"operation_id":"prepare","revision":0,"revision":0,"body":%s}`},
		{"unknown", `{"operation_id":"prepare","revision":0,"extra":false,"body":%s}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn := newRevisionConn(func(in control.Intent) ([]byte, error) {
				if in.Method == "status" {
					return statusFrame(in, 7), nil
				}
				return prefixFrame(fmt.Appendf(nil, tc.outer, raw)), nil
			})
			out, err := control.NewPreparedRunClient(conn).PrepareRevisioned(context.Background(), "prepare", runSpecFixture())
			if err == nil || out.Run.PreparationID != "" {
				t.Fatalf("bad envelope accepted: %+v,%v", out, err)
			}
		})
	}
	for _, name := range []string{"generation", "binding", "identity", "schema"} {
		t.Run(name, func(t *testing.T) {
			bad := p
			switch name {
			case "generation":
				bad.Generation = 8
			case "binding":
				bad.BindingSHA256 = "sha256:" + strings.Repeat("e", 64)
				bad.Challenges = []control.AdmissionChallenge{}
			case "identity":
				bad.PreparationID = "other"
			case "schema":
				bad.SchemaVersion = 3
			}
			conn := newRevisionConn(func(in control.Intent) ([]byte, error) {
				if in.Method == "status" {
					return statusFrame(in, 7), nil
				}
				return revisionFrame(in, 2, bad, &control.Error{Code: control.CodeRevisionConflict, Message: "stale"}), nil
			})
			out, err := control.NewPreparedRunClient(conn).ConfirmRevisioned(context.Background(), "confirm", 1, confirmationFixture())
			var conflict *control.PreparationConflictError
			if err == nil || errors.As(err, &conflict) || out.Run.PreparationID != "" {
				t.Fatalf("foreign current metadata released: %+v,%v", out, err)
			}
		})
	}
}

func TestPreparedClientLostAckAndConflict(t *testing.T) {
	p := preparationFixture()
	var saved []byte
	writes := 0
	lost := false
	conn := newRevisionConn(func(in control.Intent) ([]byte, error) {
		if in.Method == "status" {
			return statusFrame(in, 7), nil
		}
		if in.OperationID == "retry-same" {
			if in.ExpectedRevision != 0 {
				t.Fatal("retry precondition changed")
			}
			if saved == nil {
				writes++
				saved = revisionFrame(in, 1, p, nil)
			}
			if !lost {
				lost = true
				return nil, io.ErrUnexpectedEOF
			}
			return saved, nil
		}
		if in.OperationID == "conflict" {
			return revisionFrame(in, 9, p, &control.Error{Code: control.CodeRevisionConflict, Message: "stale"}), nil
		}
		if in.OperationID == "foreign" {
			return revisionFrame(in, 9, nil, &control.Error{Code: control.CodePermissionDenied, Message: "refused"}), nil
		}
		t.Fatalf("unexpected op %s", in.OperationID)
		return nil, nil
	})
	client := control.NewPreparedRunClient(conn)
	if _, err := client.ConfirmRevisioned(context.Background(), "retry-same", 0, confirmationFixture()); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("lost ack=%v", err)
	}
	out, err := client.ConfirmRevisioned(context.Background(), "retry-same", 0, confirmationFixture())
	if err != nil || out.Revision != 1 || writes != 1 {
		t.Fatalf("replay=%+v,%v writes=%d", out, err, writes)
	}
	_, err = client.ConfirmRevisioned(context.Background(), "conflict", out.Revision, confirmationFixture())
	var conflict *control.PreparationConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("conflict=%v", err)
	}
	current, ok := conflict.Current()
	if !ok || current.Revision != 9 {
		t.Fatalf("current=%+v,%v", current, ok)
	}
	current.Run.Challenges[0].Text = "mutated"
	again, _ := conflict.Current()
	if again.Run.Challenges[0].Text != "synthetic approval" {
		t.Fatal("Current aliases retained reply")
	}
	historical, err := client.ConfirmRevisioned(context.Background(), "retry-same", 0, confirmationFixture())
	if err != nil || historical.Revision != 1 {
		t.Fatalf("historical replay rewritten: %+v,%v", historical, err)
	}
	_, err = client.ConfirmRevisioned(context.Background(), "foreign", 9, confirmationFixture())
	conflict = nil
	if err == nil || errors.As(err, &conflict) {
		t.Fatalf("foreign current leaked: %v", err)
	}
	// A new connection uses the freshly authenticated service generation. The
	// old preparation cannot supply metadata in that changed generation.
	reconnect := newRevisionConn(func(in control.Intent) ([]byte, error) {
		if in.Method == "status" {
			return statusFrame(in, 8), nil
		}
		if in.Generation == nil || *in.Generation != 8 {
			t.Fatal("cached old generation")
		}
		return revisionFrame(in, 9, p, &control.Error{Code: control.CodeRevisionConflict, Message: "stale"}), nil
	})
	_, err = control.NewPreparedRunClient(reconnect).ConfirmRevisioned(context.Background(), "reconnect", 9, confirmationFixture())
	conflict = nil
	if err == nil || errors.As(err, &conflict) {
		t.Fatalf("reconnect accepted old generation: %v", err)
	}
}

func TestPreparedClientConflictWithoutCurrentPreservesError(t *testing.T) {
	for _, body := range []json.RawMessage{nil, json.RawMessage("null")} {
		t.Run(string(body), func(t *testing.T) {
			conn := newRevisionConn(func(in control.Intent) ([]byte, error) {
				if in.Method == "status" {
					return statusFrame(in, 7), nil
				}
				return control.Encode(control.Result{OperationID: in.OperationID, Revision: 3, Body: body, Error: &control.Error{Code: control.CodeRevisionConflict, Message: "no_current_evidence"}})
			})
			out, err := control.NewPreparedRunClient(conn).ConfirmRevisioned(context.Background(), "confirm", 1, confirmationFixture())
			var ce *control.Error
			var conflict *control.PreparationConflictError
			if !errors.As(err, &ce) || ce.Code != control.CodeRevisionConflict || ce.Message != "no_current_evidence" || errors.As(err, &conflict) || out.Run.PreparationID != "" {
				t.Fatalf("missing current changed ordinary error: %+v,%v", out, err)
			}
		})
	}
}

// Two actual exported clients make new requests against the same synthetic
// preparation revision. Server effects remain a Task8 obligation; this checks
// each client's framing and validated conflict metadata under concurrent calls.
func TestPreparedClientLostAckAndConflictConcurrentCallers(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	p := preparationFixture()
	arrived := make(chan string, 2)
	proceed := make(chan struct{})
	committed := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(proceed)
		}
	}()
	var writes atomic.Int64
	handler := func(in control.Intent) ([]byte, error) {
		if in.Method == "status" {
			return statusFrame(in, 7), nil
		}
		if in.ExpectedRevision != 0 || in.Object != p.PreparationID {
			return nil, fmt.Errorf("changed stale precondition: %+v", in)
		}
		arrived <- in.OperationID
		select {
		case <-proceed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if in.OperationID == "winner" {
			writes.Add(1)
			close(committed)
			return revisionFrame(in, 1, p, nil), nil
		}
		if in.OperationID != "stale" {
			return nil, fmt.Errorf("changed operation ID %q", in.OperationID)
		}
		select {
		case <-committed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return revisionFrame(in, 1, p, &control.Error{Code: control.CodeRevisionConflict, Message: "stale"}), nil
	}
	type answer struct {
		op    string
		reply control.PreparedReply
		err   error
	}
	results := make(chan answer, 2)
	for _, op := range []string{"winner", "stale"} {
		go func() {
			c := control.NewPreparedRunClient(newRevisionConn(handler))
			reply, err := c.ConfirmRevisioned(ctx, op, 0, confirmationFixture())
			results <- answer{op, reply, err}
		}()
	}
	for range 2 {
		select {
		case <-arrived:
		case <-ctx.Done():
			t.Fatal("concurrent requests did not reach framed server")
		}
	}
	close(proceed)
	released = true
	for range 2 {
		select {
		case out := <-results:
			if out.op == "winner" {
				if out.err != nil || out.reply.Revision != 1 {
					t.Fatalf("winner=%+v", out)
				}
			} else {
				var conflict *control.PreparationConflictError
				if !errors.As(out.err, &conflict) {
					t.Fatalf("stale client=%+v", out)
				}
				current, ok := conflict.Current()
				if !ok || current.Revision != 1 || current.Run.BindingSHA256 != p.BindingSHA256 {
					t.Fatalf("current=%+v,%v", current, ok)
				}
			}
		case <-ctx.Done():
			t.Fatal("concurrent reply did not complete")
		}
	}
	if writes.Load() != 1 {
		t.Fatalf("synthetic server writes=%d", writes.Load())
	}
}
