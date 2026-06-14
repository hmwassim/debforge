package core

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hmwassim/debforge/pkg/executil"
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

	if err := downloadFile(cachePath, "https://codeberg.org/hmwassim/fonts/raw/branch/main/fonts.tar.gz"); err != nil {
		return fmt.Errorf("downloading fonts: %w", err)
	}

	return extractFonts(cachePath, fontDir)
}

func downloadFile(path, url string) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	total := resp.ContentLength
	if total <= 0 {
		_, err = io.Copy(out, resp.Body)
		return err
	}

	start := time.Now()
	pb := &progressWriter{total: total, start: start}
	if _, err := io.Copy(out, io.TeeReader(resp.Body, pb)); err != nil {
		return err
	}
	pb.done()
	fmt.Fprintln(os.Stderr)
	return nil
}

type progressWriter struct {
	total     int64
	current   int64
	start     time.Time
	lastPrint time.Time
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.current += int64(n)
	if time.Since(w.lastPrint) < 100*time.Millisecond {
		return n, nil
	}
	w.lastPrint = time.Now()
	w.print()
	return n, nil
}

func (w *progressWriter) done() {
	w.current = w.total
	w.print()
}

func (w *progressWriter) print() {
	pct := float64(w.current) / float64(w.total) * 100
	barWidth := 40
	filled := int(float64(barWidth) * float64(w.current) / float64(w.total))
	bar := strings.Repeat("=", filled) + strings.Repeat("-", barWidth-filled)
	if filled < barWidth {
		bar = bar[:filled] + ">" + bar[filled+1:]
	}
	elapsed := time.Since(w.start)
	rate := float64(w.current) / elapsed.Seconds()
	var etaStr string
	if rate > 0 {
		remaining := time.Duration(float64(w.total-w.current)/rate) * time.Second
		etaStr = remaining.Truncate(time.Second).String()
	} else {
		etaStr = "?"
	}
	fmt.Fprintf(os.Stderr, "\033[2K\r  [%s] %3.0f%%  ETA %s", bar, pct, etaStr)
}

func extractFonts(path, fontDir string) error {
	if err := os.MkdirAll(fontDir, 0755); err != nil {
		return err
	}
	extract := exec.Command("tar", "-xzf", path, "-C", fontDir)
	extract.Stdout = nil
	if err := executil.Run(extract); err != nil {
		return fmt.Errorf("extracting fonts: %w", err)
	}
	return executil.Run(exec.Command("fc-cache", "-f", "-v"))
}
