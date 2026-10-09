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
	home := envValue(spec.Env, "HOME")
	if home == "" || !filepath.IsAbs(home) {
		return errors.New("contain: spec env needs an absolute HOME")
	}
	workdir := filepath.Clean(p.Workdir)

	// Detached copies are taken before /tmp and $HOME are covered by tmpfs,
	// because the workdir or a bind source may live beneath either.
	workTree, err := cloneTree(workdir)
	if err != nil {
		return fmt.Errorf("workdir: %w", err)
	}
	binds := make([]int, len(p.AuthBinds))
	for i, b := range p.AuthBinds {
		if !within(b.Target, home) {
			return fmt.Errorf("contain: auth bind target %q is outside $HOME", b.Target)
		}
		if binds[i], err = cloneTree(b.Source); err != nil {
			return fmt.Errorf("auth bind %q: %w", b.Source, err)
		}
	}

	ops := sysOps{}
	if err := isolateRoot(ops); err != nil {
		return err
	}
	for dir, mode := range map[string]uint32{"/tmp": 0o1777, home: 0o700} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := ops.Tmpfs(dir, mode); err != nil {
			return fmt.Errorf("tmpfs %s: %w", dir, err)
		}
	}
	if err := attach(workTree, workdir, p.ReadOnly, true); err != nil {
		return fmt.Errorf("workdir: %w", err)
	}
	for i, b := range p.AuthBinds {
		if err := attach(binds[i], b.Target, p.ReadOnly, false); err != nil {
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
