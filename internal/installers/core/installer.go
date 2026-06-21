package core

import (
	"context"

	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/installers"
	"github.com/hmwassim/debforge/internal/ports"
)

type Installer struct {
	runner ports.CommandRunner
	logger ports.UI
	locker ports.Locker
}

func NewInstaller(runner ports.CommandRunner, logger ports.UI, locker ports.Locker) *Installer {
	return &Installer{runner: runner, logger: logger, locker: locker}
}

func (i *Installer) Install(ctx context.Context, pkg *pkg.Package, _ ports.Spinner) error {
	i.logger.Info("Core packages are managed via 'debforge core setup'")
	return nil
}

func (i *Installer) Remove(ctx context.Context, pkg *pkg.Package, _ ports.Spinner) error {
	i.logger.Info("Core packages cannot be removed individually")
	return nil
}

func (i *Installer) Update(ctx context.Context, pkg *pkg.Package, spinner ports.Spinner) error {
	return i.Install(ctx, pkg, spinner)
}

var _ installers.Installer = (*Installer)(nil)
