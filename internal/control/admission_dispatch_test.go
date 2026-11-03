package control

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPreparedAdmissionVolatileDispatch(t *testing.T) {
	t.Parallel()
	calls := 0
	handler := func(_ context.Context, _ Peer, _ Intent) (Result, error) {
		calls++
		return Result{Revision: 0, Body: json.RawMessage(`{"volatile":true}`)}, nil
	}
	srv := NewServer(nil)
	if err := srv.RegisterPreparedAdmission(handler, handler); err != nil {
		t.Fatal(err)
	}
	in := Intent{OperationID: "op_prepare", Object: "prepare", ExpectedRevision: 0, Method: "run.prepare", Params: json.RawMessage(`{"value":1}`)}
	got, err := srv.Dispatch(t.Context(), Peer{SameUser: true}, in)
	if err != nil || string(got.Body) != `{"volatile":true}` || calls != 1 {
		t.Fatalf("volatile dispatch = %s, %v, calls=%d", got.Body, err, calls)
	}
	for _, bad := range []struct {
		name  string
		peer  Peer
		token string
	}{{"foreign peer", Peer{}, ""}, {"capability token", Peer{SameUser: true}, "attachment-token"}} {
		t.Run(bad.name, func(t *testing.T) {
			attack := in
			attack.CapabilityToken = bad.token
			_, err := srv.Dispatch(t.Context(), bad.peer, attack)
			requireCode(t, err, CodePermissionDenied)
			if calls != 1 {
				t.Fatal("handler entered before authentication")
			}
		})
	}
	if err := srv.RegisterPreparedAdmission(handler, handler); err == nil {
		t.Fatal("duplicate registration accepted")
	}
}

func TestPreparedAdmissionDigestClosed(t *testing.T) {
	t.Parallel()
	in := Intent{OperationID: "op_prepare", Object: "prepare", Method: "run.prepare", Params: json.RawMessage(`{"value":1}`)}
	got, err := PreparedAdmissionDigest(in)
	if err != nil || got != intentDigest(in) {
		t.Fatalf("digest = %q, %v", got, err)
	}
	in.Method = "run.start"
	if _, err := PreparedAdmissionDigest(in); err == nil {
		t.Fatal("start accepted by staged digest")
	}
}
