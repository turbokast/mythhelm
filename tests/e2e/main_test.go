// Package e2e exercises the final packaged mythhelm binary end to end
// (master §22.2: claims about packaging or process behaviour must test
// the packaged build). TestMain builds cmd/mythhelm once; every test runs
// that binary against temporary homes, states and repositories.
package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/turbokast/mythhelm/internal/contain"
)

// mythhelmBin is the packaged binary under test, built once by TestMain.
var mythhelmBin string

func TestMain(m *testing.M) {
	// The boundary probe re-executes this binary as `__contain`: dispatch
	// it before the build so the probe child never rebuilds or runs the
	// suite, and the availability gate reads an honest verdict.
	if len(os.Args) > 1 && os.Args[1] == contain.Command {
		os.Exit(contain.Main(os.Args[2:]))
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "e2e: cannot locate the test directory")
		os.Exit(1)
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	// A unique staging dir: concurrent test processes on one host must
	// not share or remove each other's binary.
	out, err := os.MkdirTemp("", "mythhelm-e2e-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e: staging build dir:", err)
		os.Exit(1)
	}
	name := "mythhelm-e2e"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(out, name)
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/mythhelm")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: building mythhelm: %v\n%s", err, out)
		os.Exit(1)
	}
	mythhelmBin = bin
	code := m.Run()
	_ = os.RemoveAll(out)
	os.Exit(code)
}
