package service

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// RemoveService orchestrates the removal of one or more packages, including
// orphan cleanup and state persistence.
type RemoveService struct {
	baseService
	pkgLister ports.PackageLister
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
	sys ports.System,
	aptUpdate ports.AptUpdater,
	extrepo ports.ExtrepoManager,
	pkgLister ports.PackageLister,
) *RemoveService {
	return &RemoveService{
		baseService: baseService{
			reg: reg, instReg: instReg, state: state, locker: locker,
			lockPath: lockPath, runner: runner, fs: fs, sys: sys,
			aptUpdate: aptUpdate, extrepo: extrepo,
		},
		pkgLister: pkgLister,
	}
}

// Run removes each named package in sequence, acquiring the lock first.
func (s *RemoveService) Run(ctx context.Context, names []string, spinner ports.Spinner) error {
	return withState(ctx, s.locker, s.lockPath, s.state, func(st *State) error {
		removedAny := false
		for _, name := range names {
			if err := s.RemoveOne(ctx, name, st, spinner); err != nil {
				if !errors.Is(err, ErrNotInstalled) {
					if saveErr := s.state.Save(st); saveErr != nil {
						return fmt.Errorf("save state: %w", saveErr)
					}
					spinner.Fail()
					return err
				}
			} else {
				removedAny = true
			}
		}
		if err := s.state.Save(st); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
		if removedAny {
			if len(names) > 1 {
				spinner.SetDesc("Packages removed")
			}
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

	if _, err := s.checkInstalled(ctx, st, name, p, spinner); err != nil {
		return err
	}

	inst, err := LookupInstaller(s.instReg, p.Type)
	if err != nil {
		return err
	}
	if err := inst.Remove(ctx, p, spinner); err != nil {
		return fmt.Errorf("remove %s: %w", p.Name, err)
	}

	if err := s.disableOrphanedExtrepos(ctx, p, st, spinner); err != nil {
		return fmt.Errorf("disable orphaned extrepos: %w", err)
	}

	s.state.Remove(st, name)
	s.removeDependents(ctx, st, spinner)
	if err := s.removeOrphaned(ctx, st, spinner); err != nil {
		return fmt.Errorf("cleanup orphans: %w", err)
	}

	spinner.SetDesc(name + " removed")
	return nil
}

// removeDependents removes any installed package whose Depends are no longer
// satisfied by the current state, handling transitive dependencies.
func (s *RemoveService) removeDependents(ctx context.Context, st *State, spinner ports.Spinner) {
	for {
		removed := false
		for name := range st.Packages {
			p, err := LookupPackage(s.reg, name)
			if err != nil {
				continue
			}
			if s.depUnsatisfied(p, st) {
				s.state.Remove(st, name)
				spinner.SetDesc(name + " removed (dependency)")
				removed = true
			}
		}
		if !removed {
			break
		}
	}
}

// AffectedDependents returns all installed packages whose Depends would
// no longer be satisfied if the given names were removed, including
// transitive dependents. It does not mutate state.
func (s *RemoveService) AffectedDependents(st *State, names []string) []string {
	orig := make(map[string]bool)
	gone := make(map[string]bool)
	for _, n := range names {
		if _, ok := st.Packages[n]; ok {
			orig[n] = true
			gone[n] = true
		}
	}

	for {
		added := false
		for name := range st.Packages {
			if gone[name] {
				continue
			}
			p, err := LookupPackage(s.reg, name)
			if err != nil {
				continue
			}
			for _, dep := range p.Depends {
				if _, ok := st.Packages[dep]; !ok {
					gone[name] = true
					added = true
					break
				}
				if gone[dep] {
					gone[name] = true
					added = true
					break
				}
			}
		}
		if !added {
			break
		}
	}

	result := make([]string, 0, len(gone))
	for n := range gone {
		if !orig[n] {
			result = append(result, n)
		}
	}
	sort.Strings(result)
	if len(result) == 0 {
		return nil
	}
	return result
}

// depUnsatisfied reports whether any of p's Depends are missing from st.
func (s *RemoveService) depUnsatisfied(p *pkg.Package, st *State) bool {
	for _, dep := range p.Depends {
		if _, ok := st.Packages[dep]; !ok {
			return true
		}
	}
	return false
}

func (s *RemoveService) removeOrphaned(ctx context.Context, st *State, spinner ports.Spinner) error {
	installed, err := s.pkgLister.ListInstalled(ctx)
	if err != nil {
		return fmt.Errorf("list dpkg packages: %w", err)
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
	return nil
}

// pkgIsOrphaned reports whether p tracks system packages (apt or deb) that
// are no longer installed on the system, meaning the state entry is stale.
func pkgIsOrphaned(p *pkg.Package, installed map[string]bool) bool {
	if p.Type != pkg.TypeDeb && p.Type != pkg.TypeApt {
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

// disableOrphanedExtrepos disables any extrepo that was exclusively used by
// the removed package and is not needed by any other installed package.
func (s *RemoveService) disableOrphanedExtrepos(ctx context.Context, p *pkg.Package, st *State, spinner ports.Spinner) error {
	if p.Apt == nil {
		return nil
	}
	for _, repo := range p.Apt.Extrepo {
		if s.extrepoNeeded(ctx, repo, p.Name, st) {
			continue
		}
		spinner.SetDesc("disabling extrepo " + repo)
		if _, _, err := s.runner.Run(ctx, "extrepo", "disable", repo); err != nil {
			return fmt.Errorf("disable extrepo %s: %w", repo, err)
		}
	}
	return nil
}

// extrepoNeeded checks whether any installed package other than except needs
// the given extrepo.
func (s *RemoveService) extrepoNeeded(ctx context.Context, repo, except string, st *State) bool {
	for name := range st.Packages {
		if name == except {
			continue
		}
		other, err := LookupPackage(s.reg, name)
		if err != nil {
			continue
		}
		if other.Apt != nil {
			for _, r := range other.Apt.Extrepo {
				if r == repo {
					return true
				}
			}
		}
	}
	return false
}
