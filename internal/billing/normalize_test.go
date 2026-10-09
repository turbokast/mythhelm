//nolint:misspell // the spec names Normalize and billing.Normalized
package billing

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/qualify"
)

var t0 = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func cum(scope, unit, source, v string, at time.Time) Reading {
	return Reading{Scope: scope, Unit: unit, Source: source, At: at, Cumulative: new(v)}
}

func delta(scope, unit, source, v string, at time.Time, producer string, seq int64) Reading {
	return Reading{Scope: scope, Unit: unit, Source: source, At: at, Delta: new(v), Producer: producer, Sequence: seq, HasIdentity: true}
}

func totalOf(t *testing.T, n Normalized, k ScopeKey) ScopeTotal {
	t.Helper()
	st, ok := n.Scopes[k]
	if !ok {
		t.Fatalf("no total for %+v; have %+v", k, n.Scopes)
	}
	return st
}

func wantTotal(t *testing.T, st ScopeTotal, want string) {
	t.Helper()
	if st.Total == nil || *st.Total != want {
		got := "<nil>"
		if st.Total != nil {
			got = *st.Total
		}
		t.Fatalf("total = %s, want %s", got, want)
	}
}

var tokens = ScopeKey{"m", "tokens", "native-reported"}

func TestNormalizeNewestCumulativeWins(t *testing.T) {
	rs := []Reading{
		cum("m", "tokens", "native-reported", "100", t0.Add(time.Minute)),
		cum("m", "tokens", "native-reported", "40", t0),
	}
	n, err := Normalize(rs, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantTotal(t, totalOf(t, n, tokens), "100")
}

func TestNormalizeDeltaAppliedOnce(t *testing.T) {
	rs := []Reading{
		delta("m", "tokens", "native-reported", "10", t0, "wrk_a", 1),
		delta("m", "tokens", "native-reported", "5", t0, "wrk_b", 1),
	}
	n, err := Normalize(rs, nil, map[EventID]bool{{"wrk_a", 1}: true})
	if err != nil {
		t.Fatal(err)
	}
	wantTotal(t, totalOf(t, n, tokens), "5")
}

func TestNormalizePreBaselineDeltaSkipped(t *testing.T) {
	rs := []Reading{
		cum("m", "tokens", "native-reported", "100", t0),
		delta("m", "tokens", "native-reported", "7", t0, "wrk_a", 1),
		delta("m", "tokens", "native-reported", "3", t0.Add(-time.Second), "wrk_a", 2),
		delta("m", "tokens", "native-reported", "2", t0.Add(time.Second), "wrk_a", 3),
	}
	n, err := Normalize(rs, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantTotal(t, totalOf(t, n, tokens), "102")
}

func TestNormalizeIdentityLessDeltaEstimated(t *testing.T) {
	anon := Reading{Scope: "m", Unit: "tokens", Source: "native-reported", At: t0, Delta: new("4")}
	n, err := Normalize([]Reading{delta("m", "tokens", "native-reported", "10", t0, "wrk_a", 1), anon}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	st := totalOf(t, n, tokens)
	if st.Label != qualify.Estimated {
		t.Fatalf("label = %q, want estimated", st.Label)
	}
	wantTotal(t, st, "14")
}

func TestNormalizeMissingScopeUnknown(t *testing.T) {
	rs := []Reading{delta("m", "tokens", "native-reported", "10", t0, "wrk_a", 1)}
	n, err := Normalize(rs, []string{"m", "retail-equivalent"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantTotal(t, totalOf(t, n, tokens), "10")
	st := totalOf(t, n, ScopeKey{Scope: "retail-equivalent"})
	if st.Total != nil || st.Label != qualify.DatumUnknown {
		t.Fatalf("missing scope = %+v, want nil total and unknown label", st)
	}
}

func TestNormalizeSameScopeDistinctIdentities(t *testing.T) {
	rs := []Reading{
		cum("m", "tokens", "native-reported", "100", t0),
		cum("m", "USD", "native-reported", "0.5", t0),
		cum("m", "tokens", "declared", "9", t0),
	}
	n, err := Normalize(rs, []string{"m"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Scopes) != 3 {
		t.Fatalf("got %d totals, want 3 distinct: %+v", len(n.Scopes), n.Scopes)
	}
	wantTotal(t, totalOf(t, n, tokens), "100")
	wantTotal(t, totalOf(t, n, ScopeKey{"m", "USD", "native-reported"}), "0.5")
	wantTotal(t, totalOf(t, n, ScopeKey{"m", "tokens", "declared"}), "9")
}

func TestNormalizeRejectsMalformedReadings(t *testing.T) {
	ok := cum("m", "tokens", "s", "1", t0)
	cases := []struct {
		name string
		bad  Reading
	}{
		{"empty scope", cum("", "tokens", "s", "1", t0)},
		{"empty unit", cum("m", "", "s", "1", t0)},
		{"empty source", cum("m", "tokens", "", "1", t0)},
		{"both set", Reading{Scope: "m", Unit: "u", Source: "s", Cumulative: new("1"), Delta: new("1")}},
		{"neither set", Reading{Scope: "m", Unit: "u", Source: "s"}},
		{"non-decimal cumulative", cum("m", "tokens", "s", "1e3", t0)},
		{"float-ish text", cum("m", "tokens", "s", "0.", t0)},
		{"empty quantity", cum("m", "tokens", "s", "", t0)},
		{"non-decimal delta", delta("m", "tokens", "s", "abc", t0, "p", 1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Normalize([]Reading{ok, c.bad}, nil, nil)
			if !errors.Is(err, ErrReadingShape) {
				t.Fatalf("err = %v, want ErrReadingShape", err)
			}
			if !strings.Contains(err.Error(), "reading 1") {
				t.Fatalf("err %q does not name index 1", err)
			}
		})
	}
}

func TestNormalizeDecimalExact(t *testing.T) {
	usd := ScopeKey{"retail-equivalent", "USD", "native-reported"}
	n, err := Normalize([]Reading{cum("retail-equivalent", "USD", "native-reported", "0.37", t0)}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantTotal(t, totalOf(t, n, usd), "0.37")

	n, err = Normalize([]Reading{
		cum("retail-equivalent", "USD", "native-reported", "0.10", t0),
		delta("retail-equivalent", "USD", "native-reported", "0.27", t0.Add(time.Second), "p", 1),
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantTotal(t, totalOf(t, n, usd), "0.37")
}

func TestNormalizeDeltasSumFromZero(t *testing.T) {
	usd := ScopeKey{"retail-equivalent", "USD", "native-reported"}
	n, err := Normalize([]Reading{
		delta("retail-equivalent", "USD", "native-reported", "0.10", t0, "p", 1),
		delta("retail-equivalent", "USD", "native-reported", "0.27", t0, "p", 2),
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantTotal(t, totalOf(t, n, usd), "0.37")
}

func TestNormalizeLabelsDerive(t *testing.T) {
	n, err := Normalize([]Reading{
		cum("retail-equivalent", "USD", "native-reported", "0.37", t0),
		delta("m", "tokens", "native-reported", "10", t0, "p", 1),
		delta("m", "tokens", "native-reported", "5", t0, "p", 2),
		delta("x", "tokens", "native-reported", "1", t0, "p", 3),
		{Scope: "x", Unit: "tokens", Source: "native-reported", At: t0, Delta: new("1")},
	}, []string{"gone"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[ScopeKey]qualify.DatumLabel{
		{"retail-equivalent", "USD", "native-reported"}: qualify.Estimated,
		tokens:                             qualify.Reported,
		{"x", "tokens", "native-reported"}: qualify.Estimated,
		{Scope: "gone"}:                    qualify.DatumUnknown,
	} {
		if got := totalOf(t, n, k).Label; got != want {
			t.Errorf("%+v label = %q, want %q", k, got, want)
		}
	}
}

func TestSplitUsageUnmappedCombinedOnly(t *testing.T) {
	u := adapter.TokenUsage{Input: new(int64(100)), Output: new(int64(50)), CacheRead: new(int64(10)), CacheCreation: new(int64(5))}
	st := SplitUsage("some-other-route", u)
	wantTotal(t, st, "165")
	for name, c := range st.Components {
		if c != nil {
			t.Errorf("component %s = %s, want unknown", name, *c)
		}
	}
}

func TestSplitUsageFirstRouteSplits(t *testing.T) {
	// Values from the modelUsage frame of
	// adapters/claudecode/testdata/streams/success.jsonl (synthetic).
	u := adapter.TokenUsage{Input: new(int64(100)), Output: new(int64(50)), CacheRead: new(int64(10)), CacheCreation: new(int64(5))}
	st := SplitUsage("claude-code", u)
	wantTotal(t, st, "165")
	for name, want := range map[string]string{"output": "50", "cache_read": "10", "cache_creation": "5"} {
		if c := st.Components[name]; c == nil || *c != want {
			t.Errorf("component %s = %v, want %s", name, c, want)
		}
	}
	if c, present := st.Components["reasoning"]; present && c != nil {
		t.Errorf("reasoning = %s, want unknown: the decoder reports no such field", *c)
	}
}

func TestSplitUsageMissingFieldUnknown(t *testing.T) {
	st := SplitUsage("claude-code", adapter.TokenUsage{Input: new(int64(1)), Output: new(int64(2))})
	if st.Total != nil || st.Label != qualify.DatumUnknown {
		t.Fatalf("partial counts = %+v, want nil total and unknown label", st)
	}
	if c := st.Components["output"]; c == nil || *c != "2" {
		t.Fatalf("reported output component lost: %v", c)
	}
	if st.Components["cache_read"] != nil {
		t.Fatalf("unreported cache_read = %v, want nil", st.Components["cache_read"])
	}
}
