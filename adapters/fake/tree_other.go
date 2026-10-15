//go:build !windows

package fake

import (
	"errors"
	"fmt"
)

func spawnTree(string, string) error {
	return fmt.Errorf("spawn_tree needs Windows: %w", errors.ErrUnsupported)
}

func spawnTreeGrandchild(string, string) error {
	return fmt.Errorf("spawn_tree needs Windows: %w", errors.ErrUnsupported)
}
