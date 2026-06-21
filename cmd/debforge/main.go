package main

import (
	"context"
	"fmt"
	"os"

	"github.com/hmwassim/debforge/internal/adapters/exec"
	"github.com/hmwassim/debforge/internal/adapters/fs"
	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/adapters/ui"
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

var version = "0.1.0-dev"

func main() {
	os.Exit(run())
}

func run() int {
	ctx := context.Background()
	ui := ui.NewConsoleUI()

	if len(os.Args) < 2 {
		usage(ui)
		return 0
	}

	switch os.Args[1] {
	case "--version":
		fmt.Println("debforge " + version)
		return 0

	case "--self-update":
		runner := exec.NewRunner()
		fsys := fs.NewFileSystem()
		locker := &noopLocker{}
		cfg := self.DefaultConfig()
		updater := self.NewUpdater(cfg, runner, fsys, ui, locker)
		if err := updater.Update(ctx); err != nil {
			ui.Error("%s", err)
			return 1
		}
		return 0

	case "--self-remove":
		reg, instReg, stateSvc, locker, _, err := bootstrap(ui)
		if err != nil {
			ui.Error("bootstrap: %s", err)
			return 1
		}
		runner := exec.NewRunner()
		fsys := fs.NewFileSystem()
		cfg := self.DefaultConfig()
		remover := self.NewRemover(cfg, runner, fsys, ui, locker, reg, instReg, stateSvc)
		if err := remover.Remove(ctx); err != nil {
			ui.Error("%s", err)
			return 1
		}
		return 0
	}

	reg, instReg, stateSvc, locker, lockPath, err := bootstrap(ui)
	if err != nil {
		ui.Error("bootstrap: %s", err)
		return 1
	}

	spinner := ui.Spinner(ctx, "working")
	defer spinner.Done()

	switch os.Args[1] {
	case "install":
		svc := service.NewInstallService(reg, instReg, service.NewResolver(reg), stateSvc, locker, lockPath)
		force := false
		names := os.Args[2:]
		if len(names) > 0 && names[0] == "-f" {
			force = true
			names = names[1:]
		}
		if len(names) == 0 {
			usage(ui)
			return 1
		}
		if err := svc.Run(ctx, names, force, spinner); err != nil {
			spinner.Fail()
			ui.Error("%s", err)
			return 1
		}
		spinner.Done()

	case "remove":
		svc := service.NewRemoveService(reg, instReg, stateSvc, locker, lockPath)
		names := os.Args[2:]
		if len(names) == 0 {
			usage(ui)
			return 1
		}
		if err := svc.Run(ctx, names, spinner); err != nil {
			spinner.Fail()
			ui.Error("%s", err)
			return 1
		}
		spinner.Done()

	case "update":
		svc := service.NewInstallService(reg, instReg, service.NewResolver(reg), stateSvc, locker, lockPath)
		names := os.Args[2:]
		if len(names) == 0 {
			usage(ui)
			return 1
		}
		if err := svc.Update(ctx, names, spinner); err != nil {
			spinner.Fail()
			ui.Error("%s", err)
			return 1
		}
		spinner.Done()

	default:
		usage(ui)
	}
	return 0
}

func bootstrap(ui ports.UI) (*pkg.Registry, *installer.Registry, *service.StateManager, ports.Locker, string, error) {
	_ = exec.NewRunner()
	fsys := fs.NewFileSystem()

	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()

	instReg.Register(pkg.TypeApt, aptInst.NewInstaller())
	instReg.Register(pkg.TypeDeb, debInst.NewInstaller(nil, fsys))
	instReg.Register(pkg.TypeSource, srcInst.NewInstaller(nil, fsys))
	instReg.Register(pkg.TypeConfig, cfgInst.NewInstaller(fsys))

	statePath := "/var/lib/debforge/state.json"
	st := store.NewStore[service.State](statePath)
	stateSvc := service.NewStateManager(st)
	if _, err := stateSvc.Load(); err != nil && !os.IsNotExist(err) {
		return nil, nil, nil, nil, "", fmt.Errorf("load state: %w", err)
	}

	locker := &noopLocker{}
	lockPath := "/var/lib/debforge/lock"

	return reg, instReg, stateSvc, locker, lockPath, nil
}

type noopLocker struct{}

func (l *noopLocker) Acquire(ctx context.Context, path string) (func(), error) {
	return func() {}, nil
}

func usage(ui ports.UI) {
	ui.Info("debforge — package manager")
	ui.Info("")
	ui.Info("Usage:")
	ui.Info("  debforge install [-f] <name> [<name>...]")
	ui.Info("  debforge remove <name> [<name>...]")
	ui.Info("  debforge update <name> [<name>...]")
	ui.Info("  debforge --self-update")
	ui.Info("  debforge --self-remove")
	ui.Info("  debforge --version")
}
