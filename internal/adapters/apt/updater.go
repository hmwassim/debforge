// Package apt provides an adapter that satisfies the ports.AptUpdater
// interface by delegating to the aptpty pseudo-terminal runner.
package apt

import (
	"context"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/ports"
)

// Updater satisfies ports.AptUpdater by running apt-get update through
// the aptpty pseudo-terminal.
type Updater struct {
	Runner ports.CommandRunner
}

// RunUpdate runs apt-get update via the aptpty session.
func (u *Updater) RunUpdate(ctx context.Context, spinner ports.Spinner) error {
	return aptpty.RunUpdate(ctx, u.Runner, spinner)
}
