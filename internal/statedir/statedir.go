// Package statedir locates and creates MYTHHELM's per-user state directory
// (design §5, AC-9.1).
package statedir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Resolve returns the state directory: $MYTHHELM_HOME when set, otherwise
// $XDG_STATE_HOME/mythhelm (default ~/.local/state/mythhelm) on Linux and
// other Unix systems, ~/Library/Application Support/mythhelm on macOS and
// %LocalAppData%\mythhelm on Windows. It does not create the directory.
func Resolve() (string, error) {
	return resolve(runtime.GOOS, os.Getenv, os.UserHomeDir)
}

func resolve(goos string, getenv func(string) string, homeDir func() (string, error)) (string, error) {
	if dir := getenv("MYTHHELM_HOME"); dir != "" {
		if !filepath.IsAbs(dir) {
			return "", fmt.Errorf("MYTHHELM_HOME must be an absolute path, got %q", dir)
		}
		return filepath.Clean(dir), nil
	}
	if goos == "windows" {
		dir := getenv("LocalAppData")
		if !filepath.IsAbs(dir) {
			return "", errors.New("cannot locate the state directory: %LocalAppData% is unset or not absolute; set MYTHHELM_HOME")
		}
		return filepath.Join(dir, "mythhelm"), nil
	}
	// The XDG base directory specification says relative values are invalid
	// and must be ignored.
	if dir := getenv("XDG_STATE_HOME"); goos != "darwin" && filepath.IsAbs(dir) {
		return filepath.Join(dir, "mythhelm"), nil
	}
	home, err := homeDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate the state directory: %w; set MYTHHELM_HOME", err)
	}
	if goos == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "mythhelm"), nil
	}
	return filepath.Join(home, ".local", "state", "mythhelm"), nil
}

// Ensure creates dir, and any missing parents, with mode 0700. An existing
// directory with wider permissions is narrowed to 0700 on Unix. It fails if
// dir is a symlink or not a directory, so it never changes the permissions
// of a directory someone else pointed it at.
func Ensure(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("checking state directory: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("state directory %s is a symlink; set MYTHHELM_HOME to the real directory", dir)
	}
	if !fi.IsDir() {
		return fmt.Errorf("state directory %s is not a directory", dir)
	}
	if runtime.GOOS == "windows" || fi.Mode().Perm() == 0o700 {
		return nil
	}
	// Change the mode through a handle that is verified to be the directory
	// just checked, so a swap for a symlink in between cannot redirect it.
	f, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("opening state directory: %w", err)
	}
	defer func() { _ = f.Close() }()
	opened, err := f.Stat()
	if err != nil {
		return fmt.Errorf("checking state directory: %w", err)
	}
	if !os.SameFile(fi, opened) {
		return fmt.Errorf("state directory %s changed while it was being checked", dir)
	}
	if err := f.Chmod(0o700); err != nil {
		return fmt.Errorf("restricting state directory to 0700: %w", err)
	}
	return nil
}
