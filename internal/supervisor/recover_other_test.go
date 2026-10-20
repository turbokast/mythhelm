//go:build !windows

package supervisor

import (
	"bufio"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/workers"
)

// The matching predicate is exercised on contributor hosts with explicit
// observations. Only hosted Windows tests establish Windows OS behaviour.
func TestRecordedNativeRecoveryEvidence(t *testing.T) {
	t.Parallel()
	at := time.Unix(100, 0).UTC()
	pid := 42
	denied := errors.New("access denied")
	type evidenceCase struct {
		name     string
		pid      *int
		recorded *time.Time
		observed time.Time
		err      error
		want     []int
		wantErr  error
	}
	cases := []evidenceCase{
		{name: "matching live native", pid: &pid, recorded: &at, observed: at, want: []int{pid}},
		{name: "reused PID", pid: &pid, recorded: &at, observed: at.Add(time.Second)},
		{name: "exited native", pid: &pid, recorded: &at, err: workers.ErrNoProcess},
		{name: "access denied remains unknown", pid: &pid, recorded: &at, err: denied, wantErr: denied},
		{name: "unreadable remains unknown", pid: &pid, recorded: &at, err: os.ErrPermission, wantErr: os.ErrPermission},
		{name: "missing native PID", recorded: &at},
		{name: "missing creation time", pid: &pid},
		{name: "zero creation time", pid: &pid, recorded: new(time.Time)},
		{name: "invalid PID", pid: new(int), recorded: &at},
	}
	maxPID := int(^uint(0) >> 1)
	if int64(maxPID) > math.MaxUint32 {
		cases = append(cases, evidenceCase{name: "out of Windows PID range", pid: &maxPID, recorded: &at})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			got, err := recordedNativePIDs(workers.Identity{NativePID: tc.pid, NativeStartTime: tc.recorded}, func(gotPID int) (time.Time, error) {
				calls++
				if gotPID != pid {
					t.Fatalf("probed PID %d", gotPID)
				}
				return tc.observed, tc.err
			})
			if !reflect.DeepEqual(got, tc.want) || !errors.Is(err, tc.wantErr) {
				t.Fatalf("identities=%v err=%v, want %v/%v", got, err, tc.want, tc.wantErr)
			}
			valid := tc.pid != nil && *tc.pid > 0 && int64(*tc.pid) <= math.MaxUint32 && tc.recorded != nil && !tc.recorded.IsZero()
			if valid != (calls == 1) {
				t.Fatalf("valid=%v calls=%d", valid, calls)
			}
		})
	}
}

// TestRecoveryMarkedOrphanHelper supplies a real marker-bearing process for
// the Unix compatibility test, without invoking a shell or another native.
func TestRecoveryMarkedOrphanHelper(_ *testing.T) {
	if os.Getenv("MYTHHELM_TEST_RECOVERY_ORPHAN") != "1" {
		return
	}
	fmt.Println("ready")
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
}

func TestRecoveryOrphanLookupUnchanged(t *testing.T) {
	t.Parallel()
	attemptID := "att_recovery_compatibility"
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestRecoveryMarkedOrphanHelper$") //nolint:gosec // G702: own test executable and fixed helper selector.
	cmd.Env = append(os.Environ(), "MYTHHELM_TEST_RECOVERY_ORPHAN=1", workers.AttemptMarker(attemptID))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = cmd.Wait() })
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "ready" {
		t.Fatalf("marker process readiness: %q %v", line, err)
	}
	// A live recorded native must not replace the Unix marker scan. An unrelated
	// identity error must retain the old marker reporting behaviour as well.
	self, err := workers.ProcessStartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	got, err := recoveryUnresolvedPIDs(runtime.GOOS, attemptID, workers.Identity{NativePID: new(os.Getpid()), NativeStartTime: &self}, os.ErrPermission)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "linux" {
		if !reflect.DeepEqual(got, []int{cmd.Process.Pid}) {
			t.Fatalf("Unix marked PIDs=%v want child %d", got, cmd.Process.Pid)
		}
	} else if len(got) != 0 {
		t.Fatalf("unsupported marker enumeration invented PIDs: %v", got)
	}
	want := "ownership unresolved; inspect " + workers.AttemptMarker(attemptID) + "; marked PIDs " + fmt.Sprint(got) + "; no process was signalled"
	if notice := recoveryOwnershipNotice(runtime.GOOS, attemptID, got, os.ErrPermission); notice != want {
		t.Fatalf("Unix notice=%q want %q", notice, want)
	}
}

func TestWindowsRecoveryLookupAndNotice(t *testing.T) {
	t.Parallel()
	const attemptID = "att_windows_notice"
	start, err := workers.ProcessStartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	id := workers.Identity{NativePID: new(os.Getpid()), NativeStartTime: &start}
	for _, tc := range []struct {
		name        string
		identity    workers.Identity
		identityErr error
		want        []int
		unobserved  bool
	}{
		{name: "matching live", identity: id, want: []int{os.Getpid()}},
		{name: "no recorded native"},
		{name: "reused PID", identity: workers.Identity{NativePID: id.NativePID, NativeStartTime: new(start.Add(time.Second))}},
		{name: "unreadable identity", identity: id, identityErr: os.ErrPermission, unobserved: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pids, err := recoveryUnresolvedPIDs("windows", attemptID, tc.identity, tc.identityErr)
			if !reflect.DeepEqual(pids, tc.want) || !errors.Is(err, tc.identityErr) {
				t.Fatalf("pids=%v err=%v", pids, err)
			}
			notice := recoveryOwnershipNotice("windows", attemptID, pids, err)
			if !strings.Contains(notice, workers.AttemptMarker(attemptID)) || !strings.Contains(notice, "descendant ownership unobserved; no process was signalled") {
				t.Fatalf("notice=%q", notice)
			}
			if tc.unobserved {
				if !strings.Contains(notice, "native identity unobserved (probe failed)") || strings.Contains(notice, "matching live native PIDs") {
					t.Fatalf("unknown probe described as observed empty: %q", notice)
				}
			} else if !strings.Contains(notice, "matching live native PIDs "+fmt.Sprint(tc.want)) {
				t.Fatalf("observed native notice=%q", notice)
			}
		})
	}
	for _, probeErr := range []error{os.ErrPermission, errors.New("creation time unreadable")} {
		notice := recoveryOwnershipNotice("windows", attemptID, nil, probeErr)
		if !strings.Contains(notice, "native identity unobserved (probe failed)") || strings.Contains(notice, "matching live native PIDs") {
			t.Fatalf("failed observation notice=%q", notice)
		}
	}
}
