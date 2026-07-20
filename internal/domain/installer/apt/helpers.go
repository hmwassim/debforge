package apt

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/dpkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// DefaultBackportSuite is the default suite used for backport installations
// when the package definition does not specify one.
const DefaultBackportSuite = "trixie-backports"

// FindInstalledConflicts returns the subset of names that are currently
// installed according to dpkg-query.
func FindInstalledConflicts(ctx context.Context, runner ports.CommandRunner, names []string) ([]string, error) {
	found := make([]string, 0, len(names))
	for _, name := range names {
		ok, err := dpkg.IsInstalled(ctx, runner, name)
		if err != nil {
			return nil, fmt.Errorf("check %q: %w", name, err)
		}
		if ok {
			found = append(found, name)
		}
	}
	return found, nil
}
