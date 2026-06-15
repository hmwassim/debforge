package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/hmwassim/debforge/pkg/executil"
	"github.com/hmwassim/debforge/pkg/packages"
	"github.com/hmwassim/debforge/pkg/settings"
	"github.com/hmwassim/debforge/pkg/text"
)

const fontsURL = "https://codeberg.org/hmwassim/fonts/raw/branch/main/fonts.tar.gz"

type fontCacheMeta struct {
	SHA256 string `json:"sha256"`
	ETag   string `json:"etag,omitempty"`
}

func metaPath(cachePath string) string {
	return cachePath + ".meta"
}

func installCodebergFonts(log *text.Logger) error {
	cachePath := settings.Default.CacheDir() + "/fonts.tar.gz"
	fontDir := "/usr/local/share/fonts"

	if _, err := os.Stat(cachePath); err == nil {
		s := text.StartSpinner(os.Stderr, "Installing fonts...")
		fresh, err := cacheIsFresh(cachePath)
		if err == nil && fresh {
			if err := extractFonts(cachePath, fontDir); err == nil {
				s.Done()
				return nil
			}
			log.Warn("Cached fonts are corrupt, re-downloading...")
		} else if _, metaErr := os.Stat(metaPath(cachePath)); os.IsNotExist(metaErr) {
			if err := saveMeta(cachePath); err == nil {
				if err := extractFonts(cachePath, fontDir); err == nil {
					s.Done()
					return nil
				}
			}
			log.Warn("Cached fonts are corrupt, re-downloading...")
		}
		s.Fail()
		os.Remove(cachePath)
		os.Remove(metaPath(cachePath))
	}

	if err := os.MkdirAll(settings.Default.CacheDir(), 0755); err != nil {
		return err
	}

	if err := packages.DownloadFile(cachePath, fontsURL, "Downloading custom fonts"); err != nil {
		return fmt.Errorf("downloading fonts: %w", err)
	}

	if err := saveMeta(cachePath); err != nil {
		return err
	}

	s := text.StartSpinner(os.Stderr, "Installing fonts...")
	err := extractFonts(cachePath, fontDir)
	s.Done()
	return err
}

func cacheIsFresh(path string) (bool, error) {
	meta, err := readMeta(path)
	if err != nil {
		return false, err
	}
	sum, err := hashFile(path)
	if err != nil {
		return false, err
	}
	if sum != meta.SHA256 {
		return false, nil
	}
	etag, err := headETag(fontsURL)
	if err == nil && etag != "" && meta.ETag != "" && etag != meta.ETag {
		return false, nil
	}
	return true, nil
}

func saveMeta(path string) error {
	sum, err := hashFile(path)
	if err != nil {
		return fmt.Errorf("checksumming: %w", err)
	}
	meta := fontCacheMeta{SHA256: sum}
	if etag, err := headETag(fontsURL); err == nil && etag != "" {
		meta.ETag = etag
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath(path), data, 0644)
}

func readMeta(path string) (*fontCacheMeta, error) {
	data, err := os.ReadFile(metaPath(path))
	if err != nil {
		return nil, err
	}
	var meta fontCacheMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func headETag(url string) (string, error) {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext:       (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
		},
		Timeout: 15 * time.Second,
	}
	resp, err := client.Head(url)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	return resp.Header.Get("ETag"), nil
}

func extractFonts(path, fontDir string) error {
	if err := os.MkdirAll(fontDir, 0755); err != nil {
		return err
	}
	extract := exec.Command("tar", "-xzf", path, "-C", fontDir)
	extract.Stdout = io.Discard
	if err := executil.Run(extract); err != nil {
		return fmt.Errorf("extracting fonts: %w", err)
	}
	return executil.Run(exec.Command("fc-cache", "-f"))
}
