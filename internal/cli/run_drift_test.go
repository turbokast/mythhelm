package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/adapters/claudecode"
	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/qualify"
)

// Hermetic fixtures: no native process, no network, no secret. Drift is reached
// below the gap gate (Q-15 holds the gap open, so a strict `cli.Main` run
// never gets past it): the seam tests feed persistDriftInvalidation the
// *DriftError that admission.ResolveQualification really returns for a seeded,
// drifted record, the link Decide passes up unchanged.

const (
	driftPinnedSHA   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	driftObservedSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

type driftWorld struct {
	dir      string
	manifest adapter.ConfigManifest
	evidence claudecode.AuthEvidence
	key      qualify.Key
}

// newDriftWorld seeds a live-qualified record pinned to driftPinnedSHA in a
// fresh state dir.
func newDriftWorld(t *testing.T) driftWorld {
	t.Helper()
	w := driftWorld{
		dir:      t.TempDir(),
		manifest: adapter.ConfigManifest{Digests: map[string]string{"project": strings.Repeat("b", 64)}},
		evidence: claudecode.AuthEvidence{LoggedIn: true, AuthMethod: "claude.ai", APIProvider: "firstParty", SubscriptionType: "max"},
	}
	desc := claudecode.New().Descriptor()
	w.key = qualify.Key{
		Harness:          desc.Harness,
		Surface:          desc.Surface,
		ExecutableDigest: "sha256:" + driftPinnedSHA,
		AdapterProtocol:  desc.ID + "+stream-json",
		OS:               runtime.GOOS,
		Arch:             runtime.GOARCH,
		ProviderEndpoint: "first-party-subscription",
		ModelSnapshot:    "unknown",
		EffortSettings:   "none",
		AuthCategory:     "claude.ai/max",
		ConfigDigest:     qualify.ConfigDigestOf(w.manifest.Digests),
		TrustProfile:     admission.ProfileTrustedHost,
		WorkspaceClass:   "local-checkout",
		EntitlementClass: "included-plan",
	}
	w.record(t)
	return w
}

// record stores a strict-admissible live record for the world's key as a
// new revision.
func (w driftWorld) record(t *testing.T) {
	t.Helper()
	reg, err := qualify.Open(t.Context(), w.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reg.Close() }()
	expiry := time.Now().Add(time.Hour)
	unknown := qualify.Column{Verdict: qualify.Unknown, Evidence: []qualify.Evidence{}}
	rec := qualify.Record{
		SchemaVersion: 2,
		Key:           w.key,
		Progress:      qualify.ProgressLiveQualified,
		Fidelity:      unknown,
		Entitlement: qualify.Column{Verdict: qualify.Proven, Evidence: []qualify.Evidence{{
			ID: "ev_live_1", Method: "authorised-live", Suite: "live-suite-1", Result: "pass",
			Uncertainty: "measured", Label: qualify.Observed, Source: "live-suite-1", Expiry: &expiry,
		}}},
		Lifecycle:    unknown,
		Capabilities: map[string]qualify.Capability{"stop_at_exhaustion": {Value: "supported", Evidence: "ev_live_1", Scope: "route", Expiry: &expiry}},
		Quota:        qualify.Datum{Quantity: "unknown", Label: qualify.DatumUnknown, Unit: "unknown", Scope: "unknown", Source: "unknown"},
	}
	if err := reg.Record(t.Context(), rec); err != nil {
		t.Fatal(err)
	}
}

func (w driftWorld) probe(sha string) adapter.Probe {
	return adapter.Probe{Executable: filepath.Join(string(filepath.Separator), "usr", "bin", "claude"),
		Version: "2.1.284", SHA256: sha, OS: runtime.GOOS, Arch: runtime.GOARCH, Compatibility: "fixture-tested on 2.1.284"}
}

// consult runs the strict consult for a probe digest against the state dir.
func (w driftWorld) consult(t *testing.T, sha string) (admission.Eligibility, error) {
	t.Helper()
	reg, err := admission.OpenQualificationRegistry(t.Context(), w.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reg.Close() }()
	return admission.ResolveQualification(t.Context(), reg, w.probe(sha), w.manifest, w.evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
}

// driftBlock returns the *DriftError a strict consult of the drifted probe
// produces.
func (w driftWorld) driftBlock(t *testing.T) error {
	t.Helper()
	elig, err := w.consult(t, driftObservedSHA)
	if _, ok := errors.AsType[*admission.DriftError](err); !ok || elig.Verdict != admission.Blocked {
		t.Fatalf("drifted strict consult = %+v, %v; want Blocked with a *DriftError", elig, err)
	}
	return err
}

type revisionRow struct {
	Revision   int
	RecordJSON string
}

// revisions reads every stored revision of every record, oldest first.
func revisions(t *testing.T, dir string) []revisionRow {
	t.Helper()
	db := rawDB(t, dir)
	defer func() { _ = db.Close() }()
	rows, err := db.QueryContext(t.Context(), `SELECT revision, record_json FROM qualification_records ORDER BY key_hash, revision`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []revisionRow
	for rows.Next() {
		var r revisionRow
		if err := rows.Scan(&r.Revision, &r.RecordJSON); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDriftBlockPersistsInvalidation(t *testing.T) {
	t.Parallel()
	w := newDriftWorld(t)
	before := revisions(t, w.dir)
	if len(before) != 1 {
		t.Fatalf("seeded %d revisions, want 1", len(before))
	}
	blockErr := w.driftBlock(t)
	drift, _ := errors.AsType[*admission.DriftError](blockErr)

	var diag bytes.Buffer
	got := persistDriftInvalidation(t.Context(), w.dir, blockErr, &diag)

	if !errors.Is(got, blockErr) {
		t.Errorf("returned error = %v, want the block unchanged", got)
	}
	if diag.Len() != 0 {
		t.Errorf("diagnostics = %q, want none on success", diag.String())
	}
	after := revisions(t, w.dir)
	if len(after) != 2 {
		t.Fatalf("revisions after the block = %d, want 2", len(after))
	}
	if after[0] != before[0] {
		t.Errorf("revision 1 changed:\nbefore %s\nafter  %s", before[0].RecordJSON, after[0].RecordJSON)
	}
	next, err := qualify.DecodeRecord([]byte(after[1].RecordJSON))
	if err != nil {
		t.Fatal(err)
	}
	if next.Revision != 2 || next.Progress != qualify.ProgressBlocked {
		t.Errorf("new revision = %d progress %q, want revision 2 blocked", next.Revision, next.Progress)
	}
	if next.Entitlement.Verdict != qualify.NotProven || len(next.Entitlement.Evidence) != 1 {
		t.Fatalf("entitlement = %+v, want not-proven with one marker", next.Entitlement)
	}
	marker := next.Entitlement.Evidence[0]
	if marker.Source != "drift-check" || !strings.Contains(marker.Uncertainty, drift.Reason) || !strings.Contains(marker.Uncertainty, "ev_live_1") {
		t.Errorf("marker = %+v, want the drift reason and the prior evidence id", marker)
	}
	if next.Fidelity.Verdict != qualify.Unknown || next.Lifecycle.Verdict != qualify.Unknown {
		t.Errorf("unclaimed columns changed: fidelity %q lifecycle %q, want unknown", next.Fidelity.Verdict, next.Lifecycle.Verdict)
	}
}

func TestPersistFailureNeverMasksBlock(t *testing.T) {
	t.Parallel()
	w := newDriftWorld(t)
	blockErr := w.driftBlock(t)
	drift, _ := errors.AsType[*admission.DriftError](blockErr)
	notADir := filepath.Join(t.TempDir(), "state-file")
	if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	unknownHash := &admission.DriftError{Blocked: drift.Blocked, KeyHash: strings.Repeat("0", 64), Reason: drift.Reason}

	tests := []struct {
		name     string
		stateDir string
		err      error
	}{
		{name: "state dir is not a directory", stateDir: notADir, err: blockErr},
		{name: "registry holds no record for the hash", stateDir: w.dir, err: unknownHash},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var diag bytes.Buffer
			got := persistDriftInvalidation(t.Context(), tt.stateDir, tt.err, &diag)
			if !errors.Is(got, tt.err) {
				t.Errorf("returned error = %v, want the original block", got)
			}
			if code := exitCode(got); code != ExitBlocked {
				t.Errorf("exit code = %d, want %d", code, ExitBlocked)
			}
			if blocked, ok := errors.AsType[*admission.BlockedError](got); !ok || blocked.Code != "qualification_drifted" {
				t.Errorf("block = %+v, want qualification_drifted", blocked)
			}
			want := "could not record the drift invalidation for key " + drift2hash(tt.err)
			if !strings.Contains(diag.String(), want) {
				t.Errorf("diagnostics = %q, want %q", diag.String(), want)
			}
		})
	}
	if rows := revisions(t, w.dir); len(rows) != 1 {
		t.Errorf("failed persists left %d revisions, want the seeded 1", len(rows))
	}
}

func drift2hash(err error) string {
	drift, _ := errors.AsType[*admission.DriftError](err)
	return drift.KeyHash
}

func TestNonDriftBlocksWriteNothing(t *testing.T) {
	t.Parallel()
	w := newDriftWorld(t)
	drift, _ := errors.AsType[*admission.DriftError](w.driftBlock(t))
	blocked := func(code string) error { return &admission.BlockedError{Code: code} }
	tests := []struct {
		name       string
		err        error
		wantChange bool
	}{
		{name: "no_qualification_record", err: blocked("no_qualification_record")},
		{name: "entitlement_not_proven", err: blocked("entitlement_not_proven")},
		{name: "ambiguous_qualification_match", err: blocked("ambiguous_qualification_match")},
		{name: "non-block error", err: errors.New("probing adapter: native missing")},
		// The kept broken input: the same table row shape with a drift block
		// must change the registry, so the unchanged rows above prove the
		// guard rather than an inert helper.
		{name: "drift block does write", err: drift, wantChange: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newDriftWorld(t).dir
			before := revisions(t, dir)
			var diag bytes.Buffer
			if got := persistDriftInvalidation(t.Context(), dir, tt.err, &diag); !errors.Is(got, tt.err) {
				t.Errorf("returned error = %v, want %v", got, tt.err)
			}
			changed := !slices.Equal(before, revisions(t, dir))
			if changed != tt.wantChange {
				t.Errorf("registry changed = %v, want %v (diagnostics %q)", changed, tt.wantChange, diag.String())
			}
		})
	}
}

func TestRestoredStateReadmitsOnlyAfterRetest(t *testing.T) {
	t.Parallel()
	w := newDriftWorld(t)

	// Control: before any drift the pinned probe is admitted, so the block
	// after invalidation comes from the invalidation, not from the fixture.
	if elig, err := w.consult(t, driftPinnedSHA); err != nil || elig.Verdict != admission.Eligible {
		t.Fatalf("pinned consult = %+v, %v; want Eligible", elig, err)
	}
	_ = persistDriftInvalidation(t.Context(), w.dir, w.driftBlock(t), &bytes.Buffer{})

	// Restoring the matching executable no longer drifts, and still blocks.
	elig, err := w.consult(t, driftPinnedSHA)
	if err != nil || elig.Verdict != admission.Blocked || elig.Reason != "entitlement_not_proven" {
		t.Fatalf("restored consult = %+v, %v; want Blocked entitlement_not_proven", elig, err)
	}

	// A new revision that re-proves the route admits again.
	w.record(t)
	if elig, err := w.consult(t, driftPinnedSHA); err != nil || elig.Verdict != admission.Eligible {
		t.Fatalf("re-proved consult = %+v, %v; want Eligible", elig, err)
	}
	if rows := revisions(t, w.dir); len(rows) != 3 {
		t.Errorf("revisions = %d, want 3 (live, invalidation, re-proof)", len(rows))
	}
}

// fakeClaudeOnPath installs a `claude` fixture answering --version and
// `auth status`; the blocked runs below never launch it.
func fakeClaudeOnPath(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the claudecode probe refuses Windows by design")
	}
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then printf '%s\\n' '2.1.284 (Claude Code)'; exit 0; fi\n" +
		"if [ \"$1\" = \"auth\" ] && [ \"$2\" = \"status\" ]; then\n" +
		"  printf '%s\\n' '{\"loggedIn\":true,\"authMethod\":\"claude.ai\",\"apiProvider\":\"firstParty\",\"subscriptionType\":\"max\",\"configDirectory\":\"/tmp/claude-fixture\",\"orgId\":\"org-drift-test\"}'\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 2\n"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o700); err != nil { // #nosec G306 -- executable fixture native under t.TempDir
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// strictRun is `mythhelm run` for the strict claudecode route over a clean
// repo, returning the exit code and the run.result reason.
func strictRun(t *testing.T, state string) (int, string) {
	t.Helper()
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "--quiet"}, {"config", "user.name", "Drift Test"}, {"config", "user.email", "drift@example.com"}} {
		if out, err := gitIn(repo, args...); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "task.md"), []byte("# drift task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "fixture"}} {
		if out, err := gitIn(repo, args...); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	t.Setenv("MYTHHELM_HOME", state)
	t.Setenv("HOME", t.TempDir()) // the native inventory reads the user's settings: never the developer's
	t.Chdir(repo)
	code, stdout, _ := runMain("run", "--task-file", filepath.Join(repo, "task.md"), "--adapter", "claudecode",
		"--billing", "subscription-only", "--execution-profile", "trusted-host", "--no-checks",
		"--non-interactive", "--strip-credential-env", "--format", "jsonl")
	for line := range strings.Lines(stdout) {
		var m struct {
			Type    string `json:"type"`
			Payload struct {
				Reason string `json:"reason"`
			} `json:"payload"`
		}
		if json.Unmarshal([]byte(line), &m) == nil && m.Type == "run.result" {
			return code, m.Payload.Reason
		}
	}
	t.Fatalf("no run.result in stdout:\n%s", stdout)
	return 0, ""
}

// TestStrictMainBlocksWriteNothing drives the packaged entry point to the
// reachable non-drift blocks: a missing registry (no_qualification_record,
// which skips the gap gate) and a present registry (the gap gate, a plain
// entitlement_not_proven block). Neither leaves a registry write. Not parallel:
// it sets PATH, MYTHHELM_HOME and the working directory.
func TestStrictMainBlocksWriteNothing(t *testing.T) {
	fakeClaudeOnPath(t)

	absent := filepath.Join(t.TempDir(), "absent-state")
	if code, reason := strictRun(t, absent); code != int(ExitBlocked) || reason != "no_qualification_record" {
		t.Errorf("missing registry: exit %d reason %q, want %d no_qualification_record", code, reason, ExitBlocked)
	}
	if _, err := os.Stat(filepath.Join(absent, journal.DBName)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing registry: stat db = %v, want it still absent", err)
	}

	w := newDriftWorld(t)
	before := revisions(t, w.dir)
	if code, reason := strictRun(t, w.dir); code != int(ExitBlocked) || reason != "entitlement_not_proven" {
		t.Errorf("present registry: exit %d reason %q, want %d entitlement_not_proven", code, reason, ExitBlocked)
	}
	if after := revisions(t, w.dir); !slices.Equal(before, after) {
		t.Errorf("present registry changed:\nbefore %v\nafter  %v", before, after)
	}
}

func gitIn(dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
