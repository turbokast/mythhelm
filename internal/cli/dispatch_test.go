package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/buildinfo"
)

func runMain(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = Main(args, Stdio{In: strings.NewReader(""), Out: &out, Err: &errOut})
	return code, out.String(), errOut.String()
}

func TestParseInterspersedFlags(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		positional []string
		branch     string
		force      bool
	}{
		{name: "positional first", args: []string{"run_1", "--to-branch", "x"}, positional: []string{"run_1"}, branch: "x"},
		{name: "flag first", args: []string{"--to-branch", "x", "run_1"}, positional: []string{"run_1"}, branch: "x"},
		{name: "equals form", args: []string{"run_1", "--to-branch=x"}, positional: []string{"run_1"}, branch: "x"},
		{name: "single dash", args: []string{"-to-branch", "x", "run_1"}, positional: []string{"run_1"}, branch: "x"},
		{name: "bool flag does not take the next argument", args: []string{"--force", "run_1", "--to-branch", "x"}, positional: []string{"run_1"}, branch: "x", force: true},
		{name: "positionals keep their order", args: []string{"a", "--to-branch", "x", "b"}, positional: []string{"a", "b"}, branch: "x"},
		{name: "double dash ends flags", args: []string{"--to-branch", "x", "--", "--force", "run_1"}, positional: []string{"--force", "run_1"}, branch: "x"},
		{name: "double dash as a flag value", args: []string{"--to-branch", "--", "run_1"}, positional: []string{"run_1"}, branch: "--"},
		{name: "lone dash is positional", args: []string{"-", "--to-branch", "x"}, positional: []string{"-"}, branch: "x"},
		{name: "no arguments", args: nil, positional: nil, branch: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := flag.NewFlagSet("apply", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			branch := fs.String("to-branch", "", "")
			force := fs.Bool("force", false, "")

			positional, err := ParseInterspersed(fs, tt.args)
			if err != nil {
				t.Fatalf("ParseInterspersed(%q) error: %v", tt.args, err)
			}
			if !slices.Equal(positional, tt.positional) {
				t.Errorf("positional = %q, want %q", positional, tt.positional)
			}
			if *branch != tt.branch {
				t.Errorf("to-branch = %q, want %q", *branch, tt.branch)
			}
			if *force != tt.force {
				t.Errorf("force = %v, want %v", *force, tt.force)
			}
		})
	}
}

func TestParseInterspersedFlagsErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown flag after positional", args: []string{"run_1", "--nope"}, want: "flag provided but not defined: -nope"},
		{name: "missing value", args: []string{"run_1", "--to-branch"}, want: "flag needs an argument: -to-branch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := flag.NewFlagSet("apply", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			fs.String("to-branch", "", "")

			_, err := ParseInterspersed(fs, tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestUnknownCommandExits2(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{name: "unknown command", args: []string{"bogus"}, wantStderr: `unknown command "bogus"`},
		{name: "unknown command with control characters is quoted", args: []string{"bo\x1b[2Jgus"}, wantStderr: `unknown command "bo\x1b[2Jgus"`},
		{name: "no command", args: nil, wantStderr: "usage: mythhelm <command>"},
		{name: "unknown flag", args: []string{"version", "--nope"}, wantStderr: "flag provided but not defined: -nope"},
		{name: "unknown format", args: []string{"version", "--format", "yaml"}, wantStderr: `--format must be plain or jsonl, got "yaml"`},
		{name: "unexpected argument", args: []string{"version", "extra"}, wantStderr: `unexpected argument "extra"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runMain(tt.args...)
			if code != int(ExitInvalid) {
				t.Errorf("exit code = %d, want %d", code, ExitInvalid)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tt.wantStderr)
			}
		})
	}
}

func TestHelpExits0(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStdout string
	}{
		{name: "help command", args: []string{"help"}, wantStdout: "version"},
		{name: "long help flag", args: []string{"--help"}, wantStdout: "version"},
		{name: "command help", args: []string{"version", "-h"}, wantStdout: "-format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runMain(tt.args...)
			if code != int(ExitOK) {
				t.Errorf("exit code = %d, want 0 (stderr %q)", code, stderr)
			}
			if !strings.Contains(stdout, tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout, tt.wantStdout)
			}
		})
	}
}

type versionObject struct {
	Type      string `json:"type"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

func TestVersionReportsBuildInfo(t *testing.T) {
	oldVersion, oldCommit := buildinfo.Version, buildinfo.Commit
	buildinfo.Version, buildinfo.Commit = "v1.2.3-test", "0123abcdef"
	t.Cleanup(func() { buildinfo.Version, buildinfo.Commit = oldVersion, oldCommit })

	t.Run("plain", func(t *testing.T) {
		code, stdout, stderr := runMain("version")
		if code != int(ExitOK) {
			t.Fatalf("exit code = %d, want 0 (stderr %q)", code, stderr)
		}
		for _, want := range []string{"v1.2.3-test", "0123abcdef", runtime.Version(), runtime.GOOS + "/" + runtime.GOARCH} {
			if !strings.Contains(stdout, want) {
				t.Errorf("stdout = %q, want it to contain %q", stdout, want)
			}
		}
	})

	t.Run("jsonl", func(t *testing.T) {
		code, stdout, stderr := runMain("version", "--format", "jsonl")
		if code != int(ExitOK) {
			t.Fatalf("exit code = %d, want 0 (stderr %q)", code, stderr)
		}
		got := decodeSingleVersionObject(t, stdout)
		want := versionObject{Type: "version", Version: "v1.2.3-test", Commit: "0123abcdef", GoVersion: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH}
		if got != want {
			t.Errorf("version object = %+v, want %+v", got, want)
		}
	})

	t.Run("built binary takes link-time values and exit codes", func(t *testing.T) {
		goBin, err := exec.LookPath("go")
		if err != nil {
			t.Skip("go command not on PATH; cannot build cmd/mythhelm")
		}
		bin := filepath.Join(t.TempDir(), "mythhelm")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		ldflags := "-X github.com/turbokast/mythhelm/internal/buildinfo.Version=v9.9.9-ldflags" +
			" -X github.com/turbokast/mythhelm/internal/buildinfo.Commit=feedface"
		build := exec.Command(goBin, "build", "-ldflags", ldflags, "-o", bin, "github.com/turbokast/mythhelm/cmd/mythhelm")
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("go build: %v\n%s", err, out)
		}

		out, err := exec.Command(bin, "version", "--format", "jsonl").Output()
		if err != nil {
			t.Fatalf("mythhelm version: %v", err)
		}
		got := decodeSingleVersionObject(t, string(out))
		if got.Type != "version" || got.Version != "v9.9.9-ldflags" || got.Commit != "feedface" || !strings.HasPrefix(got.GoVersion, "go1.") {
			t.Errorf("version object = %+v, want type version, version v9.9.9-ldflags, commit feedface, a go1.x go_version", got)
		}

		err = exec.Command(bin, "bogus").Run()
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != int(ExitInvalid) {
			t.Errorf("mythhelm bogus: err = %v, want exit code %d", err, ExitInvalid)
		}
	})
}

func decodeSingleVersionObject(t *testing.T, stdout string) versionObject {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("stdout has %d lines, want exactly 1: %q", len(lines), stdout)
	}
	dec := json.NewDecoder(strings.NewReader(lines[0]))
	dec.DisallowUnknownFields()
	var got versionObject
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("stdout line is not a version object: %v: %q", err, lines[0])
	}
	return got
}
