package aptpty

import (
	"context"
	"strings"

	"github.com/hmwassim/debforge/internal/ports"
)

func IsPackageInstalled(ctx context.Context, runner ports.CommandRunner, name string) bool {
	out, _, err := runner.Run(ctx, "dpkg-query", "-W", "-f=${db:Status-Status}\n", name)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "installed"
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
