//go:build !windows

package workers

// Outside Windows, renaming over an open reader succeeds; every rename
// failure is permanent and fails fast.
func retryIdentityWrite(error) bool { return false }
