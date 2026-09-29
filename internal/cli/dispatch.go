// Package cli implements the mythhelm command line.
package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/turbokast/mythhelm/internal/buildinfo"
)

// Stdio is the standard streams a command reads and writes.
type Stdio struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

type command struct {
	summary string
	run     func(args []string, stdio Stdio) error
}

var commands = map[string]command{
	"version": {summary: "print the version, commit and Go version", run: runVersion},
}

// Main runs the command named by args[0] and returns the process exit code.
func Main(args []string, stdio Stdio) int {
	if len(args) == 0 {
		printUsage(stdio.Err)
		return int(ExitInvalid)
	}
	name := args[0]
	switch name {
	case "help", "-h", "-help", "--help":
		printUsage(stdio.Out)
		return int(ExitOK)
	}
	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintf(stdio.Err, "mythhelm: unknown command %q\n\n", name)
		printUsage(stdio.Err)
		return int(ExitInvalid)
	}
	err := cmd.run(args[1:], stdio)
	code := exitCode(err)
	if code != ExitOK {
		fmt.Fprintf(stdio.Err, "mythhelm %s: %v\n", name, err)
	}
	if code == ExitInvalid {
		fmt.Fprintf(stdio.Err, "run 'mythhelm %s -h' for usage\n", name)
	}
	return int(code)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: mythhelm <command> [flags] [arguments]")
	fmt.Fprintln(w, "\nCommands:")
	for _, name := range slices.Sorted(maps.Keys(commands)) {
		fmt.Fprintf(w, "  %-10s %s\n", name, commands[name].summary)
	}
	fmt.Fprintln(w, "\nRun 'mythhelm <command> -h' for a command's flags.")
}

// ParseInterspersed parses args into fs, allowing flags and positional
// arguments in any order (D1): "apply run_1 --to-branch x" and
// "apply --to-branch x run_1" are equivalent. A "--" ends the flags, and
// a lone "-" is positional.
func ParseInterspersed(fs *flag.FlagSet, args []string) (positional []string, err error) {
	var flags []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(arg) < 2 || arg[0] != '-' {
			positional = append(positional, arg)
			continue
		}
		flags = append(flags, arg)
		if takesSeparateValue(fs, arg) && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	if err := fs.Parse(flags); err != nil {
		return nil, err
	}
	return positional, nil
}

// takesSeparateValue reports whether the flag package will consume the
// argument after arg as arg's value.
func takesSeparateValue(fs *flag.FlagSet, arg string) bool {
	name := strings.TrimPrefix(arg[1:], "-")
	if strings.Contains(name, "=") {
		return false
	}
	f := fs.Lookup(name)
	if f == nil {
		return false
	}
	b, isBool := f.Value.(interface{ IsBoolFlag() bool })
	return !isBool || !b.IsBoolFlag()
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// parseFlags parses a command's arguments. It prints the command's usage
// for -h and marks every other parse failure as a usage error.
func parseFlags(fs *flag.FlagSet, args []string, stdio Stdio) ([]string, error) {
	positional, err := ParseInterspersed(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprintf(stdio.Out, "usage: mythhelm %s [flags]\n\nFlags:\n", fs.Name())
		fs.SetOutput(stdio.Out)
		fs.PrintDefaults()
		return nil, err
	}
	if err != nil {
		return nil, usageError{err}
	}
	return positional, nil
}

func runVersion(args []string, stdio Stdio) error {
	fs := newFlagSet("version")
	format := fs.String("format", "plain", "output format: plain or jsonl")
	positional, err := parseFlags(fs, args, stdio)
	if err != nil {
		return err
	}
	if len(positional) > 0 {
		return usageErrorf("unexpected argument %q", positional[0])
	}

	info := buildinfo.Get()
	switch *format {
	case "plain":
		_, err = fmt.Fprintf(stdio.Out, "mythhelm %s\ncommit: %s\ngo: %s\nplatform: %s/%s\n",
			info.Version, info.Commit, info.GoVersion, info.OS, info.Arch)
	case "jsonl":
		err = json.NewEncoder(stdio.Out).Encode(struct {
			Type      string `json:"type"`
			Version   string `json:"version"`
			Commit    string `json:"commit"`
			GoVersion string `json:"go_version"`
			OS        string `json:"os"`
			Arch      string `json:"arch"`
		}{"version", info.Version, info.Commit, info.GoVersion, info.OS, info.Arch})
	default:
		return usageErrorf("--format must be plain or jsonl, got %q", *format)
	}
	return err
}
