package control

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestRegisterRefusesTransportOnlyAttachmentMethods keeps the three
// transport-only methods out of the ordinary table, so Execute can never
// serve them.
func TestRegisterRefusesTransportOnlyAttachmentMethods(t *testing.T) {
	t.Parallel()
	h := func(context.Context, Peer, Intent) (Result, error) { return Result{}, nil }
	srv := NewServer(nil)
	for _, m := range []string{"attachment.open", "attachment.read", "attachment.close"} {
		if err := srv.Register(m, h); err == nil || !strings.Contains(err.Error(), m) {
			t.Errorf("Register(%q) = %v, want an error naming the method", m, err)
		}
		if err := srv.RegisterLong(m, h); err == nil {
			t.Errorf("RegisterLong(%q) succeeded, want a refusal", m)
		}
	}
	if err := srv.Register("attachment.stop", h); err != nil {
		t.Errorf("Register(attachment.stop) = %v; only open/read/close are transport-only", err)
	}
}

// TestDispatchRefusesTransportOnlyAttachmentMethods: direct Dispatch on an
// ordinary or status server answers capability_unsupported for a registered
// attachment method, while the status server keeps authenticating first.
func TestDispatchRefusesTransportOnlyAttachmentMethods(t *testing.T) {
	t.Parallel()
	ran := 0
	status := NewStatusServer(func(context.Context, Peer, Intent) (Result, error) { ran++; return Result{}, nil })
	if err := status.RegisterAttachment("attachment.read", func(context.Context, Peer, Intent) (*AttachmentResponse, error) {
		ran++
		return NewAttachmentResponse(Result{}, func() {})
	}); err != nil {
		t.Fatal(err)
	}
	_, err := status.Dispatch(t.Context(), sameUser(), intent("op_1", "attachment.read"))
	requireCode(t, err, CodeCapabilityUnsupported)
	_, err = status.Dispatch(t.Context(), Peer{}, intent("op_2", "attachment.read"))
	requireCode(t, err, CodePermissionDenied)
	if ran != 0 {
		t.Fatalf("handlers ran %d times, want 0", ran)
	}
}

// TestServeOrdinaryAndStatusThroughWriterQueue keeps the existing reply
// shapes through the owned writer: replies in request order with their
// operation IDs, a malformed frame as invalid_contract, a foreign peer
// refused by the status server, an unknown method unsupported.
func TestServeOrdinaryAndStatusThroughWriterQueue(t *testing.T) {
	t.Parallel()
	status := NewStatusServer(func(_ context.Context, _ Peer, in Intent) (Result, error) {
		return Result{Body: json.RawMessage(`{"seen":"` + in.OperationID + `"}`)}, nil
	})
	conn := newFakeConn(sameUser(), false)
	f := startServe(t, status, nil, conn)
	conn.send(t, intent("op_s1", "status"))
	conn.send(t, intent("op_s2", "status"))
	conn.send(t, intent("op_x", "nope"))
	conn.inbox <- []byte{0, 0, 0, 1, '{'}
	var got []Result
	for range 4 {
		var r Result
		if err := json.Unmarshal((<-conn.entered)[prefixLen:], &r); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	f.stop(t)
	if got[0].OperationID != "op_s1" || string(got[0].Body) != `{"seen":"op_s1"}` || got[0].Error != nil {
		t.Errorf("first reply %+v", got[0])
	}
	if got[1].OperationID != "op_s2" || string(got[1].Body) != `{"seen":"op_s2"}` || got[1].Error != nil {
		t.Errorf("second reply %+v", got[1])
	}
	if got[2].OperationID != "op_x" || got[2].Error == nil || got[2].Error.Code != CodeCapabilityUnsupported {
		t.Errorf("unknown method reply %+v", got[2])
	}
	if got[3].Error == nil || got[3].Error.Code != CodeInvalidContract {
		t.Errorf("malformed frame reply %+v", got[3])
	}

	foreign := newFakeConn(Peer{UID: 1001, PID: 9}, false)
	g := startServe(t, status, nil, foreign)
	foreign.send(t, intent("op_f", "status"))
	var r Result
	if err := json.Unmarshal((<-foreign.entered)[prefixLen:], &r); err != nil {
		t.Fatal(err)
	}
	g.stop(t)
	if r.OperationID != "op_f" || r.Error == nil || r.Error.Code != CodePermissionDenied || len(r.Body) != 0 {
		t.Errorf("foreign status reply %+v, want permission_denied", r)
	}
}
