package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

func TestObserverDeliveryNeverWaitsForClient(t *testing.T) {
	r := testRuntime(t)
	a, err := newObserverAdapter()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.close() }()
	stalled := make(chan struct{})
	viewDone := make(chan struct{})
	view, err := r.views.open(t.Context(), "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	defer view.close()
	// A bounded synthetic view writer stays blocked for ten seconds. It has no
	// reference to the actor callbacks or reader lease.
	go func() {
		defer close(viewDone)
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-view.done:
		}
		close(stalled)
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	now := time.Now().UTC()
	if err := r.journal.Transact(ctx, func(tx *sql.Tx) error {
		return journal.InsertRun(ctx, tx, journal.RunRow{RunID: "run_observer", State: "created", AdapterID: "fake", SourceRepo: "synthetic", TaskSHA256: "synthetic", BillingPosture: "local-scripted", ExecutionProfile: "trusted-host", CreatedAt: now, UpdatedAt: now})
	}); err != nil {
		view.close()
		<-viewDone
		t.Fatal(err)
	}
	for seq := int64(1); seq <= 8; seq++ {
		payload := json.RawMessage(`{"synthetic":true}`)
		ev := journal.Event{SchemaVersion: 2, EventID: ids.New("evt"), RunID: "run_observer", ProducerID: "sup_synthetic", ProducerSequence: seq, Generation: 1, ObservedAt: now, Type: "service.diagnostic", Payload: payload}
		// Canonical synthetic commits test transport isolation, not diagnostic
		// semantic validation (the supervisor's closed producer tests own that).
		if err := r.journal.Append(ctx, ev, nil); err != nil {
			view.close()
			<-viewDone
			t.Fatal(err)
		}
		a.hooks().Event(ev)
		a.sink.Hint(supervisor.NoticeRenderRefresh)
		if _, err := r.server.Dispatch(ctx, control.Peer{SameUser: true}, control.Intent{OperationID: ids.New("op"), Method: "status"}); err != nil {
			view.close()
			<-viewDone
			t.Fatal(err)
		}
		if _, err := r.server.Dispatch(ctx, control.Peer{SameUser: true}, control.Intent{OperationID: ids.New("op"), Method: "stop"}); err == nil {
			view.close()
			<-viewDone
			t.Fatal("shell accepted stop")
		}
	}
	if ctx.Err() != nil {
		view.close()
		<-viewDone
		t.Fatal("stalled writer delayed commits/acknowledgments")
	}
	events, err := r.journal.Events(ctx, "run_observer", 0)
	if err != nil || len(events) != 8 {
		view.close()
		<-viewDone
		t.Fatalf("commits=%d err=%v", len(events), err)
	}
	<-stalled
	<-viewDone
}
