//go:build !unix

package workers

import "os"

// openHash opens path for hashing. Non-unix platforms have no POSIX FIFOs
// that block a read-only open, so a plain open applies; hashFile still
// rejects whatever the handle turns out not to be regular.
func openHash(path string) (*os.File, error) {
	return os.Open(path)
}
