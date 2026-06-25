package service

import (
	"context"

	"github.com/hmwassim/debforge/internal/ports"
)

// Run installs the named packages.
//
// When force is true both the force and rerun parameters are set on
// processAll: ForceInstall is propagated to every dependency (overriding
// installer-level version short-circuits) and the system-installed check
// is bypassed, guaranteeing a full reinstall of the entire tree.
func (s *InstallService) Run(ctx context.Context, names []string, force bool, spinner ports.Spinner) error {
	return withState(ctx, s.locker, s.lockPath, s.state, func(st *State) error {
		return s.processAll(ctx, names, force, force, st, spinner, "install", "installed")
	})
}
