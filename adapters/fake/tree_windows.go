//go:build windows

package fake

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

type treeReport struct {
	Role      string     `json:"role"`
	PID       int        `json:"pid,omitempty"`
	StartTime *time.Time `json:"start_time,omitempty"`
	Error     *uint32    `json:"error,omitempty"`
}

// spawnTree deliberately leaves descendants alive for the worker's lifecycle
// tests. These fixtures inherit the attempt environment, just like spawnEscapee.
func spawnTree(reportPath string) error {
	reportPath, err := filepath.Abs(reportPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o750); err != nil {
		return err
	}
	if err := startTreeProcess(reportPath, "child", windows.CREATE_NEW_PROCESS_GROUP, "--tree-child-report", reportPath); err != nil {
		return err
	}
	return startTreeProcess(reportPath, "breakaway", windows.CREATE_BREAKAWAY_FROM_JOB, "--linger", "10m")
}

func spawnTreeGrandchild(reportPath string) error {
	return startTreeProcess(reportPath, "grandchild", 0, "--linger", "10m")
}

func startTreeProcess(reportPath, role string, flags uint32, args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() { _ = null.Close() }()
	p, err := os.StartProcess(exe, append([]string{exe, AgentCommand}, args...), &os.ProcAttr{
		Env:   os.Environ(),
		Files: []*os.File{null, null, null},
		Sys:   &syscall.SysProcAttr{CreationFlags: flags},
	})
	report := treeReport{Role: role}
	if role == "breakaway" {
		var code uint32
		if err != nil {
			var errno syscall.Errno
			if !errors.As(err, &errno) {
				return fmt.Errorf("spawn_tree breakaway: %w", err)
			}
			code = uint32(errno)
		}
		report.Error = &code
	} else if err != nil {
		return fmt.Errorf("spawn_tree %s: %w", role, err)
	}
	if p != nil {
		defer func() { _ = p.Release() }()
		report.PID = p.Pid
		start, err := treeProcessStart(p.Pid)
		if err != nil {
			_ = p.Kill()
			return err
		}
		report.StartTime = &start
	}
	if err := appendTreeReport(reportPath, report); err != nil {
		if p != nil {
			_ = p.Kill()
		}
		return err
	}
	return nil
}

func treeProcessStart(pid int) (time.Time, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, creation.Nanoseconds()).UTC(), nil
}

func appendTreeReport(path string, report treeReport) error {
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304 -- embedded scenario's validated workdir-relative report path
	if err != nil {
		return err
	}
	// One append write per record: the native and its child can report concurrently.
	_, err = f.Write(append(data, '\n'))
	return errors.Join(err, f.Close())
}

func spawnEscapee(time.Duration) error {
	return fmt.Errorf("spawn_escapee needs Unix sessions: %w", errors.ErrUnsupported)
}
