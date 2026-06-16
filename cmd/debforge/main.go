package main

import (
	"os"
	"path/filepath"

	"github.com/hmwassim/debforge/pkg/cli"
	"github.com/hmwassim/debforge/pkg/core"
	"github.com/hmwassim/debforge/pkg/repo"
	"github.com/hmwassim/debforge/pkg/self"
	"github.com/hmwassim/debforge/pkg/settings"
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

	if err := repo.LoadFromDir(filepath.Join(settings.Default.SourceDir(), "data", "packages")); err != nil {
		log.Warn("Could not load package definitions: %s", err)
	}

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
	case cli.OpInstall:
		for _, name := range result.Args {
			pkg := repo.Lookup(name)
			if pkg == nil {
				log.Error("unknown package: %s", name)
				os.Exit(1)
			}
			if err := pkg.Install(log); err != nil {
				log.Error("%s", err)
				os.Exit(1)
			}
		}
	case cli.OpRemove:
		for _, name := range result.Args {
			pkg := repo.Lookup(name)
			if pkg == nil {
				log.Error("unknown package: %s", name)
				os.Exit(1)
			}
			if err := pkg.Remove(log); err != nil {
				log.Error("%s", err)
				os.Exit(1)
			}
		}
	case cli.OpList:
		state := repo.LoadState()
		for _, name := range repo.List() {
			if _, ok := state.Packages[name]; ok {
				log.Success("  %s — installed", name)
			} else {
				log.Info("  %s — not installed", name)
			}
		}
	default:
		log.Error("unhandled operation: %s", result.Op)
		os.Exit(1)
	}
}
