// Package fetch downloads release artifacts and discovers upstream
// versions.
package fetch

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MaxSize caps a single download.
const MaxSize = 4 << 30

// IdleTimeout aborts a download that receives no data for this long. There
// is deliberately no total timeout: large files on slow links must work.
var IdleTimeout = 60 * time.Second

// NewClient returns the HTTP client used for all downloads: proxies from
// the environment, TLS 1.2+, and redirects only to https.
func NewClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
			ResponseHeaderTimeout: 30 * time.Second,
			TLSHandshakeTimeout:   15 * time.Second,
			ForceAttemptHTTP2:     true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" {
				return fmt.Errorf("refusing redirect to non-https URL %s", req.URL.Redacted())
			}
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
}

// ProgressFunc reports bytes received and total (-1 if unknown).
type ProgressFunc func(done, total int64)

// Download fetches url into dest atomically (dest.part, then rename). If
// sha is non-empty the content must match it (case-insensitive hex).
func Download(ctx context.Context, c *http.Client, url, dest, sha string, prog ProgressFunc) error {
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("refusing non-https URL %s", url)
	}
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "debforge")
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	if resp.ContentLength > MaxSize {
		return fmt.Errorf("download %s: too large (%d bytes)", url, resp.ContentLength)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	part := dest + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(part)
		}
	}()

	idle := time.AfterFunc(IdleTimeout, func() { cancel(fmt.Errorf("no data received for %s", IdleTimeout)) })
	defer idle.Stop()
	h := sha256.New()
	var done int64
	buf := make([]byte, 256*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			idle.Reset(IdleTimeout)
			done += int64(n)
			if done > MaxSize {
				return fmt.Errorf("download %s: exceeds %d bytes", url, int64(MaxSize))
			}
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			h.Write(buf[:n])
			if prog != nil {
				prog(done, resp.ContentLength)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, context.Canceled) {
				return fmt.Errorf("download %s: %w", url, cause)
			}
			return fmt.Errorf("download %s: %w", url, rerr)
		}
	}
	if sha != "" {
		got := hex.EncodeToString(h.Sum(nil))
		if !strings.EqualFold(got, sha) {
			return fmt.Errorf("download %s: sha256 mismatch: got %s, want %s", url, got, strings.ToLower(sha))
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(part, dest); err != nil {
		return err
	}
	ok = true
	return nil
}

// Exists reports whether url answers HEAD with a 2xx status after
// redirects. Any other status or error means "not available".
func Exists(ctx context.Context, c *http.Client, url string) bool {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "debforge")
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
