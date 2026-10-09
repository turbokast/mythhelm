//go:build !linux && !darwin

package control

import "os/exec"

// DefaultTransport reports that this platform has no control transport yet;
// the Windows named-pipe transport is supervisor-service task 4.
func DefaultTransport() (Transport, error) { return nil, ErrSpawnUnsupported }

func detach(*exec.Cmd) error { return ErrSpawnUnsupported }
