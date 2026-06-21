package config

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

type Installer struct {
	fs ports.FileSystem
}

func NewInstaller(fs ports.FileSystem) *Installer {
	return &Installer{fs: fs}
}

func (i *Installer) Install(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	return fmt.Errorf("config installer: not implemented")
}

func (i *Installer) Remove(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	return fmt.Errorf("config installer: not implemented")
}
