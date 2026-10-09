package contain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Command is the hidden mythhelm subcommand that builds the boundary around a
// native and execs it.
const Command = "__contain"

const (
	maxSpecBytes = 1 << 20
	promptFD     = 3 // the launcher's first extra file: the native's stdin
)

// Main runs `mythhelm __contain`: it reads a ContainSpec from stdin, puts the
// prompt pipe on fd 3 onto stdin and execs the native inside the boundary. It
// returns 2 for an invalid spec and 1 for a setup failure; it does not return
// when the exec succeeds. A process started with ProbeEnv set runs the probe
// instead (ProbeLinux).
func Main(args []string) int {
	if os.Getenv(ProbeEnv) != "" {
		return RunProbeChild()
	}
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "contain: unexpected arguments")
		return 2
	}
	spec, err := readSpec(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "contain:", err)
		return 2
	}
	if err := promptToStdin(); err != nil {
		fmt.Fprintln(os.Stderr, "contain:", err)
		return 1
	}
	fmt.Fprintln(os.Stderr, "contain:", EnterLinux(spec))
	return 1
}

func readSpec(r io.Reader) (ContainSpec, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxSpecBytes+1))
	if err != nil {
		return ContainSpec{}, err
	}
	if len(b) > maxSpecBytes {
		return ContainSpec{}, fmt.Errorf("spec exceeds %d bytes", maxSpecBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var spec ContainSpec
	if err := dec.Decode(&spec); err != nil {
		return ContainSpec{}, fmt.Errorf("decoding the spec: %w", err)
	}
	switch {
	case !filepath.IsAbs(spec.Path):
		return ContainSpec{}, errors.New("spec path is not absolute")
	case len(spec.Args) == 0:
		return ContainSpec{}, errors.New("spec has no argv")
	case !filepath.IsAbs(spec.Policy.Workdir):
		return ContainSpec{}, errors.New("spec workdir is not absolute")
	}
	return spec, nil
}
