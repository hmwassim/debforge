package services

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/hmwassim/debforge/internal/utils"
	"github.com/hmwassim/debforge/internal/ports"
)

type fontCacheMeta struct {
	SHA256 string `json:"sha256"`
	ETag   string `json:"etag,omitempty"`
}

type FontInstaller struct {
	fs       ports.FileSystem
	http     ports.HTTPClient
	runner   ports.CommandRunner
	logger   ports.UI
	cacheDir string
	fontDir  string
	fontURL  string
}

func NewFontInstaller(fs ports.FileSystem, http ports.HTTPClient, runner ports.CommandRunner, logger ports.UI, cacheDir, fontDir, fontURL string) *FontInstaller {
	return &FontInstaller{fs: fs, http: http, runner: runner, logger: logger, cacheDir: cacheDir, fontDir: fontDir, fontURL: fontURL}
}

func (f *FontInstaller) Install(ctx context.Context, spinner ports.Spinner) error {

	if err := f.fs.MkdirAll(f.cacheDir, 0755); err != nil {
		return err
	}

	cachePath := filepath.Join(f.cacheDir, "fonts.tar.gz")

	if _, err := f.fs.Stat(cachePath); err == nil {
		if fresh, checkErr := f.cacheIsFresh(cachePath); checkErr != nil {
			f.logger.Warn("Could not verify font cache (%s), using cached version", checkErr)
			spinner.SetDesc("Extracting extra fonts")
			if err := f.extractTarGz(cachePath, f.fontDir); err == nil {
				spinner.SetDesc("Updating font cache")
				_, _, ferr := f.runner.Run(ctx, "fc-cache")
				return ferr
			}
			f.logger.Warn("Cached fonts are corrupt (%v), re-downloading", err)
			f.removeCache(cachePath)
		} else if fresh {
			spinner.SetDesc("Extracting extra fonts")
			if err := f.extractTarGz(cachePath, f.fontDir); err == nil {
				spinner.SetDesc("Updating font cache")
				_, _, ferr := f.runner.Run(ctx, "fc-cache")
				return ferr
			}
			f.logger.Warn("Cached fonts are corrupt (%v), re-downloading", err)
			f.removeCache(cachePath)
		}
	}

	spinner.SetDesc("Downloading extra fonts")
	if err := f.downloadFonts(ctx, cachePath, spinner); err != nil {
		return fmt.Errorf("downloading fonts: %w", err)
	}

	etag, err := f.headETag(ctx, f.fontURL)
	if err != nil {
		f.logger.Warn("Could not fetch font ETag: %s", err)
	}
	if err := f.saveFontMeta(cachePath, etag); err != nil {
		if rmErr := f.fs.RemoveAll(cachePath); rmErr != nil {
			f.logger.Warn("removing untracked font cache: %v", rmErr)
		}
		return err
	}

	spinner.SetDesc("Extracting extra fonts")
	if err := f.extractTarGz(cachePath, f.fontDir); err != nil {
		if rmErr := f.fs.RemoveAll(cachePath); rmErr != nil {
			f.logger.Warn("removing bad font cache: %v", rmErr)
		}
		if rmErr := f.fs.RemoveAll(metaPath(cachePath)); rmErr != nil {
			f.logger.Warn("removing bad font meta: %v", rmErr)
		}
		return err
	}

	spinner.SetDesc("Updating font cache")
	_, _, err = f.runner.Run(ctx, "fc-cache", "-f")
	return err
}

func (f *FontInstaller) removeCache(cachePath string) {
	if err := f.fs.RemoveAll(cachePath); err != nil {
		f.logger.Warn("removing font cache: %v", err)
	}
	if err := f.fs.RemoveAll(metaPath(cachePath)); err != nil {
		f.logger.Warn("removing font meta: %v", err)
	}
}

func metaPath(cachePath string) string {
	return cachePath + ".meta"
}

func (f *FontInstaller) cacheIsFresh(path string) (bool, error) {
	data, err := f.fs.ReadFile(metaPath(path))
	if err != nil {
		return false, err
	}
	var meta fontCacheMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return false, err
	}
	sum, err := f.hashFile(path)
	if err != nil {
		return false, err
	}
	return sum == meta.SHA256, nil
}

func (f *FontInstaller) saveFontMeta(path, etag string) error {
	sum, err := f.hashFile(path)
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
	return f.fs.AtomicWriteFile(metaPath(path), data, 0644)
}

func (f *FontInstaller) hashFile(path string) (string, error) {
	data, err := f.fs.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

func (f *FontInstaller) headETag(ctx context.Context, url string) (string, error) {
	// BYPASS: ports.HTTPClient only abstracts Do(), not request creation
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return "", err
	}
	resp, err := f.http.Do(req)
	if err != nil {
		return "", err
	}
	_ = resp.Body.Close()
	return resp.Header.Get("ETag"), nil
}

func (f *FontInstaller) downloadFonts(ctx context.Context, cachePath string, spinner ports.Spinner) error {
	dir := filepath.Dir(cachePath)
	base := filepath.Base(cachePath)

	if err := f.fs.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating parent dir: %w", err)
	}

	return utils.RetryHTTP(ctx, func() error {
		tmpDir, err := f.fs.MkdirTemp(dir, base)
		if err != nil {
			return err
		}
		defer f.fs.RemoveAll(tmpDir)
		tmpPath := filepath.Join(tmpDir, "data")

		req, err := http.NewRequestWithContext(ctx, "GET", f.fontURL, nil)
		if err != nil {
			return err
		}
		resp, err := f.http.Do(req)
		if err != nil {
			return fmt.Errorf("downloading fonts: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("downloading fonts: unexpected HTTP status: %s", resp.Status)
		}

		var reader io.Reader = resp.Body
		var progress ports.Progress
		if resp.ContentLength > 0 {
			spinner.Pause()
			progress = f.logger.Progress(resp.ContentLength, "Downloading extra fonts")
			reader = io.TeeReader(resp.Body, progress)
		}

		data, err := utils.ReadAllWithLimit(reader, 500*1024*1024)
		if err != nil {
			if progress != nil {
				progress.Fail()
			}
			return fmt.Errorf("downloading fonts: %w", err)
		}
		if progress != nil {
			progress.Done()
			spinner.Resume()
		}

		if err := f.fs.WriteFile(tmpPath, data, 0644); err != nil {
			return err
		}

		if err := f.fs.Rename(tmpPath, cachePath); err != nil {
			return err
		}
		return nil
	})
}

var tarEntryLimit int64 = 100 * 1024 * 1024

func (f *FontInstaller) extractTarGz(src, destDir string) error {
	data, err := f.fs.ReadFile(src)
	if err != nil {
		return err
	}

	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decompressing: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading archive: %w", err)
		}

		clean := filepath.Clean(filepath.Join(destDir, hdr.Name))
		if !strings.HasPrefix(clean, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("archive contains unsafe path: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := f.fs.MkdirAll(clean, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			dir := filepath.Dir(clean)
			if err := f.fs.MkdirAll(dir, 0755); err != nil {
				return err
			}
			content, err := utils.ReadAllWithLimit(tr, tarEntryLimit)
			if err != nil {
				return err
			}
			if err := f.fs.AtomicWriteFile(clean, content, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}
