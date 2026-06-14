package main

import (
	"os"

	"github.com/hmwassim/debforge/pkg/cli"
	"github.com/hmwassim/debforge/pkg/core"
	"github.com/hmwassim/debforge/pkg/self"
	"github.com/hmwassim/debforge/pkg/text"
)

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
		switch result.Args[0] {
		case "repair":
			if err := core.Repair(log); err != nil {
				log.Error("%s", err)
				os.Exit(1)
			}
		case "update":
			if err := core.Update(log); err != nil {
				log.Error("%s", err)
				os.Exit(1)
			}
		case "list":
			core.List(log)
		default:
			log.Error("unknown core subcommand: %s", result.Args[0])
			os.Exit(1)
		}
	default:
		log.Error("unhandled operation: %s", result.Op)
		os.Exit(1)
	}
}
