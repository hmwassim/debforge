package apt

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/aptpty"
)

type Installer struct{}

func NewInstaller() *Installer {
	return &Installer{}
}

func (i *Installer) Install(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if p.Type != pkg.TypeApt {
		return fmt.Errorf("apt installer called for type %s", p.Type)
	}
	if len(p.Packages) == 0 {
		return fmt.Errorf("no packages defined for apt type")
	}
	return aptpty.RunInstall(ctx, p.Packages, spinner)
}

func (i *Installer) Remove(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if p.Type != pkg.TypeApt {
		return fmt.Errorf("apt installer called for type %s", p.Type)
	}
	if len(p.Packages) == 0 {
		return nil
	}
	return aptpty.RunRemove(ctx, p.Packages, spinner)
}

func (i *Installer) Update(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	if p.Type != pkg.TypeApt {
		return fmt.Errorf("apt installer called for type %s", p.Type)
	}
	if len(p.Packages) == 0 {
		return nil
	}
	return aptpty.RunInstall(ctx, p.Packages, spinner)
}
