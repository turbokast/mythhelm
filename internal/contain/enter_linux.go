package contain

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

type sysOps struct{}

func (sysOps) Private() error {
	return unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, "")
}

// ReadOnlyTree marks / and every mount beneath it read-only. A plain
// MS_REMOUNT|MS_BIND ignores MS_REC, so child mounts such as /dev/shm would
// stay writable.
func (sysOps) ReadOnlyTree() error {
	return unix.MountSetattr(unix.AT_FDCWD, "/", unix.AT_RECURSIVE, &unix.MountAttr{Attr_set: unix.MOUNT_ATTR_RDONLY})
}

func (sysOps) Tmpfs(dir string, mode uint32) error {
	return unix.Mount("tmpfs", dir, "tmpfs", unix.MS_NOSUID|unix.MS_NODEV, fmt.Sprintf("mode=%04o", mode))
}

func (sysOps) Release() (string, error) {
	var uts unix.Utsname
	if err := unix.Uname(&uts); err != nil {
		return "", err
	}
	return unix.ByteSliceToString(uts.Release[:]), nil
}

// nsSysProcAttr starts a child in new user and mount namespaces with the
// caller mapped to root inside them.
func nsSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Cloneflags:  syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS,
		UidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}},
		GidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}},
	}
}

// ProbeLinux runs the boundary's setup in a throwaway child and reports its
// kernel version, or the step that failed.
func ProbeLinux() Availability {
	exe, err := os.Executable()
	if err != nil {
		return Availability{Reason: "cannot locate the executable for the namespace trial: " + err.Error()}
	}
	cmd := exec.Command(exe, "__contain") //nolint:gosec // G204: re-executes this binary
	cmd.Env = append(os.Environ(), ProbeEnv+"=1")
	cmd.SysProcAttr = nsSysProcAttr()
	out, err := cmd.CombinedOutput()
	return probeResult(out, err)
}

// RunProbeChild is the probe child's body: run the setup against a scratch
// tmpfs and print the kernel release. It returns the process exit code.
func RunProbeChild() int {
	version, err := probeChild(sysOps{})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(version)
	return 0
}

// EnterLinux builds the boundary around the current process and execs the
// native. The caller must already be in new user and mount namespaces
// (nsSysProcAttr). It returns only on failure.
func EnterLinux(spec ContainSpec) error {
	// Capabilities and NO_NEW_PRIVS are per-thread, and the exec below must
	// run on the thread that dropped them.
	runtime.LockOSThread()

	p := spec.Policy
	home, workdir, targets, err := resolveLayout(spec)
	if err != nil {
		return err
	}

	// Detached copies are taken before /tmp and $HOME are covered by tmpfs,
	// because the workdir or a bind source may live beneath either.
	workTree, err := cloneTree(workdir)
	if err != nil {
		return fmt.Errorf("workdir: %w", err)
	}
	binds := make([]int, len(p.AuthBinds))
	for i, b := range p.AuthBinds {
		srcDir, err := filepath.EvalSymlinks(filepath.Dir(b.Source))
		if err != nil {
			return fmt.Errorf("contain: resolve auth bind source %q: %w", b.Source, err)
		}
		if within(filepath.Join(srcDir, filepath.Base(b.Source)), workdir) {
			return fmt.Errorf("contain: auth bind source %q lies inside the workdir, which is mounted unmasked", b.Source)
		}
		if binds[i], err = cloneFile(b.Source); err != nil {
			return fmt.Errorf("auth bind %q: %w", b.Source, err)
		}
	}

	ops := sysOps{}
	if err := isolateRoot(ops); err != nil {
		return err
	}
	for _, b := range p.AuthBinds {
		if err := mask(b.Source); err != nil {
			return fmt.Errorf("mask auth source %q: %w", b.Source, err)
		}
	}
	for _, t := range []struct {
		dir  string
		mode uint32
	}{{"/tmp", 0o1777}, {home, 0o700}} {
		if err := os.MkdirAll(t.dir, 0o700); err != nil {
			return err
		}
		if err := ops.Tmpfs(t.dir, t.mode); err != nil {
			return fmt.Errorf("tmpfs %s: %w", t.dir, err)
		}
	}
	if err := attach(workTree, workdir, p.ReadOnly, true); err != nil {
		return fmt.Errorf("workdir: %w", err)
	}
	for i, b := range p.AuthBinds {
		if err := attach(binds[i], targets[i], p.ReadOnly, false); err != nil {
			return fmt.Errorf("auth bind %q: %w", b.Source, err)
		}
	}

	dir := spec.Dir
	if dir == "" {
		dir = workdir
	}
	if err := os.Chdir(dir); err != nil {
		return err
	}
	if err := dropPrivileges(); err != nil {
		return err
	}
	return syscall.Exec(spec.Path, spec.Args, spec.Env) //nolint:gosec // G204: the native admitted by the supervisor
}

// resolveLayout resolves symlinks in $HOME, the workdir and /tmp, so every
// overlap check and mount below works on the real paths. All three must exist.
// It returns the resolved HOME and workdir and each auth bind's target
// rebased under the resolved HOME.
func resolveLayout(spec ContainSpec) (home, workdir string, targets []string, err error) {
	envHome := envValue(spec.Env, "HOME")
	if envHome == "" || !filepath.IsAbs(envHome) {
		return "", "", nil, errors.New("contain: spec env needs an absolute HOME")
	}
	if home, err = filepath.EvalSymlinks(envHome); err != nil {
		return "", "", nil, fmt.Errorf("contain: resolve HOME: %w", err)
	}
	if workdir, err = filepath.EvalSymlinks(spec.Policy.Workdir); err != nil {
		return "", "", nil, fmt.Errorf("contain: resolve workdir: %w", err)
	}
	tmp, err := filepath.EvalSymlinks("/tmp")
	if err != nil {
		return "", "", nil, fmt.Errorf("contain: resolve /tmp: %w", err)
	}
	if within(home, tmp) || within(tmp, home) {
		return "", "", nil, fmt.Errorf("contain: HOME %q must lie outside /tmp, which gets its own tmpfs", home)
	}
	if within(home, workdir) || within(tmp, workdir) {
		return "", "", nil, fmt.Errorf("contain: workdir %q encloses $HOME or /tmp, whose tmpfs the workdir mount would cover", workdir)
	}
	for _, b := range spec.Policy.AuthBinds {
		target, err := rebase(b.Target, filepath.Clean(envHome), home)
		if err != nil {
			return "", "", nil, fmt.Errorf("contain: auth bind target %q is outside $HOME", b.Target)
		}
		targets = append(targets, target)
	}
	return home, workdir, targets, nil
}

// rebase moves target from beneath either spelling of HOME to beneath the
// resolved one.
func rebase(target, lexical, resolved string) (string, error) {
	for _, base := range []string{resolved, lexical} {
		if within(target, base) {
			rel, err := filepath.Rel(base, target)
			if err != nil {
				return "", err
			}
			return filepath.Join(resolved, rel), nil
		}
	}
	return "", errors.New("outside")
}

func envValue(env []string, key string) string {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			return v
		}
	}
	return ""
}

func cloneTree(path string) (int, error) {
	return unix.OpenTree(unix.AT_FDCWD, path, unix.OPEN_TREE_CLONE|unix.OPEN_TREE_CLOEXEC)
}

// cloneFile detaches a copy of one regular file. The file is opened without
// following a terminal symlink and checked through the descriptor, and that
// same descriptor is cloned, so the path cannot change between check and use.
func cloneFile(path string) (int, error) {
	fd, err := unix.Open(path, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	defer func() { _ = unix.Close(fd) }()
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return -1, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		return -1, fmt.Errorf("%s is not a regular file", path)
	}
	return unix.OpenTree(fd, "", unix.OPEN_TREE_CLONE|unix.OPEN_TREE_CLOEXEC|unix.AT_EMPTY_PATH)
}

// mask hides an authorised source at its original path, so only the bind at
// its target exposes it. The read-only root forbids writing a replacement,
// but a mount over the file needs no write.
func mask(path string) error {
	null, err := cloneTree("/dev/null")
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(null) }()
	return unix.MoveMount(null, "", unix.AT_FDCWD, path, unix.MOVE_MOUNT_F_EMPTY_PATH)
}

// attach mounts a detached tree at target. A directory target that does not
// exist is created, which only succeeds inside a tmpfs; a file target is
// created empty.
func attach(fd int, target string, readonly, dir bool) error {
	defer func() { _ = unix.Close(fd) }()
	if readonly {
		err := unix.MountSetattr(fd, "", unix.AT_EMPTY_PATH, &unix.MountAttr{Attr_set: unix.MOUNT_ATTR_RDONLY})
		if err != nil {
			return err
		}
	}
	if dir {
		if err := os.MkdirAll(target, 0o750); err != nil {
			return err
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // G304: a mount point created inside the $HOME tmpfs
		if err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return unix.MoveMount(fd, "", unix.AT_FDCWD, target, unix.MOVE_MOUNT_F_EMPTY_PATH)
}

// dropPrivileges empties the permitted, effective and inheritable capability
// sets (which also empties the ambient set) and sets NO_NEW_PRIVS, so exec as
// root inside the user namespace cannot regain them.
func dropPrivileges() error {
	hdr := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var data [2]unix.CapUserData
	if err := unix.Capset(&hdr, &data[0]); err != nil {
		return fmt.Errorf("drop capabilities: %w", err)
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("no_new_privs: %w", err)
	}
	return nil
}
