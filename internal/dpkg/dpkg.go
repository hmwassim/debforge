// Package dpkg provides helpers for querying the dpkg package database.
package dpkg

import (
	"context"
	"errors"
	"strings"

	"github.com/hmwassim/debforge/internal/ports"
)

// IsInstalled checks whether a dpkg-managed package is in the installed state.
// A context error is propagated so callers can distinguish cancellation from a
// genuinely absent package. Non-context command failures are treated
// conservatively as "not installed".
func IsInstalled(ctx context.Context, runner ports.CommandRunner, name string) (bool, error) {
	out, _, err := runner.Run(ctx, "dpkg-query", "-W", "-f=${db:Status-Status}\n", name)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false, err
		}
		return false, nil
	}
	return strings.TrimSpace(string(out)) == "installed", nil
}

// ListInstalled returns a set of all currently installed package names known
// to dpkg.
func ListInstalled(ctx context.Context, runner ports.CommandRunner) (map[string]bool, error) {
	out, _, err := runner.Run(ctx, "dpkg-query", "-W", "-f", "${Package}\n")
	if err != nil {
		return nil, err
	}
	// A typical Trixie install has ~2000-3000 packages; 2500 avoids ~12 reallocs.
	installed := make(map[string]bool, 2500)
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		installed[line] = true
	}
	return installed, nil
}
