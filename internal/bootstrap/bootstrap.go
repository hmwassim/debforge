package bootstrap

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	adapterexec "github.com/hmwassim/debforge/internal/adapters/exec"
	adapterfs "github.com/hmwassim/debforge/internal/adapters/fs"
	adapterhttp "github.com/hmwassim/debforge/internal/adapters/http"
	adapterlock "github.com/hmwassim/debforge/internal/adapters/lock"
	adaptersui "github.com/hmwassim/debforge/internal/adapters/ui"
	config "github.com/hmwassim/debforge/internal/config"

	"github.com/hmwassim/debforge/internal/coresetup"
	"github.com/hmwassim/debforge/internal/domain/apt"
	"github.com/hmwassim/debforge/internal/domain/deployer"
	domainrepo "github.com/hmwassim/debforge/internal/domain/repo"
	"github.com/hmwassim/debforge/internal/commands"
	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/installers"
	coreinstaller "github.com/hmwassim/debforge/internal/installers/core"
	debinstaller "github.com/hmwassim/debforge/internal/installers/deb"
	sourceinstaller "github.com/hmwassim/debforge/internal/installers/source"
	"github.com/hmwassim/debforge/internal/services/dependency"
	"github.com/hmwassim/debforge/internal/services/self"
	"github.com/hmwassim/debforge/internal/services/state"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/coreservices"
)

func Main() int {
	app, err := newApp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return app.Run(os.Args[1:])
}

type App struct {
	ui     ports.UI
	cmdReg *commands.Registry
}

func newApp() (*App, error) {
	cfg := config.NewDefaultConfig()
	runner := adapterexec.NewOSCommandRunner()
	httpClient := adapterhttp.NewHTTPClient()
	fs := adapterfs.NewOSFileSystem()
	locker := adapterlock.NewFlockLocker()
	ui := adaptersui.NewConsoleUI()

	pkgRegistry := pkg.NewRegistry()
	loader := domainrepo.NewLoader(fs, ui)

	packagesDir := filepath.Join(cfg.RootDir, "data", "packages")
	if _, err := fs.Stat(packagesDir); os.IsNotExist(err) {
		packagesDir = "data/packages"
	}
	if err := loader.LoadFromDir(packagesDir, pkgRegistry); err != nil {
		ui.Warn("loading packages: %v", err)
	}

	aptSvc := apt.NewService(runner, ui)
	configDeployer := deployer.NewDeployer(fs, runner, ui)

	debInst := debinstaller.NewInstallerWithTempDir(runner, httpClient, ui, configDeployer, fs, cfg.TempDir)
	sourceInst := sourceinstaller.NewInstallerWithTempDir(runner, ui, fs, cfg.TempDir)
	coreInst := coreinstaller.NewInstaller(runner, ui, locker)

	instReg := installers.NewRegistry()
	repoMgr := installers.NewRepoManager(aptSvc, runner, fs, httpClient, ui)
	instReg.Register(pkg.TypeApt, installers.NewAptInstaller(aptSvc, configDeployer, repoMgr, ui))
	instReg.Register(pkg.TypeDeb, debInst)
	instReg.Register(pkg.TypeSource, sourceInst)
	instReg.Register(pkg.TypeConfig, installers.NewConfigInstaller(configDeployer, fs, ui))
	instReg.Register(pkg.TypeCore, coreInst)

	groups := coresetup.NewGroups()

	stateSvc := state.NewService(fs, cfg.StatesDir)
	resolver := dependency.NewResolver(pkgRegistry)

	updater := self.NewUpdater(runner, locker, ui, fs, cfg)
	selfRemover := self.NewRemover(runner, locker, ui, fs, pkgRegistry, instReg, stateSvc, aptSvc, cfg)

	setupSvc := services.NewSetupService(aptSvc, configDeployer, ui, locker, fs, runner, httpClient, cfg)
	listSvc := services.NewListService(pkgRegistry, stateSvc, ui, aptSvc)
	lockPath := cfg.LockFile()
	installSvc := services.NewInstallService(pkgRegistry, instReg, stateSvc, resolver, ui, locker, lockPath)
	updateSvc := services.NewUpdateService(updater, ui)
	selfRemoveSvc := services.NewSelfRemoveService(selfRemover)
	removePkgSvc := services.NewRemoveService(pkgRegistry, instReg, stateSvc, ui, locker, lockPath)

	cmdReg := commands.NewRegistry()
	cmdReg.Register(commands.NewInstallCommand(installSvc, pkgRegistry, ui))
	cmdReg.Register(commands.NewRemoveCommand(removePkgSvc, pkgRegistry, ui))
	cmdReg.Register(commands.NewUpdateCommand(installSvc, pkgRegistry, stateSvc, aptSvc, ui))
	cmdReg.Register(commands.NewListCommand(listSvc))
	cmdReg.Register(commands.NewSearchCommand(listSvc))
	cmdReg.Register(commands.NewCoreCommand(setupSvc, listSvc, groups, ui))
	cmdReg.Register(commands.NewSelfUpdateCommand(updateSvc, ui))
	cmdReg.Register(commands.NewSelfRemoveCommand(selfRemoveSvc, ui))

	return &App{ui: ui, cmdReg: cmdReg}, nil
}

func (a *App) Run(args []string) int {
	if len(args) == 0 {
		fmt.Print(a.cmdReg.Help())
		return 0
	}

	name := args[0]
	switch name {
	case "--version", "-V":
		fmt.Printf("debforge %s\n", commands.Version)
		return 0
	case "--help", "-h":
		fmt.Print(a.cmdReg.Help())
		return 0
	case "--self-update":
		name = "self-update"
	case "--self-remove":
		name = "self-remove"
	}

	for _, arg := range args[1:] {
		if arg == "-h" || arg == "--help" {
			cmd, ok := a.cmdReg.Lookup(name)
			if !ok {
				fmt.Fprintf(os.Stderr, "error: unknown command: %s\n", name)
				return 1
			}
			fmt.Printf("Usage: debforge %s %s\n", name, cmd.Usage())
			return 0
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
		signal.Stop(sigCh)
	}()

	cmd, ok := a.cmdReg.Lookup(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "error: unknown command: %s\n", args[0])
		fmt.Print(a.cmdReg.Help())
		return 1
	}
	if err := cmd.Run(ctx, args[1:]); err != nil {
		a.ui.Error("%v", err)
		return 1
	}
	return 0
}
