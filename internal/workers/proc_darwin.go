package workers

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"golang.org/x/sys/unix"
)

// descendantScan names how escaped descendants are found on macOS.
const descendantScan = "sysctl-procargs2"

// ProcessStartTime returns when pid started, from kern.proc.pid's
// p_starttime.
func ProcessStartTime(pid int) (time.Time, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	// A PID with no process yields an empty reply, which x/sys reports as
	// EIO. A PID reaped mid-lookup surfaces ESRCH instead.
	if errors.Is(err, unix.EIO) || procGone(err) || err == nil && int(kp.Proc.P_pid) != pid {
		return time.Time{}, fmt.Errorf("%w: pid %d", ErrNoProcess, pid)
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("pid %d: %w", pid, err)
	}
	tv := kp.Proc.P_starttime
	return time.Unix(tv.Sec, int64(tv.Usec)*int64(time.Microsecond)).UTC(), nil
}

// markedPIDs lists processes other than this one whose environment holds
// marker. The kernel returns another process's environment only to its
// owner, which is enough: the native's descendants run as this user.
func markedPIDs(marker string) ([]int, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	var pids []int
	for i := range procs {
		pid := int(procs[i].Proc.P_pid)
		if pid <= 0 || pid == os.Getpid() {
			continue
		}
		buf, err := unix.SysctlRaw("kern.procargs2", pid)
		if err == nil && slices.Contains(procArgsEnv(buf), marker) {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

// procArgsEnv returns the environment in a kern.procargs2 reply: argc as a
// 32-bit integer, the executable path and its NUL padding, argc arguments,
// then the environment, each NUL-terminated and ended by an empty string.
func procArgsEnv(buf []byte) []string {
	if len(buf) < 4 {
		return nil
	}
	argc := int(binary.LittleEndian.Uint32(buf))
	rest := buf[4:]
	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return nil
	}
	rest = bytes.TrimLeft(rest[i:], "\x00")
	for ; argc > 0; argc-- {
		i := bytes.IndexByte(rest, 0)
		if i < 0 {
			return nil
		}
		rest = rest[i+1:]
	}
	var env []string
	for {
		i := bytes.IndexByte(rest, 0)
		if i <= 0 {
			return env
		}
		env = append(env, string(rest[:i]))
		rest = rest[i+1:]
	}
}
