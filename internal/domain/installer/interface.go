package installer

import (
	"context"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// Installer handles installation and removal of a specific package type.
type Installer interface {
	Install(ctx context.Context, pkg *pkg.Package, spinner ports.Spinner) error
	Remove(ctx context.Context, pkg *pkg.Package, spinner ports.Spinner) error
}
