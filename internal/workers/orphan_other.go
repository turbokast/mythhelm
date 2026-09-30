//go:build !linux

package workers

// OrphanPIDs has no recovery scanner on this platform. Recovery prints the
// attempt marker for manual inspection and keeps ownership unresolved.
func OrphanPIDs(attemptID string) ([]int, error) { return nil, nil }
