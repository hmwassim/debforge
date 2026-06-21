package self

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/service"
)

type Remover struct {
	cfg      *Config
	runner   ports.CommandRunner
	fs       ports.FileSystem
	logger   ports.UI
	locker   ports.Locker
	registry *pkg.Registry
	instReg  *installer.Registry
	stateSvc *service.StateManager
}

func NewRemover(
	cfg *Config,
	runner ports.CommandRunner,
	fs ports.FileSystem,
	logger ports.UI,
	locker ports.Locker,
	registry *pkg.Registry,
	instReg *installer.Registry,
	stateSvc *service.StateManager,
) *Remover {
	return &Remover{
		cfg: cfg, runner: runner, fs: fs, logger: logger, locker: locker,
		registry: registry, instReg: instReg, stateSvc: stateSvc,
	}
}

func (r *Remover) Remove(ctx context.Context) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("--self-remove must be run as root")
	}

	lockPath := filepath.Join(r.cfg.RootDir, "var", "lock")
	release, err := r.locker.Acquire(ctx, lockPath)
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer release()

	r.logger.Warn("This will permanently remove debforge and all data under %s", r.cfg.RootDir)
	if !r.logger.Prompt("Remove debforge?") {
		r.logger.Info("Cancelled")
		return nil
	}

	spinner := r.logger.Spinner(ctx, "Removing debforge")

	// Remove managed packages
	st, err := r.stateSvc.Load()
	if err == nil && len(st.Packages) > 0 {
		for name := range st.Packages {
			p, ok := r.registry.Lookup(name)
			if !ok {
				continue
			}
			inst, ok := r.instReg.Lookup(p.Type)
			if !ok {
				continue
			}
			spinner.SetDesc("Removing " + name)
			if err := inst.Remove(ctx, p, spinner); err != nil {
				r.logger.Warn("could not remove %s: %s", name, err)
			}
		}
	}

	// Safety check before removing root dir
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

	spinner.SetDesc("debforge has been removed")
	return nil
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
