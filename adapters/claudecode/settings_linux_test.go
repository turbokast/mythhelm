//go:build linux

package claudecode

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSettingsNeverReadNativeCredentialSymlink(t *testing.T) {
	t.Parallel()
	home, workspace := t.TempDir(), t.TempDir()
	config := filepath.Join(home, ".claude")
	target := filepath.Join(config, ".credentials.json")
	writeConfig(t, home, filepath.Join(".claude", ".credentials.json"), `{"nativeSubscriptionSession":"planted-secret-value"}`)
	if err := os.Symlink(target, filepath.Join(config, "settings.json")); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Close(fd) }()
	if _, err = unix.InotifyAddWatch(fd, target, unix.IN_OPEN|unix.IN_ACCESS); err != nil {
		t.Fatal(err)
	}
	_, inventoryErr := inventorySettings(home, workspace, config, filepath.Join(home, ".claude.json"), "", nil)
	if inventoryErr == nil {
		t.Error("symlinked native config unexpectedly admitted")
	}
	var events [4096]byte
	n, readErr := unix.Read(fd, events[:])
	if n > 0 || !errors.Is(readErr, unix.EAGAIN) {
		t.Fatalf("inventory opened/read native credential target: %d event bytes, %v", n, readErr)
	}
}
