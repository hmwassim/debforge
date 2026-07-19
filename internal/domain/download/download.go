// Package download fetches remote files over HTTPS with progress reporting
// and optional SHA-256 verification.
package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"

	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/textutil"
)

// ExpandURL replaces {version} placeholders in url with the given version.
func ExpandURL(url, version string) string {
	return textutil.ExpandVersion(url, version)
}

type progressReader struct {
	reader   io.Reader
	total    int64
	done     int64
	lastPct  int
	filename string
	spinner  ports.Spinner
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.done += int64(n)
		pct := int(pr.done * 100 / pr.total)
		if pct > pr.lastPct {
			pr.lastPct = pct
			cur := textutil.FormatSize(pr.done)
			total := textutil.FormatSize(pr.total)
			pr.spinner.SetDesc(fmt.Sprintf("Downloading %s... [%s/%s]", pr.filename, cur, total))
		}
	}
	return n, err
}

// Download fetches a file from url over HTTPS, writes it to destPath,
// optionally verifies its SHA-256 hash, and reports progress via spinner.
func Download(ctx context.Context, fs ports.FileSystem, url, destPath string, spinner ports.Spinner, sha256Hex string) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %q: %s", url, resp.Status)
	}

	if resp.Request.URL.Scheme != "https" {
		return fmt.Errorf("download %q: insecure connection (scheme=%q)", url, resp.Request.URL.Scheme)
	}

	total := resp.ContentLength
	filename := FilenameFromURL(url)

	if err := fs.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	f, err := fs.Create(destPath)
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		if err != nil {
			_ = fs.RemoveAll(destPath) // best-effort cleanup
		}
	}()

	hash := sha256.New()
	body := io.TeeReader(resp.Body, hash)

	src := io.Reader(body)
	if total > 0 && spinner != nil {
		src = &progressReader{
			reader:   body,
			total:    total,
			filename: filename,
			spinner:  spinner,
		}
	}

	if _, err := io.Copy(f, src); err != nil {
		return err
	}
	if total > 0 && spinner != nil {
		cur := textutil.FormatSize(total)
		tot := textutil.FormatSize(total)
		spinner.SetDesc(fmt.Sprintf("Downloading %s... [%s/%s]", filename, cur, tot))
	}
	if err := f.Close(); err != nil {
		return err
	}

	if fi, err := fs.Stat(destPath); err == nil && fi.Size() == 0 {
		return fmt.Errorf("downloaded file is empty: %q", url)
	}

	if sha256Hex != "" {
		got := hex.EncodeToString(hash.Sum(nil))
		if got != sha256Hex {
			return fmt.Errorf("sha256 mismatch: expected %q, got %q", sha256Hex, got)
		}
	}

	return nil
}

// FilenameFromURL extracts the filename (last path segment) from a URL,
// stripping any query parameters or fragments.
func FilenameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Path == "" {
		return filepath.Base(rawURL)
	}
	return path.Base(u.Path)
}
