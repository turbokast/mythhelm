//go:build windows

package service

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workers"
)

// Real Windows file sharing must leave readiness false until the identity can
// be read; releasing the handle restores the same complete launch observation.
func TestAdoptionFixtureSharingReadiness(t *testing.T) {
	f := newAdoptionFixture(t)
	actualDir := workers.AttemptDir(f.r.journal.StateDir(), f.run, f.attempt)
	identity, err := workers.ReadIdentity(actualDir)
	if err != nil {
		t.Fatal(err)
	}
	original, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	spool, err := os.Open(filepath.Join(actualDir, supervisor.SpoolFile)) // #nosec G304 -- actual fixture spool.
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(io.LimitReader(spool, 64<<10))
	closeErr := spool.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		t.Fatal(err)
	}
	// Preserve actual producer evidence, but isolate the exclusive handle from
	// the running worker and its supervisor's legitimate concurrent readers.
	dir := t.TempDir()
	path := filepath.Join(dir, "worker.json")
	if err := os.WriteFile(path, original, 0600); err != nil { // #nosec G703 -- isolated test copy.
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, supervisor.SpoolFile), data, 0600); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	release := func() {
		if !closed {
			closed = true
			if err := windows.CloseHandle(handle); err != nil {
				t.Error(err)
			}
		}
	}
	t.Cleanup(release)
	ready, err := adoptionFixtureLaunched(t.Context(), dir, f.run, f.attempt, f.proc.Pid)
	if ready || !errors.Is(err, windows.ERROR_SHARING_VIOLATION) || !adoptionFixtureRetryIdentity(err, "windows") {
		t.Fatalf("held identity readiness = %v, %v; want retryable sharing violation", ready, err)
	}
	release()
	ready, err = adoptionFixtureLaunched(t.Context(), dir, f.run, f.attempt, f.proc.Pid)
	if !ready || err != nil {
		t.Fatalf("released identity readiness = %v, %v; want ready", ready, err)
	}
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil { // #nosec G703 -- corrupt only this fixture identity.
		t.Fatal(err)
	}
	ready, err = adoptionFixtureLaunched(t.Context(), dir, f.run, f.attempt, f.proc.Pid)
	if ready || err == nil || adoptionFixtureRetryIdentity(err, "windows") {
		t.Fatalf("corrupt identity readiness = %v, %v; want permanent refusal", ready, err)
	}
}
