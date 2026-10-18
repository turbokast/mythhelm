//go:build windows

package workers

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobObject is held only by the worker, through conclusion (not merely Wait).
// It is lifecycle ownership, not a security boundary.
type jobObject struct{ h windows.Handle }

func newJob() (*jobObject, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, winError("CreateJobObject", err)
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		_ = windows.CloseHandle(h)
		return nil, winError("SetInformationJobObject", err)
	}
	return &jobObject{h: h}, nil
}

func (j *jobObject) assign(h windows.Handle) error {
	return winError("AssignProcessToJobObject", windows.AssignProcessToJobObject(j.h, h))
}
func (j *jobObject) terminate() error {
	return winError("TerminateJobObject", windows.TerminateJobObject(j.h, 1))
}
func (j *jobObject) close() error {
	if j == nil || j.h == 0 {
		return nil
	}
	err := windows.CloseHandle(j.h)
	if err == nil {
		j.h = 0
	}
	return winError("CloseHandle(job)", err)
}

func winError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var code syscall.Errno
	if errors.As(err, &code) {
		return fmt.Errorf("%s: Win32 error %d: %w", operation, uint32(code), err)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func processInJob(h, job windows.Handle) (bool, error) {
	call := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")
	if err := call.Find(); err != nil {
		return false, winError("resolve IsProcessInJob", err)
	}
	var inJob int32
	ok, _, err := call.Call(uintptr(h), uintptr(job), uintptr(unsafe.Pointer(&inJob)))
	if ok == 0 {
		return false, winError("IsProcessInJob", err)
	}
	return inJob != 0, nil
}

func parentJob() (bool, error) {
	inJob, err := processInJob(windows.CurrentProcess(), 0)
	if err != nil || !inJob {
		return false, err
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if err := windows.QueryInformationJobObject(0, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), nil); err != nil {
		return false, winError("QueryInformationJobObject(parent)", err)
	}
	return limits.BasicLimitInformation.LimitFlags&windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE != 0, nil
}

// jobOps is a per-worker seam, including the six persisted launch boundaries
// exercised by Task 6. Nil functions use real operations, never global hooks.
type jobOps struct {
	newJob          func() (*jobObject, error)
	parentJob       func() (bool, error)
	assign          func(*jobObject, windows.Handle) error
	terminate       func(*jobObject) error
	members         func(*jobObject) ([]ProcessIdentity, error)
	startTime       func(int) (time.Time, error)
	terminateNative func(*os.Process) error
	openWaitHandle  func(int) (windows.Handle, error)
	afterIntent     func() error
	afterJob        func(*jobObject) error
	afterSpawn      func(int) error
	afterStart      func(int) error
	beforeResume    func(*jobObject) error
	afterResume     func() error
}

func (l *launcher) prepareJob() error {
	ops := l.w.jobOps
	if ops.afterIntent != nil {
		if err := ops.afterIntent(); err != nil {
			return err
		}
	}
	query := ops.parentJob
	if query == nil {
		query = parentJob
	}
	kill, err := query()
	if err != nil {
		return winError("query parent job", err)
	}
	if kill {
		return errors.New("native launch refused inside a kill-on-close parent job")
	}
	create := ops.newJob
	if create == nil {
		create = newJob
	}
	l.job, err = create()
	if err != nil {
		return winError("create owned job", err)
	}
	if l.job == nil || l.job.h == 0 {
		return errors.New("create owned job returned no handle")
	}
	if ops.afterJob != nil {
		return ops.afterJob(l.job)
	}
	return nil
}

func (l *launcher) finishJob(p *ownedProc) error {
	ops, pid := l.w.jobOps, p.cmd.Process.Pid
	p.job = l.job
	if ops.afterSpawn != nil {
		if err := ops.afterSpawn(pid); err != nil {
			return err
		}
	}
	readStart := ops.startTime
	if readStart == nil {
		readStart = ProcessStartTime
	}
	start, err := readStart(pid)
	if err != nil {
		return winError("read native creation time", err)
	}
	l.w.id.NativePID, l.w.id.NativePGID, l.w.id.NativeStartTime = &pid, p.pgid, &start
	if err := l.w.writeIdentity(); err != nil {
		return err
	}
	if ops.afterStart != nil {
		if err := ops.afterStart(pid); err != nil {
			return err
		}
	}
	// The suspended process remains alive, so this PID cannot be reused before
	// opening its handle. Retain that handle until assignment and resume finish.
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid)) //nolint:gosec // G115: Windows process IDs are uint32.
	if err != nil {
		return winError("OpenProcess(native)", err)
	}
	defer func() { _ = windows.CloseHandle(h) }()
	assign := ops.assign
	if assign == nil {
		assign = (*jobObject).assign
	}
	if err := assign(l.job, h); err != nil {
		return winError("assign native", err)
	}
	if ops.beforeResume != nil {
		if err := ops.beforeResume(l.job); err != nil {
			return err
		}
	}
	if err := resumeNative(pid); err != nil {
		return err
	}
	if ops.afterResume != nil {
		return ops.afterResume()
	}
	return nil
}

// resumeNative resumes the sole thread of a newly created suspended process.
// Enumerating exactly one thread avoids accidentally resuming an arbitrary
// thread in a process that already ran. The process remains owned on failure.
func resumeNative(pid int) error {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return winError("snapshot native threads", err)
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	var entry windows.ThreadEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	var tid uint32
	count := 0
	for err = windows.Thread32First(snap, &entry); err == nil; err = windows.Thread32Next(snap, &entry) {
		if entry.OwnerProcessID == uint32(pid) {
			tid = entry.ThreadID
			count++
		} //nolint:gosec // G115: Windows process IDs are uint32.
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return winError("enumerate native threads", err)
	}
	if count != 1 {
		return fmt.Errorf("suspended native has %d threads, want exactly one", count)
	}
	h, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, tid)
	if err != nil {
		return winError("OpenThread(native)", err)
	}
	defer func() { _ = windows.CloseHandle(h) }()
	previous, err := windows.ResumeThread(h)
	if err != nil {
		return winError("ResumeThread(native)", err)
	}
	if previous != 1 {
		return fmt.Errorf("native suspend count = %d, want one", previous)
	}
	return nil
}

// failedLaunch stops both the job and the possibly not-yet-assigned native.
// Never wait on a native whose termination failed: preserve unconfirmed state.
func (l *launcher) failedLaunch() {
	p := l.proc
	if p == nil {
		return
	}
	// Open the exact still-live native before requesting either termination.
	// Keep its handle across both requests so PID reuse cannot affect the wait.
	openWait := l.w.jobOps.openWaitHandle
	if openWait == nil {
		openWait = func(pid int) (windows.Handle, error) {
			return windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid)) //nolint:gosec // G115: Windows process IDs are uint32.
		}
	}
	h, err := openWait(p.cmd.Process.Pid)
	if err != nil {
		// Failure to acquire confirmation evidence must not prevent cleanup
		// through the process and job handles already owned by this worker.
		l.unconfirmed = true
	} else {
		defer func() { _ = windows.CloseHandle(h) }()
	}
	ops := l.w.jobOps
	terminate := ops.terminate
	if terminate == nil {
		terminate = (*jobObject).terminate
	}
	if l.job != nil {
		_ = terminate(l.job)
	}
	kill := ops.terminateNative
	if kill == nil {
		kill = (*os.Process).Kill
	}
	_ = kill(p.cmd.Process)
	if err != nil {
		return
	}
	// API return values alone are not stop proof. Even an error may race an
	// already-completed termination; the pinned native and job queries decide.
	state, waitErr := windows.WaitForSingleObject(h, uint32(waitDelay/time.Millisecond))
	if waitErr != nil || state != windows.WAIT_OBJECT_0 {
		l.unconfirmed = true
		return
	}
	_ = p.Wait()
	members := ops.members
	if members == nil {
		members = (*jobObject).members
	}
	if l.job != nil {
		remaining, err := members(l.job)
		if err != nil || len(remaining) != 0 {
			l.unconfirmed = true
		}
	}
}

// members is bounded: a job larger than this snapshot budget stays unresolved.
// A raced identity or membership query is an error, never an empty snapshot.
func (j *jobObject) members() ([]ProcessIdentity, error) {
	var list struct {
		Assigned, Count uint32
		PIDs            [4096]uintptr
	}
	if err := windows.QueryInformationJobObject(j.h, windows.JobObjectBasicProcessIdList, uintptr(unsafe.Pointer(&list)), uint32(unsafe.Sizeof(list)), nil); err != nil {
		return nil, winError("query job members", err)
	}
	if list.Count > uint32(len(list.PIDs)) || list.Assigned > list.Count {
		return nil, errors.New("job membership exceeds snapshot budget")
	}
	result := make([]ProcessIdentity, 0, list.Count)
	for _, pid := range list.PIDs[:list.Count] {
		if pid == 0 || uint64(pid) > uint64(^uint32(0)) {
			return nil, errors.New("job returned an invalid process ID")
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid)) //nolint:gosec // G115: range checked above.
		if err != nil {
			return nil, winError("open job member", err)
		}
		member, memberErr := processInJob(h, j.h)
		var created, exited, kernel, user windows.Filetime
		timeErr := windows.GetProcessTimes(h, &created, &exited, &kernel, &user)
		_ = windows.CloseHandle(h)
		if memberErr != nil {
			return nil, memberErr
		}
		if !member {
			return nil, errors.New("job member identity changed during snapshot")
		}
		if timeErr != nil {
			return nil, winError("read job member creation time", timeErr)
		}
		result = append(result, ProcessIdentity{PID: int(pid), StartTime: time.Unix(0, created.Nanoseconds()).UTC()})
	}
	return result, nil
}

func launchError(err error) string { return winError("native launch", err).Error() }
