package core

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/hmwassim/debforge/pkg/executil"
	"github.com/hmwassim/debforge/pkg/packages"
	"github.com/hmwassim/debforge/pkg/settings"
	"github.com/hmwassim/debforge/pkg/text"
)

func installCodebergFonts(log *text.Logger) error {
	cachePath := settings.Default.CacheDir() + "/fonts.tar.gz"
	fontDir := "/usr/local/share/fonts"

	if _, err := os.Stat(cachePath); err == nil {
		log.Info("Using cached fonts...")
		if err := extractFonts(cachePath, fontDir); err == nil {
			return nil
		}
		log.Warn("Cached fonts are corrupt, re-downloading...")
		if err := os.Remove(cachePath); err != nil {
			return fmt.Errorf("removing corrupt cache: %w", err)
		}
	}

	log.Info("Downloading custom fonts...")
	if err := os.MkdirAll(settings.Default.CacheDir(), 0755); err != nil {
		return err
	}

	if err := packages.DownloadFile(cachePath, "https://codeberg.org/hmwassim/fonts/raw/branch/main/fonts.tar.gz"); err != nil {
		return fmt.Errorf("downloading fonts: %w", err)
	}

	return extractFonts(cachePath, fontDir)
}

func extractFonts(path, fontDir string) error {
	if err := packages.ExtractTarGz(path, fontDir); err != nil {
		return fmt.Errorf("extracting fonts: %w", err)
	}
	return executil.Run(exec.Command("fc-cache", "-f", "-v"))
}
