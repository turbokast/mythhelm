package service

import (
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

// observerAdapter owns exactly one actor reader; public views never take it.
// Actor callbacks only signal bounded sink primitives and perform no view I/O.
type observerAdapter struct {
	sink   *supervisor.ObserverSink
	reader *supervisor.ObserverReader
	done   chan struct{}
}

func newObserverAdapter() (*observerAdapter, error) {
	sink := supervisor.NewObserverSink()
	reader, err := sink.OpenReader()
	if err != nil {
		return nil, err
	}
	a := &observerAdapter{sink: sink, reader: reader, done: make(chan struct{})}
	go func() {
		defer close(a.done)
		for {
			select {
			case <-reader.Done():
				return
			case <-reader.Wakeups():
				_, _ = reader.DrainHints(16)
			}
		}
	}()
	return a, nil
}
func (a *observerAdapter) hooks() supervisor.Hooks {
	return supervisor.Hooks{Observer: a.sink, Event: func(e journal.Event) { a.sink.Committed(e.RunID, e.RunSequence) }}
}
func (a *observerAdapter) close() error {
	if a == nil {
		return nil
	}
	a.sink.RequestClose()
	<-a.done
	return a.reader.Close()
}
