// Package datum defines shared provenance vocabulary without storage dependencies.
package datum

// Label is the v2 §7.3 data provenance label.
type Label string

// Data provenance labels distinguish missing evidence from measured zero.
const (
	Reported     Label = "reported"
	Observed     Label = "observed"
	Estimated    Label = "estimated"
	UserDeclared Label = "user-declared"
	Unknown      Label = "unknown"
)
