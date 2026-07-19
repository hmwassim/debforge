// Package extrepo provides an adapter that satisfies the ports.ExtrepoManager
// interface by delegating to the domain extrepo package.
package extrepo

import (
	"context"

	domext "github.com/hmwassim/debforge/internal/domain/installer/extrepo"
	"github.com/hmwassim/debforge/internal/ports"
)

// Manager satisfies ports.ExtrepoManager by delegating to the domain
// extrepo functions with bound dependencies.
type Manager struct {
	Runner ports.CommandRunner
	Fs     ports.FileSystem
}

// NeedsEnable reports whether the given extrepo source file needs to be enabled.
func (m *Manager) NeedsEnable(ctx context.Context, repo string) (bool, error) {
	return domext.NeedsEnable(ctx, repo, m.Fs)
}

// Enable runs extrepo enable for the given repo.
func (m *Manager) Enable(ctx context.Context, repo string, spinner ports.Spinner) error {
	return domext.Enable(ctx, repo, m.Runner, spinner)
}
