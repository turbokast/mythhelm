//nolint:misspell // the spec names Normalize and billing.Normalized
package billing

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/qualify"
)

const retailEquivalentScope = "retail-equivalent"

var decimalText = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// Normalize applies AC-2.1 and AC-2.2: one total per (scope, unit, source)
// identity, the newest cumulative as baseline, then the unapplied deltas
// observed strictly after it. Identity-less deltas are summed but make the
// total Estimated. An expected scope with no readings becomes an unknown
// total keyed {scope, "", ""}.
func Normalize(rs []Reading, expected []string, applied map[EventID]bool) (Normalized, error) {
	groups := map[ScopeKey][]Reading{}
	scopes := map[string]bool{}
	for i, r := range rs {
		if err := checkReading(r); err != nil {
			return Normalized{}, fmt.Errorf("%w: reading %d: %w", ErrReadingShape, i, err)
		}
		k := ScopeKey{r.Scope, r.Unit, r.Source}
		groups[k] = append(groups[k], r)
		scopes[r.Scope] = true
	}

	out := Normalized{Scopes: make(map[ScopeKey]ScopeTotal, len(groups))}
	for k, g := range groups {
		st, err := totalFor(k, g, applied)
		if err != nil {
			return Normalized{}, err
		}
		out.Scopes[k] = st
	}
	for _, s := range expected {
		if !scopes[s] {
			out.Scopes[ScopeKey{Scope: s}] = ScopeTotal{Label: qualify.DatumUnknown}
		}
	}
	return out, nil
}

func checkReading(r Reading) error {
	switch {
	case r.Scope == "" || r.Unit == "" || r.Source == "":
		return errors.New("empty scope, unit or source")
	case (r.Cumulative == nil) == (r.Delta == nil):
		return errors.New("exactly one of cumulative or delta must be set")
	case r.HasIdentity && r.Producer == "":
		return errors.New("identified reading has no producer")
	}
	q := r.Cumulative
	if q == nil {
		q = r.Delta
	}
	if !decimalText.MatchString(*q) {
		return errors.New("quantity is not decimal text")
	}
	return nil
}

func totalFor(k ScopeKey, g []Reading, applied map[EventID]bool) (ScopeTotal, error) {
	var base *Reading
	for i := range g {
		c := &g[i]
		if c.Cumulative == nil {
			continue
		}
		switch {
		case base == nil || c.At.After(base.At):
			base = c
		case c.At.Equal(base.At) && !sameDecimal(*c.Cumulative, *base.Cumulative):
			return ScopeTotal{}, fmt.Errorf("%w: cumulatives %s and %s share one observation time", ErrReadingShape, *base.Cumulative, *c.Cumulative)
		}
	}

	var deltas []string
	seen := map[EventID]string{}
	estimated := k.Scope == retailEquivalentScope
	for _, r := range g {
		if r.Delta == nil || (base != nil && !r.At.After(base.At)) {
			continue
		}
		if r.HasIdentity {
			id := EventID{r.Producer, r.Sequence}
			if applied[id] {
				continue
			}
			if prev, dup := seen[id]; dup {
				if prev != *r.Delta {
					return ScopeTotal{}, fmt.Errorf("%w: event %s/%d repeated with deltas %s and %s", ErrReadingShape, id.Producer, id.Sequence, prev, *r.Delta)
				}
				continue
			}
			seen[id] = *r.Delta
		}
		deltas = append(deltas, *r.Delta)
		estimated = estimated || !r.HasIdentity
	}

	var total string
	switch {
	case base == nil && len(deltas) == 0:
		return ScopeTotal{Label: qualify.DatumUnknown}, nil
	case base == nil:
		total = sumDecimals(deltas)
	case len(deltas) == 0:
		total = *base.Cumulative
	default:
		total = sumDecimals(append([]string{*base.Cumulative}, deltas...))
	}

	label := qualify.Reported
	if estimated {
		label = qualify.Estimated
	}
	return ScopeTotal{Total: &total, Label: label}, nil
}

func sameDecimal(a, b string) bool {
	x, _ := new(big.Rat).SetString(a)
	y, _ := new(big.Rat).SetString(b)
	return x.Cmp(y) == 0
}

// sumDecimals adds validated decimal texts exactly, at the largest scale
// among them.
func sumDecimals(qs []string) string {
	scale := 0
	for _, q := range qs {
		if _, frac, ok := strings.Cut(q, "."); ok {
			scale = max(scale, len(frac))
		}
	}
	sum := new(big.Int)
	for _, q := range qs {
		whole, frac, _ := strings.Cut(q, ".")
		n, _ := new(big.Int).SetString(whole+frac+strings.Repeat("0", scale-len(frac)), 10)
		sum.Add(sum, n)
	}
	s := sum.String()
	if scale == 0 {
		return s
	}
	if len(s) <= scale {
		s = strings.Repeat("0", scale-len(s)+1) + s
	}
	return s[:len(s)-scale] + "." + s[len(s)-scale:]
}

// splitRoutes lists the adapter harness IDs with qualified per-field
// evidence. claude-code: the stream-json result frame carries separate
// inputTokens, outputTokens, cacheReadInputTokens and cacheCreationInputTokens
// counts per model (adapters/claudecode/testdata/streams/success.jsonl,
// synthetic); the API reports input exclusive of cache reads and creation, so
// the four are non-overlapping. No field maps to reasoning.
var splitRoutes = map[string]bool{"claude-code": true}

// SplitUsage totals one model's token usage and, for a route in splitRoutes,
// exposes its non-overlapping components. Any other route gets the combined
// total only, because native cache/reasoning/output counts may overlap
// (v2 §7.3). A nil or negative count is unknown: the total is unknown unless
// all four counts are reported.
func SplitUsage(route string, u adapter.TokenUsage) ScopeTotal {
	st := ScopeTotal{Label: qualify.DatumUnknown, Components: map[string]*string{
		"output": nil, "cache_read": nil, "cache_creation": nil, "reasoning": nil,
	}}
	in, out, cr, cc := reported(u.Input), reported(u.Output), reported(u.CacheRead), reported(u.CacheCreation)
	if in != nil && out != nil && cr != nil && cc != nil {
		sum := new(big.Int).Add(big.NewInt(*in), big.NewInt(*out))
		sum.Add(sum, big.NewInt(*cr)).Add(sum, big.NewInt(*cc))
		text := sum.String()
		st.Total, st.Label = &text, qualify.Reported
	}
	if splitRoutes[route] {
		st.Components["output"] = countText(out)
		st.Components["cache_read"] = countText(cr)
		st.Components["cache_creation"] = countText(cc)
	}
	return st
}

// reported keeps a count only if it is a plausible measurement: nil and
// negative counts are unknown.
func reported(n *int64) *int64 {
	if n == nil || *n < 0 {
		return nil
	}
	return n
}

func countText(n *int64) *string {
	if n == nil {
		return nil
	}
	s := strconv.FormatInt(*n, 10)
	return &s
}
