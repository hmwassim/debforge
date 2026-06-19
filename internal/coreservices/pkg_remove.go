package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/installers"
	"github.com/hmwassim/debforge/internal/services/state"
	"github.com/hmwassim/debforge/internal/ports"
)

type PackageRemover interface {
	Remove(ctx context.Context, pkgNames []string, spinner ports.Spinner) error
}

type RemoveService struct {
	pkgRegistry *pkg.Registry
	instReg     *installers.Registry
	stateSvc    *state.Service
	logger      ports.UI
	locker      ports.Locker
	lockPath    string
}

func NewRemoveService(
	pkgRegistry *pkg.Registry,
	instReg *installers.Registry,
	stateSvc *state.Service,
	logger ports.UI,
	locker ports.Locker,
	lockPath string,
) *RemoveService {
	return &RemoveService{
		pkgRegistry: pkgRegistry,
		instReg:     instReg,
		stateSvc:    stateSvc,
		logger:      logger,
		locker:      locker,
		lockPath:    lockPath,
	}
}

func (s *RemoveService) Remove(ctx context.Context, pkgNames []string, spinner ports.Spinner) error {
	for _, pkgName := range pkgNames {
		if err := s.removeSingle(ctx, pkgName, spinner); err != nil {
			return err
		}
	}
	return nil
}

func (s *RemoveService) removeSingle(ctx context.Context, pkgName string, spinner ports.Spinner) error {
	release, err := s.locker.Acquire(ctx, s.lockPath)
	if err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}
	defer release()

	p, ok := s.pkgRegistry.Lookup(pkgName)
	if !ok {
		return fmt.Errorf("unknown package: %s", pkgName)
	}

	st, err := s.stateSvc.Load()
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}

	if !s.stateSvc.IsInstalled(st, pkgName) {
		spinner.SetDesc(pkgName + " is not installed")
		return nil
	}

	if deps := s.findReverseDeps(st, pkgName); len(deps) > 0 {
		return fmt.Errorf("cannot remove %s: required by %s", pkgName, strings.Join(deps, ", "))
	}

	inst, ok := s.instReg.Lookup(p.Type)
	if !ok {
		return fmt.Errorf("no installer for type %s", p.Type)
	}

	if err := inst.Remove(ctx, p); err != nil {
		return fmt.Errorf("removing %s: %w", pkgName, err)
	}

	s.stateSvc.Remove(st, pkgName)
	spinner.SetDesc(pkgName + " removed")
	return s.stateSvc.Save(st)
}

func (s *RemoveService) findReverseDeps(st *state.PackagesState, pkgName string) []string {
	var deps []string
	for name := range st.Packages {
		if name == pkgName {
			continue
		}
		p, ok := s.pkgRegistry.Lookup(name)
		if !ok {
			continue
		}
		for _, dep := range p.Depends {
			if dep == pkgName {
				deps = append(deps, name)
				break
			}
		}
	}
	return deps
}

var _ PackageRemover = (*RemoveService)(nil)
