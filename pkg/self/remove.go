package self

import (
	"fmt"
	"os"

	"github.com/hmwassim/debforge/pkg/settings"
	"github.com/hmwassim/debforge/pkg/text"
)

func Remove(log *text.Logger) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("self-remove must be run as root")
	}

	log.Warn("This will permanently remove debforge and all its data")
	if !log.Prompt("Remove debforge?") {
		log.Info("Cancelled")
		return nil
	}

	log.Info("Removing %s...", settings.RootDir)
	if err := os.RemoveAll(settings.RootDir); err != nil {
		return fmt.Errorf("removing %s: %w", settings.RootDir, err)
	}

	log.Info("Removing binary at %s...", settings.BinaryPath)
	if err := os.Remove(settings.BinaryPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing binary: %w", err)
	}

	log.Success("debforge has been removed")
	return nil
}
