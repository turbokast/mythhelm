// Package e2e exercises the final packaged mythhelm binary end to end
// (master §22.2: claims about packaging or process behaviour must test
// the packaged build). TestMain builds cmd/mythhelm once; every test runs
// that binary against temporary homes, states and repositories.
package e2e

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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
	// A copy of this binary installed as `codex` is the synthetic native
	// Codex of codex_qualification_test.go; installed as `git` or as the
	// check gate it holds one lifecycle phase at a named barrier.
	switch filepath.Base(os.Args[0]) {
	case "codex", "codex.exe":
		os.Exit(codexFixtureMain())
	case "git", "git.exe":
		os.Exit(gitGateMain(os.Args[1:]))
	case checkGateName, checkGateName + ".exe":
		os.Exit(checkGateMain(os.Args[1:]))
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

// checkGateName is the project check that holds verification at its gate.
const checkGateName = "e2e-check-gate"

// Gate barrier files: the helper creates gateEntered when it reaches its
// phase and proceeds only once the test creates gateRelease.
const (
	gateEntered = "entered"
	gateRelease = "release"
)

// holdAtGate announces arrival in dir and waits, bounded, for the release.
// A removed gate directory ends the wait: its test has finished.
func holdAtGate(dir string) error {
	if err := os.WriteFile(filepath.Join(dir, gateEntered), nil, 0o600); err != nil { //nolint:gosec // G703: dir is the gate directory the test fixture created and passed to its own helper
		return err
	}
	deadline := time.Now().Add(5 * time.Minute)
	for {
		if _, err := os.Stat(filepath.Join(dir, gateRelease)); err == nil { //nolint:gosec // G703: fixed barrier name in the fixture-owned gate directory
			return nil
		}
		if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) { //nolint:gosec // G703: the fixture-owned gate directory
			return errors.New("e2e gate directory was removed before release")
		}
		if time.Now().After(deadline) {
			return errors.New("e2e gate was never released")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// checkGateMain is a project check that first holds at the gate directory
// named by its first argument and then passes or fails as its second says.
func checkGateMain(args []string) int {
	if len(args) != 2 || (args[1] != "pass" && args[1] != "fail") {
		fmt.Fprintln(os.Stderr, "usage: e2e-check-gate <gate-dir> pass|fail")
		return 2
	}
	if err := holdAtGate(args[0]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if args[1] == "fail" {
		fmt.Fprintln(os.Stderr, "e2e check gate: failing as configured")
		return 1
	}
	return 0
}

// gitGateMain runs E2E_REAL_GIT with the same arguments, first holding at
// E2E_GATE_DIR when the Git subcommand is E2E_GATE_GIT.
func gitGateMain(args []string) int {
	if dir := os.Getenv("E2E_GATE_DIR"); dir != "" && gitSubcommand(args) == os.Getenv("E2E_GATE_GIT") {
		if err := holdAtGate(dir); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	cmd := exec.Command(os.Getenv("E2E_REAL_GIT"), args...) //nolint:gosec // G702: the fixture sets E2E_REAL_GIT to the real Git from exec.LookPath and forwards the product's own Git argv; no shell
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		return exit.ExitCode()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e git gate:", err)
		return 1
	}
	return 0
}

// gitSubcommand is the first argument that is neither an option nor the
// value of a -c option.
func gitSubcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-c":
			i++
		case !strings.HasPrefix(args[i], "-"):
			return args[i]
		}
	}
	return ""
}
