package billing

import "time"

// ReserveNote is the label every reserve carries: the S1 reserve is estimated
// data, never provider quota (I10).
const ReserveNote = "estimate, not a reserve of provider quota"

// ReserveEstimate quantifies one verification pass plus the repair ceiling.
// It carries no token quantity: provider quota cannot be reserved in S1.
type ReserveEstimate struct {
	VerifyPassChecks int
	VerifyTimeoutSum time.Duration // sum of the admitted checks' timeouts
	RepairCeiling    int
	Note             string
}

// CheckBound is one admitted check and its timeout. Billing does not import
// admission, so the caller converts its check list.
type CheckBound struct {
	Name    string
	Timeout time.Duration
}

// EstimateReserve quantifies one execution of the check list within its
// timeouts, plus the repair ceiling.
func EstimateReserve(checks []CheckBound, c Ceilings) ReserveEstimate {
	est := ReserveEstimate{VerifyPassChecks: len(checks), RepairCeiling: c.Repairs, Note: ReserveNote}
	for _, check := range checks {
		est.VerifyTimeoutSum += check.Timeout
	}
	return est
}

// ExecutionRemainder is the execution time a run has left.
type ExecutionRemainder struct{ TimeLeft time.Duration }

// RemainderCoversReserve is the gate predicate: the remainder covers the
// reserve iff TimeLeft >= VerifyTimeoutSum. Counts are deliberately absent:
// repairs are completion work drawn from the reserve, and their per-kind
// ceiling is enforced at launch, so gating on it here would allow one repair.
func RemainderCoversReserve(rem ExecutionRemainder, est ReserveEstimate) bool {
	return rem.TimeLeft >= est.VerifyTimeoutSum
}
