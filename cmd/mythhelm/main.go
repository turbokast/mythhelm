// Command mythhelm supervises native coding agents. See docs/spec/master-spec.md.
package main

import (
	"os"

	"github.com/turbokast/mythhelm/adapters/fake"
	"github.com/turbokast/mythhelm/internal/cli"
)

func main() {
	// Hidden: the fake adapter's scripted agent, run as mythhelm's own child.
	if len(os.Args) > 1 && os.Args[1] == fake.AgentCommand {
		os.Exit(fake.AgentMain(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(cli.Main(os.Args[1:], cli.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}))
}
