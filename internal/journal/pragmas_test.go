package journal

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// TestPragmasPinned // NFR-2 (supervisor-migration design §8): the open
// pragma string carries WAL with synchronous FULL and the design-time
// busy timeout; any drift fails. TestPragmasApplied pins the effective
// per-connection values; this test pins the string that sets them.
func TestPragmasPinned(t *testing.T) {
	for _, want := range []string{"journal_mode(WAL)", "synchronous(FULL)", "busy_timeout(5000)"} {
		if !strings.Contains(pragmas, want) {
			t.Errorf("open pragma string = %q, want it to contain %q", pragmas, want)
		}
	}

	// The busy timeout is named once at design time (5000 ms, design §8):
	// every busy_timeout in this package's non-test sources names it, so
	// no second value lurks in a read-only DSN.
	timeoutArg := regexp.MustCompile(`busy_timeout\(([^)]*)\)`)
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}
	entries, err := os.ReadDir(filepath.Dir(file))
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), name)) //nolint:gosec // G304: the test's own package dir; name comes from reading it
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range timeoutArg.FindAllStringSubmatch(string(raw), -1) {
			seen = true
			if m[1] != "5000" {
				t.Errorf("%s names busy_timeout(%s), want the design-time busy_timeout(5000)", name, m[1])
			}
		}
	}
	if !seen {
		t.Error("no busy_timeout found in the package sources; the pin is vacuous")
	}
}
