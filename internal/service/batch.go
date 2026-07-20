package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
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
func (s *InstallService) flushAptBatch(ctx context.Context, b *aptBatch, pctx *pipelineCtx) (bool, error) {
	if !b.hasWork() {
		return false, nil
	}

	args := []string{"install", "-y"}
	args = append(args, b.aptPkgs...)
	args = append(args, b.debPaths...)

	pctx.spinner.SetDesc("installing packages...")
	if err := s.execApt(ctx, s.runner, args, pctx.spinner); err != nil {
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
		if err := e.bi.Finalize(ctx, e.pkg, pctx.spinner); err != nil {
			finalizeErrs = append(finalizeErrs, fmt.Errorf("%s %s: %w", pctx.verb, e.pkg.Name, err))
			continue
		}

		if e.pkg.ForceInstall || !e.exists || e.pkg.Version != e.oldVersion {
			s.state.Add(pctx.st, e.pkg.Name, newPkgEntry(e.pkg))
			pctx.spinner.SetDesc(e.pkg.Name + " " + pctx.pastTense)
			didWork = true
		} else {
			pctx.spinner.SetDesc(e.pkg.Name + " already up to date")
		}
	}

	b.reset()
	if len(finalizeErrs) > 0 {
		return didWork, errors.Join(finalizeErrs...)
	}
	return didWork, nil
}
