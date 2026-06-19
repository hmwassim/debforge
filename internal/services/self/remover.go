package self

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	config "github.com/hmwassim/debforge/internal/config"
	"github.com/hmwassim/debforge/internal/domain/package"
	"github.com/hmwassim/debforge/internal/installers"
	"github.com/hmwassim/debforge/internal/services/state"
	"github.com/hmwassim/debforge/internal/ports"

	"github.com/hmwassim/debforge/internal/domain/apt"
)

type Remover struct {
	runner   ports.CommandRunner
	locker   ports.Locker
	logger   ports.UI
	fs       ports.FileSystem
	registry *pkg.Registry
	instReg  *installers.Registry
	stateSvc *state.Service
	aptSvc   apt.Service
	cfg      *config.Config
}

func NewRemover(
	runner ports.CommandRunner,
	locker ports.Locker,
	logger ports.UI,
	fs ports.FileSystem,
	registry *pkg.Registry,
	instReg *installers.Registry,
	stateSvc *state.Service,
	aptSvc apt.Service,
	cfg *config.Config,
) *Remover {
	return &Remover{
		runner:   runner,
		locker:   locker,
		logger:   logger,
		fs:       fs,
		registry: registry,
		instReg:  instReg,
		stateSvc: stateSvc,
		aptSvc:   aptSvc,
		cfg:      cfg,
	}
}

func (r *Remover) Remove(ctx context.Context, selection string) error {
	// BYPASS: os.Geteuid is an OS-level root check; no existing port covers it
	if os.Geteuid() != 0 {
		return fmt.Errorf("self-remove must be run as root")
	}

	release, err := r.locker.Acquire(ctx, r.cfg.LockFile())
	if err != nil {
		return fmt.Errorf("cannot acquire lock: %w", err)
	}
	defer release()

	r.logger.Warn("This will permanently remove debforge and all its data")
	if !r.logger.Prompt("Remove debforge?") {
		r.logger.Info("Cancelled")
		return nil
	}

	spinner := r.logger.Spinner(ctx, "Removing debforge")

	if selection != "" {
		if err := r.removeSelected(ctx, selection, spinner); err != nil {
			r.logger.Warn("Error removing selected packages: %s", err)
		}
	} else {
		r.uninstallManagedPackages(ctx, spinner)
	}

	if err := r.verifyRemovablePath(r.cfg.RootDir); err != nil {
		spinner.Fail()
		return fmt.Errorf("refusing to remove %s: %w", r.cfg.RootDir, err)
	}
	spinner.SetDesc("Removing debforge files")
	if err := r.fs.RemoveAll(r.cfg.RootDir); err != nil {
		spinner.Fail()
		return fmt.Errorf("removing %s: %w", r.cfg.RootDir, err)
	}

	if err := r.fs.RemoveAll(r.cfg.BinaryPath); err != nil {
		spinner.Fail()
		return fmt.Errorf("removing binary: %w", err)
	}

	if err := r.restoreSourcesBackup(ctx); err != nil {
		r.logger.Warn("restoring sources backup: %v", err)
	}

	spinner.SetDesc("debforge has been removed")
	spinner.Done()
	return nil
}

func (r *Remover) removeSelected(ctx context.Context, selection string, spinner ports.Spinner) error {
	st, err := r.stateSvc.Load()
	if err != nil {
		return fmt.Errorf("loading state: %w", err)
	}
	if len(st.Packages) == 0 {
		return nil
	}

	names := make([]string, 0, len(st.Packages))
	for name := range st.Packages {
		names = append(names, name)
	}
	sort.Strings(names)

	selected := parseSelection(selection, len(names))
	if len(selected) == 0 {
		return nil
	}

	var toRemove []string
	for _, idx := range selected {
		toRemove = append(toRemove, names[idx])
	}

	r.logger.Warn("Removing %s", strings.Join(toRemove, ", "))
	spinner.Pause()
	confirmed := r.logger.Prompt("Continue?")
	spinner.Resume()
	if !confirmed {
		return nil
	}

	for _, name := range toRemove {
		p, ok := r.registry.Lookup(name)
		if !ok {
			r.logger.Warn("Package definition for %s not found, skipping", name)
			continue
		}
		inst, ok := r.instReg.Lookup(p.Type)
		if !ok {
			r.logger.Warn("No installer for type %s, skipping", name)
			continue
		}
		spinner.SetDesc("Removing " + name)
		if err := inst.Remove(ctx, p); err != nil {
			r.logger.Warn("Could not remove %s: %s", name, err)
		}
		r.stateSvc.Remove(st, name)
	}
	if err := r.stateSvc.Save(st); err != nil {
		return fmt.Errorf("saving state after remove: %w", err)
	}
	return nil
}

func (r *Remover) uninstallManagedPackages(ctx context.Context, spinner ports.Spinner) {
	st, err := r.stateSvc.Load()
	if err != nil {
		r.logger.Warn("Could not load package state: %s", err)
		return
	}
	if len(st.Packages) == 0 {
		return
	}

	r.logger.Info("The following packages are managed by debforge:")
	names := make([]string, 0, len(st.Packages))
	for name := range st.Packages {
		names = append(names, name)
	}
	sort.Strings(names)
	for i, name := range names {
		r.logger.Info("  %d. %s", i+1, name)
	}

	spinner.Pause()
	input := r.logger.PromptInput("Select packages to remove (e.g. 1, 1-3, or 0 to skip):")
	spinner.Resume()
	selected := parseSelection(input, len(names))
	if len(selected) == 0 {
		r.logger.Info("Skipping package removal")
		return
	}

	var toRemove []string
	for _, idx := range selected {
		toRemove = append(toRemove, names[idx])
	}

	r.logger.Warn("Removing %s", strings.Join(toRemove, ", "))
	spinner.Pause()
	confirmed := r.logger.Prompt("Continue?")
	spinner.Resume()
	if !confirmed {
		r.logger.Info("Skipping package removal")
		return
	}

	for _, name := range toRemove {
		p, ok := r.registry.Lookup(name)
		if !ok {
			r.logger.Warn("Package definition for %s not found, skipping", name)
			continue
		}
		inst, ok := r.instReg.Lookup(p.Type)
		if !ok {
			r.logger.Warn("No installer for type %s, skipping", name)
			continue
		}
		spinner.SetDesc("Removing " + name)
		if err := inst.Remove(ctx, p); err != nil {
			r.logger.Warn("Could not remove %s: %s", name, err)
		}
		r.stateSvc.Remove(st, name)
	}
	if err := r.stateSvc.Save(st); err != nil {
		r.logger.Warn("saving state after remove: %v", err)
	}
}

func (r *Remover) restoreSourcesBackup(ctx context.Context) error {
	sourcesPath := r.cfg.SourcesListPath
	backupPath := sourcesPath + ".debforge-backup"

	if _, err := r.fs.Stat(backupPath); err != nil {
		return fmt.Errorf("no sources.list backup found")
	}

	data, err := r.fs.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("reading backup: %w", err)
	}

	if err := r.fs.AtomicWriteFile(sourcesPath, data, 0644); err != nil {
		return fmt.Errorf("restoring sources.list: %w", err)
	}
	if _, _, err := r.runner.Run(ctx, "chattr", "-i", backupPath); err != nil {
		r.logger.Warn("Could not unlock backup %s: %v", backupPath, err)
	}
	if err := r.fs.RemoveAll(backupPath); err != nil {
		r.logger.Warn("Could not remove backup: %s", err)
	}

	r.logger.Info("Original sources.list restored")
	r.logger.Info("Updating package lists")
	if err := r.aptSvc.Update(ctx); err != nil {
		r.logger.Warn("apt-get update failed: %s", err)
	}
	return nil
}

func parseSelection(input string, max int) []int {
	if input == "0" || input == "" {
		return nil
	}
	seen := map[int]bool{}
	var selected []int
	for _, part := range strings.Split(input, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			if len(bounds) != 2 {
				continue
			}
			start := parseInt(strings.TrimSpace(bounds[0]))
			end := parseInt(strings.TrimSpace(bounds[1]))
			if start == -1 || end == -1 {
				continue
			}
			if start > end {
				start, end = end, start
			}
			for i := start; i <= end; i++ {
				if i >= 1 && i <= max && !seen[i] {
					selected = append(selected, i)
					seen[i] = true
				}
			}
		} else {
			n := parseInt(part)
			if n >= 1 && n <= max && !seen[n] {
				selected = append(selected, n)
				seen[n] = true
			}
		}
	}
	result := make([]int, len(selected))
	for i, v := range selected {
		result[i] = v - 1
	}
	sort.Ints(result)
	return result
}

func parseInt(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return n
}

var dangerousRoots = []string{
	"/", "/opt", "/usr", "/etc", "/var", "/home", "/root",
}

func (r *Remover) verifyRemovablePath(path string) error {
	clean := filepath.Clean(path)
	if clean == "" {
		return fmt.Errorf("path is empty")
	}

	expected := filepath.Clean(r.cfg.RootDir)
	if clean != expected && !strings.HasPrefix(clean, expected+"/") {
		return fmt.Errorf("path %q is not within debforge root %q", clean, expected)
	}

	for _, d := range dangerousRoots {
		if expected == d {
			return fmt.Errorf("refusing to remove: debforge root %q is a dangerous system path", expected)
		}
	}

	return nil
}
