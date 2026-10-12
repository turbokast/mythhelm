//go:build !windows

package workers

// Outside Windows an open never races the beat's atomic replace into a
// sharing violation; every read failure reads stale at once.
func beatReadTransient(error) bool { return false }
