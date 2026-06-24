package service

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

func (s *InstallService) processAll(ctx context.Context, names []string, force, rerun bool, st *State, spinner ports.Spinner, verb, pastTense string) error {
	workDone := false
	for _, name := range names {
		didWork, err := s.processOne(ctx, name, force, rerun, st, spinner, verb, pastTense)
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
		if verb == "update" && len(names) != 1 {
			spinner.SetDesc("All packages up to date")
		}
		spinner.DoneInfo()
	}
	return nil
}

func (s *InstallService) processOne(ctx context.Context, name string, force, rerun bool, st *State, spinner ports.Spinner, verb, pastTense string) (bool, error) {
	p, err := LookupPackage(s.reg, name)
	if err != nil {
		return false, err
	}

	if s.state.IsInstalled(st, name) && !rerun {
		if (p.Type != pkg.TypeDeb && p.Type != pkg.TypeApt) || systemPackageInstalled(ctx, s.runner, p.Package) {
			spinner.SetDesc(name + " already installed")
			return false, nil
		}
	}

	if force {
		p = p.Clone()
		p.ForceInstall = true
	}

	ordered, err := s.resolver.Resolve(p, s.state.InstalledMap(st), rerun)
	if err != nil {
		return false, fmt.Errorf("resolve deps: %w", err)
	}

	didWork := false
	for _, dep := range ordered {
		entry, exists := s.state.Entry(st, dep.Name)
		oldVersion := ""
		if exists {
			dep.Version = entry.Version
			oldVersion = entry.Version
		}

		inst, err := LookupInstaller(s.instReg, dep.Type)
		if err != nil {
			return false, err
		}
		if err := inst.Install(ctx, dep, spinner); err != nil {
			return false, fmt.Errorf("%s %s: %w", verb, dep.Name, err)
		}

		if dep.ForceInstall || !exists || dep.Version != oldVersion {
			s.state.Add(st, dep.Name, PkgEntry{Type: string(dep.Type), Version: dep.Version, Variant: dep.Variant})
			if err := saveState(s.state, st, dep.Name); err != nil {
				return false, err
			}
			spinner.SetDesc(dep.Name + " " + pastTense)
			didWork = true
		} else {
			spinner.SetDesc(dep.Name + " already up to date")
		}
	}

	return didWork, nil
}
