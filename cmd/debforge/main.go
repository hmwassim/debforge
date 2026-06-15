package main

import (
	"os"

	"github.com/hmwassim/debforge/pkg/cli"
	"github.com/hmwassim/debforge/pkg/core"
	"github.com/hmwassim/debforge/pkg/self"
	"github.com/hmwassim/debforge/pkg/text"
)

func hasFlag(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f {
				return true
			}
		}
	}
	return false
}

func main() {
	log := text.New()

	result, err := cli.Parse()
	if err != nil {
		log.Error("%s", err)
		cli.PrintUsage(os.Stderr)
		os.Exit(1)
	}

	switch result.Op {
	case cli.OpHelp:
		cli.PrintUsage(os.Stdout)
	case cli.OpVersion:
		cli.PrintVersion()
	case cli.OpSelfUpdate:
		if err := self.Update(log); err != nil {
			log.Error("%s", err)
			os.Exit(1)
		}
	case cli.OpSelfRemove:
		if err := self.Remove(log); err != nil {
			log.Error("%s", err)
			os.Exit(1)
		}
	case cli.OpCore:
		sub := result.Args[0]
		rest := result.Args[1:]
		switch sub {
		case "setup":
			if err := core.Setup(log, hasFlag(rest, "-f", "--force")); err != nil {
				log.Error("%s", err)
				os.Exit(1)
			}
		case "list":
			core.List(log)
		default:
			log.Error("unknown core subcommand: %s", sub)
			os.Exit(1)
		}
	default:
		log.Error("unhandled operation: %s", result.Op)
		os.Exit(1)
	}
}
