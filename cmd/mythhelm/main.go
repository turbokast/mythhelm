// Command mythhelm supervises native coding agents. See docs/spec/master-spec.md.
package main

import (
	"os"

	"github.com/turbokast/mythhelm/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], cli.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}))
}
