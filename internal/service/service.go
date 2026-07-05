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

// variantSelector is implemented by installers that allow interactive
// selection of a package variant before the main install flow begins.
type variantSelector interface {
	SelectVariant(ctx context.Context, p *pkg.Package) error
}

type baseService struct {
	reg      *pkg.Registry
	instReg  *installer.Registry
	state    *StateManager
	locker   ports.Locker
	lockPath string
	runner   ports.CommandRunner
	fs       ports.FileSystem
	sys      ports.System
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
	sys ports.System,
) *InstallService {
	return &InstallService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: state, locker: locker,
			lockPath: lockPath, runner: runner, fs: fs, sys: sys,
		},
		resolver: resolver,
	}
}

// SelectVariants runs interactive variant selection for every package in
// the dependency tree of names. When force is true the saved state variant
// is ignored so the user can choose a different variant on re-install.
// Selected variants are written directly to the registry copy rather than
// persisted to state — state persistence only happens inside processOne
// after a successful install, so a failed download, GPU check, or Ctrl+C
// does not leave a stale variant record.
func (s *InstallService) SelectVariants(ctx context.Context, names []string, force bool) error {
	st, err := s.state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	for _, name := range names {
		p, err := LookupPackage(s.reg, name)
		if err != nil {
			return err
		}
		deps, err := s.resolver.Resolve(p)
		if err != nil {
			return fmt.Errorf("resolve deps: %w", err)
		}
		for _, dep := range deps {
			if dep.Apt == nil || len(dep.Apt.Variants) == 0 {
				continue
			}
			if !force && lookupVariant(st, dep.Name) != "" {
				continue
			}
			inst, err := LookupInstaller(s.instReg, dep.Type)
			if err != nil {
				return err
			}
			vs, ok := inst.(variantSelector)
			if !ok {
				continue
			}
			if err := vs.SelectVariant(ctx, dep); err != nil {
				return err
			}
			if regPkg, ok := s.reg.Lookup(dep.Name); ok && regPkg.Apt != nil {
				regPkg.Apt.Variant = dep.Apt.Variant
			}
		}
	}
	return nil
}

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
