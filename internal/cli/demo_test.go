package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/adapters/fake"
	"github.com/turbokast/mythhelm/internal/workers"
)

func TestMain(m *testing.M) {
	// The test binary plays every process of a demo run: the demo check
	// helper, the worker and the fake agent. Checks replay the demo
	// helper, mirroring the supervisor suite's __check dispatch.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case DemoCheckCommand:
			os.Exit(DemoCheckMain(os.Args[2:], os.Stdout, os.Stderr))
		case workers.Command:
			os.Exit(workers.Main(os.Args[2:]))
		case fake.AgentCommand:
			os.Exit(fake.AgentMain(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
		}
	}
	os.Exit(m.Run())
}

func TestDemoCheckMain(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := DemoCheckMain([]string{"pass"}, &stdout, &stderr); code != 0 {
		t.Fatalf("pass exit %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("pass printed %q, want silence", stdout.String())
	}
	stdout.Reset()
	if code := DemoCheckMain([]string{"fail"}, &stdout, &stderr); code != 1 {
		t.Fatalf("fail exit %d, want 1", code)
	}
	if stdout.Len() == 0 {
		t.Fatal("fail printed no evidence")
	}
	if code := DemoCheckMain([]string{"bogus"}, &stdout, &stderr); code != 2 {
		t.Fatalf("bogus exit %d, want 2", code)
	}
	if code := DemoCheckMain(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("no-args exit %d, want 2", code)
	}
}

// gitOnlyPath returns a PATH holding git's installation and nothing else.
// On Windows git's own usr/bin comes along: without it git cannot spawn.
func gitOnlyPath(t *testing.T) string {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("demo needs git")
	}
	dirs := []string{filepath.Dir(git)}
	if runtime.GOOS == "windows" {
		if usr := filepath.Join(filepath.Dir(filepath.Dir(git)), "usr", "bin"); isDir(usr) {
			dirs = append(dirs, usr)
		} else if usr := filepath.Join(filepath.Dir(git), "usr", "bin"); isDir(usr) {
			dirs = append(dirs, usr)
		}
	}
	return strings.Join(dirs, string(os.PathListSeparator))
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func TestDemoOfflineNoCredentials(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")
	t.Setenv("PATH", gitOnlyPath(t))
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	code, stdout, stderr := runMain("demo")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "SCRIPTED DEMO") {
		t.Fatalf("demo printed no label:\n%s", stdout)
	}
}

func TestDemoCheckFailsScenarioExit5(t *testing.T) {
	code, stdout, stderr := runMain("demo", "--check=fail")
	if code != 5 {
		t.Fatalf("exit %d, stderr %q; want 5", code, stderr)
	}
	if !strings.Contains(stdout, "SCRIPTED DEMO") {
		t.Fatalf("failing demo printed no label:\n%s", stdout)
	}
}

func TestDemoLabelsEveryScreen(t *testing.T) {
	code, stdout, stderr := runMain("demo")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	var screens int
	for line := range strings.Lines(strings.TrimSpace(stdout)) {
		if strings.HasPrefix(line, "===") {
			screens++
			if !strings.Contains(line, "SCRIPTED DEMO") {
				t.Errorf("screen header without the label: %q", line)
			}
		}
	}
	if screens < 4 {
		t.Errorf("screens = %d, want at least the repository, run, review and done screens", screens)
	}
	for _, want := range []string{"Run: ", "State: ", "Candidate diff:"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("demo output lacks %q", want)
		}
	}
}

func TestDemoRemovesTempDirs(t *testing.T) {
	code, stdout, stderr := runMain("demo")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	// The demo announces its working directories before removing them.
	for line := range strings.Lines(stdout) {
		for _, prefix := range []string{"working in ", "state in "} {
			after, ok := strings.CutPrefix(strings.TrimSpace(line), prefix)
			if !ok {
				continue
			}
			dir := strings.TrimSuffix(after, " (removed afterwards)")
			dir = strings.TrimSuffix(dir, " was removed afterwards")
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Errorf("%s still exists: %v", dir, err)
			}
		}
	}
}
