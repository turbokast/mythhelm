package contain

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runContainMain runs `<test binary> __contain` with stdin and returns its
// exit code and stderr.
func runContainMain(t *testing.T, stdin []byte) (int, string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, Command)
	cmd.Stdin = bytes.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err == nil {
		return 0, stderr.String()
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatal(err)
	}
	return exit.ExitCode(), stderr.String()
}

func specJSON(t *testing.T, mutate func(*ContainSpec)) []byte {
	t.Helper()
	// Paths valid on every OS: the test binary and a temp directory.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	spec := ContainSpec{
		Path:   exe,
		Args:   []string{"sh", "-c", "true"},
		Dir:    dir,
		Env:    []string{"HOME=" + dir},
		Policy: Policy{Profile: "restricted", Workdir: filepath.Join(dir, "work")},
	}
	mutate(&spec)
	b, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestContainRejectsOversizeSpec(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	// Well-formed except for its size: only the bound can reject it.
	body := specJSON(t, func(s *ContainSpec) {
		s.Args = []string{"sh", "-c", `touch "` + marker + `"`, strings.Repeat("x", maxSpecBytes)}
	})
	if len(body) <= maxSpecBytes {
		t.Fatalf("fixture is %d bytes, want over %d", len(body), maxSpecBytes)
	}
	code, stderr := runContainMain(t, body)
	if code != 2 || !strings.Contains(stderr, "exceeds") {
		t.Fatalf("exit = %d, stderr = %q, want exit 2 naming the size bound", code, stderr)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the oversize spec's child ran")
	}
}

func TestContainRejectsInvalidSpec(t *testing.T) {
	tests := []struct {
		name  string
		stdin []byte
	}{
		{"not json", []byte("not json")},
		{"unknown field", []byte(`{"path":"/bin/sh","args":["sh"],"policy":{"workdir":"/w"},"extra":1}`)},
		{"relative path", specJSON(t, func(s *ContainSpec) { s.Path = "sh" })},
		{"no argv", specJSON(t, func(s *ContainSpec) { s.Args = nil })},
		{"trailing garbage", append(specJSON(t, func(*ContainSpec) {}), " garbage"...)},
		{"two concatenated specs", append(specJSON(t, func(*ContainSpec) {}), specJSON(t, func(*ContainSpec) {})...)},
		{"relative workdir", specJSON(t, func(s *ContainSpec) { s.Policy.Workdir = "work" })},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if code, stderr := runContainMain(t, tc.stdin); code != 2 {
				t.Fatalf("exit = %d (stderr %q), want 2", code, stderr)
			}
		})
	}
}

func TestContainSetupFailureExitsOne(t *testing.T) {
	// A valid spec without the prompt pipe on fd 3 cannot be entered.
	if code, stderr := runContainMain(t, specJSON(t, func(*ContainSpec) {})); code != 1 {
		t.Fatalf("exit = %d (stderr %q), want 1", code, stderr)
	}
}
