package service

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// pipelineCtx groups the mutable state threaded through the install/update
// pipeline so that processAll, processOne, processDep, shouldSkip,
// nonBatchInstall, and flushAptBatch no longer need long parameter lists.
type pipelineCtx struct {
	st               *State
	spinner          ports.Spinner
	sessionProcessed map[string]bool
	rerun            bool
	force            bool
	verb             string
	pastTense        string
}

// processAll installs or updates every named package and its transitive
// dependencies. The pctx carries the force/rerun flags, the spinner, and the
// session-scoped bookkeeping that processOne and processDep mutate.
func (s *InstallService) processAll(ctx context.Context, names []string, pctx *pipelineCtx) error {
	pctx.sessionProcessed = make(map[string]bool)

	var namesNeedingWork []string
	if pctx.rerun {
		namesNeedingWork = names
	} else {
		for _, name := range names {
			satisfied, err := s.alreadySatisfied(ctx, name, pctx.st)
			if err != nil {
				return err
			}
			if !satisfied {
				namesNeedingWork = append(namesNeedingWork, name)
			}
		}
	}

	if err := s.enableAllExtrepos(ctx, namesNeedingWork, pctx.spinner); err != nil {
		return err
	}

	workDone := false
	for _, name := range names {
		didWork, err := s.processOne(ctx, name, pctx)
		if err != nil {
			// Persist whatever state was accumulated before the
			// failure so partially-installed packages are not lost.
			if saveErr := s.state.Save(pctx.st); saveErr != nil {
				return fmt.Errorf("save state: %w", saveErr)
			}
			pctx.spinner.Fail()
			return err
		}
		if didWork {
			workDone = true
		}
	}

	// Persist state once at the end instead of per-package. The in-memory
	// st tracks all mutations; only the final write needs disk I/O.
	if err := s.state.Save(pctx.st); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	if workDone {
		if len(names) > 1 {
			pctx.spinner.SetDesc("Packages " + pctx.pastTense)
		}
		pctx.spinner.Done()
	} else {
		if len(names) > 1 {
			if pctx.verb == "update" {
				pctx.spinner.SetDesc("All packages up to date")
			} else {
				pctx.spinner.SetDesc("Packages already installed")
			}
		}
		pctx.spinner.DoneInfo()
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
		needed, err := s.extrepo.NeedsEnable(ctx, repo)
		if err != nil {
			return err
		}
		if !needed {
			continue
		}
		if err := s.extrepo.Enable(ctx, repo, spinner); err != nil {
			return err
		}
		anyEnabled = true
	}

	if anyEnabled {
		if err := s.aptUpdate.RunUpdate(ctx, spinner); err != nil {
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

// shouldSkip reports whether dep can be skipped in the current pass.
// When it returns true the caller should mark the dep as processed and
// continue to the next dependency without installing.
func (s *InstallService) shouldSkip(ctx context.Context, dep *pkg.Package, exists bool, pctx *pipelineCtx) (skip bool, err error) {
	if dep.SkipUpdate && !dep.ForceInstall && exists {
		ok, err := installer.CheckInstalled(ctx, s.runner, s.fs, s.sys, dep)
		if err != nil {
			return false, err
		}
		if ok {
			if pctx.verb == "update" {
				pctx.spinner.SetDesc(dep.Name + " already up to date")
			} else {
				pctx.spinner.SetDesc(dep.Name + " already installed")
			}
			pctx.sessionProcessed[dep.Name] = true
			return true, nil
		}
	}

	if !pctx.rerun && exists {
		ok, err := installer.CheckInstalled(ctx, s.runner, s.fs, s.sys, dep)
		if err != nil {
			return false, err
		}
		if ok {
			pctx.spinner.SetDesc(dep.Name + " already installed")
			pctx.sessionProcessed[dep.Name] = true
			return true, nil
		}
	}

	if dep.Apt != nil && dep.Apt.Variant == "__skip__" {
		pctx.spinner.SetDesc(dep.Name + " skipped")
		pctx.sessionProcessed[dep.Name] = true
		return true, nil
	}

	return false, nil
}

// nonBatchInstall runs a non-batch installer for dep, updates state, and
// returns whether any work was performed.
func (s *InstallService) nonBatchInstall(ctx context.Context, inst installer.Installer, dep *pkg.Package, exists bool, oldVersion string, pctx *pipelineCtx) (didWork bool, err error) {
	if err := inst.Install(ctx, dep, pctx.spinner); err != nil {
		return false, fmt.Errorf("%s %s: %w", pctx.verb, dep.Name, err)
	}
	workDone := dep.ForceInstall || !exists || dep.Version != oldVersion
	if workDone {
		s.state.Add(pctx.st, dep.Name, newPkgEntry(dep))
		pctx.spinner.SetDesc(dep.Name + " " + pctx.pastTense)
	} else {
		pctx.spinner.SetDesc(dep.Name + " already up to date")
	}
	return workDone, nil
}

// depResult holds the outcome of processing a single dependency.
type depResult struct {
	didWork    bool
	batchAdd   bool
	needsFlush bool // non-batch installer encountered; flush pending batch first
	entry      batchEntry
	aptPkgs    []string
	debPaths   []string
}

// processDep processes a single dependency through the install pipeline:
// restore state, check skip conditions, then dispatch to the appropriate
// installer. Returns the result, which may include batch entries to collect.
//
// When hasPendingBatch is true and the dependency requires a non-batch
// installer, processDep returns needsFlush=true without performing the
// install. The caller must flush the batch first, then call processDep
// again.
func (s *InstallService) processDep(ctx context.Context, dep *pkg.Package, hasPendingBatch bool, pctx *pipelineCtx) (depResult, error) {
	if pctx.force {
		dep.ForceInstall = true
	}

	exists, oldVersion := s.restoreDepState(dep, pctx.st)

	if pctx.verb == "update" {
		dep = dep.Clone()
		dep.SkipRepoSetup = true
	}

	skip, err := s.shouldSkip(ctx, dep, exists, pctx)
	if err != nil {
		return depResult{}, err
	}
	if skip {
		return depResult{}, nil
	}

	inst, err := LookupInstaller(s.instReg, dep.Type)
	if err != nil {
		return depResult{}, err
	}
	bi, ok := inst.(installer.BatchInstaller)
	if !ok {
		if hasPendingBatch {
			return depResult{needsFlush: true}, nil
		}
		didWork, err := s.nonBatchInstall(ctx, inst, dep, exists, oldVersion, pctx)
		if err != nil {
			return depResult{}, err
		}
		pctx.sessionProcessed[dep.Name] = true
		return depResult{didWork: didWork}, nil
	}

	args, err := bi.Prepare(ctx, dep, pctx.spinner)
	if err != nil {
		return depResult{}, fmt.Errorf("%s %s: %w", pctx.verb, dep.Name, err)
	}
	if args.Skipped {
		pctx.sessionProcessed[dep.Name] = true
		return depResult{}, nil
	}

	entry := batchEntry{
		pkg:        dep,
		bi:         bi,
		exists:     exists,
		oldVersion: oldVersion,
	}
	pctx.sessionProcessed[dep.Name] = true
	return depResult{
		batchAdd: true,
		entry:    entry,
		aptPkgs:  args.AptPkgs,
		debPaths: args.DebPaths,
	}, nil
}

// processOne processes a single named package and its transitive
// dependencies. See processAll for the semantics of force and rerun.
func (s *InstallService) processOne(ctx context.Context, name string, pctx *pipelineCtx) (bool, error) {
	if pctx.sessionProcessed == nil {
		pctx.sessionProcessed = make(map[string]bool)
	}
	p, err := LookupPackage(s.reg, name)
	if err != nil {
		return false, err
	}

	if !pctx.rerun {
		satisfied, err := s.alreadySatisfied(ctx, name, pctx.st)
		if err != nil {
			return false, err
		}
		if satisfied {
			pctx.spinner.SetDesc(name + " already installed")
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
		if pctx.sessionProcessed[dep.Name] {
			continue
		}

		result, err := s.processDep(ctx, dep, batch.hasWork(), pctx)
		if err != nil {
			return false, err
		}
		if result.needsFlush {
			bw, err := s.flushAptBatch(ctx, &batch, pctx)
			if err != nil {
				return false, err
			}
			if bw {
				didWork = true
			}
			result, err = s.processDep(ctx, dep, false, pctx)
			if err != nil {
				return false, err
			}
		}
		if result.batchAdd {
			if len(result.aptPkgs) > 0 {
				batch.addApt(result.aptPkgs, result.entry)
			}
			if len(result.debPaths) > 0 {
				batch.addDeb(result.debPaths, result.entry)
			}
		}
		if result.didWork {
			didWork = true
		}
	}

	if batch.hasWork() {
		bw, err := s.flushAptBatch(ctx, &batch, pctx)
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
		if v := s.state.LookupVariant(st, dep.Name); v != "" && dep.Apt != nil && dep.Apt.Variant == "" {
			dep.Apt.Variant = v
		}
	}
	return
}
