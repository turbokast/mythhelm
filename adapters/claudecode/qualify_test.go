package claudecode

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/qualify"
)

// at03Routes is the AT-03 route list the fixture inventory must cover, in
// inventory order.
var at03Routes = []string{"implementer", "planner", "reviewer", "summary", "router", "child", "experiment"}

// wantEvidenceSuitePrefix pins the Suite format of the entitlement evidence
// independently of the production constant: the note's native fact plus this
// prefix must equal every evidence Suite.
const wantEvidenceSuitePrefix = "claudecode-at03-fixtures-"

// routeInventoryEntry mirrors one entry of
// testdata/qualification/routes.json. It deliberately duplicates the
// production shape instead of reusing it, so the tests pin the fixture.
type routeInventoryEntry struct {
	Route              string   `json:"route"`
	PassingCondition   string   `json:"passing_condition"`
	AuxiliaryInventory []string `json:"auxiliary_inventory"`
	PaidAuxiliary      []string `json:"paid_auxiliary"`
}

// routeInventory mirrors the AT-03 fixture inventory file.
type routeInventory struct {
	SchemaVersion int                   `json:"schema_version"`
	Surface       string                `json:"surface"`
	NativeVersion string                `json:"native_version"`
	Routes        []routeInventoryEntry `json:"routes"`
}

// loadRouteInventory reads the AT-03 fixture inventory from the testdata tree.
func loadRouteInventory(t *testing.T) routeInventory {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "qualification", "routes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var inv routeInventory
	if err := json.Unmarshal(raw, &inv); err != nil {
		t.Fatalf("parsing AT-03 route inventory: %v", err)
	}
	return inv
}

// inventoryRouteNames returns the inventory route names in order.
func inventoryRouteNames(inv routeInventory) []string {
	names := make([]string, 0, len(inv.Routes))
	for _, r := range inv.Routes {
		names = append(names, r.Route)
	}
	return names
}

// evidenceRoutes maps each evidence entry back to its AT-03 route via the
// ev_at03_<route> id convention, failing on any entry that breaks it.
func evidenceRoutes(ev []qualify.Evidence) ([]string, error) {
	const prefix = "ev_at03_"
	out := make([]string, 0, len(ev))
	for _, e := range ev {
		route, ok := strings.CutPrefix(e.ID, prefix)
		if !ok || route == "" {
			return nil, fmt.Errorf("evidence %q breaks the ev_at03_<route> id convention", e.ID)
		}
		out = append(out, route)
	}
	return out, nil
}

// matchEvidenceToRoutes reports whether the evidence covers exactly the
// inventory routes, in order.
func matchEvidenceToRoutes(inv routeInventory, ev []qualify.Evidence) error {
	covered, err := evidenceRoutes(ev)
	if err != nil {
		return err
	}
	want := inventoryRouteNames(inv)
	if !slices.Equal(covered, want) {
		return fmt.Errorf("evidence covers %q, inventory lists %q", covered, want)
	}
	return nil
}

// checkNoPaidAuxiliary reports any route whose inventory declares a paid
// auxiliary.
func checkNoPaidAuxiliary(inv routeInventory) error {
	for _, r := range inv.Routes {
		if len(r.PaidAuxiliary) != 0 {
			return fmt.Errorf("route %q declares paid auxiliaries %q", r.Route, r.PaidAuxiliary)
		}
	}
	return nil
}

// firstRouteFixtureKey builds the first-route key per the design §4 key table
// from fixed synthetic probe values. The identity tracks the adapter
// descriptor, so a descriptor rename fails the refusal tests loudly.
func firstRouteFixtureKey() qualify.Key {
	d := New().Descriptor()
	return qualify.Key{
		Harness:          d.Harness,
		Surface:          d.Surface,
		ExecutableDigest: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		AdapterProtocol:  d.ID + "+stream-json",
		OS:               runtime.GOOS,
		Arch:             runtime.GOARCH,
		ProviderEndpoint: "first-party-subscription",
		ModelSnapshot:    "unknown",
		EffortSettings:   "none",
		AuthCategory:     "claude.ai/max",
		ConfigDigest:     qualify.ConfigDigestOf(map[string]string{"fixture": "manifest"}),
		TrustProfile:     "trusted-host",
		WorkspaceClass:   "local-checkout",
		EntitlementClass: "included-plan",
	}
}

// evidencePayload marshals the constructor outputs for byte comparison.
func evidencePayload(t *testing.T, key qualify.Key) []byte {
	t.Helper()
	draft, err := RecordDraft(key)
	if err != nil {
		t.Fatalf("RecordDraft of the fixture key: %v", err)
	}
	raw, err := json.Marshal(struct {
		Evidence []qualify.Evidence `json:"evidence"`
		Draft    qualify.Record     `json:"draft"`
	}{Evidence: EntitlementEvidence(), Draft: draft})
	if err != nil {
		t.Fatalf("marshalling evidence payload: %v", err)
	}
	return raw
}

// compatHead returns the first 10 lines of the adapter compatibility note.
func compatHead(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("COMPATIBILITY.md")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	if len(lines) > 10 {
		lines = lines[:10]
	}
	return strings.Join(lines, "\n")
}

// checkCompatNote reports a compatibility head that does not anchor the
// registry note: the registry named as the queryable record, this file as
// the per-harness view.
func checkCompatNote(head string) error {
	for _, token := range []string{"qualification registry", "queryable record", "per-harness view"} {
		if !strings.Contains(head, token) {
			return fmt.Errorf("compatibility head names no %q", token)
		}
	}
	return nil
}

var (
	noteSurfaceRe  = regexp.MustCompile(`surface="([^"]+)"`)
	noteNativeRe   = regexp.MustCompile(`native="([^"]+)"`)
	noteProgressRe = regexp.MustCompile(`progress="([a-z-]+)"`)
)

// parseNoteFacts extracts the fixture facts the compatibility note states.
func parseNoteFacts(head string) (surface, native, progress string, err error) {
	for _, fact := range []struct {
		name string
		re   *regexp.Regexp
		into *string
	}{
		{"surface", noteSurfaceRe, &surface},
		{"native", noteNativeRe, &native},
		{"progress", noteProgressRe, &progress},
	} {
		m := fact.re.FindStringSubmatch(head)
		if len(m) != 2 {
			return "", "", "", fmt.Errorf("compatibility note states no %s fact", fact.name)
		}
		*fact.into = m[1]
	}
	return surface, native, progress, nil
}

// checkFactsMatchDraft reports a fixture fact that differs from the
// corresponding RecordDraft field.
func checkFactsMatchDraft(surface, native, progress string, draft qualify.Record) error {
	if surface != draft.Key.Surface {
		return fmt.Errorf("note surface %q differs from draft surface %q", surface, draft.Key.Surface)
	}
	if progress != string(draft.Progress) {
		return fmt.Errorf("note progress %q differs from draft progress %q", progress, draft.Progress)
	}
	for _, e := range draft.Entitlement.Evidence {
		if want := wantEvidenceSuitePrefix + native; e.Suite != want {
			return fmt.Errorf("note native %q expects suite %q, evidence carries %q", native, want, e.Suite)
		}
	}
	if len(draft.Entitlement.Evidence) == 0 {
		return errors.New("draft carries no entitlement evidence to match the native fact")
	}
	return nil
}

func TestEvidenceCoversAT03Routes(t *testing.T) {
	inv := loadRouteInventory(t)
	if !slices.Equal(inventoryRouteNames(inv), at03Routes) {
		t.Fatalf("inventory routes %q, want the AT-03 list %q", inventoryRouteNames(inv), at03Routes)
	}
	ev := EntitlementEvidence()
	if err := matchEvidenceToRoutes(inv, ev); err != nil {
		t.Fatal(err)
	}
	for i, r := range inv.Routes {
		if r.PassingCondition == "" {
			t.Fatalf("route %q names no passing condition", r.Route)
		}
		if !strings.Contains(ev[i].Uncertainty, r.PassingCondition) {
			t.Fatalf("evidence for %q does not rest on its passing condition: %q", r.Route, ev[i].Uncertainty)
		}
	}
	// Red variant: dropping one route from the inventory breaks the match.
	dropped := inv
	dropped.Routes = slices.Clone(inv.Routes[:len(inv.Routes)-1])
	if err := matchEvidenceToRoutes(dropped, ev); err == nil {
		t.Fatal("matchEvidenceToRoutes accepted an inventory missing a route")
	}
}

func TestNoHiddenPaidAuxiliary(t *testing.T) {
	inv := loadRouteInventory(t)
	if len(inv.Routes) == 0 {
		t.Fatal("inventory lists no routes")
	}
	if err := checkNoPaidAuxiliary(inv); err != nil {
		t.Fatal(err)
	}
	// Red variant: a fixture with a paid auxiliary fails the check.
	tainted := inv
	tainted.Routes = slices.Clone(inv.Routes)
	tainted.Routes[0].PaidAuxiliary = []string{"paid-search-aux"}
	if err := checkNoPaidAuxiliary(tainted); err == nil {
		t.Fatal("checkNoPaidAuxiliary accepted a route with a paid auxiliary")
	}
}

func TestRecordDraftRefusesOtherKeys(t *testing.T) {
	base := firstRouteFixtureKey()
	if _, err := RecordDraft(base); err != nil {
		t.Fatalf("RecordDraft refused the first-route fixture key: %v", err)
	}
	refusals := []struct {
		name   string
		mutate func(*qualify.Key)
	}{
		{"other harness", func(k *qualify.Key) { k.Harness = "codex" }},
		{"other surface", func(k *qualify.Key) { k.Surface = "native-cli-structured (print, text)" }},
		{"empty identity", func(k *qualify.Key) { k.Harness, k.Surface = "", "" }},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			key := base
			tc.mutate(&key)
			rec, err := RecordDraft(key)
			if !errors.Is(err, ErrNotFirstRoute) {
				t.Fatalf("RecordDraft error %v, want ErrNotFirstRoute", err)
			}
			if !reflect.DeepEqual(rec, qualify.Record{}) {
				t.Fatalf("RecordDraft returned a record for a refused key: %+v", rec)
			}
		})
	}
	// The same harness and surface with different digests still passes: the
	// refusal predicate ignores digests.
	drifted := base
	drifted.ExecutableDigest = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	drifted.ConfigDigest = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
	if _, err := RecordDraft(drifted); err != nil {
		t.Fatalf("RecordDraft refused the first-route identity with different digests: %v", err)
	}
}

func TestEvidencePerformsNoLiveCall(t *testing.T) {
	// Leg 1 (static): the adapter's own code reaches no socket API, so a
	// direct connection from the evidence path is impossible. The acceptance
	// prescribes transitive `go list -deps` here, but the qualify import the
	// task's Produces requires links net transitively through the journal's
	// file-URI building and the SQLite driver (see the task's Spec
	// deviations), so the leg asserts the package's direct imports plus the
	// transitive absence of any HTTP client instead.
	out, err := exec.CommandContext(t.Context(), "go", "list", "-f", "{{.Imports}}", ".").Output()
	if err != nil {
		t.Fatalf("go list -f {{.Imports}} .: %v", err)
	}
	for imp := range strings.FieldsSeq(strings.Trim(string(out), "[]\n")) {
		if imp == "net" || imp == "net/http" {
			t.Fatalf("production package directly imports socket API %q", imp)
		}
	}
	deps, err := exec.CommandContext(t.Context(), "go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list -deps .: %v", err)
	}
	for dep := range strings.SplitSeq(strings.TrimSpace(string(deps)), "\n") {
		if dep == "net/http" {
			t.Fatalf("production package transitively reaches %q", dep)
		}
	}
	// Leg 2: poisoned proxies and an empty HOME leave the output
	// byte-identical.
	key := firstRouteFixtureKey()
	before := evidencePayload(t, key)
	for _, v := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(v, "http://127.0.0.1:9")
	}
	t.Setenv("HOME", t.TempDir())
	if after := evidencePayload(t, key); !bytes.Equal(before, after) {
		t.Fatal("evidence output changed under poisoned proxies and an empty HOME")
	}
}

func TestDraftRecordsThroughRegistry(t *testing.T) {
	ctx := t.Context()
	reg, err := qualify.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reg.Close() }()
	draft, err := RecordDraft(firstRouteFixtureKey())
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Record(ctx, draft); err != nil {
		t.Fatalf("recording the draft: %v", err)
	}
	got, err := reg.Lookup(ctx, draft.Key)
	if err != nil {
		t.Fatalf("looking up the recorded draft: %v", err)
	}
	if got.Revision != 1 {
		t.Fatalf("recorded revision %d, want 1", got.Revision)
	}
	recomputed, err := qualify.CanonicalDigest(got)
	if err != nil {
		t.Fatal(err)
	}
	if got.Digest != recomputed {
		t.Fatalf("stored digest %q does not recompute (content hashes to %q)", got.Digest, recomputed)
	}
	if got.Progress != qualify.ProgressFixtureTested {
		t.Fatalf("recorded progress %q, want fixture-tested", got.Progress)
	}
	if got.Entitlement.Verdict != qualify.Proven || len(got.Entitlement.Evidence) != len(at03Routes) {
		t.Fatalf("recorded entitlement %+v, want proven with %d evidence entries",
			got.Entitlement, len(at03Routes))
	}
}

func TestDraftQuotaLabelledUnknown(t *testing.T) {
	draft, err := RecordDraft(firstRouteFixtureKey())
	if err != nil {
		t.Fatal(err)
	}
	q := draft.Quota
	if q.Quantity != "unknown" || q.Unit != "unknown" || q.Scope != "unknown" ||
		q.Source != "unknown" || q.Label != qualify.DatumUnknown {
		t.Fatalf("draft quota %+v, want the unknown datum", q)
	}
	for _, col := range []struct {
		name string
		col  qualify.Column
	}{
		{"fidelity", draft.Fidelity},
		{"entitlement", draft.Entitlement},
		{"lifecycle", draft.Lifecycle},
	} {
		for _, e := range col.col.Evidence {
			if e.Source == "" {
				t.Errorf("%s evidence %q carries an empty source", col.name, e.ID)
			}
		}
	}
}

func TestCompatibilityNoteAnchored(t *testing.T) {
	head := compatHead(t)
	if err := checkCompatNote(head); err != nil {
		t.Fatal(err)
	}
	// Red variant: the file without the note fails the check.
	raw, err := os.ReadFile("COMPATIBILITY.md")
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		if !strings.HasPrefix(line, ">") {
			kept = append(kept, line)
		}
	}
	if len(kept) > 10 {
		kept = kept[:10]
	}
	if err := checkCompatNote(strings.Join(kept, "\n")); err == nil {
		t.Fatal("checkCompatNote accepted the file without the registry note")
	}
}

func TestCompatibilityFactsMatchDraft(t *testing.T) {
	head := compatHead(t)
	surface, native, progress, err := parseNoteFacts(head)
	if err != nil {
		t.Fatal(err)
	}
	if surface != New().Descriptor().Surface {
		t.Fatalf("note surface %q differs from the adapter surface %q", surface, New().Descriptor().Surface)
	}
	key := firstRouteFixtureKey()
	key.Surface = surface
	draft, err := RecordDraft(key)
	if err != nil {
		t.Fatalf("RecordDraft of the note surface: %v", err)
	}
	if err := checkFactsMatchDraft(surface, native, progress, draft); err != nil {
		t.Fatal(err)
	}
	// Red variants: each drifted fact fails the match.
	drifts := []struct {
		name     string
		surface  string
		native   string
		progress string
	}{
		{"surface", surface + " (drifted)", native, progress},
		{"native", surface, "9.9.999", progress},
		{"progress", surface, native, "live-qualified"},
	}
	for _, d := range drifts {
		if err := checkFactsMatchDraft(d.surface, d.native, d.progress, draft); err == nil {
			t.Errorf("checkFactsMatchDraft accepted a drifted %s fact", d.name)
		}
	}
}
