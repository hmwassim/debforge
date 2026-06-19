package installers

import (
	"context"

	"github.com/hmwassim/debforge/internal/domain/package"
)

type Installer interface {
	Install(ctx context.Context, pkg *pkg.Package) error
	Remove(ctx context.Context, pkg *pkg.Package) error
	Update(ctx context.Context, pkg *pkg.Package) error
}
