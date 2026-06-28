package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/dpkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// RemoveService orchestrates the removal of one or more packages, including
// orphan cleanup and state persistence.
type RemoveService struct {
	baseService
}

// NewRemoveService returns a new RemoveService.
func NewRemoveService(
	reg *pkg.Registry,
	instReg *installer.Registry,
	state *StateManager,
	locker ports.Locker,
	lockPath string,
	runner ports.CommandRunner,
	fs ports.FileSystem,
) *RemoveService {
	return &RemoveService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: state, locker: locker,
			lockPath: lockPath, runner: runner, fs: fs,
		},
	}
}

// Run removes each named package in sequence, acquiring the lock first.
func (s *RemoveService) Run(ctx context.Context, names []string, spinner ports.Spinner) error {
	return withState(ctx, s.locker, s.lockPath, s.state, func(st *State) error {
		removedAny := false
		for _, name := range names {
			if err := s.RemoveOne(ctx, name, st, spinner); err != nil {
				if !errors.Is(err, ErrNotInstalled) {
					spinner.Fail()
					return err
				}
			} else {
				removedAny = true
			}
		}
		if removedAny {
			spinner.Done()
		} else {
			spinner.DoneInfo()
		}
		return nil
	})
}

// RemoveOne removes a single already-resolved package from an
// already-loaded state. It is exported (rather than kept private to this
// service) so other flows that need to remove a managed package under a
// lock they already hold - such as internal/self's self-remove flow - call
// this instead of re-implementing lookup + remove + state bookkeeping by
// hand.
func (s *RemoveService) RemoveOne(ctx context.Context, name string, st *State, spinner ports.Spinner) error {
	p, err := LookupPackage(s.reg, name)
	if err != nil {
		return err
	}

	p = applyVariant(p, st, name)

	cleanedUp, err := checkInstalled(ctx, s.state, st, name, s.runner, s.fs, p, spinner)
	if err != nil {
		if cleanedUp {
			if saveErr := saveState(s.state, st, name); saveErr != nil {
				return saveErr
			}
		}
		return err
	}

	inst, err := LookupInstaller(s.instReg, p.Type)
	if err != nil {
		return err
	}
	if err := inst.Remove(ctx, p, spinner); err != nil {
		return fmt.Errorf("remove %s: %w", p.Name, err)
	}

	s.state.Remove(st, name)
	s.removeOrphaned(ctx, st, spinner)
	if err := saveState(s.state, st, p.Name); err != nil {
		return err
	}

	spinner.SetDesc(name + " removed")
	return nil
}

func (s *RemoveService) removeOrphaned(ctx context.Context, st *State, spinner ports.Spinner) {
	installed, err := dpkg.ListInstalled(ctx, s.runner)
	if err != nil {
		return
	}
	for name := range st.Packages {
		p, err := LookupPackage(s.reg, name)
		if err != nil {
			continue
		}
		if pkgIsOrphaned(p, installed) {
			s.state.Remove(st, name)
		}
	}
}

// pkgIsOrphaned reports whether p tracks system packages (apt or deb) that
// are no longer installed on the system, meaning the state entry is stale.
func pkgIsOrphaned(p *pkg.Package, installed map[string]bool) bool {
	if p.Type != pkg.TypeDeb && p.Type != pkg.TypeApt {
		return false
	}
	if len(p.Packages) > 0 {
		for _, pn := range p.Packages {
			if !installed[pn] {
				return true
			}
		}
		return false
	}
	if p.Apt != nil && len(p.Apt.Variants) > 0 {
		for _, pkgs := range p.Apt.Variants {
			for _, pn := range pkgs {
				if installed[pn] {
					return false
				}
			}
		}
		return true
	}
	return !installed[p.PrimarySystemPackage()]
}
