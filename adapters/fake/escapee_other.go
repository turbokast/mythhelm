//go:build !unix

package fake

import (
	"errors"
	"time"
)

func spawnEscapee(time.Duration) error {
	return errors.New("spawn_escapee needs Unix sessions")
}
