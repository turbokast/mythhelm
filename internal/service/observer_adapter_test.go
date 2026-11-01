package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

func TestObserverDeliveryNeverWaitsForClient(t *testing.T) {
	r := testRuntime(t)
	sink := supervisor.NewObserverSink()
	reader, err := sink.OpenReader()
	if err != nil {
		t.Fatal(err)
	}
	// A literal adapter connects the real sink/reader to a synthetic writer;
	// public observation delivery remains the later Task 10 producer.
	a := &observerAdapter{sink: sink, reader: reader, done: make(chan struct{})}
	entered := make(chan struct{})
	stalled := make(chan struct{})
	viewDone := a.done
	view, err := r.views.open(t.Context(), "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	var producers sync.WaitGroup
	t.Cleanup(func() { view.close(); <-stalled; <-viewDone; producers.Wait(); _ = reader.Close() })
	go func() {
		defer close(viewDone)
		defer close(stalled)
		select {
		case <-reader.Wakeups():
		case <-view.done:
			return
		}
		_, _ = reader.DrainHints(16)
		close(entered)
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-view.done:
		}
	}()
	prompt := func(name string, call func() error) {
		t.Helper()
		ack := make(chan error, 1)
		producers.Go(func() { ack <- call() })
		select {
		case err := <-ack:
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("blocked observer delayed %s", name)
		}
	}
	prompt("initial committed", func() error { a.sink.Committed("run_observer", 0); return nil })
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("observer consumer never entered blocked writer")
	}
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
		prompt("Committed", func() error { a.hooks().Event(ev); return nil })
		prompt("Hint", func() error { a.sink.Hint(supervisor.NoticeRenderRefresh); return nil })
		prompt("status Dispatch", func() error {
			_, err := r.server.Dispatch(ctx, control.Peer{SameUser: true}, control.Intent{OperationID: ids.New("op"), Method: "status"})
			return err
		})
		prompt("stop rejection Dispatch", func() error {
			_, err := r.server.Dispatch(ctx, control.Peer{SameUser: true}, control.Intent{OperationID: ids.New("op"), Method: "stop"})
			var ce *control.Error
			if !errors.As(err, &ce) || ce.Code != control.CodeCapabilityUnsupported {
				return fmt.Errorf("stop rejection=%w", err)
			}
			return nil
		})
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
	view.close()
	<-stalled
	<-viewDone
}
