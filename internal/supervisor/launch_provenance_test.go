package supervisor

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/v2contract"
	"github.com/turbokast/mythhelm/internal/workers"
)

// captureLaunchEnv names the file a re-executed worker writes its raw
// Launch stdin to instead of running: the test binary is the exe Spawn
// starts, so this captures the exact serialised handoff.
const captureLaunchEnv = "MYTHHELM_TEST_CAPTURE_LAUNCH"

func init() {
	path := os.Getenv(captureLaunchEnv)
	if path == "" || len(os.Args) < 2 || os.Args[1] != workers.Command {
		return
	}
	body, err := io.ReadAll(io.LimitReader(bufio.NewReader(os.Stdin), 1<<20))
	if err != nil {
		os.Exit(3)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil { //nolint:gosec // G703: the test's own capture path
		os.Exit(3)
	}
	os.Exit(0)
}

const (
	tokenSentinel = "launch-token-SENTINEL-7f3a" //nolint:gosec // G101: fixed synthetic sentinel, never a credential.
	envSentinel   = "SECRET_ENV_SENTINEL=9c1e"
)

func tokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// tokenAttempt records a launch intent whose token digest is token's.
func tokenAttempt(t *testing.T, j *journal.Journal, p *Producer, runID, token string) string {
	t.Helper()
	attemptID := ids.New("att")
	if err := RecordLaunchIntent(t.Context(), j, journal.AttemptRow{
		AttemptID:         attemptID,
		RunID:             runID,
		TaskID:            "task_1",
		AttemptNumber:     attemptNumbers.Add(1),
		LaunchTokenSHA256: tokenDigest(token),
		WorkspacePath:     "/tmp/workspace",
	}, p); err != nil {
		t.Fatalf("RecordLaunchIntent: %v", err)
	}
	return attemptID
}

// handoffMatches compares the durable record with a serialised Launch.
func handoffMatches(rec control.LaunchProvenanceV1, l workers.Launch) error {
	switch {
	case *rec.Generation != l.Generation:
		return fmt.Errorf("generation %d != handoff %d", *rec.Generation, l.Generation)
	case !rec.ExecutionDeadline.Equal(l.ExecutionDeadline):
		return fmt.Errorf("deadline %v != handoff %v", rec.ExecutionDeadline, l.ExecutionDeadline)
	case rec.LaunchTokenSHA256 != tokenDigest(l.LaunchToken):
		return errors.New("token digest differs from the handoff")
	case rec.NonceSHA256 != workers.NonceDigest(l.Nonce):
		return errors.New("nonce digest differs from the handoff")
	case rec.LadderVersion != l.LadderVersion:
		return errors.New("ladder version differs from the handoff")
	}
	return nil
}

func provenanceCode(t *testing.T, err error) (v2contract.Code, string) {
	t.Helper()
	ce, ok := errors.AsType[*v2contract.ControlError](err)
	if !ok {
		t.Fatalf("error %v is not a ControlError", err)
	}
	return ce.Code, ce.Detail["mythhelm.launch-provenance/reason"]
}

func TestLaunchProvenanceMatchesSerializedHandoff(t *testing.T) {
	// Sequential: process environment (XDG_RUNTIME_DIR, capture path).
	ctx := t.Context()
	stateDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	release, err := control.AcquireInstance(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	pinnedGen, err := control.CurrentGeneration(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	j := openPinJournal(t, stateDir)
	prod := NewProducer(ids.New("sup"), 1)
	runID := newRun(t, j, prod)
	attemptID := tokenAttempt(t, j, prod, runID, tokenSentinel)
	deadline := time.Date(2026, 11, 2, 12, 30, 0, 123456789, time.UTC)

	capture := filepath.Join(t.TempDir(), "launch.json")
	t.Setenv(captureLaunchEnv, capture)

	// The production pin runs, then the boot generation changes before the
	// production Spawn: a reread in between would hand the new value over.
	var newGen int64
	reason, err := runSpawnPinned(t, stateDir, runID, attemptID, workers.Launch{
		LaunchToken:       tokenSentinel,
		Env:               []string{envSentinel},
		ExecutionDeadline: deadline,
	}, func(ctx context.Context, l workers.Launch) (workers.Launch, error) {
		pinned, err := pinnedLaunch(ctx, j, prod, stateDir, runID, "task_1", attemptID, l)
		if err != nil {
			return pinned, err
		}
		release()
		release2, err := control.AcquireInstance(stateDir)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(release2)
		if newGen, err = control.CurrentGeneration(stateDir); err != nil || newGen == pinnedGen {
			t.Fatalf("generation after reacquire = %d, %v; want it to differ from %d", newGen, err, pinnedGen)
		}
		return pinned, nil
	})
	if err != nil {
		t.Fatalf("spawnPinned = %s, %v", reason, err)
	}
	handed := readCapture(t, capture)

	rec, err := control.ReadLaunchProvenance(ctx, j, runID, attemptID)
	if err != nil {
		t.Fatalf("ReadLaunchProvenance: %v", err)
	}
	if handed.Generation != pinnedGen { // I12/I19 (v2 §6): no reread between pin and Spawn
		t.Fatalf("handoff generation = %d, want the pinned %d (the reread value is %d)", handed.Generation, pinnedGen, newGen)
	}
	if !handed.ExecutionDeadline.Equal(deadline) || handed.LaunchToken != tokenSentinel {
		t.Fatalf("handoff deadline = %v, want the original %v with the attempt's token", handed.ExecutionDeadline, deadline)
	}
	if err := handoffMatches(rec, handed); err != nil {
		t.Fatalf("durable record differs from the serialised handoff: %v", err)
	}
	if *rec.Generation != pinnedGen || !rec.ExecutionDeadline.Equal(deadline) {
		t.Fatalf("record generation/deadline = %d/%v, want %d/%v", *rec.Generation, rec.ExecutionDeadline, pinnedGen, deadline)
	}

	events, err := j.Events(ctx, runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		for _, secret := range []string{tokenSentinel, envSentinel, handed.Nonce} {
			if strings.Contains(string(ev.Payload), secret) {
				t.Errorf("event %s carries a raw launch secret", ev.Type) // I20 (v2 §8)
			}
		}
	}
}

// runSpawnPinned drives the production pin-to-Spawn path with the test
// binary as the capture worker and waits for it when one started.
func runSpawnPinned(t *testing.T, stateDir, runID, attemptID string, l workers.Launch, pin func(context.Context, workers.Launch) (workers.Launch, error)) (string, error) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	proc, reason, err := spawnPinned(t.Context(), exe, stateDir, runID, attemptID, l, pin)
	if proc != nil {
		if st, werr := proc.Wait(); werr != nil || !st.Success() {
			t.Fatalf("capture worker = %v, %v", st, werr)
		}
	}
	return reason, err
}

func readCapture(t *testing.T, path string) workers.Launch {
	t.Helper()
	raw, err := os.ReadFile(path) //nolint:gosec // G304: the test's own capture path
	if err != nil {
		t.Fatalf("no serialised handoff reached the capture worker: %v", err)
	}
	var handed workers.Launch
	if err := json.Unmarshal(raw, &handed); err != nil {
		t.Fatal(err)
	}
	return handed
}

func TestLaunchProvenanceWriteBeforeSpawn(t *testing.T) {
	// Sequential: process environment (capture path).
	ctx := t.Context()
	stateDir := t.TempDir()
	j := openPinJournal(t, stateDir)
	prod := NewProducer(ids.New("sup"), 1)
	runID := newRun(t, j, prod)
	// A started, paused run with spent counters: the pin must leave this
	// canonical execution clock exactly as journaled.
	seeded := journal.EnvelopeRow{RunID: runID, ExecutionSeconds: 600, Repairs: 3, Replans: 2, TransportRetries: 5,
		FirstStartAt: "2026-11-02T12:00:00Z", RepairsUsed: 1, ReplansUsed: 1, TransportRetriesSeen: 2,
		PauseSpans: `[{"start":"2026-11-02T12:05:00Z","end":"2026-11-02T12:07:00Z"}]`, UpdatedAt: "2026-11-02T12:07:00Z"}
	if err := j.Transact(ctx, func(tx *sql.Tx) error { return journal.UpsertRunEnvelope(ctx, tx, seeded) }); err != nil {
		t.Fatal(err)
	}
	envelope := func(t *testing.T) journal.EnvelopeRow {
		t.Helper()
		env, err := j.RunEnvelope(ctx, runID)
		if err != nil {
			t.Fatalf("RunEnvelope: %v", err)
		}
		return env
	}
	if got := envelope(t); got != seeded {
		t.Fatalf("seeded envelope reads back as %+v, want %+v", got, seeded)
	}
	deadline := time.Date(2026, 11, 2, 13, 0, 0, 0, time.UTC)
	base := workers.Launch{LaunchToken: tokenSentinel, ExecutionDeadline: deadline}
	pinFor := func(attemptID string) func(context.Context, workers.Launch) (workers.Launch, error) {
		return func(ctx context.Context, l workers.Launch) (workers.Launch, error) {
			return pinnedLaunch(ctx, j, prod, stateDir, runID, "task_1", attemptID, l)
		}
	}
	// unchangedClock requires the durable envelope and attempt row to equal
	// their values from before the production pin.
	unchangedClock := func(t *testing.T, attemptID string, attempt journal.AttemptRow) {
		t.Helper()
		if got := envelope(t); got != seeded {
			t.Fatalf("pin changed the canonical execution clock: %+v -> %+v", seeded, got)
		}
		after, err := j.Attempt(ctx, attemptID)
		if err != nil {
			t.Fatal(err)
		}
		if after != attempt {
			t.Fatalf("attempt changed across the pin: %+v -> %+v", attempt, after)
		}
	}
	// refusedSpawn drives a failing production pin and requires that no
	// worker received a handoff and the clock and attempt row are untouched.
	refusedSpawn := func(t *testing.T, attemptID string) {
		t.Helper()
		capture := filepath.Join(t.TempDir(), "launch.json")
		t.Setenv(captureLaunchEnv, capture)
		before, err := j.Attempt(ctx, attemptID)
		if err != nil {
			t.Fatal(err)
		}
		reason, err := runSpawnPinned(t, stateDir, runID, attemptID, base, pinFor(attemptID))
		if err == nil || reason != "admission_pin_failed" {
			t.Fatalf("spawnPinned = %q, %v; want admission_pin_failed", reason, err)
		}
		if _, serr := os.Stat(capture); !errors.Is(serr, os.ErrNotExist) {
			t.Fatalf("a failed pin still spawned a worker (capture stat: %v)", serr)
		}
		unchangedClock(t, attemptID, before)
	}

	t.Run("token mismatch refuses the pin and spawns nothing", func(t *testing.T) {
		attemptID := tokenAttempt(t, j, prod, runID, "another-token")
		refusedSpawn(t, attemptID)
		if _, _, err := control.PinnedAdmission(ctx, j, runID, attemptID); err == nil {
			t.Fatal("a refused pin journaled attempt.admission_pinned")
		}
	})

	t.Run("an existing pin conflicts and spawns nothing", func(t *testing.T) {
		attemptID := tokenAttempt(t, j, prod, runID, tokenSentinel)
		if err := RecordAdmissionPinned(ctx, j, runID, "task_1", attemptID, stopLadderVersion(runtime.GOOS), workers.NonceDigest("legacy"), prod); err != nil {
			t.Fatal(err)
		}
		refusedSpawn(t, attemptID)
		code, reason := provenanceCode(t, func() error {
			_, err := control.ReadLaunchProvenance(ctx, j, runID, attemptID)
			return err
		}())
		if code != v2contract.CodeOwnershipUnresolved || reason != "missing" {
			t.Fatalf("legacy-only pin = %s/%s, want ownership_unresolved/missing", code, reason)
		}
	})

	t.Run("explicit zero generation spawns, a later conflicting pin refuses", func(t *testing.T) {
		attemptID := tokenAttempt(t, j, prod, runID, tokenSentinel)
		capture := filepath.Join(t.TempDir(), "launch.json")
		t.Setenv(captureLaunchEnv, capture)
		before, err := j.Attempt(ctx, attemptID)
		if err != nil {
			t.Fatal(err)
		}
		if reason, err := runSpawnPinned(t, stateDir, runID, attemptID, base, pinFor(attemptID)); err != nil {
			t.Fatalf("valid control spawnPinned = %s, %v", reason, err)
		}
		handed := readCapture(t, capture)
		unchangedClock(t, attemptID, before)
		rec, err := control.ReadLaunchProvenance(ctx, j, runID, attemptID)
		if err != nil {
			t.Fatalf("valid control: %v", err)
		}
		// No instance holder: the legacy fallback is an explicit, present 0.
		if rec.Generation == nil || *rec.Generation != 0 || handed.Generation != 0 || !handed.ExecutionDeadline.Equal(deadline) {
			t.Fatalf("zero-generation handoff = %d/%v, record %v", handed.Generation, handed.ExecutionDeadline, rec.Generation)
		}
		if err := handoffMatches(rec, handed); err != nil {
			t.Fatalf("durable record differs from the serialised handoff: %v", err)
		}
		if err := RecordAdmissionPinned(ctx, j, runID, "task_1", attemptID, stopLadderVersion(runtime.GOOS), workers.NonceDigest("other"), prod); err != nil {
			t.Fatal(err)
		}
		code, reason := provenanceCode(t, func() error {
			_, err := control.ReadLaunchProvenance(ctx, j, runID, attemptID)
			return err
		}())
		if code != v2contract.CodeInvalidContract || reason != "conflicting" {
			t.Fatalf("conflicting later pin = %s/%s, want invalid_contract/conflicting", code, reason)
		}
	})
}

// pinWithProvenance journals an admission pin whose nested record is raw.
func pinWithProvenance(t *testing.T, j *journal.Journal, p *Producer, runID, attemptID, nonceSHA, raw string) {
	t.Helper()
	payload := fmt.Sprintf(`{"ladder_version":%q,"nonce_sha256":%q,"launch_provenance":%s}`, stopLadderVersion(runtime.GOOS), nonceSHA, raw)
	ev, err := newEvent(runID, "task_1", attemptID, control.EventAdmissionPinned, json.RawMessage(payload), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := p.append(t.Context(), j, ev, nil); err != nil {
		t.Fatal(err)
	}
}

func TestLaunchProvenanceKnownZeroAndUnknown(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	j := openPinJournal(t, t.TempDir())
	prod := NewProducer(ids.New("sup"), 1)
	runID := newRun(t, j, prod)
	nonceSHA := workers.NonceDigest("nonce")
	ladder := stopLadderVersion(runtime.GOOS)
	record := func(attemptID, gen string, mutate func(string) string) string {
		r := fmt.Sprintf(`{"schema_version":1,"run_id":%q,"attempt_id":%q,"launch_token_sha256":%q,"nonce_sha256":%q,"ladder_version":%q,"generation":%s,"execution_deadline":"2026-11-02T12:00:00Z"}`,
			runID, attemptID, tokenDigest(tokenSentinel), nonceSHA, ladder, gen)
		if mutate != nil {
			r = mutate(r)
		}
		return r
	}

	t.Run("missing historical provenance is unknown, not zero", func(t *testing.T) {
		attemptID := tokenAttempt(t, j, prod, runID, tokenSentinel)
		if err := RecordAdmissionPinned(ctx, j, runID, "task_1", attemptID, ladder, nonceSHA, prod); err != nil {
			t.Fatal(err)
		}
		rec, err := control.ReadLaunchProvenance(ctx, j, runID, attemptID)
		if rec.Generation != nil {
			t.Fatalf("missing provenance returned generation %d", *rec.Generation)
		}
		code, reason := provenanceCode(t, err)
		if code != v2contract.CodeOwnershipUnresolved || reason != "missing" {
			t.Fatalf("missing provenance = %s/%s, want ownership_unresolved/missing", code, reason)
		}
		if v, d, err := control.PinnedAdmission(ctx, j, runID, attemptID); err != nil || v != ladder || d != nonceSHA {
			t.Fatalf("historical PinnedAdmission = %q %q %v", v, d, err)
		}
	})

	for _, tc := range []struct {
		name string
		gen  string
		want int64
	}{
		{"explicit zero is a known generation", "0", 0},
		{"nonzero generation is returned", "3", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attemptID := tokenAttempt(t, j, prod, runID, tokenSentinel)
			pinWithProvenance(t, j, prod, runID, attemptID, nonceSHA, record(attemptID, tc.gen, nil))
			rec, err := control.ReadLaunchProvenance(ctx, j, runID, attemptID)
			if err != nil {
				t.Fatalf("ReadLaunchProvenance: %v", err)
			}
			if rec.Generation == nil || *rec.Generation != tc.want { // I09 (v2 §3): explicit 0 differs from absent
				t.Fatalf("generation = %v, want %d", rec.Generation, tc.want)
			}
		})
	}

	refusals := []struct {
		name, reason string
		code         v2contract.Code
		raw          func(attemptID string) string
	}{
		{"null record", "malformed", v2contract.CodeInvalidContract, func(string) string { return "null" }},
		{"null generation", "malformed", v2contract.CodeInvalidContract, func(a string) string { return record(a, "null", nil) }},
		{"negative generation", "malformed", v2contract.CodeInvalidContract, func(a string) string { return record(a, "-1", nil) }},
		{"omitted generation", "malformed", v2contract.CodeInvalidContract, func(a string) string {
			return record(a, "1", func(r string) string { return strings.Replace(r, `,"generation":1`, "", 1) })
		}},
		{"duplicate generation", "malformed", v2contract.CodeInvalidContract, func(a string) string {
			return record(a, "1", func(r string) string { return strings.Replace(r, `"generation":1`, `"generation":1,"generation":0`, 1) })
		}},
		{"unknown field", "malformed", v2contract.CodeInvalidContract, func(a string) string {
			return record(a, "1", func(r string) string { return strings.Replace(r, `{`, `{"extra":1,`, 1) })
		}},
		{"malformed deadline", "malformed", v2contract.CodeInvalidContract, func(a string) string {
			return record(a, "1", func(r string) string { return strings.Replace(r, "2026-11-02T12:00:00Z", "soon", 1) })
		}},
		{"future version", "future", v2contract.CodeCapabilityUnsupported, func(a string) string {
			return record(a, "1", func(r string) string { return strings.Replace(r, `"schema_version":1`, `"schema_version":2`, 1) })
		}},
		{"wrong run", "identity_mismatch", v2contract.CodeInvalidContract, func(a string) string {
			return record(a, "1", func(r string) string { return strings.Replace(r, runID, "run_other", 1) })
		}},
		{"wrong attempt", "identity_mismatch", v2contract.CodeInvalidContract, func(a string) string {
			return record(a, "1", func(r string) string { return strings.Replace(r, a, "att_other", 1) })
		}},
		{"wrong token", "identity_mismatch", v2contract.CodeInvalidContract, func(a string) string {
			return record(a, "1", func(r string) string { return strings.Replace(r, tokenDigest(tokenSentinel), tokenDigest("x"), 1) })
		}},
		{"wrong nonce", "identity_mismatch", v2contract.CodeInvalidContract, func(a string) string {
			return record(a, "1", func(r string) string {
				return strings.Replace(r, `"nonce_sha256":"`+nonceSHA, `"nonce_sha256":"`+workers.NonceDigest("x"), 1)
			})
		}},
		{"wrong ladder", "identity_mismatch", v2contract.CodeInvalidContract, func(a string) string {
			return record(a, "1", func(r string) string {
				return strings.Replace(r, `"ladder_version":"`+ladder, `"ladder_version":"other`, 1)
			})
		}},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			attemptID := tokenAttempt(t, j, prod, runID, tokenSentinel)
			pinWithProvenance(t, j, prod, runID, attemptID, nonceSHA, tc.raw(attemptID))
			_, err := control.ReadLaunchProvenance(ctx, j, runID, attemptID)
			code, reason := provenanceCode(t, err)
			if code != tc.code || reason != tc.reason {
				t.Fatalf("%s = %s/%s, want %s/%s", tc.name, code, reason, tc.code, tc.reason)
			}
		})
	}
}
