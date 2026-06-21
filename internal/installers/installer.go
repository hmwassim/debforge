package installers

import (
	"context"

	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/ports"
)

type Installer interface {
	Install(ctx context.Context, pkg *pkg.Package, spinner ports.Spinner) error
	Remove(ctx context.Context, pkg *pkg.Package, spinner ports.Spinner) error
	Update(ctx context.Context, pkg *pkg.Package, spinner ports.Spinner) error
}
