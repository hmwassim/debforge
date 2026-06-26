package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hmwassim/debforge/internal/adapters/store"
	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/definition"
	"github.com/hmwassim/debforge/internal/domain/installer"
	aptInst "github.com/hmwassim/debforge/internal/domain/installer/apt"
	configInst "github.com/hmwassim/debforge/internal/domain/installer/config"
	debInst "github.com/hmwassim/debforge/internal/domain/installer/deb"
	sourceInst "github.com/hmwassim/debforge/internal/domain/installer/source"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/self"
	"github.com/hmwassim/debforge/internal/service"
)

func runInstall(ctx context.Context, u ports.UI, reg *pkg.Registry, instReg *installer.Registry, stateSvc *service.StateManager, locker ports.Locker, cfg *self.Config, runner ports.CommandRunner, fsys ports.FileSystem, names []string, forceMode bool) int {
	svc := service.NewInstallService(reg, instReg, service.NewResolver(reg), stateSvc, locker, cfg.LockPath, runner, fsys)

	var conflicts []string
	for _, name := range names {
		p, ok := reg.Lookup(name)
		if !ok {
			continue
		}
		if p.Apt != nil {
			conflicts = append(conflicts, aptpty.FindInstalledConflicts(ctx, runner, p.Apt.Conflicts)...)
		}
	}
	if len(conflicts) > 0 {
		u.Info("Conflicting package(s) installed: %s", strings.Join(conflicts, ", "))
	}

	if err := svc.SelectVariants(ctx, names); err != nil {
		u.Error("%s", err)
		return 1
	}

	return withConfirm(ctx, u, func(spinner ports.Spinner) error {
		return svc.Run(ctx, names, forceMode, spinner)
	})
}

func runRemove(ctx context.Context, u ports.UI, reg *pkg.Registry, instReg *installer.Registry, stateSvc *service.StateManager, locker ports.Locker, cfg *self.Config, runner ports.CommandRunner, fsys ports.FileSystem, names []string) int {
	svc := service.NewRemoveService(reg, instReg, stateSvc, locker, cfg.LockPath, runner, fsys)
	return withConfirm(ctx, u, func(spinner ports.Spinner) error {
		return svc.Run(ctx, names, spinner)
	})
}

func runUpdate(ctx context.Context, u ports.UI, reg *pkg.Registry, instReg *installer.Registry, stateSvc *service.StateManager, locker ports.Locker, cfg *self.Config, runner ports.CommandRunner, fsys ports.FileSystem, names []string, forceMode, allMode bool) int {
	svc := service.NewInstallService(reg, instReg, service.NewResolver(reg), stateSvc, locker, cfg.LockPath, runner, fsys)
	return withConfirm(ctx, u, func(spinner ports.Spinner) error {
		if err := aptpty.RunUpdate(ctx, runner, spinner); err != nil {
			return err
		}
		if allMode {
			if err := aptpty.RunUpgrade(ctx, runner, spinner); err != nil {
				return err
			}
		}
		return svc.Update(ctx, names, forceMode, allMode, spinner)
	})
}

func extractFlags(ss []string, yes, force, all *bool) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		switch s {
		case "-y", "--yes":
			*yes = true
		case "-f", "--force":
			*force = true
		case "-a", "--all":
			*all = true
		default:
			out = append(out, s)
		}
	}
	return out
}

func bootstrap(cfg *self.Config, fsys ports.FileSystem, runner ports.CommandRunner, ui ports.UI) (*pkg.Registry, *installer.Registry, *service.StateManager, error) {
	reg := pkg.NewRegistry()
	instReg := installer.NewRegistry()

	instReg.Register(pkg.TypeApt, aptInst.NewInstaller(runner, fsys, ui))
	instReg.Register(pkg.TypeDeb, debInst.NewInstaller(runner, fsys, ui))
	instReg.Register(pkg.TypeSource, sourceInst.NewInstaller(runner, fsys, ui))
	instReg.Register(pkg.TypeConfig, configInst.NewInstaller(runner, fsys, ui))

	if err := definition.LoadAll(cfg.PkgsDir, fsys, reg); err != nil {
		return nil, nil, nil, fmt.Errorf("load definitions: %w", err)
	}

	st := store.NewStore[service.State](fsys, cfg.StatePath)
	stateSvc := service.NewStateManager(st)
	if _, err := stateSvc.Load(); err != nil {
		return nil, nil, nil, fmt.Errorf("load state: %w", err)
	}

	return reg, instReg, stateSvc, nil
}

func loadYAMLDefinitions(reg *pkg.Registry, names []string, fsys ports.FileSystem) error {
	for i, n := range names {
		if !strings.HasSuffix(n, ".yaml") {
			continue
		}
		p, err := definition.Parse(n, fsys)
		if err != nil {
			return fmt.Errorf("load %s: %w", n, err)
		}
		reg.Register(p)
		names[i] = p.Name
	}
	return nil
}

func loadDefs(reg *pkg.Registry, names []string, fsys ports.FileSystem, u ports.UI) bool {
	if err := loadYAMLDefinitions(reg, names, fsys); err != nil {
		u.Error("%s", err)
		return false
	}
	return true
}

func withConfirm(ctx context.Context, u ports.UI, fn func(ports.Spinner) error) int {
	if !u.Prompt("Continue?") {
		u.Info("Cancelled")
		return 0
	}
	spinner := u.Spinner(ctx, "Working")
	if err := fn(spinner); err != nil {
		if !errors.Is(err, service.ErrNotInstalled) {
			u.Error("%s", err)
		}
		return 1
	}
	return 0
}
