//go:build unix

package workers

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestUnixLaunchRecordUnchanged(t *testing.T) { // I09 (v2 §2): absence remains unknown.
	t.Parallel()
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	pid, pgid := 101, 101
	id := Identity{SchemaVersion: 1, RunID: "run_synthetic", AttemptID: "att_synthetic", PID: 100, StartTime: stamp,
		LaunchToken: "synthetic-token", NativeLaunchIntent: &stamp, NativePID: &pid, NativePGID: &pgid, Nonce: "synthetic-nonce"}
	want, err := os.ReadFile("testdata/worker-json-unix.golden")
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if !bytes.Equal(got, want) {
		t.Fatalf("Unix identity changed:\ngot %s\nwant %s", got, want)
	}
	// Retain a violating identity: emitting even a zero start time changes the contract.
	zero := time.Time{}
	id.NativeStartTime = &zero
	changed, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(append(changed, '\n'), want) {
		t.Fatal("golden accepted an invented zero native creation time")
	}
}

func TestUnixLaunchEventUnchanged(t *testing.T) { // I09 (v2 §2): no invented native creation time.
	t.Parallel()
	a := newAttempt(t)
	w := &worker{}
	if err := a.runInProcess(t, a.launch(t, "happy"), w); err != nil {
		t.Fatal(err)
	}
	event := find(t, a.events(t), "attempt.launched")
	if event.Type != "attempt.launched" {
		t.Fatalf("type=%q", event.Type)
	}
	want, err := json.Marshal(map[string]any{"worker_pid": w.id.PID, "worker_start_time": w.id.StartTime,
		"native_pid": w.id.NativePID, "native_pgid": w.id.NativePGID})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(event.Payload, want) {
		t.Fatalf("launched payload=%s, want exact Unix payload %s", event.Payload, want)
	}
}
