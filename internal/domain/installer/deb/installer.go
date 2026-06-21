package deb

import (
	"context"
	"fmt"

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
	return fmt.Errorf("deb installer: not implemented")
}

func (i *Installer) Remove(ctx context.Context, p *pkg.Package, spinner ports.Spinner) error {
	return fmt.Errorf("deb installer: not implemented")
}
