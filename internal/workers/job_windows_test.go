//go:build windows

package workers

import (
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestWindowsRunnerParentJobSpike measures Task 3's prerequisite; it does not
// qualify process ownership or assert that a runner permits native launch.
// Keep this independent of the production parentJob implementation: production
// changes wait for measurements from both native Windows runner architectures.
func TestWindowsRunnerParentJobSpike(t *testing.T) {
	t.Parallel()
	isProcessInJob := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")
	if err := isProcessInJob.Find(); err != nil {
		t.Fatalf("resolve IsProcessInJob: %v", err)
	}
	var inJob int32
	ok, _, callErr := isProcessInJob.Call(
		uintptr(windows.CurrentProcess()), 0, uintptr(unsafe.Pointer(&inJob)),
	)
	if ok == 0 {
		t.Fatalf("IsProcessInJob(GetCurrentProcess(), 0): %v", callErr)
	}
	if inJob == 0 {
		t.Logf("parent_job_spike goos=%s goarch=%s in_job=false limit_flags=not_queried kill_on_close=false", runtime.GOOS, runtime.GOARCH)
		return
	}
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	err := windows.QueryInformationJobObject(0, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), nil)
	if err != nil {
		t.Fatalf("QueryInformationJobObject(0, JobObjectExtendedLimitInformation): %v", err)
	}
	flags := limits.BasicLimitInformation.LimitFlags
	killOnClose := flags&windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE != 0
	t.Logf("parent_job_spike goos=%s goarch=%s in_job=true limit_flags=0x%08x kill_on_close=%t", runtime.GOOS, runtime.GOARCH, flags, killOnClose)
	if killOnClose {
		t.Log("Task 3 production work must stop for a reviewed parent-job policy decision (ADR 0017 D11)")
	}
}
