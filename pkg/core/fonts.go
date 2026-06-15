package core

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hmwassim/debforge/pkg/executil"
	"github.com/hmwassim/debforge/pkg/packages"
	"github.com/hmwassim/debforge/pkg/settings"
	"github.com/hmwassim/debforge/pkg/text"
)

const fontsURL = "https://codeberg.org/hmwassim/fonts/raw/branch/main/fonts.tar.gz"

var etagClient = &http.Client{
	Transport: &http.Transport{
		DialContext:       (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
	},
	Timeout: 15 * time.Second,
}

type fontCacheMeta struct {
	SHA256 string `json:"sha256"`
	ETag   string `json:"etag,omitempty"`
}

func metaPath(cachePath string) string {
	return cachePath + ".meta"
}

func installCodebergFonts(log *text.Logger, s *text.Spinner, force bool) error {
	cachePath := settings.Default.CacheDir() + "/fonts.tar.gz"
	fontDir := "/usr/local/share/fonts"

	if !force {
		if _, err := os.Stat(cachePath); err == nil {
			fresh, checkErr := cacheIsFresh(cachePath)
			if checkErr != nil {
				log.Warn("Could not verify font cache (%s), using cached version", checkErr)
				if metaMissing := readMetaFails(cachePath); metaMissing {
					if err := saveMetaFromCache(cachePath); err == nil {
						log.Debug("Generated missing font cache metadata")
					}
				}
				if err := extractFonts(cachePath, fontDir, false); err == nil {
					return nil
				}
				log.Warn("Cached fonts are corrupt, re-downloading...")
				os.Remove(cachePath)
				os.Remove(metaPath(cachePath))
			} else if fresh {
				if err := extractFonts(cachePath, fontDir, false); err == nil {
					return nil
				}
				log.Warn("Cached fonts are corrupt, re-downloading...")
				os.Remove(cachePath)
				os.Remove(metaPath(cachePath))
			}
		}
	}

	if err := os.MkdirAll(settings.Default.CacheDir(), 0755); err != nil {
		return err
	}

	s.Pause()
	err := packages.DownloadFile(cachePath, fontsURL, "Downloading custom-fonts")
	s.Resume()
	if err != nil {
		return fmt.Errorf("downloading fonts: %w", err)
	}

	os.Remove(metaPath(cachePath))

	etag, _ := headETag(fontsURL)
	if err := saveMeta(cachePath, etag); err != nil {
		os.Remove(cachePath)
		return err
	}

	if err := extractFonts(cachePath, fontDir, true); err != nil {
		os.Remove(cachePath)
		os.Remove(metaPath(cachePath))
		return err
	}
	return nil
}

func readMetaFails(path string) bool {
	_, err := readMeta(path)
	return err != nil
}

func saveMetaFromCache(cachePath string) error {
	sum, err := hashFile(cachePath)
	if err != nil {
		return err
	}
	meta := fontCacheMeta{SHA256: sum}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath(cachePath), data, 0644)
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
	return sum == meta.SHA256, nil
}

func saveMeta(path, etag string) error {
	sum, err := hashFile(path)
	if err != nil {
		return fmt.Errorf("checksumming: %w", err)
	}
	meta := fontCacheMeta{SHA256: sum}
	if etag != "" {
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
	resp, err := etagClient.Head(url)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	return resp.Header.Get("ETag"), nil
}

func extractFonts(path, fontDir string, force bool) error {
	if err := os.MkdirAll(fontDir, 0755); err != nil {
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading font archive: %w", err)
		}

		clean := filepath.Clean(filepath.Join(fontDir, hdr.Name))
		if !strings.HasPrefix(clean, filepath.Clean(fontDir)+string(os.PathSeparator)) {
			return fmt.Errorf("font archive contains unsafe path: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(clean, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(clean), 0755); err != nil {
				return err
			}
			fw, err := os.Create(clean)
			if err != nil {
				return err
			}
			if _, err := io.Copy(fw, tr); err != nil {
				fw.Close()
				return err
			}
			fw.Close()
		}
	}

	if force {
		return executil.Run(exec.Command("fc-cache", "-f"))
	}
	return executil.Run(exec.Command("fc-cache"))
}
