package service

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

type InstallService struct {
	reg      *pkg.Registry
	instReg  *installer.Registry
	resolver *Resolver
	state    *StateManager
	locker   ports.Locker
	lockPath string
}

func NewInstallService(
	reg *pkg.Registry,
	instReg *installer.Registry,
	resolver *Resolver,
	state *StateManager,
	locker ports.Locker,
	lockPath string,
) *InstallService {
	return &InstallService{
		reg:      reg,
		instReg:  instReg,
		resolver: resolver,
		state:    state,
		locker:   locker,
		lockPath: lockPath,
	}
}

func (s *InstallService) Run(ctx context.Context, names []string, force bool, spinner ports.Spinner) error {
	release, err := s.locker.Acquire(ctx, s.lockPath)
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer release()

	st, err := s.state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	for _, name := range names {
		if err := s.installOne(ctx, name, force, st, spinner); err != nil {
			return err
		}
	}
	return nil
}

func (s *InstallService) installOne(ctx context.Context, name string, force bool, st *State, spinner ports.Spinner) error {
	p, ok := s.reg.Lookup(name)
	if !ok {
		return fmt.Errorf("unknown package: %s", name)
	}

	if s.state.IsInstalled(st, name) && !force {
		spinner.SetDesc(name + " already installed")
		return nil
	}

	if force {
		p = p.Clone()
		p.ForceInstall = true
	}

	installed := map[string]bool{}
	for n := range st.Packages {
		installed[n] = true
	}

	ordered, err := s.resolver.Resolve(p, installed, force)
	if err != nil {
		return fmt.Errorf("resolve deps: %w", err)
	}

	for _, dep := range ordered {
		if entry, exists := st.Packages[dep.Name]; exists {
			dep.Version = entry.Version
		}

		inst, ok := s.instReg.Lookup(dep.Type)
		if !ok {
			return fmt.Errorf("no installer for type %s", dep.Type)
		}
		if err := inst.Install(ctx, dep, spinner); err != nil {
			return fmt.Errorf("install %s: %w", dep.Name, err)
		}

		s.state.Add(st, dep.Name, PkgEntry{Type: string(dep.Type), Version: dep.Version})
		if err := s.state.Save(st); err != nil {
			return fmt.Errorf("save state after %s: %w", dep.Name, err)
		}
		spinner.SetDesc(dep.Name + " installed")
	}

	return nil
}

func (s *InstallService) Update(ctx context.Context, names []string, spinner ports.Spinner) error {
	release, err := s.locker.Acquire(ctx, s.lockPath)
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer release()

	st, err := s.state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	for _, name := range names {
		if err := s.updateOne(ctx, name, st, spinner); err != nil {
			return err
		}
	}
	return nil
}

func (s *InstallService) updateOne(ctx context.Context, name string, st *State, spinner ports.Spinner) error {
	p, ok := s.reg.Lookup(name)
	if !ok {
		return fmt.Errorf("unknown package: %s", name)
	}

	installed := map[string]bool{}
	for n := range st.Packages {
		installed[n] = true
	}

	ordered, err := s.resolver.Resolve(p, installed, true)
	if err != nil {
		return fmt.Errorf("resolve deps: %w", err)
	}

	for _, dep := range ordered {
		if entry, exists := st.Packages[dep.Name]; exists {
			dep.Version = entry.Version
		}
		inst, ok := s.instReg.Lookup(dep.Type)
		if !ok {
			return fmt.Errorf("no installer for type %s", dep.Type)
		}
		if err := inst.Install(ctx, dep, spinner); err != nil {
			return fmt.Errorf("update %s: %w", dep.Name, err)
		}
		s.state.Add(st, dep.Name, PkgEntry{Type: string(dep.Type), Version: dep.Version})
		if err := s.state.Save(st); err != nil {
			return fmt.Errorf("save state after %s: %w", dep.Name, err)
		}
		spinner.SetDesc(dep.Name + " updated")
	}

	return nil
}
