//go:build unix && !linux && !darwin

package workers

import (
	"errors"
	"time"
)

// descendantScan is unavailable on Unix systems other than Linux and macOS.
const descendantScan = "unavailable"

// ProcessStartTime is not implemented here, so a worker cannot prove its
// identity and does not run.
func ProcessStartTime(int) (time.Time, error) {
	return time.Time{}, errors.ErrUnsupported
}

func markedPIDs(string) ([]int, error) {
	return nil, errors.ErrUnsupported
}
