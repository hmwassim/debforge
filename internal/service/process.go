package service

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/installer/extrepo"
	"github.com/hmwassim/debforge/internal/domain/pkg"
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

	var namesNeedingWork []string
	if rerun {
		namesNeedingWork = names
	} else {
		for _, name := range names {
			satisfied, err := s.alreadySatisfied(ctx, name, st)
			if err != nil {
				return err
			}
			if !satisfied {
				namesNeedingWork = append(namesNeedingWork, name)
			}
		}
	}

	if err := s.enableAllExtrepos(ctx, namesNeedingWork, spinner); err != nil {
		return err
	}

	workDone := false
	for _, name := range names {
		didWork, err := s.processOne(ctx, name, force, rerun, st, spinner, verb, pastTense, sessionProcessed)
		if err != nil {
			// Persist whatever state was accumulated before the
			// failure so partially-installed packages are not lost.
			if saveErr := s.state.Save(st); saveErr != nil {
				return fmt.Errorf("save state: %w", saveErr)
			}
			spinner.Fail()
			return err
		}
		if didWork {
			workDone = true
		}
	}

	// Persist state once at the end instead of per-package. The in-memory
	// st tracks all mutations; only the final write needs disk I/O.
	if err := s.state.Save(st); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	if workDone {
		if len(names) > 1 {
			spinner.SetDesc("Packages " + pastTense)
		}
		spinner.Done()
	} else {
		if len(names) > 1 {
			if verb == "update" {
				spinner.SetDesc("All packages up to date")
			} else {
				spinner.SetDesc("Packages already installed")
			}
		}
		spinner.DoneInfo()
	}
	return nil
}

// enableAllExtrepos pre-enables every extrepo referenced by any package in the
// dependency tree, then runs apt-get update once. This avoids redundant
// extrepo enable / apt-get update cycles per-package.
func (s *InstallService) enableAllExtrepos(ctx context.Context, names []string, spinner ports.Spinner) error {
	seen := make(map[string]bool)
	var allRepos []string

	for _, name := range names {
		p, err := LookupPackage(s.reg, name)
		if err != nil {
			continue
		}
		deps, err := s.resolver.Resolve(p)
		if err != nil {
			return fmt.Errorf("resolve deps for %q: %w", name, err)
		}
		for _, dep := range deps {
			if dep.Type != pkg.TypeApt || dep.Apt == nil {
				continue
			}
			for _, repo := range dep.Apt.Extrepo {
				if !seen[repo] {
					seen[repo] = true
					allRepos = append(allRepos, repo)
				}
			}
		}
	}

	if len(allRepos) == 0 {
		return nil
	}

	anyEnabled := false
	for _, repo := range allRepos {
		needed, err := extrepo.NeedsEnable(ctx, repo, s.fs)
		if err != nil {
			return err
		}
		if !needed {
			continue
		}
		if err := extrepo.Enable(ctx, repo, s.runner, spinner); err != nil {
			return err
		}
		anyEnabled = true
	}

	if anyEnabled {
		if err := aptpty.RunUpdate(ctx, s.runner, spinner); err != nil {
			return fmt.Errorf("apt-get update: %w", err)
		}
	}

	return nil
}

// alreadySatisfied checks whether a package is already installed and up to
// date, making a no-op of the full install pipeline. Returns true when the
// package can be skipped entirely.
func (s *InstallService) alreadySatisfied(ctx context.Context, name string, st *State) (bool, error) {
	if !s.state.IsInstalled(st, name) {
		return false, nil
	}
	p, err := LookupPackage(s.reg, name)
	if err != nil {
		return false, err
	}
	ok, err := installer.CheckInstalled(ctx, s.runner, s.fs, s.sys, p)
	if err != nil {
		return false, err
	}
	return ok, nil
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

	if !rerun {
		satisfied, err := s.alreadySatisfied(ctx, name, st)
		if err != nil {
			return false, err
		}
		if satisfied {
			spinner.SetDesc(name + " already installed")
			return false, nil
		}
	}

	ordered, err := s.resolver.Resolve(p)
	if err != nil {
		return false, fmt.Errorf("resolve deps: %w", err)
	}

	didWork := false
	var batch aptBatch

	for _, dep := range ordered {
		if sessionProcessed[dep.Name] {
			continue
		}
		if force {
			dep.ForceInstall = true
		}

		exists, oldVersion := s.restoreDepState(dep, st)

		if verb == "update" {
			dep = dep.Clone()
			dep.SkipRepoSetup = true
		}

		if dep.SkipUpdate && !dep.ForceInstall && exists {
			ok, err := installer.CheckInstalled(ctx, s.runner, s.fs, s.sys, dep)
			if err != nil {
				return false, err
			}
			if ok {
				if verb == "update" {
					spinner.SetDesc(dep.Name + " already up to date")
				} else {
					spinner.SetDesc(dep.Name + " already installed")
				}
				sessionProcessed[dep.Name] = true
				continue
			}
		}

		if !rerun && exists {
			ok, err := installer.CheckInstalled(ctx, s.runner, s.fs, s.sys, dep)
			if err != nil {
				return false, err
			}
			if ok {
				spinner.SetDesc(dep.Name + " already installed")
				sessionProcessed[dep.Name] = true
				continue
			}
		}

		if dep.Apt != nil && dep.Apt.Variant == "__skip__" {
			spinner.SetDesc(dep.Name + " skipped")
			sessionProcessed[dep.Name] = true
			continue
		}

		inst, err := LookupInstaller(s.instReg, dep.Type)
		if err != nil {
			return false, err
		}

		bi, ok := inst.(installer.BatchInstaller)
		if !ok {
			if batch.hasWork() {
				bw, err := s.flushAptBatch(ctx, &batch, st, spinner, verb, pastTense)
				if err != nil {
					return false, err
				}
				if bw {
					didWork = true
				}
			}
			if err := inst.Install(ctx, dep, spinner); err != nil {
				return false, fmt.Errorf("%s %s: %w", verb, dep.Name, err)
			}
			if dep.ForceInstall || !exists || dep.Version != oldVersion {
				entry := PkgEntry{
					Type:         string(dep.Type),
					Version:      dep.Version,
					ConfigHashes: dep.ConfigHashes,
				}
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
			continue
		}

		args, err := bi.Prepare(ctx, dep, spinner)
		if err != nil {
			return false, fmt.Errorf("%s %s: %w", verb, dep.Name, err)
		}
		if args.Skipped {
			sessionProcessed[dep.Name] = true
			continue
		}

		entry := batchEntry{
			pkg:        dep,
			bi:         bi,
			exists:     exists,
			oldVersion: oldVersion,
		}
		if len(args.AptPkgs) > 0 {
			batch.addApt(args.AptPkgs, entry)
		}
		if len(args.DebPaths) > 0 {
			batch.addDeb(args.DebPaths, entry)
		}
		sessionProcessed[dep.Name] = true
	}

	if batch.hasWork() {
		bw, err := s.flushAptBatch(ctx, &batch, st, spinner, verb, pastTense)
		if err != nil {
			return false, err
		}
		if bw {
			didWork = true
		}
	}

	return didWork, nil
}

// restoreDepState reads the persisted state entry for dep and restores its
// Version and Apt.Variant fields. Returns whether an entry existed and the
// old version string (for comparison after install).
func (s *InstallService) restoreDepState(dep *pkg.Package, st *State) (exists bool, oldVersion string) {
	entry, exists := s.state.Entry(st, dep.Name)
	if exists {
		dep.Version = entry.Version
		dep.ConfigHashes = entry.ConfigHashes
		oldVersion = entry.Version
		if v := lookupVariant(st, dep.Name); v != "" && dep.Apt != nil && dep.Apt.Variant == "" {
			dep.Apt.Variant = v
		}
	}
	return
}
