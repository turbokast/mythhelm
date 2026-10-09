// Package billing is the typed vocabulary of the budget ledger (v2 §§2, 7.3):
// usage readings, per-identity totals, run envelopes and the S1-local
// exhaustion codes. It holds pure types and arithmetic; row access lives in
// the journal.
package billing

import (
	"errors"
	"time"

	"github.com/turbokast/mythhelm/internal/qualify"
)

// Reading is one ingested counter value. Exactly one of Cumulative or Delta
// is set; a nil member is unreported, never zero (I09 (v2 §2)). Quantities are
// exact decimal text, so a USD estimate passes through verbatim.
type Reading struct {
	Scope, Unit, Source string
	At                  time.Time
	Cumulative, Delta   *string
	Producer            string
	Sequence            int64
	HasIdentity         bool
}

// ScopeTotal is the total for one reading identity. A nil Total is unknown;
// a nil Components member is unknown.
type ScopeTotal struct {
	Total      *string
	Label      qualify.DatumLabel
	Components map[string]*string
}

// ScopeKey is the full reading identity: totals never merge across units or
// sources, so two identities sharing one scope stay distinct entries.
type ScopeKey struct{ Scope, Unit, Source string }

// Normalized holds one total per matched reading identity.
//
//nolint:misspell // the spec names this type billing.Normalized
type Normalized struct{ Scopes map[ScopeKey]ScopeTotal }

// EventID identifies one producer-sequenced event, so an applied delta is
// never counted twice.
type EventID struct {
	Producer string
	Sequence int64
}

// ErrReadingShape marks a malformed Reading.
var ErrReadingShape = errors.New("billing: malformed reading")

// Code is an S1-local error code carrying the exact v2 §4.5 string.
type Code string

// The exhaustion codes; MH-21's catalogue adopts these strings.
const (
	CodeAllowanceExhausted Code = "allowance_exhausted"
	CodeBudgetExhausted    Code = "budget_exhausted"
)

// Sentinels for the exhaustion codes, matched with errors.Is.
var (
	ErrAllowanceExhausted = errors.New(string(CodeAllowanceExhausted))
	ErrBudgetExhausted    = errors.New(string(CodeBudgetExhausted))
)
