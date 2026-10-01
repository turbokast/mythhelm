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
)

// mythhelmBin is the packaged binary under test, built once by TestMain.
var mythhelmBin string

func TestMain(m *testing.M) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "e2e: cannot locate the test directory")
		os.Exit(1)
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	out := filepath.Join(os.TempDir(), "mythhelm-e2e-bin")
	name := "mythhelm-e2e"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(out, name)
	if err := os.MkdirAll(out, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "e2e: staging build dir:", err)
		os.Exit(1)
	}
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
