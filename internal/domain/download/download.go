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
	"strings"

	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/textutil"
)

func ExpandURL(url, version string) string {
	return strings.ReplaceAll(url, "{version}", version)
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

func Download(ctx context.Context, fs ports.FileSystem, url, destPath string, spinner ports.Spinner, sha256Hex string) error {
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
		return fmt.Errorf("download %s: %s", url, resp.Status)
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
		f.Close()
		fs.RemoveAll(destPath)
		return err
	}
	if total > 0 && spinner != nil {
		cur := textutil.FormatSize(total)
		tot := textutil.FormatSize(total)
		spinner.SetDesc(fmt.Sprintf("Downloading %s... [%s/%s]", filename, cur, tot))
	}
	if err := f.Close(); err != nil {
		fs.RemoveAll(destPath)
		return err
	}

	if fi, err := fs.Stat(destPath); err == nil && fi.Size() == 0 {
		fs.RemoveAll(destPath)
		return fmt.Errorf("downloaded file is empty: %s", url)
	}

	if sha256Hex != "" {
		got := hex.EncodeToString(hash.Sum(nil))
		if got != sha256Hex {
			fs.RemoveAll(destPath)
			return fmt.Errorf("sha256 mismatch: expected %s, got %s", sha256Hex, got)
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
