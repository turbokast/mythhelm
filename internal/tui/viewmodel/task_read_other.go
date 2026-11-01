//go:build !windows

package viewmodel

// transientReadErr never retries off Windows: no sharing-mode denial exists there.
func transientReadErr(error) bool { return false }
