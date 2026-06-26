package service

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/ports"
)

// processAll installs or updates every named package and its transitive
// dependencies.
//
// Parameters:
//   - force: when true, ForceInstall is set on every package (root + deps).
//     Each installer should skip its own version-up-to-date check when
//     ForceInstall is true, guaranteeing the install pipeline runs.
//   - rerun: when true, the system-installed check and the root-package
//     early-return gate are bypassed, so every package reaches the installer.
//     The installer may still short-circuit unless force is also true.
func (s *InstallService) processAll(ctx context.Context, names []string, force, rerun bool, st *State, spinner ports.Spinner, verb, pastTense string) error {
	sessionProcessed := make(map[string]bool)
	workDone := false
	for _, name := range names {
		didWork, err := s.processOne(ctx, name, force, rerun, st, spinner, verb, pastTense, sessionProcessed)
		if err != nil {
			spinner.Fail()
			return err
		}
		if didWork {
			workDone = true
		}
	}
	if verb == "update" && len(names) != 1 {
		spinner.SetDesc("All packages up to date")
	}
	if workDone {
		spinner.Done()
	} else {
		spinner.DoneInfo()
	}
	return nil
}

// processOne processes a single named package and its transitive
// dependencies. See processAll for the semantics of force and rerun.
func (s *InstallService) processOne(ctx context.Context, name string, force, rerun bool, st *State, spinner ports.Spinner, verb, pastTense string, sessionProcessed map[string]bool) (bool, error) {
	if sessionProcessed == nil {
		sessionProcessed = make(map[string]bool)
	}
	p, err := LookupPackage(s.reg, name)
	if err != nil {
		return false, err
	}

	if s.state.IsInstalled(st, name) && !rerun {
		if installer.CheckInstalled(ctx, s.runner, s.fs, p) {
			spinner.SetDesc(name + " already installed")
			return false, nil
		}
	}

	ordered, err := s.resolver.Resolve(p)
	if err != nil {
		return false, fmt.Errorf("resolve deps: %w", err)
	}

	didWork := false
	for _, dep := range ordered {
		if sessionProcessed[dep.Name] {
			continue
		}
		if force {
			dep.ForceInstall = true
		}

		entry, exists := s.state.Entry(st, dep.Name)
		oldVersion := ""
		if exists {
			dep.Version = entry.Version
			oldVersion = entry.Version
			if dep.Apt != nil && entry.Variant != "" {
				dep.Apt.Variant = entry.Variant
			}
		}

		// During update, skip extrepo setup on deps — main.go already ran
		// apt-get update, and any extrepo needed by the root was configured
		// during the original install. The version check added in the apt
		// installer then short-circuits when the candidate hasn't changed,
		// so this arm is typically a no-op for deps that are already current.
		// One gap: a transitive dep freshly pulled in by an updated root
		// definition that needs its own extrepo will fail during update;
		// users should run install for that dep first.
		if verb == "update" {
			dep = dep.Clone()
			dep.SkipRepoSetup = true
		}

		if !rerun && exists && installer.CheckInstalled(ctx, s.runner, s.fs, dep) {
			spinner.SetDesc(dep.Name + " already installed")
			continue
		}

		inst, err := LookupInstaller(s.instReg, dep.Type)
		if err != nil {
			return false, err
		}
		if err := inst.Install(ctx, dep, spinner); err != nil {
			return false, fmt.Errorf("%s %s: %w", verb, dep.Name, err)
		}

		if dep.ForceInstall || !exists || dep.Version != oldVersion {
			entry := PkgEntry{Type: string(dep.Type), Version: dep.Version}
			if dep.Apt != nil {
				entry.Variant = dep.Apt.Variant
			}
			s.state.Add(st, dep.Name, entry)
			spinner.SetDesc(dep.Name + " " + pastTense)
			didWork = true
		} else {
			spinner.SetDesc(dep.Name + " already up to date")
		}
		sessionProcessed[dep.Name] = true
	}

	if didWork {
		if err := saveState(s.state, st, pastTense); err != nil {
			return false, err
		}
	}

	return didWork, nil
}
