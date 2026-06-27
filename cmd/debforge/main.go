package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/hmwassim/debforge/internal/adapters/exec"
	"github.com/hmwassim/debforge/internal/adapters/fs"
	"github.com/hmwassim/debforge/internal/adapters/lock"
	"github.com/hmwassim/debforge/internal/adapters/system"
	"github.com/hmwassim/debforge/internal/adapters/ui"
	"github.com/hmwassim/debforge/internal/buildmeta"
	"github.com/hmwassim/debforge/internal/self"
)

var version = buildmeta.DefaultVersion

func main() {
	os.Exit(run())
}

func run() int {
	ctx := context.Background()

	rawArgs := os.Args[1:]
	if len(rawArgs) == 0 {
		usage()
		return 0
	}

	cfgu := self.DefaultConfig()
	if v := os.Getenv("DEBFORGE_PKGS_DIR"); v != "" {
		cfgu.PkgsDir = v
	}
	fsys := fs.NewFileSystem()
	runner := exec.NewRunner()
	locker := lock.NewFLock()
	sys := system.NewSystem()
	u := ui.NewConsoleUI()

	switch rawArgs[0] {
	case "--version":
		fmt.Println("debforge " + version)
		return 0

	case "--self-update":
		updater := self.NewUpdater(cfgu, runner, fsys, u, locker, sys)
		if err := updater.Update(ctx); err != nil {
			u.Error("%s", err)
			return 1
		}
		return 0

	case "--help":
		usage()
		return 0

	case "--self-remove":
		h, err := newHandler(cfgu, fsys, runner, locker, u)
		if err != nil {
			u.Error("bootstrap: %s", err)
			return 1
		}
		remover := self.NewRemover(cfgu, runner, fsys, u, locker, sys, h.reg, h.instReg, h.stateSvc)
		if err := remover.Remove(ctx); err != nil {
			u.Error("%s", err)
			return 1
		}
		return 0
	}

	fs := flag.NewFlagSet("debforge", flag.ContinueOnError)
	y := fs.Bool("y", false, "Skip confirmation prompts")
	yes := fs.Bool("yes", false, "Skip confirmation prompts")
	f := fs.Bool("f", false, "Force operation (reinstall)")
	force := fs.Bool("force", false, "Force operation (reinstall)")
	a := fs.Bool("a", false, "Update all packages (update only)")
	all := fs.Bool("all", false, "Update all packages (update only)")
	fs.Usage = usage

	if err := fs.Parse(rawArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		u.Error("%s", err)
		return 1
	}

	yesMode := *y || *yes
	forceMode := *f || *force
	allMode := *a || *all
	args := fs.Args()

	if yesMode {
		u.SetYes(true)
	}

	if len(args) == 0 {
		usage()
		return 0
	}

	h, err := newHandler(cfgu, fsys, runner, locker, u)
	if err != nil {
		u.Error("bootstrap: %s", err)
		return 1
	}

	switch args[0] {
	case "install":
		names := extractFlags(args[1:], &yesMode, &forceMode, &allMode)
		if len(names) == 0 {
			usage()
			return 1
		}
		if !loadDefs(h.reg, names, fsys, u) {
			return 1
		}
		return h.install(ctx, u, names, forceMode)

	case "remove":
		names := extractFlags(args[1:], &yesMode, &forceMode, &allMode)
		if len(names) == 0 {
			usage()
			return 1
		}
		if !loadDefs(h.reg, names, fsys, u) {
			return 1
		}
		return h.remove(ctx, u, names)

	case "update":
		names := extractFlags(args[1:], &yesMode, &forceMode, &allMode)
		if len(names) == 0 && !allMode {
			usage()
			return 1
		}
		if allMode && len(names) > 0 {
			u.Warn("--all updates every managed package; ignoring explicit name(s): %s", strings.Join(names, ", "))
			names = nil
		}
		if !loadDefs(h.reg, names, fsys, u) {
			return 1
		}
		return h.update(ctx, u, names, forceMode, allMode)

	default:
		usage()
	}
	return 0
}

func usage() {
	fmt.Println("debforge - package manager")
	fmt.Println()
	fmt.Println("Usage: debforge [flags] <command> [<name>...]")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("    -y, --yes           Skip confirmation prompts")
	fmt.Println("    -f, --force         Force operation (reinstall)")
	fmt.Println("    -a, --all           Update all packages (update only)")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("    install <name>...    Install packages")
	fmt.Println("    remove <name>...    Remove packages from system")
	fmt.Println("    update [<name>...]   Reinstall packages (runs apt-get update)")
	fmt.Println("        --all           Update all packages and run apt-get upgrade")
	fmt.Println("    --self-update       Update debforge itself")
	fmt.Println("    --self-remove       Remove debforge from system")
	fmt.Println("    --help              Show this help")
	fmt.Println("    --version           Show version")
}
