package installer

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// Installer handles installation and removal of a specific package type.
type Installer interface {
	Install(ctx context.Context, pkg *pkg.Package, spinner ports.Spinner) error
	Remove(ctx context.Context, pkg *pkg.Package, spinner ports.Spinner) error
}

// AssertType returns an error if typ does not match expected, using name in
// the error message to identify the installer.
func AssertType(typ, expected pkg.Type, name string) error {
	if typ != expected {
		return fmt.Errorf("%s installer called for type %s", name, typ)
	}
	return nil
}
