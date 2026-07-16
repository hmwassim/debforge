package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// batchEntry holds per-package metadata collected during the prepare phase
// so that Finalize and state recording can happen after the batch install.
type batchEntry struct {
	pkg        *pkg.Package
	bi         installer.BatchInstaller
	exists     bool
	oldVersion string
}

// aptBatch collects apt/deb packages for a single apt-get install call.
type aptBatch struct {
	aptPkgs  []string     // apt package names (from BatchArgs.AptPkgs)
	debPaths []string     // deb file paths (from BatchArgs.DebPaths)
	entries  []batchEntry // per-package metadata for finalization
}

func (b *aptBatch) addApt(pkgs []string, entry batchEntry) {
	b.aptPkgs = append(b.aptPkgs, pkgs...)
	b.entries = append(b.entries, entry)
}

func (b *aptBatch) addDeb(paths []string, entry batchEntry) {
	b.debPaths = append(b.debPaths, paths...)
	b.entries = append(b.entries, entry)
}

func (b *aptBatch) hasWork() bool {
	return len(b.aptPkgs) > 0 || len(b.debPaths) > 0
}

func (b *aptBatch) reset() {
	b.aptPkgs = b.aptPkgs[:0]
	b.debPaths = b.debPaths[:0]
	b.entries = b.entries[:0]
}

// flushAptBatch runs a single apt-get install -y for all collected packages,
// then finalizes each package (version recording, configs, scripts, state).
func (s *InstallService) flushAptBatch(ctx context.Context, b *aptBatch, st *State, spinner ports.Spinner, verb, pastTense string) (bool, error) {
	if !b.hasWork() {
		return false, nil
	}

	args := []string{"install", "-y"}
	args = append(args, b.aptPkgs...)
	args = append(args, b.debPaths...)

	spinner.SetDesc("installing packages...")
	if err := s.execApt(ctx, s.runner, args, spinner); err != nil {
		for _, e := range b.entries {
			if ab, ok := e.bi.(installer.BatchAborter); ok {
				ab.Abort(e.pkg)
			}
		}
		return false, fmt.Errorf("batch apt-get install: %w", err)
	}

	didWork := false
	var finalizeErrs []error
	for _, e := range b.entries {
		if err := e.bi.Finalize(ctx, e.pkg, spinner); err != nil {
			finalizeErrs = append(finalizeErrs, fmt.Errorf("%s %s: %w", verb, e.pkg.Name, err))
			continue
		}

		if e.pkg.ForceInstall || !e.exists || e.pkg.Version != e.oldVersion {
			entry := PkgEntry{
				Type:         string(e.pkg.Type),
				Version:      e.pkg.Version,
				ConfigHashes: e.pkg.ConfigHashes,
			}
			if e.pkg.Apt != nil {
				entry.Variant = e.pkg.Apt.Variant
			}
			s.state.Add(st, e.pkg.Name, entry)
			if err := s.state.Save(st); err != nil {
				return false, fmt.Errorf("save state after %s: %w", e.pkg.Name, err)
			}
			spinner.SetDesc(e.pkg.Name + " " + pastTense)
			didWork = true
		} else {
			spinner.SetDesc(e.pkg.Name + " already up to date")
		}
	}

	b.reset()
	if len(finalizeErrs) > 0 {
		return didWork, errors.Join(finalizeErrs...)
	}
	return didWork, nil
}
