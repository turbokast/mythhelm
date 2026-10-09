//nolint:misspell // Artifact is the spec-mandated type name and wire key
package v2contract_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

var matrixStates = []string{"documented", "fixture-tested", "blocked", "live-qualified"}

// supportRow is one parsed row of the SUPPORT.md table.
type supportRow struct{ name, state, evidence string }

func parseSupport(md string) []supportRow {
	var rows []supportRow
	for line := range strings.SplitSeq(md, "\n") {
		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		if !strings.HasPrefix(strings.TrimSpace(line), "|") || len(cells) != 3 {
			continue
		}
		r := supportRow{strings.TrimSpace(cells[0]), strings.TrimSpace(cells[1]), strings.TrimSpace(cells[2])}
		if r.name == "Deliverable" || strings.Trim(r.name, "-") == "" {
			continue
		}
		rows = append(rows, r)
	}
	return rows
}

// supportProblems lists every way md fails to match the shipped deliverables
// and the tests that exist.
func supportProblems(md string, deliverables []string, tests map[string]bool) []string {
	var problems []string
	rows := parseSupport(md)
	byName := map[string]supportRow{}
	for _, r := range rows {
		if _, dup := byName[r.name]; dup {
			problems = append(problems, "duplicate row "+r.name)
		}
		byName[r.name] = r
		if !slices.Contains(matrixStates, r.state) {
			problems = append(problems, "row "+r.name+" has unknown state "+r.state)
		}
		if r.state == "live-qualified" {
			problems = append(problems, "row "+r.name+" claims live qualification")
		}
	}
	for _, d := range deliverables {
		r, ok := byName[d]
		switch {
		case !ok:
			problems = append(problems, "no row for deliverable "+d)
		case r.state != "fixture-tested":
			problems = append(problems, "deliverable "+d+" is "+r.state+", want fixture-tested")
		default:
			for name := range strings.SplitSeq(r.evidence, ",") {
				if name = strings.TrimSpace(name); !tests[name] {
					problems = append(problems, "row "+d+" names missing test "+name)
				}
			}
		}
	}
	for _, r := range rows {
		if !slices.Contains(deliverables, r.name) && r.state != "blocked" {
			problems = append(problems, "row "+r.name+" is neither a deliverable nor blocked")
		}
	}
	return problems
}

// packageTests returns the names of the Test functions in this package.
func packageTests(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob("*_test.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob test files: %v (%d)", err, len(files))
	}
	re := regexp.MustCompile(`(?m)^func (Test\w+)\(`)
	tests := map[string]bool{}
	for _, f := range files {
		b, err := os.ReadFile(f) //nolint:gosec // G304: f comes from globbing this package's directory
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			tests[m[1]] = true
		}
	}
	return tests
}

// shippedDeliverables is one name per record golden plus the fixed non-record
// deliverables, so a new record golden without a matrix row fails.
func shippedDeliverables(t *testing.T) []string {
	t.Helper()
	goldens, err := filepath.Glob("testdata/records/*.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(goldens) != 14 {
		t.Fatalf("record goldens = %d, want the 14 v2 §4.1 records", len(goldens))
	}
	var out []string
	for _, g := range goldens {
		out = append(out, strings.TrimSuffix(filepath.Base(g), ".golden.json"))
	}
	return append(out, "run_lifecycle", "task_lifecycle", "attempt_lifecycle", "error_catalogue", "event_envelope", "frame_limits")
}

// I14 (v2 §2): honest support states; AC-9.2.
func TestSupportMatrixMatchesEvidence(t *testing.T) {
	t.Parallel()
	md, err := os.ReadFile("SUPPORT.md")
	if err != nil {
		t.Fatal(err)
	}
	deliverables := shippedDeliverables(t)
	tests := packageTests(t)
	if problems := supportProblems(string(md), deliverables, tests); len(problems) != 0 {
		t.Errorf("SUPPORT.md problems: %v", problems)
	}

	// The checker must reject the broken matrices it exists to catch.
	good := "| a | fixture-tested | TestX |\n"
	broken := map[string]string{
		"missing row":         "| other | blocked | why |\n",
		"live-qualified":      "| a | live-qualified | TestX |\n",
		"missing test":        "| a | fixture-tested | TestGone |\n",
		"deliverable blocked": "| a | blocked | why |\n",
		"unlisted claim":      good + "| extra | fixture-tested | TestX |\n",
	}
	for name, md := range broken {
		if got := supportProblems(md, []string{"a"}, map[string]bool{"TestX": true}); len(got) == 0 {
			t.Errorf("broken matrix %q passed the checker", name)
		}
	}
	if got := supportProblems(good, []string{"a"}, map[string]bool{"TestX": true}); len(got) != 0 {
		t.Errorf("good matrix rejected: %v", got)
	}
}

// forbiddenDeps returns the dependency lines that give the contract a network
// or database dependency.
func forbiddenDeps(listOutput string) []string {
	var bad []string
	for dep := range strings.FieldsSeq(listOutput) {
		if dep == "net" || strings.HasPrefix(dep, "modernc.org/sqlite") {
			bad = append(bad, dep)
		}
	}
	return bad
}

// I13 (v2 §2) and NFR-4: the contract opens no network service and links no
// database driver.
func TestContractHasNoNetworkDependency(t *testing.T) {
	t.Parallel()
	if got := forbiddenDeps("fmt\nnet\nmodernc.org/sqlite/lib\n"); len(got) != 2 {
		t.Fatalf("forbiddenDeps missed a net or sqlite dependency: %v", got)
	}
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	if !bytes.Contains(out, []byte("encoding/json")) {
		t.Fatalf("go list -deps output looks empty: %q", out)
	}
	if bad := forbiddenDeps(string(out)); len(bad) != 0 {
		t.Errorf("contract depends on %v", bad)
	}
}

// I14 (v2 §2): an unknown key in an authority contract is an error naming it.
func TestUnknownAuthorityKeyRejected(t *testing.T) {
	t.Parallel()
	_, err := v2contract.Decode[v2contract.Grant](loadGolden(t, "invalid/unknown_authority_key.json"))
	if err == nil || !strings.Contains(err.Error(), "root_override") {
		t.Errorf("Decode(unknown authority key) error = %v, want naming root_override", err)
	}
	if _, err := v2contract.Decode[v2contract.Grant](loadGolden(t, "records/grant.golden.json")); err != nil {
		t.Errorf("control: valid grant rejected: %v", err)
	}
}

// I09 (v2 §2): a null measurement is rejected, never read as 0 or "".
func TestNullMeasurementsRejected(t *testing.T) {
	t.Parallel()
	var f struct {
		Reservation json.RawMessage `json:"reservation"`
		Artifact    json.RawMessage `json:"artifact"`
	}
	if err := json.Unmarshal(loadGolden(t, "invalid/null_measurement.json"), &f); err != nil {
		t.Fatal(err)
	}
	if _, err := v2contract.Decode[v2contract.Reservation](f.Reservation); err == nil || !strings.Contains(err.Error(), "quantity") {
		t.Errorf("Decode(null quantity) error = %v, want naming quantity", err)
	}
	if _, err := v2contract.Decode[v2contract.Artifact](f.Artifact); err == nil || !strings.Contains(err.Error(), "size_bytes") {
		t.Errorf("Decode(null size_bytes) error = %v, want naming size_bytes", err)
	}

	// Controls: the honest unknown and a real zero are different from null.
	res, err := v2contract.Decode[v2contract.Reservation](bytes.Replace(f.Reservation, []byte(`"quantity":null`), []byte(`"quantity":"unknown"`), 1))
	if err != nil || res.Quantity != "unknown" {
		t.Errorf("Decode(quantity unknown) = %q, %v; want the string unknown", res.Quantity, err)
	}
	art, err := v2contract.Decode[v2contract.Artifact](bytes.Replace(f.Artifact, []byte(`"size_bytes":null`), []byte(`"size_bytes":0`), 1))
	if err != nil || art.SizeBytes == nil || *art.SizeBytes != 0 {
		t.Errorf("Decode(size_bytes 0) = %v, %v; want a present zero", art.SizeBytes, err)
	}
}

func keySet(md toml.MetaData) []string {
	var keys []string
	for _, k := range md.Keys() {
		keys = append(keys, k.String())
	}
	sort.Strings(keys)
	return keys
}

// AC-1.1, AC-9.1: the toml tags are the wire names, and unknown keys are
// rejected rather than dropped.
func TestTOMLTagsRoundTrip(t *testing.T) {
	t.Parallel()
	golden := loadGolden(t, "policy_version.golden.toml")
	var pv v2contract.PolicyVersion
	md, err := toml.Decode(string(golden), &pv)
	if err != nil {
		t.Fatalf("decode golden: %v", err)
	}
	if u := md.Undecoded(); len(u) != 0 {
		t.Errorf("golden keys not claimed by a toml tag: %v", u)
	}
	if err := pv.Validate(); err != nil {
		t.Errorf("golden fails Validate: %v", err)
	}

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(pv); err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	var again v2contract.PolicyVersion
	md2, err := toml.Decode(buf.String(), &again)
	if err != nil {
		t.Fatalf("decode re-encoded: %v", err)
	}
	if got, want := keySet(md2), keySet(md); !slices.Equal(got, want) {
		t.Errorf("re-encoded keys = %v, want %v", got, want)
	}

	var probe v2contract.PolicyVersion
	bad, err := toml.Decode("bogus_key = 1\n"+string(golden), &probe)
	if err != nil {
		t.Fatal(err)
	}
	if u := bad.Undecoded(); len(u) != 1 || u[0].String() != "bogus_key" {
		t.Errorf("unknown TOML key not detected: %v", u)
	}
}

var secretWords = regexp.MustCompile(`(?i)sk-|secret|token|apikey`)

// secretHits walks the string values (never the keys) of a decoded document.
func secretHits(v any) []string {
	switch x := v.(type) {
	case string:
		if secretWords.MatchString(x) {
			return []string{x}
		}
	case map[string]any:
		var hits []string
		for _, e := range x {
			hits = append(hits, secretHits(e)...)
		}
		return hits
	case []any:
		var hits []string
		for _, e := range x {
			hits = append(hits, secretHits(e)...)
		}
		return hits
	}
	return nil
}

// NFR-4: fixtures carry no credentials.
func TestFixturesCarryNoSecrets(t *testing.T) {
	t.Parallel()
	if len(secretHits(map[string]any{"token": "fine as a key", "v": []any{"sk-live"}})) != 1 { //nolint:gosec // G101: synthetic probe for the scanner
		t.Fatal("secretHits must flag a secret value and ignore a key named token")
	}
	var goldens []string
	for _, pat := range []string{"testdata/*.golden.json", "testdata/records/*.golden.json", "testdata/*.golden.toml"} {
		m, err := filepath.Glob(pat)
		if err != nil {
			t.Fatal(err)
		}
		goldens = append(goldens, m...)
	}
	if len(goldens) < 17 {
		t.Fatalf("found %d goldens, want at least 17", len(goldens))
	}
	for _, g := range goldens {
		b, err := os.ReadFile(g) //nolint:gosec // G304: g comes from globbing testdata
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if strings.HasSuffix(g, ".toml") {
			err = toml.Unmarshal(b, &doc)
		} else {
			err = json.Unmarshal(b, &doc)
		}
		if err != nil {
			t.Errorf("%s: %v", g, err)
			continue
		}
		if hits := secretHits(doc); len(hits) != 0 {
			t.Errorf("%s: string values look like credentials: %v", g, hits)
		}
	}
}
