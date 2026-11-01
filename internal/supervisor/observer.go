package supervisor

import (
	"sync"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

// NoticeCode is a closed vocabulary of informational display hints. Durable
// state, authority and terminal outcomes belong in the canonical journal.
type NoticeCode string

// Informational notice codes accepted by ObserverSink.Hint.
const (
	NoticeReservationHeld  NoticeCode = "reservation_held"
	NoticeWorkerReattached NoticeCode = "worker_reattached"
	NoticeRenderRefresh    NoticeCode = "render_refresh"
)

const observerHintCapacity = 16

// HintBatch owns its Codes slice. Counters are cumulative, saturating display
// diagnostics; neither a drain nor a dropped hint advances an event cursor.
type HintBatch struct {
	Codes             []NoticeCode
	Dropped, Rejected uint64
	Closed            bool
}

// ObserverSink delivers bounded wakeups and hints without client I/O or owner
// callbacks. Construct it with NewObserverSink; copies share one lifetime.
// Task 6 supplies its service adapter and durable notice classification.
type ObserverSink struct{ state *observerState }

// observerState.mu guards all mutable sink and reader fields. No client work,
// I/O, goroutine join or blocking channel delivery occurs while it is held.
type observerState struct {
	mu                sync.Mutex
	wakeups           chan struct{}
	hints             [observerHintCapacity]NoticeCode
	head, count       int
	dropped, rejected uint64
	closed            bool
	reader            *observerReaderState
}

type observerReaderState struct {
	sink               *observerState
	done               chan struct{}
	closed, doneClosed bool
}

// ObserverReader is a single-reader lease. Copies share the same lease; closing
// an old copy cannot release or drain a subsequently opened lease.
type ObserverReader struct{ state *observerReaderState }

// NewObserverSink creates a sink with one coalesced wakeup and sixteen hints.
func NewObserverSink() *ObserverSink {
	return &ObserverSink{state: &observerState{wakeups: make(chan struct{}, 1)}}
}

// Committed signals that canonical data may be available. It retains neither
// runID nor sequence: consumers must pull authoritative journal pages.
func (s *ObserverSink) Committed(_ string, _ int64) {
	if s == nil || s.state == nil {
		return
	}
	st := s.state
	st.mu.Lock()
	defer st.mu.Unlock()
	if !st.closed {
		st.wake()
	}
}

// wake is called with mu held. Wakeups is never closed, including at shutdown.
func (st *observerState) wake() {
	select {
	case st.wakeups <- struct{}{}:
	default:
	}
}

// Hint enqueues only the three informational codes, replacing the oldest at
// capacity. Unknown or authority-bearing values count as rejected, not evidence.
func (s *ObserverSink) Hint(code NoticeCode) {
	if s == nil || s.state == nil {
		return
	}
	st := s.state
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.closed {
		return
	}
	switch code {
	case NoticeReservationHeld, NoticeWorkerReattached, NoticeRenderRefresh:
	default:
		saturateObserverCounter(&st.rejected)
		return
	}
	if st.count == observerHintCapacity {
		st.hints[st.head] = code
		st.head = (st.head + 1) % observerHintCapacity
		saturateObserverCounter(&st.dropped)
	} else {
		st.hints[(st.head+st.count)%observerHintCapacity] = code
		st.count++
	}
	st.wake()
}

func saturateObserverCounter(counter *uint64) {
	if *counter != ^uint64(0) {
		*counter++
	}
}

// RequestClose atomically closes delivery and signals the active reader. It
// does not close Wakeups, join execution, or change ownership of any process.
func (s *ObserverSink) RequestClose() {
	if s == nil || s.state == nil {
		return
	}
	st := s.state
	st.mu.Lock()
	defer st.mu.Unlock()
	st.closed = true
	if st.reader != nil {
		st.reader.signalDone()
	}
}

// OpenReader acquires the sole adapter-reader lease. Public observation handlers
// use their own canonical journal pages rather than acquiring this lease.
func (s *ObserverSink) OpenReader() (*ObserverReader, error) {
	if s == nil || s.state == nil {
		return nil, observerError("open_reader", v2contract.CodeInvalidContract, "invalid_contract")
	}
	st := s.state
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.closed {
		return nil, observerError("open_reader", v2contract.CodeDependencyStale, "observer_closed")
	}
	if st.reader != nil {
		return nil, observerError("open_reader", v2contract.CodeRevisionConflict, "observer_reader_active")
	}
	r := &observerReaderState{sink: st, done: make(chan struct{})}
	st.reader = r
	return &ObserverReader{state: r}, nil
}

// Wakeups returns the read-only coalesced signal, which is never closed. Always
// select with Done; a wakeup alone conveys no outcome or event cursor.
func (r *ObserverReader) Wakeups() <-chan struct{} {
	if r == nil || r.state == nil {
		return nil
	}
	return r.state.sink.wakeups
}

// Done closes once for reader or sink closure. An invalid reader is already done.
func (r *ObserverReader) Done() <-chan struct{} {
	if r == nil || r.state == nil {
		done := make(chan struct{})
		close(done)
		return done
	}
	return r.state.done
}

// DrainHints removes up to limit oldest hints and returns an independent copy.
// Zero means sixteen. Sink closure permits final drains; reader closure does not.
func (r *ObserverReader) DrainHints(limit int) (HintBatch, error) {
	if r == nil || r.state == nil || limit < 0 || limit > observerHintCapacity {
		return HintBatch{}, observerError("drain_hints", v2contract.CodeInvalidContract, "invalid_contract")
	}
	rs := r.state
	st := rs.sink
	st.mu.Lock()
	defer st.mu.Unlock()
	if rs.closed || st.reader != rs {
		return HintBatch{}, observerError("drain_hints", v2contract.CodeInvalidContract, "invalid_contract")
	}
	if limit == 0 {
		limit = observerHintCapacity
	}
	count := min(limit, st.count)
	batch := HintBatch{Codes: make([]NoticeCode, count), Dropped: st.dropped, Rejected: st.rejected, Closed: st.closed}
	for i := range count {
		batch.Codes[i] = st.hints[st.head]
		st.hints[st.head] = ""
		st.head = (st.head + 1) % observerHintCapacity
	}
	st.count -= count
	return batch, nil
}

// Close releases only this lease and never waits for or closes the actor sink.
func (r *ObserverReader) Close() error {
	if r == nil || r.state == nil {
		return observerError("close_reader", v2contract.CodeInvalidContract, "invalid_contract")
	}
	rs := r.state
	st := rs.sink
	st.mu.Lock()
	defer st.mu.Unlock()
	if !rs.closed {
		rs.closed = true
		rs.signalDone()
		if st.reader == rs {
			st.reader = nil
		}
	}
	return nil
}

// signalDone requires sink.mu.
func (rs *observerReaderState) signalDone() {
	if !rs.doneClosed {
		close(rs.done)
		rs.doneClosed = true
	}
}

func observerError(operation string, code v2contract.Code, reason string) error {
	return &v2contract.ControlError{Code: code, Owner: "supervisor", OperationID: "observer." + operation, Disposition: code.DefaultDisposition(), NextAction: "use a live observer reader with valid bounds", Detail: map[string]string{"mythhelm.observer/reason": reason}}
}
