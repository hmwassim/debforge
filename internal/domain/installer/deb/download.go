package deb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hmwassim/debforge/internal/ports"
)

func expandURL(url, version string) string {
	return strings.ReplaceAll(url, "{version}", version)
}

func humanSize(v int64) string {
	switch {
	case v >= 1000000000:
		return strconv.FormatFloat(float64(v)/1000000000, 'f', 1, 64) + "G"
	case v >= 1000000:
		return strconv.FormatFloat(float64(v)/1000000, 'f', 1, 64) + "M"
	case v >= 1000:
		return strconv.FormatInt(v/1000, 10) + "k"
	default:
		return strconv.FormatInt(v, 10)
	}
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
			cur := humanSize(pr.done)
			total := humanSize(pr.total)
			pr.spinner.SetDesc(fmt.Sprintf("Downloading %s... [%s/%s]", pr.filename, cur, total))
		}
	}
	return n, err
}

func Download(ctx context.Context, url, sha256Hex, destPath string, spinner ports.Spinner) error {
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
	filename := filepath.Base(url)

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}

	hash := sha256.New()
	body := io.TeeReader(resp.Body, hash)

	src := io.Reader(body)
	if total > 0 {
		src = &progressReader{
			reader:   body,
			total:    total,
			filename: filename,
			spinner:  spinner,
		}
	}

	if _, err := io.Copy(f, src); err != nil {
		f.Close()
		os.Remove(destPath)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(destPath)
		return err
	}

	if sha256Hex != "" {
		got := hex.EncodeToString(hash.Sum(nil))
		if got != sha256Hex {
			os.Remove(destPath)
			return fmt.Errorf("sha256 mismatch: expected %s, got %s", sha256Hex, got)
		}
	}

	return nil
}
