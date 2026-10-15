//go:build windows

package fake

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	var reports []treeReportTest
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.DisallowUnknownFields()
	for {
		var report treeReportTest
		err := dec.Decode(&report)
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return reports
		}
		if err != nil {
			t.Fatal(err)
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
	waitDone := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(waitDone)
	}()
	defer func() {
		_ = cmd.Process.Kill()
		<-waitDone
	}()
	defer func() {
		for _, report := range readTreeReports(t, reportPath) {
			if report.PID == 0 {
				continue
			}
			h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, report.PID)
			if err != nil {
				continue
			}
			var creation, exit, kernel, user windows.Filetime
			if windows.GetProcessTimes(h, &creation, &exit, &kernel, &user) == nil && time.Unix(0, creation.Nanoseconds()).Equal(report.StartTime) {
				_ = windows.TerminateProcess(h, 1)
			}
			_ = windows.CloseHandle(h)
		}
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
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, report.PID)
		if err != nil {
			t.Fatal(err)
		}
		var creation, exit, kernel, user windows.Filetime
		err = windows.GetProcessTimes(h, &creation, &exit, &kernel, &user)
		var code uint32
		codeErr := windows.GetExitCodeProcess(h, &code)
		_ = windows.CloseHandle(h)
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
}
