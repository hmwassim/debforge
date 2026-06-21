package installer

import (
	"context"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

type Installer interface {
	Install(ctx context.Context, pkg *pkg.Package, spinner ports.Spinner) error
	Remove(ctx context.Context, pkg *pkg.Package, spinner ports.Spinner) error
}
