//go:build !windows

package supervisor

func retryIdentityRead(error) bool { return false }
