package packages

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hmwassim/debforge/pkg/cli"
	"github.com/hmwassim/debforge/pkg/executil"
	"github.com/hmwassim/debforge/pkg/text"
)

var httpClient = &http.Client{
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	},
	Timeout: 0,
}

func DownloadFile(path, url string) error {
	tmp := path + ".tmp"

	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	abort := true
	defer func() {
		if f != nil {
			f.Close()
		}
		if abort {
			os.Remove(tmp)
		}
	}()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "debforge/"+cli.Version)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: unexpected HTTP status: %s", url, resp.Status)
	}

	total := resp.ContentLength
	if total <= 0 {
		_, err = io.Copy(f, resp.Body)
		if err != nil {
			return err
		}
	} else {
		start := time.Now()
		pb := &progressWriter{total: total, start: start}
		if _, err := io.Copy(f, io.TeeReader(resp.Body, pb)); err != nil {
			return err
		}
		pb.done()
		fmt.Fprintln(os.Stderr)
	}

	if err := f.Close(); err != nil {
		return err
	}
	f = nil
	abort = false

	return os.Rename(tmp, path)
}

func ExtractTarGz(src, dest string) error {
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	extract := exec.Command("tar", "-xzf", src, "-C", dest)
	extract.Stdout = io.Discard
	return executil.Run(extract)
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
	if !text.IsTerminal(os.Stderr) {
		return
	}
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
