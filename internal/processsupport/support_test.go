package processsupport

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/adapter"
)

const header = "| arch | surface | process_tree_ownership | worker_detachment | crash behaviour | tests | CI job |\n|---|---|---|---|---|---|---|\n"

// Synthetic qualification evidence: never inserted into the shipped matrix.
const fixture = "| amd64 | scripted-child-process | supported | unknown | kill_on_close | `TestStopKillsChildAndGrandchild` | windows-latest https://github.com/turbokast/mythhelm/actions/runs/123 |\n"

func TestSupportMatrixMatchesEvidence(t *testing.T) {
	t.Parallel()
	tests := windowsTests(t)
	for _, tc := range []struct {
		name, table string
		valid       bool
	}{
		{"shipped empty", string(table), true},
		{"existing Windows test", header + fixture, true},
		{"missing test mutation", header + strings.ReplaceAll(fixture, "TestStopKillsChildAndGrandchild", "TestMissingWindowsEvidence"), false},
		{"ordinary source function mutation", header + strings.ReplaceAll(fixture, "TestStopKillsChildAndGrandchild", "StopKillsChildAndGrandchild"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := Parse([]byte(tc.table))
			valid := err == nil && evidenceExists(m, tests)
			if valid != tc.valid {
				t.Fatalf("evidence valid=%v err=%v, want %v", valid, err, tc.valid)
			}
			if tc.valid && tc.name == "existing Windows test" && m.Windows("amd64", "scripted-child-process") != (Support{adapter.Supported, adapter.Unknown}) {
				t.Fatal("valid evidence did not establish exact capability values")
			}
		})
	}
	if len(table) == 0 {
		t.Fatal("embedded matrix missing")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		if got := Windows(arch, "scripted-child-process"); got != (Support{adapter.Unknown, adapter.Unknown}) {
			t.Fatalf("empty shipped matrix promotes %s: %+v", arch, got)
		}
	}
}

func TestUntestedPairIsUnknown(t *testing.T) {
	t.Parallel()
	m, err := Parse([]byte(header + fixture))
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"amd64", "scripted-child-process"}, {"arm64", "scripted-child-process"}, {"amd64", "native-cli-structured (print, stream-json)"}, {"AMD64", "scripted-child-process"}, {"amd64", "scripted-child-process "}, {"", ""}} {
		want := Support{adapter.Unknown, adapter.Unknown}
		if pair == [2]string{"amd64", "scripted-child-process"} {
			want.ProcessTreeOwnership = adapter.Supported
		}
		if got := m.Windows(pair[0], pair[1]); got != want {
			t.Errorf("Windows(%q,%q)=%+v, want %+v", pair[0], pair[1], got, want)
		} // I09, I14 (v2 §2, §17.1).
	}
}

func TestMalformedTableIsUnknown(t *testing.T) {
	t.Parallel()
	for name, src := range map[string]string{
		"no header": fixture, "empty": "", "missing separator": strings.ReplaceAll(header, "|---|---|---|---|---|---|---|\n", "") + fixture,
		"wrong columns":  header + "| amd64 | scripted-child-process | supported |\n",
		"duplicate pair": header + fixture + fixture, "invalid capability": header + strings.ReplaceAll(fixture, "supported", "yes"),
		"invalid crash behaviour": header + strings.ReplaceAll(fixture, "kill_on_close", "unknown"),
		"missing evidence":        header + strings.ReplaceAll(fixture, "`TestStopKillsChildAndGrandchild`", ""),
		"invalid architecture":    header + strings.ReplaceAll(fixture, "amd64", "x86"),
		"empty surface":           header + strings.ReplaceAll(fixture, "scripted-child-process", ""),
		"valid then invalid":      header + fixture + "| broken |\n",
		"oversize":                header + strings.Repeat("x", 65537),
	} {
		t.Run(name, func(t *testing.T) {
			m, err := Parse([]byte(src))
			if err == nil {
				t.Fatal("malformed table accepted")
			}
			for _, arch := range []string{"amd64", "arm64"} {
				if got := m.Windows(arch, "scripted-child-process"); got != (Support{adapter.Unknown, adapter.Unknown}) {
					t.Fatalf("partially parsed support survived error: %+v", got)
				}
			}
		})
	}
}

func TestRowRequiresWindowsJobAndRunID(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, job string
		valid     bool
	}{
		{"amd64 evidence", "windows-latest https://github.com/turbokast/mythhelm/actions/runs/123", true},
		{"arm job wrong architecture", "windows-11-arm https://github.com/turbokast/mythhelm/actions/runs/123", false},
		{"no job", "https://github.com/turbokast/mythhelm/actions/runs/123", false},
		{"unix job", "ubuntu-latest https://github.com/turbokast/mythhelm/actions/runs/123", false},
		{"lookalike job", "not-windows-latest https://github.com/turbokast/mythhelm/actions/runs/123", false},
		{"no run", "windows-latest", false}, {"zero run", "windows-latest https://github.com/turbokast/mythhelm/actions/runs/0", false},
		{"non numeric run", "windows-latest https://github.com/turbokast/mythhelm/actions/runs/abc", false},
		{"foreign repo", "windows-latest https://github.com/example/other/actions/runs/123", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := strings.ReplaceAll(fixture, "windows-latest https://github.com/turbokast/mythhelm/actions/runs/123", tc.job)
			_, err := Parse([]byte(header + src))
			if (err == nil) != tc.valid {
				t.Fatalf("Parse valid=%v err=%v, want %v", err == nil, err, tc.valid)
			}
		})
	}
	m, err := Parse([]byte(header + strings.ReplaceAll(strings.ReplaceAll(fixture, "amd64", "arm64"), "windows-latest", "windows-11-arm")))
	if err != nil || m.Windows("arm64", "scripted-child-process").ProcessTreeOwnership != adapter.Supported {
		t.Fatalf("valid arm64 evidence rejected: %v", err)
	}
}

func TestNoLeafCycle(t *testing.T) {
	t.Parallel()
	cmd := exec.CommandContext(t.Context(), "go", "list", "-deps", ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	deps := map[string]bool{}
	for line := range strings.SplitSeq(string(out), "\n") {
		deps[line] = true
	}
	const prefix = "github.com/turbokast/mythhelm/"
	if !deps[prefix+"internal/adapter"] {
		t.Fatal("leaf lost adapter contract dependency")
	}
	for _, forbidden := range []string{"adapters/fake", "adapters/claudecode", "internal/workers"} {
		if deps[prefix+forbidden] {
			t.Errorf("leaf depends on %s", forbidden)
		}
	}
}

func windowsTests(t *testing.T) map[string]bool {
	t.Helper()
	var files []string
	for _, dir := range []string{filepath.Join("..", "workers"), filepath.Join("..", "supervisor"), filepath.Join("..", "..", "tests", "e2e"), filepath.Join("..", "..", "adapters", "fake")} {
		matches, err := filepath.Glob(filepath.Join(dir, "*_windows_test.go"))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, matches...)
	}
	if len(files) == 0 {
		t.Fatal("no Windows evidence source")
	}
	tests := map[string]bool{}
	for _, path := range files {
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || fn.Name.Name == "TestMain" || fn.Type.Results != nil || fn.Type.Params.NumFields() != 1 {
				continue
			}
			p := fn.Type.Params.List[0]
			star, ok := p.Type.(*ast.StarExpr)
			if !ok {
				continue
			}
			sel, ok := star.X.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			pkg, ok := sel.X.(*ast.Ident)
			if ok && pkg.Name == "testing" && sel.Sel.Name == "T" {
				tests[fn.Name.Name] = true
			}
		}
	}
	return tests
}

func evidenceExists(m Matrix, tests map[string]bool) bool {
	for _, r := range m.rows {
		for _, name := range r.tests {
			if !tests[name] {
				return false
			}
		}
	}
	return true
}

func TestMalformedEmbeddedTableIsUnknown(t *testing.T) {
	// Serial: the test temporarily replaces the embedded bytes and restores them.
	original := table
	t.Cleanup(func() { table = original })
	table = []byte(header + fixture + "| broken |\n")
	if got := Windows("amd64", "scripted-child-process"); got != (Support{adapter.Unknown, adapter.Unknown}) {
		t.Fatalf("malformed embedded matrix promoted partial evidence: %+v", got)
	}
	table = []byte(header + fixture)
	if got := Windows("amd64", "scripted-child-process"); got.ProcessTreeOwnership != adapter.Supported {
		t.Fatalf("valid embedded counterfactual did not promote fixture: %+v", got)
	}
}

func TestCapabilityFieldsRemainDistinct(t *testing.T) {
	t.Parallel()
	src := strings.ReplaceAll(fixture, "supported | unknown", "unknown | supported")
	m, err := Parse([]byte(header + src))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Windows("amd64", "scripted-child-process"); got != (Support{adapter.Unknown, adapter.Supported}) {
		t.Fatalf("distinct fields collapsed: %+v", got)
	}
	src = strings.ReplaceAll(src, "unknown | supported", "unsupported | unsupported")
	m, err = Parse([]byte(header + src))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Windows("amd64", "scripted-child-process"); got != (Support{adapter.Unsupported, adapter.Unsupported}) {
		t.Fatalf("unsupported evidence lost: %+v", got)
	}
}
