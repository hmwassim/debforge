package main

import (
	"context"
	"fmt"
	"os"

	"github.com/hmwassim/debforge/internal/adapters/exec"
	"github.com/hmwassim/debforge/internal/adapters/fs"
	"github.com/hmwassim/debforge/internal/adapters/lock"
	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/adapters/ui"
	"github.com/hmwassim/debforge/internal/buildmeta"
	"github.com/hmwassim/debforge/internal/domain/installer"
	aptInst "github.com/hmwassim/debforge/internal/domain/installer/apt"
	cfgInst "github.com/hmwassim/debforge/internal/domain/installer/config"
	debInst "github.com/hmwassim/debforge/internal/domain/installer/deb"
	srcInst "github.com/hmwassim/debforge/internal/domain/installer/source"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/self"
	"github.com/hmwassim/debforge/internal/service"
)

var version = buildmeta.DefaultVersion

func main() {
	os.Exit(run())
}

func run() int {
	ctx := context.Background()

	yesMode, forceMode, args := parseFlags(os.Args[1:])

	ui := ui.NewConsoleUI()
	if yesMode {
		ui.SetYes(true)
	}

	if len(args) == 0 {
		usage()
		return 0
	}

	cfg := self.DefaultConfig()
	fsys := fs.NewFileSystem()
	runner := exec.NewRunner()
	locker := lock.NewNoop()

	switch args[0] {
	case "--version":
		fmt.Println("debforge " + version)
		return 0

	case "--self-update":
		updater := self.NewUpdater(cfg, runner, fsys, ui, locker)
		if err := updater.Update(ctx); err != nil {
			ui.Error("%s", err)
			return 1
		}
		return 0

	case "--help":
		usage()
		return 0

	case "--self-remove":
		reg, instReg, stateSvc, err := bootstrap(cfg, fsys, runner)
		if err != nil {
			ui.Error("bootstrap: %s", err)
			return 1
		}
		remover := self.NewRemover(cfg, runner, fsys, ui, locker, reg, instReg, stateSvc)
		if err := remover.Remove(ctx); err != nil {
			ui.Error("%s", err)
			return 1
		}
		return 0
	}

	reg, instReg, stateSvc, err := bootstrap(cfg, fsys, runner)
	if err != nil {
		ui.Error("bootstrap: %s", err)
		return 1
	}

	switch args[0] {
	case "install":
		names := args[1:]
		if len(names) == 0 {
			usage()
			return 1
		}
		svc := service.NewInstallService(reg, instReg, service.NewResolver(reg), stateSvc, locker, cfg.LockPath)
		return withConfirm(ctx, ui, func(spinner ports.Spinner) error {
			return svc.Run(ctx, names, forceMode, spinner)
		})

	case "remove":
		if len(args) < 2 {
			usage()
			return 1
		}
		svc := service.NewRemoveService(reg, instReg, stateSvc, locker, cfg.LockPath)
		return withConfirm(ctx, ui, func(spinner ports.Spinner) error {
			return svc.Run(ctx, args[1:], spinner)
		})

	case "update":
		if len(args) < 2 {
			usage()
			return 1
		}
		svc := service.NewInstallService(reg, instReg, service.NewResolver(reg), stateSvc, locker, cfg.LockPath)
		return withConfirm(ctx, ui, func(spinner ports.Spinner) error {
			return svc.Update(ctx, args[1:], spinner)
		})

	default:
		usage()
	}
	return 0
}

func parseFlags(args []string) (yes, force bool, rest []string) {
	for _, a := range args {
		switch a {
		case "-y", "--yes":
			yes = true
		case "-f", "--force":
			force = true
		default:
			rest = append(rest, a)
		}
	}
	return
}

// bootstrap wires the package registry, installer registry, and state
// manager shared by every package-management command (install/remove/
// update) and by --self-remove. cfg is the single source of truth for
// every on-disk path involved (see internal/self.Config), and runner is
// the one CommandRunner instance shared by every installer that needs to
// shell out.
func bootstrap(cfg *self.Config, fsys ports.FileSystem, runner ports.CommandRunner) (*pkg.Registry, *installer.Registry, *service.StateManager, error) {
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()

	instReg.Register(pkg.TypeApt, aptInst.NewInstaller(runner, fsys))
	instReg.Register(pkg.TypeDeb, debInst.NewInstaller(runner, fsys))
	instReg.Register(pkg.TypeSource, srcInst.NewInstaller(runner, fsys))
	instReg.Register(pkg.TypeConfig, cfgInst.NewInstaller(runner, fsys))

	st := store.NewStore[service.State](fsys, cfg.StatePath)
	stateSvc := service.NewStateManager(st)
	if _, err := stateSvc.Load(); err != nil && !os.IsNotExist(err) {
		return nil, nil, nil, fmt.Errorf("load state: %w", err)
	}

	return reg, instReg, stateSvc, nil
}

func withConfirm(ctx context.Context, ui ports.UI, fn func(ports.Spinner) error) int {
	if !ui.Prompt("Continue?") {
		ui.Info("Cancelled")
		return 0
	}
	spinner := ui.Spinner(ctx, "Working")
	if err := fn(spinner); err != nil {
		spinner.Fail()
		ui.Error("%s", err)
		return 1
	}
	spinner.Done()
	return 0
}

func usage() {
	fmt.Println("debforge - package manager")
	fmt.Println()
	fmt.Println("Usage: debforge [flags] <command>")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("    -y, --yes           Skip confirmation prompts")
	fmt.Println("    -f, --force         Force operation (reinstall)")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("    --self-update       Update debforge")
	fmt.Println("    --self-remove       Remove debforge")
	fmt.Println("    --help              Show this help")
	fmt.Println("    --version           Show version")
}
