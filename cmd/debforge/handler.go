package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"

	"github.com/hmwassim/debforge/internal/adapters/apt"
	adpDpkg "github.com/hmwassim/debforge/internal/adapters/dpkg"
	adpExtrepo "github.com/hmwassim/debforge/internal/adapters/extrepo"
	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/definition"
	"github.com/hmwassim/debforge/internal/domain/installer"
	aptInst "github.com/hmwassim/debforge/internal/domain/installer/apt"
	configInst "github.com/hmwassim/debforge/internal/domain/installer/config"
	debInst "github.com/hmwassim/debforge/internal/domain/installer/deb"
	sourceInst "github.com/hmwassim/debforge/internal/domain/installer/source"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/dpkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/self"
	"github.com/hmwassim/debforge/internal/service"
)

var isTerminal = term.IsTerminal
var lookPath = exec.LookPath

type commandHandler struct {
	reg      *pkg.Registry
	instReg  *installer.Registry
	stateSvc *service.StateManager
	locker   ports.Locker
	cfg      *self.Config
	runner   ports.CommandRunner
	fsys     ports.FileSystem
	sys      ports.System
	aptUpd   ports.AptUpdater
	extrepo  ports.ExtrepoManager
	pkgList  ports.PackageLister
}

func newHandler(cfg *self.Config, fsys ports.FileSystem, runner ports.CommandRunner, locker ports.Locker, ui ports.UI, sys ports.System) (*commandHandler, error) {
	runner = dpkg.NewCachedRunner(runner)
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()

	instReg.Register(pkg.TypeApt, aptInst.NewInstaller(runner, fsys, ui, sys))
	instReg.Register(pkg.TypeDeb, debInst.NewInstaller(runner, fsys, ui, sys))
	instReg.Register(pkg.TypeSource, sourceInst.NewInstaller(runner, fsys, ui))
	instReg.Register(pkg.TypeConfig, configInst.NewInstaller(runner, fsys, ui, sys))

	if err := definition.LoadAll(cfg.PkgsDir, fsys, reg); err != nil {
		return nil, fmt.Errorf("load definitions: %w", err)
	}

	st := store.NewStore[service.State](fsys, cfg.StatePath)
	stateSvc := service.NewStateManager(st)
	if _, err := stateSvc.Load(); err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}

	return &commandHandler{
		reg: reg, instReg: instReg, stateSvc: stateSvc,
		locker: locker, cfg: cfg, runner: runner, fsys: fsys, sys: sys,
		aptUpd:  &apt.Updater{Runner: runner},
		extrepo: &adpExtrepo.Manager{Runner: runner, Fs: fsys},
		pkgList: &adpDpkg.Lister{Runner: runner},
	}, nil
}

func selectPager() (cmd string, args []string) {
	envPager := os.Getenv("PAGER")
	if envPager != "" {
		parts := strings.Fields(envPager)
		cmd = parts[0]
		if len(parts) > 1 {
			args = parts[1:]
		}
		return
	}
	if p, err := lookPath("less"); err == nil {
		return p, []string{"-FRSX"}
	}
	return "", nil
}

func displayWithPager(out string) int {
	if !isTerminal(int(os.Stdout.Fd())) {
		fmt.Print(out)
		return 0
	}
	pagerCmd, pagerArgs := selectPager()
	if pagerCmd == "" {
		fmt.Print(out)
		return 0
	}
	cmd := exec.Command(pagerCmd, pagerArgs...)
	cmd.Stdin = strings.NewReader(out)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Print(out)
	}
	return 0
}

func withConfirm(ctx context.Context, u ports.UI, fn func(ports.Spinner) error) int {
	if !u.Prompt("Continue?") {
		u.Info("Cancelled")
		return 0
	}
	spinner := u.Spinner(ctx, "Processing")
	defer spinner.Stop()
	if err := fn(spinner); err != nil {
		if !errors.Is(err, service.ErrNotInstalled) {
			u.Error("%s", err)
		}
		return 1
	}
	return 0
}
