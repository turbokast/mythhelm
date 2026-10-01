//go:build !linux

package workers

import "errors"

// OrphanPIDs has no recovery scanner on this platform. Recovery prints the
// attempt marker for manual inspection and keeps ownership unresolved.
func OrphanPIDs(string) ([]int, error) { return nil, errors.ErrUnsupported }
