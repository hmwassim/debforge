package commands

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/coreservices"
)

type InstallCommand struct {
	svc         services.PackageInstaller
	pkgRegistry *pkg.Registry
	ui          ports.UI
}

func NewInstallCommand(svc services.PackageInstaller, pkgRegistry *pkg.Registry, ui ports.UI) *InstallCommand {
	return &InstallCommand{svc: svc, pkgRegistry: pkgRegistry, ui: ui}
}

func (c *InstallCommand) Name() string { return "install" }

func (c *InstallCommand) Usage() string {
	return "Install packages from external repos"
}

func (c *InstallCommand) Run(ctx context.Context, args []string) error {
	var names []string
	force := false
	for _, a := range args {
		if a == "-f" || a == "--force" {
			force = true
		} else {
			names = append(names, a)
		}
	}
	if len(names) == 0 {
		return fmt.Errorf("install requires a package name")
	}
	variants := make(map[string]string)
	for _, name := range names {
		p, ok := c.pkgRegistry.Lookup(name)
		if !ok {
			return fmt.Errorf("unknown package: %s", name)
		}
		if len(p.Variants) > 0 {
			variant := PromptVariant(c.ui, p.Variants)
			if variant == "" {
				return nil
			}
			variants[name] = variant
		}
	}
	return withSpinner(ctx, c.ui, fmt.Sprintf("Installing %s...", names[0]), func(spinner ports.Spinner) error {
		return c.svc.Install(ctx, names, variants, force, spinner)
	})
}
