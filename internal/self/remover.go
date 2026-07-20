package self

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/service"
)

// Remover handles the self-remove operation — removing all managed
// packages, then deleting debforge's root directory and link.
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

// NewRemover returns a new Remover with the given dependencies.
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
	aptUpdate ports.AptUpdater,
	extrepo ports.ExtrepoManager,
	pkgLister ports.PackageLister,
) *Remover {
	return &Remover{
		cfg: cfg, runner: runner, fs: fs, logger: logger, locker: locker, sys: sys,
		registry: registry, instReg: instReg, stateSvc: stateSvc,
		// removeSvc reuses InstallService's sibling RemoveOne logic (lookup
		// + remove + state bookkeeping) instead of Remover re-implementing
		// that loop by hand. lockPath is unused here since RemoveOne is
		// called while Remove already holds the lock.
		removeSvc: service.NewRemoveService(service.Deps{
			Reg: registry, InstReg: instReg, State: stateSvc, Locker: locker,
			LockPath: cfg.LockPath, Runner: runner, Fs: fs, Sys: sys,
			AptUpd: aptUpdate, Extrepo: extrepo,
		}, pkgLister),
	}
}

// Remove runs the self-remove flow: confirmation prompt, removal of
// managed packages, then deletion of the root directory and symlink.
func (r *Remover) Remove(ctx context.Context) error {
	return withRootAndLock(ctx, "self-remove", r.sys, r.locker, r.cfg.LockPath, r.remove)
}

func (r *Remover) remove(ctx context.Context) error {
	r.logger.Warn("This will permanently remove debforge and all data under %s", r.cfg.RootDir)
	if !r.logger.Prompt("Remove debforge?") {
		r.logger.Info("Cancelled")
		return nil
	}

	st, err := r.stateSvc.Load()
	var names []string
	if err != nil {
		r.logger.Warn("could not load state, skipping managed package removal: %s", err)
	} else {
		names = r.stateSvc.ListPackages(st)
		if len(names) > 0 {
			names = r.selectPackages(names)
		}
		if len(names) > 0 {
			if aff := r.removeSvc.AffectedDependents(st, names); len(aff) > 0 {
				r.logger.Info("Also removing: %s", strings.Join(aff, ", "))
				names = append(names, aff...)
			}
		}
	}

	spinner := r.logger.Spinner(ctx, "Removing debforge")
	defer spinner.Done()

	if len(names) > 0 {
		r.removeManagedPackages(ctx, names, st, spinner)
	}

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

// selectPackages displays a numbered list of managed packages and prompts
// the user to choose which to remove. Returns nil when nothing should be
// removed (user enters "0" or invalid input), all names for "a" / "A", or
// a filtered subset based on comma-separated numbers and ranges.
func (r *Remover) selectPackages(names []string) []string {
	sort.Strings(names)

	r.logger.Info("Packages managed by debforge:")
	for i, name := range names {
		r.logger.Info("  %d: %s", i+1, name)
	}

	input := r.logger.PromptInput("a", "0 = Skip, a = All, or select (e.g. 1,2,3 or 1-3)")
	input = strings.TrimSpace(input)
	if input == "" || input == "0" {
		return nil
	}
	if input == "a" || input == "A" {
		return names
	}

	selected := make(map[int]bool)
	for _, part := range strings.Split(input, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			start, err1 := strconv.Atoi(strings.TrimSpace(bounds[0]))
			end, err2 := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err1 != nil || err2 != nil || start < 1 || end > len(names) || start > end {
				continue
			}
			for i := start; i <= end; i++ {
				selected[i-1] = true
			}
		} else {
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 1 || idx > len(names) {
				continue
			}
			selected[idx-1] = true
		}
	}

	if len(selected) == 0 {
		return nil
	}

	result := make([]string, 0, len(selected))
	for i, name := range names {
		if selected[i] {
			result = append(result, name)
		}
	}
	return result
}

// removeManagedPackages best-effort removes the given managed packages via
// the same RemoveOne path used by "debforge remove". Failures are warned
// about, not fatal: the root directory (including the state file) is about
// to be deleted regardless, so self-remove should keep going rather than
// abort partway.
func (r *Remover) removeManagedPackages(ctx context.Context, names []string, st *service.State, spinner ports.Spinner) {
	for _, name := range names {
		if !r.stateSvc.IsInstalled(st, name) {
			continue
		}
		spinner.SetDesc("Removing " + name)
		if err := r.removeSvc.RemoveOne(ctx, name, st, spinner); err != nil {
			r.logger.Warn("could not remove %s: %s", name, err)
		}
	}
}

func verifyRemovablePath(path string) error {
	if path == "" {
		return fmt.Errorf("path is empty")
	}
	clean := filepath.Clean(path)
	for _, d := range installer.DangerousRoots {
		if clean == d {
			return fmt.Errorf("refusing to remove: %q is a dangerous system path", clean)
		}
	}
	return nil
}
