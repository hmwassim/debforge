package commands

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/coreservices"
)

type RemoveCommand struct {
	svc         services.PackageRemover
	pkgRegistry *pkg.Registry
	ui          ports.UI
}

func NewRemoveCommand(svc services.PackageRemover, pkgRegistry *pkg.Registry, ui ports.UI) *RemoveCommand {
	return &RemoveCommand{svc: svc, pkgRegistry: pkgRegistry, ui: ui}
}

func (c *RemoveCommand) Name() string { return "remove" }

func (c *RemoveCommand) Usage() string { return "Remove packages and repo sources" }

func (c *RemoveCommand) Run(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("remove requires a package name")
	}
	confirmed := make([]string, 0, len(args))
	for _, name := range args {
		p, ok := c.pkgRegistry.Lookup(name)
		if !ok {
			return fmt.Errorf("unknown package: %s", name)
		}
		if p.Type == pkg.TypeApt || p.Type == pkg.TypeConfig || p.Type == pkg.TypeCore {
			c.ui.Info("Removing %s", name)
			if !c.ui.Prompt("Continue?") {
				c.ui.Info("Cancelled")
				continue
			}
		}
		confirmed = append(confirmed, name)
	}
	return withSpinner(ctx, c.ui, fmt.Sprintf("Removing %s...", confirmed[0]), func(spinner ports.Spinner) error {
		return c.svc.Remove(ctx, confirmed, spinner)
	})
}
