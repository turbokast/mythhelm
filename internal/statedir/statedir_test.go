package statedir

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolve(t *testing.T) {
	// Absolute on every host OS, so filepath.IsAbs agrees with the fixture.
	root := t.TempDir()
	home := filepath.Join(root, "home", "ada")
	custom := filepath.Join(root, "srv", "mh")
	xdg := filepath.Join(root, "xdg", "state")
	appData := filepath.Join(root, "AppData", "Local")
	tests := []struct {
		name string
		goos string
		env  map[string]string
		want string
	}{
		{name: "MYTHHELM_HOME wins", goos: "linux", env: map[string]string{"MYTHHELM_HOME": custom, "XDG_STATE_HOME": xdg}, want: custom},
		{name: "MYTHHELM_HOME wins on windows", goos: "windows", env: map[string]string{"MYTHHELM_HOME": custom, "LocalAppData": appData}, want: custom},
		{name: "linux XDG_STATE_HOME", goos: "linux", env: map[string]string{"XDG_STATE_HOME": xdg}, want: filepath.Join(xdg, "mythhelm")},
		{name: "linux default", goos: "linux", want: filepath.Join(home, ".local", "state", "mythhelm")},
		{name: "linux relative XDG_STATE_HOME is ignored", goos: "linux", env: map[string]string{"XDG_STATE_HOME": "rel"}, want: filepath.Join(home, ".local", "state", "mythhelm")},
		{name: "freebsd follows XDG", goos: "freebsd", want: filepath.Join(home, ".local", "state", "mythhelm")},
		{name: "macOS ignores XDG", goos: "darwin", env: map[string]string{"XDG_STATE_HOME": xdg}, want: filepath.Join(home, "Library", "Application Support", "mythhelm")},
		{name: "windows", goos: "windows", env: map[string]string{"LocalAppData": appData}, want: filepath.Join(appData, "mythhelm")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolve(tt.goos, func(k string) string { return tt.env[k] }, func() (string, error) { return home, nil })
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolve = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveErrors(t *testing.T) {
	noHome := func() (string, error) { return "", errors.New("no home") }
	noEnv := func(string) string { return "" }
	if _, err := resolve("linux", noEnv, noHome); err == nil {
		t.Error("linux without a home directory: want an error")
	}
	if _, err := resolve("windows", noEnv, noHome); err == nil {
		t.Error("windows without LocalAppData: want an error")
	}
	if _, err := resolve("linux", func(k string) string {
		if k == "MYTHHELM_HOME" {
			return "relative/state"
		}
		return ""
	}, noHome); err == nil {
		t.Error("relative MYTHHELM_HOME: want an error")
	}
}

func TestStateDirMode0700(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits do not apply on Windows")
	}
	root := t.TempDir()

	fresh := filepath.Join(root, "parent", "mythhelm")
	if err := Ensure(fresh); err != nil {
		t.Fatalf("Ensure(new dir): %v", err)
	}
	assertMode(t, fresh, 0o700)

	existing := filepath.Join(root, "existing")
	wideDir(t, existing)
	if err := Ensure(existing); err != nil {
		t.Fatalf("Ensure(existing 0755 dir): %v", err)
	}
	assertMode(t, existing, 0o700)
}

func TestEnsureRejectsFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(file); err == nil {
		t.Fatal("Ensure on a regular file: want an error")
	}
}

func TestEnsureRefusesSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "shared")
	wideDir(t, target)
	link := filepath.Join(root, "mythhelm")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
	if err := Ensure(link); err == nil {
		t.Fatal("Ensure accepted a symlinked state directory")
	}
	if runtime.GOOS != "windows" {
		assertMode(t, target, 0o755)
	}
}

// wideDir creates a directory with mode 0755, wider than a state directory
// may be, for Ensure to narrow or to leave alone.
func wideDir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o755); err != nil { //nolint:gosec // G302: the fixture must be wider than 0700
		t.Fatal(err)
	}
}

func assertMode(t *testing.T, dir string, want os.FileMode) {
	t.Helper()
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != want {
		t.Fatalf("%s has mode %#o, want %#o", dir, got, want)
	}
}
