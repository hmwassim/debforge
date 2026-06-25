package aptpty

import (
	"context"

	"github.com/hmwassim/debforge/internal/dpkg"
	"github.com/hmwassim/debforge/internal/ports"
)

func IsPackageInstalled(ctx context.Context, runner ports.CommandRunner, name string) bool {
	return dpkg.IsInstalled(ctx, runner, name)
}

func FindInstalledConflicts(ctx context.Context, runner ports.CommandRunner, names []string) []string {
	var found []string
	for _, name := range names {
		if IsPackageInstalled(ctx, runner, name) {
			found = append(found, name)
		}
	}
	return found
}
