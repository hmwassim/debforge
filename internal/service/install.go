package service

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/lockrun"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/textutil"
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
	return withState(ctx, s.locker, s.lockPath, s.state, func(st *State) error {
		return s.processAll(ctx, names, force, st, spinner, "install", "installed")
	})
}

func (s *InstallService) processAll(ctx context.Context, names []string, force bool, st *State, spinner ports.Spinner, verb, pastTense string) error {
	workDone := false
	for _, name := range names {
		didWork, err := s.processOne(ctx, name, force, st, spinner, verb, pastTense)
		if err != nil {
			spinner.Fail()
			return err
		}
		if didWork {
			workDone = true
		}
	}
	if workDone {
		spinner.Done()
	} else {
		spinner.DoneInfo()
	}
	return nil
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

func (s *InstallService) processOne(ctx context.Context, name string, force bool, st *State, spinner ports.Spinner, verb, pastTense string) (bool, error) {
	p, err := LookupPackage(s.reg, name)
	if err != nil {
		return false, err
	}

	if s.state.IsInstalled(st, name) && !force {
		spinner.SetDesc(textutil.UcFirst(name + " already installed"))
		return false, nil
	}

	if force {
		p = p.Clone()
		p.ForceInstall = true
	}

	ordered, err := s.resolver.Resolve(p, s.state.InstalledMap(st), force)
	if err != nil {
		return false, fmt.Errorf("resolve deps: %w", err)
	}

	for _, dep := range ordered {
		if entry, exists := s.state.Entry(st, dep.Name); exists {
			dep.Version = entry.Version
		}

		inst, err := LookupInstaller(s.instReg, dep.Type)
		if err != nil {
			return false, err
		}
		if err := inst.Install(ctx, dep, spinner); err != nil {
			return false, fmt.Errorf("%s %s: %w", verb, dep.Name, err)
		}

		s.state.Add(st, dep.Name, PkgEntry{Type: string(dep.Type), Version: dep.Version, Variant: dep.Variant})
		if err := saveState(s.state, st, dep.Name); err != nil {
			return false, err
		}
		spinner.SetDesc(textutil.UcFirst(dep.Name + " " + pastTense))
	}

	return true, nil
}
