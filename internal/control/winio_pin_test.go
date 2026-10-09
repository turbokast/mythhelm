package control

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// reviewedWinio is the go-winio version the maintainer approved under OQ-7.
const reviewedWinio = "v0.6.3"

var (
	winioRequire = regexp.MustCompile(`(?m)^\s*(?:require\s+)?github\.com/Microsoft/go-winio\s+(\S+)`)
	winioReview  = regexp.MustCompile(`go-winio (v\d+\.\d+\.\d+)`)
)

func findUp(t *testing.T, rel string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if p := filepath.Join(dir, rel); fileExists(p) {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("%s not found above the test directory", rel)
		}
		dir = parent
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func TestGoWinioPinned(t *testing.T) {
	t.Parallel()

	mod, err := os.ReadFile(findUp(t, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	m := winioRequire.FindSubmatch(mod)
	if m == nil {
		t.Fatal("go.mod does not require github.com/Microsoft/go-winio")
	}
	if got := string(m[1]); got != reviewedWinio {
		t.Fatalf("go.mod pins go-winio %s, want the reviewed %s", got, reviewedWinio)
	}

	matches, err := filepath.Glob(filepath.Join(filepath.Dir(findUp(t, "go.mod")), "specs", "*", "supervisor-service", "scratchpad.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range matches {
		note, err := os.ReadFile(p) //nolint:gosec // G304: p comes from a glob under the repository root
		if err != nil {
			t.Fatal(err)
		}
		r := winioReview.FindSubmatch(note)
		if r == nil || string(r[1]) != reviewedWinio {
			t.Fatalf("%s does not record approval of go-winio %s", p, reviewedWinio)
		}
	}
}
