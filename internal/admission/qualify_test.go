package admission_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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
	"github.com/turbokast/mythhelm/internal/cli"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/qualify"
)

// TestMain dispatches a copy of the test binary installed as `claude` on PATH
// to the fake native; the fake answers --version and auth status from the
// fakeclaude.json sidecar in its own directory.
func TestMain(m *testing.M) {
	if base := filepath.Base(os.Args[0]); base == "claude" || base == "claude.exe" {
		os.Exit(fakeClaudeMain())
	}
	os.Exit(m.Run())
}

// fakeClaudeConfig is the fakeclaude.json sidecar beside the `claude` copy:
// the version and auth answers a consult needs. It never launches.
type fakeClaudeConfig struct {
	Version string         `json:"version"`
	Auth    map[string]any `json:"auth"`
}

func fakeClaudeMain() int {
	exe, err := os.Executable()
	if err != nil {
		return 9
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "fakeclaude.json")) // #nosec G304 -- test sidecar beside the copied binary
	if err != nil {
		return 9
	}
	var cfg fakeClaudeConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return 9
	}
	switch strings.Join(os.Args[1:], " ") {
	case "--version":
		fmt.Printf("%s (Claude Code)\n", cfg.Version)
		return 0
	case "auth status":
		out, _ := json.Marshal(cfg.Auth)
		fmt.Print(string(out))
		return 0
	}
	return 9
}

// installFakeClaude copies the test binary to a fresh dir as `claude` with
// its sidecar, and prepends the dir to PATH.
func installFakeClaude(t *testing.T, cfg fakeClaudeConfig) {
	t.Helper()
	dir := t.TempDir()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(self) // #nosec G304 -- test copies its own binary from os.Executable
	if err != nil {
		t.Fatal(err)
	}
	name := "claude"
	if runtime.GOOS == "windows" {
		name = "claude.exe"
	}
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o700); err != nil { // #nosec G306 G703 -- executable test fixture under t.TempDir
		t.Fatal(err)
	}
	sidecar, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fakeclaude.json"), sidecar, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// fixtureSHA256 is a fixed native digest the consult tests build keys from.
const fixtureSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// fixtureProbe is a fixed claudecode probe: caller overrides fields per leg.
func fixtureProbe() adapter.Probe {
	return adapter.Probe{
		Executable:    filepath.Join(string(filepath.Separator), "usr", "bin", "claude"),
		Version:       "2.1.284",
		SHA256:        fixtureSHA256,
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		Compatibility: "fixture-tested on 2.1.284",
	}
}

// fixtureManifest is a fixed config manifest with two digest sources.
func fixtureManifest() adapter.ConfigManifest {
	return adapter.ConfigManifest{Digests: map[string]string{
		"project": strings.Repeat("b", 64),
		"user":    strings.Repeat("c", 64),
	}}
}

// fixtureEvidence is a fixed first-party max auth evidence; the consult reads
// AuthMethod, APIProvider and SubscriptionType.
func fixtureEvidence() admission.AuthEvidence {
	return admission.AuthEvidence{
		LoggedIn:         true,
		AuthMethod:       "claude.ai",
		APIProvider:      "firstParty",
		SubscriptionType: "max",
		ConfigDirectory:  filepath.Join(string(filepath.Separator), "home", "fixture", ".claude"),
		IdentityRef:      strings.Repeat("d", 64),
	}
}

// observedKey transcribes the design §4 key table for the fixture values, so
// the agreement tests pin the table field by field.
func observedKey(probe adapter.Probe, manifest adapter.ConfigManifest, evidence admission.AuthEvidence, profile string) qualify.Key {
	desc := claudecode.New().Descriptor()
	endpoint := "unknown"
	if evidence.APIProvider == "firstParty" {
		endpoint = "first-party-subscription"
	}
	return qualify.Key{
		Harness:          desc.Harness,
		Surface:          desc.Surface,
		ExecutableDigest: "sha256:" + probe.SHA256,
		AdapterProtocol:  desc.ID + "+stream-json",
		OS:               probe.OS,
		Arch:             probe.Arch,
		ProviderEndpoint: endpoint,
		ModelSnapshot:    "unknown",
		EffortSettings:   "none",
		AuthCategory:     evidence.AuthMethod + "/" + evidence.SubscriptionType,
		ConfigDigest:     qualify.ConfigDigestOf(manifest.Digests),
		TrustProfile:     profile,
		WorkspaceClass:   "local-checkout",
		EntitlementClass: "included-plan",
	}
}

// unknownDatum is the AC-5.2 unknown quantity: labelled unknown, never zero.
func unknownDatum() qualify.Datum {
	return qualify.Datum{
		Quantity: "unknown",
		Label:    qualify.DatumUnknown,
		Unit:     "unknown",
		Scope:    "unknown",
		Source:   "unknown",
	}
}

// unknownColumn is a column with nothing claimed.
func unknownColumn() qualify.Column {
	return qualify.Column{Verdict: qualify.Unknown, Evidence: []qualify.Evidence{}}
}

// liveEvidence is one unexpired (future=true) or expired live evidence entry.
func liveEvidence(future bool) []qualify.Evidence {
	expiry := time.Now().Add(time.Hour)
	if !future {
		expiry = time.Now().Add(-time.Hour)
	}
	return []qualify.Evidence{{
		ID:          "ev_live_1",
		Method:      "authorised-live",
		Suite:       "live-suite-1",
		Result:      "pass",
		Uncertainty: "measured",
		Label:       qualify.Observed,
		Source:      "live-suite-1",
		Expiry:      &expiry,
	}}
}

// liveStop is a supported stop_at_exhaustion capability, unexpired or expired.
func liveStop(future bool) qualify.Capability {
	expiry := time.Now().Add(time.Hour)
	if !future {
		expiry = time.Now().Add(-time.Hour)
	}
	return qualify.Capability{Value: "supported", Evidence: "ev_live_1", Scope: "route", Expiry: &expiry}
}

// liveRecord is a strict-admissible record: live-qualified, entitlement
// proven with the given evidence, stop marker as given, quota unknown.
func liveRecord(key qualify.Key, evidence []qualify.Evidence, stop qualify.Capability) qualify.Record {
	return qualify.Record{
		SchemaVersion: 2,
		Key:           key,
		Progress:      qualify.ProgressLiveQualified,
		Fidelity:      unknownColumn(),
		Entitlement:   qualify.Column{Verdict: qualify.Proven, Evidence: evidence},
		Lifecycle:     unknownColumn(),
		Capabilities:  map[string]qualify.Capability{"stop_at_exhaustion": stop},
		Quota:         unknownDatum(),
	}
}

// openRegistry opens a read-write registry over dir for the test.
func openRegistry(t *testing.T, dir string) *qualify.Registry {
	t.Helper()
	reg, err := qualify.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reg.Close() })
	return reg
}

// storeRecord records rec, failing the test on error.
func storeRecord(t *testing.T, reg *qualify.Registry, rec qualify.Record) {
	t.Helper()
	if err := reg.Record(t.Context(), rec); err != nil {
		t.Fatal(err)
	}
}

// resolve runs the consult for the fixture identity and billing mode.
func resolve(t *testing.T, reg *qualify.Registry, probe adapter.Probe, manifest adapter.ConfigManifest, evidence admission.AuthEvidence, billing, profile string) admission.Eligibility {
	t.Helper()
	elig, err := admission.ResolveQualification(t.Context(), reg, probe, manifest, evidence, billing, profile)
	if err != nil {
		t.Fatal(err)
	}
	return elig
}

// fileURI builds a writable SQLite file URI for dir/mythhelm.db.
func fileURI(dir string) string {
	p := filepath.ToSlash(filepath.Join(dir, journal.DBName))
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// downgradeToV1 closes over a fresh v2 database and rewrites it as a v1
// database the supervisor could still migrate: user_version 1 with the v2
// table dropped.
func downgradeToV1(t *testing.T, dir string) {
	t.Helper()
	j, err := journal.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", fileURI(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`DROP TABLE qualification_records`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
}

// userVersion reads PRAGMA user_version through a throwaway connection.
func userVersion(t *testing.T, dir string) int {
	t.Helper()
	db, err := sql.Open("sqlite", fileURI(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}

// requireUnavailable fails unless err is the fail-closed registry error.
func requireUnavailable(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "qualify: registry unavailable") {
		t.Fatalf("want qualify: registry unavailable, got %v", err)
	}
}

// require consult verdict helpers keep the mapping table readable.
func requireVerdict(t *testing.T, elig admission.Eligibility, verdict admission.EligibilityVerdict, reason string) {
	t.Helper()
	if elig.Verdict != verdict || elig.Reason != reason {
		t.Fatalf("verdict = (%s, %q), want (%s, %q)", elig.Verdict, elig.Reason, verdict, reason)
	}
}

func TestNoRecordBlocks(t *testing.T) {
	reg := openRegistry(t, t.TempDir())
	for _, rec := range qualify.SeedV1() {
		storeRecord(t, reg, rec)
	}
	probe, manifest, evidence := fixtureProbe(), fixtureManifest(), fixtureEvidence()
	elig := resolve(t, reg, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, elig, admission.Blocked, "no_qualification_record")
	if elig.Record != nil {
		t.Fatalf("absent consult attached %+v, want nil", elig.Record)
	}
}

func TestNonLiveRecordBlocksStrict(t *testing.T) {
	reg := openRegistry(t, t.TempDir())
	probe, manifest, evidence := fixtureProbe(), fixtureManifest(), fixtureEvidence()
	rec := liveRecord(observedKey(probe, manifest, evidence, admission.ProfileTrustedHost), liveEvidence(true), liveStop(true))
	rec.Progress = qualify.ProgressFixtureTested
	rec.Entitlement = qualify.Column{Verdict: qualify.NotProven, Evidence: []qualify.Evidence{}}
	rec.NextTest = "run the authorised live suite; authority: maintainer live-test grant"
	storeRecord(t, reg, rec)
	elig := resolve(t, reg, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, elig, admission.Blocked, "entitlement_not_proven")
	if elig.Record == nil {
		t.Fatal("non-live consult attached no record")
	}
}

func TestAmbiguousMatchBlocks(t *testing.T) {
	reg := openRegistry(t, t.TempDir())
	probe, manifest, evidence := fixtureProbe(), fixtureManifest(), fixtureEvidence()
	key := observedKey(probe, manifest, evidence, admission.ProfileTrustedHost)
	for _, digest := range []string{"sha256:" + strings.Repeat("1", 64), "sha256:" + strings.Repeat("2", 64)} {
		dupe := key
		dupe.ExecutableDigest = digest
		rec := liveRecord(dupe, liveEvidence(true), liveStop(true))
		rec.Progress = qualify.ProgressBlocked
		rec.NextTest = "fixture; authority: test"
		storeRecord(t, reg, rec)
	}
	probe.SHA256 = strings.Repeat("3", 64)
	elig := resolve(t, reg, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, elig, admission.Blocked, "ambiguous_qualification_match")
	if elig.Record != nil {
		t.Fatalf("ambiguous consult attached %+v, want nil", elig.Record)
	}
	permissive := resolve(t, reg, probe, manifest, evidence, admission.BillingSubscriptionDeclared, admission.ProfileTrustedHost)
	if permissive.Verdict != admission.Eligible || permissive.Record != nil {
		t.Fatalf("declared ambiguous consult = (%s, %+v), want (eligible, nil)", permissive.Verdict, permissive.Record)
	}

	// Narrowed: a third record on another account class is not a candidate
	// for this identity, so the same two records stay ambiguous, while an
	// observed identity on the third record's class matches it alone.
	other := key
	other.AuthCategory = "claude.ai/pro"
	other.ExecutableDigest = "sha256:" + strings.Repeat("4", 64)
	rec := liveRecord(other, liveEvidence(true), liveStop(true))
	rec.Progress = qualify.ProgressBlocked
	rec.NextTest = "fixture; authority: test"
	storeRecord(t, reg, rec)
	stillAmbiguous := resolve(t, reg, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, stillAmbiguous, admission.Blocked, "ambiguous_qualification_match")
	pro := evidence
	pro.SubscriptionType = "pro"
	narrowed, drift := resolveDrift(t, reg, probe, manifest, pro)
	requireVerdict(t, narrowed, admission.Blocked, "qualification_drifted")
	if narrowed.Record == nil || narrowed.Record.Key.AuthCategory != "claude.ai/pro" || drift.KeyHash != qualify.KeyHash(other) {
		t.Fatalf("narrowed consult matched %+v (hash %s), want the pro record", narrowed.Record, drift.KeyHash)
	}
}

func TestTamperedDigestNeverAmbiguous(t *testing.T) {
	// A tampered digest can carry arbitrary text — including text naming the
	// ambiguity report — because the digest field is free-form and only
	// checked by recomputation. The consult must surface the corruption as a
	// registry error under every billing mode, never classify it as an
	// ambiguous match (which would admit declared runs).
	dir := t.TempDir()
	probe, manifest, evidence := fixtureProbe(), fixtureManifest(), fixtureEvidence()
	rec := liveRecord(observedKey(probe, manifest, evidence, admission.ProfileTrustedHost), liveEvidence(true), liveStop(true))
	rec.Digest = "sha256: tampered digest carrying the ambiguous stable match text"
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	j, err := journal.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.InsertQualificationRecord(t.Context(), qualify.KeyHash(rec.Key), 1, rec.Digest, string(raw)); err != nil {
		_ = j.Close()
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	reg := openRegistry(t, dir)
	for _, billing := range []string{admission.BillingSubscriptionOnly, admission.BillingSubscriptionDeclared} {
		_, err := admission.ResolveQualification(t.Context(), reg, probe, manifest, evidence, billing, admission.ProfileTrustedHost)
		requireUnavailable(t, err)
	}
}

func TestUnsupportedSurfaceMapsUnsupported(t *testing.T) {
	reg := openRegistry(t, t.TempDir())
	probe, manifest, evidence := fixtureProbe(), fixtureManifest(), fixtureEvidence()
	rec := liveRecord(observedKey(probe, manifest, evidence, admission.ProfileTrustedHost), liveEvidence(true), liveStop(true))
	rec.Progress = qualify.ProgressUnsupported
	storeRecord(t, reg, rec)
	for _, billing := range []string{admission.BillingSubscriptionOnly, admission.BillingSubscriptionDeclared, admission.BillingLocalScripted} {
		elig := resolve(t, reg, probe, manifest, evidence, billing, admission.ProfileTrustedHost)
		requireVerdict(t, elig, admission.Unsupported, "qualification_unsupported")
		if elig.Record == nil {
			t.Fatalf("unsupported consult under %s attached no record", billing)
		}
	}

	// The CLI leg below needs the claudecode probe, which refuses Windows
	// by design (process_tree_ownership unsupported); the resolve() legs
	// above keep covering the mapping on every platform.
	if runtime.GOOS == "windows" {
		t.Skip("claudecode probe refuses Windows; the CLI leg cannot run there")
	}

	// An unsupported record blocks the declared run as a capability refusal.
	world := newDecideWorld(t)
	seeded := t.TempDir()
	seedStable(t, seeded, qualify.ProgressUnsupported)
	t.Setenv("MYTHHELM_HOME", seeded)
	t.Chdir(world.repo)
	var out, diagnostics bytes.Buffer
	code := cli.Main([]string{"run", "--adapter", "claudecode", "--billing", "subscription-declared", "--task-file", world.task,
		"--execution-profile", "trusted-host", "--non-interactive", "--no-checks", "--format", "jsonl",
		"--declare-entitlement", "plan=max,extra-usage=disabled", "--strip-credential-env"},
		cli.Stdio{In: strings.NewReader(""), Out: &out, Err: &diagnostics})
	if code != int(cli.ExitCapability) || !strings.Contains(out.String(), `"reason":"qualification_unsupported"`) {
		t.Fatalf("unsupported CLI = %d, %s, %s", code, &out, &diagnostics)
	}
}

func TestExpiredEvidenceIgnored(t *testing.T) {
	reg := openRegistry(t, t.TempDir())
	probe, manifest, evidence := fixtureProbe(), fixtureManifest(), fixtureEvidence()
	key := observedKey(probe, manifest, evidence, admission.ProfileTrustedHost)
	storeRecord(t, reg, liveRecord(key, liveEvidence(false), liveStop(true)))
	stale := resolve(t, reg, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, stale, admission.Blocked, "entitlement_not_proven")

	// The expired-only record consults exactly like a not-proven one.
	plainReg := openRegistry(t, t.TempDir())
	unprovenRec := liveRecord(key, []qualify.Evidence{}, liveStop(true))
	unprovenRec.Entitlement = qualify.Column{Verdict: qualify.NotProven, Evidence: []qualify.Evidence{}}
	storeRecord(t, plainReg, unprovenRec)
	plain := resolve(t, plainReg, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, plain, stale.Verdict, stale.Reason)

	fresh := openRegistry(t, t.TempDir())
	storeRecord(t, fresh, liveRecord(key, liveEvidence(true), liveStop(true)))
	qualifies := resolve(t, fresh, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, qualifies, admission.Eligible, "")
	if qualifies.Record == nil {
		t.Fatal("qualifying consult attached no record")
	}

	// An expired entry beside one without an expiry still qualifies: the
	// expired entry is ignored and entries without an expiry never expire.
	mixed := openRegistry(t, t.TempDir())
	evergreen := liveEvidence(true)[0]
	evergreen.Expiry = nil
	storeRecord(t, mixed, liveRecord(key, append(liveEvidence(false), evergreen), liveStop(true)))
	combined := resolve(t, mixed, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, combined, admission.Eligible, "")
}

func TestExpiredCapabilityIgnored(t *testing.T) {
	reg := openRegistry(t, t.TempDir())
	probe, manifest, evidence := fixtureProbe(), fixtureManifest(), fixtureEvidence()
	key := observedKey(probe, manifest, evidence, admission.ProfileTrustedHost)
	storeRecord(t, reg, liveRecord(key, liveEvidence(true), liveStop(false)))
	stale := resolve(t, reg, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, stale, admission.Blocked, "stop_at_exhaustion_unproven")

	// An expired supported marker consults exactly like a missing one.
	// (AC-3.3: the exhausted route must reliably stop, not merely claim to.)
	unknown := openRegistry(t, t.TempDir())
	unmarked := liveRecord(key, liveEvidence(true), liveStop(true))
	unmarked.Capabilities = map[string]qualify.Capability{}
	storeRecord(t, unknown, unmarked)
	plain := resolve(t, unknown, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, plain, stale.Verdict, stale.Reason)

	fresh := openRegistry(t, t.TempDir())
	storeRecord(t, fresh, liveRecord(key, liveEvidence(true), liveStop(true)))
	qualifies := resolve(t, fresh, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, qualifies, admission.Eligible, "")
}

func TestConsultSchemaMismatchMapsAbsent(t *testing.T) {
	dir := t.TempDir()
	downgradeToV1(t, dir)
	reg, err := admission.OpenQualificationRegistry(t.Context(), dir)
	if err != nil {
		t.Fatalf("v1 database errors the consult: %v", err)
	}
	if reg != nil {
		_ = reg.Close()
		t.Fatal("v1 database yields a registry, want nil (absent record)")
	}
	probe, manifest, evidence := fixtureProbe(), fixtureManifest(), fixtureEvidence()
	strict := resolve(t, nil, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, strict, admission.Blocked, "no_qualification_record")
	permissive := resolve(t, nil, probe, manifest, evidence, admission.BillingSubscriptionDeclared, admission.ProfileTrustedHost)
	requireVerdict(t, permissive, admission.Eligible, "")
	if version := userVersion(t, dir); version != 1 {
		t.Fatalf("consult migrated the v1 database to version %d, want 1", version)
	}

	denied := t.TempDir()
	restrictDir(t, denied)
	_, err = admission.OpenQualificationRegistry(t.Context(), denied)
	requireUnavailable(t, err)
}

func TestUnknownQuotaAdmitsStopAtExhaustion(t *testing.T) {
	reg := openRegistry(t, t.TempDir())
	probe, manifest, evidence := fixtureProbe(), fixtureManifest(), fixtureEvidence()
	key := observedKey(probe, manifest, evidence, admission.ProfileTrustedHost)
	forever := liveStop(true)
	forever.Expiry = nil
	storeRecord(t, reg, liveRecord(key, liveEvidence(true), forever))
	elig := resolve(t, reg, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, elig, admission.Eligible, "")
	if elig.Record == nil {
		t.Fatal("qualifying consult attached no record")
	}
	if quota := elig.Record.Quota; quota.Quantity != "unknown" || quota.Label != qualify.DatumUnknown {
		t.Fatalf("quota = (%q, %s), want (unknown, unknown)", quota.Quantity, quota.Label)
	}

	unmarked := openRegistry(t, t.TempDir())
	rec := liveRecord(key, liveEvidence(true), liveStop(true))
	rec.Capabilities = map[string]qualify.Capability{}
	storeRecord(t, unmarked, rec)
	blocked := resolve(t, unmarked, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, blocked, admission.Blocked, "stop_at_exhaustion_unproven")

	refused := openRegistry(t, t.TempDir())
	unsupported := liveRecord(key, liveEvidence(true), liveStop(true))
	unsupported.Capabilities = map[string]qualify.Capability{
		"stop_at_exhaustion": {Value: "unsupported", Evidence: "ev_live_1", Scope: "route"},
	}
	storeRecord(t, refused, unsupported)
	negative := resolve(t, refused, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, negative, blocked.Verdict, blocked.Reason)
}

func TestRegistryUnreadableErrors(t *testing.T) {
	// A path that cannot be a registry directory fails the open loudly.
	notDir := filepath.Join(t.TempDir(), "mythhelm.db")
	if err := os.WriteFile(notDir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := admission.OpenQualificationRegistry(t.Context(), notDir)
	requireUnavailable(t, err)

	// A corrupt row fails the consult loudly, never silently eligible.
	dir := t.TempDir()
	reg := openRegistry(t, dir)
	probe, manifest, evidence := fixtureProbe(), fixtureManifest(), fixtureEvidence()
	storeRecord(t, reg, liveRecord(observedKey(probe, manifest, evidence, admission.ProfileTrustedHost), liveEvidence(true), liveStop(true)))
	if err := reg.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", fileURI(dir))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE qualification_records SET record_json = 'not json'`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	tainted := openRegistry(t, dir)
	_, err = admission.ResolveQualification(t.Context(), tainted, probe, manifest, evidence, admission.BillingSubscriptionDeclared, admission.ProfileTrustedHost)
	requireUnavailable(t, err)

	denied := t.TempDir()
	restrictDir(t, denied)
	_, err = admission.OpenQualificationRegistry(t.Context(), denied)
	requireUnavailable(t, err)
}

func TestConsultMissingMapsAbsent(t *testing.T) {
	probe, manifest, evidence := fixtureProbe(), fixtureManifest(), fixtureEvidence()
	strict := resolve(t, nil, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, strict, admission.Blocked, "no_qualification_record")
	if strict.Record != nil {
		t.Fatalf("nil-registry strict consult attached %+v, want nil", strict.Record)
	}
	permissive := resolve(t, nil, probe, manifest, evidence, admission.BillingSubscriptionDeclared, admission.ProfileTrustedHost)
	requireVerdict(t, permissive, admission.Eligible, "")
	if permissive.Record != nil {
		t.Fatalf("nil-registry declared consult attached %+v, want nil", permissive.Record)
	}

	// A missing dir or database never errors the consult open.
	missing, err := admission.OpenQualificationRegistry(t.Context(), filepath.Join(t.TempDir(), "no-such-dir"))
	if err != nil || missing != nil {
		t.Fatalf("missing-dir open = (%v, %v), want (nil, nil)", missing, err)
	}
	empty, err := admission.OpenQualificationRegistry(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("database-less dir errors the consult: %v", err)
	}
	if empty == nil {
		t.Fatal("database-less dir yields nil, want an empty registry")
	}
	defer func() { _ = empty.Close() }()
	absent := resolve(t, empty, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, absent, admission.Blocked, "no_qualification_record")

	// An unknown billing mode is a caller error, never silent eligibility.
	_, err = admission.ResolveQualification(t.Context(), empty, probe, manifest, evidence, "metered", admission.ProfileTrustedHost)
	if !errors.Is(err, admission.ErrInvalid) || !strings.Contains(err.Error(), "unknown billing mode") {
		t.Fatalf("unknown billing consult = %v, want an unknown-billing-mode error", err)
	}
}

func TestResolveQualificationFindsRecordDraft(t *testing.T) {
	reg := openRegistry(t, t.TempDir())
	probe, manifest, evidence := fixtureProbe(), fixtureManifest(), fixtureEvidence()
	draft, err := claudecode.RecordDraft(observedKey(probe, manifest, evidence, admission.ProfileTrustedHost))
	if err != nil {
		t.Fatal(err)
	}
	storeRecord(t, reg, draft)
	found := resolve(t, reg, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, found, admission.Blocked, "entitlement_not_proven")
	if found.Record == nil {
		t.Fatal("draft consult attached no record")
	}

	// Drifting the fixture digest consults the same record as drifted.
	probe.SHA256 = strings.Repeat("e", 64)
	drifted, drift := resolveDrift(t, reg, probe, manifest, evidence)
	requireVerdict(t, drifted, admission.Blocked, "qualification_drifted")
	if !strings.Contains(drift.Reason, "executable_digest") {
		t.Fatalf("drifted draft reason %q, want it to name executable_digest", drift.Reason)
	}
}

func TestDriftedRecordBlocks(t *testing.T) {
	reg := openRegistry(t, t.TempDir())
	probe, manifest, evidence := fixtureProbe(), fixtureManifest(), fixtureEvidence()
	key := observedKey(probe, manifest, evidence, admission.ProfileTrustedHost)
	storeRecord(t, reg, liveRecord(key, liveEvidence(true), liveStop(true)))
	probe.SHA256 = strings.Repeat("f", 64)
	drifted, drift := resolveDrift(t, reg, probe, manifest, evidence)
	requireVerdict(t, drifted, admission.Blocked, "qualification_drifted")
	if !strings.Contains(drift.Reason, "executable_digest") {
		t.Fatalf("drifted reason %q, want it to name executable_digest", drift.Reason)
	}
	if drifted.Record == nil {
		t.Fatal("drifted consult attached no record")
	}

	configured := openRegistry(t, t.TempDir())
	storeRecord(t, configured, liveRecord(key, liveEvidence(true), liveStop(true)))
	probe.SHA256 = fixtureSHA256
	manifest.Digests["project"] = strings.Repeat("0", 64)
	driftedConfig, configDrift := resolveDrift(t, configured, probe, manifest, evidence)
	requireVerdict(t, driftedConfig, admission.Blocked, "qualification_drifted")
	if !strings.Contains(configDrift.Reason, "config_digest") {
		t.Fatalf("config-drift reason %q, want it to name config_digest", configDrift.Reason)
	}
}

// decideWorld is a runnable declared claudecode admission: a fake native on
// PATH, an empty home, a clean repo, a task file and a credential-free env.
type decideWorld struct {
	repo string
	task string
	env  []string
}

func newDecideWorld(t *testing.T) decideWorld {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	installFakeClaude(t, fakeClaudeConfig{
		Version: "2.1.284",
		Auth: map[string]any{
			"loggedIn": true, "authMethod": "claude.ai", "apiProvider": "firstParty",
			"subscriptionType": "max", "configDirectory": t.TempDir(), "orgId": "org-qualification-test",
		},
	})
	return decideWorld{repo: initRepo(t), task: writeTask(t), env: decideEnv()}
}

// request builds the declared admission request over stateDir.
func (w decideWorld) request(stateDir string) admission.Request {
	return admission.Request{
		StateDir:           stateDir,
		Repo:               w.repo,
		TaskFile:           w.task,
		Adapter:            admission.AdapterClaudeCode,
		Billing:            admission.BillingSubscriptionDeclared,
		ExecutionProfile:   admission.ProfileTrustedHost,
		Env:                w.env,
		DeclareEntitlement: "plan=max,extra-usage=disabled",
		NoChecks:           true,
	}
}

// decideEnv is the process env without credential-route names, so the fixture
// never trips the credential screen on ambient CI variables.
func decideEnv() []string {
	names := claudecode.CredentialOverrideNames()
	var out []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(name, n) }) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// initRepo creates a clean git repo with local identity and one commit.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-c", "commit.gpgsign=false", "-c", "safe.directory=*"}, args...)
		cmd := exec.Command("git", full...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	git("init")
	git("config", "user.name", "Qualification Test")
	git("config", "user.email", "qual-test@example.com")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "fixture")
	return dir
}

// writeTask writes a task file outside any repo.
func writeTask(t *testing.T) string {
	t.Helper()
	task := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(task, []byte("# fixture task\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return task
}

// seedStable stores one record carrying the Decide fixture's identity (every
// non-drift dimension exact) with unestablished digests, so the consult
// matches it without drift.
func seedStable(t *testing.T, dir string, progress qualify.Progress) {
	t.Helper()
	desc := claudecode.New().Descriptor()
	rec := qualify.Record{
		SchemaVersion: 2,
		Key: qualify.Key{
			Harness:          desc.Harness,
			Surface:          desc.Surface,
			ExecutableDigest: "unknown",
			AdapterProtocol:  desc.ID + "+stream-json",
			OS:               runtime.GOOS,
			Arch:             runtime.GOARCH,
			ConfigDigest:     "unknown",
			TrustProfile:     admission.ProfileTrustedHost,
			EntitlementClass: "included-plan",
			ProviderEndpoint: "first-party-subscription",
			ModelSnapshot:    "unknown",
			EffortSettings:   "none",
			AuthCategory:     "claude.ai/max",
			WorkspaceClass:   "local-checkout",
		},
		Progress:     progress,
		Fidelity:     unknownColumn(),
		Entitlement:  unknownColumn(),
		Lifecycle:    unknownColumn(),
		Capabilities: map[string]qualify.Capability{},
		Quota:        unknownDatum(),
		NextTest:     "fixture; authority: test",
	}
	if progress == qualify.ProgressUnsupported {
		rec.NextTest = ""
	}
	storeRecord(t, openRegistry(t, dir), rec)
}

func TestDeclaredUnaffectedByMissingRecord(t *testing.T) {
	probe, manifest, evidence := fixtureProbe(), fixtureManifest(), fixtureEvidence()
	missing := resolve(t, nil, probe, manifest, evidence, admission.BillingSubscriptionDeclared, admission.ProfileTrustedHost)
	requireVerdict(t, missing, admission.Eligible, "")
	if missing.Record != nil {
		t.Fatalf("declared missing consult attached %+v, want nil", missing.Record)
	}
	scripted := resolve(t, nil, probe, manifest, evidence, admission.BillingLocalScripted, admission.ProfileTrustedHost)
	requireVerdict(t, scripted, admission.Eligible, "")

	nonLive := openRegistry(t, t.TempDir())
	rec := liveRecord(observedKey(probe, manifest, evidence, admission.ProfileTrustedHost), liveEvidence(true), liveStop(true))
	rec.Progress = qualify.ProgressBlocked
	rec.Entitlement = unknownColumn()
	rec.NextTest = "fixture; authority: test"
	storeRecord(t, nonLive, rec)
	attached := resolve(t, nonLive, probe, manifest, evidence, admission.BillingSubscriptionDeclared, admission.ProfileTrustedHost)
	requireVerdict(t, attached, admission.Eligible, "")
	if attached.Record == nil {
		t.Fatal("declared non-live consult attached no record")
	}

	// The Decide legs below need the claudecode probe, which refuses Windows
	// by design (process_tree_ownership unsupported); the resolve() legs
	// above keep covering the mapping on every platform.
	if runtime.GOOS == "windows" {
		t.Skip("claudecode probe refuses Windows; the Decide legs cannot run there")
	}

	// The declared dogfood path admits with no registry at all.
	world := newDecideWorld(t)
	decided, err := admission.Decide(t.Context(), world.request(t.TempDir()))
	if err != nil {
		t.Fatalf("declared Decide with no registry: %v", err)
	}
	if decided.Probe.Version != "2.1.284" {
		t.Fatalf("probed version %q, want the fake native's 2.1.284", decided.Probe.Version)
	}

	// And with a non-live record waiting, which attaches without blocking.
	seeded := t.TempDir()
	seedStable(t, seeded, qualify.ProgressBlocked)
	if _, err := admission.Decide(t.Context(), world.request(seeded)); err != nil {
		t.Fatalf("declared Decide with a non-live record: %v", err)
	}
}

// strictCLI runs the packaged strict claudecode path through cli.Main against
// a fake native and the registry state at stateDir.
func strictCLI(t *testing.T, world decideWorld, stateDir string) (code int, out, diagnostics string) {
	t.Helper()
	t.Setenv("MYTHHELM_HOME", stateDir)
	t.Chdir(world.repo)
	var stdout, stderr bytes.Buffer
	code = cli.Main([]string{"run", "--adapter", "claudecode", "--billing", "subscription-only", "--task-file", world.task, "--execution-profile", "trusted-host", "--non-interactive", "--no-checks", "--strip-credential-env", "--format", "jsonl"}, cli.Stdio{In: strings.NewReader(""), Out: &stdout, Err: &stderr})
	return code, stdout.String(), stderr.String()
}

func TestStrictStillBlocksEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("claudecode probe refuses Windows; the strict path cannot run there")
	}
	world := newDecideWorld(t)
	code, out, diagnostics := strictCLI(t, world, missingStateDir(t))
	if code != int(cli.ExitBlocked) || !strings.Contains(out, `"reason":"no_qualification_record"`) {
		t.Fatalf("strict CLI = %d, %s, %s", code, out, diagnostics)
	}
}

// restrictDir removes all permission bits from dir, skipping where permission
// bits cannot restrict (Windows) or the process ignores them (root).
func restrictDir(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("permission bits do not restrict reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not restrict root")
	}
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) //nolint:gosec // G302: the fixture dir must be traversable again for TempDir removal
}

// resolveDrift runs the strict consult expecting a drift refusal: the
// *DriftError the consult returns and the eligibility behind it.
func resolveDrift(t *testing.T, reg *qualify.Registry, probe adapter.Probe, manifest adapter.ConfigManifest, evidence admission.AuthEvidence) (admission.Eligibility, *admission.DriftError) {
	t.Helper()
	elig, err := admission.ResolveQualification(t.Context(), reg, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	drift, ok := errors.AsType[*admission.DriftError](err)
	if !ok {
		t.Fatalf("drifted consult error = %v, want a *DriftError", err)
	}
	return elig, drift
}

// strictLiveRegistry returns a state dir whose registry holds a live-qualified,
// entitlement-proven record for the exact key the world's fake native observes.
func strictLiveRegistry(t *testing.T, world decideWorld) string {
	t.Helper()
	probed, err := admission.Decide(t.Context(), world.request(t.TempDir()))
	if err != nil {
		t.Fatalf("declared Decide to observe the key: %v", err)
	}
	key := observedKey(probed.Probe, probed.Proposal.Manifest, *probed.NativeAuth, probed.Profile.Name)
	dir := t.TempDir()
	reg := openRegistry(t, dir)
	storeRecord(t, reg, liveRecord(key, liveEvidence(true), liveStop(true)))
	if err := reg.Close(); err != nil {
		t.Fatal(err)
	}
	return dir
}

// missingStateDir is a state dir that does not exist, so the consult sees a
// missing registry (an existing dir without a database reads as an empty,
// present one).
func missingStateDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "absent")
}

func strictRequest(world decideWorld, stateDir string) admission.Request {
	req := world.request(stateDir)
	req.Billing = admission.BillingSubscriptionOnly
	req.DeclareEntitlement = ""
	return req
}

func TestStrictAdmitsLiveProven(t *testing.T) {
	reg := openRegistry(t, t.TempDir())
	probe, manifest, evidence := fixtureProbe(), fixtureManifest(), fixtureEvidence()
	storeRecord(t, reg, liveRecord(observedKey(probe, manifest, evidence, admission.ProfileTrustedHost), liveEvidence(true), liveStop(true)))
	elig := resolve(t, reg, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, elig, admission.Eligible, "")

	// A passed declaration never reaches the strict posture (I15).
	posture, err := admission.ResolveBilling(t.Context(), admission.BillingSubscriptionOnly, evidence, &admission.Declaration{PlanClass: "max", ExtraUsage: "disabled"}, elig)
	if err != nil {
		t.Fatal(err)
	}
	want := admission.BillingPosture{Mode: admission.BillingSubscriptionOnly, CredentialProvenance: "native-login", EntitlementClass: "included-plan", EntitlementSource: "registry:live-qualified", PaidContinuation: "prevented", PaidContinuationUserDeclaration: "", Qualified: true, G05: "passed"}
	if posture != want {
		t.Fatalf("strict posture = %+v, want %+v", posture, want)
	}

	// The same key one account class away is another identity, never admitted.
	pro := evidence
	pro.SubscriptionType = "pro"
	absent := resolve(t, reg, probe, manifest, pro, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
	requireVerdict(t, absent, admission.Blocked, "no_qualification_record")
}

func TestStrictGapBlocksEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("claudecode probe refuses Windows; the strict path cannot run there")
	}
	if len(claudecode.GapSources(claudecode.UnresolvedSources)) == 0 {
		t.Skip("the source gap is resolved; the post-resolution strict proof owns this path")
	}
	world := newDecideWorld(t)
	dir := strictLiveRegistry(t, world)
	code, out, diagnostics := strictCLI(t, world, dir)
	if code != int(cli.ExitBlocked) || !strings.Contains(out, `"reason":"entitlement_not_proven"`) || !strings.Contains(out+diagnostics, "missing-source:managed-remote-cache") {
		t.Fatalf("strict gap CLI = %d, %s, %s", code, out, diagnostics)
	}
}

func TestStrictConsultReasonsDirect(t *testing.T) {
	probe, manifest, evidence := fixtureProbe(), fixtureManifest(), fixtureEvidence()
	key := observedKey(probe, manifest, evidence, admission.ProfileTrustedHost)

	empty := openRegistry(t, t.TempDir())
	requireVerdict(t, resolve(t, empty, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost), admission.Blocked, "no_qualification_record")

	for _, tc := range []struct {
		name   string
		mutate func(*qualify.Record)
	}{
		{"fixture-only evidence does not prove entitlement", func(r *qualify.Record) {
			r.Progress = qualify.ProgressFixtureTested
			r.Entitlement.Evidence[0].Method = "offline-fixture"
		}},
		{"live record with an unproven entitlement column", func(r *qualify.Record) {
			r.Entitlement = qualify.Column{Verdict: qualify.NotProven, Evidence: []qualify.Evidence{}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg := openRegistry(t, t.TempDir())
			rec := liveRecord(key, liveEvidence(true), liveStop(true))
			tc.mutate(&rec)
			rec.NextTest = "run the authorised live suite; authority: maintainer live-test grant"
			storeRecord(t, reg, rec)
			elig := resolve(t, reg, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost)
			requireVerdict(t, elig, admission.Blocked, "entitlement_not_proven")
		})
	}

	drifted := openRegistry(t, t.TempDir())
	storeRecord(t, drifted, liveRecord(key, liveEvidence(true), liveStop(true)))
	probe.SHA256 = strings.Repeat("f", 64)
	elig, _ := resolveDrift(t, drifted, probe, manifest, evidence)
	requireVerdict(t, elig, admission.Blocked, "qualification_drifted")
}

func TestDriftErrorUnwraps(t *testing.T) {
	reg := openRegistry(t, t.TempDir())
	probe, manifest, evidence := fixtureProbe(), fixtureManifest(), fixtureEvidence()
	key := observedKey(probe, manifest, evidence, admission.ProfileTrustedHost)
	storeRecord(t, reg, liveRecord(key, liveEvidence(true), liveStop(true)))
	probe.SHA256 = strings.Repeat("f", 64)
	_, drift := resolveDrift(t, reg, probe, manifest, evidence)
	if drift.Blocked == nil || drift.Blocked.Code != "qualification_drifted" {
		t.Fatalf("drift block = %+v, want code qualification_drifted", drift.Blocked)
	}
	if drift.KeyHash != qualify.KeyHash(key) || !strings.Contains(drift.Reason, "executable_digest") {
		t.Fatalf("drift = (%s, %q), want the matched record's hash and an executable_digest reason", drift.KeyHash, drift.Reason)
	}
	var err error = drift
	if blocked, ok := errors.AsType[*admission.BlockedError](err); !ok || blocked != drift.Blocked {
		t.Fatalf("errors.As BlockedError = (%v, %v), want the drift block", blocked, ok)
	}
	if again, ok := errors.AsType[*admission.DriftError](fmt.Errorf("wrapped: %w", err)); !ok || again != drift {
		t.Fatal("errors.As does not find the DriftError through a wrap")
	}
}

func TestDeclaredIgnoresGaps(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("claudecode probe refuses Windows; the Decide legs cannot run there")
	}
	if len(claudecode.GapSources(claudecode.UnresolvedSources)) == 0 {
		t.Skip("the source gap is resolved; there is no gap to ignore")
	}
	world := newDecideWorld(t)
	dir := strictLiveRegistry(t, world)
	decided, err := admission.Decide(t.Context(), world.request(dir))
	if err != nil {
		t.Fatalf("declared Decide with a gap-bearing config and a present registry: %v", err)
	}
	posture := decided.Proposal.Billing
	if posture.Mode != admission.BillingSubscriptionDeclared || posture.Qualified || posture.G05 != "not-passed" || posture.PaidContinuation != "unknown" {
		t.Fatalf("declared posture = %+v, want the unchanged unverified labels", posture)
	}
}

func TestStrictDeclarationOnlyEvidenceBlocks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("claudecode probe refuses Windows; the Decide legs cannot run there")
	}
	// Sign-in-only evidence: no registry at all. A missing registry skips the
	// gap check, so the pin holds before and after the source gap resolves.
	world := newDecideWorld(t)
	_, err := admission.Decide(t.Context(), strictRequest(world, missingStateDir(t)))
	blocked(t, err, "no_qualification_record")

	// Declaration-only evidence: a present registry whose record was never
	// live-tested stays entitlement_not_proven at the consult.
	probe, manifest, evidence := fixtureProbe(), fixtureManifest(), fixtureEvidence()
	reg := openRegistry(t, t.TempDir())
	rec := liveRecord(observedKey(probe, manifest, evidence, admission.ProfileTrustedHost), liveEvidence(true), liveStop(true))
	rec.Progress = qualify.ProgressFixtureTested
	rec.Entitlement.Evidence[0].Method = "offline-fixture"
	rec.NextTest = "run the authorised live suite; authority: maintainer live-test grant"
	storeRecord(t, reg, rec)
	requireVerdict(t, resolve(t, reg, probe, manifest, evidence, admission.BillingSubscriptionOnly, admission.ProfileTrustedHost), admission.Blocked, "entitlement_not_proven")
}

func TestEarlyReturnGone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("claudecode probe refuses Windows; the Decide legs cannot run there")
	}
	world := newDecideWorld(t)
	_, err := admission.Decide(t.Context(), strictRequest(world, missingStateDir(t)))
	b, ok := errors.AsType[*admission.BlockedError](err)
	if !ok || b.Code != "no_qualification_record" || b.Field == "--billing subscription-only" {
		t.Fatalf("strict Decide with a missing registry = %v, want the consult's no_qualification_record", err)
	}
}

func TestRegistryUnreadableStillFailsClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("claudecode probe refuses Windows; the Decide legs cannot run there")
	}
	world := newDecideWorld(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, journal.DBName), []byte("this is not a database file"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := admission.Decide(t.Context(), strictRequest(world, dir))
	if err == nil {
		t.Fatal("strict Decide over an unreadable registry admitted")
	}
	// The trust journal and the consult open the same database, so the
	// failure surfaces from whichever reads it first; either way it is an
	// admission error, never the gap gate's block.
	if _, isBlocked := errors.AsType[*admission.BlockedError](err); isBlocked {
		t.Fatalf("unreadable registry mapped to a block (%v), want an admission error that outranks the gap gate", err)
	}
}
