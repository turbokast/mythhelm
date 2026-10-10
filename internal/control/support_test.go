package control

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// supportRow is one row of SUPPORT.md: a shipped (or blocked) deliverable,
// its status, the platforms its evidence runs on and the tests that are that
// evidence.
type supportRow struct {
	ID, Deliverable, Status, Platforms, Evidence string
	Tests                                        []string
}

var testName = regexp.MustCompile("`(Test[A-Za-z0-9_]+)`")

// parseSupport reads the matrix: every table row with five cells whose first
// cell is a backticked ID.
func parseSupport(t *testing.T, src string) []supportRow {
	t.Helper()
	var rows []supportRow
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
		r := supportRow{ID: strings.Trim(cells[0], "`"), Deliverable: cells[1], Status: cells[2], Platforms: cells[3], Evidence: cells[4]}
		for _, m := range testName.FindAllStringSubmatch(cells[4], -1) {
			r.Tests = append(r.Tests, m[1])
		}
		rows = append(rows, r)
	}
	return rows
}

func readSupport(t *testing.T) []supportRow {
	t.Helper()
	raw, err := os.ReadFile("SUPPORT.md")
	if err != nil {
		t.Fatal(err)
	}
	rows := parseSupport(t, string(raw))
	if len(rows) == 0 {
		t.Fatal("SUPPORT.md has no matrix rows")
	}
	return rows
}

// testPlatforms maps every test function in this package to the platforms
// its file is built for: "all" or "linux, darwin".
func testPlatforms(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
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
		// An implicit GOOS filename suffix constrains the file even without a
		// build tag; without this an untagged foo_windows_test.go would be
		// mislabelled "all".
		base := strings.TrimSuffix(name, "_test.go")
		for _, goos := range []string{"linux", "darwin", "windows"} {
			if strings.HasSuffix(base, "_"+goos) {
				platforms = goos
				break
			}
		}
		switch platforms {
		case "all", "linux || darwin", "windows":
			platforms = strings.Replace(platforms, " || ", ", ", 1)
		default:
			continue // a file for platforms outside the matrix (the stand-in for the rest)
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, raw, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") && fn.Name.Name != "TestMain" {
				out[fn.Name.Name] = platforms
			}
		}
	}
	return out
}

// supportProblems checks rows against the code and the tests, returning one
// message per defect.
func supportProblems(rows []supportRow, methods []string, tests map[string]string) []string {
	var problems []string
	ids := map[string]bool{}
	var methodRows, otherRows []string
	for _, r := range rows {
		if ids[r.ID] {
			problems = append(problems, "duplicate row "+r.ID)
		}
		ids[r.ID] = true
		if m, ok := strings.CutPrefix(r.ID, "method:"); ok {
			methodRows = append(methodRows, m)
		} else {
			otherRows = append(otherRows, r.ID)
		}
		switch r.Status {
		case "fixture-tested":
			if len(r.Tests) == 0 {
				problems = append(problems, r.ID+": fixture-tested with no evidence test")
			}
			for _, name := range r.Tests {
				platforms, ok := tests[name]
				switch {
				case !ok:
					problems = append(problems, fmt.Sprintf("%s: evidence %s is not a test in this package", r.ID, name))
				case platforms != r.Platforms:
					problems = append(problems, fmt.Sprintf("%s: claims %q but %s is built for %q", r.ID, r.Platforms, name, platforms))
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
	slices.Sort(methodRows)
	want := slices.Sorted(slices.Values(methods))
	if !slices.Equal(methodRows, want) {
		problems = append(problems, fmt.Sprintf("method rows %v differ from the methods the supervisor serves %v", methodRows, want))
	}
	slices.Sort(otherRows)
	if !slices.Equal(otherRows, shippedDeliverables) {
		problems = append(problems, fmt.Sprintf("deliverable rows %v differ from the shipped deliverables %v", otherRows, shippedDeliverables))
	}
	return problems
}

// shippedDeliverables are the non-method rows, sorted. A new deliverable
// adds its row here and to SUPPORT.md together.
var shippedDeliverables = []string{
	"capability-tokens", "envelope-reconnect", "execute-idempotency", "frame-codec", "instance-lock",
	"lazy-start", "pinned-ladder", "sole-writer-mutate", "transport-unix", "transport-windows-pipe",
}

func servedMethods() []string {
	srv := NewSupervisorServer(nil)
	var out []string
	for m := range srv.methods {
		out = append(out, m)
	}
	return out
}

func TestSupportMatrixMatchesEvidence(t *testing.T) {
	t.Parallel()
	rows := readSupport(t)
	if problems := supportProblems(rows, servedMethods(), testPlatforms(t)); len(problems) > 0 {
		t.Fatalf("SUPPORT.md disagrees with the code:\n  %s", strings.Join(problems, "\n  "))
	}

	// The check can fail: each defect below is reported.
	tests := testPlatforms(t)
	for name, mutate := range map[string]func([]supportRow) ([]supportRow, []string){
		"a new control method without a row": func(r []supportRow) ([]supportRow, []string) { return r, append(servedMethods(), "newmethod") },
		"a live-qualified claim": func(r []supportRow) ([]supportRow, []string) {
			r = slices.Clone(r)
			r[0].Status = "live-qualified"
			return r, servedMethods()
		},
		"a row naming a test that does not exist": func(r []supportRow) ([]supportRow, []string) {
			r = slices.Clone(r)
			r[0].Tests = append(slices.Clone(r[0].Tests), "TestNoSuchThing")
			return r, servedMethods()
		},
		"a row for a deliverable that was removed": func(r []supportRow) ([]supportRow, []string) {
			return r[1:], servedMethods()
		},
	} {
		mutated, methods := mutate(rows)
		if len(supportProblems(mutated, methods, tests)) == 0 {
			t.Errorf("%s passes the matrix check", name)
		}
	}
}

// windowsRowProblem reports how the Windows pipe row contradicts whether the
// transport shipped.
func windowsRowProblem(rows []supportRow, shipped bool) string {
	for _, r := range rows {
		if r.ID != "transport-windows-pipe" {
			continue
		}
		want := "blocked"
		if shipped {
			want = "fixture-tested"
		}
		if r.Status != want {
			return fmt.Sprintf("the Windows pipe row reads %q, but the transport has shipped=%v so it must read %q", r.Status, shipped, want)
		}
		return ""
	}
	return "no Windows pipe row"
}

func TestWindowsRowMatchesLanding(t *testing.T) {
	t.Parallel()
	rows := readSupport(t)
	_, err := os.Stat("transport_windows.go")
	shipped := err == nil
	if problem := windowsRowProblem(rows, shipped); problem != "" {
		t.Fatal(problem)
	}
	// Both contradictions are caught.
	if windowsRowProblem(rows, !shipped) == "" {
		t.Error("a row contradicting the shipped code passes")
	}
	flipped := slices.Clone(rows)
	for i := range flipped {
		if flipped[i].ID == "transport-windows-pipe" {
			flipped[i].Status = "fixture-tested"
			if shipped {
				flipped[i].Status = "blocked"
			}
		}
	}
	if windowsRowProblem(flipped, shipped) == "" {
		t.Error("a flipped Windows row passes")
	}
}
