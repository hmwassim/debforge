package self

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/service"
)

type Remover struct {
	cfg       *Config
	runner    ports.CommandRunner
	fs        ports.FileSystem
	logger    ports.UI
	locker    ports.Locker
	sys       ports.System
	registry  *pkg.Registry
	instReg   *installer.Registry
	stateSvc  *service.StateManager
	removeSvc *service.RemoveService
}

func NewRemover(
	cfg *Config,
	runner ports.CommandRunner,
	fs ports.FileSystem,
	logger ports.UI,
	locker ports.Locker,
	sys ports.System,
	registry *pkg.Registry,
	instReg *installer.Registry,
	stateSvc *service.StateManager,
) *Remover {
	return &Remover{
		cfg: cfg, runner: runner, fs: fs, logger: logger, locker: locker, sys: sys,
		registry: registry, instReg: instReg, stateSvc: stateSvc,
		// removeSvc reuses InstallService's sibling RemoveOne logic (lookup
		// + remove + state bookkeeping) instead of Remover re-implementing
		// that loop by hand. lockPath is unused here since RemoveOne is
		// called while Remove already holds the lock.
		removeSvc: service.NewRemoveService(registry, instReg, stateSvc, locker, cfg.LockPath, runner),
	}
}

func (r *Remover) Remove(ctx context.Context) error {
	return withRootAndLock(ctx, "self-remove", r.sys, r.locker, r.cfg.LockPath, r.remove)
}

func (r *Remover) remove(ctx context.Context) error {
	r.logger.Warn("This will permanently remove debforge and all data under %s", r.cfg.RootDir)
	if !r.logger.Prompt("Remove debforge?") {
		r.logger.Info("Cancelled")
		return nil
	}

	spinner := r.logger.Spinner(ctx, "Removing debforge")
	defer spinner.Done()

	r.removeManagedPackages(ctx, spinner)

	if err := verifyRemovablePath(r.cfg.RootDir); err != nil {
		spinner.Fail()
		return err
	}

	spinner.SetDesc("Removing debforge files")
	if err := r.fs.RemoveAll(r.cfg.RootDir); err != nil {
		spinner.Fail()
		return fmt.Errorf("remove %s: %w", r.cfg.RootDir, err)
	}

	if err := r.fs.RemoveAll(r.cfg.LinkPath); err != nil {
		r.logger.Warn("could not remove %s: %s", r.cfg.LinkPath, err)
	}

	spinner.SetDesc("Debforge has been removed")
	return nil
}

// removeManagedPackages best-effort removes every package debforge has
// installed, via the same RemoveOne path used by "debforge remove" - so
// self-remove no longer skips updating/saving state the way the old
// hand-rolled loop here did. Failures are warned about, not fatal: the
// root directory (including the state file) is about to be deleted
// regardless, so self-remove should keep going rather than abort partway.
func (r *Remover) removeManagedPackages(ctx context.Context, spinner ports.Spinner) {
	st, err := r.stateSvc.Load()
	if err != nil {
		return
	}
	names := r.stateSvc.ListPackages(st)
	if len(names) == 0 {
		return
	}

	for _, name := range names {
		spinner.SetDesc("Removing " + name)
		if err := r.removeSvc.RemoveOne(ctx, name, st, spinner); err != nil {
			r.logger.Warn("could not remove %s: %s", name, err)
		}
	}
}

var dangerousRoots = []string{
	"/", "/opt", "/usr", "/etc", "/var", "/home", "/root",
}

func verifyRemovablePath(path string) error {
	clean := filepath.Clean(path)
	if clean == "" {
		return fmt.Errorf("path is empty")
	}
	for _, d := range dangerousRoots {
		if clean == d {
			return fmt.Errorf("refusing to remove: %q is a dangerous system path", clean)
		}
	}
	return nil
}
