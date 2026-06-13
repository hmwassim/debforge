package main

import (
	"os"

	"github.com/hmwassim/debforge/pkg/cli"
	"github.com/hmwassim/debforge/pkg/self"
	"github.com/hmwassim/debforge/pkg/text"
)

func main() {
	log := text.New()

	result, err := cli.Parse()
	if err != nil {
		log.Error(err.Error())
		cli.PrintUsageErr()
		os.Exit(1)
	}

	switch result.Op {
	case cli.OpHelp:
		cli.PrintUsage()
	case cli.OpVersion:
		cli.PrintVersion()
	case cli.OpSelfUpdate:
		if err := self.Update(log); err != nil {
			log.Error(err.Error())
			os.Exit(1)
		}
	case cli.OpSelfRemove:
		if err := self.Remove(log); err != nil {
			log.Error(err.Error())
			os.Exit(1)
		}
	}
}
