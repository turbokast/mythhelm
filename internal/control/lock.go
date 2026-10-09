// Package control implements the supervisor's local control protocol: the
// per-user instance lock, frame codec, transports, intent server,
// idempotency ledger access and reservation manager (supervisor-service
// design §2).
package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrInstanceHeld reports that another supervisor holds the per-user
// instance lock on this root.
var ErrInstanceHeld = errors.New("control: supervisor instance already running")

// ErrRootConflict reports that a supervisor is active under a different
// MYTHHELM_HOME. It is a refusal, never a second authority.
var ErrRootConflict = errors.New("control: supervisor active under another root")

var errLockHeld = errors.New("lock held")

// lockMetadata is the instance-lock file content: which state root the
// holder serves, which process holds it, when it started, and the boot
// generation fencing stale control generations (design §3).
type lockMetadata struct {
	Root       string    `json:"root"`
	PID        int       `json:"pid"`
	StartedAt  time.Time `json:"started_at"`
	Generation uint64    `json:"generation"`
}

// LockPath returns the stable per-user, per-host instance-lock path. It is
// derived from the OS user identity and a host-local runtime directory —
// never from the state root — so it is identical for every MYTHHELM_HOME
// and never lives under the resolved state root. A lock under the
// selectable root would let two roots hold two locks and silently run two
// authorities (design §3).
func LockPath() (string, error) {
	base := runtimeBaseDir()
	dir := filepath.Join(base, "mythhelm-"+userKey())
	return filepath.Join(dir, "supervisor.lock"), nil
}

// runtimeBaseDir is the host-local, per-user directory holding the lock:
// $XDG_RUNTIME_DIR where set (per-user and host-local by definition),
// otherwise the OS temp dir (per-user on macOS/Windows, user-keyed below
// on Linux).
func runtimeBaseDir() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); filepath.IsAbs(dir) {
		return dir
	}
	return os.TempDir()
}

// AcquireInstance takes the per-user instance lock at LockPath and holds it
// until release is called or the process exits. Mutual exclusion comes from
// the OS file lock (flock on Unix, LockFileEx on Windows, following the
// ownerlock.go precedent); the lock-file metadata only classifies refusals
// and records the boot generation, so a torn or stale metadata read can
// never create a second authority.
//
// A live holder on the same root yields ErrInstanceHeld; a live holder on
// a different root yields ErrRootConflict. A successful file lock proves
// the previous holder released — live or dead — so the acquirer always
// adopts and records a higher boot generation; no separate liveness probe
// gates adoption (a live-but-released recorder under another root must not
// veto, and pid reuse must not false-refuse). release is idempotent.
func AcquireInstance(dir string) (func(), error) {
	path, err := LockPath()
	if err != nil {
		return nil, err
	}
	if err := ensureLockDir(path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // G304: path is built by LockPath, never from caller input
	if err != nil {
		return nil, fmt.Errorf("control: opening instance lock: %w", err)
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		if errors.Is(err, errLockHeld) {
			return nil, classifyHeld(dir, path)
		}
		return nil, fmt.Errorf("control: taking instance lock: %w", err)
	}
	release := sync.OnceFunc(func() {
		_ = unlockFile(f)
		_ = f.Close()
	})
	root := canonicalRoot(dir)
	var gen uint64 = 1
	if meta, ok := readMetadata(path); ok {
		gen = meta.Generation + 1
	}
	if err := writeMetadata(f, lockMetadata{
		Root:       root,
		PID:        os.Getpid(),
		StartedAt:  time.Now().UTC(),
		Generation: gen,
	}); err != nil {
		release()
		return nil, fmt.Errorf("control: recording instance lock: %w", err)
	}
	return release, nil
}

// classifyHeld reports why a lock held by a live supervisor refused dir:
// the metadata root decides between a same-root second instance and a
// conflicting root. Unreadable metadata (a holder that has not recorded
// itself yet) refuses as a held instance.
func classifyHeld(dir, path string) error {
	root := canonicalRoot(dir)
	if meta, ok := readMetadata(path); ok && !sameRoot(meta.Root, root) {
		return fmt.Errorf("%w: supervisor active under %s", ErrRootConflict, meta.Root)
	}
	return fmt.Errorf("%w: %s", ErrInstanceHeld, root)
}

// ensureLockDir creates the lock's parent directory restricted to the
// current user and refuses to use one owned by someone else, so a lock
// under a shared temp dir cannot be pre-created by another user.
func ensureLockDir(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("control: creating instance lock directory: %w", err)
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("control: checking instance lock directory: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("control: instance lock directory %s is a symlink", dir)
	}
	if !fi.IsDir() {
		return fmt.Errorf("control: instance lock directory %s is not a directory", dir)
	}
	if err := checkDirOwner(dir, fi); err != nil {
		return err
	}
	return nil
}

// canonicalRoot resolves dir to a stable absolute form for root comparison
// and lock metadata. Symlinks resolve where possible (macOS temp dirs,
// TMPDIR overrides); an unresolvable dir still canonicalises to its
// absolute cleaned path.
func canonicalRoot(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = filepath.Clean(dir)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

// readMetadata parses the lock file's metadata. It reports false on any
// I/O or parse failure: only the flock holder writes, readers use the
// metadata for classification only, and a torn read must never look like
// an absent holder.
func readMetadata(path string) (lockMetadata, bool) {
	var meta lockMetadata
	raw, err := os.ReadFile(path) //nolint:gosec // G304: path is built by LockPath, never from caller input
	if err != nil {
		return meta, false
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return lockMetadata{}, false
	}
	return meta, true
}

// writeMetadata records the holder in place on the flocked file. The write
// must stay on this open file: replacing the path would move new acquirers
// onto a fresh inode outside this flock.
func writeMetadata(f *os.File, meta lockMetadata) error {
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return err
	}
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return err
	}
	return f.Sync()
}
