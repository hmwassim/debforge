package commands

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/apt"
	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/services/state"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/coreservices"
)

type UpdateCommand struct {
	installSvc *services.InstallService
	pkgReg     *pkg.Registry
	stateSvc   *state.Service
	aptSvc     apt.Service
	ui         ports.UI
}

func NewUpdateCommand(installSvc *services.InstallService, pkgReg *pkg.Registry, stateSvc *state.Service, aptSvc apt.Service, ui ports.UI) *UpdateCommand {
	return &UpdateCommand{
		installSvc: installSvc,
		pkgReg:     pkgReg,
		stateSvc:   stateSvc,
		aptSvc:     aptSvc,
		ui:         ui,
	}
}

func (c *UpdateCommand) Name() string { return "update" }

func (c *UpdateCommand) Usage() string { return "Update specific packages or all" }

func (c *UpdateCommand) Run(ctx context.Context, args []string) error {
	all := false
	var names []string
	for _, a := range args {
		if a == "--all" {
			all = true
		} else {
			names = append(names, a)
		}
	}
	if all {
		return withSpinner(ctx, c.ui, "Updating all packages...", func(spinner ports.Spinner) error {
			return c.updateAll(ctx, spinner)
		})
	}
	if len(names) == 0 {
		return fmt.Errorf("update requires a package name or --all")
	}
	return withSpinner(ctx, c.ui, fmt.Sprintf("Updating %s...", names[0]), func(spinner ports.Spinner) error {
		if _, ok := c.pkgReg.Lookup(names[0]); !ok {
			return fmt.Errorf("unknown package: %s", names[0])
		}
		st, err := c.stateSvc.Load()
		if err != nil {
			return err
		}
		if _, ok := st.Packages[names[0]]; !ok {
			spinner.SetDesc(names[0] + " is not installed")
			spinner.DoneWarn()
			return nil
		}
		oldVersion := st.Packages[names[0]].Version
		if err := c.installSvc.UpdateSingle(ctx, names[0], spinner); err != nil {
			return err
		}
		st, err = c.stateSvc.Load()
		if err != nil {
			return err
		}
		if st.Packages[names[0]].Version != oldVersion {
			spinner.SetDesc(names[0] + " updated")
		} else {
			spinner.SetDesc(names[0] + " already up to date")
		}
		return nil
	})
}

func (c *UpdateCommand) updateAll(ctx context.Context, spinner ports.Spinner) error {
	spinner.SetDesc("Updating package lists")
	if err := c.aptSvc.Update(ctx); err != nil {
		return err
	}
	spinner.SetDesc("Upgrading system packages")
	if err := c.aptSvc.Upgrade(ctx); err != nil {
		return err
	}
	st, err := c.stateSvc.Load()
	if err != nil {
		return err
	}
	for name, entry := range st.Packages {
		if _, ok := c.pkgReg.Lookup(name); !ok {
			continue
		}
		if entry.Type != "deb" && entry.Type != "source" {
			continue
		}
		spinner.SetDesc("Updating " + name)
		if err := c.installSvc.UpdateSingle(ctx, name, spinner); err != nil {
			return fmt.Errorf("updating %s: %w", name, err)
		}
	}
	spinner.SetDesc("All packages updated")
	return nil
}
