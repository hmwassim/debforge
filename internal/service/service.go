// Package service implements the high-level install, remove, and update
// workflows that coordinate package resolution, execution, and state
// persistence.
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/lockrun"
	"github.com/hmwassim/debforge/internal/ports"
)

// ErrNotInstalled is returned when attempting to remove or update a package
// that is not recorded in the state file.
var ErrNotInstalled = errors.New("not installed")

type baseService struct {
	reg      *pkg.Registry
	instReg  *installer.Registry
	state    *StateManager
	locker   ports.Locker
	lockPath string
	runner   ports.CommandRunner
	fs       ports.FileSystem
}

// InstallService orchestrates the installation of one or more packages
// along with their transitive dependencies.
type InstallService struct {
	baseService
	resolver *Resolver
}

// NewInstallService returns a new InstallService.
func NewInstallService(
	reg *pkg.Registry,
	instReg *installer.Registry,
	resolver *Resolver,
	state *StateManager,
	locker ports.Locker,
	lockPath string,
	runner ports.CommandRunner,
	fs ports.FileSystem,
) *InstallService {
	return &InstallService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: state, locker: locker,
			lockPath: lockPath, runner: runner, fs: fs,
		},
		resolver: resolver,
	}
}

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

func withState(ctx context.Context, locker ports.Locker, lockPath string, state *StateManager, fn func(*State) error) error {
	return lockrun.WithLock(ctx, locker, lockPath, func() error {
		st, err := state.Load()
		if err != nil {
			return fmt.Errorf("load state: %w", err)
		}
		return fn(st)
	})
}
