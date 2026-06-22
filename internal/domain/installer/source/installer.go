package source

import (
	"context"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

type Installer struct{}

func NewInstaller(ports.CommandRunner, ports.FileSystem) *Installer {
	return &Installer{}
}

func (i *Installer) Install(_ context.Context, _ *pkg.Package, _ ports.Spinner) error {
	return installer.StubError("source")
}

func (i *Installer) Remove(_ context.Context, _ *pkg.Package, _ ports.Spinner) error {
	return installer.StubError("source")
}
