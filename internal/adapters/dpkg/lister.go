// Package dpkg provides an adapter that satisfies the ports.PackageLister
// interface by delegating to the dpkg package.
package dpkg

import (
	"context"

	"github.com/hmwassim/debforge/internal/dpkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// Lister satisfies ports.PackageLister by running dpkg-query.
type Lister struct {
	Runner ports.CommandRunner
}

// ListInstalled returns all system packages currently installed via dpkg.
func (l *Lister) ListInstalled(ctx context.Context) (map[string]bool, error) {
	return dpkg.ListInstalled(ctx, l.Runner)
}
