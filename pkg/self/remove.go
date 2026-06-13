package self

import (
	"fmt"
	"os"
	"path/filepath"

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

	log.Success("debforge has been removed")
	return nil
}

func verifyRemovablePath(path string) error {
	if path == "" {
		return fmt.Errorf("path is empty")
	}
	if filepath.Clean(path) == "/" {
		return fmt.Errorf("will not remove filesystem root")
	}
	return nil
}
