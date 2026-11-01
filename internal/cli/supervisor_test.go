package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
)

// This fixture changes process-wide endpoint discovery; it is not parallel.
func TestServiceEntrypointShell(t *testing.T) {
	base, err := os.MkdirTemp("", "mh-shell-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	root := filepath.Join(base, "state")
	rt := filepath.Join(base, "rt")
	if err := os.Mkdir(rt, 0700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(base, "mythhelm")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "../../cmd/mythhelm")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	t.Setenv("MYTHHELM_HOME", root)
	t.Setenv("XDG_RUNTIME_DIR", rt)
	t.Setenv("HOME", base)
	t.Setenv("TEMP", rt)
	t.Setenv("TMP", rt)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, SupervisorCommand)
	cmd.Env = []string{"MYTHHELM_HOME=" + root, "XDG_RUNTIME_DIR=" + rt, "HOME=" + base, "TEMP=" + rt, "TMP=" + rt, "USERNAME=" + os.Getenv("USERNAME"), "USERPROFILE=" + base, "SystemRoot=" + os.Getenv("SystemRoot")}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() { cancel(); <-done }()
	transport, err := control.DefaultTransport()
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := control.EndpointPath()
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	var conn control.Conn
	for {
		conn, err = transport.Dial(endpoint)
		if err == nil {
			break
		}
		select {
		case <-deadline.C:
			t.Fatalf("no shell: %v", err)
		case <-tick.C:
		}
	}
	defer func() { _ = conn.Close() }()
	if !conn.Peer().SameUser {
		t.Fatal("shell was not authenticated")
	}
	status, err := control.Call(t.Context(), conn, control.Intent{OperationID: ids.New("op"), Method: "status"})
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		PID        int    `json:"pid"`
		Generation int64  `json:"generation"`
		Root       string `json:"root"`
		Endpoint   string `json:"endpoint"`
	}
	if err := json.Unmarshal(status.Body, &st); err != nil {
		t.Fatal(err)
	}
	heldGeneration, err := control.CurrentGeneration(root)
	if err != nil {
		t.Fatal(err)
	}
	physicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if st.PID != cmd.Process.Pid || st.Generation != heldGeneration || st.Root != physicalRoot || st.Endpoint != endpoint {
		t.Fatalf("status does not match held singleton: %+v, generation %d", st, heldGeneration)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(root, journal.DBName))+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var operations int
	if err := db.QueryRow(`SELECT COUNT(*) FROM operations`).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if operations != 0 {
		t.Fatalf("read-only shell status wrote %d operations", operations)
	}
	var phase string
	if err := db.QueryRow(`SELECT phase FROM migration_state WHERE id=1`).Scan(&phase); err != nil {
		t.Fatal(err)
	}
	if phase != "adopted" {
		t.Fatalf("fresh root phase=%s", phase)
	}
	before := map[string]int{}
	for _, table := range []string{"operations", "run_assignments", "attempts", "journal"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		before[table] = count
	}
	for _, method := range []string{"run.start", "attachment.open", "assign", "reserve", "stop", "recover"} {
		params := json.RawMessage(nil)
		if method == "run.start" {
			params = json.RawMessage(`{"schema_version":2,"preparation_id":"prep_synthetic","binding_sha256":"` + strings.Repeat("a", 64) + `"}`)
		}
		if method == "attachment.open" {
			params = json.RawMessage(`{"run_id":"run_synthetic"}`)
		}
		_, err := control.Call(t.Context(), conn, control.Intent{OperationID: ids.New("op"), Method: method, Params: params})
		if !errors.Is(err, &control.Error{Code: control.CodeCapabilityUnsupported}) {
			t.Fatalf("%s error=%v", method, err)
		}
	}
	for table, want := range before {
		var got int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("unsupported changed %s from%d to%d", table, want, got)
		}
	}
}
