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
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseWriter := func() { releaseOnce.Do(func() { close(release) }) }
	viewDone := a.done
	view, err := r.views.open(t.Context(), "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	var producers sync.WaitGroup
	t.Cleanup(func() {
		// Release the synthetic writer even when an assertion fails, then join
		// it and the acknowledged producers before the runtime closes.
		releaseWriter()
		view.close()
		joined := make(chan struct{})
		go func() { <-viewDone; producers.Wait(); close(joined) }()
		select {
		case <-joined:
		case <-time.After(2 * time.Second):
			t.Error("observer fixture cleanup did not join")
		}
		_ = reader.Close()
	})
	go func() {
		defer close(viewDone)
		select {
		case <-reader.Wakeups():
		case <-view.done:
			return
		}
		_, _ = reader.DrainHints(16)
		close(entered)
		// No timer may unblock the writer while the request loop is running.
		// Only explicit release after all acknowledgments (or cleanup) does.
		<-release
	}()
	prompt := func(name string, call func(context.Context) error) {
		t.Helper()
		// Design §8 gives each request two seconds, not one shared budget
		// across all eight commits and their independent control requests.
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		deadline, _ := ctx.Deadline()
		type completion struct {
			err      error
			finished time.Time
		}
		ack := make(chan completion, 1)
		producers.Go(func() {
			err := call(ctx)
			ack <- completion{err: err, finished: time.Now()}
		})
		var result completion
		select {
		case result = <-ack:
		case <-ctx.Done():
			// Coordinator scheduling cannot turn an on-time buffered result
			// into a failure, or let a late nil result win a ready-case race.
			select {
			case result = <-ack:
			default:
				t.Fatalf("blocked observer delayed %s", name)
			}
		}
		if result.finished.After(deadline) {
			t.Fatalf("blocked observer delayed %s", name)
		}
		if result.err != nil {
			t.Fatalf("%s: %v", name, result.err)
		}
	}
	prompt("initial committed", func(context.Context) error { a.sink.Committed("run_observer", 0); return nil })
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("observer consumer never entered blocked writer")
	}
	now := time.Now().UTC()
	prompt("InsertRun", func(ctx context.Context) error {
		return r.journal.Transact(ctx, func(tx *sql.Tx) error {
			return journal.InsertRun(ctx, tx, journal.RunRow{RunID: "run_observer", State: "created", AdapterID: "fake", SourceRepo: "synthetic", TaskSHA256: "synthetic", BillingPosture: "local-scripted", ExecutionProfile: "trusted-host", CreatedAt: now, UpdatedAt: now})
		})
	})
	for seq := int64(1); seq <= 8; seq++ {
		payload := json.RawMessage(`{"synthetic":true}`)
		ev := journal.Event{SchemaVersion: 2, EventID: ids.New("evt"), RunID: "run_observer", ProducerID: "sup_synthetic", ProducerSequence: seq, Generation: 1, ObservedAt: now, Type: "service.diagnostic", Payload: payload}
		// Canonical synthetic commits test transport isolation, not diagnostic
		// semantic validation (the supervisor's closed producer tests own that).
		prompt("canonical Append", func(ctx context.Context) error {
			return r.journal.Append(ctx, ev, nil)
		})
		prompt("Committed", func(context.Context) error { a.hooks().Event(ev); return nil })
		prompt("Hint", func(context.Context) error { a.sink.Hint(supervisor.NoticeRenderRefresh); return nil })
		prompt("status Dispatch", func(ctx context.Context) error {
			_, err := r.server.Dispatch(ctx, control.Peer{SameUser: true}, control.Intent{OperationID: ids.New("op"), Method: "status"})
			return err
		})
		prompt("stop rejection Dispatch", func(ctx context.Context) error {
			_, err := r.server.Dispatch(ctx, control.Peer{SameUser: true}, control.Intent{OperationID: ids.New("op"), Method: "stop"})
			var ce *control.Error
			if !errors.As(err, &ce) || ce.Code != control.CodeCapabilityUnsupported {
				return fmt.Errorf("stop rejection=%w", err)
			}
			return nil
		})
	}
	prompt("canonical Events", func(ctx context.Context) error {
		events, err := r.journal.Events(ctx, "run_observer", 0)
		if err != nil {
			return fmt.Errorf("read canonical commits: %w", err)
		}
		if len(events) != 8 {
			return fmt.Errorf("commits=%d, want 8", len(events))
		}
		return nil
	})
	select {
	case <-viewDone:
		t.Fatal("observer writer escaped before all acknowledgments")
	default:
	}
	releaseWriter()
}
