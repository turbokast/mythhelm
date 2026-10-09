package migrate

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// matrixRow is one row of SUPPORT.md: a shipped (or blocked) deliverable,
// its status, the platforms its evidence runs on and the tests that are that
// evidence. Evidence names are backticked package-qualified test refs
// (`journal.TestX`); the blocked row names no test.
type matrixRow struct {
	ID, Deliverable, Status, Platforms, Evidence string
	Tests                                        []string // "pkg.TestName" refs
}

var matrixTestRef = regexp.MustCompile("`([a-z]+)\\.(Test[A-Za-z0-9_]+)`")

// parseSupportMatrix reads the matrix: every table row with five cells whose
// first cell is a backticked ID.
func parseSupportMatrix(t *testing.T, src string) []matrixRow {
	t.Helper()
	var rows []matrixRow
	for line := range strings.SplitSeq(src, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) != 5 {
			continue
		}
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		if !strings.HasPrefix(cells[0], "`") {
			continue // the header and the separator
		}
		r := matrixRow{ID: strings.Trim(cells[0], "`"), Deliverable: cells[1], Status: cells[2], Platforms: cells[3], Evidence: cells[4]}
		for _, m := range matrixTestRef.FindAllStringSubmatch(cells[4], -1) {
			r.Tests = append(r.Tests, m[1]+"."+m[2])
		}
		rows = append(rows, r)
	}
	return rows
}

func readSupportMatrix(t *testing.T) []matrixRow {
	t.Helper()
	raw, err := os.ReadFile("SUPPORT.md")
	if err != nil {
		t.Fatal(err)
	}
	rows := parseSupportMatrix(t, string(raw))
	if len(rows) == 0 {
		t.Fatal("SUPPORT.md has no matrix rows")
	}
	return rows
}

// matrixEvidenceDirs maps the evidence-ref package key to the directory
// holding its tests, relative to internal/migrate.
var matrixEvidenceDirs = map[string]string{
	"migrate": ".",
	"journal": "../journal",
	"cli":     "../cli",
	"tests":   "../../tests",
}

// evidenceTestPlatforms maps every "pkg.TestName" in the evidence packages to
// the platforms its file is built for: "all", "linux, darwin" or one GOOS.
func evidenceTestPlatforms(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for pkg, dir := range matrixEvidenceDirs {
		files, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range files {
			raw, err := os.ReadFile(filepath.Clean(name))
			if err != nil {
				t.Fatal(err)
			}
			platforms := "all"
			for line := range strings.SplitSeq(string(raw), "\n") {
				if line = strings.TrimSpace(line); strings.HasPrefix(line, "//go:build") {
					platforms = strings.TrimSpace(strings.TrimPrefix(line, "//go:build"))
					break
				}
				if strings.HasPrefix(line, "package ") {
					break
				}
			}
			// An implicit GOOS filename suffix constrains the file even
			// without a build tag; without this an untagged
			// foo_windows_test.go would be mislabelled "all".
			base := strings.TrimSuffix(filepath.Base(name), "_test.go")
			for _, goos := range []string{"linux", "darwin", "windows"} {
				if strings.HasSuffix(base, "_"+goos) {
					platforms = goos
					break
				}
			}
			switch platforms {
			case "all", "linux || darwin", "linux", "darwin", "windows":
				platforms = strings.Replace(platforms, " || ", ", ", 1)
			default:
				continue // a file for platforms outside the matrix
			}
			f, err := parser.ParseFile(token.NewFileSet(), name, raw, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range f.Decls {
				if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") && fn.Name.Name != "TestMain" {
					out[pkg+"."+fn.Name.Name] = platforms
				}
			}
		}
	}
	return out
}

// matrixProblems checks rows against the tests, returning one message per
// defect.
func matrixProblems(rows []matrixRow, tests map[string]string) []string {
	var problems []string
	ids := map[string]bool{}
	var got []string
	for _, r := range rows {
		if ids[r.ID] {
			problems = append(problems, "duplicate row "+r.ID)
		}
		ids[r.ID] = true
		got = append(got, r.ID)
		switch r.Status {
		case "fixture-tested":
			if len(r.Tests) == 0 {
				problems = append(problems, r.ID+": fixture-tested with no evidence test")
			}
			for _, ref := range r.Tests {
				platforms, ok := tests[ref]
				switch {
				case !ok:
					problems = append(problems, fmt.Sprintf("%s: evidence %s is not a test in the evidence packages", r.ID, ref))
				case platforms != r.Platforms:
					problems = append(problems, fmt.Sprintf("%s: claims %q but %s is built for %q", r.ID, r.Platforms, ref, platforms))
				}
			}
		case "blocked":
			if len(r.Tests) != 0 || !strings.HasPrefix(r.Evidence, "blocked:") {
				problems = append(problems, r.ID+": a blocked row names no tests and starts its evidence with \"blocked:\"")
			}
		default:
			problems = append(problems, fmt.Sprintf("%s: status %q is not fixture-tested or blocked (nothing here is live-qualified)", r.ID, r.Status))
		}
	}
	slices.Sort(got)
	if !slices.Equal(got, shippedMigrationDeliverables) {
		problems = append(problems, fmt.Sprintf("matrix rows %v differ from the shipped deliverables %v", got, shippedMigrationDeliverables))
	}
	return problems
}

// shippedMigrationDeliverables are the matrix row IDs, sorted. A new
// deliverable adds its row here and to SUPPORT.md together.
var shippedMigrationDeliverables = []string{
	"backup-restore", "drain", "import", "migrate-cli", "nfr2-proof", "v2-tables",
}

// migrationRowExports maps every exported declaration of internal/migrate and
// its preview subpackage to the matrix row covering it, so adding behaviour
// without a row fails the matrix check.
var migrationRowExports = map[string]string{
	"Apply":           "migrate-cli",
	"Backup":          "backup-restore",
	"BackupInfo":      "backup-restore",
	"Restore":         "backup-restore",
	"Drain":           "drain",
	"DrainReport":     "drain",
	"ImportOptions":   "import",
	"ImportResult":    "import",
	"ImportRun":       "import",
	"Phase":           "v2-tables",
	"PhaseNotStarted": "v2-tables",
	"PhasePreviewed":  "v2-tables",
	"PhaseDrained":    "v2-tables",
	"PhaseImported":   "v2-tables",
	"PhaseAdopted":    "v2-tables",
	"Owners":          "drain",
	"Plan":            "migrate-cli",
	"PreviewPlan":     "migrate-cli",
	"Run":             "migrate-cli",
}

// exportedDecls lists the exported top-level functions (these types carry no
// methods), types and named values declared by the non-test Go files in dir.
func exportedDecls(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, raw, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && ast.IsExported(d.Name.Name) {
					out = append(out, d.Name.Name)
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						if ast.IsExported(s.Name.Name) {
							out = append(out, s.Name.Name)
						}
					case *ast.ValueSpec:
						for _, n := range s.Names {
							if ast.IsExported(n.Name) {
								out = append(out, n.Name)
							}
						}
					}
				}
			}
		}
	}
	return out
}

// exportProblems checks the package's exported surface against the row
// mapping, returning one message per unmapped or dangling declaration.
func exportProblems(found []string, mapping map[string]string, rowIDs map[string]bool) []string {
	var problems []string
	seen := map[string]bool{}
	for _, name := range found {
		if seen[name] {
			continue
		}
		seen[name] = true
		row, ok := mapping[name]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("exported %s has no matrix row (add its row to SUPPORT.md and migrationRowExports together)", name))
		case !rowIDs[row]:
			problems = append(problems, fmt.Sprintf("exported %s maps to %q, which is not a matrix row", name, row))
		}
	}
	return problems
}

func TestSupportMatrixMatchesEvidence(t *testing.T) {
	t.Parallel()
	rows := readSupportMatrix(t)
	tests := evidenceTestPlatforms(t)
	if problems := matrixProblems(rows, tests); len(problems) > 0 {
		t.Fatalf("SUPPORT.md disagrees with the tests:\n  %s", strings.Join(problems, "\n  "))
	}
	rowIDs := map[string]bool{}
	for _, r := range rows {
		rowIDs[r.ID] = true
	}
	found := append(exportedDecls(t, "."), exportedDecls(t, "preview")...)
	if problems := exportProblems(found, migrationRowExports, rowIDs); len(problems) > 0 {
		t.Fatalf("package exports disagree with the matrix:\n  %s", strings.Join(problems, "\n  "))
	}

	// The check can fail: each defect below is reported.
	for name, mutate := range map[string]func() ([]matrixRow, []string, map[string]string, map[string]bool){
		"a live-qualified claim": func() ([]matrixRow, []string, map[string]string, map[string]bool) {
			r := slices.Clone(rows)
			r[0].Status = "live-qualified"
			return r, found, migrationRowExports, rowIDs
		},
		"a row naming a test that does not exist": func() ([]matrixRow, []string, map[string]string, map[string]bool) {
			r := slices.Clone(rows)
			r[0].Tests = append(slices.Clone(r[0].Tests), "migrate.TestNoSuchThing")
			return r, found, migrationRowExports, rowIDs
		},
		"a row for a deliverable that was removed": func() ([]matrixRow, []string, map[string]string, map[string]bool) {
			return rows[1:], found, migrationRowExports, rowIDs
		},
		"a row for a deliverable that never shipped": func() ([]matrixRow, []string, map[string]string, map[string]bool) {
			r := slices.Clone(rows)
			r = append(r, matrixRow{ID: "never-shipped", Status: "fixture-tested", Platforms: "all", Tests: rows[0].Tests})
			return r, found, migrationRowExports, rowIDs
		},
		"a new export without a row": func() ([]matrixRow, []string, map[string]string, map[string]bool) {
			return rows, append(slices.Clone(found), "NewExport"), migrationRowExports, rowIDs
		},
		"an export mapped to a row that does not exist": func() ([]matrixRow, []string, map[string]string, map[string]bool) {
			mapping := map[string]string{}
			maps.Copy(mapping, migrationRowExports)
			mapping[found[0]] = "no-such-row"
			return rows, found, mapping, rowIDs
		},
	} {
		mutated, exports, mapping, ids := mutate()
		if len(matrixProblems(mutated, tests)) == 0 && len(exportProblems(exports, mapping, ids)) == 0 {
			t.Errorf("%s passes the matrix check", name)
		}
	}
}

// migrationADR is the decision record this matrix ships with, relative to
// internal/migrate.
const migrationADR = "../../docs/decisions/0016-migration-import.md"

func TestMigrationADRNamesDecisions(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Clean(migrationADR))
	if err != nil {
		t.Fatalf("migration ADR missing: %v", err)
	}
	src := string(raw)
	for _, want := range []string{"OQ-1", "OQ-2"} {
		if !strings.Contains(src, want) {
			t.Errorf("ADR does not name the %s decision", want)
		}
	}
	statusOK := false
	for line := range strings.SplitSeq(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- Status:") {
			statusOK = strings.Contains(line, "proposed") || strings.Contains(line, "accepted")
		}
	}
	if !statusOK {
		t.Error("ADR has no `- Status:` line reading proposed or accepted (acceptance is the maintainer's to grant)")
	}
}
