package adapter_test

import (
	"encoding/json"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
)

type fakeProc struct {
	mu     sync.Mutex
	goneOn adapter.StopSignal
	sent   []adapter.StopSignal
	gone   bool
}

func (p *fakeProc) Stdout() io.Reader { return strings.NewReader("") }

func (p *fakeProc) Wait() adapter.NativeExit { return adapter.NativeExit{} }

func (p *fakeProc) Signal(sig adapter.StopSignal) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sent = append(p.sent, sig)
	if sig == p.goneOn {
		p.gone = true
	}
	return nil
}

func (p *fakeProc) GroupGone() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.gone
}

func (p *fakeProc) delivered() []adapter.StopSignal {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]adapter.StopSignal(nil), p.sent...)
}

func shortLadder() []adapter.StopStep {
	g := 40 * time.Millisecond
	return []adapter.StopStep{
		{Signal: adapter.StopInterrupt, Grace: g},
		{Signal: adapter.StopTerminate, Grace: g},
		{Signal: adapter.StopKill, Grace: g},
	}
}

func jsonKeys(t *testing.T, v any) []string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sorted(s ...string) []string {
	sort.Strings(s)
	return s
}

func TestInterruptReportJSONOmitsUnsetAcknowledged(t *testing.T) {
	t.Parallel()
	proc := &fakeProc{goneOn: adapter.StopTerminate}
	rep := adapter.ClimbLadder(t.Context(), proc, shortLadder())

	if rep.Acknowledged != "" {
		t.Fatalf("ClimbLadder set Acknowledged = %q, want unset", rep.Acknowledged)
	}
	if got, want := jsonKeys(t, rep), sorted("confirmed", "sent"); !reflect.DeepEqual(got, want) {
		t.Errorf("report keys = %v, want %v", got, want)
	}

	rep.Acknowledged = "unknown"
	if got, want := jsonKeys(t, rep), sorted("acknowledged", "confirmed", "sent"); !reflect.DeepEqual(got, want) {
		t.Errorf("report keys with Acknowledged set = %v, want %v", got, want)
	}
}

func TestCapabilitiesJSONOmitsUnsetEntries(t *testing.T) {
	t.Parallel()
	base := []string{
		"structured_events", "resume", "live_steer", "approval_bridge",
		"usage_tokens", "quota_remaining", "hard_monetary_limit", "native_subagents",
	}

	tests := []struct {
		name  string
		caps  adapter.Capabilities
		extra []string
	}{
		{name: "new fields unset leave the pre-change key set", caps: adapter.Capabilities{}},
		{
			name:  "setting Reconnect adds only reconnect",
			caps:  adapter.Capabilities{Reconnect: adapter.Unknown},
			extra: []string{"reconnect"},
		},
		{
			name:  "setting ModelMetadata adds only model_metadata",
			caps:  adapter.Capabilities{ModelMetadata: adapter.Unknown},
			extra: []string{"model_metadata"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := sorted(append(append([]string{}, base...), tc.extra...)...)
			if got := jsonKeys(t, tc.caps); !reflect.DeepEqual(got, want) {
				t.Errorf("keys = %v, want %v", got, want)
			}
		})
	}
}

func TestClimbLadderSentAndConfirmed(t *testing.T) {
	t.Parallel()
	all := []adapter.StopSignal{adapter.StopInterrupt, adapter.StopTerminate, adapter.StopKill}
	tests := []struct {
		name          string
		goneOn        adapter.StopSignal
		wantSent      []adapter.StopSignal
		wantConfirmed bool
	}{
		{
			name:          "group that ignores interrupt and terminate is confirmed only after kill",
			goneOn:        adapter.StopKill,
			wantSent:      all,
			wantConfirmed: true,
		},
		{
			name:          "group that ignores interrupt is confirmed after terminate and kill is not sent",
			goneOn:        adapter.StopTerminate,
			wantSent:      all[:2],
			wantConfirmed: true,
		},
		{
			name:          "group that never goes away has every rung sent and stays unconfirmed",
			goneOn:        "never",
			wantSent:      all,
			wantConfirmed: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			proc := &fakeProc{goneOn: tc.goneOn}
			rep := adapter.ClimbLadder(t.Context(), proc, shortLadder())
			if !reflect.DeepEqual(rep.Sent, tc.wantSent) {
				t.Errorf("Sent = %v, want %v", rep.Sent, tc.wantSent)
			}
			if rep.Confirmed != tc.wantConfirmed {
				t.Errorf("Confirmed = %v, want %v", rep.Confirmed, tc.wantConfirmed)
			}
			if got := proc.delivered(); !reflect.DeepEqual(got, tc.wantSent) {
				t.Errorf("signals delivered = %v, want %v", got, tc.wantSent)
			}
		})
	}
}
