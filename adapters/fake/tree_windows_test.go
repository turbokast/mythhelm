//go:build windows

package fake

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type treeReportTest struct {
	Role      string    `json:"role"`
	PID       uint32    `json:"pid"`
	StartTime time.Time `json:"start_time"`
	Error     *uint32   `json:"error"`
}

func readTreeReports(t *testing.T, path string) []treeReportTest {
	t.Helper()
	src, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	reports, err := decodeTreeReports(src)
	if err != nil {
		t.Fatal(err)
	}
	return reports
}

func decodeTreeReports(src []byte) ([]treeReportTest, error) {
	var reports []treeReportTest
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.DisallowUnknownFields()
	for {
		var report treeReportTest
		err := dec.Decode(&report)
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return reports, nil
		}
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
}

func TestSpawnTreeReportsDescendants(t *testing.T) {
	// I13 (v2 §2): a credential-free executable supplies native descendants.
	if _, err := loadScenario("tree"); err != nil {
		t.Fatalf("load tree: %v", err)
	}
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "tree.jsonl")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, exe, AgentCommand, "--scenario", "tree", "--workdir", dir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = 10 * time.Second
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Pin the native before its waiter can release os.Process's handle. This
	// cleanup does not read report JSON, so even unreported descendants are found.
	native, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal(err)
	}
	waitDone := make(chan struct{})
	var waitErr error
	go func() { waitErr = cmd.Wait(); close(waitDone) }()
	defer func() {
		if err := stopTreeHandle(native); err != nil {
			t.Errorf("fixture cleanup: %v", err)
		}
		<-waitDone
		_ = windows.CloseHandle(native)
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var reports []treeReportTest
	for len(reports) < 3 {
		select {
		case <-ctx.Done():
			t.Fatalf("tree report incomplete: %+v", reports)
		case <-ticker.C:
			reports = readTreeReports(t, reportPath)
		}
	}
	var descendants []windows.Handle
	defer func() {
		for _, h := range descendants {
			_ = windows.CloseHandle(h)
		}
	}()
	byRole := map[string]treeReportTest{}
	for _, report := range reports {
		if _, exists := byRole[report.Role]; exists {
			t.Fatalf("duplicate role %q", report.Role)
		}
		byRole[report.Role] = report
		switch report.Role {
		case "child", "grandchild":
			if report.Error != nil {
				t.Fatalf("descendant has error: %+v", report)
			}
		case "breakaway":
			if report.Error == nil {
				t.Fatalf("breakaway has no error code: %+v", report)
			}
			if *report.Error != 0 {
				if report.PID != 0 || !report.StartTime.IsZero() {
					t.Fatalf("failed breakaway has identity: %+v", report)
				}
				continue
			}
		default:
			t.Fatalf("unexpected report role %q", report.Role)
		}
		if report.PID == 0 || report.StartTime.IsZero() || report.StartTime.Location() != time.UTC {
			t.Fatalf("missing UTC identity: %+v", report)
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, report.PID)
		if err != nil {
			t.Fatal(err)
		}
		var creation, exit, kernel, user windows.Filetime
		err = windows.GetProcessTimes(h, &creation, &exit, &kernel, &user)
		var code uint32
		codeErr := windows.GetExitCodeProcess(h, &code)
		descendants = append(descendants, h)
		if err != nil || codeErr != nil || code != 259 || !time.Unix(0, creation.Nanoseconds()).Equal(report.StartTime) {
			t.Fatalf("%s identity/liveness: time error %v, exit error %v, code %d, creation %v, report %+v", report.Role, err, codeErr, code, creation, report)
		}
	}
	for _, role := range []string{"child", "grandchild", "breakaway"} {
		if _, ok := byRole[role]; !ok {
			t.Fatalf("missing %s: %+v", role, reports)
		}
	}
	// A pair of sibling processes is not a child/grandchild fixture.
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()
	parents := map[uint32]uint32{}
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		parents[entry.ProcessID] = entry.ParentProcessID
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		t.Fatal(err)
	}
	if parents[byRole["child"].PID] != uint32(cmd.Process.Pid) || parents[byRole["grandchild"].PID] != byRole["child"].PID {
		t.Fatalf("wrong ancestry: child parent %d, grandchild parent %d", parents[byRole["child"].PID], parents[byRole["grandchild"].PID])
	}
	// Descendants must not keep the native's stdout/stderr copy pipes open.
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("native wait did not finish before WaitDelay; descendants may hold its pipes")
	}
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) {
		t.Fatalf("native wait = %v, want killed exit", waitErr)
	}
	// The fixture itself must not supply kill-on-native-exit ownership. Later
	// worker tests need these processes to survive unless the worker stops them.
	for _, h := range descendants {
		state, err := windows.WaitForSingleObject(h, 0)
		if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
			t.Fatalf("descendant did not outlive native: %d, %v", state, err)
		}
	}
	if err := os.WriteFile(reportPath, []byte(`{"role":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := stopTreeHandle(native); err != nil {
		t.Fatal(err)
	}
	for _, h := range descendants {
		state, err := windows.WaitForSingleObject(h, 0)
		if err != nil || state != windows.WAIT_OBJECT_0 {
			t.Errorf("descendant survived cleanup without report: %d, %v", state, err)
		}
	}
}

func TestTreeSetupFailureCleansDescendants(t *testing.T) {
	for _, failedRole := range []string{"child", "breakaway"} {
		t.Run(failedRole, func(t *testing.T) {
			dir := t.TempDir()
			rejected := errors.New("injected report failure")
			var observed []windows.Handle
			defer func() {
				for _, h := range observed {
					_ = windows.TerminateProcess(h, 1)
					_ = windows.CloseHandle(h)
				}
			}()
			fixture := treeFixture{workdir: dir, reportPath: "tree.jsonl"}
			fixture.writeReport = func(root *os.Root, path string, report treeReport) error {
				if report.Role != failedRole {
					return appendTreeReport(root, path, report)
				}
				// Force failure only after the grandchild exists. Corrupting the report
				// proves rollback does not depend on collecting complete report records.
				deadline := time.NewTimer(10 * time.Second)
				defer deadline.Stop()
				ticker := time.NewTicker(10 * time.Millisecond)
				defer ticker.Stop()
				var reports []treeReportTest
				for {
					data, err := root.ReadFile(path)
					if err != nil {
						return err
					}
					reports, err = decodeTreeReports(data)
					if err != nil {
						return err
					}
					found := false
					for _, r := range reports {
						if r.Role == "grandchild" {
							found = true
						}
					}
					if found {
						break
					}
					select {
					case <-deadline.C:
						return errors.New("grandchild was not created before injected failure")
					case <-ticker.C:
					}
				}
				if report.PID != 0 {
					reports = append(reports, treeReportTest{PID: uint32(report.PID), StartTime: *report.StartTime})
				}
				for _, r := range reports {
					if r.PID == 0 {
						continue
					}
					h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, r.PID)
					if err != nil {
						return err
					}
					observed = append(observed, h)
					var creation, exit, kernel, user windows.Filetime
					if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
						return err
					}
					if !time.Unix(0, creation.Nanoseconds()).Equal(r.StartTime) {
						return errors.New("wrong process identity")
					}
					state, err := windows.WaitForSingleObject(h, 0)
					if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
						return fmt.Errorf("process was not alive before failure: %d, %v", state, err)
					}
				}
				if err := root.WriteFile(path, []byte(`{"role":`), 0o600); err != nil {
					return err
				}
				return rejected
			}
			if err := fixture.run(); !errors.Is(err, rejected) {
				t.Fatalf("setup error = %v, want injected report failure", err)
			}
			if len(observed) < 2 {
				t.Fatalf("observed %d processes, want child and grandchild", len(observed))
			}
			for _, h := range observed {
				state, err := windows.WaitForSingleObject(h, 0)
				if err != nil || state != windows.WAIT_OBJECT_0 {
					t.Errorf("process survived rollback: %d, %v", state, err)
				}
			}
		})
	}
}

func TestTreeGrandchildReportFailureCleansProcess(t *testing.T) {
	root, err := prepareTreeReport(t.TempDir(), "tree.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	var observed windows.Handle
	defer func() {
		if observed != 0 {
			_ = windows.TerminateProcess(observed, 1)
			_ = windows.CloseHandle(observed)
		}
	}()
	rejected := errors.New("grandchild report failed")
	reject := func(root *os.Root, path string, report treeReport) error {
		var err error
		observed, err = windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(report.PID))
		if err != nil {
			return err
		}
		state, err := windows.WaitForSingleObject(observed, 0)
		if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
			return fmt.Errorf("grandchild not live before write failure: %d, %v", state, err)
		}
		if err := root.WriteFile(path, []byte(`{"role":`), 0o600); err != nil {
			return err
		}
		return rejected
	}
	// This is the same start-and-report path the re-executed child calls before
	// AgentMain returns its error. The process must be gone before that return.
	p, err := startTreeProcess(root, "tree.jsonl", "grandchild", 0, reject, "--linger", "10m")
	if p != nil {
		_ = p.Release()
		t.Fatal("failed start returned a live process")
	}
	if !errors.Is(err, rejected) || observed == 0 {
		t.Fatalf("grandchild report failure = %v, handle %v", err, observed)
	}
	state, err := windows.WaitForSingleObject(observed, 0)
	if err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatalf("grandchild survived report failure: %d, %v", state, err)
	}
}
