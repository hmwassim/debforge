package self

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hmwassim/debforge/pkg/executil"
	"github.com/hmwassim/debforge/pkg/lock"
	"github.com/hmwassim/debforge/pkg/settings"
	"github.com/hmwassim/debforge/pkg/text"
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
		return
	}

	data, err := os.ReadFile(backupPath)
	if err != nil {
		log.Warn("Could not read sources.list backup: %s", err)
		return
	}

	if err := os.WriteFile(sourcesPath, data, 0644); err != nil {
		log.Warn("Could not restore sources.list: %s", err)
		return
	}
	os.Remove(backupPath)

	log.Info("Original sources.list restored")
	log.Info("Updating package lists...")
	if err := executil.Run(exec.Command("apt-get", "update")); err != nil {
		log.Warn("apt-get update failed: %s", err)
	}
	log.Info("Upgrading system packages...")
	if err := executil.Run(exec.Command("apt-get", "upgrade", "-y")); err != nil {
		log.Warn("apt-get upgrade failed: %s", err)
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
