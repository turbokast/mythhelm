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
	"unsafe"

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
func spawnTree(workdir, reportPath string) error {
	return (treeFixture{workdir: workdir, reportPath: reportPath}).run()
}

type treeFixture struct {
	workdir, reportPath string
	writeReport         func(*os.Root, string, treeReport) error
}

func (f treeFixture) run() (err error) {
	workdir, err := filepath.Abs(f.workdir)
	if err != nil {
		return err
	}
	root, err := prepareTreeReport(workdir, f.reportPath)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	if f.writeReport == nil {
		f.writeReport = appendTreeReport
	}
	var started []*os.Process
	defer func() {
		for _, p := range started {
			if err != nil {
				err = errors.Join(err, stopTreeProcess(p))
			}
			err = errors.Join(err, p.Release())
		}
	}()
	child, err := startTreeProcess(root, f.reportPath, "child", windows.CREATE_NEW_PROCESS_GROUP, f.writeReport,
		"--workdir", workdir, "--tree-child-report", f.reportPath)
	if err != nil {
		return err
	}
	started = append(started, child)
	breakaway, err := startTreeProcess(root, f.reportPath, "breakaway", windows.CREATE_BREAKAWAY_FROM_JOB, f.writeReport, "--linger", "10m")
	if breakaway != nil {
		started = append(started, breakaway)
	}
	return err
}

func spawnTreeGrandchild(workdir, reportPath string) error {
	root, err := os.OpenRoot(workdir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	p, err := startTreeProcess(root, reportPath, "grandchild", 0, appendTreeReport, "--linger", "10m")
	if p != nil {
		err = errors.Join(err, p.Release())
	}
	return err
}

func startTreeProcess(root *os.Root, reportPath, role string, flags uint32, writeReport func(*os.Root, string, treeReport) error, args ...string) (*os.Process, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = null.Close() }()
	p, err := os.StartProcess(exe, append([]string{exe, AgentCommand}, args...), &os.ProcAttr{
		Env: os.Environ(), Files: []*os.File{null, null, null}, Sys: &syscall.SysProcAttr{CreationFlags: flags},
	})
	report := treeReport{Role: role}
	if role == "breakaway" {
		var code uint32
		if err != nil {
			var errno syscall.Errno
			if !errors.As(err, &errno) {
				return nil, fmt.Errorf("spawn_tree breakaway: %w", err)
			}
			code = uint32(errno)
		}
		report.Error = &code
	} else if err != nil {
		return nil, fmt.Errorf("spawn_tree %s: %w", role, err)
	}
	fail := func(err error) (*os.Process, error) {
		if p != nil {
			err = errors.Join(err, stopTreeProcess(p), p.Release())
		}
		return nil, err
	}
	if p != nil {
		report.PID = p.Pid
		start, err := treeProcessStart(p.Pid)
		if err != nil {
			return fail(err)
		}
		report.StartTime = &start
	}
	if err := writeReport(root, reportPath, report); err != nil {
		return fail(err)
	}
	return p, nil
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

func appendTreeReport(root *os.Root, path string, report treeReport) error {
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}
	f, err := root.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
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

// stopTreeProcess is rollback only: successful fixtures leave ownership to the
// worker. The caller retains p until cleanup finishes, preventing PID reuse.
func stopTreeProcess(p *os.Process) error {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(p.Pid))
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(h) }()
	return stopTreeHandle(h)
}

// stopTreeHandle pins each parent while examining children. A Windows PID is
// not reused until its last handle closes; creation/exit times exclude children
// of an earlier process that used this PID. Stop the parent before enumerating
// so it cannot create another child after the snapshot.
func stopTreeHandle(h windows.Handle) error {
	pid, err := windows.GetProcessId(h)
	if err != nil {
		return err
	}
	state, err := windows.WaitForSingleObject(h, 0)
	if err != nil {
		return err
	}
	if state == uint32(windows.WAIT_TIMEOUT) {
		if err := windows.TerminateProcess(h, 1); err != nil {
			return err
		}
		state, err = windows.WaitForSingleObject(h, 5000)
	}
	if err != nil {
		return err
	}
	if state != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("spawn_tree cleanup wait: %d", state)
	}
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return err
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	var cleanupErr error
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if entry.ParentProcessID != pid || entry.ProcessID == pid {
			continue
		}
		child, openErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, entry.ProcessID)
		if errors.Is(openErr, windows.ERROR_INVALID_PARAMETER) {
			continue
		} // Already gone.
		if openErr != nil {
			cleanupErr = errors.Join(cleanupErr, openErr)
			continue
		}
		var childCreation, childExit, childKernel, childUser windows.Filetime
		timeErr := windows.GetProcessTimes(child, &childCreation, &childExit, &childKernel, &childUser)
		if timeErr != nil {
			cleanupErr = errors.Join(cleanupErr, timeErr)
		} else if childCreation.Nanoseconds() >= creation.Nanoseconds() && childCreation.Nanoseconds() <= exit.Nanoseconds() {
			cleanupErr = errors.Join(cleanupErr, stopTreeHandle(child))
		}
		cleanupErr = errors.Join(cleanupErr, windows.CloseHandle(child))
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		cleanupErr = errors.Join(cleanupErr, err)
	}
	return cleanupErr
}
