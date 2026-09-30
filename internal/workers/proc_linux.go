package workers

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// descendantScan names how escaped descendants are found on Linux.
const descendantScan = "proc-environ"

// clockTicks is USER_HZ, the unit of /proc/<pid>/stat times. It is 100 on
// every architecture Go supports on Linux.
const clockTicks = 100

// ProcessStartTime returns when pid started: boot time (/proc/stat btime)
// plus field 22 of /proc/<pid>/stat. A clock step between two reads shifts
// btime, so a live worker then fails identity checks and is treated as lost:
// the safe direction.
func ProcessStartTime(pid int) (time.Time, error) {
	stat, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, fmt.Errorf("%w: pid %d", ErrNoProcess, pid)
	}
	if err != nil {
		return time.Time{}, err
	}
	// The command name is parenthesised and may hold spaces; fields resume
	// after the last ')': state is field 3, starttime field 22.
	fields := strings.Fields(string(stat[bytes.LastIndexByte(stat, ')')+1:]))
	if len(fields) < 20 {
		return time.Time{}, fmt.Errorf("pid %d: short /proc stat", pid)
	}
	if fields[0] == "Z" {
		// An unreaped orphan cannot run or write. PID 1 may retain its
		// zombie entry indefinitely; recovery must not mistake that for
		// a live worker or escaped writer.
		return time.Time{}, fmt.Errorf("%w: pid %d is a zombie", ErrNoProcess, pid)
	}
	ticks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("pid %d: starttime: %w", pid, err)
	}
	boot, err := bootTime()
	if err != nil {
		return time.Time{}, err
	}
	return boot.Add(time.Duration(ticks) * time.Second / clockTicks), nil
}

func bootTime() (time.Time, error) {
	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}, err
	}
	for line := range strings.Lines(string(stat)) {
		if v, ok := strings.CutPrefix(line, "btime "); ok {
			sec, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				return time.Time{}, fmt.Errorf("/proc/stat btime: %w", err)
			}
			return time.Unix(sec, 0).UTC(), nil
		}
	}
	return time.Time{}, errors.New("/proc/stat has no btime")
}

// markedPIDs lists processes other than this one whose environment holds
// marker. Processes that vanish or cannot be read are skipped.
func markedPIDs(marker string) ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == os.Getpid() {
			continue
		}
		env, err := os.ReadFile(filepath.Join("/proc", e.Name(), "environ"))
		if err == nil && slices.Contains(strings.Split(string(env), "\x00"), marker) {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}
