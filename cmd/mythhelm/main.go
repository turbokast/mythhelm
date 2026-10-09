// Command mythhelm supervises native coding agents. See docs/spec/master-spec.md.
package main

import (
	"os"

	"github.com/turbokast/mythhelm/adapters/fake"
	"github.com/turbokast/mythhelm/internal/cli"
	"github.com/turbokast/mythhelm/internal/workers"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case fake.AgentCommand: // hidden: the fake adapter's scripted agent, run as mythhelm's own child
			os.Exit(fake.AgentMain(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
		case workers.Command: // hidden: an attempt's detached worker (design §3)
			os.Exit(workers.Main(os.Args[2:]))
		case cli.SupervisorCommand: // hidden: the lazily started per-user supervisor
			os.Exit(cli.SupervisorMain(os.Stderr))
		case cli.DemoCheckCommand: // hidden: the demo repository's scripted check
			os.Exit(cli.DemoCheckMain(os.Args[2:], os.Stdout, os.Stderr))
		}
	}
	os.Exit(cli.Main(os.Args[1:], cli.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}))
}
