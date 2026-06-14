package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"

	"github.com/hmwassim/debforge/pkg/executil"
	"github.com/hmwassim/debforge/pkg/packages"
	"github.com/hmwassim/debforge/pkg/settings"
	"github.com/hmwassim/debforge/pkg/text"
)

const expectedFontsSHA256 = "ba2f262512f8acb31aac951cedb9195202f3ec6415a4c2c3c1ac3e123f64fc2f"

func installCodebergFonts(log *text.Logger) error {
	cachePath := settings.Default.CacheDir() + "/fonts.tar.gz"
	fontDir := "/usr/local/share/fonts"

	if _, err := os.Stat(cachePath); err == nil {
		log.Info("Using cached fonts...")
		if err := verifyAndExtractFonts(cachePath, fontDir); err == nil {
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

	return verifyAndExtractFonts(cachePath, fontDir)
}

func verifyAndExtractFonts(path, fontDir string) error {
	if err := verifySHA256(path); err != nil {
		os.Remove(path)
		return err
	}
	return extractFonts(path, fontDir)
}

func verifySHA256(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != expectedFontsSHA256 {
		return fmt.Errorf("SHA256 mismatch: got %s, expected %s", got, expectedFontsSHA256)
	}
	return nil
}

func extractFonts(path, fontDir string) error {
	if err := packages.ExtractTarGz(path, fontDir); err != nil {
		return fmt.Errorf("extracting fonts: %w", err)
	}
	return executil.Run(exec.Command("fc-cache", "-f", "-v"))
}
