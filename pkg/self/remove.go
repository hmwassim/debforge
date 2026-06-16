package self

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/hmwassim/debforge/pkg/executil"
	"github.com/hmwassim/debforge/pkg/lock"
	"github.com/hmwassim/debforge/pkg/repo"
	"github.com/hmwassim/debforge/pkg/settings"
	"github.com/hmwassim/debforge/pkg/text"
	"github.com/hmwassim/debforge/pkg/writeutil"
)

func Remove(log *text.Logger) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("self-remove must be run as root")
	}

	release, err := lock.Acquire(settings.Default.LockFile())
	if err != nil {
		return fmt.Errorf("cannot acquire lock: %w", err)
	}
	defer release()

	log.Warn("This will permanently remove debforge and all its data")
	if !log.Prompt("Remove debforge?") {
		log.Info("Cancelled")
		return nil
	}

	uninstallManagedPackages(log)

	if err := verifyRemovablePath(settings.Default.RootDir); err != nil {
		return fmt.Errorf("refusing to remove %s: %w", settings.Default.RootDir, err)
	}
	log.Info("Removing %s...", settings.Default.RootDir)
	if err := os.RemoveAll(settings.Default.RootDir); err != nil {
		return fmt.Errorf("removing %s: %w", settings.Default.RootDir, err)
	}

	log.Info("Removing binary at %s...", settings.Default.BinaryPath)
	if err := os.Remove(settings.Default.BinaryPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing binary: %w", err)
	}

	restoreSourcesBackup(log)

	log.Success("debforge has been removed")
	return nil
}

func restoreSourcesBackup(log *text.Logger) {
	const backupPath = "/etc/apt/sources.list.debforge-backup"
	const sourcesPath = "/etc/apt/sources.list"

	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		log.Warn("No sources.list backup found — original not restored")
		return
	}

	data, err := os.ReadFile(backupPath)
	if err != nil {
		log.Warn("Could not read sources.list backup: %s", err)
		return
	}

	if err := writeutil.AtomicFile(sourcesPath, data, 0644); err != nil {
		log.Warn("Could not restore sources.list: %s", err)
		return
	}
	if err := writeutil.SetImmutable(backupPath, false); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not unlock backup %s: %v\n", backupPath, err)
	}
	if err := os.Remove(backupPath); err != nil {
		log.Warn("Could not remove backup: %s", err)
	}

	log.Info("Original sources.list restored")
	log.Info("Updating package lists...")
	if err := executil.Run(exec.Command("apt-get", "update")); err != nil {
		log.Warn("apt-get update failed: %s", err)
	}
}

func uninstallManagedPackages(log *text.Logger) {
	state, err := repo.LoadState()
	if err != nil {
		log.Warn("Could not load package state: %s", err)
		return
	}
	if len(state.Packages) == 0 {
		return
	}

	log.Info("The following packages are managed by debforge:")
	names := make([]string, 0, len(state.Packages))
	for name := range state.Packages {
		names = append(names, name)
	}
	sort.Strings(names)
	for i, name := range names {
		log.Info("  %d. %s", i+1, name)
	}

	input := log.PromptLine("Select packages to remove (e.g. 1, 1-3, or 0 to skip):")
	selected := parseSelection(input, len(names))
	if len(selected) == 0 {
		log.Info("Skipping package removal")
		return
	}

	var toRemove []string
	for _, idx := range selected {
		toRemove = append(toRemove, names[idx])
	}

	log.Warn("Removing %s", strings.Join(toRemove, ", "))
	if !log.Prompt("Continue?") {
		log.Info("Skipping package removal")
		return
	}

	for _, name := range toRemove {
		pkg := repo.Lookup(name)
		if pkg == nil {
			log.Warn("Package definition for %s not found, skipping", name)
			continue
		}
		if err := pkg.Remove(log); err != nil {
			log.Warn("Could not remove %s: %s", name, err)
		}
	}
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

func verifyRemovablePath(path string) error {
	clean := filepath.Clean(path)
	if clean == "" {
		return fmt.Errorf("path is empty")
	}

	// Verify the path matches the expected debforge root.
	expected := filepath.Clean(settings.Default.RootDir)
	if clean != expected && !strings.HasPrefix(clean, expected+"/") {
		return fmt.Errorf("path %q is not within debforge root %q", clean, expected)
	}

	// Block removal if the debforge root itself is a dangerous system directory.
	for _, d := range dangerousRoots {
		if expected == d {
			return fmt.Errorf("refusing to remove: debforge root %q is a dangerous system path", expected)
		}
	}

	return nil
}
