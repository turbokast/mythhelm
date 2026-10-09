//go:build !linux && !darwin

package control

// cleanupE2EBinary has nothing to remove: the packaged-binary tests run on
// platforms with a control transport.
func cleanupE2EBinary() {}
