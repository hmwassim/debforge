package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

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

func filterArgs(args []string, flags ...string) []string {
	var out []string
	for _, a := range args {
		skip := false
		for _, f := range flags {
			if a == f {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, a)
		}
	}
	return out
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
		names := filterArgs(result.Args, "-f", "--force")
		force := len(names) != len(result.Args)
		for _, name := range names {
			pkg := repo.Lookup(name)
			if pkg == nil {
				log.Error("unknown package: %s", name)
				os.Exit(1)
			}
			if err := pkg.Install(log, force); err != nil {
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
	case cli.OpUpdate:
		names := filterArgs(result.Args, "--all")
		all := len(names) != len(result.Args)

		if err := repo.SystemUpdate(log); err != nil {
			log.Error("%s", err)
			os.Exit(1)
		}

		state, err := repo.LoadState()
		if err != nil {
			log.Warn("Could not load state: %s", err)
			state = &repo.PackagesState{Packages: map[string]repo.PkgEntry{}}
		}

		if all {
			for name, entry := range state.Packages {
				if entry.Type != "deb" {
					continue
				}
				pkg := repo.Lookup(name)
				if pkg == nil {
					continue
				}
				if err := pkg.Install(log, false); err != nil {
					log.Error("updating %s: %s", name, err)
				}
			}
		} else {
			for _, name := range names {
				pkg := repo.Lookup(name)
				if pkg == nil {
					log.Error("unknown package: %s", name)
					os.Exit(1)
				}
				if _, ok := state.Packages[name]; !ok {
					log.Warn("%s is not installed", name)
					continue
				}
				if pkg.Type != "deb" {
					log.Warn("%s is not a deb package; system upgrade handles it", name)
					continue
				}
				if err := pkg.Install(log, false); err != nil {
					log.Error("updating %s: %s", name, err)
				}
			}
		}
	case cli.OpList:
		state, err := repo.LoadState()
		if err != nil {
			log.Warn("Could not load state: %s", err)
			state = &repo.PackagesState{Packages: map[string]repo.PkgEntry{}}
		}
		names := repo.List()
		sort.Strings(names)
		for _, name := range names {
			if _, ok := state.Packages[name]; ok {
				log.Success("  %s", name)
			} else {
				log.Muted("  %s", name)
			}
		}
	case cli.OpSearch:
		query := strings.Join(result.Args, " ")
		if query == "" {
			log.Error("search requires a query")
			os.Exit(1)
		}
		state, err := repo.LoadState()
		if err != nil {
			log.Warn("Could not load state: %s", err)
			state = &repo.PackagesState{Packages: map[string]repo.PkgEntry{}}
		}
		names := repo.List()
		sort.Strings(names)
		for _, name := range names {
			if !strings.Contains(name, query) {
				continue
			}
			if _, ok := state.Packages[name]; ok {
				log.Success("  %s", name)
			} else {
				log.Muted("  %s", name)
			}
		}
	default:
		log.Error("unhandled operation: %s", result.Op)
		os.Exit(1)
	}
}
