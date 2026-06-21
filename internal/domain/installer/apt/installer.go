package apt

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

type Installer struct {
	runner ports.CommandRunner
	fs     ports.FileSystem
}

func NewInstaller(runner ports.CommandRunner, fs ports.FileSystem) *Installer {
	return &Installer{runner: runner, fs: fs}
}

func (i *Installer) Install(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if p.Type != pkg.TypeApt {
		return fmt.Errorf("apt installer called for type %s", p.Type)
	}
	if len(p.Packages) == 0 {
		return fmt.Errorf("no packages defined for apt type")
	}
	return aptpty.RunInstall(ctx, i.runner, p.Packages, spinner)
}

func (i *Installer) Remove(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if p.Type != pkg.TypeApt {
		return fmt.Errorf("apt installer called for type %s", p.Type)
	}
	if len(p.Packages) == 0 {
		return nil
	}
	return aptpty.RunRemove(ctx, i.runner, p.Packages, spinner)
}
