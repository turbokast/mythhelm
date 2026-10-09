//go:build !linux && !darwin && !windows

package control

import "os/exec"

// DefaultTransport reports that this platform has no control transport yet;
// Linux, macOS and Windows have one.
func DefaultTransport() (Transport, error) { return nil, ErrSpawnUnsupported }

func detach(*exec.Cmd) error { return ErrSpawnUnsupported }
