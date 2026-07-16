package service

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/aptpty"
	"github.com/hmwassim/debforge/internal/domain/installer"
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

	if err := s.enableAllExtrepos(ctx, names, spinner); err != nil {
		return err
	}

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
			continue
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

	for _, repo := range allRepos {
		spinner.SetDesc("enabling extrepo " + repo)
		if _, _, err := s.runner.Run(ctx, "extrepo", "enable", repo); err != nil {
			return fmt.Errorf("enable extrepo %s: %w", repo, err)
		}
	}

	if err := aptpty.RunUpdate(ctx, s.runner, spinner); err != nil {
		return fmt.Errorf("apt-get update: %w", err)
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
		ok, err := installer.CheckInstalled(ctx, s.runner, s.fs, s.sys, p)
		if err != nil {
			return false, err
		}
		if ok {
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
				if err := s.state.Save(st); err != nil {
					return false, fmt.Errorf("save state after %s: %w", dep.Name, err)
				}
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
