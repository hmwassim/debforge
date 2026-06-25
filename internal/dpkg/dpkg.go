package dpkg

import (
	"context"
	"strings"

	"github.com/hmwassim/debforge/internal/ports"
)

// IsInstalled checks whether a dpkg-managed package is in the installed state.
func IsInstalled(ctx context.Context, runner ports.CommandRunner, name string) bool {
	out, _, err := runner.Run(ctx, "dpkg-query", "-W", "-f=${db:Status-Status}\n", name)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "installed"
}

// ListInstalled returns a set of all currently installed package names known
// to dpkg.
func ListInstalled(ctx context.Context, runner ports.CommandRunner) (map[string]bool, error) {
	out, _, err := runner.Run(ctx, "dpkg-query", "-W", "-f", "${Package}\n")
	if err != nil {
		return nil, err
	}
	installed := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		installed[line] = true
	}
	return installed, nil
}
